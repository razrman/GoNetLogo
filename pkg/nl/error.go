package nl

import (
	"errors"
	"fmt"
)

// rtError is a runtime error recovered at the top of a procedure call.
type rtError struct {
	msg  string
	pos  Pos
	proc string
}

func (e *rtError) Error() string {
	if e.pos.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.pos.Line, e.msg)
	}
	return e.msg
}

// LocateError reports the procedure and source line of a NetLogo runtime
// error. ok is false when err did not come from the interpreter.
func LocateError(err error) (proc string, line int, ok bool) {
	var re *rtError
	if !errors.As(err, &re) || re == nil {
		return "", 0, false
	}
	return re.proc, re.pos.Line, true
}

func errorAt(pos Pos, format string, args ...any) error {
	return &rtError{msg: fmt.Sprintf(format, args...), pos: pos}
}
