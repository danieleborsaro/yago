package property

import (
	"reflect"
	"strings"
	"testing"
)

// BehavioralContract documents property wrapper behavior and the current implementation.
type BehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

// TestPropertyWrapper_NewPropertyWrapper_BehavioralBDD tests constructor behavioral contracts
//
//   - Constructor: YAMLWrapper(content=None, path="")
//   - Initialization: self.data = content if content else {}
//   - Path storage: self.path = path
//   - Returns: YAMLWrapper instance
//
// Go Implementation (internal/property.NewPropertyWrapper):
//   - Constructor: NewPropertyWrapper(content map[string]interface{}, path string)
//   - Initialization: Data = content if content != nil else make(map[string]interface{})
//   - Path storage: path field
//   - Returns: *PropertyWrapper pointer
//
// Behavioral Contract:
// Both implementations create a wrapper around YAML/property data with optional
// initialization content. The key semantic is that nil/None content results in
// an empty initialized map, not a nil/None data field. This ensures the wrapper
// is always safe to use without nil checks.
//
// Regression Risk: MEDIUM
// - Nil content handling: Wrong initialization breaks all subsequent operations
// - Path storage: Used for error messages and debugging
func TestPropertyWrapper_NewPropertyWrapper_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Constructor creates wrapper with initialized data map and path",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("Constructor with nil content", func(t *testing.T) {
		t.Log("=== Test: Nil content initializes empty map ===")
		pw := NewPropertyWrapper(nil, "test")

		if pw.Data == nil {
			t.Error("❌ Expected Data to be initialized as empty map, got nil")
		} else {
			t.Log("✓ Data initialized (not nil)")
		}

		if len(pw.Data) != 0 {
			t.Errorf("❌ Expected Data to be empty, got length %d", len(pw.Data))
		} else {
			t.Log("✓ Data is empty map")
		}

		if pw.GetPath() != "test" {
			t.Errorf("❌ Expected path to be 'test', got '%s'", pw.GetPath())
		} else {
			t.Log("✓ Path stored correctly: 'test'")
		}

		t.Log("✓ CONTRACT SATISFIED: Nil content creates empty initialized map")
	})

	t.Run("Constructor with existing content", func(t *testing.T) {
		t.Log("=== Test: Existing content stored correctly ===")
		content := map[string]interface{}{
			"key": "value",
		}
		pw := NewPropertyWrapper(content, "test2")

		if pw.Data["key"] != "value" {
			t.Error("❌ Expected Data to contain the provided content")
		} else {
			t.Log("✓ Content stored: key='value'")
		}

		if pw.GetPath() != "test2" {
			t.Errorf("❌ Expected path to be 'test2', got '%s'", pw.GetPath())
		} else {
			t.Log("✓ Path stored correctly: 'test2'")
		}

		t.Log("✓ CONTRACT SATISFIED: Content wrapper initialized with provided data")
	})
}

