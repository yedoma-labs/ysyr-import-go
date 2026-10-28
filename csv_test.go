package ysyrimport

import (
	"errors"
	"reflect"
	"testing"
)

func defaultParseOptions() parseOptions {
	return parseOptions{delimiter: ",", maxRows: 100_000, maxFieldLength: 10_000}
}

func TestParseCSVRecords(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		delimiter string
		want      []record
	}{
		{
			name:  "plain records with LF",
			input: "a,b\n1,2\n",
			want: []record{
				{line: 1, values: []string{"a", "b"}},
				{line: 2, values: []string{"1", "2"}},
			},
		},
		{
			name:  "CRLF line endings",
			input: "a,b\r\n1,2\r\n",
			want: []record{
				{line: 1, values: []string{"a", "b"}},
				{line: 2, values: []string{"1", "2"}},
			},
		},
		{
			name:  "bare CR line endings",
			input: "a,b\r1,2\r",
			want: []record{
				{line: 1, values: []string{"a", "b"}},
				{line: 2, values: []string{"1", "2"}},
			},
		},
		{
			name:  "quoted delimiter",
			input: `"a,b",c`,
			want:  []record{{line: 1, values: []string{"a,b", "c"}}},
		},
		{
			name:  "doubled quote is an escaped quote",
			input: `"say ""hi""",x`,
			want:  []record{{line: 1, values: []string{`say "hi"`, "x"}}},
		},
		{
			name:  "empty quoted field is kept",
			input: `"",x`,
			want:  []record{{line: 1, values: []string{"", "x"}}},
		},
		{
			name:  "embedded LF keeps the record start line",
			input: "\"a\nb\",c\nd,e",
			want: []record{
				{line: 1, values: []string{"a\nb", "c"}},
				{line: 3, values: []string{"d", "e"}},
			},
		},
		{
			name:  "embedded CRLF inside quotes counts one line",
			input: "\"a\r\nb\"\r\nx",
			want: []record{
				{line: 1, values: []string{"a\r\nb"}},
				{line: 3, values: []string{"x"}},
			},
		},
		{
			name:  "empty fields",
			input: "a,,c",
			want:  []record{{line: 1, values: []string{"a", "", "c"}}},
		},
		{
			name:  "trailing delimiter yields a trailing empty field",
			input: "a,b,",
			want:  []record{{line: 1, values: []string{"a", "b", ""}}},
		},
		{
			name:  "blank lines are skipped but count as physical lines",
			input: "a\n\n\nb\n",
			want: []record{
				{line: 1, values: []string{"a"}},
				{line: 4, values: []string{"b"}},
			},
		},
		{
			name:  "no trailing newline",
			input: "a\nb",
			want: []record{
				{line: 1, values: []string{"a"}},
				{line: 2, values: []string{"b"}},
			},
		},
		{
			name:  "single trailing newline does not create a record",
			input: "a\n",
			want:  []record{{line: 1, values: []string{"a"}}},
		},
		{
			name:  "empty input has no records",
			input: "",
			want:  nil,
		},
		{
			name:  "only blank lines have no records",
			input: "\n\r\n\r",
			want:  nil,
		},
		{
			name:  "BOM is removed",
			input: "\uFEFFa,b\n",
			want:  []record{{line: 1, values: []string{"a", "b"}}},
		},
		{
			name:  "quote in the middle of an unquoted field is literal",
			input: `a"b,c`,
			want:  []record{{line: 1, values: []string{`a"b`, "c"}}},
		},
		{
			name:      "semicolon delimiter",
			input:     "a;b\n1;\"x;y\"",
			delimiter: ";",
			want: []record{
				{line: 1, values: []string{"a", "b"}},
				{line: 2, values: []string{"1", "x;y"}},
			},
		},
		{
			name:      "multi-byte delimiter",
			input:     "a§b\n1§2",
			delimiter: "§",
			want: []record{
				{line: 1, values: []string{"a", "b"}},
				{line: 2, values: []string{"1", "2"}},
			},
		},
		{
			name:  "unterminated quote runs to end of input",
			input: "a,\"b\nc\nd",
			want: []record{
				{line: 1, values: []string{"a", "b\nc\nd"}, quoteErr: "unterminated quoted field"},
			},
		},
		{
			name:  "characters after closing quote are a quoting error",
			input: "\"a\"b,c",
			want: []record{
				{line: 1, values: []string{"ab", "c"}, quoteErr: "unexpected character after closing quote"},
			},
		},
		{
			name:  "quoting error does not stop the following records",
			input: "\"a\"x\n1,2",
			want: []record{
				{line: 1, values: []string{"ax"}, quoteErr: "unexpected character after closing quote"},
				{line: 2, values: []string{"1", "2"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultParseOptions()
			if tt.delimiter != "" {
				opts.delimiter = tt.delimiter
			}
			got, err := parseCSV(tt.input, opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("records mismatch\n got: %#v\nwant: %#v", got, tt.want)
			}
		})
	}
}

func TestParseCSVRecordLinesWithEmbeddedBreaks(t *testing.T) {
	// Physical lines: 1 header, 2-3 record with quoted LF, 4 blank, 5-6 record with quoted CRLF,
	// 7 last record.
	input := "h1,h2\n" +
		"\"x\ny\",1\n" +
		"\n" +
		"2,\"p\r\nq\"\r\n" +
		"3,4\n"
	got, err := parseCSV(input, defaultParseOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantLines := []int{1, 2, 5, 7}
	if len(got) != len(wantLines) {
		t.Fatalf("got %d records, want %d", len(got), len(wantLines))
	}
	for i, want := range wantLines {
		if got[i].line != want {
			t.Errorf("record %d: line %d, want %d", i, got[i].line, want)
		}
	}
}

func TestParseCSVLimits(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		maxRows int
		maxLen  int
		wantErr bool
	}{
		{name: "rows exactly at limit", input: "h\n1\n2\n", maxRows: 2, maxLen: 10},
		{name: "rows one over limit", input: "h\n1\n2\n3\n", maxRows: 2, maxLen: 10, wantErr: true},
		{name: "blank lines do not count as rows", input: "h\n\n1\n\n2\n", maxRows: 2, maxLen: 10},
		{name: "field exactly at limit", input: "h\nabcde\n", maxRows: 10, maxLen: 5},
		{name: "field one over limit", input: "h\nabcdef\n", maxRows: 10, maxLen: 5, wantErr: true},
		{name: "limit counts runes not bytes", input: "h\näää\n", maxRows: 10, maxLen: 3},
		{name: "header field counts too", input: "abcdefg\n1\n", maxRows: 10, maxLen: 6, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseOptions{delimiter: ",", maxRows: tt.maxRows, maxFieldLength: tt.maxLen}
			_, err := parseCSV(tt.input, opts)
			if tt.wantErr {
				if !errors.Is(err, ErrLimitExceeded) {
					t.Fatalf("want ErrLimitExceeded, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
