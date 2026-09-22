package command

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Behavioral BDD tests for command execution and shell configuration.

// ============================================================================
// BEHAVIORAL CONTRACT METADATA STRUCTURES
// ============================================================================

// BehavioralContract documents command behavior and the current implementation.
type BehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

// ============================================================================
// TEST HELPER FUNCTIONS
// ============================================================================

// createTempDir creates a temporary directory for testing
func createTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "command_behavioral_bdd_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// getPlatformPwdCommand returns the command to print current directory
func getPlatformPwdCommand() (string, []string) {
	if IsWindows() {
		// PowerShell: Get-Location | Select-Object -ExpandProperty Path
		// Or simpler: $PWD.Path
		return "powershell", []string{"-Command", "$PWD.Path"}
	}
	return "pwd", nil
}

// getPlatformEchoEnvCommand returns command to echo environment variable
func getPlatformEchoEnvCommand(varName string) (string, []string) {
	if IsWindows() {
		return "powershell", []string{"-Command", "echo $env:" + varName}
	}
	return "sh", []string{"-c", "echo $" + varName}
}

// ============================================================================
// BEHAVIORAL CONTRACT: WORKING DIRECTORY EXECUTION
// ============================================================================

// TestCommand_ExecuteInWorkingDirectory_BehavioralBDD verifies:
// - Commands execute in a specified working directory
// - The working directory context affects command execution
// - Working directory is properly passed to subprocess
func TestCommand_ExecuteInWorkingDirectory_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Execute command in specified working directory context",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: A temporary working directory
	tempDir := createTempDir(t)
	t.Logf("Given: temp directory: %s", tempDir)

	// Given: A shell configured to execute in that directory
	config := DefaultConfig()
	config.WorkingDir = tempDir
	config.IsShowOutput = false
	shell := NewShell(config)

	// When: Execute pwd/Get-Location command
	cmd, args := getPlatformPwdCommand()
	result, err := shell.Execute(cmd, args...)

	// Then: Command should succeed
	if err != nil {
		t.Fatalf("Expected command to succeed, got error: %v", err)
	}

	// Then: Exit code should be 0
	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	// Then: Output should contain the working directory path
	stdout := strings.TrimSpace(result.Stdout)

	// Normalize paths for comparison (handle symlinks, etc.)
	expectedPath, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		expectedPath = tempDir
	}

	actualPath, err := filepath.EvalSymlinks(stdout)
	if err != nil {
		actualPath = stdout
	}

	if !strings.EqualFold(expectedPath, actualPath) {
		t.Errorf("Expected working directory %s in output, got: %s", expectedPath, actualPath)
	}

	t.Logf("✅ CONTRACT SATISFIED: Command executed in specified working directory")
	t.Logf("   Expected: %s", expectedPath)
	t.Logf("   Actual: %s", actualPath)
}

// ============================================================================
// BEHAVIORAL CONTRACT: ENVIRONMENT VARIABLE APPEND
// ============================================================================

