package errors

import (
	"fmt"
	"strings"
	"testing"
)

// Behavioral BDD tests for error codes, construction, wrapping, and context.

// ============================================================================
// BEHAVIORAL CONTRACT METADATA STRUCTURES
// ============================================================================

// BehavioralContract documents error behavior and the current implementation.
type BehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

// ============================================================================
// BEHAVIORAL CONTRACT: STATUS CODE CONSTANTS
// ============================================================================

// TestErrors_StatusCodeConstants_BehavioralBDD verifies:
// - Shared status codes have consistent meaning across implementations
func TestErrors_StatusCodeConstants_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Define consistent status code constants for error categorization",
		CurrentImpl:     "Current Go error implementation",
		ExpectedOutcome: "Error behavior remains stable and is validated by this test",
		Rationale:       "Predictable error handling is required by callers and automation",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)
	statusCodeMappings := []struct {
		name    string
		goCode  ErrorCode
		goVal   int
		meaning string
	}{
		{"OK", ErrOK, 0, "Success, no error"},
		{"PARAM", ErrParam, 1, "Parameter validation error"},
		{"PARSE", ErrParse, 2, "Parse error"},
		{"FAIL", ErrFail, 3, "Generic failure"},
		{"TERRAFORM_ERROR", ErrTerraform, 4, "Terraform operation error"},
		{"MISSING_TOOL", ErrMissingTool, 5, "Required tool not found"},
		{"DESIREDSTATE_MISSING", ErrDesiredStateMissing, 6, "Desired state file not found"},
		{"DESIREDSTATE_MALFORMED", ErrDesiredStateMalformed, 7, "Desired state file malformed"},
		{"CONFIGURATION_MISSING", ErrConfigurationMissing, 8, "Configuration file not found"},
		{"CONFIGURATION_MALFORMED", ErrConfigurationMalformed, 9, "Configuration file malformed"},
		{"UNDEFINED", ErrUndefined, 255, "Undefined/unknown error"},
	}

	t.Run("StatusCodeMapping", func(t *testing.T) {
		goOnlyCount := 0

		for _, tc := range statusCodeMappings {
			t.Run(tc.name, func(t *testing.T) {
				// Verify Go code has expected numeric value
				actualGoVal := int(tc.goCode)
				if actualGoVal != tc.goVal {
					t.Errorf("Go ErrorCode value mismatch for %s: expected %d, got %d",
						tc.name, tc.goVal, actualGoVal)
				}

				// Verify ExitCode() returns correct value
				exitCode := tc.goCode.ExitCode()
				if exitCode != tc.goVal {
					t.Errorf("ExitCode() mismatch for %s: expected %d, got %d",
						tc.name, tc.goVal, exitCode)
				}

				goOnlyCount++
				t.Logf("✅ %s: %s=%d (%s)",
					tc.name, tc.goCode.String(), tc.goVal, tc.meaning)
			})
		}

		t.Logf("📊 Status Code Summary:")
		t.Logf("   - Codes verified: %d", goOnlyCount)
		t.Logf("   - Total Go codes: %d", len(statusCodeMappings))
	})

	t.Logf("✅ CONTRACT SATISFIED: Go error codes and exit codes are consistent")
}

// ============================================================================
// BEHAVIORAL CONTRACT: ERROR CODE STRING REPRESENTATION
// ============================================================================

