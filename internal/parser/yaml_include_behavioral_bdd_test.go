package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// FILE MIGRATION METADATA
// ============================================================================
// Original file: internal/parser/yaml_include_test.go
// Migration date: October 13, 2025
// Tests migrated: 4 (Simple, Nested, CircularDependency, MissingFile)
// Behavioral contracts created: 4
//
// This file documents the YAML !include tag functionality through Time-Based
// BDD. It characterizes how the YAML handler processes file inclusion
// directives, enabling modular configuration management.
//
// YAML INCLUDE OVERVIEW:
// The !include tag is a custom YAML tag that enables file inclusion during
// parsing. When encountered, the tag is replaced with the contents of the
// referenced file, supporting composition and reusability.
//
// SYNTAX:
//   config: !include base.yaml
//
// Result: The 'config' key contains the parsed contents of base.yaml
//
// KEY BEHAVIORS:
// 1. Simple inclusion: Single-level file references
// 2. Nested includes: Files that include other files (recursive)
// 3. Circular dependency detection: Prevents infinite loops
// 4. Missing file handling: Error reporting for non-existent files
//
// INCLUDE RESOLUTION:
// - Relative paths: Resolved relative to working directory
// - File loading: Recursive LoadFile calls
// - Content replacement: Tag replaced with file contents
// - Type preservation: Included content structure preserved
//
// USE CASES:
// - Configuration composition (base + overrides)
// - Shared configuration libraries
// - Environment-specific configs (secrets, endpoints)
// - Modular GitOps repository structures
//
// INTEGRATION POINTS:
// - Used by: Repository configurations, multi-environment setups
// - Depends on: LoadFile method, working directory resolution
// - Security: File system access within working directory
//
// TYPICAL USAGE:
//   handler := NewYAMLHandler("/path/to/configs")
//   content, err := handler.LoadFile("main.yaml", nil)
//   // main.yaml can contain: config: !include shared/base.yaml
//
// ============================================================================

// Note: GoBehavioralContract struct already defined in yaml_basic_behavioral_bdd_test.go

// ============================================================================
// BEHAVIORAL CONTRACT 1: Simple File Inclusion
// ============================================================================
// CUSTOM TAG: !include <filename>
//
// DESCRIPTION:
// The !include tag triggers file loading and content substitution. When the
// parser encounters !include, it loads the referenced file, parses it, and
// replaces the tag with the parsed content.
//
// BEHAVIOR ANALYSIS:
// During YAML parsing, custom tag handlers detect !include, extract the
// filename, call LoadFile recursively, and substitute the tag with the
// loaded content. The process is transparent to the caller.
//
// RESOLUTION RULES:
// - Relative paths: Resolved relative to handler's working directory
// - Absolute paths: Used as-is (if supported)
// - File extension: Typically .yaml or .yml
// - Parse format: Same as main file (YAML parsing)
//
// CONTENT REPLACEMENT:
//   Before (in file):    config: !include base.yaml
//   After (in memory):   config: { database: { host: localhost, port: 5432 } }
//
// STRUCTURE PRESERVATION:
// - Maps → maps
// - Sequences → sequences
// - Scalars → scalars
// - Type information preserved
//
// ERROR CONDITIONS:
// - File not found (missing include target)
// - Permission denied (file access)
// - Parse error (malformed YAML in included file)
// - Circular dependency (A includes B includes A)
//
// DESIGN RATIONALE:
// File inclusion enables configuration composition without duplication.
// Essential for managing large GitOps configurations across environments.
//
// ============================================================================

