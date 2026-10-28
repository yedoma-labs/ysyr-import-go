package ysyrimport

import (
	"io"
	"strconv"
	"strings"
)

// reportHeader is the first line of every error report.
var reportHeader = []string{"line", "row", "column", "code", "message"}

// WriteErrorReport writes the rejected rows as CSV with the columns line, row, column, code and
// message. The output starts with a header line. Lines end with CRLF, as RFC 4180 describes.
// A cell is quoted when it contains a comma, a double quote or a line break, and quotes inside a
// cell are doubled.
//
// CSV injection protection: a text cell that starts with =, +, -, @, a tab or a carriage return
// is prefixed with an apostrophe. Spreadsheet programs then show the text instead of evaluating
// it as a formula. The apostrophe is part of the cell, so a reader sees the prefix.
//
// The writer is written by hand instead of using encoding/csv, because encoding/csv drops bare
// carriage returns when CRLF line endings are enabled. A report must reproduce every cell.
func WriteErrorReport(w io.Writer, rejected []RowError) error {
	if err := writeLine(w, reportHeader); err != nil {
		return err
	}
	for _, e := range rejected {
		err := writeLine(w, []string{
			strconv.Itoa(e.Line),
			strconv.Itoa(e.Row),
			guardCell(e.Column),
			guardCell(string(e.Code)),
			guardCell(e.Message),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// writeLine writes one CSV line with CRLF as line ending.
func writeLine(w io.Writer, cells []string) error {
	var b strings.Builder
	for i, cell := range cells {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(quoteCell(cell))
	}
	b.WriteString("\r\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// quoteCell quotes a cell per RFC 4180 when it contains a delimiter, a quote or a line break.
func quoteCell(s string) string {
	if !strings.ContainsAny(s, ",\"\r\n") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// guardCell prefixes a text cell with an apostrophe if spreadsheets would read it as a formula.
func guardCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
