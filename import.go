package ysyrimport

import (
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"unicode/utf8"
)

// RowErrorCode identifies the kind of a row-level problem.
type RowErrorCode string

const (
	// CodeRequired means a required cell is empty.
	CodeRequired RowErrorCode = "REQUIRED"
	// CodeParse means a column parser rejected the value of a cell. Column is set.
	CodeParse RowErrorCode = "PARSE"
	// CodeValidation means the row validator set by WithValidate rejected the row.
	CodeValidation RowErrorCode = "VALIDATION"
	// CodeUnknownColumn means a non-empty value sits in a column that is not part of the schema.
	// It is reported only with WithUnknownColumns(Reject).
	CodeUnknownColumn RowErrorCode = "UNKNOWN_COLUMN"
	// CodeWrongFieldCount means the row has a different number of fields than the header.
	CodeWrongFieldCount RowErrorCode = "WRONG_FIELD_COUNT"
	// CodeParseQuote means the quoting of the row is broken.
	CodeParseQuote RowErrorCode = "PARSE_QUOTE"
)

// RowError describes one problem in one row. A row with several problems yields one RowError per
// problem.
type RowError struct {
	// Row is the data row number, starting at 1. The header line is not counted.
	Row int
	// Line is the physical line on which the row starts, starting at 1. Use it to find the row in
	// an editor.
	Line int
	// Column is the column name, sanitized. It is empty for row-level problems.
	Column string
	Code   RowErrorCode
	// Message is an English text of at most 100 runes without control characters.
	Message string
}

// Summary counts the outcome of an import.
type Summary struct {
	// Total is the number of data rows. Blank lines are not counted.
	Total int
	// Accepted is the number of rows in Result.Accepted.
	Accepted int
	// Rejected is the number of distinct rejected rows. A row with several errors counts once.
	Rejected int
}

// Result is the outcome of Import. Accepted and Rejected are never nil.
type Result[T any] struct {
	// Accepted holds the valid rows in input order.
	Accepted []T
	// Rejected holds one entry per problem, in input order.
	Rejected []RowError
	Summary  Summary
}

// UnknownColumnPolicy decides what happens to columns that are not part of the schema.
type UnknownColumnPolicy int

const (
	// Ignore skips columns that are not part of the schema. This is the default.
	Ignore UnknownColumnPolicy = iota
	// Reject reports a non-empty value in such a column as CodeUnknownColumn.
	Reject
)

// Option configures Import.
type Option func(*config)

type config struct {
	delimiter      rune
	unknown        UnknownColumnPolicy
	maxBytes       int64
	maxRows        int
	maxFieldLength int
	validate       func(any) error
	validateType   reflect.Type
}

func defaultConfig() config {
	return config{
		delimiter:      ',',
		unknown:        Ignore,
		maxBytes:       10 << 20,
		maxRows:        100_000,
		maxFieldLength: 10_000,
	}
}

func (c config) check() error {
	switch {
	case !utf8.ValidRune(c.delimiter) || c.delimiter == utf8.RuneError || c.delimiter == '"' ||
		c.delimiter == '\r' || c.delimiter == '\n':
		return fmt.Errorf("ysyrimport: invalid delimiter %q", c.delimiter)
	case c.unknown != Ignore && c.unknown != Reject:
		return errors.New("ysyrimport: invalid unknown-column policy")
	case c.maxBytes < 0 || c.maxBytes == math.MaxInt64:
		return errors.New("ysyrimport: maxBytes must be between 0 and math.MaxInt64-1")
	case c.maxRows < 0:
		return errors.New("ysyrimport: maxRows must not be negative")
	case c.maxFieldLength < 0:
		return errors.New("ysyrimport: maxFieldLength must not be negative")
	}
	return nil
}

// WithDelimiter sets the field delimiter. The default is ','. The rune must not be a double
// quote or a line break.
func WithDelimiter(d rune) Option {
	return func(c *config) { c.delimiter = d }
}

// WithUnknownColumns sets the policy for columns that are not part of the schema. The default is
// Ignore.
func WithUnknownColumns(p UnknownColumnPolicy) Option {
	return func(c *config) { c.unknown = p }
}

// WithMaxBytes sets the maximum input size in bytes. The default is 10 MiB. A limit equal to the
// input size is accepted.
func WithMaxBytes(n int64) Option {
	return func(c *config) { c.maxBytes = n }
}

// WithMaxRows sets the maximum number of data rows. The default is 100000. The header and blank
// lines do not count.
func WithMaxRows(n int) Option {
	return func(c *config) { c.maxRows = n }
}

// WithMaxFieldLength sets the maximum length of one field in runes. The default is 10000.
func WithMaxFieldLength(n int) Option {
	return func(c *config) { c.maxFieldLength = n }
}

// WithValidate sets a row-level check. It runs only for rows that passed all column rules. An
// error returned by fn becomes a CodeValidation row error. The type parameter must be the row
// type of Import, otherwise Import returns an error.
func WithValidate[T any](fn func(T) error) Option {
	return func(c *config) {
		c.validate = func(v any) error {
			row, _ := v.(T)
			return fn(row)
		}
		c.validateType = reflect.TypeFor[T]()
	}
}

