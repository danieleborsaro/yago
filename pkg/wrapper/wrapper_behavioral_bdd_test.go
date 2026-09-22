package wrapper

import (
	"os"
	"path/filepath"
	"testing"
)

// BehavioralContract documents expected behavior and the current implementation.
type BehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

// ===================================================================================================
// BASE PARSER BEHAVIORAL BDD TESTS
// Testing base parser behavior.
// ===================================================================================================

func TestBaseParser_Initialization_BehavioralBDD(t *testing.T) {
	t.Log("=== BEHAVIORAL CONTRACT: Initialize BaseParser with environment and variables ===")
	contract := BehavioralContract{
		Behavior:        "BaseParser initializes environment, variables, and workspace/build prefixes",
		CurrentImpl:     "NewBaseParser initializes environment state and defaults workspace to gitops and build paths to .gitops",
		ExpectedOutcome: "Parser state starts predictable and document references are nil",
		Rationale:       "Shared defaults keep wrapper workspace handling consistent",
	}

	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Rationale: %s", contract.Rationale)

	// Test 1: Basic initialization
	t.Run("basic_initialization", func(t *testing.T) {
		parser := NewBaseParser("dev", nil)

		if parser.GetEnvironment() != "dev" {
			t.Errorf("Expected environment 'dev', got '%s'", parser.GetEnvironment())
		}

		if parser.workspacePrefix != "gitops" {
			t.Errorf("Expected workspace prefix 'gitops', got '%s'", parser.workspacePrefix)
		}

		if parser.buildDirPrefix != ".gitops" {
			t.Errorf("Expected build prefix '.gitops', got '%s'", parser.buildDirPrefix)
		}

		if parser.GetDesiredState() != nil {
			t.Error("DesiredState should be nil initially")
		}

		if parser.GetConfiguration() != nil {
			t.Error("Configuration should be nil initially")
		}

		t.Log("✓ Parser initialized with correct defaults")
	})

	// Test 2: With environment variables
	t.Run("with_environment_variables", func(t *testing.T) {
		envVars := map[string]string{
			"AWS_REGION":  "us-east-1",
			"ENVIRONMENT": "production",
		}

		parser := NewBaseParser("prod", envVars)

		if parser.GetEnvironment() != "prod" {
			t.Errorf("Expected environment 'prod', got '%s'", parser.GetEnvironment())
		}

		retrievedVars := parser.GetEnvVariables()
		if retrievedVars["AWS_REGION"] != "us-east-1" {
			t.Errorf("Expected AWS_REGION 'us-east-1', got '%s'", retrievedVars["AWS_REGION"])
		}

		t.Log("✓ Parser initialized with environment variables")
	})

	t.Log("\n✅ CONTRACT SATISFIED: BaseParser initialization works correctly")
}

