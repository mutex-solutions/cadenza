package kernel

import (
	"errors"
	"fmt"
)

// Kind says what went wrong, independent of transport. The HTTP layer maps
// kinds to status codes and the CLI maps them to exit codes.
type Kind string

const (
	KindInvalid      Kind = "invalid"      // the request is wrong; Fix says how to correct it
	KindNotFound     Kind = "not_found"    // the thing does not exist or is not visible to the caller
	KindConflict     Kind = "conflict"     // the thing is in a state that does not allow this
	KindUnauthorized Kind = "unauthorized" // no or bad credentials
	KindForbidden    Kind = "forbidden"    // credentials are fine but may not do this
	KindUnavailable  Kind = "unavailable"  // a dependency (storage, push) is down
	KindInternal     Kind = "internal"     // a bug
)

// Error is the one error type that crosses package boundaries. Msg is safe to
// show to agents and users. Fix, when set, is the exact correction.
type Error struct {
	Kind Kind
	Msg  string
	Fix  string
	Err  error // wrapped cause, never shown to callers
}

func (e *Error) Error() string {
	s := e.Msg
	if e.Fix != "" {
		s += " (fix: " + e.Fix + ")"
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// WithFix returns a copy of e with the fix text set.
func (e *Error) WithFix(format string, args ...any) *Error {
	c := *e
	c.Fix = fmt.Sprintf(format, args...)
	return &c
}

func newErr(k Kind, format string, args ...any) *Error {
	return &Error{Kind: k, Msg: fmt.Sprintf(format, args...)}
}

func Invalid(format string, args ...any) *Error  { return newErr(KindInvalid, format, args...) }
func NotFound(format string, args ...any) *Error { return newErr(KindNotFound, format, args...) }
func Conflict(format string, args ...any) *Error { return newErr(KindConflict, format, args...) }
func Unauthorized(format string, args ...any) *Error {
	return newErr(KindUnauthorized, format, args...)
}
func Forbidden(format string, args ...any) *Error   { return newErr(KindForbidden, format, args...) }
func Unavailable(format string, args ...any) *Error { return newErr(KindUnavailable, format, args...) }

// Internal wraps an unexpected error. The message shown to callers is generic.
func Internal(err error, format string, args ...any) *Error {
	e := newErr(KindInternal, format, args...)
	e.Err = err
	return e
}

// KindOf returns the kind of err, or KindInternal for errors that are not *Error.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindInternal
}

// Is reports whether err is a kernel error of kind k.
func Is(err error, k Kind) bool { return err != nil && KindOf(err) == k }
