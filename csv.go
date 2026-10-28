package ysyrimport

import (
	"strings"
	"unicode/utf8"
)

// Parser decision: the input is scanned by our own RFC 4180 scanner, not by encoding/csv.
// encoding/csv stops at the first malformed record, and with LazyQuotes it accepts broken quoting
// silently. Partial success needs a different behaviour: a broken quote becomes one rejected row,
// and the scan continues with the next line. The report writer is hand-written as well (report.go),
// so encoding/csv is not used anywhere in the package.

const utf8BOM = "\uFEFF"

// record is one data record as found in the input. The first record of a parse is the header.
type record struct {
	// line is the physical line on which the record starts, starting at 1. For a record that
	// spans several lines through quoted line breaks, this is the first of them.
	line   int
	values []string
	// quoteErr is non-empty when the quoting of the record is broken. values then holds a
	// best-effort recovery and must not be used.
	quoteErr string
}

// parseOptions configures parseCSV.
type parseOptions struct {
	// delimiter is a single, valid UTF-8 character, not a quote and not a line break.
	delimiter      string
	maxRows        int
	maxFieldLength int
}

// parseCSV splits data into records. The first record is the header candidate.
//
// Rules: fields may be quoted with a double quote, a doubled quote inside quotes is a literal
// quote, and quoted fields may contain delimiters and line breaks. Line endings are \n, \r\n and
// \r. A leading UTF-8 byte order mark is removed. Blank lines are skipped and do not count as
// records.
//
// A quoting error does not abort the scan. It is attached to the record in which it occurs.
// An unterminated quote runs to the end of the input, as RFC 4180 implies, so it swallows the
// rest of the file into one record.
//
// maxRows counts data records, so the header does not count. maxFieldLength counts runes in
// each field, including the header fields. Both limits stop the scan with ErrLimitExceeded as
// soon as they are exceeded.
func parseCSV(data string, o parseOptions) ([]record, error) {
	src := strings.TrimPrefix(data, utf8BOM)
	n := len(src)
	var out []record
	i, line := 0, 1

	for i < n {
		startLine := line
		var values []string
		var quoteErr string
		quoted := false

		for {
			var value string
			if i < n && src[i] == '"' {
				quoted = true
				i++
				var sb strings.Builder
				segment := i
				closed := false
				for i < n {
					c := src[i]
					if c == '"' {
						if i+1 < n && src[i+1] == '"' {
							sb.WriteString(src[segment : i+1])
							i += 2
							segment = i
							continue
						}
						sb.WriteString(src[segment:i])
						i++
						closed = true
						break
					}
					if c == '\n' {
						line++
					} else if c == '\r' {
						line++
						if i+1 < n && src[i+1] == '\n' {
							i++
						}
					}
					i++
				}
				switch {
				case !closed:
					sb.WriteString(src[segment:n])
					quoteErr = firstSet(quoteErr, "unterminated quoted field")
				case i < n && !isTerminator(src, i, o.delimiter):
					quoteErr = firstSet(quoteErr, "unexpected character after closing quote")
					start := i
					for i < n && !isTerminator(src, i, o.delimiter) {
						i++
					}
					sb.WriteString(src[start:i])
				}
				value = sb.String()
			} else {
				start := i
				for i < n && !isTerminator(src, i, o.delimiter) {
					i++
				}
				value = src[start:i]
			}

			if utf8.RuneCountInString(value) > o.maxFieldLength {
				return nil, limitError("maxFieldLength", int64(o.maxFieldLength))
			}
			values = append(values, value)

			if i >= n {
				break
			}
			if strings.HasPrefix(src[i:], o.delimiter) {
				i += len(o.delimiter)
				continue
			}
			if src[i] == '\r' && i+1 < n && src[i+1] == '\n' {
				i += 2
			} else {
				i++
			}
			line++
			break
		}

		if quoted || quoteErr != "" || len(values) != 1 || values[0] != "" {
			out = append(out, record{line: startLine, values: values, quoteErr: quoteErr})
			if len(out)-1 > o.maxRows {
				return nil, limitError("maxRows", int64(o.maxRows))
			}
		}
	}
	return out, nil
}

// isTerminator reports whether position i ends an unquoted field: a line break or the delimiter.
func isTerminator(src string, i int, delimiter string) bool {
	c := src[i]
	return c == '\n' || c == '\r' || strings.HasPrefix(src[i:], delimiter)
}

func firstSet(current, message string) string {
	if current != "" {
		return current
	}
	return message
}
