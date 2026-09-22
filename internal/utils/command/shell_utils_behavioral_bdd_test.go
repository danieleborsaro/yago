package command

// This file documents behavioral contracts for shell utility functions.
//
// Migration Information:
// - Original File: shell_test.go (350 lines)
// - Original Tests: 11 utility and integration tests
// - Migration Date: October 13, 2025
// - Migrated Tests:
//   1. PlatformDetection - runtime.GOOS based platform detection
//   2. GetShell - Platform-specific shell selection
//   3. EscapePath - Platform-specific path escaping for shells
//   4. Execute - Basic command execution (already extensively covered in shell_behavioral_bdd_test.go)
//   5. DryRun - Dry-run mode (already covered)
//   6. PreviewCommand - Command preview mode (already covered)
//   7. EnvOverride - Environment override (already covered)
//   8. EnvAppend - Environment append (already covered)
//   9. ExecuteScript - Script execution (already covered)
//   10. ExecuteWithTimeout - Timeout handling (already covered)
//   11. ExecuteNonExistentCommand - Error handling (already covered)
//   12. MeasureDuration - Duration measurement (already covered)
//   13. QuickUtilityFunctions - Run/RunSilent helpers (already covered)
//
// Tests Already Covered:
// - Most command execution tests are extensively covered in shell_behavioral_bdd_test.go
// - This file focuses on utility functions (platform detection, shell selection, path escaping)
//   as standalone, self-contained behaviors
//
// Testing Philosophy: Time-Based BDD
// These tests characterize current Go behavior for platform-specific utilities
// that enable cross-platform command execution.

import (
	"runtime"
	"strings"
	"testing"
)

// GoBehavioralContract documents expected Go behavior at a specific point in time.
type GoBehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	TestScenario    string
	Rationale       string
	RegressionRisk  string
}

// TestCommand_PlatformDetection_BehavioralBDD documents platform detection utilities.
//
// BEHAVIORAL CONTRACT:
// Platform detection functions (GetPlatform, IsWindows, IsLinux, IsDarwin) use
// runtime.GOOS to determine the current operating system. These functions provide
// a consistent API for platform-specific behavior without checking runtime.GOOS
// everywhere.
//
// CRITICAL SEMANTICS:
// - runtime.GOOS source: All detection based on Go's runtime.GOOS variable
// - Mutual exclusion: Exactly one IsXXX() function returns true
// - Platform strings: "windows", "linux", "darwin" (lowercase, matching runtime.GOOS)
// - Static detection: Platform determined at runtime, not compile-time
//
// RATIONALE:
// Centralized platform detection provides consistent behavior across the codebase
// and simplifies platform-specific command construction (shell selection, path
// escaping). These are pure Go implementations enabling cross-platform support.
//
// REGRESSION RISK: CRITICAL
// - Wrong platform detection would cause incorrect shell/path handling
// - Non-exclusive platform checks would break platform-specific logic
// - Changing platform strings would break string comparisons throughout codebase
func TestCommand_PlatformDetection_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Platform detection functions return current OS based on runtime.GOOS",
		CurrentImpl:     "GetPlatform() returns runtime.GOOS, IsXXX() compare runtime.GOOS to platform names",
		ExpectedOutcome: "Exactly one platform detected, matches runtime.GOOS, platform string returned",
		TestScenario:    "Call platform functions, verify one and only one returns true, matches runtime.GOOS",
		Rationale:       "Centralized platform detection enables cross-platform command execution",
		RegressionRisk:  "CRITICAL - Wrong detection breaks shell selection and path escaping",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)

	// TEST: GetPlatform returns non-empty string
	platform := GetPlatform()
	if platform == "" {
		t.Fatal("BEHAVIORAL CONTRACT VIOLATION: GetPlatform() must return non-empty platform string")
	}

	// TEST: Exactly one platform detected (mutual exclusion)
	platformCount := 0
	if IsWindows() {
		platformCount++
	}
	if IsLinux() {
		platformCount++
	}
	if IsDarwin() {
		platformCount++
	}

	if platformCount == 0 {
		t.Fatal("BEHAVIORAL CONTRACT VIOLATION: At least one platform must be detected")
	}

	if platformCount > 1 {
		t.Fatal("BEHAVIORAL CONTRACT VIOLATION: Multiple platforms detected - must be mutually exclusive")
	}

	// TEST: Platform detection matches runtime.GOOS
	switch runtime.GOOS {
	case "windows":
		if !IsWindows() {
			t.Error("BEHAVIORAL CONTRACT VIOLATION: IsWindows() must return true when runtime.GOOS='windows'")
		}
		if GetPlatform() != "windows" {
			t.Errorf("BEHAVIORAL CONTRACT VIOLATION: GetPlatform() must return 'windows', got '%s'", GetPlatform())
		}

	case "linux":
		if !IsLinux() {
			t.Error("BEHAVIORAL CONTRACT VIOLATION: IsLinux() must return true when runtime.GOOS='linux'")
		}
		if GetPlatform() != "linux" {
			t.Errorf("BEHAVIORAL CONTRACT VIOLATION: GetPlatform() must return 'linux', got '%s'", GetPlatform())
		}

	case "darwin":
		if !IsDarwin() {
			t.Error("BEHAVIORAL CONTRACT VIOLATION: IsDarwin() must return true when runtime.GOOS='darwin'")
		}
		if GetPlatform() != "darwin" {
			t.Errorf("BEHAVIORAL CONTRACT VIOLATION: GetPlatform() must return 'darwin', got '%s'", GetPlatform())
		}

	default:
		t.Logf("Note: Running on unsupported platform '%s' - platform detection may not work correctly", runtime.GOOS)
	}

	t.Logf("✓ Behavior verified: Platform detection matches runtime.GOOS, mutual exclusion enforced")
}