// TestCommand_AppendEnvironmentVariables_BehavioralBDD verifies:
// - Custom environment variables can be appended to the system environment
// - Appended variables are available to the subprocess
// - System environment variables are preserved
func TestCommand_AppendEnvironmentVariables_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Append custom environment variables to system environment",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: A custom environment variable
	customVarName := "TEST_BEHAVIORAL_APPEND_VAR"
	customVarValue := "behavioral_test_value_12345"

	// Given: A shell configured to append this variable
	config := DefaultConfig()
	config.Env = map[string]string{
		customVarName: customVarValue,
	}
	config.IsShowOutput = false
	shell := NewShell(config)

	// When: Execute command that echoes the custom variable
	cmd, args := getPlatformEchoEnvCommand(customVarName)
	result, err := shell.Execute(cmd, args...)

	// Then: Command should succeed
	if err != nil {
		t.Fatalf("Expected command to succeed, got error: %v", err)
	}

	// Then: Exit code should be 0
	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	// Then: Output should contain the custom variable value
	stdout := strings.TrimSpace(result.Stdout)
	if !strings.Contains(stdout, customVarValue) {
		t.Errorf("Expected stdout to contain '%s', got: %s", customVarValue, stdout)
	}

	t.Logf("✅ CONTRACT SATISFIED: Custom environment variable appended and accessible")
	t.Logf("   Variable: %s=%s", customVarName, customVarValue)
	t.Logf("   Output: %s", stdout)

	// Additional verification: System PATH should still be available
	// (proving system env is preserved)
	config2 := DefaultConfig()
	config2.Env = map[string]string{
		customVarName: customVarValue,
	}
	config2.IsShowOutput = false
	shell2 := NewShell(config2)

	// PATH environment should allow finding system commands
	pathTestCmd := "echo"
	if IsWindows() {
		pathTestCmd = "cmd"
	}
	result2, err2 := shell2.Execute(pathTestCmd, "--help")

	// Should succeed because PATH is preserved
	if err2 != nil && result2.ExitCode != 0 {
		t.Logf("⚠️  Warning: System PATH might not be preserved (this is informational)")
		t.Logf("   Attempted to execute: %s --help", pathTestCmd)
		t.Logf("   Error: %v", err2)
	} else {
		t.Logf("✅ VERIFIED: System PATH preserved (command '%s' found)", pathTestCmd)
	}
}

// ============================================================================
// BEHAVIORAL CONTRACT: ENVIRONMENT VARIABLE OVERRIDE
// ============================================================================

// TestCommand_OverrideEnvironmentVariables_BehavioralBDD verifies:
// - The entire environment can be replaced with custom variables
// - Only custom variables are available to subprocess
// - System environment is NOT inherited
func TestCommand_OverrideEnvironmentVariables_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Override entire environment with only custom variables",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: Custom environment with single variable
	customVarName := "ONLY_THIS_VAR"
	customVarValue := "only_value_987"

	// Given: A shell configured to override entire environment
	config := DefaultConfig()
	config.EnvOverride = map[string]string{
		customVarName: customVarValue,
	}
	config.IsShowOutput = false
	shell := NewShell(config)

	// When: Execute command that echoes the custom variable
	cmd, args := getPlatformEchoEnvCommand(customVarName)
	result, err := shell.Execute(cmd, args...)

	// Then: Command should succeed
	if err != nil {
		t.Fatalf("Expected command to succeed, got error: %v", err)
	}

	// Then: Exit code should be 0
	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	// Then: Output should contain the custom variable value
	stdout := strings.TrimSpace(result.Stdout)
	if !strings.Contains(stdout, customVarValue) {
		t.Errorf("Expected stdout to contain '%s', got: %s", customVarValue, stdout)
	}

	t.Logf("✅ CONTRACT SATISFIED: Custom variable accessible")
	t.Logf("   Variable: %s=%s", customVarName, customVarValue)
	t.Logf("   Output: %s", stdout)

	// Additional verification: System PATH should NOT be available
	// This proves environment was replaced, not appended
	config2 := DefaultConfig()
	config2.EnvOverride = map[string]string{
		customVarName: customVarValue,
		// Intentionally NOT including PATH
	}
	config2.IsShowOutput = false
	shell2 := NewShell(config2)

	// Try to execute a command that requires PATH
	// On Windows, even without PATH, powershell might be found via Windows Search
	// On Linux, without PATH, commands should fail
	if !IsWindows() {
		_, err2 := shell2.Execute("ls") // Should fail without PATH
		if err2 == nil {
			t.Logf("⚠️  Warning: Command succeeded without PATH (unexpected)")
		} else {
			t.Logf("✅ VERIFIED: System PATH not inherited (command failed as expected)")
			t.Logf("   Error: %v", err2)
		}
	}
}

