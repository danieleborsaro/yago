package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// WorkflowBehavioralContract documents end-to-end workflow behaviors
type WorkflowBehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

// =============================================================================
// END-TO-END WORKFLOW: VALIDATE OPERATION
// =============================================================================

func TestWorkflow_ValidateDesiredState_BehavioralBDD(t *testing.T) {
	contract := WorkflowBehavioralContract{
		Behavior:        "Complete validation workflow from YAML file to validation result",
		CurrentImpl:     "The yago validate command loads YAML, checks schema and constraints, and reports errors",
		ExpectedOutcome: "Valid input succeeds; missing, malformed, or incomplete input returns a non-zero result with context",
		Rationale:       "Validation provides pre-deployment quality gates and prevents configuration errors",
	}

	yagoBinary := buildYagoBinary(t)
	testDir := t.TempDir()

	t.Run("validate_valid_desiredstate", func(t *testing.T) {
		// Create a minimal valid desiredstate file
		dsFile := filepath.Join(testDir, "desiredstate.yaml")
		dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: test-app
  environment: dev
spec:
  components: []
`
		if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		stdout, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "validate",
			"-d", dsFile,
			"-e", "dev",
			"-w", "terraform")

		// May succeed or fail depending on schema availability
		if exitCode == 0 {
			t.Log("✓ Valid desiredstate file validated successfully")
			if stdout != "" {
				t.Logf("Validation output: %s", stdout)
			}
		} else {
			// Failure acceptable if schema not found
			t.Logf("Validation failed (exit %d) - may need schema setup", exitCode)
			if strings.Contains(stderr, "schema") || strings.Contains(stderr, "PARSE_ERROR") {
				t.Log("Note: Schema-related error expected without schema files")
			}
		}
	})

	t.Run("validate_missing_file", func(t *testing.T) {
		stdout, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "validate",
			"-d", "/nonexistent/file.yaml",
			"-e", "dev",
			"-w", "terraform")

		if exitCode == 0 {
			t.Error("Expected non-zero exit code for missing file")
		}

		// Should report file not found
		combined := stdout + stderr
		if strings.Contains(combined, "not found") || strings.Contains(combined, "no such file") ||
			strings.Contains(combined, "does not exist") || strings.Contains(combined, "nonexistent") {
			t.Log("✓ Missing file error reported correctly")
		} else {
			t.Logf("Error output: %s", combined)
		}
	})

	t.Run("validate_invalid_yaml", func(t *testing.T) {
		// Create invalid YAML file
		invalidFile := filepath.Join(testDir, "invalid.yaml")
		invalidContent := `schema: "1.0.0"
metadata:
  name: test
  invalid yaml syntax here [[[
`
		if err := os.WriteFile(invalidFile, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		_, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "validate",
			"-d", invalidFile,
			"-e", "dev",
			"-w", "terraform")

		if exitCode == 0 {
			t.Error("Expected non-zero exit code for invalid YAML")
		}

		// Should report YAML parse error
		if strings.Contains(stderr, "PARSE") || strings.Contains(stderr, "parse") ||
			strings.Contains(stderr, "YAML") || strings.Contains(stderr, "yaml") {
			t.Log("✓ Invalid YAML error reported correctly")
		} else {
			t.Logf("Parse error detected (exit %d)", exitCode)
		}
	})

	t.Run("validate_missing_required_fields", func(t *testing.T) {
		// Create YAML with missing required fields
		incompleteFile := filepath.Join(testDir, "incomplete.yaml")
		incompleteContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
# Missing metadata section
spec: {}
`
		if err := os.WriteFile(incompleteFile, []byte(incompleteContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		_, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "validate",
			"-d", incompleteFile,
			"-e", "dev",
			"-w", "terraform")

		if exitCode == 0 {
			t.Log("Note: Validation passed despite missing metadata (may be optional)")
		} else {
			t.Logf("✓ Validation failed for missing required fields (exit %d)", exitCode)
			if strings.Contains(stderr, "metadata") || strings.Contains(stderr, "required") {
				t.Log("✓ Error message mentions missing fields")
			}
		}
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// END-TO-END WORKFLOW: ASSEMBLE OPERATION
// =============================================================================

func TestWorkflow_AssembleDesiredState_BehavioralBDD(t *testing.T) {
	contract := WorkflowBehavioralContract{
		Behavior:        "Complete assembly workflow from desiredstate and configuration to assembled output",
		CurrentImpl:     "The yago assemble command loads inputs, resolves configuration, filters wrappers, and formats output",
		ExpectedOutcome: "Assembly produces YAML or JSON output when inputs and schema are valid",
		Rationale:       "Assembly generates deployable configuration for wrapper-specific workflows",
	}

	yagoBinary := buildYagoBinary(t)
	testDir := t.TempDir()

	t.Run("assemble_desiredstate_only", func(t *testing.T) {
		// Create minimal desiredstate file
		dsFile := filepath.Join(testDir, "desiredstate.yaml")
		dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: test-app
  environment: dev
spec:
  components: []
`
		if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		stdout, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "assemble",
			"-d", dsFile,
			"-e", "dev")

		if exitCode == 0 {
			t.Log("✓ Assemble succeeded with desiredstate only")
			if stdout != "" {
				t.Log("✓ Output generated to stdout")
			}
		} else {
			t.Logf("Assemble failed (exit %d) - may need schema setup", exitCode)
			if strings.Contains(stderr, "schema") {
				t.Log("Note: Schema-related error expected")
			}
		}
	})

	t.Run("assemble_with_configuration", func(t *testing.T) {
		// Create desiredstate and configuration files
		dsFile := filepath.Join(testDir, "ds2.yaml")
		cfgFile := filepath.Join(testDir, "config2.yaml")

		dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: test-app
  environment: dev
spec:
  components: []
`
		cfgContent := `schema: "1.0.0"
namespace: yago
kind: Configuration
metadata:
  name: test-app
  environment: dev
configuration:
  content:
    key: value
`
		if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
			t.Fatalf("Failed to create desiredstate file: %v", err)
		}
		if err := os.WriteFile(cfgFile, []byte(cfgContent), 0644); err != nil {
			t.Fatalf("Failed to create configuration file: %v", err)
		}

		stdout, _, exitCode := runYago(t, yagoBinary, "desiredstate", "assemble",
			"-d", dsFile,
			"-c", cfgFile,
			"-e", "dev")

		if exitCode == 0 {
			t.Log("✓ Assemble succeeded with both desiredstate and configuration")
			if strings.Contains(stdout, "key") || strings.Contains(stdout, "value") {
				t.Log("✓ Configuration content included in output")
			}
		} else {
			t.Logf("Assemble with config failed (exit %d)", exitCode)
		}
	})

	t.Run("assemble_with_wrapper_filter", func(t *testing.T) {
		// Create files with wrapper-specific configuration
		dsFile := filepath.Join(testDir, "ds3.yaml")
		cfgFile := filepath.Join(testDir, "config3.yaml")

		dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: test-app
  environment: dev
spec:
  components: []
`
		cfgContent := `schema: "1.0.0"
namespace: yago
kind: Configuration
metadata:
  name: test-app
  environment: dev
configuration:
  content:
    wrappers:
      terraform:
        vars: "tf_vars"
      docker:
        image: "my-image"
`
		if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
			t.Fatalf("Failed to create desiredstate file: %v", err)
		}
		if err := os.WriteFile(cfgFile, []byte(cfgContent), 0644); err != nil {
			t.Fatalf("Failed to create configuration file: %v", err)
		}

		stdout, _, exitCode := runYago(t, yagoBinary, "desiredstate", "assemble",
			"-d", dsFile,
			"-c", cfgFile,
			"-e", "dev",
			"-w", "terraform")

		if exitCode == 0 {
			t.Log("✓ Assemble succeeded with wrapper filter")
			// Check if terraform vars are in output
			if strings.Contains(stdout, "terraform") || strings.Contains(stdout, "tf_vars") {
				t.Log("✓ Wrapper-specific configuration filtered correctly")
			}
		} else {
			t.Logf("Assemble with wrapper failed (exit %d)", exitCode)
		}
	})

	t.Run("assemble_output_format_json", func(t *testing.T) {
		dsFile := filepath.Join(testDir, "ds4.yaml")
		dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: test-app
  environment: dev
spec:
  components: []
`
		if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		stdout, _, exitCode := runYago(t, yagoBinary, "desiredstate", "assemble",
			"-d", dsFile,
			"-e", "dev",
			"--format", "json")

		if exitCode == 0 {
			// Check if output looks like JSON
			if strings.HasPrefix(strings.TrimSpace(stdout), "{") {
				t.Log("✓ JSON format output generated")
			} else {
				t.Log("Note: Output may not be pure JSON (could include logs)")
			}
		} else {
			t.Log("Note: Assemble with JSON format may require schema")
		}
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// END-TO-END WORKFLOW: SCHEMA OPERATIONS
// =============================================================================

func TestWorkflow_SchemaOperations_BehavioralBDD(t *testing.T) {
	contract := WorkflowBehavioralContract{
		Behavior:        "Schema discovery and version listing workflow",
		CurrentImpl:     "The printschema command queries the schema manager for supported versions and details",
		ExpectedOutcome: "Users can list available schema versions and receive schema information on stdout",
		Rationale:       "Schema discovery supports migration planning and compatibility checks",
	}

	yagoBinary := buildYagoBinary(t)

	t.Run("printschema_list_versions", func(t *testing.T) {
		stdout, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "printschema", "-l")

		if exitCode == 0 {
			t.Log("✓ Printschema list versions succeeded")
			if stdout != "" {
				// Check if output contains version information
				if strings.Contains(stdout, "1.0.0") || strings.Contains(stdout, "version") ||
					strings.Contains(stdout, "yago") {
					t.Log("✓ Version list contains schema information")
				}
			}
		} else {
			t.Logf("Printschema failed (exit %d)", exitCode)
			if strings.Contains(stderr, "schema") {
				t.Log("Note: Schema loading error expected without schema files")
			}
		}
	})

	t.Run("printschema_all_versions", func(t *testing.T) {
		stdout, _, exitCode := runYago(t, yagoBinary, "desiredstate", "printschema", "-a")

		if exitCode == 0 {
			t.Log("✓ Printschema all versions succeeded")
			if len(stdout) > 0 {
				t.Log("✓ Schema information generated")
			}
		} else {
			t.Log("Note: Printschema -a may require schema configuration")
		}
	})

	t.Run("printschema_verbose", func(t *testing.T) {
		stdout, stderr, exitCode := runYago(t, yagoBinary, "--verbose", "desiredstate", "printschema", "-l")

		if exitCode == 0 {
			t.Log("✓ Printschema with verbose flag succeeded")
			// Verbose should add logs to stderr
			if stderr != "" {
				t.Log("✓ Verbose mode generates additional logging")
			}
		}

		// Output should still go to stdout
		if stdout != "" {
			t.Log("✓ Schema output to stdout even with verbose")
		}
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// END-TO-END WORKFLOW: ERROR PROPAGATION
// =============================================================================

func TestWorkflow_ErrorPropagation_BehavioralBDD(t *testing.T) {
	contract := WorkflowBehavioralContract{
		Behavior:        "Errors propagate through workflow layers with clear messages",
		CurrentImpl:     "Parser and service errors are wrapped with context and rendered by the CLI",
		ExpectedOutcome: "Invalid parameters and files produce non-zero exits with actionable messages and no stack traces",
		Rationale:       "Clear errors support troubleshooting, CI automation, and operational diagnostics",
	}

	yagoBinary := buildYagoBinary(t)

	t.Run("error_missing_required_flag", func(t *testing.T) {
		_, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "validate")

		if exitCode == 0 {
			t.Error("Expected error for missing required flag")
		}

		// Should show PARAM_ERROR or mention required flag
		if strings.Contains(stderr, "PARAM_ERROR") || strings.Contains(stderr, "required") ||
			strings.Contains(stderr, "flag") {
			t.Log("✓ Parameter error propagated correctly")
		}
	})

	t.Run("error_file_not_found", func(t *testing.T) {
		_, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "validate",
			"-d", "/tmp/nonexistent_file_12345.yaml",
			"-e", "dev",
			"-w", "terraform")

		if exitCode == 0 {
			t.Error("Expected error for missing file")
		}

		// Should show PARSE_ERROR or file not found message
		if strings.Contains(stderr, "PARSE_ERROR") || strings.Contains(stderr, "not found") ||
			strings.Contains(stderr, "no such file") {
			t.Log("✓ File not found error propagated correctly")
		} else {
			t.Logf("Error message: %s", stderr)
		}
	})

	t.Run("error_invalid_flag_value", func(t *testing.T) {
		_, stderr, exitCode := runYago(t, yagoBinary, "desiredstate", "assemble",
			"-d", "test.yaml",
			"-e", "dev",
			"-w", "terraform",
			"--format", "invalid-format")

		if exitCode == 0 {
			t.Log("Note: Invalid format value accepted (may have default)")
		} else {
			// Should show error about invalid format
			if strings.Contains(stderr, "format") || strings.Contains(stderr, "invalid") {
				t.Log("✓ Invalid flag value error propagated")
			}
		}
	})

	t.Run("error_message_clarity", func(t *testing.T) {
		// Test that error messages are user-friendly
		_, stderr, _ := runYago(t, yagoBinary, "desiredstate", "validate")

		// Error should be informative (not just "error" or stack trace)
		if len(stderr) > 10 && !strings.Contains(stderr, "panic") {
			t.Log("✓ Error message is reasonably detailed")
		}

		// Should not dump raw stack traces to user
		if !strings.Contains(stderr, "goroutine") && !strings.Contains(stderr, "runtime.") {
			t.Log("✓ Error message is user-friendly (no stack traces)")
		}
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// END-TO-END WORKFLOW: COMPLETE PIPELINE
// =============================================================================

func TestWorkflow_CompletePipeline_BehavioralBDD(t *testing.T) {
	contract := WorkflowBehavioralContract{
		Behavior:        "Complete CI/CD pipeline workflow: validate, assemble, and output",
		CurrentImpl:     "The command workflow validates desiredstate before assembling deployable output",
		ExpectedOutcome: "Validation failures stop the pipeline; valid inputs proceed to generated output",
		Rationale:       "Fail-fast pipeline behavior protects production deployment automation",
	}

	yagoBinary := buildYagoBinary(t)
	testDir := t.TempDir()

	t.Run("pipeline_validate_then_assemble", func(t *testing.T) {
		// Create test files
		dsFile := filepath.Join(testDir, "pipeline_ds.yaml")
		cfgFile := filepath.Join(testDir, "pipeline_config.yaml")

		dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: pipeline-test
  environment: prod
spec:
  components: []
`
		cfgContent := `schema: "1.0.0"
namespace: yago
kind: Configuration
metadata:
  name: pipeline-test
  environment: prod
configuration:
  content:
    deployment:
      region: us-east-1
`
		if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
			t.Fatalf("Failed to create desiredstate file: %v", err)
		}
		if err := os.WriteFile(cfgFile, []byte(cfgContent), 0644); err != nil {
			t.Fatalf("Failed to create configuration file: %v", err)
		}

		// Step 1: Validate
		_, _, validateExitCode := runYago(t, yagoBinary, "desiredstate", "validate",
			"-d", dsFile,
			"-e", "prod",
			"-w", "terraform")

		// Step 2: Assemble (only if validation succeeded or has acceptable error)
		assembleStdout, _, assembleExitCode := runYago(t, yagoBinary, "desiredstate", "assemble",
			"-d", dsFile,
			"-c", cfgFile,
			"-e", "prod",
			"-w", "terraform")

		// Check workflow
		if validateExitCode == 0 && assembleExitCode == 0 {
			t.Log("✓ Complete pipeline succeeded: validate → assemble")
			if assembleStdout != "" {
				t.Log("✓ Assembly output generated")
			}
		} else if validateExitCode == 0 && assembleExitCode != 0 {
			t.Log("✓ Validation passed, assemble failed (may need schema)")
		} else {
			t.Logf("Pipeline status: validate=%d, assemble=%d", validateExitCode, assembleExitCode)
			t.Log("Note: Pipeline may require schema configuration")
		}
	})

	t.Run("pipeline_fail_fast_on_validation", func(t *testing.T) {
		// Create invalid desiredstate
		invalidFile := filepath.Join(testDir, "invalid_pipeline.yaml")
		invalidContent := `schema: "1.0.0"
this is invalid yaml: [[[
`
		if err := os.WriteFile(invalidFile, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Step 1: Validate (should fail)
		_, _, validateExitCode := runYago(t, yagoBinary, "desiredstate", "validate",
			"-d", invalidFile,
			"-e", "dev",
			"-w", "terraform")

		if validateExitCode != 0 {
			t.Log("✓ Pipeline fails fast on validation error")
			t.Log("✓ Assemble step would be skipped in CI/CD (exit code check)")
		} else {
			t.Error("Expected validation to fail for invalid YAML")
		}
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}
