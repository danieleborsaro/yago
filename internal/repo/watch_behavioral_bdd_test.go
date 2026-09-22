package repo

import (
	"testing"

	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	uRepo "github.com/danieleborsaro/yago/internal/utils/repo"
)

// ============================================================================
// FILE MIGRATION METADATA
// ============================================================================
// Original file: internal/repo/watch_test.go
// Migration date: October 13, 2025
// Tests migrated: 2 (parseRepoWatch, GetWatchList)
// Behavioral contracts created: 2
//
// This file documents the repository watch functionality's behavior through
// Time-Based BDD (Behavioral-Driven Development). It characterizes how the
// system monitors repository paths for changes and manages watch lists.
//
// WATCH SYSTEM OVERVIEW:
// The watch system allows selective monitoring of repository paths using glob
// patterns. This enables efficient change detection by focusing on specific
// directories or file patterns rather than monitoring entire repositories.
//
// KEY BEHAVIORS:
// 1. parseRepoWatch: Extracts watch patterns from YAML configuration
// 2. GetWatchList: Retrieves watch patterns for a repository
//
// DESIGN PATTERNS:
// - Glob patterns for flexible path matching (path/to/**/*.yaml)
// - Empty watch lists mean "monitor everything" (default behavior)
// - Watch lists are stored as string slices (order preserved)
// - Configuration parsing uses property paths for nested access
//
// INTEGRATION POINTS:
// - parser.YAMLHandler: For extracting configuration data
// - schema.SchemaVersion: For version-specific property paths
// - Repo struct: For storing and accessing watch lists
//
// TYPICAL USAGE:
//   // Parse watch list from configuration
//   watchList, err := parseRepoWatch(content, "desiredstate.meta.repo", propertyPaths, config)
//
//   // Get watch list from repository
//   patterns := repo.GetWatchList()
//
// ============================================================================
// Note: GoBehavioralContract struct is defined in git_behavioral_bdd_test.go
// ============================================================================
// BEHAVIORAL CONTRACT 1: parseRepoWatch Function
// ============================================================================
// FUNCTION SIGNATURE: parseRepoWatch(content map[string]interface{}, item string,
//                                     propertyPaths map[string]parser.PropertyPath,
//                                     config *uRepo.RepoConfig) ([]string, error)
//
// DESCRIPTION:
// parseRepoWatch extracts watch patterns from a nested YAML configuration
// structure. It navigates to the repository configuration section and extracts
// an array of glob patterns that specify which paths to monitor.
//
// BEHAVIOR ANALYSIS:
// The function uses property paths to navigate nested YAML structures, locates
// the "watch" field within repository configuration, and converts the interface{}
// array into a string slice. It handles various edge cases gracefully.
//
// WATCH LIST SEMANTICS:
// - Present and populated: Monitor only specified paths
// - Present and empty: Monitor nothing (empty list is explicit)
// - Absent: No watch configuration (returns empty list, but caller may interpret as "monitor all")
// - Invalid format: Error (type assertion failure)
//
// CONFIGURATION STRUCTURE:
//   schema: "1.0.0"
//   desiredstate:
//     meta:
//       repo:
//         url: "git@github.com:user/repo.git"
//         branch: "main"
//         watch:
//           - "path1/**"
//           - "path2/**/*.yaml"
//
// INTEGRATION:
// - Uses parser.YAMLHandler to get property paths
// - Requires schema.SchemaVersion for version-specific paths
// - Returns []string for direct use with file monitoring systems
//
// ERROR CONDITIONS:
// - Schema version extraction failure
// - Property path retrieval failure
// - Type assertion failure (watch field is not []interface{})
//
// DESIGN RATIONALE:
// Separates configuration parsing from watch logic, allowing different
// configuration formats while maintaining consistent watch behavior.
//
// ============================================================================