// Import reads CSV data from r and checks it against columns, with partial success. Valid rows
// go to Result.Accepted. Broken rows are listed in Result.Rejected with their line, row, column,
// code and message. The rest of the file is still processed.
//
// The first line is the header. Header names are trimmed. If a name occurs more than once, the
// first occurrence is used and the others are unknown columns.
//
// Import returns an error that matches ErrMissingColumn, ErrInvalidHeader or ErrLimitExceeded
// with errors.Is when the whole file cannot be imported. It returns a zero Result in that case.
// Errors for an invalid option or a mismatched WithValidate type are returned as well.
//
// Import does not store anything. Persisting Result.Accepted, and any transaction around it, is
// up to the caller.
func Import[T any](r io.Reader, columns Schema[T], opts ...Option) (Result[T], error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := cfg.check(); err != nil {
		return Result[T]{}, err
	}
	if cfg.validateType != nil && cfg.validateType != reflect.TypeFor[T]() {
		return Result[T]{}, errors.New("ysyrimport: WithValidate type does not match the row type of Import")
	}

	raw, err := io.ReadAll(io.LimitReader(r, cfg.maxBytes+1))
	if err != nil {
		return Result[T]{}, fmt.Errorf("ysyrimport: read input: %w", err)
	}
	if int64(len(raw)) > cfg.maxBytes {
		return Result[T]{}, limitError("maxBytes", cfg.maxBytes)
	}

	records, err := parseCSV(string(raw), parseOptions{
		delimiter:      string(cfg.delimiter),
		maxRows:        cfg.maxRows,
		maxFieldLength: cfg.maxFieldLength,
	})
	if err != nil {
		return Result[T]{}, err
	}

	var header []string
	var body []record
	if len(records) > 0 {
		head := records[0]
		if head.quoteErr != "" {
			return Result[T]{}, fmt.Errorf("%w: the header line has broken quoting", ErrInvalidHeader)
		}
		header = make([]string, len(head.values))
		for i, name := range head.values {
			header[i] = strings.TrimSpace(name)
		}
		body = records[1:]
	}

	ck, err := newChecker(columns, header, cfg)
	if err != nil {
		return Result[T]{}, err
	}

	res := Result[T]{Accepted: []T{}, Rejected: []RowError{}}
	rejectedRows := 0
	for i, rec := range body {
		value, errs, ok := ck.check(rec, i+1)
		if ok {
			res.Accepted = append(res.Accepted, value)
			continue
		}
		res.Rejected = append(res.Rejected, errs...)
		rejectedRows++
	}
	res.Summary = Summary{
		Total:    len(body),
		Accepted: len(res.Accepted),
		Rejected: rejectedRows,
	}
	return res, nil
}

// checker holds what is needed to check the rows of one import.
type checker[T any] struct {
	columns  Schema[T]
	width    int
	index    map[string]int
	unmapped []unmappedColumn
	policy   UnknownColumnPolicy
	validate func(any) error
}

// unmappedColumn is a header column that is not used by the schema.
type unmappedColumn struct {
	index     int
	name      string
	duplicate bool
}

// newChecker validates the schema against the header and prepares the checks. It returns
// ErrMissingColumn when a required column is absent.
func newChecker[T any](columns Schema[T], header []string, cfg config) (*checker[T], error) {
	schemaNames := make(map[string]bool, len(columns))
	for _, rule := range columns {
		if schemaNames[rule.name] {
			return nil, fmt.Errorf("ysyrimport: duplicate schema column %q", rule.name)
		}
		schemaNames[rule.name] = true
	}

	index := make(map[string]int, len(header))
	for i, name := range header {
		if _, seen := index[name]; !seen {
			index[name] = i
		}
	}

	var missing []string
	for _, rule := range columns {
		if _, ok := index[rule.name]; !ok && rule.required {
			missing = append(missing, rule.name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrMissingColumn, strings.Join(missing, ", "))
	}

	ck := &checker[T]{
		columns:  columns,
		width:    len(header),
		index:    index,
		policy:   cfg.unknown,
		validate: cfg.validate,
	}
	if cfg.unknown == Reject {
		for i, name := range header {
			primary := index[name] == i
			if !primary || !schemaNames[name] {
				ck.unmapped = append(ck.unmapped, unmappedColumn{
					index:     i,
					name:      sanitizeText(name),
					duplicate: !primary,
				})
			}
		}
	}
	return ck, nil
}

// check validates one record. It returns the typed row and true when the row is accepted.
// Otherwise it returns the row errors. A row with a quoting error or a wrong field count is
// rejected before any cell is looked at.
func (c *checker[T]) check(rec record, row int) (T, []RowError, bool) {
	var zero T
	at := func(code RowErrorCode, column, message string) RowError {
		return RowError{
			Row:     row,
			Line:    rec.line,
			Column:  sanitizeText(column),
			Code:    code,
			Message: sanitizeText(message),
		}
	}

	if rec.quoteErr != "" {
		return zero, []RowError{at(CodeParseQuote, "", rec.quoteErr)}, false
	}
	if len(rec.values) != c.width {
		message := fmt.Sprintf("expected %d fields, found %d", c.width, len(rec.values))
		return zero, []RowError{at(CodeWrongFieldCount, "", message)}, false
	}

	var value T
	var errs []RowError
	for _, rule := range c.columns {
		raw := ""
		if i, ok := c.index[rule.name]; ok {
			raw = rec.values[i]
		}
		if strings.TrimSpace(raw) == "" {
			if rule.required {
				errs = append(errs, at(CodeRequired, rule.name, "value is required"))
			}
			continue
		}
		if err := rule.apply(raw, &value); err != nil {
			errs = append(errs, at(CodeParse, rule.name, errorText(err)))
		}
	}

	if c.policy == Reject {
		for _, u := range c.unmapped {
			if strings.TrimSpace(rec.values[u.index]) == "" {
				continue
			}
			message := "column is not part of the schema"
			if u.duplicate {
				message = "duplicate column name"
			}
			errs = append(errs, at(CodeUnknownColumn, u.name, message))
		}
	}

	if len(errs) > 0 {
		return zero, errs, false
	}
	if c.validate != nil {
		if err := c.validate(value); err != nil {
			return zero, []RowError{at(CodeValidation, "", errorText(err))}, false
		}
	}
	return value, nil, true
}
