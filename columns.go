package ysyrimport

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Schema is the ordered list of column rules for the row type T. The order decides in which
// sequence the cells of one row are checked, and therefore the order of its row errors.
//
// Go cannot set struct fields generically, so every rule carries a setter. A rule is created
// with Column or with one of the typed helpers and placed in a composite literal:
//
//	schema := ysyrimport.Schema[Customer]{
//		ysyrimport.StringColumn("email", func(c *Customer, v string) { c.Email = v }, ysyrimport.Required()),
//		ysyrimport.IntColumn("age", func(c *Customer, v int) { c.Age = v }),
//	}
type Schema[T any] []Rule[T]

// Rule describes how one CSV column is parsed into a field of T.
type Rule[T any] struct {
	name     string
	required bool
	// apply parses raw, a non-empty cell, and stores the value in dst. It returns the parse error.
	apply func(raw string, dst *T) error
}

type columnSettings struct {
	required bool
}

// ColumnOption configures a column rule.
type ColumnOption func(*columnSettings)

// Required makes an empty cell a CodeRequired row error. Without it, an empty cell leaves the
// field at its zero value.
func Required() ColumnOption {
	return func(s *columnSettings) { s.required = true }
}

// Column creates a rule for the column name. parse converts the raw cell text into a value of
// type V, and set stores that value in the row. Cells are passed to parse untrimmed, except that
// empty and whitespace-only cells are handled before parse is called. An error returned by parse
// becomes a CodeParse row error with the error's message.
func Column[T, V any](name string, parse func(raw string) (V, error), set func(*T, V), opts ...ColumnOption) Rule[T] {
	var s columnSettings
	for _, opt := range opts {
		opt(&s)
	}
	return Rule[T]{
		name:     name,
		required: s.required,
		apply: func(raw string, dst *T) error {
			v, err := parse(raw)
			if err != nil {
				return err
			}
			set(dst, v)
			return nil
		},
	}
}

// StringColumn creates a rule for a text column. Surrounding whitespace is removed.
func StringColumn[T any](name string, set func(*T, string), opts ...ColumnOption) Rule[T] {
	return Column(name, func(raw string) (string, error) {
		return strings.TrimSpace(raw), nil
	}, set, opts...)
}

// IntColumn creates a rule for a whole number in the int range, for example -42.
func IntColumn[T any](name string, set func(*T, int), opts ...ColumnOption) Rule[T] {
	return Column(name, func(raw string) (int, error) {
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 0)
		if err != nil {
			return 0, errors.New("expected an integer")
		}
		return int(v), nil
	}, set, opts...)
}

// Int64Column creates a rule for a whole number in the int64 range.
func Int64Column[T any](name string, set func(*T, int64), opts ...ColumnOption) Rule[T] {
	return Column(name, func(raw string) (int64, error) {
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return 0, errors.New("expected an integer")
		}
		return v, nil
	}, set, opts...)
}

// decimalPattern accepts plain decimal notation such as 1, -2.5, .5 or 3e4. It rejects NaN,
// infinities, hexadecimal floats and underscores.
var decimalPattern = regexp.MustCompile(`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// FloatColumn creates a rule for a decimal number with a dot as separator, for example 1.5.
func FloatColumn[T any](name string, set func(*T, float64), opts ...ColumnOption) Rule[T] {
	return Column(name, func(raw string) (float64, error) {
		s := strings.TrimSpace(raw)
		if !decimalPattern.MatchString(s) {
			return 0, errors.New("expected a decimal number")
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, errors.New("expected a decimal number")
		}
		return v, nil
	}, set, opts...)
}

// BoolColumn creates a rule for a boolean. The accepted values are true, false, 1 and 0,
// case-insensitively.
func BoolColumn[T any](name string, set func(*T, bool), opts ...ColumnOption) Rule[T] {
	return Column(name, func(raw string) (bool, error) {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true", "1":
			return true, nil
		case "false", "0":
			return false, nil
		default:
			return false, errors.New("expected true, false, 1 or 0")
		}
	}, set, opts...)
}

// DateColumn creates a rule for a calendar date in the form YYYY-MM-DD. The value is midnight UTC.
// Dates that do not exist, such as 2026-02-30, are rejected.
func DateColumn[T any](name string, set func(*T, time.Time), opts ...ColumnOption) Rule[T] {
	return Column(name, func(raw string) (time.Time, error) {
		v, err := time.Parse(time.DateOnly, strings.TrimSpace(raw))
		if err != nil {
			return time.Time{}, errors.New("expected a date in the form YYYY-MM-DD")
		}
		return v, nil
	}, set, opts...)
}

// EnumColumn creates a rule that accepts exactly one of values. The comparison is case-sensitive.
// The type E is any string-based type, so a set of constants can be used directly.
func EnumColumn[T any, E ~string](name string, values []E, set func(*T, E), opts ...ColumnOption) Rule[T] {
	allowed := slices.Clone(values)
	names := make([]string, len(allowed))
	for i, v := range allowed {
		names[i] = string(v)
	}
	expected := fmt.Sprintf("expected one of: %s", strings.Join(names, ", "))
	return Column(name, func(raw string) (E, error) {
		v := E(strings.TrimSpace(raw))
		if slices.Contains(allowed, v) {
			return v, nil
		}
		return "", errors.New(expected)
	}, set, opts...)
}