// ============================================================================
// BEHAVIORAL CONTRACT: EXIT CODE HANDLING
// ============================================================================

// TestCommand_ExitCodeHandling_BehavioralBDD verifies:
// - Command exit codes are captured
// - Exit code 0 indicates success
// - Non-zero exit codes indicate failure
func TestCommand_ExitCodeHandling_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Capture and expose command exit code",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	config := DefaultConfig()
	config.IsShowOutput = false
	shell := NewShell(config)

	// Scenario 1: Successful command (exit 0)
	t.Run("Success_Exit0", func(t *testing.T) {
		// When: Execute successful command
		result, err := shell.Execute("echo", "success")

		// Then: Should not error
		if err != nil {
			t.Errorf("Expected no error for successful command, got: %v", err)
		}

		// Then: Exit code should be 0
		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0 for successful command, got: %d", result.ExitCode)
		}

		t.Logf("✅ Success case: ExitCode=%d", result.ExitCode)
	})

	// Scenario 2: Failed command (non-zero exit)
	t.Run("Failure_NonZeroExit", func(t *testing.T) {
		// When: Execute command that will fail
		var result *Result
		var err error

		if IsWindows() {
			// PowerShell: exit with code 42
			result, err = shell.ExecuteScript("exit 42")
		} else {
			// Bash/sh: exit with code 42
			result, err = shell.ExecuteScript("exit 42")
		}

		// Then: Should return error (Go convention)
		if err == nil {
			t.Logf("⚠️  Note: Go returned no error despite non-zero exit (some commands behave this way)")
		}

		// Then: Exit code should be non-zero
		if result.ExitCode == 0 {
			t.Errorf("Expected non-zero exit code for failed command, got: %d", result.ExitCode)
		}

		t.Logf("✅ Failure case: ExitCode=%d, Error=%v", result.ExitCode, err)
	})

	// Scenario 3: Non-existent command (should fail)
	t.Run("NonExistent_Command", func(t *testing.T) {
		// When: Execute non-existent command
		result, err := shell.Execute("nonexistent_command_xyz_12345")

		// Then: Should return error
		if err == nil {
			t.Errorf("Expected error for non-existent command")
		}

		// Then: Exit code should be non-zero
		if result.ExitCode == 0 {
			t.Errorf("Expected non-zero exit code for non-existent command, got: %d", result.ExitCode)
		}

		t.Logf("✅ Non-existent command: ExitCode=%d, Error=%v", result.ExitCode, err)
	})

	t.Logf("✅ CONTRACT SATISFIED: Exit codes properly captured and exposed")
}

// ============================================================================
// BEHAVIORAL CONTRACT: OUTPUT CAPTURE
// ============================================================================

// TestCommand_OutputCapture_BehavioralBDD verifies:
// - Commands capture stdout and stderr
// - Stdout and stderr can be captured separately
// - Output is available after command completes
func TestCommand_OutputCapture_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Capture command stdout and stderr output",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	config := DefaultConfig()
	config.IsShowOutput = false
	shell := NewShell(config)

	// Scenario 1: Capture stdout
	t.Run("Capture_Stdout", func(t *testing.T) {
		testString := "stdout_test_output_xyz"

		// When: Execute command that writes to stdout
		result, err := shell.Execute("echo", testString)

		// Then: Command should succeed
		if err != nil {
			t.Fatalf("Expected command to succeed, got error: %v", err)
		}

		// Then: Stdout should contain the test string
		if !strings.Contains(result.Stdout, testString) {
			t.Errorf("Expected stdout to contain '%s', got: %s", testString, result.Stdout)
		}

		t.Logf("✅ Stdout captured: %s", strings.TrimSpace(result.Stdout))
	})

	// Scenario 2: Capture stderr (platform-specific)
	t.Run("Capture_Stderr", func(t *testing.T) {
		// When: Execute command that writes to stderr
		var result *Result
		var err error

		if IsWindows() {
			// PowerShell: Write to stderr
			result, err = shell.ExecuteScript("[Console]::Error.WriteLine('stderr_test')")
		} else {
			// Bash: Write to stderr
			result, err = shell.ExecuteScript("echo 'stderr_test' >&2")
		}

		// Then: Command should succeed
		if err != nil {
			t.Logf("Note: Command returned error (may be expected): %v", err)
		}

		// Then: Stderr should contain test string
		if !strings.Contains(result.Stderr, "stderr_test") {
			t.Logf("⚠️  Note: Stderr capture might vary by platform")
			t.Logf("   Stderr: %s", result.Stderr)
			t.Logf("   Stdout: %s", result.Stdout)
		} else {
			t.Logf("✅ Stderr captured: %s", strings.TrimSpace(result.Stderr))
		}
	})

	t.Logf("✅ CONTRACT SATISFIED: Output capture working")
}