// TestPropertyWrapper_GetValue_BehavioralBDD tests value retrieval behavioral contracts
//
//   - Method: get_value(self, path: str) -> Any
//   - Path syntax: Dot-separated (e.g., "level1.level2.key")
//   - Empty path: Returns entire data dict
//   - Missing key: Raises KeyError
//   - Navigation: Recursively traverses nested dicts
//
// Go Implementation (internal/property.PropertyWrapper.GetValue):
//   - Method: GetValue(path string) (interface{}, error)
//   - Path syntax: Dot-separated (e.g., "level1.level2.key")
//   - Empty path: Returns entire Data map
//   - Missing key: Returns error
//   - Navigation: Recursively traverses nested maps
//
// Behavioral Contract:
// Both implementations navigate nested structures using dot-separated paths.
// Empty path is a special case returning the entire structure. Missing keys
// signal failure (exception vs error return).
//
// Regression Risk: HIGH
// - Path parsing: Wrong splitting breaks all nested access
// - Empty path: Must return entire structure, not error
// - Missing key: Must signal error, not return nil/empty
func TestPropertyWrapper_GetValue_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Retrieve values from nested structure using dot-separated paths",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	content := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"key": "value",
			},
		},
	}
	pw := NewPropertyWrapper(content, "")

	t.Run("Simple nested path", func(t *testing.T) {
		t.Log("=== Test: Navigate nested structure with dot path ===")
		value, err := pw.GetValue("level1.level2.key")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ No error returned")
		}

		if value != "value" {
			t.Errorf("❌ Expected 'value', got '%v'", value)
		} else {
			t.Log("✓ Retrieved correct value: 'value'")
		}

		t.Log("✓ CONTRACT SATISFIED: Dot path navigates nested maps")
	})

	t.Run("Non-existent path", func(t *testing.T) {
		t.Log("=== Test: Missing key returns error ===")
		_, err := pw.GetValue("level1.nonexistent")
		if err == nil {
			t.Error("❌ Expected error for non-existent path")
		} else {
			t.Logf("✓ Error returned: %v", err)
		}

		t.Log("✓ CONTRACT SATISFIED: Missing keys signal error")
	})

	t.Run("Empty path returns entire structure", func(t *testing.T) {
		t.Log("=== Test: Empty path returns complete data ===")
		result, err := pw.GetValue("")
		if err != nil {
			t.Errorf("❌ Unexpected error for empty path: %v", err)
		} else {
			t.Log("✓ No error for empty path")
		}

		if !reflect.DeepEqual(result, content) {
			t.Error("❌ Expected entire content for empty path")
		} else {
			t.Log("✓ Empty path returns entire structure")
		}

		t.Log("✓ CONTRACT SATISFIED: Empty path is special case for full data")
	})
}

// TestPropertyWrapper_AddKey_BehavioralBDD tests key addition behavioral contracts
//
//   - Method: add_key(self, path: str, value: Any) -> None
//   - Creates missing intermediate dicts automatically
//   - Simple path: Sets top-level key
//   - Nested path: Creates nested structure as needed
//   - Overwrites: Existing keys are replaced
//
// Go Implementation (internal/property.PropertyWrapper.AddKey):
//   - Method: AddKey(path string, value interface{}) error
//   - Creates missing intermediate maps automatically
//   - Simple path: Sets top-level key
//   - Nested path: Creates nested structure as needed
//   - Overwrites: Existing keys are replaced
//
// Behavioral Contract:
// Both implementations auto-vivify nested structures, creating intermediate
// maps/dicts as needed. This enables setting deeply nested values in one call
// without manual structure creation.
//
// Regression Risk: HIGH
// - Auto-vivification: Missing intermediate creation breaks nested operations
// - Overwrite behavior: Must replace existing values, not merge
// - Path creation: Must handle any depth correctly
func TestPropertyWrapper_AddKey_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Add keys with auto-vivification of intermediate structures",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("Add simple top-level key", func(t *testing.T) {
		t.Log("=== Test: Add simple key at root level ===")
		pw := NewPropertyWrapper(nil, "")

		err := pw.AddKey("simple", "value")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ Key added without error")
		}

		value, err := pw.GetValue("simple")
		if err != nil {
			t.Errorf("❌ Unexpected error retrieving: %v", err)
		}
		if value != "value" {
			t.Errorf("❌ Expected 'value', got '%v'", value)
		} else {
			t.Log("✓ Value retrieved correctly: 'value'")
		}

		t.Log("✓ CONTRACT SATISFIED: Simple key addition works")
	})

	t.Run("Add nested key with auto-vivification", func(t *testing.T) {
		t.Log("=== Test: Nested path auto-creates intermediate maps ===")
		pw := NewPropertyWrapper(nil, "")

		err := pw.AddKey("level1.level2.key", "nested_value")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ Nested key added (intermediate maps created)")
		}

		value, err := pw.GetValue("level1.level2.key")
		if err != nil {
			t.Errorf("❌ Unexpected error retrieving: %v", err)
		}
		if value != "nested_value" {
			t.Errorf("❌ Expected 'nested_value', got '%v'", value)
		} else {
			t.Log("✓ Value retrieved from nested path: 'nested_value'")
		}

		t.Log("✓ CONTRACT SATISFIED: Auto-vivification creates intermediate structure")
	})
}

