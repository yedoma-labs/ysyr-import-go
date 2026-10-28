package ysyrimport

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type customer struct {
	Email  string
	Name   string
	Age    int
	Plan   string
	Joined time.Time
}

func customerSchema() Schema[customer] {
	return Schema[customer]{
		StringColumn("email", func(c *customer, v string) { c.Email = v }, Required()),
		StringColumn("name", func(c *customer, v string) { c.Name = v }, Required()),
		IntColumn("age", func(c *customer, v int) { c.Age = v }),
		EnumColumn("plan", []string{"free", "pro"}, func(c *customer, v string) { c.Plan = v }, Required()),
	}
}

// sameList compares two slices and treats nil and empty as equal.
func sameList[T any](a, b []T) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

type importCase struct {
	name     string
	input    string
	schema   Schema[customer]
	opts     []Option
	accepted []customer
	rejected []RowError
	summary  Summary
	wantErr  error
}

func TestImport(t *testing.T) {
	tests := []importCase{
		{
			name: "mixed file keeps valid rows and reports broken ones",
			input: "email,name,age,plan\n" +
				"ann@example.com,Ann,34,pro\n" +
				"bob@example.com,,41,free\n" +
				"cy@example.com,Cy,forty,pro\n" +
				"di@example.com,Di,29\n" +
				"ed@example.com,\"Ed, Jr.\",52,free\n",
			accepted: []customer{
				{Email: "ann@example.com", Name: "Ann", Age: 34, Plan: "pro"},
				{Email: "ed@example.com", Name: "Ed, Jr.", Age: 52, Plan: "free"},
			},
			rejected: []RowError{
				{Row: 2, Line: 3, Column: "name", Code: CodeRequired, Message: "value is required"},
				{Row: 3, Line: 4, Column: "age", Code: CodeParse, Message: "expected an integer"},
				{Row: 4, Line: 5, Code: CodeWrongFieldCount, Message: "expected 4 fields, found 3"},
			},
			summary: Summary{Total: 5, Accepted: 2, Rejected: 3},
		},
		{
			name:  "multiple problems in one row are reported separately",
			input: "email,name,age,plan\nbob@example.com,,x,bogus\n",
			rejected: []RowError{
				{Row: 1, Line: 2, Column: "name", Code: CodeRequired, Message: "value is required"},
				{Row: 1, Line: 2, Column: "age", Code: CodeParse, Message: "expected an integer"},
				{Row: 1, Line: 2, Column: "plan", Code: CodeParse, Message: "expected one of: free, pro"},
			},
			summary: Summary{Total: 1, Accepted: 0, Rejected: 1},
		},
		{
			name:  "helpers trim surrounding whitespace",
			input: "email,name,age,plan\na@x.de, Ann ,  1 ,free\n",
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann", Age: 1, Plan: "free"},
			},
			summary: Summary{Total: 1, Accepted: 1},
		},
		{
			name:  "whitespace-only required cell is REQUIRED",
			input: "email,name,age,plan\na@x.de,   ,1,free\n",
			rejected: []RowError{
				{Row: 1, Line: 2, Column: "name", Code: CodeRequired, Message: "value is required"},
			},
			summary: Summary{Total: 1, Rejected: 1},
		},
		{
			name:  "empty optional cell keeps the zero value",
			input: "email,name,age,plan\na@x.de,Ann,,free\n",
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann", Age: 0, Plan: "free"},
			},
			summary: Summary{Total: 1, Accepted: 1},
		},
		{
			name:  "quoted field with escaped quotes",
			input: "email,name,age,plan\na@x.de,\"Ann \"\"A\"\" Lee\",1,free\n",
			accepted: []customer{
				{Email: "a@x.de", Name: `Ann "A" Lee`, Age: 1, Plan: "free"},
			},
			summary: Summary{Total: 1, Accepted: 1},
		},
		{
			name: "embedded newline keeps physical line numbers",
			input: "email,name,age,plan\n" +
				"a@x.de,\"Multi\nline\",1,free\n" +
				"b@x.de,,2,pro\n" +
				"c@x.de,Cee,3,pro\n",
			accepted: []customer{
				{Email: "a@x.de", Name: "Multi\nline", Age: 1, Plan: "free"},
				{Email: "c@x.de", Name: "Cee", Age: 3, Plan: "pro"},
			},
			rejected: []RowError{
				{Row: 2, Line: 4, Column: "name", Code: CodeRequired, Message: "value is required"},
			},
			summary: Summary{Total: 3, Accepted: 2, Rejected: 1},
		},
		{
			name:  "unterminated quote swallows the rest of the input",
			input: "email,name,age,plan\na@x.de,\"Ann,1,free\nb@x.de,Bob,2,pro\n",
			rejected: []RowError{
				{Row: 1, Line: 2, Code: CodeParseQuote, Message: "unterminated quoted field"},
			},
			summary: Summary{Total: 1, Rejected: 1},
		},
		{
			name:  "characters after a closing quote are PARSE_QUOTE and the next row is kept",
			input: "email,name,age,plan\na@x.de,\"Ann\"x,1,free\nb@x.de,Bob,2,pro\n",
			accepted: []customer{
				{Email: "b@x.de", Name: "Bob", Age: 2, Plan: "pro"},
			},
			rejected: []RowError{
				{Row: 1, Line: 2, Code: CodeParseQuote, Message: "unexpected character after closing quote"},
			},
			summary: Summary{Total: 2, Accepted: 1, Rejected: 1},
		},
		{
			name:  "too few and too many fields",
			input: "email,name,age,plan\na@x.de,Ann,1\na@x.de,Ann,1,free,extra\n",
			rejected: []RowError{
				{Row: 1, Line: 2, Code: CodeWrongFieldCount, Message: "expected 4 fields, found 3"},
				{Row: 2, Line: 3, Code: CodeWrongFieldCount, Message: "expected 4 fields, found 5"},
			},
			summary: Summary{Total: 2, Rejected: 2},
		},
		{
			name: "blank lines are skipped and not counted as rows",
			input: "email,name,age,plan\n" +
				"\n" +
				"a@x.de,Ann,1,free\n" +
				"\n" +
				"\n" +
				"b@x.de,,2,pro\n",
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann", Age: 1, Plan: "free"},
			},
			rejected: []RowError{
				{Row: 2, Line: 6, Column: "name", Code: CodeRequired, Message: "value is required"},
			},
			summary: Summary{Total: 2, Accepted: 1, Rejected: 1},
		},
		{
			name:  "CRLF line endings",
			input: "email,name,age,plan\r\na@x.de,Ann,1,free\r\nb@x.de,Bob,2,pro\r\n",
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann", Age: 1, Plan: "free"},
				{Email: "b@x.de", Name: "Bob", Age: 2, Plan: "pro"},
			},
			summary: Summary{Total: 2, Accepted: 2},
		},
		{
			name:  "byte order mark is removed",
			input: "\uFEFFemail,name,age,plan\na@x.de,Ann,1,free",
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann", Age: 1, Plan: "free"},
			},
			summary: Summary{Total: 1, Accepted: 1},
		},
		{
			name:  "header names are trimmed",
			input: " email , name,age ,plan\na@x.de,Ann,1,free\n",
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann", Age: 1, Plan: "free"},
			},
			summary: Summary{Total: 1, Accepted: 1},
		},
		{
			name:    "empty input misses the required columns",
			input:   "",
			wantErr: ErrMissingColumn,
		},
		{
			name:    "header only with required columns present",
			input:   "email,name,age,plan\n",
			summary: Summary{},
		},
		{
			name:    "missing required column is a whole-file error",
			input:   "email,age,plan\na@x.de,1,free\n",
			wantErr: ErrMissingColumn,
		},
		{
			name:  "optional column may be absent from the header",
			input: "email,name,plan\na@x.de,Ann,free\n",
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann", Plan: "free"},
			},
			summary: Summary{Total: 1, Accepted: 1},
		},
		{
			name:    "broken header quoting is a whole-file error",
			input:   "\"email,name,age,plan\na@x.de,Ann,1,free\n",
			wantErr: ErrInvalidHeader,
		},
		{
			name:  "unknown columns are ignored by default",
			input: "email,name,age,plan,note\na@x.de,Ann,1,free,hello\n",
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann", Age: 1, Plan: "free"},
			},
			summary: Summary{Total: 1, Accepted: 1},
		},
		{
			name:  "unknown column with a value is rejected with Reject",
			input: "email,name,age,plan,note\na@x.de,Ann,1,free,hello\nb@x.de,Bob,2,pro,\n",
			opts:  []Option{WithUnknownColumns(Reject)},
			accepted: []customer{
				{Email: "b@x.de", Name: "Bob", Age: 2, Plan: "pro"},
			},
			rejected: []RowError{
				{Row: 1, Line: 2, Column: "note", Code: CodeUnknownColumn, Message: "column is not part of the schema"},
			},
			summary: Summary{Total: 2, Accepted: 1, Rejected: 1},
		},
		{
			name:  "duplicate header name: first occurrence wins with Ignore",
			input: "email,name,age,plan,name\na@x.de,Ann,1,free,Zed\n",
			opts:  []Option{WithUnknownColumns(Ignore)},
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann", Age: 1, Plan: "free"},
			},
			summary: Summary{Total: 1, Accepted: 1},
		},
		{
			name:  "duplicate header name is an unknown column with Reject",
			input: "email,name,age,plan,name\na@x.de,Ann,1,free,Zed\n",
			opts:  []Option{WithUnknownColumns(Reject)},
			rejected: []RowError{
				{Row: 1, Line: 2, Column: "name", Code: CodeUnknownColumn, Message: "duplicate column name"},
			},
			summary: Summary{Total: 1, Rejected: 1},
		},
		{
			name:  "validator rejects rows that passed the column rules",
			input: "email,name,age,plan\na@x.de,Kid,10,free\nb@x.de,Ann,20,pro\nc@x.de,,10,free\n",
			opts: []Option{WithValidate(func(c customer) error {
				if c.Age < 18 {
					return errors.New("must be an adult")
				}
				return nil
			})},
			accepted: []customer{
				{Email: "b@x.de", Name: "Ann", Age: 20, Plan: "pro"},
			},
			rejected: []RowError{
				{Row: 1, Line: 2, Code: CodeValidation, Message: "must be an adult"},
				{Row: 3, Line: 4, Column: "name", Code: CodeRequired, Message: "value is required"},
			},
			summary: Summary{Total: 3, Accepted: 1, Rejected: 2},
		},
		{
			name:  "semicolon delimiter with a quoted delimiter",
			input: "email;name;age;plan\na@x.de;\"Ann;Jr\";1;free\n",
			opts:  []Option{WithDelimiter(';')},
			accepted: []customer{
				{Email: "a@x.de", Name: "Ann;Jr", Age: 1, Plan: "free"},
			},
			summary: Summary{Total: 1, Accepted: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.schema
			if schema == nil {
				schema = customerSchema()
			}
			got, err := Import(strings.NewReader(tt.input), schema, tt.opts...)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want error matching %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !sameList(got.Accepted, tt.accepted) {
				t.Errorf("accepted mismatch\n got: %#v\nwant: %#v", got.Accepted, tt.accepted)
			}
			if !sameList(got.Rejected, tt.rejected) {
				t.Errorf("rejected mismatch\n got: %#v\nwant: %#v", got.Rejected, tt.rejected)
			}
			if got.Summary != tt.summary {
				t.Errorf("summary: got %+v, want %+v", got.Summary, tt.summary)
			}
		})
	}
}

