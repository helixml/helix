// package agent - errors.go
// Defines session-specific errors and demonstrates customized retryable
// and ignorable errors using custom types.

package agent

import (
	"errors"
	"fmt"
)

var (
	ErrSessionClosed = errors.New("session has been closed")
	ErrNoMessage     = errors.New("no message available")
)

// RetryableError is the custom type for errors that can be retried.
type RetryableError struct {
	msg string
}

// Error returns the error message for RetryableError.
func (e *RetryableError) Error() string {
	return e.msg
}

// NewRetryableError creates a new instance of RetryableError.
func NewRetryableError(format string, a ...interface{}) error {
	return &RetryableError{
		msg: fmt.Sprintf(format, a...),
	}
}

// IgnorableError is the custom type for errors that can be ignored.
type IgnorableError struct {
	msg string
}

// Error returns the error message for IgnorableError.
func (e *IgnorableError) Error() string {
	return e.msg
}

// NewIgnorableError creates a new instance of IgnorableError.
func NewIgnorableError(format string, a ...interface{}) error {
	return &IgnorableError{
		msg: fmt.Sprintf(format, a...),
	}
}