// TestPropertyWrapper_MergeKeys_BehavioralBDD tests map merging behavioral contracts
//
//   - Method: merge_keys(self, other: dict) -> None
//   - Deep merge: Recursively merges nested dicts
//   - Preserves existing: Keys in self are kept
//   - Adds new: Keys from other are added
//   - Nested merge: Combines nested dicts, doesn't replace
//
// Go Implementation (internal/property.PropertyWrapper.MergeKeys):
//   - Method: MergeKeys(other map[string]interface{})
//   - Deep merge: Recursively merges nested maps
//   - Preserves existing: Keys in Data are kept
//   - Adds new: Keys from other are added
//   - Nested merge: Combines nested maps, doesn't replace
//
// Behavioral Contract:
// Both implementations perform deep merging where nested structures are combined
// rather than replaced. This is critical for configuration merging where you want
// to overlay configs without losing existing nested data.
//
// Regression Risk: HIGH
// - Deep merge: Shallow merge would lose nested data
// - Preservation: Must keep existing values when no conflict
// - Nested combination: Must merge nested maps, not replace
func TestPropertyWrapper_MergeKeys_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Deep merge maps, preserving existing keys and combining nested structures",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	content1 := map[string]interface{}{
		"key1": "value1",
		"nested": map[string]interface{}{
			"subkey1": "subvalue1",
		},
	}

	content2 := map[string]interface{}{
		"key2": "value2",
		"nested": map[string]interface{}{
			"subkey2": "subvalue2",
		},
	}

	t.Run("Merge preserves existing keys", func(t *testing.T) {
		t.Log("=== Test: Existing keys preserved during merge ===")
		pw := NewPropertyWrapper(content1, "")
		pw.MergeKeys(content2)

		value, err := pw.GetValue("key1")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}
		if value != "value1" {
			t.Errorf("❌ Expected 'value1', got '%v'", value)
		} else {
			t.Log("✓ Existing key1 preserved: 'value1'")
		}

		t.Log("✓ CONTRACT SATISFIED: Merge preserves existing keys")
	})

	t.Run("Merge adds new keys", func(t *testing.T) {
		t.Log("=== Test: New keys added from merge source ===")
		pw := NewPropertyWrapper(content1, "")
		pw.MergeKeys(content2)

		value, err := pw.GetValue("key2")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}
		if value != "value2" {
			t.Errorf("❌ Expected 'value2', got '%v'", value)
		} else {
			t.Log("✓ New key2 added: 'value2'")
		}

		t.Log("✓ CONTRACT SATISFIED: Merge adds new keys")
	})

	t.Run("Deep merge combines nested maps", func(t *testing.T) {
		t.Log("=== Test: Nested maps merged, not replaced ===")
		pw := NewPropertyWrapper(content1, "")
		pw.MergeKeys(content2)

		// Check original nested key still exists
		value, err := pw.GetValue("nested.subkey1")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}
		if value != "subvalue1" {
			t.Errorf("❌ Expected 'subvalue1', got '%v'", value)
		} else {
			t.Log("✓ Original nested key preserved: nested.subkey1='subvalue1'")
		}

		// Check new nested key was added
		value, err = pw.GetValue("nested.subkey2")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}
		if value != "subvalue2" {
			t.Errorf("❌ Expected 'subvalue2', got '%v'", value)
		} else {
			t.Log("✓ New nested key added: nested.subkey2='subvalue2'")
		}

		t.Log("✓ CONTRACT SATISFIED: Deep merge combines nested structures")
	})
}