func TestParseRepoWatch_BehavioralBDD(t *testing.T) {
	// Contract: parseRepoWatch extracts watch patterns from YAML configuration
	contract := GoBehavioralContract{
		Behavior:        "parseRepoWatch navigates nested YAML configuration to extract repository watch patterns",
		CurrentImpl:     "Uses property paths to access nested 'watch' field, converts []interface{} to []string, handles missing/empty watch lists by returning empty slice",
		ExpectedOutcome: "Returns string slice of glob patterns for path monitoring, or empty slice if no watch list configured",
		TestScenario:    "Parse watch patterns from various YAML configuration structures including single entry, multiple entries, missing field, and empty list",
		Rationale:       "Watch patterns enable selective repository monitoring for change detection. Configuration must support flexible pattern specification while handling common edge cases gracefully",
		RegressionRisk:  "HIGH - Callers rely on empty slice semantics (may mean 'monitor all' vs 'monitor nothing'), type conversion failures could break monitoring, property path changes affect nested access",
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

	// Test setup: Logger and configuration for repository operations
	logger := logging.NewLogger(logging.DEBUG)
	config := &uRepo.RepoConfig{
		Logger:          logger,
		ProgressHandler: uRepo.NewLogProgressHandler(logger),
	}

	// Test cases covering different watch list configurations
	tests := []struct {
		name          string
		content       map[string]interface{}
		item          string
		expectedWatch []string
	}{
		{
			name: "watch list with one entry",
			content: map[string]interface{}{
				"schema": "1.0.0",
				"desiredstate": map[string]interface{}{
					"meta": map[string]interface{}{
						"repo": map[string]interface{}{
							"url":    "git@github.com:test/repo.git",
							"branch": "main",
							"watch": []interface{}{
								"path/to/watch/**",
							},
						},
					},
				},
			},
			item:          "desiredstate.meta.repo",
			expectedWatch: []string{"path/to/watch/**"},
		},
		{
			name: "watch list with multiple entries",
			content: map[string]interface{}{
				"schema": "1.0.0",
				"desiredstate": map[string]interface{}{
					"meta": map[string]interface{}{
						"repo": map[string]interface{}{
							"url":    "git@github.com:test/repo.git",
							"branch": "main",
							"watch": []interface{}{
								"path1/**",
								"path2/**",
								"path3/**",
							},
						},
					},
				},
			},
			item:          "desiredstate.meta.repo",
			expectedWatch: []string{"path1/**", "path2/**", "path3/**"},
		},
		{
			name: "no watch list",
			content: map[string]interface{}{
				"schema": "1.0.0",
				"desiredstate": map[string]interface{}{
					"meta": map[string]interface{}{
						"repo": map[string]interface{}{
							"url":    "git@github.com:test/repo.git",
							"branch": "main",
						},
					},
				},
			},
			item:          "desiredstate.meta.repo",
			expectedWatch: []string{},
		},
		{
			name: "empty watch list",
			content: map[string]interface{}{
				"schema": "1.0.0",
				"desiredstate": map[string]interface{}{
					"meta": map[string]interface{}{
						"repo": map[string]interface{}{
							"url":    "git@github.com:test/repo.git",
							"branch": "main",
							"watch":  []interface{}{},
						},
					},
				},
			},
			item:          "desiredstate.meta.repo",
			expectedWatch: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// STEP 1: Extract schema version from configuration
			// Behavior: Schema version determines property path structure
			versionStr, err := extractSchemaVersion(tt.content)
			if err != nil {
				t.Fatalf("Failed to extract schema version: %v", err)
			}
			version := schema.SchemaVersion(versionStr)

			// STEP 2: Get property paths for this schema version
			// Behavior: Property paths enable nested YAML navigation
			handler := parser.NewYAMLHandler("")
			propertyPaths, err := handler.GetPropertyPaths(version, true)
			if err != nil {
				t.Fatalf("Failed to get property paths: %v", err)
			}

			// STEP 3: Parse watch list from configuration
			// Behavior: Extracts watch patterns using property paths
			watchList, err := parseRepoWatch(tt.content, tt.item, propertyPaths, config)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// VERIFICATION 1: Watch list length matches expected
			// Behavior: Returns correct number of watch patterns
			if len(watchList) != len(tt.expectedWatch) {
				t.Errorf("Expected %d watch entries, got %d", len(tt.expectedWatch), len(watchList))
			}

			// VERIFICATION 2: Each watch pattern matches expected value
			// Behavior: Preserves exact glob patterns from configuration
			for i, expected := range tt.expectedWatch {
				if i >= len(watchList) {
					t.Errorf("Missing watch entry at index %d: expected %s", i, expected)
					continue
				}
				if watchList[i] != expected {
					t.Errorf("Watch entry %d: expected %s, got %s", i, expected, watchList[i])
				}
			}
		})
	}

	// BEHAVIORAL INSIGHTS:
	// 1. Empty slice semantics: Both "no watch field" and "empty watch array" return []string{}
	//    - Callers must distinguish between "not configured" and "explicitly empty"
	//    - Common interpretation: missing = monitor all, empty = monitor nothing
	//
	// 2. Property path dependency: Function requires correct schema version
	//    - Schema version changes may affect property path structure
	//    - Version compatibility is critical for configuration parsing
	//
	// 3. Type conversion: Converts []interface{} to []string
	//    - Assumes all watch entries are strings (type assertion)
	//    - Non-string entries would cause runtime panic
	//
	// 4. Glob pattern support: Returns patterns as-is
	//    - No validation of glob syntax
	//    - Caller responsible for pattern matching implementation
	//
	// 5. Order preservation: Maintains watch pattern order from YAML
	//    - May be significant for priority-based monitoring
	//    - Order changes in YAML reflected in returned slice
}