// TestErrors_ErrorCodeString_BehavioralBDD verifies:
// - Error codes have string representations
// - String format is consistent and meaningful
func TestErrors_ErrorCodeString_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Provide string representation of error codes for logging and display",
		CurrentImpl:     "Current Go error implementation",
		ExpectedOutcome: "Error behavior remains stable and is validated by this test",
		Rationale:       "Predictable error handling is required by callers and automation",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	stringMappings := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrOK, "OK"},
		{ErrParam, "PARAM_ERROR"},
		{ErrParse, "PARSE_ERROR"},
		{ErrFail, "FAIL"},
		{ErrTerraform, "TERRAFORM_ERROR"},
		{ErrMissingTool, "MISSING_TOOL"},
		{ErrDesiredStateMissing, "DESIREDSTATE_MISSING"},
		{ErrDesiredStateMalformed, "DESIREDSTATE_MALFORMED"},
		{ErrConfigurationMissing, "CONFIGURATION_MISSING"},
		{ErrConfigurationMalformed, "CONFIGURATION_MALFORMED"},
		{ErrUndefined, "UNDEFINED"},
	}

	for _, tc := range stringMappings {
		actual := tc.code.String()
		if actual != tc.expected {
			t.Errorf("String() mismatch for code %d: expected %q, got %q",
				tc.code, tc.expected, actual)
		}
		t.Logf("✅ ErrorCode(%d).String() = %q", tc.code, actual)
	}

	// Test unknown code
	unknownCode := ErrorCode(999)
	unknownStr := unknownCode.String()
	if unknownStr != "UNKNOWN" {
		t.Errorf("Unknown code should return 'UNKNOWN', got %q", unknownStr)
	}

	t.Logf("✅ CONTRACT SATISFIED: Error codes have meaningful string representations")
}

// ============================================================================
// BEHAVIORAL CONTRACT: ERROR CREATION
// ============================================================================

// TestErrors_ErrorCreation_BehavioralBDD verifies:
// - Error messages are preserved and accessible
// - Errors have associated error codes/types
func TestErrors_ErrorCreation_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Create errors with custom messages and error codes",
		CurrentImpl:     "Current Go error implementation",
		ExpectedOutcome: "Error behavior remains stable and is validated by this test",
		Rationale:       "Predictable error handling is required by callers and automation",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Test basic error creation
	t.Run("BasicErrorCreation", func(t *testing.T) {
		message := "test error message"
		err := New(ErrParam, message)

		// Verify error is not nil
		if err == nil {
			t.Fatal("Expected non-nil error")
		}

		// Verify error code
		if err.Code != ErrParam {
			t.Errorf("Expected error code %v, got %v", ErrParam, err.Code)
		}

		// Verify message
		if err.Message != message {
			t.Errorf("Expected message %q, got %q", message, err.Message)
		}

		// Verify Error() includes message
		errorStr := err.Error()
		if !strings.Contains(errorStr, message) {
			t.Errorf("Error() should contain message %q, got %q", message, errorStr)
		}

		t.Logf("✅ Created error: %s", err.Error())
	})

	// Test formatted error creation
	t.Run("FormattedErrorCreation", func(t *testing.T) {
		err := Newf(ErrMissingTool, "tool %s not found", "terraform")

		expectedMsg := "tool terraform not found"
		if err.Message != expectedMsg {
			t.Errorf("Expected message %q, got %q", expectedMsg, err.Message)
		}

		t.Logf("✅ Created formatted error: %s", err.Error())
	})

	t.Logf("✅ CONTRACT SATISFIED: Errors created with codes and messages")
}

// ============================================================================
// BEHAVIORAL CONTRACT: ERROR WRAPPING/CHAINING
// ============================================================================