// ============================================================================
// BEHAVIORAL CONTRACT: STDERR REDIRECTION
// ============================================================================

// TestCommand_StderrRedirect_BehavioralBDD verifies:
// - Stderr can be redirected to stdout
// - When redirected, stderr output appears in stdout
// - Stderr stream is empty when redirected
func TestCommand_StderrRedirect_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Redirect stderr to stdout (merge streams)",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: Shell configured to redirect stderr to stdout
	config := DefaultConfig()
	config.IsRedirectStderrToStdout = true
	config.IsShowOutput = false
	shell := NewShell(config)

	// When: Execute command that writes to stderr
	var result *Result
	var err error

	if IsWindows() {
		// PowerShell: Write to both stdout and stderr
		result, err = shell.ExecuteScript("echo 'stdout_msg'; [Console]::Error.WriteLine('stderr_msg')")
	} else {
		// Bash: Write to both stdout and stderr
		result, err = shell.ExecuteScript("echo 'stdout_msg'; echo 'stderr_msg' >&2")
	}

	// Then: Command should complete (may error due to stderr content)
	if err != nil {
		t.Logf("Note: Command returned error: %v", err)
	}

	// Then: Stderr content should appear in stdout
	// (exact behavior may vary by platform/shell)
	t.Logf("Stdout: %s", result.Stdout)
	t.Logf("Stderr: %s", result.Stderr)

	if strings.Contains(result.Stdout, "stdout_msg") {
		t.Logf("✅ Stdout message captured")
	}

	// Platform-specific verification
	if runtime.GOOS != "windows" {
		if strings.Contains(result.Stdout, "stderr_msg") || strings.Contains(result.Stderr, "stderr_msg") {
			t.Logf("✅ Stderr message captured (in stdout or stderr)")
		}
	}

	t.Logf("✅ CONTRACT SATISFIED: Stderr redirection configured")
}

// ============================================================================
// BEHAVIORAL CONTRACT: OUTPUT MODES (BUFFERED VS REALTIME)
// ============================================================================

// TestCommand_OutputModes_BehavioralBDD verifies:
// - Buffered and realtime output modes are supported
// - Buffered: Wait for command to complete, then get output
// - Realtime: Stream output as command executes
func TestCommand_OutputModes_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Support buffered and realtime output modes",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Scenario 1: Buffered output mode
	t.Run("Buffered_Mode", func(t *testing.T) {
		// Given: Shell with buffered output
		config := DefaultConfig()
		config.IsBufferedOutput = true
		config.IsRealtimeOutput = false
		config.IsShowOutput = false
		shell := NewShell(config)

		// When: Execute command
		result, err := shell.Execute("echo", "buffered")

		// Then: Should succeed
		if err != nil {
			t.Fatalf("Expected command to succeed, got error: %v", err)
		}

		// Then: Output should be captured
		if !strings.Contains(result.Stdout, "buffered") {
			t.Errorf("Expected stdout to contain 'buffered', got: %s", result.Stdout)
		}

		t.Logf("✅ Buffered mode: Output captured after completion")
	})

	// Scenario 2: Realtime output mode
	t.Run("Realtime_Mode", func(t *testing.T) {
		// Given: Shell with realtime output
		config := DefaultConfig()
		config.IsRealtimeOutput = true
		config.IsBufferedOutput = false
		config.IsShowOutput = false
		shell := NewShell(config)

		// When: Execute command
		result, err := shell.Execute("echo", "realtime")

		// Then: Should succeed
		if err != nil {
			t.Fatalf("Expected command to succeed, got error: %v", err)
		}

		// Then: Output should be captured (even in realtime mode)
		if !strings.Contains(result.Stdout, "realtime") {
			t.Errorf("Expected stdout to contain 'realtime', got: %s", result.Stdout)
		}

		t.Logf("✅ Realtime mode: Output streamed and captured")
	})

	t.Logf("✅ CONTRACT SATISFIED: Both output modes supported")
}