// TestCommand_GetShell_BehavioralBDD documents platform-specific shell selection.
//
// BEHAVIORAL CONTRACT:
// GetShell() returns the appropriate shell executable for the current platform:
// - Windows: "powershell" (PowerShell for modern Windows scripting)
// - Linux/Darwin: "/bin/bash" (Bash for Unix-like systems)
//
// This enables consistent script execution across platforms.
//
// CRITICAL SEMANTICS:
// - Platform-specific: Different shell per OS
// - Windows: PowerShell (not cmd.exe) for better scripting capabilities
// - Unix: Bash with absolute path (/bin/bash) for reliability
// - No user configuration: Always returns platform default (not $SHELL)
//
// RATIONALE:
// Consistent shell selection ensures scripts work across platforms without
// user configuration. PowerShell on Windows provides features closer to Bash
// (pipes, variables, etc.). Absolute path /bin/bash ensures availability.
//
// REGRESSION RISK: HIGH
// - Wrong shell would break scripts expecting specific syntax/features
// - Relative path (bash vs /bin/bash) could find wrong binary in PATH
// - Using cmd.exe on Windows would break scripts using PowerShell features
func TestCommand_GetShell_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "GetShell returns platform-appropriate shell executable",
		CurrentImpl:     "Returns 'powershell' on Windows, '/bin/bash' on Unix-like systems",
		ExpectedOutcome: "Platform-specific shell path returned, suitable for script execution",
		TestScenario:    "Call GetShell(), verify matches platform expectations",
		Rationale:       "Consistent shell selection enables cross-platform scripting",
		RegressionRisk:  "HIGH - Wrong shell breaks scripts, missing path causes exec failures",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)

	shell := GetShell()

	// TEST: Non-empty shell path
	if shell == "" {
		t.Fatal("BEHAVIORAL CONTRACT VIOLATION: GetShell() must return non-empty shell path")
	}

	// TEST: Platform-specific shell selection
	switch runtime.GOOS {
	case "windows":
		if shell != "powershell" {
			t.Errorf("BEHAVIORAL CONTRACT VIOLATION: Windows must use 'powershell', got '%s'", shell)
		}
		t.Logf("✓ Windows shell: %s (PowerShell for modern scripting)", shell)

	case "linux", "darwin":
		if shell != "/bin/bash" {
			t.Errorf("BEHAVIORAL CONTRACT VIOLATION: Unix must use '/bin/bash' (absolute path), got '%s'", shell)
		}
		t.Logf("✓ Unix shell: %s (Bash with absolute path)", shell)

	default:
		t.Logf("Warning: Unsupported platform '%s', shell selection may not work correctly", runtime.GOOS)
	}

	t.Logf("✓ Behavior verified: Platform-appropriate shell selected")
}