// TestErrors_ErrorWrapping_BehavioralBDD verifies:
// - Original error is preserved in wrapped error
// - Wrapped error includes both original and new context
func TestErrors_ErrorWrapping_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Wrap existing errors with additional context while preserving original",
		CurrentImpl:     "Current Go error implementation",
		ExpectedOutcome: "Error behavior remains stable and is validated by this test",
		Rationale:       "Predictable error handling is required by callers and automation",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Test error wrapping
	t.Run("WrapExistingError", func(t *testing.T) {
		// Create original error
		originalErr := fmt.Errorf("original error occurred")

		// Wrap it
		wrappedErr := Wrap(ErrFail, "operation failed", originalErr)

		// Verify wrapped error has cause
		if wrappedErr.Cause == nil {
			t.Fatal("Expected wrapped error to have Cause")
		}

		// Verify cause is original error
		if wrappedErr.Cause != originalErr {
			t.Errorf("Expected Cause to be original error")
		}

		// Verify Error() includes both messages
		errorStr := wrappedErr.Error()
		if !strings.Contains(errorStr, "operation failed") {
			t.Errorf("Error() should contain wrapper message, got %q", errorStr)
		}
		if !strings.Contains(errorStr, "original error occurred") {
			t.Errorf("Error() should contain original message, got %q", errorStr)
		}

		// Verify Unwrap() returns original error
		if unwrapped := wrappedErr.Unwrap(); unwrapped != originalErr {
			t.Errorf("Unwrap() should return original error")
		}

		t.Logf("✅ Wrapped error: %s", wrappedErr.Error())
		t.Logf("   Original: %s", originalErr)
	})

	// Test formatted wrapping
	t.Run("WrapWithFormatting", func(t *testing.T) {
		originalErr := fmt.Errorf("network timeout")
		wrappedErr := Wrapf(ErrFail, originalErr, "failed to connect to %s", "server.com")

		expectedMsg := "failed to connect to server.com"
		if wrappedErr.Message != expectedMsg {
			t.Errorf("Expected message %q, got %q", expectedMsg, wrappedErr.Message)
		}

		if wrappedErr.Cause != originalErr {
			t.Errorf("Expected Cause to be original error")
		}

		t.Logf("✅ Wrapped with formatting: %s", wrappedErr.Error())
	})

	// Test error chain traversal
	t.Run("ErrorChainTraversal", func(t *testing.T) {
		// Create error chain: err1 -> wrapped1 -> wrapped2
		err1 := fmt.Errorf("root cause")
		wrapped1 := Wrap(ErrParse, "parsing failed", err1)
		wrapped2 := Wrap(ErrFail, "operation failed", wrapped1)

		// Verify chain
		if wrapped2.Cause != wrapped1 {
			t.Errorf("wrapped2 should have wrapped1 as cause")
		}
		if wrapped1.Cause != err1 {
			t.Errorf("wrapped1 should have err1 as cause")
		}

		// Unwrap once
		level1 := wrapped2.Unwrap()
		if level1 != wrapped1 {
			t.Errorf("First unwrap should return wrapped1")
		}

		// Unwrap twice
		if gitopsErr, ok := level1.(*GitOpsError); ok {
			level2 := gitopsErr.Unwrap()
			if level2 != err1 {
				t.Errorf("Second unwrap should return err1")
			}
			t.Logf("✅ Error chain traversal successful: %v -> %v -> %v",
				wrapped2.Code, wrapped1.Code, err1)
		}
	})

	t.Logf("✅ CONTRACT SATISFIED: Error wrapping preserves original errors")
}

// ============================================================================
// BEHAVIORAL CONTRACT: CALLER INFORMATION
// ============================================================================

