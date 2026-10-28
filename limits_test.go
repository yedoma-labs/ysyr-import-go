package ysyrimport

import (
	"errors"
	"strings"
	"testing"
)

type nameRow struct {
	Name string
}

func nameSchema() Schema[nameRow] {
	return Schema[nameRow]{
		StringColumn("name", func(r *nameRow, v string) { r.Name = v }),
	}
}

// importLimited runs Import on input with the name schema and checks that a limit error leaves
// no partial result behind.
func importLimited(t *testing.T, input string, opts ...Option) error {
	t.Helper()
	res, err := Import(strings.NewReader(input), nameSchema(), opts...)
	if err != nil {
		if len(res.Accepted) != 0 || len(res.Rejected) != 0 {
			t.Fatalf("a failed import must return a zero Result, got %+v", res)
		}
	}
	return err
}

func TestLimitMaxBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		limit   int64
		wantErr bool
	}{
		{name: "ascii input equal to limit", input: "name\nabc\n", limit: 9},
		{name: "ascii input one byte over limit", input: "name\nabc\n", limit: 8, wantErr: true},
		{name: "multi-byte rune counts as bytes", input: "name\nä\n", limit: 8},
		{name: "multi-byte rune over byte limit", input: "name\nä\n", limit: 7, wantErr: true},
		{name: "zero limit rejects any input", input: "name\n", limit: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := importLimited(t, tt.input, WithMaxBytes(tt.limit))
			assertLimit(t, err, tt.wantErr)
		})
	}
}

func TestLimitMaxBytesDefault(t *testing.T) {
	const defaultMaxBytes = 10 << 20
	if got := defaultConfig().maxBytes; got != defaultMaxBytes {
		t.Fatalf("default maxBytes = %d, want %d", got, defaultMaxBytes)
	}
	over := "name\n" + strings.Repeat("a", defaultMaxBytes)
	assertLimit(t, importLimited(t, over), true)
}

func TestLimitMaxRows(t *testing.T) {
	rows := func(n int) string {
		return "name\n" + strings.Repeat("a\n", n)
	}
	tests := []struct {
		name    string
		input   string
		limit   int
		wantErr bool
	}{
		{name: "rows equal to limit", input: rows(3), limit: 3},
		{name: "one row over limit", input: rows(4), limit: 3, wantErr: true},
		{name: "blank lines do not count", input: "name\n\na\n\na\n\na\n\n", limit: 3},
		{name: "broken rows count as rows", input: "name\na,b\na,b\na,b\na,b\n", limit: 3, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := importLimited(t, tt.input, WithMaxRows(tt.limit))
			assertLimit(t, err, tt.wantErr)
		})
	}
}

func TestLimitMaxRowsDefault(t *testing.T) {
	const defaultMaxRows = 100_000
	atLimit := "name\n" + strings.Repeat("a\n", defaultMaxRows)
	assertLimit(t, importLimited(t, atLimit), false)

	overLimit := "name\n" + strings.Repeat("a\n", defaultMaxRows+1)
	assertLimit(t, importLimited(t, overLimit), true)
}

func TestLimitMaxFieldLength(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		limit   int
		wantErr bool
	}{
		{name: "field equal to limit", input: "name\n" + strings.Repeat("a", 10) + "\n", limit: 10},
		{name: "field one over limit", input: "name\n" + strings.Repeat("a", 11) + "\n", limit: 10, wantErr: true},
		{name: "limit counts runes not bytes", input: "name\n" + strings.Repeat("ä", 10) + "\n", limit: 10},
		{name: "multi-byte field over rune limit", input: "name\n" + strings.Repeat("ä", 10) + "\n", limit: 9, wantErr: true},
		{name: "header field counts too", input: "nameeeeeeeeee\na\n", limit: 10, wantErr: true},
		{name: "quoted field counts its content", input: "name\n\"" + strings.Repeat("a", 11) + "\"\n", limit: 10, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := importLimited(t, tt.input, WithMaxFieldLength(tt.limit))
			assertLimit(t, err, tt.wantErr)
		})
	}
}

func TestLimitMaxFieldLengthDefault(t *testing.T) {
	const defaultMaxFieldLength = 10_000
	atLimit := "name\n" + strings.Repeat("a", defaultMaxFieldLength) + "\n"
	assertLimit(t, importLimited(t, atLimit), false)

	overLimit := "name\n" + strings.Repeat("a", defaultMaxFieldLength+1) + "\n"
	assertLimit(t, importLimited(t, overLimit), true)
}

// assertLimit checks that err is ErrLimitExceeded when wantErr is set, and nil otherwise.
func assertLimit(t *testing.T, err error, wantErr bool) {
	t.Helper()
	if wantErr {
		if !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("want ErrLimitExceeded, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
