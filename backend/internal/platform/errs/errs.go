// Package errs defines the typed application error used across every layer.
// It depends only on the standard library so domain packages may import it.
package errs

import (
	"errors"
	"maps"
)

// Kind classifies an error; transport maps Kind to a status code in one place.
type Kind int

// Error kinds.
const (
	KindInternal Kind = iota
	KindValidation
	KindNotFound
	KindConflict
	KindUnauthorized
	KindForbidden
	KindRateLimited
	KindUnavailable
)

var kindNames = [...]string{"internal", "validation", "not_found", "conflict", "unauthorized", "forbidden", "rate_limited", "unavailable"}

// String returns the snake_case kind name.
func (k Kind) String() string {
	if k < 0 || int(k) >= len(kindNames) {
		return "unknown"
	}
	return kindNames[k]
}

// Error is the result-object error. Message is an i18n key ("errors.<code>").
type Error struct {
	Code    string
	Kind    Kind
	Message string
	Fields  map[string]string
	cause   error
}

// New creates an error whose message key is "errors.<code>".
func New(kind Kind, code string) *Error {
	return &Error{Code: code, Kind: kind, Message: "errors." + code}
}

// Constructors per kind.
func Validation(code string) *Error   { return New(KindValidation, code) }
func NotFound(code string) *Error     { return New(KindNotFound, code) }
func Conflict(code string) *Error     { return New(KindConflict, code) }
func Unauthorized(code string) *Error { return New(KindUnauthorized, code) }
func Forbidden(code string) *Error    { return New(KindForbidden, code) }
func RateLimited(code string) *Error  { return New(KindRateLimited, code) }
func Unavailable(code string) *Error  { return New(KindUnavailable, code) }
func Internal(code string) *Error     { return New(KindInternal, code) }

// Error implements error. Cause text is included for logs only.
func (e *Error) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.cause.Error()
	}
	return e.Code
}

// Unwrap exposes the cause.
func (e *Error) Unwrap() error { return e.cause }

// Is matches any *Error with the same code, so copies of a sentinel compare equal.
func (e *Error) Is(target error) bool {
	var t *Error
	if errors.As(target, &t) {
		return t.Code == e.Code
	}
	return false
}

func (e *Error) clone() *Error {
	c := *e
	c.Fields = maps.Clone(e.Fields)
	return &c
}

// WithField returns a copy carrying an extra field detail.
func (e *Error) WithField(name, detail string) *Error {
	c := e.clone()
	if c.Fields == nil {
		c.Fields = map[string]string{}
	}
	c.Fields[name] = detail
	return c
}

// Wrap returns a copy carrying cause.
func (e *Error) Wrap(cause error) *Error {
	c := e.clone()
	c.cause = cause
	return c
}

// As extracts an *Error from err's chain.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// From converts any error to *Error; unknown errors become internal.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := As(err); ok {
		return e
	}
	return Internal("internal").Wrap(err)
}