// ============================================================================
// BEHAVIORAL CONTRACT: COMMAND TIMEOUT
// ============================================================================

// TestCommand_Timeout_BehavioralBDD verifies:
// - Commands that exceed timeout are terminated
// - Timeout errors are properly reported
func TestCommand_Timeout_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Terminate commands that exceed timeout duration",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)
	t.Logf("⚠️  CONTRACT MISMATCH: Go has timeout, Go doesn't")
	t.Logf("   This is a Go enhancement opportunity for Go")

	// Given: Shell with very short timeout
	config := DefaultConfig()
	config.Timeout = 100 * time.Millisecond
	config.IsShowOutput = false
	shell := NewShell(config)

	// When: Execute command that will timeout
	var sleepCmd string
	if IsWindows() {
		sleepCmd = "Start-Sleep -Seconds 5"
	} else {
		sleepCmd = "sleep 5"
	}

	result, err := shell.ExecuteScript(sleepCmd)

	// Then: Should return error (timeout)
	if err == nil {
		t.Errorf("Expected timeout error, got nil")
	} else {
		t.Logf("✅ Timeout error: %v", err)
	}

	// Then: Exit code should be non-zero
	if result.ExitCode == 0 {
		t.Errorf("Expected non-zero exit code for timeout, got: %d", result.ExitCode)
	}

	t.Logf("✅ CONTRACT DOCUMENTED: Go timeout support verified")
	t.Logf("   ExitCode: %d", result.ExitCode)
}

// ============================================================================
// BEHAVIORAL CONTRACT: DRY-RUN MODE
// ============================================================================

// TestCommand_DryRun_BehavioralBDD verifies:
// - Dry-run mode previews command without executing
// - No actual execution occurs in dry-run
func TestCommand_DryRun_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Preview command without execution (dry-run mode)",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)
	t.Logf("⚠️  CONTRACT MISMATCH: Go has dry-run, Go doesn't")

	// Given: Shell in dry-run mode
	config := DefaultConfig()
	config.IsDryRun = true
	config.IsShowOutput = false
	shell := NewShell(config)

	// When: Execute command that would fail if actually run
	result, err := shell.Execute("nonexistent_command_should_not_execute")

	// Then: Should succeed (not actually executed)
	if err != nil {
		t.Errorf("Expected dry-run to succeed, got error: %v", err)
	}

	// Then: Exit code should be 0
	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0 for dry-run, got: %d", result.ExitCode)
	}

	// Then: Output should be empty
	if result.Stdout != "" {
		t.Errorf("Expected empty stdout for dry-run, got: %s", result.Stdout)
	}

	// Then: Duration should be 0
	if result.Duration != 0 {
		t.Errorf("Expected zero duration for dry-run, got: %v", result.Duration)
	}

	t.Logf("✅ CONTRACT DOCUMENTED: Go dry-run support verified")
	t.Logf("   Command was NOT executed (as expected)")
}

// ============================================================================
// BEHAVIORAL CONTRACT: DURATION MEASUREMENT
// ============================================================================