func TestIncludeTag_Simple_BehavioralBDD(t *testing.T) {
	// Contract: !include tag loads file and replaces tag with content
	contract := GoBehavioralContract{
		Behavior:        "LoadFile processes !include tags by loading referenced files and substituting tag with parsed file content",
		CurrentImpl:     "Detects !include <filename> custom tag, resolves path relative to working directory, recursively calls LoadFile, replaces tag with loaded content",
		ExpectedOutcome: "config field contains parsed content from base.yaml (database.host, database.port), not the !include tag itself",
		TestScenario:    "main.yaml contains 'config: !include base.yaml', base.yaml has database config, verify config contains database after loading",
		Rationale:       "File inclusion enables modular configuration management, shared config libraries, and environment-specific composition without duplication",
		RegressionRisk:  "CRITICAL - !include is core GitOps feature. Tag syntax change breaks all modular configs. Path resolution changes affect file discovery",
	}

	t.Logf("\n"+
		"=== BEHAVIORAL CONTRACT ===\n"+
		"Behavior: %s\n"+
		"Current Implementation: %s\n"+
		"Expected Outcome: %s\n"+
		"Test Scenario: %s\n"+
		"Rationale: %s\n"+
		"Regression Risk: %s\n"+
		"===========================\n",
		contract.Behavior,
		contract.CurrentImpl,
		contract.ExpectedOutcome,
		contract.TestScenario,
		contract.Rationale,
		contract.RegressionRisk,
	)

	// SETUP: Create temporary test directory
	tmpDir := t.TempDir()

	// GIVEN: base.yaml with database configuration
	baseContent := `database:
  host: localhost
  port: 5432
`
	baseFile := filepath.Join(tmpDir, "base.yaml")
	err := os.WriteFile(baseFile, []byte(baseContent), 0644)
	require.NoError(t, err, "Failed to create base file")

	// GIVEN: main.yaml with !include tag
	mainContent := `app:
  name: myapp
config: !include base.yaml
`
	mainFile := filepath.Join(tmpDir, "main.yaml")
	err = os.WriteFile(mainFile, []byte(mainContent), 0644)
	require.NoError(t, err, "Failed to create main file")

	// WHEN: Load main file with !include tag
	handler := NewYAMLHandler(tmpDir)
	content, err := handler.LoadFile(mainFile, nil)

	// THEN: Should load successfully
	require.NoError(t, err, "Failed to load file with !include")
	require.NotNil(t, content, "Loaded content should not be nil")

	// VERIFICATION 1: Original fields preserved
	// Behavior: Fields without !include tags remain unchanged
	assert.NotNil(t, content["app"], "app field should exist")

	// VERIFICATION 2: !include tag replaced with file content
	// Behavior: config no longer contains tag, contains file content instead
	assert.NotNil(t, content["config"], "config field should exist after !include substitution")

	config, ok := content["config"].(map[string]interface{})
	require.True(t, ok, "config should be a map after !include, got %T", content["config"])

	// VERIFICATION 3: Included file structure preserved
	// Behavior: base.yaml structure appears under config key
	database, ok := config["database"].(map[string]interface{})
	require.True(t, ok, "database should be a map, got %T", config["database"])

	// VERIFICATION 4: Included values correct
	// Behavior: Values from base.yaml preserved accurately
	assert.Equal(t, "localhost", database["host"], "host should be 'localhost'")
	assert.Equal(t, 5432, database["port"], "port should be 5432")

	t.Logf("✓ !include tag successfully loaded and replaced content")

	// BEHAVIORAL INSIGHTS:
	// 1. Tag substitution: !include replaced entirely
	//    - Before: config: !include base.yaml
	//    - After: config: { database: { host: ..., port: ... } }
	//    - No trace of !include in final structure
	//
	// 2. Relative path resolution: base.yaml found in tmpDir
	//    - Working directory used as base for resolution
	//    - Handler initialized with tmpDir
	//    - No need for absolute paths in YAML
	//
	// 3. Structure preservation: Nested maps maintained
	//    - database.host, database.port structure preserved
	//    - Type information retained (int port, string host)
	//    - Not flattened or modified
	//
	// 4. Merge semantics: Replacement, not merge
	//    - config field entirely replaced by base.yaml content
	//    - If config had existing fields, they'd be lost
	//    - Different from deep merge (tested elsewhere)
	//
	// 5. Use case: Shared database config
	//    - base.yaml contains DB connection details
	//    - Multiple apps include same base
	//    - Single source of truth for credentials
}