// TestCommand_EscapePath_BehavioralBDD documents platform-specific path escaping.
//
// BEHAVIORAL CONTRACT:
// EscapePath() escapes special characters in file paths for shell safety:
// - Windows: Backtick escaping (` space`, `(`, `)`)
// - Unix: Backslash escaping (\\ space\\, \\(, \\))
//
// This prevents shell interpretation of spaces, parentheses, and other special
// characters in paths.
//
// CRITICAL SEMANTICS:
// - Platform-specific: Different escaping per OS
// - Windows backticks: PowerShell escape character
// - Unix backslashes: Bash escape character
// - Special chars escaped: spaces, parentheses (common in paths)
// - Preserves path: Original path structure maintained, only escaping added
//
// RATIONALE:
// File paths with spaces or special characters must be escaped for shell commands
// to interpret them correctly. Without escaping, "my dir/file" becomes two
// arguments ("my", "dir/file"). Platform-specific escaping ensures compatibility.
//
// REGRESSION RISK: HIGH
// - Missing escaping causes commands to fail on paths with spaces
// - Wrong escaping (bash escapes on Windows) breaks PowerShell execution
// - Over-escaping makes paths invalid
// - Under-escaping leaves vulnerabilities to shell injection
func TestCommand_EscapePath_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "EscapePath escapes special characters using platform-specific syntax",
		CurrentImpl:     "Windows uses backtick escaping, Unix uses backslash escaping",
		ExpectedOutcome: "Spaces and parentheses escaped, path safe for shell execution",
		TestScenario:    "Escape paths with spaces/parentheses, verify platform-appropriate escaping",
		Rationale:       "Path escaping prevents shell misinterpretation and enables safe command construction",
		RegressionRisk:  "HIGH - Wrong escaping breaks commands, missing escaping causes security issues",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)

	// TEST: Path with spaces
	t.Run("spaces", func(t *testing.T) {
		input := "/path/to/my directory"
		result := EscapePath(input)

		if result == "" {
			t.Fatal("BEHAVIORAL CONTRACT VIOLATION: EscapePath must return non-empty result")
		}

		if result == input {
			t.Error("BEHAVIORAL CONTRACT VIOLATION: Path with spaces must be escaped")
		}

		if runtime.GOOS == "windows" {
			if !strings.Contains(result, "` ") {
				t.Error("BEHAVIORAL CONTRACT VIOLATION: Windows must use backtick-escaped spaces")
			}
			t.Logf("✓ Windows escaping: '%s' → '%s'", input, result)
		} else {
			if !strings.Contains(result, "\\ ") {
				t.Error("BEHAVIORAL CONTRACT VIOLATION: Unix must use backslash-escaped spaces")
			}
			t.Logf("✓ Unix escaping: '%s' → '%s'", input, result)
		}
	})

	// TEST: Path with parentheses
	t.Run("parentheses", func(t *testing.T) {
		input := "/path/to/dir(1)"
		result := EscapePath(input)

		if result == input {
			t.Error("BEHAVIORAL CONTRACT VIOLATION: Path with parentheses must be escaped")
		}

		if runtime.GOOS == "windows" {
			if !strings.Contains(result, "`(") || !strings.Contains(result, "`)") {
				t.Error("BEHAVIORAL CONTRACT VIOLATION: Windows must escape parentheses with backticks")
			}
			t.Logf("✓ Windows escaping: '%s' → '%s'", input, result)
		} else {
			if !strings.Contains(result, "\\(") || !strings.Contains(result, "\\)") {
				t.Error("BEHAVIORAL CONTRACT VIOLATION: Unix must escape parentheses with backslashes")
			}
			t.Logf("✓ Unix escaping: '%s' → '%s'", input, result)
		}
	})

	// TEST: Path with both spaces and parentheses
	t.Run("complex_path", func(t *testing.T) {
		input := "/path/to/my dir (files)"
		result := EscapePath(input)

		if result == input {
			t.Error("BEHAVIORAL CONTRACT VIOLATION: Complex path must be escaped")
		}

		// Verify both space and parenthesis escaping present
		if runtime.GOOS == "windows" {
			hasBacktickSpace := strings.Contains(result, "` ")
			hasBacktickParen := strings.Contains(result, "`(") || strings.Contains(result, "`)")

			if !hasBacktickSpace || !hasBacktickParen {
				t.Error("BEHAVIORAL CONTRACT VIOLATION: Windows must escape all special characters")
			}
			t.Logf("✓ Windows complex escaping: '%s' → '%s'", input, result)
		} else {
			hasBackslashSpace := strings.Contains(result, "\\ ")
			hasBackslashParen := strings.Contains(result, "\\(") || strings.Contains(result, "\\)")

			if !hasBackslashSpace || !hasBackslashParen {
				t.Error("BEHAVIORAL CONTRACT VIOLATION: Unix must escape all special characters")
			}
			t.Logf("✓ Unix complex escaping: '%s' → '%s'", input, result)
		}
	})

	t.Logf("✓ Behavior verified: Platform-specific path escaping works correctly")
}

// NOTE: The following test categories are already extensively covered in
// shell_behavioral_bdd_test.go and do not need to be duplicated here:
//
// - TestCommand_ExecuteInWorkingDirectory_BehavioralBDD (working directory)
// - TestCommand_AppendEnvironmentVariables_BehavioralBDD (Env append)
// - TestCommand_OverrideEnvironmentVariables_BehavioralBDD (EnvOverride)
// - TestCommand_ExitCodeHandling_BehavioralBDD (exit codes, non-existent commands)
// - TestCommand_OutputCapture_BehavioralBDD (stdout/stderr capture)
// - TestCommand_StderrRedirect_BehavioralBDD (stderr handling)
// - TestCommand_OutputModes_BehavioralBDD (IsShowOutput, silent execution)
// - TestCommand_Timeout_BehavioralBDD (timeout handling)
// - TestCommand_DryRun_BehavioralBDD (dry-run mode)
// - TestCommand_DurationMeasurement_BehavioralBDD (IsMeasureDuration, timing)
//
// These comprehensive behavioral tests cover:
// - Basic Execute() functionality
// - ExecuteScript() script execution
// - Quick utility functions (Run, RunSilent)
// - All configuration options (Config struct fields)
// - Error handling and edge cases
//
// Total coverage: 18 behavioral contracts in shell_behavioral_bdd_test.go (1095 lines)
// covering all command execution scenarios.
//
// This file adds 3 additional behavioral contracts for pure Go utility functions:
// - PlatformDetection (GetPlatform, IsWindows, IsLinux, IsDarwin)
// - GetShell (platform-specific shell selection)
// - EscapePath (platform-specific path escaping)
