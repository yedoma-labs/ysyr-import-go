package ysyrimport

import (
	"errors"
	"fmt"
)

// Whole-file errors. Import returns them, possibly wrapped, and aborts. Test them with errors.Is.
// Row-level problems never abort; they are listed in Result.Rejected.
var (
	// ErrMissingColumn means a required column is not part of the header.
	ErrMissingColumn = errors.New("ysyrimport: missing required column")

	// ErrInvalidHeader means the header line cannot be parsed, for example because its quoting is
	// broken, so the column names cannot be determined.
	ErrInvalidHeader = errors.New("ysyrimport: invalid header")

	// ErrLimitExceeded means the input, the number of rows or a field is larger than the configured
	// limit. Input is never truncated silently.
	ErrLimitExceeded = errors.New("ysyrimport: limit exceeded")
)

// limitError wraps ErrLimitExceeded and names the limit that was hit.
func limitError(name string, limit int64) error {
	return fmt.Errorf("%w: %s is %d", ErrLimitExceeded, name, limit)
}