// ============================================================================
// BEHAVIORAL CONTRACT 2: GetWatchList Method
// ============================================================================
// METHOD SIGNATURE: func (r *Repo) GetWatchList() []string
//
// DESCRIPTION:
// GetWatchList returns the watch patterns configured for this repository.
// It provides a safe accessor for the Repo.Watch field, handling nil slices
// gracefully.
//
// BEHAVIOR ANALYSIS:
// Simple getter method that returns the watch list or an empty slice if nil.
// This provides a consistent interface for accessing watch patterns without
// callers needing to check for nil.
//
// RETURN VALUE SEMANTICS:
// - nil Watch field: Returns []string{} (empty slice, not nil)
// - Empty Watch field: Returns []string{} (same as nil)
// - Populated Watch field: Returns the slice as-is
//
// NIL HANDLING:
// The method distinguishes between:
// - Uninitialized: Watch is nil → returns empty slice
// - Explicitly empty: Watch is []string{} → returns empty slice
// - Populated: Watch has entries → returns those entries
//
// However, to callers, both nil and empty look identical ([]string{}).
//
// USAGE PATTERNS:
//   patterns := repo.GetWatchList()
//   if len(patterns) == 0 {
//       // Monitor entire repository (or nothing, depending on context)
//   } else {
//       // Monitor only specified paths
//       for _, pattern := range patterns {
//           // Apply glob matching
//       }
//   }
//
// DESIGN RATIONALE:
// Encapsulates nil checking in a single place, providing simpler calling code.
// The empty slice return value allows callers to use len() and range without
// special handling.
//
// THREAD SAFETY:
// Returns the slice directly (not a copy), so concurrent modifications could
// affect callers. In practice, watch lists are typically immutable after
// initialization.
//
// ============================================================================

