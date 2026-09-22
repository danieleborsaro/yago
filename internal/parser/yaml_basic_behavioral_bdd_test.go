package parser

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// FILE MIGRATION METADATA
// ============================================================================
// Original file: internal/parser/yaml_test.go
// Migration date: October 13, 2025
// Tests migrated: 4 (LoadString, EnvLookup, YamlLookup, MergeFiles)
// Behavioral contracts created: 4
//
// This file documents the basic YAML parsing and lookup functionality through
// Time-Based BDD. It characterizes YAMLHandler's core operations: loading,
// environment variable lookups, YAML value references, and file merging.
//
// YAML HANDLER OVERVIEW:
// The YAMLHandler provides YAML parsing with GitOps-specific extensions:
// - Standard YAML parsing (scalar, maps, sequences)
// - Environment variable interpolation: [[gitops.getEnvValue(VAR)]]
// - YAML value references: [[gitops.getYamlValue(path.to.value)]]
// - Multi-document merging with deep merge semantics
//
// KEY BEHAVIORS:
// 1. LoadString: Parse YAML from string into map[string]interface{}
// 2. EnvLookup: Resolve environment variable references
// 3. YamlLookup: Resolve internal YAML value references
// 4. MergeFiles: Deep merge multiple YAML documents
//
// GITOPS EXTENSIONS:
// The [[gitops.*]] syntax provides templating capabilities:
// - getEnvValue(VAR): Replaced with os.Getenv(VAR) value
// - getYamlValue(path): Replaced with value at YAML path
// - Evaluated during parsing (not post-processing)
//
// INTEGRATION POINTS:
// - Used by: schema/manager, core/document, repo/git
// - Depends on: yaml.v3 library, custom lookup functions
// - Working directory: Base path for relative file operations
//
// TYPICAL USAGE:
//   handler := NewYAMLHandler("/path/to/workdir")
//   content, err := handler.LoadString(yamlText)
//   // content is map[string]interface{} with lookups resolved
//
// ============================================================================

// GoBehavioralContract defines a behavioral specification for Go test code
// characterization. This structure supports Time-Based BDD by documenting
// what the code currently does, providing a foundation for future refactoring
// and regression prevention.
type GoBehavioralContract struct {
	// Behavior describes what the system currently does (present tense)
	Behavior string

	// CurrentImpl describes the current implementation approach
	CurrentImpl string

	// ExpectedOutcome describes what results the current code produces
	ExpectedOutcome string

	// TestScenario describes the specific test case being validated
	TestScenario string

	// Rationale explains why this behavior exists or is being tested
	Rationale string

	// RegressionRisk identifies potential issues if this behavior changes
	RegressionRisk string
}

// ============================================================================
// BEHAVIORAL CONTRACT 1: LoadString Method
// ============================================================================
// METHOD SIGNATURE: func (h *YAMLHandler) LoadString(yamlContent string) (map[string]interface{}, error)
//
// DESCRIPTION:
// LoadString parses YAML content from a string and returns a map representation.
// It handles standard YAML structures (scalars, maps, sequences) and converts
// them into Go's map[string]interface{} format for dynamic access.
//
// BEHAVIOR ANALYSIS:
// The method uses yaml.v3 library to parse YAML, performs type conversions,
// and resolves any GitOps-specific lookup functions embedded in the YAML.
// It preserves nested structure and type information.
//
// YAML STRUCTURE SUPPORT:
// - Scalars: strings, integers, floats, booleans, null
// - Maps: Converted to map[string]interface{}
// - Sequences: Converted to []interface{}
// - Nested structures: Recursively parsed
//
// TYPE CONVERSIONS:
//   YAML           →  Go Type
//   -------------  →  -----------------
//   string         →  string
//   integer        →  int (or int64)
//   float          →  float64
//   boolean        →  bool
//   null           →  nil
//   mapping        →  map[string]interface{}
//   sequence       →  []interface{}
//
// ERROR CONDITIONS:
// - Invalid YAML syntax (unclosed quotes, indentation errors)
// - Malformed structures (duplicate keys in some contexts)
// - Lookup function failures (missing env vars, invalid paths)
//
// DESIGN RATIONALE:
// Using map[string]interface{} provides maximum flexibility for dynamic
// YAML processing where structure isn't known at compile time. This is
// essential for schema-agnostic operations.
//
// ============================================================================