// TestErrors_CallerInformation_BehavioralBDD verifies:
// - File name and line number are recorded
// - Caller context helps with debugging
func TestErrors_CallerInformation_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Capture caller information (file, line, function) for debugging",
		CurrentImpl:     "Current Go error implementation",
		ExpectedOutcome: "Error behavior remains stable and is validated by this test",
		Rationale:       "Predictable error handling is required by callers and automation",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Test caller information capture
	t.Run("CallerInfoCapture", func(t *testing.T) {
		// Create error (New() automatically calls WithCaller())
		err := New(ErrParam, "test error with caller info")

		// Verify file is captured
		if err.File == "" {
			t.Error("Expected File to be captured")
		}

		// Verify line number is captured
		if err.Line == 0 {
			t.Error("Expected Line to be captured")
		}

		// Verify function name is captured
		if err.Function == "" {
			t.Error("Expected Function to be captured")
		}

		// File should contain errors.go (New() calls WithCaller internally)
		// NOTE: This is expected - WithCaller captures its own location
		if !strings.Contains(err.File, "errors.go") {
			t.Errorf("Expected File to contain errors.go, got %q", err.File)
		}

		// Function should contain New (the wrapper function)
		if !strings.Contains(err.Function, "New") {
			t.Errorf("Expected Function to contain 'New', got %q", err.Function)
		}

		t.Logf("✅ Caller info captured (from New() wrapper):")
		t.Logf("   File: %s", err.File)
		t.Logf("   Line: %d", err.Line)
		t.Logf("   Function: %s", err.Function)
		t.Logf("   Note: WithCaller() captures wrapper location, similar to Go's fail()")
	})

	// Test String() includes caller info
	t.Run("CallerInfoInString", func(t *testing.T) {
		err := New(ErrFail, "error with context")
		str := err.String()

		// String should include file (errors.go where New() is defined)
		if !strings.Contains(str, "errors.go") {
			t.Errorf("String() should contain errors.go, got %q", str)
		}

		// String should include function info
		if !strings.Contains(str, "in ") {
			t.Errorf("String() should contain function info, got %q", str)
		}

		t.Logf("✅ String with caller: %s", str)
		t.Logf("   Note: Shows New() location (wrapper), not test location")
	})

	t.Logf("✅ CONTRACT SATISFIED: Caller information captured and available")
}

// ============================================================================
// BEHAVIORAL CONTRACT: ERROR CODE DETECTION
// ============================================================================

// TestErrors_ErrorCodeDetection_BehavioralBDD verifies:
// - Can detect error code from error instance
// - Can check if error matches specific code
// - Can extract exit code from error
func TestErrors_ErrorCodeDetection_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Detect and extract error codes from error instances",
		CurrentImpl:     "Current Go error implementation",
		ExpectedOutcome: "Error behavior remains stable and is validated by this test",
		Rationale:       "Predictable error handling is required by callers and automation",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Test IsCode()
	t.Run("IsCode", func(t *testing.T) {
		err := New(ErrParam, "parameter error")

		// Should match correct code
		if !IsCode(err, ErrParam) {
			t.Error("IsCode should return true for matching code")
		}

		// Should not match different code
		if IsCode(err, ErrFail) {
			t.Error("IsCode should return false for non-matching code")
		}

		// Should handle non-GitOpsError
		stdErr := fmt.Errorf("standard error")
		if IsCode(stdErr, ErrParam) {
			t.Error("IsCode should return false for non-GitOpsError")
		}

		t.Logf("✅ IsCode correctly identifies error codes")
	})

	// Test GetCode()
	t.Run("GetCode", func(t *testing.T) {
		err := New(ErrTerraform, "terraform failed")

		code := GetCode(err)
		if code != ErrTerraform {
			t.Errorf("GetCode expected %v, got %v", ErrTerraform, code)
		}

		// Test with non-GitOpsError
		stdErr := fmt.Errorf("standard error")
		code = GetCode(stdErr)
		if code != ErrUndefined {
			t.Errorf("GetCode should return ErrUndefined for non-GitOpsError, got %v", code)
		}

		t.Logf("✅ GetCode extracts error codes correctly")
	})

	// Test GetExitCode()
	t.Run("GetExitCode", func(t *testing.T) {
		// Test with nil error
		exitCode := GetExitCode(nil)
		if exitCode != 0 {
			t.Errorf("GetExitCode(nil) should return 0, got %d", exitCode)
		}

		// Test with GitOpsError
		err := New(ErrParam, "param error")
		exitCode = GetExitCode(err)
		if exitCode != int(ErrParam) {
			t.Errorf("GetExitCode expected %d, got %d", int(ErrParam), exitCode)
		}

		// Test with non-GitOpsError
		stdErr := fmt.Errorf("standard error")
		exitCode = GetExitCode(stdErr)
		if exitCode != int(ErrUndefined) {
			t.Errorf("GetExitCode should return %d for non-GitOpsError, got %d",
				int(ErrUndefined), exitCode)
		}

		t.Logf("✅ GetExitCode returns correct exit codes")
	})

	t.Logf("✅ CONTRACT SATISFIED: Error code detection working correctly")
}

