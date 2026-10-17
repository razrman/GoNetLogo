package nl

import "fmt"

// rtError is a runtime error recovered at the top of a procedure call.
type rtError struct {
	msg string
	pos Pos
}

func (e *rtError) Error() string {
	if e.pos.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.pos.Line, e.msg)
	}
	return e.msg
}

func errorAt(pos Pos, format string, args ...any) error {
	return &rtError{msg: fmt.Sprintf(format, args...), pos: pos}
}
