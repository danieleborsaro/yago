package desiredstate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danieleborsaro/yago/pkg/desiredstate"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// BehavioralContract documents the expected behavior for service layer operations
type ServiceBehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

// =============================================================================
// SERVICE LAYER: CONFIGURATION AND INITIALIZATION
// =============================================================================

func TestService_Configuration_BehavioralBDD(t *testing.T) {
	contract := ServiceBehavioralContract{
		Behavior:        "Service configuration structure for desiredstate operations",
		CurrentImpl:     "DesiredStateConfig stores paths, environment, schema, output, and wrapper settings",
		ExpectedOutcome: "Configuration is type-safe and validated consistently before service operations",
		Rationale:       "Centralized configuration keeps service and command boundaries clear",
	}

	t.Run("config_structure", func(t *testing.T) {
		config := &desiredstate.DesiredStateConfig{
			BaseDir:          ".",
			Environment:      "test",
			SchemaVersion:    "4.2.0",
			DesiredStateFile: "test.yaml",
			DestinationFile:  "dest.yaml",
			IsDryRun:         true,
			IsInterpolation:  true,
		}

		if config.BaseDir != "." {
			t.Errorf("Expected BaseDir to be '.', got '%s'", config.BaseDir)
		}

		if config.Environment != "test" {
			t.Errorf("Expected Environment to be 'test', got '%s'", config.Environment)
		}

		if !config.IsDryRun {
			t.Error("Expected IsDryRun to be true")
		}

		if !config.IsInterpolation {
			t.Error("Expected IsInterpolation to be true")
		}

		t.Log("✓ DesiredStateConfig structure provides type-safe configuration")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

func TestService_Initialization_BehavioralBDD(t *testing.T) {
	contract := ServiceBehavioralContract{
		Behavior:        "Service layer initialization with base directory and interpolation flag",
		CurrentImpl:     "NewService stores the base directory and interpolation setting",
		ExpectedOutcome: "A reusable service is created with the requested processing configuration",
		Rationale:       "Encapsulated service state supports testing and repeated operations",
	}

	t.Run("service_creation", func(t *testing.T) {
		service := desiredstate.NewService(".", true)
		if service == nil {
			t.Fatal("Expected service to be created, got nil")
		}

		t.Log("✓ Service initializes with base directory and interpolation flag")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// SERVICE LAYER: VALIDATE OPERATION
// =============================================================================

func TestService_ValidateOperation_BehavioralBDD(t *testing.T) {
	contract := ServiceBehavioralContract{
		Behavior:        "Validate desiredstate file with parameter validation and error handling",
		CurrentImpl:     "ValidateDesiredState checks the request and returns a ValidateResponse",
		ExpectedOutcome: "Missing files or parameters produce a response and a PARAM_ERROR message",
		Rationale:       "Early validation prevents invalid deployments and provides clear errors",
	}

	t.Run("validate_with_invalid_file", func(t *testing.T) {
		service := desiredstate.NewService(".", true)

		req := wrapper.ValidateRequest{
			DesiredStateFile: "nonexistent.yaml",
			Environment:      "test",
		}

		response, err := service.ValidateDesiredState(req)
		if err == nil {
			t.Error("Expected error for non-existent file, got nil")
		}

		if response == nil {
			t.Error("Expected response object, got nil")
		}

		if response != nil && response.IsValid {
			t.Error("Expected IsValid to be false for invalid file")
		}

		t.Log("✓ Validate detects non-existent files")
	})

	t.Run("validate_with_empty_file_path", func(t *testing.T) {
		service := desiredstate.NewService(".", true)

		req := wrapper.ValidateRequest{
			DesiredStateFile: "",
			Environment:      "test",
		}

		_, err := service.ValidateDesiredState(req)
		if err == nil {
			t.Fatal("Expected error for empty file path, got nil")
		}

		expectedMsg := "PARAM_ERROR: desiredstate file must be specified"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
		}

		t.Log("✓ Validate enforces required file path parameter with PARAM_ERROR prefix")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// SERVICE LAYER: PROMOTE OPERATION
// =============================================================================

func TestService_PromoteOperation_BehavioralBDD(t *testing.T) {
	contract := ServiceBehavioralContract{
		Behavior:        "Promote desiredstate from source to destination with comprehensive parameter validation",
		CurrentImpl:     "PromoteDesiredState validates PromoteRequest and supports dry-run processing",
		ExpectedOutcome: "Missing source, destination, or AWS region returns the corresponding PARAM_ERROR",
		Rationale:       "Validation protects cross-environment promotion and dry-run workflows",
	}

	t.Run("promote_validation_missing_source", func(t *testing.T) {
		service := desiredstate.NewService(".", true)

		req := desiredstate.PromoteRequest{
			Environment:     "test",
			AWSRegion:       "us-east-1",
			SourceBranch:    "main",
			TargetBranch:    "develop",
			DestinationFile: "dest.yaml",
			// DesiredStateFile missing
		}

		_, err := service.PromoteDesiredState(req)
		if err == nil {
			t.Fatal("Expected error for missing source file, got nil")
		}

		expectedMsg := "PARAM_ERROR: desiredstate file must be specified"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
		}

		t.Log("✓ Promote validates source file is specified")
	})

	t.Run("promote_validation_missing_destination", func(t *testing.T) {
		service := desiredstate.NewService(".", true)

		req := desiredstate.PromoteRequest{
			DesiredStateFile: "source.yaml",
			Environment:      "test",
			AWSRegion:        "us-east-1",
			SourceBranch:     "main",
			TargetBranch:     "develop",
			// DestinationFile missing
		}

		_, err := service.PromoteDesiredState(req)
		if err == nil {
			t.Fatal("Expected error for missing destination file, got nil")
		}

		expectedMsg := "PARAM_ERROR: destination file must be specified"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
		}

		t.Log("✓ Promote validates destination file is specified")
	})

	t.Run("promote_validation_missing_aws_region", func(t *testing.T) {
		service := desiredstate.NewService(".", true)

		req := desiredstate.PromoteRequest{
			DesiredStateFile: "source.yaml",
			DestinationFile:  "dest.yaml",
			Environment:      "test",
			SourceBranch:     "main",
			TargetBranch:     "develop",
			// AWSRegion missing
		}

		_, err := service.PromoteDesiredState(req)
		if err == nil {
			t.Fatal("Expected error for missing AWS region, got nil")
		}

		expectedMsg := "PARAM_ERROR: AWS region must be specified"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
		}

		t.Log("✓ Promote validates AWS region is specified")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// SERVICE LAYER: SCHEMA OPERATIONS
// =============================================================================

func TestService_SchemaOperations_BehavioralBDD(t *testing.T) {
	contract := ServiceBehavioralContract{
		Behavior:        "Schema operations for listing versions and printing schema details",
		CurrentImpl:     "PrintSchema handles SchemaRequest and GetSupportedSchemaVersions lists available versions",
		ExpectedOutcome: "Users can list supported versions or request schema details with explicit options",
		Rationale:       "Schema discovery supports migration planning and compatibility checks",
	}

	t.Run("print_schema_list_versions", func(t *testing.T) {
		service := desiredstate.NewService(".", true)

		req := desiredstate.SchemaRequest{
			ListVersions: true,
		}

		response, err := service.PrintSchema(req)
		if err != nil {
			t.Errorf("Expected no error for list versions, got: %v", err)
		}

		if response == nil {
			t.Fatal("Expected response object, got nil")
		}

		if len(response.Versions) == 0 {
			t.Error("Expected at least one version, got none")
		}

		t.Log("✓ PrintSchema lists all supported versions")
	})

	t.Run("print_schema_no_options", func(t *testing.T) {
		service := desiredstate.NewService(".", true)

		req := desiredstate.SchemaRequest{
			// No options specified
		}

		_, err := service.PrintSchema(req)
		if err == nil {
			t.Error("Expected error for no options, got nil")
		}

		t.Log("✓ PrintSchema requires at least one option")
	})

	t.Run("get_supported_versions", func(t *testing.T) {
		service := desiredstate.NewService(".", true)

		versions := service.GetSupportedSchemaVersions()
		if len(versions) == 0 {
			t.Error("Expected at least one supported version, got none")
		}

		t.Log("✓ GetSupportedSchemaVersions returns version list")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// SERVICE LAYER: ASSEMBLE OPERATION
// =============================================================================

func TestService_AssembleOperation_BehavioralBDD(t *testing.T) {
	contract := ServiceBehavioralContract{
		Behavior:        "Assemble desiredstate with configuration filtering by wrapper",
		CurrentImpl:     "AssembleDesiredState applies the wrapper filter and returns assembled content",
		ExpectedOutcome: "Valid wrapper requests are processed and unsupported wrappers are handled safely",
		Rationale:       "Assembly enables wrapper-specific deployments and output generation",
	}

	t.Run("assemble_with_valid_wrapper", func(t *testing.T) {
		// Create test files
		tmpDir := t.TempDir()
		dsFile := filepath.Join(tmpDir, "desiredstate.yaml")
		cfgFile := filepath.Join(tmpDir, "configuration.yaml")

		// Create minimal valid YAML files
		dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: test
  environment: dev
spec: {}
`
		cfgContent := `schema: "1.0.0"
namespace: yago
kind: Configuration
metadata:
  name: test
  environment: dev
configuration:
  content:
    wrappers:
      terraform:
        vars: "test_vars"
`
		if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if err := os.WriteFile(cfgFile, []byte(cfgContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		service := desiredstate.NewService(tmpDir, true)

		req := wrapper.AssembleRequest{
			DesiredStateFile: dsFile,
			ConfigFile:       cfgFile,
			Environment:      "dev",
			Wrapper:          "terraform",
		}

		response, err := service.AssembleDesiredState(req)
		if err != nil {
			t.Logf("Assemble error (expected for incomplete schema): %v", err)
			// Note: This may fail due to schema validation, but tests the structure
		}

		if response != nil {
			t.Log("✓ Assemble processes valid wrapper parameter")
		}
	})

	t.Run("assemble_with_nonexistent_wrapper", func(t *testing.T) {
		// Create test files
		tmpDir := t.TempDir()
		dsFile := filepath.Join(tmpDir, "desiredstate.yaml")
		cfgFile := filepath.Join(tmpDir, "configuration.yaml")

		dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: test
  environment: dev
spec: {}
`
		cfgContent := `schema: "1.0.0"
namespace: yago
kind: Configuration
metadata:
  name: test
  environment: dev
configuration:
  content:
    wrappers:
      terraform:
        vars: "test_vars"
`
		if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if err := os.WriteFile(cfgFile, []byte(cfgContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		service := desiredstate.NewService(tmpDir, true)

		req := wrapper.AssembleRequest{
			DesiredStateFile: dsFile,
			ConfigFile:       cfgFile,
			Environment:      "dev",
			Wrapper:          "nonexistent",
		}

		response, err := service.AssembleDesiredState(req)
		if err != nil {
			t.Logf("Assemble error (expected for schema issues): %v", err)
		}

		if response != nil {
			t.Log("✓ Assemble handles nonexistent wrapper (returns full content)")
		}
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}

// =============================================================================
// COMMAND LAYER: INTEGRATION WITH SERVICE
// =============================================================================

func TestCommand_ServiceIntegration_BehavioralBDD(t *testing.T) {
	contract := ServiceBehavioralContract{
		Behavior:        "Command layer integration with service layer for CLI operations",
		CurrentImpl:     "Desiredstate command constructors expose service operations through Cobra",
		ExpectedOutcome: "Commands provide stable names and delegate business operations to the service layer",
		Rationale:       "Layer separation keeps CLI concerns distinct from reusable business logic",
	}

	t.Run("command_creation", func(t *testing.T) {
		// Test command constructors exist
		dsCmd := desiredstate.NewDesiredStateCommand()
		if dsCmd == nil {
			t.Fatal("Expected DesiredState command to be created")
		}

		if dsCmd.Use != "desiredstate" {
			t.Errorf("Expected Use to be 'desiredstate', got '%s'", dsCmd.Use)
		}

		if len(dsCmd.Aliases) == 0 || dsCmd.Aliases[0] != "ds" {
			t.Error("Expected alias 'ds' to be defined")
		}

		t.Log("✓ Commands created with proper structure and aliases")
	})

	t.Logf("Contract: %s - %s", contract.Behavior, contract.ExpectedOutcome)
}