func TestGetWatchList_BehavioralBDD(t *testing.T) {
	// Contract: GetWatchList provides safe access to repository watch patterns
	contract := GoBehavioralContract{
		Behavior:        "GetWatchList returns the repository's watch patterns, converting nil to empty slice for safe iteration",
		CurrentImpl:     "Directly returns r.Watch slice, or empty []string{} if Watch is nil",
		ExpectedOutcome: "Returns []string containing glob patterns, never returns nil (always safe for len() and range)",
		TestScenario:    "Access watch list in three states: nil (uninitialized), empty (explicitly set to []), and populated (with patterns)",
		Rationale:       "Provides consistent nil-safe interface for watch list access. Callers can use len() and range without nil checks, simplifying monitoring logic",
		RegressionRisk:  "MEDIUM - Changing nil handling (returning nil instead of empty slice) would break callers expecting safe iteration. Watch list modifications could affect concurrent readers if not copied",
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

	// Test cases covering different watch list states
	tests := []struct {
		name     string
		repo     *Repo
		expected []string
	}{
		{
			name: "nil watch list",
			repo: &Repo{
				Watch: nil,
			},
			expected: []string{},
		},
		{
			name: "empty watch list",
			repo: &Repo{
				Watch: []string{},
			},
			expected: []string{},
		},
		{
			name: "watch list with entries",
			repo: &Repo{
				Watch: []string{"path1/**", "path2/**"},
			},
			expected: []string{"path1/**", "path2/**"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// EXECUTION: Call GetWatchList on repository
			// Behavior: Returns watch patterns or empty slice
			result := tt.repo.GetWatchList()

			// VERIFICATION 1: Result is never nil
			// Behavior: Always returns a slice (possibly empty)
			if result == nil {
				t.Error("GetWatchList returned nil, expected empty slice")
			}

			// VERIFICATION 2: Length matches expected
			// Behavior: Returns correct number of patterns
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d entries, got %d", len(tt.expected), len(result))
			}

			// VERIFICATION 3: Each pattern matches expected value
			// Behavior: Preserves exact patterns and order
			for i, expected := range tt.expected {
				if i >= len(result) {
					t.Errorf("Missing entry at index %d: expected %s", i, expected)
					continue
				}
				if result[i] != expected {
					t.Errorf("Entry %d: expected %s, got %s", i, expected, result[i])
				}
			}
		})
	}

	// BEHAVIORAL INSIGHTS:
	// 1. Nil safety: Method guarantees non-nil return value
	//    - Callers can always use len() and range safely
	//    - No need for "if result != nil" checks
	//
	// 2. Empty slice equivalence: nil and empty treated identically
	//    - Both return []string{} to caller
	//    - Distinction lost at API boundary
	//    - Callers cannot determine if Watch was nil or empty
	//
	// 3. Direct slice return: Returns underlying slice (not a copy)
	//    - Modifications by caller affect Repo.Watch
	//    - Concurrent modifications possible
	//    - Performance: No allocation for copy
	//
	// 4. No validation: Returns patterns as stored
	//    - No glob syntax checking
	//    - No duplicate removal
	//    - No normalization (e.g., path separators)
	//
	// 5. Common usage pattern:
	//    if len(repo.GetWatchList()) == 0 {
	//        // Handle "monitor all" case
	//    } else {
	//        // Apply specific patterns
	//    }
	//
	// SEMANTIC AMBIGUITY:
	// The empty slice return value has two possible meanings:
	// - Configuration interpretation: "monitor nothing" (explicit empty list)
	// - Default interpretation: "monitor everything" (no restrictions)
	// The method itself doesn't distinguish - context determines meaning.
}

// ============================================================================
// END OF BEHAVIORAL TEST SUITE
// ============================================================================
// SUMMARY:
// - 2 behavioral contracts documented
// - 7 sub-test scenarios (4 parseRepoWatch + 3 GetWatchList)
// - Key behaviors: Configuration parsing, nil-safe access, glob patterns
// - Integration: parser, schema, logging systems
//
// MAINTENANCE NOTES:
// - Watch patterns are glob strings (**, *.yaml, etc.)
// - Empty slice semantics vary by context (monitor all vs nothing)
// - Property paths depend on schema version
// - GetWatchList returns underlying slice (not a copy)
//
// TESTING APPROACH:
// Time-Based BDD characterizes current behavior for:
// - Future refactoring safety
// - Regression prevention
// - Documentation of implicit contracts
// - Understanding caller expectations
//
// ============================================================================