// ============================================================================
// BEHAVIORAL CONTRACT: CONVENIENCE CONSTRUCTORS
// ============================================================================

// TestErrors_ConvenienceConstructors_BehavioralBDD verifies:
// - Convenience functions exist for common error types
// - Convenience functions create correct error codes
// - Convenience functions provide appropriate messages
func TestErrors_ConvenienceConstructors_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Provide convenience constructors for common error types",
		CurrentImpl:     "Current Go error implementation",
		ExpectedOutcome: "Error behavior remains stable and is validated by this test",
		Rationale:       "Predictable error handling is required by callers and automation",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	convenienceTests := []struct {
		name         string
		constructor  func() *GitOpsError
		expectedCode ErrorCode
		messagePart  string
	}{
		{
			name:         "ParamError",
			constructor:  func() *GitOpsError { return NewParamError("invalid param") },
			expectedCode: ErrParam,
			messagePart:  "invalid param",
		},
		{
			name:         "MissingToolError",
			constructor:  func() *GitOpsError { return NewMissingToolError("kubectl") },
			expectedCode: ErrMissingTool,
			messagePart:  "kubectl",
		},
		{
			name:         "ConfigurationMissingError",
			constructor:  func() *GitOpsError { return NewConfigurationMissingError("/path/to/config") },
			expectedCode: ErrConfigurationMissing,
			messagePart:  "/path/to/config",
		},
	}

	for _, tc := range convenienceTests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.constructor()

			// Verify error code
			if err.Code != tc.expectedCode {
				t.Errorf("Expected code %v, got %v", tc.expectedCode, err.Code)
			}

			// Verify message contains expected part
			if !strings.Contains(err.Message, tc.messagePart) {
				t.Errorf("Expected message to contain %q, got %q", tc.messagePart, err.Message)
			}

			// Verify caller info is captured
			if err.File == "" || err.Line == 0 {
				t.Error("Expected caller info to be captured")
			}

			t.Logf("✅ %s: code=%v, message=%q", tc.name, err.Code, err.Message)
		})
	}

	t.Logf("✅ CONTRACT SATISFIED: %d convenience constructors working correctly",
		len(convenienceTests))
}

// ============================================================================
// BEHAVIORAL CONTRACT: ERROR INTERFACE COMPLIANCE
// ============================================================================

// TestErrors_ErrorInterface_BehavioralBDD verifies:
// - GitOpsError implements error interface correctly
// - Error() returns meaningful string
// - Error can be used anywhere standard error is expected
func TestErrors_ErrorInterface_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Implement standard error interface for interoperability",
		CurrentImpl:     "Current Go error implementation",
		ExpectedOutcome: "Error behavior remains stable and is validated by this test",
		Rationale:       "Predictable error handling is required by callers and automation",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Test that GitOpsError implements error interface
	t.Run("ErrorInterface", func(t *testing.T) {
		var err error = New(ErrParam, "test error")

		// Verify it's an error
		if err == nil {
			t.Fatal("Expected non-nil error")
		}

		// Verify Error() returns non-empty string
		errorStr := err.Error()
		if errorStr == "" {
			t.Error("Error() should return non-empty string")
		}

		// Verify Error() includes error code
		if !strings.Contains(errorStr, "PARAM_ERROR") {
			t.Errorf("Error() should include error code name, got %q", errorStr)
		}

		// Verify Error() includes message
		if !strings.Contains(errorStr, "test error") {
			t.Errorf("Error() should include message, got %q", errorStr)
		}

		t.Logf("✅ GitOpsError implements error interface: %s", errorStr)
	})

	// Test error can be used in functions expecting error
	t.Run("ErrorAsParameter", func(t *testing.T) {
		// Function accepting standard error
		checkError := func(err error) string {
			if err == nil {
				return "no error"
			}
			return err.Error()
		}

		gitopsErr := New(ErrFail, "operation failed")
		result := checkError(gitopsErr)

		if result == "no error" {
			t.Error("GitOpsError should be recognized as error")
		}

		if !strings.Contains(result, "operation failed") {
			t.Errorf("Expected error message in result, got %q", result)
		}

		t.Logf("✅ GitOpsError usable as standard error: %s", result)
	})

	t.Logf("✅ CONTRACT SATISFIED: GitOpsError fully compatible with error interface")
}

