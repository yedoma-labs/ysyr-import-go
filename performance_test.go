package ysyrimport

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestImportPerformance50kRows imports 50,000 rows, every tenth of which is broken, and checks the
// counts and a generous time limit. The limit is meant to catch quadratic behaviour, not to
// benchmark the machine.
func TestImportPerformance50kRows(t *testing.T) {
	const rows = 50_000
	const maxDuration = 5 * time.Second

	var b strings.Builder
	b.WriteString("email,name,age,plan\n")
	for i := range rows {
		plan := "free"
		if i%2 == 0 {
			plan = "pro"
		}
		age := fmt.Sprint(i%90 + 1)
		if i%10 == 0 {
			age = "x"
		}
		fmt.Fprintf(&b, "user%d@example.com,User %d,%s,%s\n", i, i, age, plan)
	}
	input := b.String()

	start := time.Now()
	res, err := Import(strings.NewReader(input), customerSchema())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if elapsed > maxDuration {
		t.Fatalf("import of %d rows took %v, limit %v", rows, elapsed, maxDuration)
	}

	want := Summary{Total: rows, Accepted: rows - rows/10, Rejected: rows / 10}
	if res.Summary != want {
		t.Fatalf("summary: got %+v, want %+v", res.Summary, want)
	}
	if len(res.Rejected) != rows/10 {
		t.Fatalf("got %d row errors, want %d", len(res.Rejected), rows/10)
	}
}