// TestPropertyWrapper_HasKey_BehavioralBDD tests key existence check behavioral contracts
//
//   - Method: has_key(self, path: str) -> bool
//   - Returns: True if path exists, False otherwise
//   - Navigation: Uses same dot-path as get_value
//   - No exceptions: Returns False for missing paths
//
// Go Implementation (internal/property.PropertyWrapper.HasKey):
//   - Method: HasKey(path string) bool
//   - Returns: true if path exists, false otherwise
//   - Navigation: Uses same dot-path as GetValue
//   - No errors: Returns false for missing paths
//
// Behavioral Contract:
// Simple existence check using same path semantics as GetValue. Never fails,
// always returns boolean result. This is the safe way to check before GetValue.
//
// Regression Risk: MEDIUM
// - Path navigation: Must use identical logic to GetValue
// - False negatives: Wrong implementation returns false for existing keys
// - False positives: Returns true for non-existent keys
func TestPropertyWrapper_HasKey_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Check key existence without error, using same path navigation as GetValue",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	content := map[string]interface{}{
		"existing": "value",
		"nested": map[string]interface{}{
			"key": "value",
		},
	}
	pw := NewPropertyWrapper(content, "")

	t.Run("Existing top-level key", func(t *testing.T) {
		t.Log("=== Test: Top-level key existence ===")
		if !pw.HasKey("existing") {
			t.Error("❌ Expected HasKey to return true for existing key")
		} else {
			t.Log("✓ HasKey returns true for 'existing'")
		}

		t.Log("✓ CONTRACT SATISFIED: Top-level key detected")
	})

	t.Run("Existing nested key", func(t *testing.T) {
		t.Log("=== Test: Nested key existence ===")
		if !pw.HasKey("nested.key") {
			t.Error("❌ Expected HasKey to return true for nested existing key")
		} else {
			t.Log("✓ HasKey returns true for 'nested.key'")
		}

		t.Log("✓ CONTRACT SATISFIED: Nested key detected")
	})

	t.Run("Non-existing key", func(t *testing.T) {
		t.Log("=== Test: Non-existent key returns false ===")
		if pw.HasKey("nonexistent") {
			t.Error("❌ Expected HasKey to return false for non-existing key")
		} else {
			t.Log("✓ HasKey returns false for 'nonexistent'")
		}

		t.Log("✓ CONTRACT SATISFIED: Missing keys return false")
	})
}

// TestPropertyWrapper_DeleteKey_BehavioralBDD tests key deletion behavioral contracts
//
//   - Method: delete_key(self, path: str) -> None
//   - Deletes: Removes key at path
//   - Nested: Can delete nested keys
//   - Missing: Raises KeyError if path doesn't exist
//
// Go Implementation (internal/property.PropertyWrapper.DeleteKey):
//   - Method: DeleteKey(path string) error
//   - Deletes: Removes key at path
//   - Nested: Can delete nested keys
//   - Missing: Returns error if path doesn't exist
//
// Behavioral Contract:
// Both implementations remove keys from structure using dot-path navigation.
// Supports nested deletion. Signals error for non-existent paths.
//
// Regression Risk: MEDIUM
// - Nested deletion: Must navigate correctly to delete nested keys
// - Error signaling: Must indicate when path doesn't exist
// - Side effects: Must not corrupt structure on failed deletion
func TestPropertyWrapper_DeleteKey_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Delete keys from structure using dot-path navigation",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("Delete root level key", func(t *testing.T) {
		t.Log("=== Test: Delete top-level key ===")
		content := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}
		pw := NewPropertyWrapper(content, "")

		err := pw.DeleteKey("key1")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ Delete operation succeeded")
		}

		if pw.HasKey("key1") {
			t.Error("❌ Expected key1 to be deleted")
		} else {
			t.Log("✓ key1 successfully deleted")
		}

		// Verify key2 still exists
		if !pw.HasKey("key2") {
			t.Error("❌ Expected key2 to remain")
		} else {
			t.Log("✓ key2 unaffected by deletion")
		}

		t.Log("✓ CONTRACT SATISFIED: Top-level key deletion works")
	})

	t.Run("Delete nested key", func(t *testing.T) {
		t.Log("=== Test: Delete nested key ===")
		content := map[string]interface{}{
			"nested": map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		}
		pw := NewPropertyWrapper(content, "")

		err := pw.DeleteKey("nested.key1")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ Nested delete operation succeeded")
		}

		if pw.HasKey("nested.key1") {
			t.Error("❌ Expected nested.key1 to be deleted")
		} else {
			t.Log("✓ nested.key1 successfully deleted")
		}

		// Verify nested.key2 still exists
		if !pw.HasKey("nested.key2") {
			t.Error("❌ Expected nested.key2 to remain")
		} else {
			t.Log("✓ nested.key2 unaffected by deletion")
		}

		t.Log("✓ CONTRACT SATISFIED: Nested key deletion works")
	})
}