func TestImportMissingColumnListsNames(t *testing.T) {
	_, err := Import(strings.NewReader("name\nAnn\n"), customerSchema())
	if !errors.Is(err, ErrMissingColumn) {
		t.Fatalf("want ErrMissingColumn, got %v", err)
	}
	if !strings.Contains(err.Error(), "email") || !strings.Contains(err.Error(), "plan") {
		t.Fatalf("error should name the missing columns, got %q", err.Error())
	}
}

func TestImportWithValidateTypeMismatch(t *testing.T) {
	_, err := Import(strings.NewReader("email,name,age,plan\n"), customerSchema(),
		WithValidate(func(s string) error { return nil }))
	if err == nil {
		t.Fatal("want error for a validator of another type")
	}
}

func TestImportRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
	}{
		{name: "double quote as delimiter", opt: WithDelimiter('"')},
		{name: "line break as delimiter", opt: WithDelimiter('\n')},
		{name: "replacement rune as delimiter", opt: WithDelimiter('�')},
		{name: "negative maxBytes", opt: WithMaxBytes(-1)},
		{name: "negative maxRows", opt: WithMaxRows(-1)},
		{name: "negative maxFieldLength", opt: WithMaxFieldLength(-1)},
		{name: "unknown policy", opt: WithUnknownColumns(UnknownColumnPolicy(9))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Import(strings.NewReader("email,name,age,plan\n"), customerSchema(), tt.opt)
			if err == nil {
				t.Fatal("want error for invalid option")
			}
		})
	}
}

func TestImportCustomParseErrorIsSanitizedAndTruncated(t *testing.T) {
	schema := Schema[customer]{
		Column("email", func(raw string) (string, error) {
			return "", errors.New("bad\x07value" + strings.Repeat("x", 200))
		}, func(c *customer, v string) { c.Email = v }, Required()),
	}
	got, err := Import(strings.NewReader("email\nabc\n"), schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Rejected) != 1 {
		t.Fatalf("want 1 rejected row, got %d", len(got.Rejected))
	}
	want := "bad value" + strings.Repeat("x", 90) + "…"
	if got.Rejected[0].Message != want {
		t.Fatalf("message\n got: %q\nwant: %q", got.Rejected[0].Message, want)
	}
	if got.Rejected[0].Code != CodeParse || got.Rejected[0].Column != "email" {
		t.Fatalf("unexpected row error %+v", got.Rejected[0])
	}
}