// ============================================================================
// BEHAVIORAL CONTRACT 2: Nested Include Resolution
// ============================================================================
// DESCRIPTION:
// Includes can be nested: file A includes file B which includes file C.
// The system recursively processes includes to arbitrary depth, building
// the final configuration from multiple layers.
//
// BEHAVIOR ANALYSIS:
// Each !include triggers recursive LoadFile call. Nested includes processed
// depth-first: innermost files loaded first, results propagated upward.
//
// NESTED STRUCTURE:
//   main.yaml:  root: !include level2.yaml
//   level2.yaml:  data: { nested: !include level3.yaml }
//   level3.yaml:  value: "deep nested"
//
//   Result: main → level2 → level3 → value accessible
//
// RECURSION DEPTH:
// - No explicit limit (implementation-dependent)
// - Practical limits: File system, memory, stack depth
// - Circular dependency protection prevents infinite recursion
//
// PATH RESOLUTION:
// - Each include resolved relative to handler's working directory
// - Not relative to including file (important distinction)
// - Consistent base directory for all includes
//
// ============================================================================

func TestIncludeTag_Nested_BehavioralBDD(t *testing.T) {
	// Contract: Nested !include tags processed recursively
	contract := GoBehavioralContract{
		Behavior:        "LoadFile recursively processes nested !include tags, allowing files to include files that include other files",
		CurrentImpl:     "Each !include triggers recursive LoadFile call, nested includes resolved depth-first, paths resolved relative to working directory",
		ExpectedOutcome: "Three-level nesting (main→level2→level3) successfully resolves, final structure contains deeply nested value from level3.yaml",
		TestScenario:    "main includes level2, level2 includes level3, level3 has value='deep nested', verify root.data.nested.value accessible",
		Rationale:       "Nested includes enable layered configuration hierarchies, shared component libraries, and complex GitOps structures without flattening",
		RegressionRisk:  "HIGH - Nested includes used in complex GitOps repos. Recursion depth limits or path resolution changes would break existing configs",
	}

	t.Logf("\n"+
		"=== BEHAVIORAL CONTRACT ===\n"+
		"Behavior: %s\n"+
		"Current Implementation: %s\n"+
		"Expected Outcome: %s\n"+
		"Test Scenario: %s\n"+
		"Rationale: %s\n"+
		"Regression Risk: %s\n"+
		"===========================\n",
		contract.Behavior,
		contract.CurrentImpl,
		contract.ExpectedOutcome,
		contract.TestScenario,
		contract.Rationale,
		contract.RegressionRisk,
	)

	// SETUP: Create temporary test directory
	tmpDir := t.TempDir()

	// GIVEN: level3.yaml (deepest level)
	level3Content := `value: "deep nested"`
	level3File := filepath.Join(tmpDir, "level3.yaml")
	err := os.WriteFile(level3File, []byte(level3Content), 0644)
	require.NoError(t, err, "Failed to create level3 file")

	// GIVEN: level2.yaml includes level3
	level2Content := `data:
  nested: !include level3.yaml
`
	level2File := filepath.Join(tmpDir, "level2.yaml")
	err = os.WriteFile(level2File, []byte(level2Content), 0644)
	require.NoError(t, err, "Failed to create level2 file")

	// GIVEN: main.yaml includes level2
	mainContent := `root: !include level2.yaml`
	mainFile := filepath.Join(tmpDir, "main.yaml")
	err = os.WriteFile(mainFile, []byte(mainContent), 0644)
	require.NoError(t, err, "Failed to create main file")

	// WHEN: Load main file (triggers nested include resolution)
	handler := NewYAMLHandler(tmpDir)
	content, err := handler.LoadFile(mainFile, nil)

	// THEN: Should resolve all nested includes
	require.NoError(t, err, "Failed to load file with nested !include")
	require.NotNil(t, content, "Loaded content should not be nil")

	// VERIFICATION 1: First level (root from level2.yaml)
	root, ok := content["root"].(map[string]interface{})
	require.True(t, ok, "root should be a map, got %T", content["root"])

	// VERIFICATION 2: Second level (data from level2.yaml)
	data, ok := root["data"].(map[string]interface{})
	require.True(t, ok, "data should be a map, got %T", root["data"])

	// VERIFICATION 3: Third level (nested from level3.yaml)
	nested, ok := data["nested"].(map[string]interface{})
	require.True(t, ok, "nested should be a map, got %T", data["nested"])

	// VERIFICATION 4: Final value (value from level3.yaml)
	assert.Equal(t, "deep nested", nested["value"], "value should be 'deep nested'")

	t.Logf("✓ Nested !include tags work correctly")

	// BEHAVIORAL INSIGHTS:
	// 1. Depth-first resolution: Innermost includes processed first
	//    - level3.yaml loaded first (no dependencies)
	//    - level2.yaml loaded next (includes level3)
	//    - main.yaml loaded last (includes level2)
	//    - Bottom-up construction of final structure
	//
	// 2. Path resolution: All relative to working directory
	//    - level2.yaml references "level3.yaml" (not ../level3.yaml)
	//    - All files in same directory (tmpDir)
	//    - Important: NOT relative to including file
	//
	// 3. Recursion mechanics: Each include is independent LoadFile
	//    - Stack depth grows with nesting
	//    - No tail recursion optimization
	//    - Practical limit depends on stack size
	//
	// 4. Structure nesting: Arbitrary depth supported
	//    - root.data.nested.value (4 levels)
	//    - Could go deeper (level4, level5, etc.)
	//    - Only limited by practical constraints
	//
	// 5. Use case: Component hierarchies
	//    - app.yaml includes common.yaml includes base.yaml
	//    - Shared components at different levels
	//    - Layered defaults (system → organization → team → app)
}

