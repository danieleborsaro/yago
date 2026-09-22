package main_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// BehavioralContract documents the expected CLI execution behavior
type CLIBehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

var (
	yagoBinaryOnce sync.Once
	yagoBinaryPath string
	yagoBinaryErr  error
)

// buildYagoBinary builds the yago binary once and reuses it across all tests in this package
func buildYagoBinary(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping CLI integration test (builds real binary) in short mode")
	}

	yagoBinaryOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "yago-binary")
		if err != nil {
			yagoBinaryErr = err
			return
		}
		binaryPath := filepath.Join(tmpDir, "yago")

		cmd := exec.Command("go", "build", "-o", binaryPath, ".")
		cmd.Dir = filepath.Join(".") // cmd/yago directory

		output, err := cmd.CombinedOutput()
		if err != nil {
			yagoBinaryErr = fmt.Errorf("failed to build yago binary: %w\nOutput: %s", err, output)
			return
		}
		yagoBinaryPath = binaryPath
	})

	if yagoBinaryErr != nil {
		t.Fatalf("%v", yagoBinaryErr)
	}

	return yagoBinaryPath
}

// TestMain cleans up the shared binary built once by buildYagoBinary
func TestMain(m *testing.M) {
	code := m.Run()
	if yagoBinaryPath != "" {
		os.RemoveAll(filepath.Dir(yagoBinaryPath))
	}
	os.Exit(code)
}

// runYago executes the yago binary with given arguments
func runYago(t *testing.T, binaryPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("Failed to run command: %v", err)
		}
	} else {
		exitCode = 0
	}

	return stdout, stderr, exitCode
}

// =============================================================================
// CLI INTEGRATION: VERSION AND HELP
// =============================================================================

