// Package fail holds the error type Standin uses when it refuses to go on.
//
// A refusal has a short code (safe to put in an issue report, it never holds
// data) and a plain message for the person. Messages must never contain a real
// value from a data file.
package fail

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Error is a refusal: Standin stops, writes nothing, and exits 2.
type Error struct {
	Code string // short, data-free, for `standin report`
	Msg  string // plain words for the person
}

func (e *Error) Error() string { return e.Msg }

// New makes a refusal with a code and a message.
func New(code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Code returns the code of err if it is a refusal, or "error" if not.
func Code(err error) string {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return "error"
}

// Reason gives the cause of an operating-system error without the file path
// in it (a path may itself be private, like a file named after a person).
func Reason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err.Error()
	}
	if err == nil {
		return ""
	}
	return "an error from the system"
}