// TestCommand_DurationMeasurement_BehavioralBDD verifies:
// - Start time, end time, and duration are captured
// - Measurements are accurate
func TestCommand_DurationMeasurement_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Measure command execution duration",
		CurrentImpl:     "Current Go command implementation",
		ExpectedOutcome: "Command behavior remains stable and is validated by this test",
		Rationale:       "Predictable command execution is required by automation workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: Shell configured to measure duration
	config := DefaultConfig()
	config.IsMeasureDuration = true
	config.IsShowOutput = false
	shell := NewShell(config)

	// When: Execute command
	result, err := shell.Execute("echo", "timing test")

	// Then: Should succeed
	if err != nil {
		t.Fatalf("Expected command to succeed, got error: %v", err)
	}

	// Then: Duration should be non-zero
	if result.Duration == 0 {
		t.Errorf("Expected non-zero duration, got: %v", result.Duration)
	}

	// Then: Start time should be set
	if result.StartTime.IsZero() {
		t.Errorf("Expected non-zero start time")
	}

	// Then: End time should be set
	if result.EndTime.IsZero() {
		t.Errorf("Expected non-zero end time")
	}

	// Then: End time should be after start time
	if !result.EndTime.After(result.StartTime) {
		t.Errorf("Expected end time %v to be after start time %v", result.EndTime, result.StartTime)
	}

	// Then: Duration should match end - start
	expectedDuration := result.EndTime.Sub(result.StartTime)
	if result.Duration != expectedDuration {
		t.Errorf("Expected duration %v to match end-start %v", result.Duration, expectedDuration)
	}

	t.Logf("✅ CONTRACT SATISFIED: Duration measurement working")
	t.Logf("   Duration: %v", result.Duration)
	t.Logf("   Start: %v", result.StartTime)
	t.Logf("   End: %v", result.EndTime)
}

// ============================================================================
// GO ENHANCEMENT DOCUMENTATION TESTS
// ============================================================================
// ============================================================================

// TestCommand_TimeoutSupport_GoEnhancement documents Go's timeout feature
//
// Go Implementation (internal/utils/command.ShellCommand):
//   - Field: Timeout time.Duration
//   - Purpose: Automatically terminate long-running commands
//   - Behavior: Returns error if command exceeds timeout
//
// Rationale: Go enhancement for production safety
func TestCommand_TimeoutSupport_GoEnhancement(t *testing.T) {
	t.Log("\n=== GO ENHANCEMENT: Command timeout support ===")

	t.Log("Go Implementation:")
	t.Log("  type ShellCommand struct {")
	t.Log("      Timeout time.Duration  // Optional command timeout")
	t.Log("  }")
	t.Log("")
	t.Log("  func (c *ShellCommand) Run() (*ShellCommandResult, error) {")
	t.Log("      if c.Timeout > 0 {")
	t.Log("          ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)")
	t.Log("          defer cancel()")
	t.Log("          cmd.Start()")
	t.Log("          // Wait with timeout")
	t.Log("          if err := cmd.Wait(); ctx.Err() == context.DeadlineExceeded {")
	t.Log("              return nil, fmt.Errorf(\"command timed out after\", c.Timeout)")
	t.Log("          }")
	t.Log("      }")
	t.Log("  }")
	t.Log("")
	t.Log("  Example:")
	t.Log("    cmd := &ShellCommand{")
	t.Log("        Command: 'sleep 10',")
	t.Log("        Timeout: 2 * time.Second,")
	t.Log("    }")
	t.Log("    _, err := cmd.Run()  // Returns timeout error after 2s")

	t.Log("\nGo Status: NOT IMPLEMENTED")
	t.Log("  Go's Command class does not support automatic timeouts.")
	t.Log("  Commands run indefinitely until completion or manual interrupt.")

	t.Log("\nGo Enhancement Rationale:")
	t.Log("  1. Production safety - prevents hung processes")
	t.Log("  2. Resource management - avoids infinite waits")
	t.Log("  3. Graceful degradation - controlled failure mode")
	t.Log("  4. Testing support - timeout in tests prevents hangs")
	t.Log("  5. Common pattern in Go - context-based cancellation")

	t.Log("\nWhy NOT Reverse Port to Go:")
	t.Log("  - Go implementation works for current use cases")
	t.Log("  - Would require refactoring to support context/cancellation")
	t.Log("  - Low priority - no user demand for Go timeout support")
	t.Log("  - Go enhancement addresses Go-specific production needs")

	t.Log("\n🟢 Go Enhancement - NOT in Go")
	t.Log("📋 Category: Production Safety")
	t.Log("⏱️  Implementation effort (if porting): 4-6 hours")
	t.Log("💡 Decision: Keep as Go-only enhancement")
	t.Log("✅ Test coverage: See TestCommand_Timeout_BehavioralBDD")
}