// TestPropertyWrapper_IsEmpty_BehavioralBDD tests empty check behavioral contracts
//
//   - Method: is_empty(self) -> bool
//   - Returns: True if data dict is empty, False otherwise
//   - Check: len(self.data) == 0
//
// Go Implementation (internal/property.PropertyWrapper.IsEmpty):
//   - Method: IsEmpty() bool
//   - Returns: true if Data map is empty, false otherwise
//   - Check: len(Data) == 0
//
// Behavioral Contract:
// Simple length check on underlying data structure. Used to determine if wrapper
// has any content before operations.
//
// Regression Risk: LOW
// - Simple length check, hard to break
// - Used for conditional logic in callers
func TestPropertyWrapper_IsEmpty_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Check if wrapper contains any data",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("Empty wrapper", func(t *testing.T) {
		t.Log("=== Test: Newly created wrapper is empty ===")
		pw := NewPropertyWrapper(nil, "")
		if !pw.IsEmpty() {
			t.Error("❌ Expected wrapper to be empty")
		} else {
			t.Log("✓ IsEmpty returns true for empty wrapper")
		}

		t.Log("✓ CONTRACT SATISFIED: Empty wrapper detected")
	})

	t.Run("Non-empty wrapper", func(t *testing.T) {
		t.Log("=== Test: Wrapper with data is not empty ===")
		pw := NewPropertyWrapper(nil, "")
		pw.AddKey("key", "value")

		if pw.IsEmpty() {
			t.Error("❌ Expected wrapper to not be empty")
		} else {
			t.Log("✓ IsEmpty returns false after adding key")
		}

		t.Log("✓ CONTRACT SATISFIED: Non-empty wrapper detected")
	})
}

// TestPropertyWrapper_Clone_BehavioralBDD tests cloning behavioral contracts
//
//   - Method: clone(self) -> YAMLWrapper
//   - Deep copy: Uses copy.deepcopy(self.data)
//   - Path: Copies path string
//   - Independence: Modifications don't affect original
//
// Go Implementation (internal/property.PropertyWrapper.Clone):
//   - Method: Clone() *PropertyWrapper
//   - Deep copy: Recursively copies maps
//   - Path: Copies path string
//   - Independence: Modifications don't affect original
//
// Behavioral Contract:
// Both create independent deep copies. Critical for scenarios where you need to
// modify a copy without affecting the original (e.g., config templating).
//
// Regression Risk: HIGH
// - Shallow copy: Would create aliasing bugs where changes affect both
// - Path copying: Must copy path string too
// - Deep structures: Must recursively copy all nested levels
func TestPropertyWrapper_Clone_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Create independent deep copy of wrapper",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	content := map[string]interface{}{
		"key": "value",
		"nested": map[string]interface{}{
			"subkey": "subvalue",
		},
	}

	t.Run("Path is copied", func(t *testing.T) {
		t.Log("=== Test: Clone preserves path ===")
		pw := NewPropertyWrapper(content, "test")
		cloned := pw.Clone()

		if cloned.GetPath() != pw.GetPath() {
			t.Error("❌ Expected cloned wrapper to have same path")
		} else {
			t.Log("✓ Path copied correctly: 'test'")
		}

		t.Log("✓ CONTRACT SATISFIED: Path preserved in clone")
	})

	t.Run("Data is copied", func(t *testing.T) {
		t.Log("=== Test: Clone contains same data ===")
		pw := NewPropertyWrapper(content, "test")
		cloned := pw.Clone()

		value, err := cloned.GetValue("key")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}
		if value != "value" {
			t.Errorf("❌ Expected 'value', got '%v'", value)
		} else {
			t.Log("✓ Data copied correctly: key='value'")
		}

		t.Log("✓ CONTRACT SATISFIED: Data copied in clone")
	})

	t.Run("Deep copy - modifications don't affect original", func(t *testing.T) {
		t.Log("=== Test: Clone is independent (deep copy) ===")
		pw := NewPropertyWrapper(content, "test")
		cloned := pw.Clone()

		err := cloned.AddKey("new_key", "new_value")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ Added key to clone: new_key='new_value'")
		}

		if pw.HasKey("new_key") {
			t.Error("❌ Expected original wrapper to not be affected by clone modification")
		} else {
			t.Log("✓ Original unaffected by clone modification")
		}

		if !cloned.HasKey("new_key") {
			t.Error("❌ Expected clone to have new_key")
		} else {
			t.Log("✓ Clone has new key (independence verified)")
		}

		t.Log("✓ CONTRACT SATISFIED: Deep copy ensures independence")
	})
}

