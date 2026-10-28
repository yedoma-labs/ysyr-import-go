package ysyrimport

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// cell is the destination row used by the column tests.
type cell struct {
	S   string
	I   int
	I64 int64
	F   float64
	B   bool
	D   time.Time
	E   string
}

type colCase[V any] struct {
	name string
	raw  string
	want V
	fail bool
}

// runRule applies rule to raw and returns the row that was filled, or the error.
func runRule[T any](rule Rule[T], raw string) (T, error) {
	var dst T
	err := rule.apply(raw, &dst)
	return dst, err
}

func TestColumnHelpers(t *testing.T) {
	t.Run("string trims whitespace", func(t *testing.T) {
		rule := StringColumn("s", func(c *cell, v string) { c.S = v })
		for _, tt := range []colCase[string]{
			{name: "plain", raw: "abc", want: "abc"},
			{name: "surrounding spaces", raw: "  abc \t", want: "abc"},
			{name: "inner spaces kept", raw: "a b", want: "a b"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				got, err := runRule(rule, tt.raw)
				if err != nil || got.S != tt.want {
					t.Fatalf("got %q, %v; want %q", got.S, err, tt.want)
				}
			})
		}
	})

	t.Run("int", func(t *testing.T) {
		rule := IntColumn("i", func(c *cell, v int) { c.I = v })
		for _, tt := range []colCase[int]{
			{name: "positive", raw: "34", want: 34},
			{name: "negative", raw: "-42", want: -42},
			{name: "trimmed", raw: " 7 ", want: 7},
			{name: "decimal rejected", raw: "1.5", fail: true},
			{name: "word rejected", raw: "forty", fail: true},
			{name: "underscore rejected", raw: "1_000", fail: true},
			{name: "overflow rejected", raw: "99999999999999999999999", fail: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				got, err := runRule(rule, tt.raw)
				if tt.fail {
					if err == nil {
						t.Fatalf("want error, got %d", got.I)
					}
					return
				}
				if err != nil || got.I != tt.want {
					t.Fatalf("got %d, %v; want %d", got.I, err, tt.want)
				}
			})
		}
	})

	t.Run("int64", func(t *testing.T) {
		rule := Int64Column("i", func(c *cell, v int64) { c.I64 = v })
		got, err := runRule(rule, "9223372036854775807")
		if err != nil || got.I64 != 9223372036854775807 {
			t.Fatalf("max int64: got %d, %v", got.I64, err)
		}
		if _, err := runRule(rule, "9223372036854775808"); err == nil {
			t.Fatal("overflow must be rejected")
		}
	})

	t.Run("float", func(t *testing.T) {
		rule := FloatColumn("f", func(c *cell, v float64) { c.F = v })
		for _, tt := range []colCase[float64]{
			{name: "decimal", raw: "1.5", want: 1.5},
			{name: "negative", raw: "-2", want: -2},
			{name: "exponent", raw: "-2e3", want: -2000},
			{name: "leading dot", raw: ".5", want: 0.5},
			{name: "NaN rejected", raw: "NaN", fail: true},
			{name: "infinity rejected", raw: "Inf", fail: true},
			{name: "hex rejected", raw: "0x1p4", fail: true},
			{name: "comma rejected", raw: "1,5", fail: true},
			{name: "overflow rejected", raw: "1e400", fail: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				got, err := runRule(rule, tt.raw)
				if tt.fail {
					if err == nil {
						t.Fatalf("want error, got %v", got.F)
					}
					return
				}
				if err != nil || got.F != tt.want {
					t.Fatalf("got %v, %v; want %v", got.F, err, tt.want)
				}
			})
		}
	})

	t.Run("bool", func(t *testing.T) {
		rule := BoolColumn("b", func(c *cell, v bool) { c.B = v })
		for _, tt := range []colCase[bool]{
			{name: "true", raw: "true", want: true},
			{name: "TRUE upper case", raw: "TRUE", want: true},
			{name: "one", raw: "1", want: true},
			{name: "false", raw: "False", want: false},
			{name: "zero", raw: "0", want: false},
			{name: "yes rejected", raw: "yes", fail: true},
			{name: "letter rejected", raw: "x", fail: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				got, err := runRule(rule, tt.raw)
				if tt.fail {
					if err == nil {
						t.Fatalf("want error, got %v", got.B)
					}
					return
				}
				if err != nil || got.B != tt.want {
					t.Fatalf("got %v, %v; want %v", got.B, err, tt.want)
				}
			})
		}
	})

	t.Run("date", func(t *testing.T) {
		rule := DateColumn("d", func(c *cell, v time.Time) { c.D = v })
		utc := func(y int, m time.Month, d int) time.Time {
			return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		}
		for _, tt := range []colCase[time.Time]{
			{name: "valid date", raw: "2026-03-14", want: utc(2026, 3, 14)},
			{name: "leap day", raw: "2028-02-29", want: utc(2028, 2, 29)},
			{name: "trimmed", raw: " 2026-01-01 ", want: utc(2026, 1, 1)},
			{name: "impossible day rejected", raw: "2026-02-30", fail: true},
			{name: "non-leap day rejected", raw: "2026-02-29", fail: true},
			{name: "German format rejected", raw: "14.03.2026", fail: true},
			{name: "single digit month rejected", raw: "2026-3-14", fail: true},
			{name: "time part rejected", raw: "2026-03-14T10:00:00Z", fail: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				got, err := runRule(rule, tt.raw)
				if tt.fail {
					if err == nil {
						t.Fatalf("want error, got %v", got.D)
					}
					return
				}
				if err != nil || !reflect.DeepEqual(got.D, tt.want) {
					t.Fatalf("got %v, %v; want %v", got.D, err, tt.want)
				}
			})
		}
	})

	t.Run("enum", func(t *testing.T) {
		rule := EnumColumn("p", []string{"free", "pro"}, func(c *cell, v string) { c.E = v })
		for _, tt := range []colCase[string]{
			{name: "first value", raw: "free", want: "free"},
			{name: "second value trimmed", raw: " pro ", want: "pro"},
			{name: "case-sensitive", raw: "Pro", fail: true},
			{name: "unknown value", raw: "enterprise", fail: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				got, err := runRule(rule, tt.raw)
				if tt.fail {
					if err == nil || err.Error() != "expected one of: free, pro" {
						t.Fatalf("want enum error, got %v", err)
					}
					return
				}
				if err != nil || got.E != tt.want {
					t.Fatalf("got %q, %v; want %q", got.E, err, tt.want)
				}
			})
		}
	})

	t.Run("custom column passes raw value and maps parse error", func(t *testing.T) {
		rule := Column("c",
			func(raw string) (string, error) {
				if raw != " mixed Case " {
					return "", errors.New("raw value was modified")
				}
				return "lower", nil
			},
			func(c *cell, v string) { c.S = v },
		)
		got, err := runRule(rule, " mixed Case ")
		if err != nil || got.S != "lower" {
			t.Fatalf("got %q, %v", got.S, err)
		}
	})
}