func TestYAMLHandler_LoadString_BehavioralBDD(t *testing.T) {
	// Contract: LoadString parses YAML into Go map structure
	contract := GoBehavioralContract{
		Behavior:        "LoadString parses YAML string into map[string]interface{}, preserving nested structure and type information",
		CurrentImpl:     "Uses yaml.v3 unmarshaling into map[string]interface{}, converts YAML types to Go equivalents, resolves GitOps lookup functions during parse",
		ExpectedOutcome: "Returns map with correct structure, types preserved (string, int, bool, nested maps), lookups not evaluated in this basic case",
		TestScenario:    "Parse YAML with scalars (string, float), nested map structure (config.debug, config.port), verify type preservation and structure",
		Rationale:       "YAML parsing is foundation for all configuration loading. Type preservation critical for schema validation. Nested map support required for complex configurations",
		RegressionRisk:  "HIGH - Type conversion changes would break schema validation. Structure changes would break property access. Used throughout codebase for config loading",
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

	// SETUP: Create YAML handler with working directory
	handler := NewYAMLHandler("/tmp")

	// GIVEN: YAML content with various types and nested structure
	yamlContent := `---
name: test
version: 1.0.0
config:
  debug: true
  port: 8080`

	// WHEN: LoadString is called to parse the YAML
	content, err := handler.LoadString(yamlContent)

	// THEN: Should parse successfully
	require.NoError(t, err, "LoadString should succeed for valid YAML")
	require.NotNil(t, content, "Returned content should not be nil")

	// VERIFICATION 1: Top-level string scalar
	// Behavior: String values preserved as Go strings
	assert.Equal(t, "test", content["name"], "String scalar 'name' should be 'test'")

	// VERIFICATION 2: Top-level version string (could be number)
	// Behavior: Quoted numbers treated as strings, preserves semantic meaning
	assert.Equal(t, "1.0.0", content["version"], "Version string should be '1.0.0'")

	// VERIFICATION 3: Nested map structure
	// Behavior: YAML maps converted to map[string]interface{}
	config, ok := content["config"].(map[string]interface{})
	require.True(t, ok, "config should be a map[string]interface{}, got %T", content["config"])

	// VERIFICATION 4: Boolean type preservation
	// Behavior: YAML booleans converted to Go bool
	assert.Equal(t, true, config["debug"], "Boolean 'debug' should be true")

	// VERIFICATION 5: Integer type preservation
	// Behavior: YAML integers converted to Go int
	assert.Equal(t, 8080, config["port"], "Integer 'port' should be 8080")

	// BEHAVIORAL INSIGHTS:
	// 1. Type preservation: YAML types map directly to Go types
	//    - Critical for schema validation (expects specific types)
	//    - Boolean true != string "true" (type matters)
	//
	// 2. Nested structure: Maps within maps preserved
	//    - Enables property path access (config.debug)
	//    - Depth unlimited (recursive parsing)
	//
	// 3. No lookup resolution in this test: Pure YAML parsing
	//    - Lookups would be tested separately
	//    - This isolates parsing from lookup logic
	//
	// 4. String vs number: Version "1.0.0" is string (quoted in YAML)
	//    - Semantic distinction: version numbers vs arithmetic values
	//    - YAML's type inference vs explicit quoting
	//
	// 5. Error handling: Invalid YAML would return error
	//    - Callers must check error before accessing content
	//    - No panic on parse failure (graceful degradation)
}

// ============================================================================
// BEHAVIORAL CONTRACT 2: Environment Variable Lookup
// ============================================================================
// GITOPS EXTENSION: [[gitops.getEnvValue(VAR_NAME)]]
//
// DESCRIPTION:
// The getEnvValue lookup function resolves environment variable references
// embedded in YAML content. Syntax [[gitops.getEnvValue(VAR)]] is replaced
// with os.Getenv(VAR) value during parsing.
//
// BEHAVIOR ANALYSIS:
// Lookup functions are evaluated during YAML parsing (not post-processing).
// The raw YAML contains [[gitops.*]] syntax, which is detected, parsed, and
// replaced with actual values before returning the content map.
//
// SYNTAX:
//   key: "[[gitops.getEnvValue(VAR_NAME)]]"
//
// Becomes:
//   key: "<value of $VAR_NAME>"
//
// USE CASES:
// - Inject secrets from environment (API keys, passwords)
// - Environment-specific configuration (dev/staging/prod)
// - CI/CD integration (build numbers, git refs)
// - Runtime configuration (port numbers, hostnames)
//
// MISSING VARIABLE BEHAVIOR:
// If environment variable doesn't exist:
// - os.Getenv returns empty string ""
// - No error raised (follows Go os.Getenv semantics)
// - Caller must validate required variables
//
// SECURITY CONSIDERATIONS:
// - Environment variables visible to process have unrestricted access
// - No sandboxing or permission checks
// - Secrets should use proper secret management (not env vars ideally)
//
// ============================================================================

func TestYAMLHandler_EnvLookup_BehavioralBDD(t *testing.T) {
	// Contract: getEnvValue lookup resolves environment variables
	contract := GoBehavioralContract{
		Behavior:        "LoadString resolves [[gitops.getEnvValue(VAR)]] syntax by replacing with os.Getenv(VAR) value during YAML parsing",
		CurrentImpl:     "Detects [[gitops.getEnvValue(...)]] pattern, extracts variable name, calls os.Getenv, replaces entire [[...]] with env value",
		ExpectedOutcome: "YAML field contains environment variable value (not the [[...]] syntax), empty string if variable undefined",
		TestScenario:    "Set TEST_ENV_VAR='test_env_value', parse YAML with [[gitops.getEnvValue(TEST_ENV_VAR)]], verify value is 'test_env_value'",
		Rationale:       "Environment variable injection enables runtime configuration, secrets management, and environment-specific deployments without hard-coding values",
		RegressionRisk:  "CRITICAL - Syntax change breaks all environment-based configurations. Empty string default for missing vars is assumed by callers. Used for secrets/credentials",
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

	// SETUP: Create YAML handler
	handler := NewYAMLHandler("/tmp")

	// GIVEN: Environment variable is set
	testValue := "test_env_value"
	os.Setenv("TEST_ENV_VAR", testValue)
	defer os.Unsetenv("TEST_ENV_VAR") // Cleanup after test

	// GIVEN: YAML content with environment variable lookup
	yamlContent := `---
name: test
env_value: "[[gitops.getEnvValue(TEST_ENV_VAR)]]"`

	// WHEN: LoadString parses YAML with lookup function
	content, err := handler.LoadString(yamlContent)

	// THEN: Should parse successfully and resolve environment variable
	require.NoError(t, err, "LoadString should succeed with env lookup")
	require.NotNil(t, content, "Returned content should not be nil")

	// VERIFICATION: env_value contains actual environment variable value
	// Behavior: [[gitops.getEnvValue(...)]] replaced with os.Getenv result
	assert.Equal(t, testValue, content["env_value"],
		"env_value should contain actual environment variable value, not [[...]] syntax")

	// BEHAVIORAL INSIGHTS:
	// 1. Lookup timing: Resolved during parsing (not lazy/deferred)
	//    - Content map contains final values
	//    - No trace of [[gitops.*]] syntax in result
	//    - Cannot distinguish env-sourced vs hard-coded values post-parse
	//
	// 2. Missing variable handling: os.Getenv returns ""
	//    - No error raised for undefined variables
	//    - Callers must validate if variable is required
	//    - Follows Go standard library semantics
	//
	// 3. Security: Full environment access
	//    - Any variable accessible to process can be read
	//    - No filtering or validation
	//    - Risk: accidental exposure of sensitive vars
	//
	// 4. Use case: Runtime configuration
	//    - Enables same YAML with different env values
	//    - CI/CD pipelines inject variables
	//    - Docker/Kubernetes ConfigMaps/Secrets
	//
	// 5. Syntax requirement: Must be quoted string in YAML
	//    - env_value: [[...]]  (invalid YAML - treated as sequence)
	//    - env_value: "[[...]]" (valid - string with lookup)
}

// ============================================================================
// BEHAVIORAL CONTRACT 3: YAML Value Reference Lookup
// ============================================================================
// GITOPS EXTENSION: [[gitops.getYamlValue(path.to.value)]]
//
// DESCRIPTION:
// The getYamlValue lookup function resolves references to other values within
// the same YAML document. It enables value reuse and computed configurations
// by referencing values using dot-notation paths.
//
// BEHAVIOR ANALYSIS:
// During parsing, [[gitops.getYamlValue(path)]] syntax is detected, the path
// is evaluated against the partially-parsed YAML structure, and the referenced
// value is retrieved and substituted.
//
// SYNTAX:
//   source:
//     value: "hello world"
//   target: "[[gitops.getYamlValue(source.value)]]"
//
// Becomes:
//   target: "hello world"
//
// PATH RESOLUTION:
// - Dot notation: source.nested.value
// - Traverses nested maps
// - Must reference already-parsed values (order matters)
//
// USE CASES:
// - DRY (Don't Repeat Yourself): Define once, reference many times
// - Computed values: Reference and transform other values
// - Consistency: Single source of truth for shared values
// - Default value patterns: Base config + overrides
//
// ORDER DEPENDENCY:
// Referenced value must appear before reference in YAML:
// - YAML parsed top-to-bottom
// - Forward references would fail (value not yet available)
// - Multi-pass parsing not implemented
//
// ERROR CONDITIONS:
// - Path doesn't exist (undefined key)
// - Path is ambiguous (multiple matches)
// - Circular references (A→B→A)
// - Invalid path syntax
//
// ============================================================================

func TestYAMLHandler_YamlLookup_BehavioralBDD(t *testing.T) {
	// Contract: getYamlValue lookup resolves internal YAML references
	contract := GoBehavioralContract{
		Behavior:        "LoadString resolves [[gitops.getYamlValue(path)]] by navigating dot-notation path in YAML structure and substituting referenced value",
		CurrentImpl:     "Detects [[gitops.getYamlValue(...)]] pattern, parses dot path, traverses parsed YAML map, retrieves value, replaces [[...]] with actual value",
		ExpectedOutcome: "Target field contains value from source path (not [[...]] syntax), order-dependent (source must be parsed first)",
		TestScenario:    "Define source.value='hello world', reference with [[gitops.getYamlValue(source.value)]], verify target contains 'hello world'",
		Rationale:       "Value references enable DRY configurations, single source of truth, and computed values. Reduces duplication and maintains consistency across YAML",
		RegressionRisk:  "HIGH - Path syntax change breaks all internal references. Order dependency is implicit contract. Circular reference detection critical for stability",
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

	// SETUP: Create YAML handler
	handler := NewYAMLHandler("/tmp")

	// GIVEN: YAML with source value and reference to it
	yamlContent := `---
source:
  value: "hello world"
target: "[[gitops.getYamlValue(source.value)]]"`

	// WHEN: LoadString parses YAML with value reference
	content, err := handler.LoadString(yamlContent)

	// THEN: Should parse successfully and resolve reference
	require.NoError(t, err, "LoadString should succeed with YAML lookup")
	require.NotNil(t, content, "Returned content should not be nil")

	// VERIFICATION: target contains value from source.value
	// Behavior: [[gitops.getYamlValue(...)]] replaced with actual referenced value
	assert.Equal(t, "hello world", content["target"],
		"target should contain value from source.value, not [[...]] syntax")

	// BEHAVIORAL INSIGHTS:
	// 1. Order dependency: source.value must appear before target
	//    - YAML parsed sequentially (top-to-bottom)
	//    - Forward references would fail (value not yet available)
	//    - Documented limitation of current implementation
	//
	// 2. Dot-notation path: Standard property access syntax
	//    - source.value navigates: content["source"]["value"]
	//    - Nested structures supported (arbitrary depth)
	//    - Same syntax as property paths in schema
	//
	// 3. Value reuse: Same value referenced multiple times
	//    - Update source → all references updated
	//    - Single source of truth pattern
	//    - Reduces configuration duplication
	//
	// 4. Type preservation: Referenced value type preserved
	//    - If source is int, target becomes int
	//    - Not converted to string (direct value copy)
	//    - Important for schema validation
	//
	// 5. Circular reference risk: A→B→A would cause infinite loop
	//    - Implementation must detect/prevent
	//    - Test suite should verify protection
	//    - Not tested here (basic happy path)
	//
	// 6. Missing path behavior: TBD (not tested here)
	//    - Error? Empty string? Preserve [[...]] syntax?
	//    - Error handling contract unclear
}

// ============================================================================
// BEHAVIORAL CONTRACT 4: Multi-Document Merging
// ============================================================================
// METHOD SIGNATURE: func (h *YAMLHandler) LoadBuffers(buffers []string) (map[string]interface{}, error)
//
// DESCRIPTION:
// LoadBuffers merges multiple YAML documents into a single map structure.
// It implements deep merge semantics where nested maps are merged recursively
// rather than replaced entirely.
//
// BEHAVIOR ANALYSIS:
// Each YAML document is parsed independently, then merged left-to-right.
// Maps are deep-merged (nested keys combined), while scalars and sequences
// are replaced by later values.
//
// MERGE SEMANTICS:
//   Document 1:          Document 2:          Result:
//   config:              config:              config:
//     debug: true          port: 8080           debug: true
//   name: test           version: 1.0           port: 8080
//                                             name: test
//                                             version: 1.0
//
// MERGE RULES:
// 1. Map + Map → Deep merge (combine keys, recurse on conflicts)
// 2. Scalar + Scalar → Second value wins (replacement)
// 3. Map + Scalar → Second value wins (type change, replacement)
// 4. Sequence + Sequence → Second value wins (no merge, replacement)
//
// USE CASES:
// - Base configuration + environment overrides
// - Multi-file configuration (config.yaml + secrets.yaml)
// - Include files with partial configs
// - Layered defaults (system → user → project)
//
// MERGE ORDER:
// Left-to-right: LoadBuffers([base, override1, override2])
// - base parsed first
// - override1 merged into base
// - override2 merged into result
// - Later documents win on conflicts
//
// DESIGN RATIONALE:
// Deep merge allows modular configuration without full document replacement.
// Enables configuration composition patterns common in GitOps.
//
// ============================================================================

func TestYAMLHandler_MergeFiles_BehavioralBDD(t *testing.T) {
	// Contract: LoadBuffers deep-merges multiple YAML documents
	contract := GoBehavioralContract{
		Behavior:        "LoadBuffers parses multiple YAML strings and deep-merges them into single map, combining nested maps recursively while replacing scalars",
		CurrentImpl:     "Parses each buffer independently, merges left-to-right with deep merge for maps (recursive key combination), replacement for scalars and sequences",
		ExpectedOutcome: "Final map contains all keys from all documents, nested maps merged (not replaced), later values win on scalar conflicts",
		TestScenario:    "Merge 2 docs: doc1 has name+config.debug, doc2 has version+config.port, verify all 4 values present in result with merged config map",
		Rationale:       "Deep merge enables modular configuration composition. Base configs with overrides. Multi-file configs. Layered defaults. Essential for GitOps patterns",
		RegressionRisk:  "CRITICAL - Merge semantics change would break all multi-file configs. Deep vs shallow merge distinction critical. Order dependency is contract",
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

	// SETUP: Create YAML handler
	handler := NewYAMLHandler("/tmp")

	// GIVEN: Two YAML documents with overlapping and distinct content
	content1 := `---
name: test
config:
  debug: true`

	content2 := `---
config:
  port: 8080
version: 1.0.0`

	// WHEN: LoadBuffers merges both documents
	content, err := handler.LoadBuffers([]string{content1, content2})

	// THEN: Should merge successfully
	require.NoError(t, err, "LoadBuffers should succeed for valid YAML documents")
	require.NotNil(t, content, "Merged content should not be nil")

	// VERIFICATION 1: Unique keys from document 1 preserved
	// Behavior: Keys only in doc1 appear in result
	assert.Equal(t, "test", content["name"], "name from doc1 should be preserved")

	// VERIFICATION 2: Unique keys from document 2 added
	// Behavior: Keys only in doc2 appear in result
	assert.Equal(t, "1.0.0", content["version"], "version from doc2 should be added")

	// VERIFICATION 3: Nested map structure merged (not replaced)
	// Behavior: config map contains keys from both documents
	config, ok := content["config"].(map[string]interface{})
	require.True(t, ok, "config should be a merged map, got %T", content["config"])

	// VERIFICATION 4: config.debug from document 1 preserved
	// Behavior: Nested keys from doc1 preserved during merge
	assert.Equal(t, true, config["debug"], "config.debug from doc1 should be preserved")

	// VERIFICATION 5: config.port from document 2 added
	// Behavior: Nested keys from doc2 added during merge
	assert.Equal(t, 8080, config["port"], "config.port from doc2 should be added")

	// BEHAVIORAL INSIGHTS:
	// 1. Deep merge: Nested maps combined (not replaced)
	//    - Shallow merge: config from doc2 would replace config from doc1
	//    - Deep merge: config.debug AND config.port both present
	//    - Critical distinction for modular configs
	//
	// 2. Merge order: Left-to-right processing
	//    - LoadBuffers([A, B, C]) → A merged first, then B, then C
	//    - Later documents override earlier (on conflicts)
	//    - Order matters: LoadBuffers([B, A]) gives different result
	//
	// 3. Type conflicts: What if config is scalar in doc2?
	//    - Scalar would replace entire map (type change)
	//    - Not tested here (complex edge case)
	//    - Potential data loss if types mismatch
	//
	// 4. Use case: Base + overrides
	//    - base.yaml: Full defaults
	//    - prod.yaml: Production overrides (partial)
	//    - Result: base config + prod changes
	//    - Common GitOps pattern
	//
	// 5. Sequence merge: Arrays replaced (not merged)
	//    - lists: [1,2] + lists: [3,4] → lists: [3,4]
	//    - No array concatenation or element merge
	//    - Documented limitation
	//
	// 6. Performance: O(n*m) where n=docs, m=avg keys
	//    - Each doc parsed independently (parallelizable?)
	//    - Merge is sequential (order dependency)
	//    - Not optimized for huge doc counts
}

// ============================================================================
// END OF BEHAVIORAL TEST SUITE
// ============================================================================
// SUMMARY:
// - 4 behavioral contracts documented
// - 4 top-level test scenarios
// - Key behaviors: YAML parsing, env lookups, value references, multi-doc merge
// - Integration: Core config loading for all GitOps operations
//
// MAINTENANCE NOTES:
// - Type preservation critical for schema validation
// - Lookup syntax [[gitops.*]] is public API (breaking change risk)
// - Deep merge semantics essential for config composition
// - Order dependencies: both value references and merge order
//
// TESTING APPROACH:
// Time-Based BDD characterizes current behavior for:
// - Future refactoring safety
// - Regression prevention
// - Documentation of implicit contracts
// - Understanding caller expectations
//
// GITOPS EXTENSIONS SUMMARY:
// 1. getEnvValue(VAR): os.Getenv(VAR) injection
// 2. getYamlValue(path): Internal value references
// Both resolved during parsing (not post-processing)
//
// ============================================================================
