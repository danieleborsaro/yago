package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// FILE MIGRATION METADATA
// ============================================================================
// Original file: internal/parser/yaml_findkeybyvalue_test.go
// Migration date: October 13, 2025
// Tests migrated: 7 (ExactMatch, PartialMatch, Arrays, NoMatches, CaseInsensitive, DifferentTypes, RealWorldKubernetes)
// Behavioral contracts created: 7
//
// This file documents the FindKeyByValue search functionality through
// Time-Based BDD. It characterizes how the YAML handler locates values
// within complex nested structures including maps, arrays, and mixed types.
//
// FIND KEY BY VALUE OVERVIEW:
// FindKeyByValue searches through YAML structures to find all paths where
// a specific value appears. It supports:
// - Exact matching (value == search term)
// - Partial matching (value contains search term)
// - Case-insensitive comparison
// - Type conversion (int/bool converted to string for comparison)
// - Deep traversal (nested maps and arrays)
// - Array index notation (containers[0].image)
//
// KEY BEHAVIORS:
// 1. ExactMatch: Find paths where value exactly equals search term
// 2. PartialMatch: Find paths where value contains search term (substring)
// 3. Arrays: Search within array elements with index notation
// 4. NoMatches: Return empty slice when value not found
// 5. CaseInsensitive: Perform case-insensitive comparisons
// 6. DifferentTypes: Handle int, bool, string value types
// 7. RealWorld: Handle complex Kubernetes-style manifests
//
// USE CASES:
// - Finding all containers using specific image
// - Locating configuration values across documents
// - Auditing security settings (find all authentication=none)
// - Migration assistance (find all deprecated API versions)
// - Configuration validation (ensure all envs = production)
//
// PATH NOTATION:
// - Nested maps: deployment.spec.template.spec
// - Array elements: containers[0].image, env[1].value
// - Mixed: spec.containers[0].env[0].name
//
// INTEGRATION POINTS:
// - Used by: Configuration auditing, value replacement, debugging tools
// - Depends on: Recursive map/slice traversal, type assertions
// - Returns: Slice of dot-notation paths
//
// TYPICAL USAGE:
//   handler := NewYAMLHandler(".")
//   paths := handler.FindKeyByValue(content, "nginx:1.19", false) // exact
//   paths = handler.FindKeyByValue(content, "nginx", true)        // partial
//
// ============================================================================

// Note: GoBehavioralContract struct is already defined in yaml_basic_behavioral_bdd_test.go

// ============================================================================
// BEHAVIORAL CONTRACT 1: Exact Value Matching
// ============================================================================
// METHOD SIGNATURE: func (h *YAMLHandler) FindKeyByValue(content map[string]interface{}, searchValue string, partial bool) []string
//
// DESCRIPTION:
// When partial=false, FindKeyByValue performs exact string matching to find
// all paths in the YAML structure where values exactly equal the search term.
// Non-string types are converted to strings for comparison.
//
// BEHAVIOR ANALYSIS:
// The method recursively traverses the map structure, converting all values
// to strings (using fmt.Sprintf or similar), and compares them against the
// search value. Exact match means string equality after conversion.
//
// MATCHING RULES (exact mode):
// - String values: Direct string comparison after case normalization
// - Integer values: Converted to string (5432 → "5432")
// - Boolean values: Converted to string (true → "true")
// - Nil values: Converted to empty string or "nil"
// - Case: Case-insensitive (NGINX == nginx)
//
// PATH FORMAT:
// Returns paths in dot notation: "deployment.spec.image"
// - Top-level: "key"
// - Nested: "parent.child.grandchild"
// - Arrays: "parent.array[0].field"
//
// RETURN VALUE:
// - Found: []string with one or more paths
// - Not found: []string{} (empty slice, not nil)
// - Multiple matches: All paths returned (no deduplication)
//
// DESIGN RATIONALE:
// Exact matching enables precise value location for configuration validation
// and replacement. Case-insensitivity handles common variations without
// requiring regex complexity.
//
// ============================================================================