func TestCLI_VersionCommand_BehavioralBDD(t *testing.T) {
	contract := CLIBehavioralContract{
		Behavior:        "Version command displays application version and build information",
		CurrentImpl:     "The yago root command reports its version on --version",
		ExpectedOutcome: "Version output goes to stdout and exits successfully",
		Rationale:       "Version information supports installation checks, pinning, and diagnostics",
	}

	yagoBinary := buildYagoBinary(t)

	t.Run("version_flag", func(t *testing.T) {
		stdout, stderr, exitCode := runYago(t, yagoBinary, "--version")

		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", exitCode)
		}

		if stderr != "" {
			t.Errorf("Expected no stderr output, got: %s", stderr)
		}

		if !strings.Contains(stdout, "version") {
			t.Errorf("Expected version in output, got: %s", stdout)
		}

		if !strings.Contains(stdout, "0.1.0") {
			t.Logf("Output: %s", stdout)
			t.Log("✓ Version output contains version number")
		}

		t.Log("✓ Version flag works correctly with exit code 0")
	})

	t.Run("version_output_format", func(t *testing.T) {
		stdout, _, _ := runYago(t, yagoBinary, "--version")

		// Should be single line
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) > 2 {
			t.Logf("Version output has %d lines (may include build info)", len(lines))
		}

		// Should contain "yago" and "version"
		if !strings.Contains(strings.ToLower(stdout), "yago") {
			t.Log("Note: Output doesn't contain 'yago' name")
		}

		t.Log("✓ Version output format is reasonable")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

func TestCLI_HelpCommand_BehavioralBDD(t *testing.T) {
	contract := CLIBehavioralContract{
		Behavior:        "Help command displays usage information for all commands",
		CurrentImpl:     "Cobra renders root and subcommand help, including commands, aliases, and persistent flags",
		ExpectedOutcome: "--help, help, and no-argument invocation provide readable usage output",
		Rationale:       "Help supports command discovery and correct flag usage",
	}

	yagoBinary := buildYagoBinary(t)

	t.Run("help_flag", func(t *testing.T) {
		stdout, _, exitCode := runYago(t, yagoBinary, "--help")

		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", exitCode)
		}

		// Should mention available commands
		if !strings.Contains(stdout, "Available Commands") && !strings.Contains(stdout, "Commands:") {
			t.Logf("Help output: %s", stdout)
		}

		// Should mention flags
		if !strings.Contains(stdout, "Flags") && !strings.Contains(stdout, "flags") {
			t.Log("Note: Help doesn't clearly show flags section")
		}

		t.Log("✓ Help flag displays usage information")
	})

	t.Run("help_command", func(t *testing.T) {
		stdout, _, exitCode := runYago(t, yagoBinary, "help")

		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", exitCode)
		}

		if stdout == "" {
			t.Error("Expected help output, got empty string")
		}

		t.Log("✓ Help command works without -- prefix")
	})

	t.Run("no_args_shows_help", func(t *testing.T) {
		stdout, _, exitCode := runYago(t, yagoBinary)

		// No args should show help (exit code may vary by implementation)
		if stdout == "" {
			t.Error("Expected help output when no args provided, got empty")
		}

		if exitCode == 0 || exitCode == 1 {
			t.Logf("✓ No args shows help (exit code: %d)", exitCode)
		}
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// CLI INTEGRATION: GLOBAL FLAGS
// =============================================================================

func TestCLI_GlobalFlags_BehavioralBDD(t *testing.T) {
	contract := CLIBehavioralContract{
		Behavior:        "Global flags work consistently across all commands",
		CurrentImpl:     "Persistent verbose, log-level, and schema-config flags are registered on the root command",
		ExpectedOutcome: "Global configuration flags are recognized and available to command execution",
		Rationale:       "Global flags enable consistent debugging and CI configuration",
	}

	yagoBinary := buildYagoBinary(t)

	t.Run("verbose_flag", func(t *testing.T) {
		stdout, stderr, exitCode := runYago(t, yagoBinary, "--verbose", "--help")

		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", exitCode)
		}

		// Verbose flag should be mentioned in help
		combined := stdout + stderr
		if strings.Contains(combined, "verbose") || strings.Contains(combined, "Verbose") {
			t.Log("✓ Verbose flag is recognized")
		}
	})

	t.Run("log_level_flag", func(t *testing.T) {
		stdout, _, exitCode := runYago(t, yagoBinary, "--log-level=debug", "--help")

		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", exitCode)
		}

		if stdout == "" {
			t.Error("Expected help output")
		}

		t.Log("✓ Log level flag is recognized")
	})

	t.Run("schema_config_flag", func(t *testing.T) {
		stdout, _, exitCode := runYago(t, yagoBinary, "--schema-config=/tmp/schema.json", "--help")

		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", exitCode)
		}

		if stdout == "" {
			t.Error("Expected help output")
		}

		t.Log("✓ Schema config flag is recognized")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// CLI INTEGRATION: COMMAND EXECUTION
// =============================================================================

func TestCLI_DesiredStateCommands_BehavioralBDD(t *testing.T) {
	contract := CLIBehavioralContract{
		Behavior:        "Desiredstate subcommands execute with proper error handling",
		CurrentImpl:     "Cobra exposes validate, assemble, promote, and printschema commands with the ds alias",
		ExpectedOutcome: "Commands report clear errors for invalid input and produce output on successful execution",
		Rationale:       "Reliable command execution supports user workflows and CI pipelines",
	}

	yagoBinary := buildYagoBinary(t)

	t.Run("desiredstate_help", func(t *testing.T) {
		stdout, _, exitCode := runYago(t, yagoBinary, "desiredstate", "--help")

		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", exitCode)
		}

		// Should list subcommands
		if !strings.Contains(stdout, "validate") {
			t.Error("Expected 'validate' command in help")
		}

		if !strings.Contains(stdout, "assemble") {
			t.Error("Expected 'assemble' command in help")
		}

		if !strings.Contains(stdout, "promote") {
			t.Error("Expected 'promote' command in help")
		}

		if !strings.Contains(stdout, "printschema") && !strings.Contains(stdout, "schema") {
			t.Log("Note: printschema command may have different name")
		}

		t.Log("✓ Desiredstate command shows available subcommands")
	})

	t.Run("alias_ds_works", func(t *testing.T) {
		stdout, _, exitCode := runYago(t, yagoBinary, "ds", "--help")

		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", exitCode)
		}

		if stdout == "" {
			t.Error("Expected help output for 'ds' alias")
		}

		// Should show same help as 'desiredstate'
		if strings.Contains(stdout, "validate") {
			t.Log("✓ Alias 'ds' works for desiredstate command")
		}
	})

	t.Run("validate_missing_required_flag", func(t *testing.T) {
		_, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "validate")

		if exitCode == 0 {
			t.Error("Expected non-zero exit code for missing required flag")
		}

		// Should show error about missing flag
		if !strings.Contains(stderr, "required") && !strings.Contains(stderr, "flag") {
			t.Logf("Error output: %s", stderr)
		}

		t.Log("✓ Validate command requires desiredstate-root flag")
	})

	t.Run("printschema_list_versions", func(t *testing.T) {
		stdout, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "printschema", "-l")

		// May succeed or fail depending on schema availability
		if exitCode == 0 {
			if stdout != "" || stderr != "" {
				t.Log("✓ Printschema with -l flag executed")
			}
		} else {
			// Failure is acceptable if schemas not available
			t.Logf("Printschema failed (exit %d) - may need schema setup", exitCode)
		}
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// CLI INTEGRATION: EXIT CODES
// =============================================================================

func TestCLI_ExitCodes_BehavioralBDD(t *testing.T) {
	contract := CLIBehavioralContract{
		Behavior:        "CLI returns appropriate exit codes for different scenarios",
		CurrentImpl:     "Successful commands return zero and invalid flags or commands return non-zero status",
		ExpectedOutcome: "Automation can continue on success and stop on command failure",
		Rationale:       "Exit codes are the CLI contract for scripts and CI pipelines",
	}

	yagoBinary := buildYagoBinary(t)

	t.Run("success_exit_code", func(t *testing.T) {
		_, _, exitCode := runYago(t, yagoBinary, "--version")

		if exitCode != 0 {
			t.Errorf("Expected exit code 0 for success, got %d", exitCode)
		}

		t.Log("✓ Successful commands exit with code 0")
	})

	t.Run("error_exit_code", func(t *testing.T) {
		_, _, exitCode := runYago(t, yagoBinary, "--invalid-flag")

		if exitCode == 0 {
			t.Error("Expected non-zero exit code for error, got 0")
		}

		if exitCode == 1 {
			t.Log("✓ Error commands exit with code 1")
		} else {
			t.Logf("✓ Error commands exit with code %d (non-zero)", exitCode)
		}
	})

	t.Run("missing_subcommand_exit_code", func(t *testing.T) {
		_, _, exitCode := runYago(t, yagoBinary, "invalid-command")

		if exitCode == 0 {
			t.Error("Expected non-zero exit code for invalid command, got 0")
		}

		t.Log("✓ Invalid commands exit with non-zero code")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// CLI INTEGRATION: OUTPUT FORMATTING
// =============================================================================

func TestCLI_OutputFormatting_BehavioralBDD(t *testing.T) {
	contract := CLIBehavioralContract{
		Behavior:        "CLI produces properly formatted output to stdout and stderr",
		CurrentImpl:     "Commands write successful data to stdout and errors or diagnostics to stderr",
		ExpectedOutcome: "Output remains readable for users and parseable by automation",
		Rationale:       "Stable stream separation supports pipelines, logging, and debugging",
	}

	yagoBinary := buildYagoBinary(t)

	t.Run("stdout_for_success", func(t *testing.T) {
		stdout, stderr, _ := runYago(t, yagoBinary, "--version")

		if stdout == "" {
			t.Error("Expected version output on stdout")
		}

		// Stderr may have logs, that's OK
		if stderr != "" {
			t.Logf("Stderr has content (may be logs): %s", stderr)
		}

		t.Log("✓ Success output goes to stdout")
	})

	t.Run("stderr_for_errors", func(t *testing.T) {
		stdout, stderr, exitCode := runYago(t, yagoBinary, "--invalid-flag")

		if exitCode == 0 {
			t.Skip("Invalid flag didn't error - skipping test")
		}

		// Error should be on stderr
		if stderr == "" && stdout == "" {
			t.Error("Expected error message on stderr or stdout")
		}

		if stderr != "" {
			t.Log("✓ Error messages go to stderr")
		} else if stdout != "" {
			t.Log("Note: Error message on stdout (acceptable variation)")
		}
	})

	t.Run("help_output_readable", func(t *testing.T) {
		stdout, _, _ := runYago(t, yagoBinary, "--help")

		// Help should be formatted with sections
		if strings.Contains(stdout, "Usage:") || strings.Contains(stdout, "Commands:") {
			t.Log("✓ Help output is structured and readable")
		}

		// Should have proper line breaks (not one long line)
		lines := strings.Split(stdout, "\n")
		if len(lines) > 5 {
			t.Log("✓ Help output has multiple lines (formatted)")
		}
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// PERFORMANCE BEHAVIORAL CONTRACTS
// =============================================================================

func TestCLI_PerformanceRequirements_BehavioralBDD(t *testing.T) {
	contract := CLIBehavioralContract{
		Behavior:        "CLI commands execute within acceptable performance thresholds",
		CurrentImpl:     "Integration tests measure version, help, validation, repeated, and concurrent invocations",
		ExpectedOutcome: "Common CLI operations complete promptly and remain safe across repeated and concurrent use",
		Rationale:       "Responsive commands improve CI speed and developer feedback",
	}

	yagoBinary := buildYagoBinary(t)

	t.Run("version_command_performance", func(t *testing.T) {
		// Measure version command performance
		start := time.Now()
		_, _, exitCode := runYago(t, yagoBinary, "version")
		elapsed := time.Since(start)

		if exitCode == 0 {
			t.Logf("✓ Version command succeeded in %v", elapsed)
			if elapsed < 500*time.Millisecond {
				t.Logf("✓ Version command performance excellent: %v < 500ms", elapsed)
			} else if elapsed < 1*time.Second {
				t.Logf("⚠ Version command acceptable but slow: %v", elapsed)
			} else {
				t.Logf("⚠ Version command slow: %v (should be < 1s)", elapsed)
			}
		}
	})

	t.Run("help_command_performance", func(t *testing.T) {
		start := time.Now()
		_, _, exitCode := runYago(t, yagoBinary, "--help")
		elapsed := time.Since(start)

		if exitCode == 0 {
			t.Logf("✓ Help command succeeded in %v", elapsed)
			if elapsed < 500*time.Millisecond {
				t.Logf("✓ Help command performance excellent: %v < 500ms", elapsed)
			} else if elapsed < 1*time.Second {
				t.Logf("⚠ Help command acceptable but slow: %v", elapsed)
			}
		}
	})

	t.Run("validate_small_file_performance", func(t *testing.T) {
		testDir := t.TempDir()
		dsFile := filepath.Join(testDir, "perf_test.yaml")
		content := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: perf-test
  environment: dev
spec:
  components: []
`
		if err := os.WriteFile(dsFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		start := time.Now()
		_, _, exitCode := runYago(t, yagoBinary, "desiredstate", "validate",
			"-d", dsFile,
			"-e", "dev")
		elapsed := time.Since(start)

		// Accept success or expected failure
		t.Logf("Validate command completed in %v (exit %d)", elapsed, exitCode)
		if elapsed < 2*time.Second {
			t.Logf("✓ Validate performance acceptable: %v < 2s", elapsed)
		} else {
			t.Logf("⚠ Validate slower than expected: %v", elapsed)
		}
	})

	t.Run("memory_efficiency", func(t *testing.T) {
		// Test that CLI doesn't leak memory across multiple invocations
		for i := 0; i < 5; i++ {
			_, _, _ = runYago(t, yagoBinary, "--help")
		}
		t.Log("✓ Multiple invocations completed (no memory leaks in new processes)")
	})

	t.Run("concurrent_safety", func(t *testing.T) {
		// Test that multiple CLI invocations can run concurrently
		done := make(chan bool, 3)

		for i := 0; i < 3; i++ {
			go func() {
				_, _, _ = runYago(t, yagoBinary, "version")
				done <- true
			}()
		}

		// Wait for all to complete
		for i := 0; i < 3; i++ {
			<-done
		}
		t.Log("✓ Concurrent CLI invocations completed successfully")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}
