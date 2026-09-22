package errors

import (
	"fmt"
	"runtime"
	"strings"
)

// ErrorCode represents different types of errors
type ErrorCode int

const (
	// General errors
	ErrOK ErrorCode = iota
	ErrParam
	ErrParse
	ErrFail

	// GitOps specific errors
	ErrTerraform
	ErrMissingTool
	ErrDesiredStateMissing
	ErrDesiredStateMalformed
	ErrConfigurationMissing
	ErrConfigurationMalformed

	// Generic undefined error
	ErrUndefined = 255
)

// String returns the string representation of an error code
func (e ErrorCode) String() string {
	switch e {
	case ErrOK:
		return "OK"
	case ErrParam:
		return "PARAM_ERROR"
	case ErrParse:
		return "PARSE_ERROR"
	case ErrFail:
		return "FAIL"
	case ErrTerraform:
		return "TERRAFORM_ERROR"
	case ErrMissingTool:
		return "MISSING_TOOL"
	case ErrDesiredStateMissing:
		return "DESIREDSTATE_MISSING"
	case ErrDesiredStateMalformed:
		return "DESIREDSTATE_MALFORMED"
	case ErrConfigurationMissing:
		return "CONFIGURATION_MISSING"
	case ErrConfigurationMalformed:
		return "CONFIGURATION_MALFORMED"
	case ErrUndefined:
		return "UNDEFINED"
	default:
		return "UNKNOWN"
	}
}

// ExitCode returns the exit code for an error
func (e ErrorCode) ExitCode() int {
	return int(e)
}

// GitOpsError represents a custom error with additional context
type GitOpsError struct {
	Code     ErrorCode
	Message  string
	Cause    error
	File     string
	Line     int
	Function string
}

// Error implements the error interface
func (e *GitOpsError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code.String(), e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code.String(), e.Message)
}

// Unwrap implements the unwrap interface for error chaining
func (e *GitOpsError) Unwrap() error {
	return e.Cause
}

// WithCaller adds caller information to the error
func (e *GitOpsError) WithCaller() *GitOpsError {
	pc, file, line, ok := runtime.Caller(1)
	if ok {
		e.File = file
		e.Line = line

		if fn := runtime.FuncForPC(pc); fn != nil {
			e.Function = fn.Name()
		}
	}
	return e
}

// String returns a detailed string representation
func (e *GitOpsError) String() string {
	var parts []string

	if e.File != "" && e.Line > 0 {
		// Extract just the filename from the full path
		fileParts := strings.Split(e.File, "/")
		filename := fileParts[len(fileParts)-1]
		parts = append(parts, fmt.Sprintf("[%s:%d]", filename, e.Line))
	}

	if e.Function != "" {
		// Extract just the function name from the full path
		funcParts := strings.Split(e.Function, ".")
		funcName := funcParts[len(funcParts)-1]
		parts = append(parts, fmt.Sprintf("in %s()", funcName))
	}

	parts = append(parts, e.Error())

	return strings.Join(parts, " ")
}

// New creates a new GitOpsError
func New(code ErrorCode, message string) *GitOpsError {
	err := &GitOpsError{
		Code:    code,
		Message: message,
	}
	return err.WithCaller()
}

// Newf creates a new GitOpsError with formatted message
func Newf(code ErrorCode, format string, args ...interface{}) *GitOpsError {
	err := &GitOpsError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
	return err.WithCaller()
}

// Wrap wraps an existing error with additional context
func Wrap(code ErrorCode, message string, cause error) *GitOpsError {
	err := &GitOpsError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
	return err.WithCaller()
}

// Wrapf wraps an existing error with formatted message
func Wrapf(code ErrorCode, cause error, format string, args ...interface{}) *GitOpsError {
	err := &GitOpsError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Cause:   cause,
	}
	return err.WithCaller()
}

// IsCode checks if an error is of a specific code
func IsCode(err error, code ErrorCode) bool {
	if gitopsErr, ok := err.(*GitOpsError); ok {
		return gitopsErr.Code == code
	}
	return false
}

// GetCode extracts the error code from an error, returns ErrUndefined if not a GitOpsError
func GetCode(err error) ErrorCode {
	if gitopsErr, ok := err.(*GitOpsError); ok {
		return gitopsErr.Code
	}
	return ErrUndefined
}

// GetExitCode returns the appropriate exit code for an error
func GetExitCode(err error) int {
	if err == nil {
		return int(ErrOK)
	}

	if gitopsErr, ok := err.(*GitOpsError); ok {
		return gitopsErr.Code.ExitCode()
	}

	return int(ErrUndefined)
}

// Common error constructors for convenience

// NewParamError creates a parameter validation error
func NewParamError(message string) *GitOpsError {
	return New(ErrParam, message)
}

// NewParseError creates a parsing error
func NewParseError(message string) *GitOpsError {
	return New(ErrParse, message)
}

// NewTerraformError creates a Terraform-related error
func NewTerraformError(message string) *GitOpsError {
	return New(ErrTerraform, message)
}

// NewMissingToolError creates a missing tool error
func NewMissingToolError(tool string) *GitOpsError {
	return Newf(ErrMissingTool, "required tool not found: %s", tool)
}

// NewDesiredStateMissingError creates a missing desired state error
func NewDesiredStateMissingError(path string) *GitOpsError {
	return Newf(ErrDesiredStateMissing, "desired state file not found: %s", path)
}

// NewDesiredStateMalformedError creates a malformed desired state error
func NewDesiredStateMalformedError(message string) *GitOpsError {
	return Newf(ErrDesiredStateMalformed, "malformed desired state: %s", message)
}

// NewConfigurationMissingError creates a missing configuration error
func NewConfigurationMissingError(path string) *GitOpsError {
	return Newf(ErrConfigurationMissing, "configuration file not found: %s", path)
}

// NewConfigurationMalformedError creates a malformed configuration error
func NewConfigurationMalformedError(message string) *GitOpsError {
	return Newf(ErrConfigurationMalformed, "malformed configuration: %s", message)
}