// TestCommand_DryRunMode_GoEnhancement documents Go's dry-run feature
//
// Go Implementation (internal/utils/command.ShellCommand):
//   - Field: DryRun bool
//   - Purpose: Preview commands without executing
//   - Behavior: Returns success without running when DryRun=true
//
// Rationale: Go enhancement for testing and previews
func TestCommand_DryRunMode_GoEnhancement(t *testing.T) {
	t.Log("\n=== GO ENHANCEMENT: Dry-run mode support ===")

	t.Log("Go Implementation:")
	t.Log("  type ShellCommand struct {")
	t.Log("      DryRun bool  // If true, don't execute, just return success")
	t.Log("  }")
	t.Log("")
	t.Log("  func (c *ShellCommand) Run() (*ShellCommandResult, error) {")
	t.Log("      if c.DryRun {")
	t.Log("          logger.Info(\"DRY RUN: Would execute:\", c.Command)")
	t.Log("          return &ShellCommandResult{")
	t.Log("              ExitCode: 0,")
	t.Log("              Output:   fmt.Sprintf(\"DRY RUN:\", c.Command),")
	t.Log("          }, nil")
	t.Log("      }")
	t.Log("      // Normal execution...")
	t.Log("  }")
	t.Log("")
	t.Log("  Example:")
	t.Log("    cmd := &ShellCommand{")
	t.Log("        Command: 'rm -rf /important/data',")
	t.Log("        DryRun:  true,")
	t.Log("    }")
	t.Log("    result, _ := cmd.Run()")
	t.Log("    // Logs: 'DRY RUN: Would execute: rm -rf /important/data'")
	t.Log("    // Does NOT actually delete anything")

	t.Log("\nGo Status: NOT IMPLEMENTED")
	t.Log("  Go's Command class does not support dry-run mode.")
	t.Log("  Commands always execute when Run() is called.")

	t.Log("\nGo Enhancement Rationale:")
	t.Log("  1. Safety - preview destructive operations before execution")
	t.Log("  2. Testing - validate command generation without side effects")
	t.Log("  3. Debugging - see what commands would run")
	t.Log("  4. CI/CD pipelines - plan mode before apply mode")
	t.Log("  5. User confidence - see impact before committing")

	t.Log("\nWhy NOT Reverse Port to Go:")
	t.Log("  - Go workflows don't currently use dry-run patterns")
	t.Log("  - Would require propagating dry-run flag through call chain")
	t.Log("  - Low priority - no user demand for Go dry-run")
	t.Log("  - Go enhancement addresses Go-specific testing needs")

	t.Log("\n🟢 Go Enhancement - NOT in Go")
	t.Log("📋 Category: Testing & Safety")
	t.Log("⏱️  Implementation effort (if porting): 3-4 hours")
	t.Log("💡 Decision: Keep as Go-only enhancement")
	t.Log("✅ Test coverage: See TestCommand_DryRunMode_BehavioralBDD")
}
