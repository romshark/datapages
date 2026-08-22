package serverscan

import (
	"fmt"
	"go/token"
	"strings"
)

// Error is one problem found in a NewServer call, at the position it is at.
type Error struct {
	// Pos is where the problem is. A position without a line is a file
	// reference, which is what a "declared here" note is.
	Pos token.Position

	// Msg describes the problem.
	Msg string

	// Hint is what to do about it, empty when there is nothing to add.
	Hint string
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Pos.Filename)
	if e.Pos.Line > 0 {
		_, _ = fmt.Fprintf(&b, ":%d:%d", e.Pos.Line, e.Pos.Column)
	}
	b.WriteString(": ")
	b.WriteString(e.Msg)
	if e.Hint != "" {
		b.WriteString("\n  ")
		b.WriteString(e.Hint)
	}
	return b.String()
}

// Errors collects what a scan found wrong.
type Errors struct{ list []*Error }

// Len returns the number of errors collected.
func (e *Errors) Len() int { return len(e.list) }

// addf appends an error at pos.
func (e *Errors) addf(pos token.Position, format string, args ...any) {
	e.list = append(e.list, &Error{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

// hint attaches a hint to the error added last.
func (e *Errors) hint(s string) {
	if len(e.list) > 0 {
		e.list[len(e.list)-1].Hint = s
	}
}

// Err returns the collected errors as one, nil when there are none.
func (e *Errors) Err() error {
	if len(e.list) == 0 {
		return nil
	}
	return e
}

func (e *Errors) Error() string {
	msgs := make([]string, len(e.list))
	for i, x := range e.list {
		msgs[i] = x.Error()
	}
	return strings.Join(msgs, "\n")
}
