package ysyrimport_test

import (
	"fmt"
	"strings"

	ysyrimport "github.com/yedoma-labs/ysyr-import-go"
)

type Customer struct {
	Email string
	Name  string
	Age   int
	Plan  string
}

func ExampleImport() {
	schema := ysyrimport.Schema[Customer]{
		ysyrimport.StringColumn("email", func(c *Customer, v string) { c.Email = v }, ysyrimport.Required()),
		ysyrimport.StringColumn("name", func(c *Customer, v string) { c.Name = v }, ysyrimport.Required()),
		ysyrimport.IntColumn("age", func(c *Customer, v int) { c.Age = v }),
		ysyrimport.EnumColumn("plan", []string{"free", "pro"}, func(c *Customer, v string) { c.Plan = v }, ysyrimport.Required()),
	}
	input := `email,name,age,plan
ann@example.com,Ann,34,pro
bob@example.com,,41,free
cy@example.com,Cy,forty,pro
di@example.com,Di,29
ed@example.com,"Ed, Jr.",52,free
`
	res, err := ysyrimport.Import(strings.NewReader(input), schema)
	if err != nil {
		fmt.Println("import failed:", err)
		return
	}
	fmt.Printf("%d accepted, %d rejected\n", len(res.Accepted), res.Summary.Rejected)
	for _, e := range res.Rejected {
		fmt.Printf("line %d (row %d) column %q: %s: %s\n", e.Line, e.Row, e.Column, e.Code, e.Message)
	}
	// Output:
	// 2 accepted, 3 rejected
	// line 3 (row 2) column "name": REQUIRED: value is required
	// line 4 (row 3) column "age": PARSE: expected an integer
	// line 5 (row 4) column "": WRONG_FIELD_COUNT: expected 4 fields, found 3
}