// ============================================================================
// BEHAVIORAL CONTRACT 3: Circular Dependency Detection
// ============================================================================
// DESCRIPTION:
// Circular dependencies occur when file A includes B, and B includes A,
// creating an infinite loop. The system must detect and prevent this to
// avoid stack overflow and infinite recursion.
//
// BEHAVIOR ANALYSIS:
// During include processing, the system tracks which files are currently
// being loaded (active include chain). If a file appears twice in the chain,
// a circular dependency is detected and an error is returned.
//
// DETECTION MECHANISM:
// - Track stack: Maintain set/list of files being processed
// - On each include: Check if file already in stack
// - If found: Circular dependency detected
// - Error: Return descriptive error with cycle information
//
// CIRCULAR PATTERNS:
//   Direct: A → B → A
//   Indirect: A → B → C → A
//   Self-reference: A → A (immediate cycle)
//
// ERROR MESSAGE:
// Typically includes "circular dependency" or "cycle" with file names
//
// DESIGN RATIONALE:
// Without detection, circular includes would cause:
// - Stack overflow (infinite recursion)
// - Process hang (infinite loop)
// - Resource exhaustion
// Protection is essential for system stability.
//
// ============================================================================

func TestIncludeTag_CircularDependency_BehavioralBDD(t *testing.T) {
	// Contract: Circular dependencies detected and prevented
	contract := GoBehavioralContract{
		Behavior:        "LoadFile detects circular include dependencies (A→B→A) and returns error to prevent infinite recursion",
		CurrentImpl:     "Tracks active include chain, checks for duplicate files before loading, returns 'circular dependency' error if cycle detected",
		ExpectedOutcome: "Loading a.yaml (which includes b.yaml which includes a.yaml) returns error containing 'circular dependency'",
		TestScenario:    "Create a.yaml including b.yaml, b.yaml including a.yaml (circular), attempt load, verify error returned",
		Rationale:       "Circular dependency detection prevents infinite recursion, stack overflow, and process hangs. Essential for system stability",
		RegressionRisk:  "CRITICAL - Removing detection would cause crashes. Error message format might be parsed by tooling. Must detect all cycle types",
	}

	t.Logf("\n"+
		"=== BEHAVIORAL CONTRACT ===\n"+
		"Behavior: %s\n"+
		"Current Implementation: %s\n"+
		"Expected Outcome: %s\n"+
		"Test Scenario: %s\n"+
		"Rationale: %s\n"+
		"Regression Risk: %s\n"+
		"===========================\n",
		contract.Behavior,
		contract.CurrentImpl,
		contract.ExpectedOutcome,
		contract.TestScenario,
		contract.Rationale,
		contract.RegressionRisk,
	)

	// SETUP: Create temporary test directory
	tmpDir := t.TempDir()

	// GIVEN: a.yaml includes b.yaml
	aContent := `data: !include b.yaml`
	aFile := filepath.Join(tmpDir, "a.yaml")
	err := os.WriteFile(aFile, []byte(aContent), 0644)
	require.NoError(t, err, "Failed to create a.yaml")

	// GIVEN: b.yaml includes a.yaml (creating circular dependency)
	bContent := `data: !include a.yaml`
	bFile := filepath.Join(tmpDir, "b.yaml")
	err = os.WriteFile(bFile, []byte(bContent), 0644)
	require.NoError(t, err, "Failed to create b.yaml")

	// WHEN: Attempt to load file with circular dependency
	handler := NewYAMLHandler(tmpDir)
	_, err = handler.LoadFile(aFile, nil)

	// THEN: Should detect circular dependency and return error
	require.Error(t, err, "Expected error for circular dependency")

	// VERIFICATION: Error message indicates circular dependency
	errorMsg := err.Error()
	assert.True(t,
		strings.Contains(strings.ToLower(errorMsg), "circular") ||
			strings.Contains(strings.ToLower(errorMsg), "cycle"),
		"Error should mention 'circular' or 'cycle', got: %s", errorMsg)

	t.Logf("✓ Circular dependency correctly detected: %v", err)

	// BEHAVIORAL INSIGHTS:
	// 1. Detection timing: Caught before infinite recursion
	//    - Not after stack overflow (proactive detection)
	//    - Error returned at point of cycle detection
	//    - No partial loading of circular files
	//
	// 2. Error message: Contains "circular" or "cycle"
	//    - Descriptive error for debugging
	//    - Might include file names in cycle
	//    - Helps users identify and fix issue
	//
	// 3. Detection algorithm: Track active include chain
	//    - Likely uses map/set of currently loading files
	//    - Check on each include: "is this file already loading?"
	//    - O(1) lookup for cycle detection
	//
	// 4. All cycle types detected:
	//    - Direct: A → B → A
	//    - Indirect: A → B → C → A
	//    - Self-reference: A → A (immediate)
	//    - This test validates direct cycle (A → B → A)
	//
	// 5. Recovery: Error allows graceful failure
	//    - No crash or hang
	//    - Caller can handle error
	//    - System remains stable
	//
	// 6. Use case prevention:
	//    - Accidental circular references in configs
	//    - Copy-paste errors in GitOps repos
	//    - Refactoring mistakes (moved includes)
}

