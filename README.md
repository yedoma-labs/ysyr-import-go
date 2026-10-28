# ysyr-import-go
<picture>
  <source media="(max-width: 640px)" srcset="https://raw.githubusercontent.com/yedoma-labs/assets/main/resized/banner-resized-mobile.png">
  <img src="https://raw.githubusercontent.com/yedoma-labs/assets/main/resized/banner-resized.png" alt="Project Header">
</picture>

Zero-dependency CSV import with partial success for Go. Valid rows are accepted, broken rows are
collected with their line number, column and reason, and the result can be returned to the user as
a CSV error report.

This is the Go counterpart of [`@yedoma-labs/ysyr-import`](https://github.com/yedoma-labs/ysyr-import).
Semantics, error codes and limits follow the TypeScript package. Differences: field length is
counted in runes instead of UTF-16 code units, there is no delimiter auto-detection, and column
setters are passed explicitly, because Go cannot set struct fields generically.

## The problem

Most CSV importers stop at the first broken line, or fail the whole file. A 10,000-row upload fails
because three rows have a typo in the date column, and the user gets no hint which rows are wrong.

`ysyr-import-go` takes a different approach:

- valid rows are returned in `Result.Accepted`
- broken rows are returned in `Result.Rejected`, each with `Row`, `Line`, `Column`, `Code` and `Message`
- `WriteErrorReport` turns the rejected rows into a CSV file the user can fix and re-upload

## Installation

```sh
go get github.com/yedoma-labs/ysyr-import-go
```

Requires Go 1.23 or newer. The package has no dependencies outside the standard library.

## Quickstart

Given this file, `customers.csv`:

```csv
email,name,age,plan
ann@example.com,Ann,34,pro
bob@example.com,,41,free
cy@example.com,Cy,forty,pro
di@example.com,Di,29
ed@example.com,"Ed, Jr.",52,free
```

```go
package main

import (
	"fmt"
	"os"

	ysyrimport "github.com/yedoma-labs/ysyr-import-go"
)

type Customer struct {
	Email string
	Name  string
	Age   int
	Plan  string
}

func main() {
	schema := ysyrimport.Schema[Customer]{
		ysyrimport.StringColumn("email", func(c *Customer, v string) { c.Email = v }, ysyrimport.Required()),
		ysyrimport.StringColumn("name", func(c *Customer, v string) { c.Name = v }, ysyrimport.Required()),
		ysyrimport.IntColumn("age", func(c *Customer, v int) { c.Age = v }),
		ysyrimport.EnumColumn("plan", []string{"free", "pro"}, func(c *Customer, v string) { c.Plan = v }, ysyrimport.Required()),
	}

	f, err := os.Open("customers.csv")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	res, err := ysyrimport.Import(f, schema)
	if err != nil {
		// A whole-file error, such as a missing required column. Nothing was imported.
		panic(err)
	}

	fmt.Println(res.Accepted)
	// [{ann@example.com Ann 34 pro} {ed@example.com Ed, Jr. 52 free}]

	fmt.Println(res.Summary)
	// {5 2 3}

	report, err := os.Create("errors.csv")
	if err != nil {
		panic(err)
	}
	defer report.Close()
	if err := ysyrimport.WriteErrorReport(report, res.Rejected); err != nil {
		panic(err)
	}
}
```

`res.Rejected` contains one entry per problem:

```go
[]ysyrimport.RowError{
	{Row: 2, Line: 3, Column: "name", Code: "REQUIRED", Message: "value is required"},
	{Row: 3, Line: 4, Column: "age", Code: "PARSE", Message: "expected an integer"},
	{Row: 4, Line: 5, Column: "", Code: "WRONG_FIELD_COUNT", Message: "expected 4 fields, found 3"},
}
```

The report `errors.csv` is a CSV file that you can offer for download or show in the UI:

```csv
line,row,column,code,message
3,2,name,REQUIRED,value is required
4,3,age,PARSE,expected an integer
5,4,,WRONG_FIELD_COUNT,"expected 4 fields, found 3"
```

`Row` counts data rows starting at 1, the header line is not counted. `Line` is the physical line
in the file where the row starts, starting at 1. Use `Line` when the user opens the file in an
editor, and `Row` when you talk about records in your application.

## Storing the result

This library does not write to any storage. What happens to the accepted rows is up to you. Two
common patterns:

- **Partial commit:** insert `res.Accepted` and return `res.Rejected` to the user. Simple, and
  the user can re-upload only the fixed rows.
- **All or nothing:** only insert when `len(res.Rejected) == 0`, otherwise return the report.

For large files, insert the accepted rows in batches inside a transaction, so that a failure in the
database does not leave half of the file stored.

## API

### `Import`

```go
func Import[T any](r io.Reader, columns Schema[T], opts ...Option) (Result[T], error)
```

Options:

| Option | Default | Meaning |
| --- | --- | --- |
| `WithDelimiter(r rune)` | `','` | field delimiter; not a quote or a line break |
| `WithUnknownColumns(p)` | `Ignore` | `Ignore` skips columns outside the schema, `Reject` reports a non-empty value there as `CodeUnknownColumn` |
| `WithMaxBytes(n int64)` | 10 MiB | maximum input size in bytes |
| `WithMaxRows(n int)` | 100000 | maximum number of data rows |
| `WithMaxFieldLength(n int)` | 10000 | maximum length of one field in runes |
| `WithValidate(fn func(T) error)` | none | row check, runs only for rows that passed all column rules |

### Columns

A schema is an ordered list of rules. Every rule names a column and a setter that stores the parsed
value in the row. Go cannot set struct fields generically, so the setter is passed explicitly.

```go
ysyrimport.Column(name string, parse func(raw string) (V, error), set func(*T, V), opts ...ColumnOption)
```

| Helper | Value | Accepts |
| --- | --- | --- |
| `StringColumn` | `string` | any text, trimmed |
| `IntColumn` | `int` | whole numbers, e.g. `-42` |
| `Int64Column` | `int64` | whole numbers in the int64 range |
| `FloatColumn` | `float64` | decimals with a dot, e.g. `1.5`, `-2e3`; no NaN, infinity or hex |
| `BoolColumn` | `bool` | `true`, `false`, `1`, `0` (case-insensitive) |
| `DateColumn` | `time.Time` | `YYYY-MM-DD`, midnight UTC, real calendar dates only |
| `EnumColumn` | any `~string` | exactly one of the given values (case-sensitive) |

Use `Required()` as an option to report an empty cell as `CodeRequired`. Without it, an empty cell
leaves the field at its zero value.

### Error codes

| Code | Meaning | `Column` |
| --- | --- | --- |
| `REQUIRED` | a required cell is empty | yes |
| `PARSE` | a column parser rejected the value | yes |
| `VALIDATION` | the `WithValidate` function returned an error | no |
| `UNKNOWN_COLUMN` | non-empty value in a column outside the schema (only with `Reject`) | yes |
| `WRONG_FIELD_COUNT` | the row has a different number of fields than the header | no |
| `PARSE_QUOTE` | broken quoting: unterminated quote, or characters after a closing quote | no |

Messages are in English, at most 100 runes, and never contain control characters.

### Whole-file errors

`Import` returns these when the whole input cannot be imported. The error matches the sentinel with
`errors.Is`, and the returned `Result` is the zero value:

- `ErrMissingColumn`: a required column is not in the header.
- `ErrInvalidHeader`: the header line has broken quoting, so the columns cannot be determined.
- `ErrLimitExceeded`: input, rows or a field exceed a limit. Input is never truncated silently.

### `WriteErrorReport`

```go
func WriteErrorReport(w io.Writer, rejected []ysyrimport.RowError) error
```

Writes the rejected rows as CSV with the columns `line,row,column,code,message` and CRLF line
endings. Text cells that start with `=`, `+`, `-`, `@`, a tab or a carriage return are prefixed
with `'`, so spreadsheet programs do not evaluate them as formulas.

## Limits

| Limit | Default | Applies to |
| --- | --- | --- |
| `WithMaxBytes` | 10 MiB | the whole input, in UTF-8 bytes |
| `WithMaxRows` | 100,000 | data rows, the header and blank lines excluded |
| `WithMaxFieldLength` | 10,000 | one field, in runes, header fields included |

Exceeding a limit returns `ErrLimitExceeded`. A value equal to the limit is accepted.

## CSV parsing rules

- RFC 4180 quoting: fields may be quoted with `"`, a doubled `""` is a literal quote, and quoted
  fields may contain delimiters and line breaks.
- Line endings: `\n`, `\r\n` and `\r`.
- A leading UTF-8 byte order mark is removed.
- Blank lines are skipped and do not count as rows.
- A trailing newline at the end of the file does not create a row.
- Header names are trimmed. Values are trimmed by the helpers, custom parse functions get the raw cell.
- Quoting errors are row errors, not aborts. An unterminated quote runs to the end of the input, as
  RFC 4180 implies, so it is reported as one `PARSE_QUOTE` error for the record where it starts.

The parser is written in this package. `encoding/csv` stops at the first malformed record, and with
`LazyQuotes` it accepts broken quoting silently. Neither fits partial success.

## Development

```sh
gofmt -l .
go vet ./...
go test -race ./...
go build ./...
```

## License

MIT
