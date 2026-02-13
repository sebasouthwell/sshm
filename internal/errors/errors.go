package errors

import (
	"fmt"
)

// Error codes
const (
	EUsage     = 2  // Bad arguments, invalid syntax
	ENotFound  = 10 // Alias not found
	EDeps      = 11 // Missing dependency
	EResolve   = 12 // Selector resolution failed/ambiguous
	EValidate  = 13 // Key missing, invalid entry
	EExec      = 14 // Underlying command failed
	ETTY       = 15 // TTY required but not available
)

// SSHMError represents an SSHM-specific error with exit code
type SSHMError struct {
	Code    int
	Message string
	Err     error
}

func (e *SSHMError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *SSHMError) Unwrap() error {
	return e.Err
}

// NewUsageError creates a usage error
func NewUsageError(format string, args ...interface{}) *SSHMError {
	return &SSHMError{
		Code:    EUsage,
		Message: fmt.Sprintf(format, args...),
	}
}

// NewNotFoundError creates a not found error
func NewNotFoundError(alias string) *SSHMError {
	return &SSHMError{
		Code:    ENotFound,
		Message: fmt.Sprintf("alias not found: %s", alias),
	}
}

// NewDependencyError creates a dependency error
func NewDependencyError(dep string, installHint string) *SSHMError {
	msg := fmt.Sprintf("missing dependency: %s", dep)
	if installHint != "" {
		msg += fmt.Sprintf(" (install via: %s)", installHint)
	}
	return &SSHMError{
		Code:    EDeps,
		Message: msg,
	}
}

// NewResolveError creates a resolution error
func NewResolveError(format string, args ...interface{}) *SSHMError {
	return &SSHMError{
		Code:    EResolve,
		Message: fmt.Sprintf(format, args...),
	}
}

// NewValidationError creates a validation error
func NewValidationError(format string, args ...interface{}) *SSHMError {
	return &SSHMError{
		Code:    EValidate,
		Message: fmt.Sprintf(format, args...),
	}
}

// NewExecError creates an execution error
func NewExecError(err error) *SSHMError {
	return &SSHMError{
		Code:    EExec,
		Message: "command execution failed",
		Err:     err,
	}
}

// NewTTYError creates a TTY error
func NewTTYError() *SSHMError {
	return &SSHMError{
		Code:    ETTY,
		Message: "interactive session requires a TTY",
	}
}