// ============================================================================
// BEHAVIORAL CONTRACT 4: Missing File Error Handling
// ============================================================================
// DESCRIPTION:
// When !include references a non-existent file, the system returns an error
// rather than silently failing or substituting a default value. This ensures
// configuration errors are caught early.
//
// BEHAVIOR ANALYSIS:
// During include processing, file existence is checked before loading. If
// file doesn't exist, an error is returned immediately with information
// about the missing file.
//
// ERROR SCENARIOS:
// - File doesn't exist (typo in filename)
// - Wrong directory (path resolution issue)
// - Permission denied (file exists but not readable)
// - Invalid path (malformed path syntax)
//
// ERROR VS WARNING:
// - Error: Stops loading, returns error to caller
// - Not warning: Doesn't continue with missing include
// - Fail-fast: Catches configuration errors early
//
// DESIGN RATIONALE:
// Explicit errors prevent subtle bugs from missing configuration. Better to
// fail loudly at load time than succeed with incomplete configuration.
//
// ============================================================================

func TestIncludeTag_MissingFile_BehavioralBDD(t *testing.T) {
	// Contract: Missing include files cause load errors
	contract := GoBehavioralContract{
		Behavior:        "LoadFile returns error when !include references non-existent file, preventing silent configuration errors",
		CurrentImpl:     "Checks file existence before loading, returns descriptive error for missing files, includes filename in error message",
		ExpectedOutcome: "Loading main.yaml with '!include nonexistent.yaml' returns error (not nil, not warning)",
		TestScenario:    "Create main.yaml with !include nonexistent.yaml (file doesn't exist), attempt load, verify error returned",
		Rationale:       "Explicit errors for missing includes prevent silent configuration failures, catch typos and path issues early, enforce fail-fast principle",
		RegressionRisk:  "MEDIUM - Changing to warning/ignore would hide configuration errors. Error format might be parsed by tooling",
	}

	t.Logf("\n"+
		"=== BEHAVIORAL CONTRACT ===\n"+
		"Behavior: %s\n"+
		"Current Implementation: %s\n"+
		"Expected Outcome: %s\n"+
		"Test Scenario: %s\n"+
		"Rationale: %s\n"+
		"Regression Risk: %s\n"+
		"===========================\n",
		contract.Behavior,
		contract.CurrentImpl,
		contract.ExpectedOutcome,
		contract.TestScenario,
		contract.Rationale,
		contract.RegressionRisk,
	)

	// SETUP: Create temporary test directory
	tmpDir := t.TempDir()

	// GIVEN: main.yaml with !include to non-existent file
	mainContent := `data: !include nonexistent.yaml`
	mainFile := filepath.Join(tmpDir, "main.yaml")
	err := os.WriteFile(mainFile, []byte(mainContent), 0644)
	require.NoError(t, err, "Failed to create main file")

	// WHEN: Attempt to load file with missing include
	handler := NewYAMLHandler(tmpDir)
	_, err = handler.LoadFile(mainFile, nil)

	// THEN: Should fail with error (not succeed, not warning)
	require.Error(t, err, "Expected error for missing include file")

	t.Logf("✓ Missing file correctly handled with error: %v", err)

	// BEHAVIORAL INSIGHTS:
	// 1. Error (not warning): Loading fails completely
	//    - Doesn't return partial content
	//    - Doesn't substitute default/empty value
	//    - Fail-fast principle enforced
	//
	// 2. Error information: Likely includes filename
	//    - "nonexistent.yaml" mentioned in error
	//    - Helps user identify which include failed
	//    - Full path or relative path in message
	//
	// 3. Detection point: During include processing
	//    - Not after entire parse (early detection)
	//    - File check before attempting to read
	//    - os.Stat or os.Open reveals missing file
	//
	// 4. No graceful degradation: Strict validation
	//    - Missing include is error, not acceptable
	//    - Configuration must be complete
	//    - Better to fail than run with wrong config
	//
	// 5. Common causes:
	//    - Typo in filename (nonexistant vs nonexistent)
	//    - Wrong directory (expected in subdirectory)
	//    - File moved/deleted in GitOps repo
	//    - Case sensitivity (Windows vs Linux)
	//
	// 6. Use case: Configuration validation
	//    - CI/CD checks for missing includes
	//    - GitOps repo health monitoring
	//    - Pre-deployment validation
	//    - Catch errors before production
}

