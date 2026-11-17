// Package domain holds myco's core model: agents, messages, their invariants
// and the repository ports the rest of the system depends on. It has no
// dependency on transport, storage or any third-party package.
package domain

import (
	"errors"
	"fmt"
)

// ErrorCode is a stable, machine-readable error category. Codes are part of
// the public API contract (HTTP responses and CLI JSON errors).
type ErrorCode string

const (
	CodeInvalidArgument  ErrorCode = "invalid_argument"
	CodeNotFound         ErrorCode = "not_found"
	CodeConflict         ErrorCode = "conflict"
	CodeInvalidState     ErrorCode = "invalid_state"
	CodePermissionDenied ErrorCode = "permission_denied"
	CodeUnauthenticated  ErrorCode = "unauthenticated"
	CodeInternal         ErrorCode = "internal"
)

// Error is a domain error carrying a code and an optional recovery hint.
type Error struct {
	Code       ErrorCode
	Message    string
	Suggestion string
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// WithSuggestion returns a copy of e with a recovery suggestion attached.
func (e *Error) WithSuggestion(format string, args ...any) *Error {
	c := *e
	c.Suggestion = fmt.Sprintf(format, args...)
	return &c
}

// Errf builds a domain error.
func Errf(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// CodeOf extracts the ErrorCode of err, defaulting to CodeInternal.
func CodeOf(err error) ErrorCode {
	var de *Error
	if errors.As(err, &de) {
		return de.Code
	}
	return CodeInternal
}

// IsCode reports whether err is a domain error with the given code.
func IsCode(err error, code ErrorCode) bool {
	var de *Error
	return errors.As(err, &de) && de.Code == code
}