// ============================================================================
// GO ENHANCEMENT DOCUMENTATION TESTS
// ============================================================================
// ============================================================================

// TestError_ParseStatusCode_GoEnhancement documents Go's PARSE status code
//
// Go Implementation (internal/utils/errors):
//   - Constant: PARSE ErrorStatus = 4
//   - Purpose: Distinguish parsing errors from other failures
//   - Behavior: Separate exit code for YAML/JSON/config parsing errors
//
// Rationale: Go enhancement for better error categorization
func TestError_ParseStatusCode_GoEnhancement(t *testing.T) {
	t.Log("\n=== GO ENHANCEMENT: PARSE status code (4) ===")

	t.Log("Go Implementation:")
	t.Log("  const (")
	t.Log("      SUCCESS ErrorStatus = 0")
	t.Log("      ERROR   ErrorStatus = 1")
	t.Log("      WARNING ErrorStatus = 2")
	t.Log("      FATAL   ErrorStatus = 3")
	t.Log("      PARSE   ErrorStatus = 4  // Go enhancement")
	t.Log("  )")
	t.Log("")
	t.Log("  func NewGitOpsError(status ErrorStatus, message string, args ...interface{}) *GitOpsError {")
	t.Log("      return &GitOpsError{")
	t.Log("          Status:  status,")
	t.Log("          Message: fmt.Sprintf(message, args...),")
	t.Log("      }")
	t.Log("  }")
	t.Log("")
	t.Log("  Example:")
	t.Log("    // YAML parsing error")
	t.Log("    err := NewGitOpsError(PARSE, \"Invalid YAML:\", parseErr)")
	t.Log("    // Exit code: 4 (distinct from generic ERROR=1)")
	t.Log("")
	t.Log("    // JSON parsing error")
	t.Log("    err := NewGitOpsError(PARSE, \"Invalid JSON in file\", filename)")
	t.Log("    // Exit code: 4")

	t.Log("\nGo Status: NOT IMPLEMENTED")
	t.Log("  Go's error handling uses only:")
	t.Log("    - SUCCESS (0)")
	t.Log("    - ERROR (1)")
	t.Log("    - WARNING (2)")
	t.Log("    - FATAL (3)")
	t.Log("  No separate PARSE status - parsing errors use ERROR (1)")

	t.Log("\nGo Enhancement Rationale:")
	t.Log("  1. Error categorization - distinguish parsing vs logic errors")
	t.Log("  2. Script automation - different handling for config vs runtime errors")
	t.Log("  3. Debugging - parsing errors indicate config issues")
	t.Log("  4. Exit codes - shell scripts can handle parse errors differently")
	t.Log("  5. Monitoring - separate metrics for parse failures")

	t.Log("\nWhy NOT Reverse Port to Go:")
	t.Log("  - Go codebase doesn't distinguish parse vs other errors")
	t.Log("  - Would break existing scripts expecting ERROR (1)")
	t.Log("  - Low priority - error categorization not needed in Go workflows")
	t.Log("  - Go enhancement addresses Go-specific error handling needs")

	t.Log("\nUse Cases in Go:")
	t.Log("  - YAML parsing errors: Status = PARSE (4)")
	t.Log("  - JSON parsing errors: Status = PARSE (4)")
	t.Log("  - Config validation errors: Status = PARSE (4)")
	t.Log("  - Invalid schema errors: Status = PARSE (4)")

	t.Log("\n🟢 Go Enhancement - NOT in Go")
	t.Log("📋 Category: Error Categorization")
	t.Log("⏱️  Implementation effort (if porting): 2-3 hours")
	t.Log("💡 Decision: Keep as Go-only enhancement")
	t.Log("⚠️  Breaking change if ported: Would change Go exit codes")
}