func TestBaseParser_SetWorkspace_BehavioralBDD(t *testing.T) {
	t.Log("=== BEHAVIORAL CONTRACT: Create and manage workspace directory ===")
	contract := BehavioralContract{
		Behavior:        "SetWorkspace creates a temporary workspace or uses a provided directory",
		CurrentImpl:     "SetWorkspace creates directories with the gitops prefix and reuses an existing workspace",
		ExpectedOutcome: "Workspace paths exist, are stable across repeated calls, and can be cleaned up",
		Rationale:       "Unique staging directories prevent conflicts during repository processing",
	}

	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Rationale: %s", contract.Rationale)

	// Test 1: Auto-create temporary workspace
	t.Run("auto_create_temp_workspace", func(t *testing.T) {
		parser := NewBaseParser("dev", nil)

		err := parser.SetWorkspace("")
		if err != nil {
			t.Fatalf("SetWorkspace failed: %v", err)
		}

		workspaceDir := parser.GetWorkspaceDir()
		if workspaceDir == "" {
			t.Error("Workspace directory should not be empty")
		}

		// Verify directory exists
		if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
			t.Errorf("Workspace directory doesn't exist: %s", workspaceDir)
		}

		// Verify prefix
		baseName := filepath.Base(workspaceDir)
		if len(baseName) < 6 || baseName[:6] != "gitops" {
			t.Errorf("Workspace should have 'gitops' prefix, got: %s", baseName)
		}

		// Cleanup
		os.RemoveAll(workspaceDir)

		t.Logf("✓ Created temporary workspace: %s", workspaceDir)
	})

	// Test 2: Use provided directory
	t.Run("use_provided_directory", func(t *testing.T) {
		parser := NewBaseParser("dev", nil)

		tmpDir := t.TempDir()
		customDir := filepath.Join(tmpDir, "custom-workspace")

		err := parser.SetWorkspace(customDir)
		if err != nil {
			t.Fatalf("SetWorkspace failed: %v", err)
		}

		if parser.GetWorkspaceDir() != customDir {
			t.Errorf("Expected workspace '%s', got '%s'", customDir, parser.GetWorkspaceDir())
		}

		// Verify directory exists
		if _, err := os.Stat(customDir); os.IsNotExist(err) {
			t.Errorf("Custom workspace directory doesn't exist: %s", customDir)
		}

		t.Logf("✓ Used provided directory: %s", customDir)
	})

	// Test 3: Idempotent - don't recreate if already set
	t.Run("idempotent_workspace", func(t *testing.T) {
		parser := NewBaseParser("dev", nil)

		// First call
		err := parser.SetWorkspace("")
		if err != nil {
			t.Fatalf("First SetWorkspace failed: %v", err)
		}
		firstDir := parser.GetWorkspaceDir()

		// Second call - should not change
		err = parser.SetWorkspace("")
		if err != nil {
			t.Fatalf("Second SetWorkspace failed: %v", err)
		}
		secondDir := parser.GetWorkspaceDir()

		if firstDir != secondDir {
			t.Errorf("Workspace changed on second call: %s != %s", firstDir, secondDir)
		}

		// Cleanup
		os.RemoveAll(firstDir)

		t.Log("✓ Workspace remains the same on subsequent calls")
	})

	t.Log("\n✅ CONTRACT SATISFIED: SetWorkspace manages directories correctly")
}

func TestBaseParser_Cache_BehavioralBDD(t *testing.T) {
	t.Log("=== BEHAVIORAL CONTRACT: Cache assembled documents to files ===")
	contract := BehavioralContract{
		Behavior:        "Cache saves assembled documents to a temporary or provided build directory",
		CurrentImpl:     "Cache accepts a structured request and returns a CacheResponse with the build path",
		ExpectedOutcome: "Documents are cached in the requested format under a stable build directory",
		Rationale:       "Cached artifacts support multi-stage GitOps workflows and downstream tools",
	}

	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Rationale: %s", contract.Rationale)

	t.Log("\n✅ CONTRACT DOCUMENTED: Cache behavior specified")
	t.Log("   Creates build directory (temp or provided)")
	t.Log("   Caches documents to files")
	t.Log("   Supports JSON/YAML formats")
	t.Log("   Returns structured response (CacheResponse)")
	t.Log("\n⚠️  FULL INTEGRATION TEST: Requires actual document objects with cache() method")
}

