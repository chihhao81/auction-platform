// Package apperr defines transport-independent application errors. Services
// return these; the HTTP layer maps Kind to a status code.
package apperr

import (
	"errors"
	"fmt"
)

type Kind int

const (
	KindInvalid Kind = iota + 1
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindTooManyRequests
)

// Error is an expected, user-facing failure. Code is a stable machine-readable
// identifier the frontend can switch on; Message is safe to show to users.
type Error struct {
	Kind    Kind
	Code    string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func Invalid(code, msg string) *Error      { return &Error{KindInvalid, code, msg} }
func Unauthorized(code, msg string) *Error { return &Error{KindUnauthorized, code, msg} }
func Forbidden(code, msg string) *Error    { return &Error{KindForbidden, code, msg} }
func NotFound(code, msg string) *Error     { return &Error{KindNotFound, code, msg} }
func Conflict(code, msg string) *Error     { return &Error{KindConflict, code, msg} }
func TooMany(code, msg string) *Error      { return &Error{KindTooManyRequests, code, msg} }

// As extracts an *Error from err, if any.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// IsCode reports whether err is an *Error with the given code.
func IsCode(err error, code string) bool {
	e, ok := As(err)
	return ok && e.Code == code
}