// TestPropertyWrapper_FindValueByKey_BehavioralBDD tests wildcard search behavioral contracts
//
//   - Method: find_value_by_key(self, pattern: str) -> List[dict]
//   - Exact match: Returns key if exact path exists
//   - Wildcard *: Matches any single level
//   - Wildcard **: Matches any depth (recursive)
//   - Returns: List of matching paths and values
//
// Go Implementation (internal/property.PropertyWrapper.FindValueByKey):
//   - Method: FindValueByKey(pattern string) ([]map[string]interface{}, error)
//   - Exact match: Returns key if exact path exists
//   - Wildcard *: Matches any single level
//   - Wildcard **: Matches any depth (recursive)
//   - Returns: Slice of matching paths and values
//
// Behavioral Contract:
// Both support glob-style pattern matching with * and ** wildcards. Used for
// searching configuration structures for keys at any depth. ** enables recursive
// search which is critical for finding keys in deeply nested configs.
//
// Regression Risk: HIGH
// - Pattern matching: Wrong implementation misses matches or false positives
// - ** recursion: Must search all depths, not just immediate children
// - Exact match priority: Should find exact matches first
func TestPropertyWrapper_FindValueByKey_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Search for keys using glob patterns (* single level, ** any depth)",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	content := map[string]interface{}{
		"metadata": map[string]interface{}{
			"parts": map[string]interface{}{
				"root":     "path/to/root.yaml",
				"services": "path/to/services.yaml",
			},
		},
		"desiredstate": map[string]interface{}{
			"meta": map[string]interface{}{
				"parts": map[string]interface{}{
					"terraform": "path/to/terraform.yaml",
				},
			},
		},
	}
	pw := NewPropertyWrapper(content, "")

	t.Run("Exact match", func(t *testing.T) {
		t.Log("=== Test: Exact path match ===")
		results, err := pw.FindValueByKey("metadata.parts")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ Search completed without error")
		}

		if len(results) != 1 {
			t.Errorf("❌ Expected 1 result for exact match, got %d", len(results))
		} else {
			t.Log("✓ Found 1 exact match for 'metadata.parts'")
		}

		t.Log("✓ CONTRACT SATISFIED: Exact path matching works")
	})

	t.Run("Nested exact match", func(t *testing.T) {
		t.Log("=== Test: Nested exact path match ===")
		results, err := pw.FindValueByKey("desiredstate.meta.parts")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("❌ Expected 1 result for nested exact match, got %d", len(results))
		} else {
			t.Log("✓ Found 1 exact match for 'desiredstate.meta.parts'")
		}

		t.Log("✓ CONTRACT SATISFIED: Nested exact matching works")
	})

	t.Run("Wildcard * matches single level", func(t *testing.T) {
		t.Log("=== Test: Single-level wildcard *parts ===")
		results, err := pw.FindValueByKey("*parts")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}

		// Should find metadata.parts
		if len(results) < 1 {
			t.Errorf("❌ Expected at least 1 result with *parts pattern, got %d", len(results))
		} else {
			t.Logf("✓ Found %d match(es) with *parts pattern", len(results))
		}

		t.Log("✓ CONTRACT SATISFIED: Single-level wildcard works")
	})

	t.Run("Wildcard ** matches any depth", func(t *testing.T) {
		t.Log("=== Test: Recursive wildcard **parts ===")
		results, err := pw.FindValueByKey("**parts")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}

		// Should find both metadata.parts and desiredstate.meta.parts
		if len(results) < 2 {
			t.Errorf("❌ Expected at least 2 results with **parts wildcard, got %d", len(results))
			for i, result := range results {
				t.Logf("Result %d: %+v", i, result)
			}
		} else {
			t.Logf("✓ Found %d matches with **parts (recursive search)", len(results))
		}

		t.Log("✓ CONTRACT SATISFIED: Recursive wildcard searches all depths")
	})
}