func TestBaseParser_Clear_BehavioralBDD(t *testing.T) {
	t.Log("=== BEHAVIORAL CONTRACT: Clear cached files ===")
	contract := BehavioralContract{
		Behavior:        "Clear removes the cached build directory and temporary files",
		CurrentImpl:     "Clear removes the build directory and resets its tracked path",
		ExpectedOutcome: "Temporary artifacts are removed and the parser returns to an uncached state",
		Rationale:       "Cleanup prevents disk accumulation and stale state between operations",
	}

	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Rationale: %s", contract.Rationale)

	// Test: Clear removes build directory
	t.Run("clear_build_directory", func(t *testing.T) {
		parser := NewBaseParser("dev", nil)

		// Create a build directory
		tmpDir := t.TempDir()
		buildDir := filepath.Join(tmpDir, ".gitops-test")
		err := os.MkdirAll(buildDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create build directory: %v", err)
		}

		parser.buildDirPath = buildDir

		// Verify it exists
		if _, err := os.Stat(buildDir); os.IsNotExist(err) {
			t.Fatal("Build directory should exist before Clear()")
		}

		// Clear
		err = parser.Clear()
		if err != nil {
			t.Errorf("Clear failed: %v", err)
		}

		// Verify it's removed
		if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
			t.Error("Build directory should be removed after Clear()")
		}

		// Verify buildDirPath is reset
		if parser.buildDirPath != "" {
			t.Errorf("buildDirPath should be empty after Clear(), got: %s", parser.buildDirPath)
		}

		t.Log("✓ Build directory removed successfully")
	})

	t.Log("\n✅ CONTRACT SATISFIED: Clear removes build directory correctly")
}

// ===================================================================================================
// BASE DESIREDSTATE BEHAVIORAL BDD TESTS
// Testing base desiredstate behavior.
// ===================================================================================================

func TestBaseDesiredState_Initialization_BehavioralBDD(t *testing.T) {
	t.Log("=== BEHAVIORAL CONTRACT: Initialize BaseDesiredState with environment and wrapper ===")
	contract := BehavioralContract{
		Behavior:        "BaseDesiredState initializes environment, wrapper, and its document",
		CurrentImpl:     "NewBaseDesiredState composes a GitOpsDocument and stores environment and wrapper state",
		ExpectedOutcome: "Desiredstate objects expose initialized environment, wrapper, and document values",
		Rationale:       "The desiredstate object is the source of truth for wrapper-specific operations",
	}

	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Rationale: %s", contract.Rationale)

	// Test: Basic initialization
	t.Run("basic_initialization", func(t *testing.T) {
		ds := NewBaseDesiredState("dev", "terraform", nil)

		if ds.GetEnvironment() != "dev" {
			t.Errorf("Expected environment 'dev', got '%s'", ds.GetEnvironment())
		}

		if ds.GetWrapper() != "terraform" {
			t.Errorf("Expected wrapper 'terraform', got '%s'", ds.GetWrapper())
		}

		if ds.GetDocument() == nil {
			t.Error("Wrapped GitOpsDocument should not be nil")
		}

		t.Log("✓ DesiredState initialized correctly")
	})

	t.Log("\n✅ CONTRACT SATISFIED: BaseDesiredState initialization works correctly")
	t.Log("   Composition keeps document state explicit and reusable")
}

// ===================================================================================================
// ARCHITECTURAL IMPROVEMENT DOCUMENTATION
// ===================================================================================================

func TestArchitecturalImprovement_SchemaJSON_BehavioralBDD(t *testing.T) {
	t.Log("=== ARCHITECTURAL IMPROVEMENT: Schema JSON Queries vs Hardcoded Properties ===")
	improvement := BehavioralContract{
		Behavior:        "Access schema-specific property paths for wrappers",
		CurrentImpl:     "Wrapper properties are resolved dynamically from versioned schema JSON",
		ExpectedOutcome: "Schema changes and new wrapper paths remain data-driven rather than hardcoded",
		Rationale:       "Schema-driven access improves maintainability, versioning, and extensibility",
	}

	t.Logf("ARCHITECTURAL IMPROVEMENT: %s", improvement.Behavior)
	t.Logf("Current implementation: %s", improvement.CurrentImpl)
	t.Logf("Expected outcome: %s", improvement.ExpectedOutcome)
	t.Logf("Rationale: %s", improvement.Rationale)

	t.Log("\n✅ ARCHITECTURAL IMPROVEMENT DOCUMENTED")
	t.Log("   Schema JSON queries are dynamic, versioned, and maintainable")
	t.Log("   Impact: New wrapper paths require schema updates rather than code changes")
}