// ============================================================================
// END OF BEHAVIORAL TEST SUITE
// ============================================================================
// SUMMARY:
// - 4 behavioral contracts documented
// - 4 test scenarios covering all !include behaviors
// - Key behaviors: Simple include, nested includes, circular detection, error handling
// - Integration: Modular configuration management, GitOps composition
//
// MAINTENANCE NOTES:
// - !include is custom YAML tag (not standard YAML)
// - Path resolution relative to working directory (not including file)
// - Circular dependency detection is critical safety feature
// - Missing file errors prevent silent configuration failures
// - Nested includes support arbitrary depth (no explicit limit)
//
// TESTING APPROACH:
// Time-Based BDD characterizes current behavior for:
// - Future refactoring safety
// - Regression prevention
// - Documentation of include semantics
// - Understanding path resolution rules
//
// INCLUDE SYSTEM FEATURES:
// 1. File composition: Modular configuration management
// 2. Recursive resolution: Nested includes to arbitrary depth
// 3. Cycle detection: Prevents infinite recursion
// 4. Error handling: Fail-fast on missing/invalid includes
//
// TYPICAL USAGE PATTERNS:
// - Base + environment overrides (base.yaml, prod.yaml)
// - Shared component libraries (common/database.yaml)
// - Multi-repository configs (cross-repo includes)
// - Layered defaults (system → org → team → app)
//
// ============================================================================