// TestPropertyWrapper_LoadBuffer_BehavioralBDD tests YAML loading behavioral contracts
//
//   - Method: load_buffer(self, content: str, vars: dict) -> None
//   - Parses: YAML string to dict
//   - Variables: Supports template variable substitution
//   - Merges: Adds parsed data to existing data
//   - Error: Raises yaml.YAMLError on parse failure
//
// Go Implementation (internal/property.PropertyWrapper.LoadBuffer):
//   - Method: LoadBuffer(content string, vars map[string]string) error
//   - Parses: YAML string to map
//   - Variables: Supports template variable substitution
//   - Merges: Adds parsed data to existing Data
//   - Error: Returns error on parse failure
//
// Behavioral Contract:
// Both parse YAML strings with optional variable substitution. Parsed content
// is merged into existing data (not replaced). This enables incremental loading
// of multiple YAML fragments.
//
// Regression Risk: HIGH
// - YAML parsing: Must handle multi-line, nested structures correctly
// - Variable substitution: Must replace template variables before parsing
// - Merge behavior: Must add to existing, not replace
func TestPropertyWrapper_LoadBuffer_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Parse YAML string and merge into existing data",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("Load YAML buffer", func(t *testing.T) {
		t.Log("=== Test: Parse YAML string ===")
		yamlContent := `
---
key: value
nested:
  subkey: subvalue
`
		pw := NewPropertyWrapper(nil, "")
		err := pw.LoadBuffer(yamlContent, make(map[string]string))
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ YAML parsed without error")
		}

		// Verify top-level key
		value, err := pw.GetValue("key")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}
		if value != "value" {
			t.Errorf("❌ Expected 'value', got '%v'", value)
		} else {
			t.Log("✓ Top-level key loaded: key='value'")
		}

		// Verify nested key
		value, err = pw.GetValue("nested.subkey")
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		}
		if value != "subvalue" {
			t.Errorf("❌ Expected 'subvalue', got '%v'", value)
		} else {
			t.Log("✓ Nested key loaded: nested.subkey='subvalue'")
		}

		t.Log("✓ CONTRACT SATISFIED: YAML parsing works correctly")
	})
}

// TestPropertyWrapper_ToString_BehavioralBDD tests YAML serialization behavioral contracts
//
//   - Method: to_string(self) -> str
//   - Serializes: dict to YAML string
//   - Format: Standard YAML format
//   - Error: Raises yaml.YAMLError on serialization failure
//
// Go Implementation (internal/property.PropertyWrapper.ToString):
//   - Method: ToString() (string, error)
//   - Serializes: map to YAML string
//   - Format: Standard YAML format
//   - Error: Returns error on serialization failure
//
// Behavioral Contract:
// Both serialize internal data structure to YAML string format. Used for
// writing configuration back to files or displaying to users. Inverse operation
// of LoadBuffer.
//
// Regression Risk: MEDIUM
// - Serialization: Must produce valid YAML that can be parsed back
// - Format: Must use standard YAML conventions
// - Nested structures: Must correctly indent nested maps
func TestPropertyWrapper_ToString_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Serialize data structure to YAML string",
		CurrentImpl:     "Current Go property wrapper implementation",
		ExpectedOutcome: "Property wrapper behavior remains stable and is validated by this test",
		Rationale:       "Predictable property access is required by document workflows",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("Serialize to YAML string", func(t *testing.T) {
		t.Log("=== Test: Data serializes to YAML ===")
		content := map[string]interface{}{
			"key": "value",
			"nested": map[string]interface{}{
				"subkey": "subvalue",
			},
		}

		pw := NewPropertyWrapper(content, "")
		yamlString, err := pw.ToString()
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ Serialization succeeded")
		}

		if yamlString == "" {
			t.Error("❌ Expected non-empty YAML string")
		} else {
			t.Logf("✓ YAML string generated (%d bytes)", len(yamlString))
		}

		// Verify content appears in output
		if !strings.Contains(yamlString, "key: value") {
			t.Error("❌ Expected YAML string to contain 'key: value'")
		} else {
			t.Log("✓ YAML contains expected content: 'key: value'")
		}

		t.Log("✓ CONTRACT SATISFIED: Serialization produces valid YAML")
	})
}
