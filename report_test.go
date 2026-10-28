package ysyrimport

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// reportParseOptions are the parser options used to read reports back.
func reportParseOptions() parseOptions {
	return parseOptions{delimiter: ",", maxRows: 100_000, maxFieldLength: 10_000}
}

func writeReport(t *testing.T, rejected []RowError) string {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteErrorReport(&buf, rejected); err != nil {
		t.Fatalf("WriteErrorReport: %v", err)
	}
	return buf.String()
}

func TestWriteErrorReportFormat(t *testing.T) {
	tests := []struct {
		name     string
		rejected []RowError
		want     string
	}{
		{
			name:     "header only without rows",
			rejected: nil,
			want:     "line,row,column,code,message\r\n",
		},
		{
			name: "rows as in the README example",
			rejected: []RowError{
				{Row: 2, Line: 3, Column: "name", Code: CodeRequired, Message: "value is required"},
				{Row: 3, Line: 4, Column: "age", Code: CodeParse, Message: "expected an integer"},
				{Row: 4, Line: 5, Code: CodeWrongFieldCount, Message: "expected 4 fields, found 3"},
			},
			want: "line,row,column,code,message\r\n" +
				"3,2,name,REQUIRED,value is required\r\n" +
				"4,3,age,PARSE,expected an integer\r\n" +
				"5,4,,WRONG_FIELD_COUNT,\"expected 4 fields, found 3\"\r\n",
		},
		{
			name: "quotes and line breaks are escaped per RFC 4180",
			rejected: []RowError{
				{Row: 1, Line: 2, Column: "note", Code: CodeParse, Message: "said \"no\"\nnow"},
			},
			want: "line,row,column,code,message\r\n" +
				"2,1,note,PARSE,\"said \"\"no\"\"\nnow\"\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := writeReport(t, tt.rejected)
			if got != tt.want {
				t.Fatalf("report mismatch\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestWriteErrorReportNeutralizesFormulas(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "equals sign", in: "=1+1", want: "'=1+1"},
		{name: "HYPERLINK formula", in: `=HYPERLINK("http://x","go")`, want: `'=HYPERLINK("http://x","go")`},
		{name: "plus sign", in: "+49 170", want: "'+49 170"},
		{name: "minus sign", in: "-2", want: "'-2"},
		{name: "at sign", in: "@SUM(A1)", want: "'@SUM(A1)"},
		{name: "leading tab", in: "\tcmd", want: "'\tcmd"},
		{name: "leading carriage return", in: "\rcmd", want: "'\rcmd"},
		{name: "sign inside text is untouched", in: "a=b", want: "a=b"},
		{name: "plain text is untouched", in: "value is required", want: "value is required"},
		{name: "empty cell is untouched", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := writeReport(t, []RowError{
				{Row: 1, Line: 2, Column: tt.in, Code: CodeParse, Message: tt.in},
			})
			records, err := parseCSV(report, reportParseOptions())
			if err != nil {
				t.Fatalf("parse report: %v", err)
			}
			if len(records) != 2 {
				t.Fatalf("got %d records, want header and one row", len(records))
			}
			row := records[1].values
			if row[2] != tt.want {
				t.Errorf("column cell: got %q, want %q", row[2], tt.want)
			}
			if row[4] != tt.want {
				t.Errorf("message cell: got %q, want %q", row[4], tt.want)
			}
		})
	}
}

func TestWriteErrorReportRoundTrip(t *testing.T) {
	input := "email,name,age,plan\n" +
		"ann@example.com,Ann,34,pro\n" +
		"bob@example.com,,41,free\n" +
		"cy@example.com,Cy,forty,pro\n" +
		"di@example.com,Di,29\n" +
		"ed@example.com,\"Ed, Jr.\",52,free\n" +
		"fi@example.com,\"Fi\nLine\",1,\"pro\"x\n"

	res, err := Import(strings.NewReader(input), customerSchema())
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(res.Rejected) == 0 {
		t.Fatal("test input must produce rejected rows")
	}

	report := writeReport(t, res.Rejected)
	records, err := parseCSV(report, reportParseOptions())
	if err != nil {
		t.Fatalf("parse report: %v", err)
	}
	if len(records) != len(res.Rejected)+1 {
		t.Fatalf("report has %d records, want %d", len(records), len(res.Rejected)+1)
	}
	if strings.Join(records[0].values, ",") != "line,row,column,code,message" {
		t.Fatalf("unexpected header %q", records[0].values)
	}
	for i, want := range res.Rejected {
		got := records[i+1].values
		if len(got) != 5 {
			t.Fatalf("row %d: got %d cells, want 5", i, len(got))
		}
		if got[0] != strconv.Itoa(want.Line) || got[1] != strconv.Itoa(want.Row) {
			t.Errorf("row %d: line/row %s/%s, want %d/%d", i, got[0], got[1], want.Line, want.Row)
		}
		if got[2] != want.Column || got[3] != string(want.Code) || got[4] != want.Message {
			t.Errorf("row %d: got %q, want %+v", i, got, want)
		}
	}
}