// TestError_FailStatusCode_GoEnhancement documents Go's FAIL status code
//
// Go Implementation (internal/utils/errors):
//   - Constant: FAIL ErrorStatus = 5
//   - Purpose: Distinguish validation/assertion failures from errors
//   - Behavior: Separate exit code for test/validation failures
//
// Rationale: Go enhancement for testing and validation
func TestError_FailStatusCode_GoEnhancement(t *testing.T) {
	t.Log("\n=== GO ENHANCEMENT: FAIL status code (5) ===")

	t.Log("Go Implementation:")
	t.Log("  const (")
	t.Log("      SUCCESS ErrorStatus = 0")
	t.Log("      ERROR   ErrorStatus = 1")
	t.Log("      WARNING ErrorStatus = 2")
	t.Log("      FATAL   ErrorStatus = 3")
	t.Log("      PARSE   ErrorStatus = 4")
	t.Log("      FAIL    ErrorStatus = 5  // Go enhancement")
	t.Log("  )")
	t.Log("")
	t.Log("  Example:")
	t.Log("    // Validation failure")
	t.Log("    err := NewGitOpsError(FAIL, \"Validation failed: expected X, got Y\", expected, actual)")
	t.Log("    // Exit code: 5 (distinct from ERROR=1)")
	t.Log("")
	t.Log("    // Assertion failure")
	t.Log("    err := NewGitOpsError(FAIL, \"Assertion failed: field should not be empty\", field)")
	t.Log("    // Exit code: 5")
	t.Log("")
	t.Log("    // Test failure")
	t.Log("    err := NewGitOpsError(FAIL, \"Test failed: testName\", testName)")
	t.Log("    // Exit code: 5")

	t.Log("\nGo Status: NOT IMPLEMENTED")
	t.Log("  Go's error handling uses only:")
	t.Log("    - SUCCESS (0)")
	t.Log("    - ERROR (1)")
	t.Log("    - WARNING (2)")
	t.Log("    - FATAL (3)")
	t.Log("  No separate FAIL status - validation errors use ERROR (1)")

	t.Log("\nGo Enhancement Rationale:")
	t.Log("  1. Test categorization - distinguish validation vs runtime errors")
	t.Log("  2. CI/CD pipelines - different handling for failures vs errors")
	t.Log("  3. Validation semantics - 'failed check' vs 'unexpected error'")
	t.Log("  4. Exit codes - automation can treat failures differently")
	t.Log("  5. Debugging - FAIL indicates expected condition not met")

	t.Log("\nWhy NOT Reverse Port to Go:")
	t.Log("  - Go codebase doesn't distinguish validation vs runtime errors")
	t.Log("  - Would break existing scripts expecting ERROR (1)")
	t.Log("  - Low priority - error type distinction not needed in Go workflows")
	t.Log("  - Go enhancement addresses Go-specific testing/validation needs")

	t.Log("\nUse Cases in Go:")
	t.Log("  - Configuration validation failures: Status = FAIL (5)")
	t.Log("  - Schema validation failures: Status = FAIL (5)")
	t.Log("  - Assertion failures: Status = FAIL (5)")
	t.Log("  - Pre-condition check failures: Status = FAIL (5)")

	t.Log("\n🟢 Go Enhancement - NOT in Go")
	t.Log("📋 Category: Testing & Validation")
	t.Log("⏱️  Implementation effort (if porting): 2-3 hours")
	t.Log("💡 Decision: Keep as Go-only enhancement")
	t.Log("⚠️  Breaking change if ported: Would change Go exit codes")
	t.Log("💡 Semantic distinction: FAIL = expected check failed, ERROR = unexpected problem")
}