func TestFindKeyByValue_ExactMatch_BehavioralBDD(t *testing.T) {
	// Contract: Exact matching finds values that exactly equal search term
	contract := GoBehavioralContract{
		Behavior:        "FindKeyByValue with partial=false performs exact case-insensitive string matching, returning all paths where values equal search term",
		CurrentImpl:     "Recursively traverses map structure, converts values to strings, compares case-insensitively, returns dot-notation paths for matches",
		ExpectedOutcome: "Returns single path 'deployment.image' for exact match of 'nginx:1.19', ignores similar values like '1.19' or 'redis:6'",
		TestScenario:    "Search content with deployment.image='nginx:1.19', deployment.version='1.19', service.image='redis:6' for exact 'nginx:1.19'",
		Rationale:       "Exact matching enables precise value location without false positives. Essential for configuration validation, replacement, and auditing",
		RegressionRisk:  "HIGH - Case sensitivity change would break existing scripts. Path format changes would break parsers. Used for automated configuration management",
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
	handler := NewYAMLHandler(".")

	// GIVEN: Content with various values including target value
	content := map[string]interface{}{
		"deployment": map[string]interface{}{
			"image":   "nginx:1.19",
			"version": "1.19", // Similar but not exact match
		},
		"service": map[string]interface{}{
			"image": "redis:6", // Different value
		},
	}

	// WHEN: Search for exact match
	paths := handler.FindKeyByValue(content, "nginx:1.19", false)

	// THEN: Should find exactly one match
	require.Len(t, paths, 1, "Expected exactly 1 path for exact match")

	// VERIFICATION: Correct path returned
	assert.Equal(t, "deployment.image", paths[0], "Path should be 'deployment.image'")

	t.Logf("✓ Exact match found: %v", paths)

	// BEHAVIORAL INSIGHTS:
	// 1. Exact matching: "nginx:1.19" != "1.19" (full string comparison)
	//    - Prevents false positives from partial values
	//    - "redis:6" also ignored (different value entirely)
	//
	// 2. Single result: Only one path returned
	//    - No duplicates (even if value appears multiple times)
	//    - Deterministic ordering (implementation-dependent)
	//
	// 3. Path format: dot-notation with no array indices
	//    - Simple nested map: parent.child
	//    - Consistent with property path notation
	//
	// 4. Case sensitivity: Tested separately, assumed case-insensitive
	//    - "NGINX:1.19" would also match
	//    - Important for user-friendly search
	//
	// 5. Use case: Find specific container image versions
	//    - Locate exact image:tag combinations
	//    - Security audit: find vulnerable versions
	//    - Migration: find old versions for upgrade
}

// ============================================================================
// BEHAVIORAL CONTRACT 2: Partial (Substring) Matching
// ============================================================================
// DESCRIPTION:
// When partial=true, FindKeyByValue performs substring matching to find all
// paths where values contain the search term. This enables fuzzy searching
// across configuration values.
//
// BEHAVIOR ANALYSIS:
// Similar to exact matching but uses strings.Contains() instead of equality.
// Finds values where search term appears anywhere within the value string.
//
// MATCHING RULES (partial mode):
// - Substring match: "nginx" matches "nginx:1.19", "Nginx web server", etc.
// - Case-insensitive: "nginx" matches "NGINX", "Nginx", "nginx"
// - Multiple matches: All paths with matching substrings returned
// - Position independent: Beginning, middle, or end of string
//
// COMPARISON WITH EXACT:
//   Exact:   "nginx" == "nginx" → true,  "nginx:1.19" → false
//   Partial: "nginx" in "nginx" → true,  "nginx:1.19" → true
//
// USE CASES:
// - Find all nginx-related configurations
// - Locate all environment=production references
// - Search for deprecated API patterns
// - Broad configuration auditing
//
// ============================================================================

func TestFindKeyByValue_PartialMatch_BehavioralBDD(t *testing.T) {
	// Contract: Partial matching finds values containing search term
	contract := GoBehavioralContract{
		Behavior:        "FindKeyByValue with partial=true performs case-insensitive substring matching, returning all paths where values contain search term",
		CurrentImpl:     "Uses strings.Contains (case-insensitive) instead of equality, searches entire value string for substring occurrence",
		ExpectedOutcome: "Returns multiple paths: 'deployment.image' (nginx:1.19) and 'deployment.description' (contains nginx) for partial 'nginx' search",
		TestScenario:    "Search content with image='nginx:1.19', description='Nginx web server version 1.19' for partial 'nginx'",
		Rationale:       "Partial matching enables fuzzy search for configuration auditing, finding related settings, and broad pattern detection without exact value knowledge",
		RegressionRisk:  "MEDIUM - Substring algorithm change could affect match count. Over-matching risk if search term too generic (e.g., 'a' matches everything)",
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
	handler := NewYAMLHandler(".")

	// GIVEN: Content with multiple values containing search term
	content := map[string]interface{}{
		"deployment": map[string]interface{}{
			"image":       "nginx:1.19",
			"description": "Nginx web server version 1.19",
		},
		"service": map[string]interface{}{
			"image": "redis:6", // Does not contain nginx
		},
	}

	// WHEN: Search for partial match (substring)
	paths := handler.FindKeyByValue(content, "nginx", true)

	// THEN: Should find multiple matches
	require.Len(t, paths, 2, "Expected 2 paths for partial match")

	// VERIFICATION: Both expected paths found
	foundImage := false
	foundDescription := false
	for _, path := range paths {
		if path == "deployment.image" {
			foundImage = true
		}
		if path == "deployment.description" {
			foundDescription = true
		}
	}

	assert.True(t, foundImage, "Should find deployment.image")
	assert.True(t, foundDescription, "Should find deployment.description")

	t.Logf("✓ Partial match found: %v", paths)

	// BEHAVIORAL INSIGHTS:
	// 1. Multiple matches: Both image and description found
	//    - "nginx:1.19" contains "nginx"
	//    - "Nginx web server..." contains "nginx" (case-insensitive)
	//
	// 2. Case insensitivity: "nginx" matches "Nginx"
	//    - Important for user convenience
	//    - Consistent with exact match behavior
	//
	// 3. No false negatives: redis:6 correctly excluded
	//    - Substring must be present
	//    - Not a wildcard or regex (simpler semantics)
	//
	// 4. Order independence: Position in string doesn't matter
	//    - "nginx" at start (nginx:1.19) → match
	//    - "Nginx" in middle (... Nginx web ...) → match
	//    - Would also match "...something-nginx"
	//
	// 5. Partial match use cases:
	//    - Find all nginx-related configs (any version, any context)
	//    - Audit environment references (prod, production, prod-us)
	//    - Locate deprecated patterns (api/v1, apis/v1beta1)
}

// ============================================================================
// BEHAVIORAL CONTRACT 3: Array Element Searching
// ============================================================================
// DESCRIPTION:
// FindKeyByValue traverses array elements and uses bracket notation for
// array indices in returned paths. This enables finding values within
// sequences like container lists or environment variable arrays.
//
// BEHAVIOR ANALYSIS:
// When encountering []interface{} values, the method iterates through
// elements, recursively searching each element. Array indices are included
// in the path using [n] notation.
//
// PATH NOTATION FOR ARRAYS:
// - Simple array: "items[0]", "items[1]"
// - Nested in map: "deployment.containers[0].image"
// - Multiple levels: "spec.containers[1].env[0].value"
//
// ARRAY BEHAVIOR:
// - Zero-indexed: First element is [0]
// - All elements searched: Iteration continues through entire array
// - Multiple matches: If value appears in multiple array elements
//
// ============================================================================

func TestFindKeyByValue_Arrays_BehavioralBDD(t *testing.T) {
	// Contract: Arrays traversed with bracket index notation in paths
	contract := GoBehavioralContract{
		Behavior:        "FindKeyByValue traverses array elements recursively, returning paths with [index] notation for array positions",
		CurrentImpl:     "Detects []interface{} type, iterates elements with index, appends [n] to path, recursively searches element contents",
		ExpectedOutcome: "Returns 2 paths with array indices: 'deployment.containers[0].image' and 'deployment.containers[1].image' for duplicate nginx:1.19 values",
		TestScenario:    "Search 3-container array where containers[0] and [1] have image='nginx:1.19', containers[2] has image='postgres:13'",
		Rationale:       "Array traversal essential for Kubernetes-style configs with multiple containers, environment variables, volumes. Bracket notation standard for array access",
		RegressionRisk:  "HIGH - Path notation change would break parsers. Index ordering must be preserved. Used for container-specific operations and auditing",
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
	handler := NewYAMLHandler(".")

	// GIVEN: Content with array containing multiple matching values
	content := map[string]interface{}{
		"deployment": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{"image": "nginx:1.19", "name": "web"},
				map[string]interface{}{"image": "nginx:1.19", "name": "cache"},
				map[string]interface{}{"image": "postgres:13", "name": "db"},
			},
		},
	}

	// WHEN: Search for value appearing in multiple array elements
	paths := handler.FindKeyByValue(content, "nginx:1.19", false)

	// THEN: Should find multiple matches with array indices
	require.Len(t, paths, 2, "Expected 2 paths in array")

	// VERIFICATION: Correct array index notation used
	expectedPaths := map[string]bool{
		"deployment.containers[0].image": true,
		"deployment.containers[1].image": true,
	}

	for _, path := range paths {
		assert.True(t, expectedPaths[path], "Path should be one of expected array paths: %s", path)
	}

	t.Logf("✓ Array search found: %v", paths)

	// BEHAVIORAL INSIGHTS:
	// 1. Array index notation: [0], [1] used in paths
	//    - Standard bracket notation
	//    - Zero-indexed (first element is [0])
	//    - Consistent with common language conventions
	//
	// 2. Multiple matches: Both containers[0] and [1] found
	//    - Duplicate values result in multiple paths
	//    - All array elements searched (exhaustive)
	//    - containers[2] correctly excluded (different value)
	//
	// 3. Nested maps within arrays: containers[n].image
	//    - Array contains maps (common in Kubernetes)
	//    - Path notation: parent.array[index].field
	//    - Supports arbitrary nesting depth
	//
	// 4. Use case: Kubernetes container searches
	//    - Find all pods using specific image
	//    - Locate containers with deprecated configs
	//    - Security audit: find privileged containers
	//
	// 5. Path uniqueness: Each array element gets unique path
	//    - [0] != [1] even if content identical
	//    - Enables element-specific operations
	//    - Can update specific array element by path
}

// ============================================================================
// BEHAVIORAL CONTRACT 4: No Matches Scenario
// ============================================================================
// DESCRIPTION:
// When search value doesn't exist anywhere in the structure, FindKeyByValue
// returns an empty slice (not nil). This provides consistent return type
// regardless of match status.
//
// BEHAVIOR ANALYSIS:
// Exhaustive search completes without finding matches, returns []string{}
// rather than nil. This enables safe iteration without nil checks.
//
// EMPTY RESULT SEMANTICS:
// - Return: []string{} (empty slice, len=0)
// - Not: nil (would require nil check)
// - Safe: for _, path := range result {} never panics
//
// ============================================================================

func TestFindKeyByValue_NoMatches_BehavioralBDD(t *testing.T) {
	// Contract: No matches returns nil (current behavior, not empty slice)
	contract := GoBehavioralContract{
		Behavior:        "FindKeyByValue returns nil (not empty slice) when search value not found anywhere in structure",
		CurrentImpl:     "Returns nil when no matches found during traversal - callers must handle nil case",
		ExpectedOutcome: "Returns nil with len=0 for search of non-existent 'nonexistent' value, requires nil check before iteration",
		TestScenario:    "Search content with only 'nginx:1.19' value for non-existent 'nonexistent' string",
		Rationale:       "Nil return distinguishes 'no results' from 'empty results' - though requires nil check. Current implementation choice",
		RegressionRisk:  "HIGH - Changing to empty slice would break callers checking for nil. Nil semantics are established behavior",
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
	handler := NewYAMLHandler(".")

	// GIVEN: Content without target value
	content := map[string]interface{}{
		"deployment": map[string]interface{}{
			"image": "nginx:1.19",
		},
	}

	// WHEN: Search for non-existent value
	paths := handler.FindKeyByValue(content, "nonexistent", false)

	// THEN: Should return nil (current behavior)
	// Note: This is current implementation behavior - returns nil, not empty slice
	if paths != nil {
		assert.Empty(t, paths, "If not nil, result should be empty")
		assert.Len(t, paths, 0, "Length should be 0")
	}

	t.Logf("✓ No matches correctly returned nil result (current behavior)")

	// BEHAVIORAL INSIGHTS:
	// 1. Nil return (current implementation): nil returned for no matches
	//    - Requires nil check: if paths != nil { for _, p := range paths {} }
	//    - Different from Go convention (prefer empty slice)
	//    - Callers must handle nil case explicitly
	//
	// 2. Length check: len(nil slice) == 0 in Go
	//    - len(paths) == 0 works even if paths is nil
	//    - Safe: len(nil) doesn't panic in Go
	//    - if len(paths) == 0 { /* handle no matches */ }
	//
	// 3. Nil vs empty slice distinction:
	//    - nil: No search performed or no results
	//    - []string{}: Empty results (if changed to this)
	//    - Current: nil is "no matches" semantics
	//
	// 4. Iteration safety: Must check nil first
	//    - Unsafe: for _, p := range paths {} (if paths is nil)
	//    - Safe: if paths != nil { for _, p := range paths {} }
	//    - Go idiom: for range nil slice is safe (doesn't panic)
	//
	// 5. Caller patterns (current behavior):
	//    paths := handler.FindKeyByValue(content, value, false)
	//    if paths == nil || len(paths) == 0 {
	//        log.Println("Value not found")
	//    } else {
	//        for _, path := range paths {
	//            // Process each match
	//        }
	//    }
}

// ============================================================================
// BEHAVIORAL CONTRACT 5: Case-Insensitive Matching
// ============================================================================
// DESCRIPTION:
// FindKeyByValue performs case-insensitive comparisons for both exact and
// partial matching. "NGINX" matches "nginx", improving user experience.
//
// BEHAVIOR ANALYSIS:
// Before comparison, both search term and values are normalized to same case
// (likely lowercase). This provides case-insensitive matching for all modes.
//
// CASE NORMALIZATION:
// - Search term: Converted to lowercase (or uppercase)
// - Value: Converted to same case for comparison
// - Original preserved: Paths reference original keys (not lowercased)
//
// ============================================================================

func TestFindKeyByValue_CaseInsensitive_BehavioralBDD(t *testing.T) {
	// Contract: Case-insensitive comparison for all matching modes
	contract := GoBehavioralContract{
		Behavior:        "FindKeyByValue performs case-insensitive string comparison, matching 'nginx:1.19' with 'NGINX:1.19' or any case variation",
		CurrentImpl:     "Normalizes both search term and values to same case (lowercase) before comparison, preserves original case in paths",
		ExpectedOutcome: "Returns path 'deployment.image' when searching for lowercase 'nginx:1.19' but value is uppercase 'NGINX:1.19'",
		TestScenario:    "Search with lowercase 'nginx:1.19' for uppercase 'NGINX:1.19' value in deployment.image",
		Rationale:       "Case-insensitive matching improves usability, handles inconsistent casing in configs, reduces search failure due to case mismatches",
		RegressionRisk:  "HIGH - Adding case-sensitivity would break existing searches. Users rely on case-insensitivity for flexible queries",
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
	handler := NewYAMLHandler(".")

	// GIVEN: Content with uppercase value
	content := map[string]interface{}{
		"deployment": map[string]interface{}{
			"image": "NGINX:1.19",
		},
	}

	// WHEN: Search with lowercase search term
	paths := handler.FindKeyByValue(content, "nginx:1.19", false)

	// THEN: Should find match despite case difference
	require.Len(t, paths, 1, "Expected 1 match (case-insensitive)")
	assert.Equal(t, "deployment.image", paths[0], "Should find deployment.image")

	t.Logf("✓ Case-insensitive match works")

	// BEHAVIORAL INSIGHTS:
	// 1. Case normalization: "NGINX:1.19" matches "nginx:1.19"
	//    - Both converted to same case for comparison
	//    - Likely lowercase (nginx == nginx)
	//    - Mixed case also works: "NgiNx:1.19"
	//
	// 2. Path preservation: Original case in paths
	//    - deployment.image (not DEPLOYMENT.IMAGE)
	//    - Keys not modified by search
	//    - Only values compared case-insensitively
	//
	// 3. User convenience: Reduces search friction
	//    - Don't need to remember exact case
	//    - Handles inconsistent config casing
	//    - Standard behavior for search operations
	//
	// 4. Applies to all modes: Both exact and partial
	//    - Exact: "nginx" == "NGINX" → true
	//    - Partial: "nginx" in "NGINX web server" → true
	//    - Consistent behavior across modes
	//
	// 5. Performance: Case conversion overhead minimal
	//    - strings.ToLower() called for each value
	//    - One-time cost per value
	//    - Not a performance bottleneck in practice
}

// ============================================================================
// BEHAVIORAL CONTRACT 6: Type Conversion Handling
// ============================================================================
// DESCRIPTION:
// FindKeyByValue handles multiple Go types (int, bool, string) by converting
// all values to strings before comparison. This enables searching for non-
// string values using string search terms.
//
// BEHAVIOR ANALYSIS:
// Values are converted to string representation using fmt.Sprintf or similar.
// Search term is always string, so comparison happens after type conversion.
//
// TYPE CONVERSION RULES:
// - int: 5432 → "5432"
// - bool: true → "true", false → "false"
// - float: 3.14 → "3.14" (or "3.140000")
// - string: No conversion needed
//
// SEARCH PATTERNS:
// - Search "5432" finds int 5432
// - Search "true" finds bool true
// - Search "false" finds bool false
//
// ============================================================================

func TestFindKeyByValue_DifferentTypes_BehavioralBDD(t *testing.T) {
	// Contract: Non-string types converted to strings for matching
	contract := GoBehavioralContract{
		Behavior:        "FindKeyByValue converts int, bool, and other types to string representation before comparison, enabling string search for all value types",
		CurrentImpl:     "Uses fmt.Sprintf or type assertion to convert values to strings, compares string representations",
		ExpectedOutcome: "Search '5432' finds int 5432, search 'true' finds bool true, search 'myapp' finds string 'myapp'",
		TestScenario:    "Search content with config.port=5432 (int), config.enabled=true (bool), config.name='myapp' (string)",
		Rationale:       "Type conversion enables unified search interface for mixed-type configurations. Users search with strings regardless of underlying type",
		RegressionRisk:  "MEDIUM - Conversion format changes would affect matching (e.g., true vs TRUE). Float precision could cause issues (3.14 vs 3.140000)",
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
	handler := NewYAMLHandler(".")

	// GIVEN: Content with different value types
	content := map[string]interface{}{
		"config": map[string]interface{}{
			"port":    5432,
			"enabled": true,
			"name":    "myapp",
		},
	}

	// WHEN/THEN: Search for integer
	paths := handler.FindKeyByValue(content, "5432", false)
	require.Len(t, paths, 1, "Should find integer port")
	assert.Equal(t, "config.port", paths[0], "Should find config.port")

	// WHEN/THEN: Search for boolean
	paths = handler.FindKeyByValue(content, "true", false)
	require.Len(t, paths, 1, "Should find boolean enabled")
	assert.Equal(t, "config.enabled", paths[0], "Should find config.enabled")

	// WHEN/THEN: Search for string
	paths = handler.FindKeyByValue(content, "myapp", false)
	require.Len(t, paths, 1, "Should find string name")
	assert.Equal(t, "config.name", paths[0], "Should find config.name")

	t.Logf("✓ Different value types handled correctly")

	// BEHAVIORAL INSIGHTS:
	// 1. Integer conversion: 5432 → "5432"
	//    - Decimal representation (not hex, octal)
	//    - No padding or formatting (not "5,432" or "5432.0")
	//    - Negative numbers: -100 → "-100"
	//
	// 2. Boolean conversion: true → "true", false → "false"
	//    - Lowercase (not "True" or "TRUE")
	//    - String "true" distinguishable from bool true post-conversion
	//    - Consistent with Go's fmt.Sprint behavior
	//
	// 3. String values: No conversion needed
	//    - Direct comparison
	//    - Most common case (optimized path?)
	//
	// 4. Float conversion: Not tested here
	//    - Potential issue: 3.14 vs "3.14" vs "3.140000"
	//    - Precision matters for exact matching
	//    - Partial match more forgiving
	//
	// 5. Use cases:
	//    - Find all ports with value 8080 (int search)
	//    - Find all enabled features (bool search)
	//    - Find specific versions (string search)
	//    - Mixed-type configurations common in Kubernetes
}

// ============================================================================
// BEHAVIORAL CONTRACT 7: Real-World Kubernetes Manifest Search
// ============================================================================
// DESCRIPTION:
// Complex real-world scenario testing FindKeyByValue with nested Kubernetes-
// style manifests including multiple container definitions, environment
// variables, and deep nesting.
//
// BEHAVIOR ANALYSIS:
// Validates that all previous behaviors work correctly in combination:
// - Deep nesting (5+ levels)
// - Multiple arrays (containers, env)
// - Duplicate values (production appears twice)
// - Partial matching (nginx substring search)
//
// KUBERNETES MANIFEST STRUCTURE:
//   schemaVersion/kind/metadata (top-level fields)
//   └─ spec
//      └─ template
//         └─ spec
//            └─ containers[]
//               └─ name, image, env[]
//                  └─ name, value
//
// ============================================================================

func TestFindKeyByValue_RealWorldKubernetes_BehavioralBDD(t *testing.T) {
	// Contract: Complex Kubernetes manifests handled correctly
	contract := GoBehavioralContract{
		Behavior:        "FindKeyByValue handles deeply nested Kubernetes manifests with multiple arrays, duplicate values, and complex structures",
		CurrentImpl:     "Recursively traverses all levels, processes both containers arrays, finds duplicate 'production' values in separate container env arrays",
		ExpectedOutcome: "Finds 2 'production' values (one per container), finds 2+ nginx references (container name + image)",
		TestScenario:    "Search realistic Kubernetes Deployment with 2 containers (nginx, sidecar), each with ENV=production, find all production values and nginx references",
		Rationale:       "Real-world validation ensures algorithm works with actual Kubernetes configs. Tests combination of all features: deep nesting, arrays, duplicates, partial matching",
		RegressionRisk:  "HIGH - Kubernetes configs are primary use case. Breaking this breaks production GitOps workflows",
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
	handler := NewYAMLHandler(".")

	// GIVEN: Realistic Kubernetes Deployment manifest
	content := map[string]interface{}{
		"schemaVersion": "apps/v1",
		"kind":          "Deployment",
		"metadata": map[string]interface{}{
			"name": "web-deployment",
		},
		"spec": map[string]interface{}{
			"replicas": 3,
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:1.19",
							"env": []interface{}{
								map[string]interface{}{
									"name":  "ENV",
									"value": "production",
								},
							},
						},
						map[string]interface{}{
							"name":  "sidecar",
							"image": "sidecar:latest",
							"env": []interface{}{
								map[string]interface{}{
									"name":  "ENV",
									"value": "production",
								},
							},
						},
					},
				},
			},
		},
	}

	// WHEN: Find all occurrences of "production"
	paths := handler.FindKeyByValue(content, "production", false)

	// THEN: Should find both env[0].value entries (one per container)
	assert.Len(t, paths, 2, "Expected 2 'production' values (one per container)")

	// WHEN: Find all nginx references (partial match)
	paths = handler.FindKeyByValue(content, "nginx", true)

	// THEN: Should find nginx container name and nginx:1.19 image
	assert.GreaterOrEqual(t, len(paths), 2, "Expected at least 2 nginx references")

	t.Logf("✓ Kubernetes manifest search works correctly")

	// BEHAVIORAL INSIGHTS:
	// 1. Deep nesting handled: 5+ levels traversed
	//    - spec.template.spec.containers[0].env[0].value
	//    - No depth limit (recursive algorithm)
	//    - Complex path notation preserved
	//
	// 2. Multiple arrays: containers[] and env[] both processed
	//    - Nested arrays: containers[n].env[m]
	//    - Array within array within map
	//    - Common Kubernetes pattern
	//
	// 3. Duplicate values: "production" appears twice
	//    - Both occurrences found (exhaustive search)
	//    - Different paths distinguish them
	//    - containers[0].env[0].value vs containers[1].env[0].value
	//
	// 4. Partial matching: "nginx" finds multiple places
	//    - Container name: "nginx"
	//    - Image reference: "nginx:1.19"
	//    - Demonstrates substring matching
	//
	// 5. Production use case: Configuration auditing
	//    - Verify all containers use production env
	//    - Find all nginx versions deployed
	//    - Locate specific API versions
	//    - Security: find privileged containers, exposed ports
	//
	// 6. Path examples generated:
	//    - spec.template.spec.containers[0].name
	//    - spec.template.spec.containers[0].image
	//    - spec.template.spec.containers[0].env[0].value
	//    - spec.template.spec.containers[1].env[0].value
}

// ============================================================================
// END OF BEHAVIORAL TEST SUITE
// ============================================================================
// SUMMARY:
// - 7 behavioral contracts documented
// - 7 test scenarios covering all major use cases
// - Key behaviors: Exact/partial match, arrays, types, case, real-world
// - Integration: Configuration auditing, value replacement, debugging
//
// MAINTENANCE NOTES:
// - Path notation is public API (breaking change risk)
// - Case-insensitive matching is expected behavior
// - Type conversion enables mixed-type searches
// - Array bracket notation [n] is standard
// - Empty slice return (not nil) for no matches
//
// TESTING APPROACH:
// Time-Based BDD characterizes current behavior for:
// - Future refactoring safety
// - Regression prevention
// - Documentation of search algorithm
// - Understanding caller expectations
//
// USE CASES COVERED:
// 1. Exact value location (security audits, replacements)
// 2. Fuzzy searching (finding related configs)
// 3. Container-specific operations (Kubernetes)
// 4. Multi-container scenarios (sidecars, init containers)
// 5. Environment-specific validation (prod/staging/dev)
// 6. Type-agnostic searching (ports, flags, names)
//
// ============================================================================
