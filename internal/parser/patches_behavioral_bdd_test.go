package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BehavioralContractJSONPatch represents behavioral contracts for JSON Patch operations
type BehavioralContractJSONPatch struct {
	Behavior    string
	Operation   string
	Example     string
	Description string
}

// TestJSONPatchEngine_BehavioralBDD tests JSON Patch (RFC 6902) operations behavioral contracts
//
// RFC 6902 Standard (JavaScript Object Notation (JSON) Patch):
// https://tools.ietf.org/html/rfc6902
//
// Operations:
//   - add:    Add value at path
//   - remove: Remove value at path
//   - replace: Replace value at path
//   - move:   Move value from source to destination
//   - copy:   Copy value from source to destination
//   - test:   Test assertion at path
//
// Go Implementation (internal/parser.JSONPatchEngine):
//   - Method: Apply(content map[string]interface{}, patches []JSONPatchOperation) (map[string]interface{}, error)
//   - RFC 6902 compliant
//   - Supports all 6 operations
//   - Validates patch structure
//
// Behavioral Contract:
// Apply a sequence of JSON Patch operations to transform a source document into
// a target document. Operations are applied in order. Each operation modifies the
// document and subsequent operations see the modified state.
func TestJSONPatchEngine_BehavioralBDD_Replace(t *testing.T) {
	contract := BehavioralContractJSONPatch{
		Behavior:  "RFC 6902 REPLACE operation - replace value at path",
		Operation: "replace",
		Example: `
patches := []JSONPatchOperation{
    {Op: "replace", Path: "/spec/replicas", Value: float64(3)},
}
result, err := engine.Apply(originalConfig, patches)
// Result: result["spec"].(map)["replicas"] == 3
`,
		Description: "Replace operation overwrites value at specified path",
	}

	_ = contract // Contract documentation

	// Test 1: Replace simple value
	t.Run("BDD-Replace-SimpleValue", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": float64(1),
			},
		}

		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/spec/replicas", Value: float64(3)},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		assert.Equal(t, float64(3), result["spec"].(map[string]interface{})["replicas"])
	})

	// Test 2: Replace nested value
	t.Run("BDD-Replace-NestedValue", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test-deployment",
			},
		}

		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/metadata/name", Value: "new-name"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		assert.Equal(t, "new-name", result["metadata"].(map[string]interface{})["name"])
	})

	// Test 3: Replace with complex object
	t.Run("BDD-Replace-ComplexObject", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"config": map[string]interface{}{
				"old": "value",
			},
		}

		newConfig := map[string]interface{}{
			"new": "config",
			"nested": map[string]interface{}{
				"value": float64(1),
			},
		}

		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/config", Value: newConfig},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		config := result["config"].(map[string]interface{})
		assert.Equal(t, "config", config["new"])
		assert.Equal(t, float64(1), config["nested"].(map[string]interface{})["value"])
	})

	// Test 4: Replace with null value
	t.Run("BDD-Replace-NullValue", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"field": "value",
		}

		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/field", Value: nil},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		assert.Nil(t, result["field"])
	})
}

// TestJSONPatchEngine_BehavioralBDD_Add tests the ADD operation
func TestJSONPatchEngine_BehavioralBDD_Add(t *testing.T) {
	contract := BehavioralContractJSONPatch{
		Behavior:  "RFC 6902 ADD operation - add value at path",
		Operation: "add",
		Example: `
patches := []JSONPatchOperation{
    {Op: "add", Path: "/metadata/version", Value: "1.0"},
}
result, err := engine.Apply(config, patches)
// Result: result["metadata"].(map)["version"] == "1.0"
`,
		Description: "Add operation adds value at path, creating intermediate objects as needed",
	}

	_ = contract

	// Test 1: Add new field
	t.Run("BDD-Add-NewField", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
			},
		}

		patches := []JSONPatchOperation{
			{Op: "add", Path: "/metadata/version", Value: "1.0"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		assert.Equal(t, "1.0", result["metadata"].(map[string]interface{})["version"])
	})

	// Test 2: Add creates intermediate objects
	t.Run("BDD-Add-CreatesIntermediateObjects", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{}

		patches := []JSONPatchOperation{
			{Op: "add", Path: "/new/nested/path", Value: "value"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		assert.Equal(t, "value", result["new"].(map[string]interface{})["nested"].(map[string]interface{})["path"])
	})

	// Test 3: Add to deeply nested path
	t.Run("BDD-Add-DeeplyNested", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "myapp",
						},
					},
				},
			},
		}

		patches := []JSONPatchOperation{
			{Op: "add", Path: "/spec/template/metadata/labels/env", Value: "production"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		labels := result["spec"].(map[string]interface{})["template"].(map[string]interface{})["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
		assert.Equal(t, "production", labels["env"])
		assert.Equal(t, "myapp", labels["app"]) // Original preserved
	})
}

// TestJSONPatchEngine_BehavioralBDD_Remove tests the REMOVE operation
func TestJSONPatchEngine_BehavioralBDD_Remove(t *testing.T) {
	contract := BehavioralContractJSONPatch{
		Behavior:  "RFC 6902 REMOVE operation - remove value at path",
		Operation: "remove",
		Example: `
patches := []JSONPatchOperation{
    {Op: "remove", Path: "/spec/deprecated"},
}
result, err := engine.Apply(config, patches)
// Result: "deprecated" not in result["spec"]
`,
		Description: "Remove operation deletes value at path",
	}

	_ = contract

	// Test 1: Remove field
	t.Run("BDD-Remove-Field", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas":   float64(1),
				"deprecated": "field",
			},
		}

		patches := []JSONPatchOperation{
			{Op: "remove", Path: "/spec/deprecated"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		_, exists := result["spec"].(map[string]interface{})["deprecated"]
		assert.False(t, exists)
		assert.Equal(t, float64(1), result["spec"].(map[string]interface{})["replicas"])
	})

	// Test 2: Remove nested field preserves siblings
	t.Run("BDD-Remove-PreservesSiblings", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app":  "myapp",
							"tier": "backend",
						},
					},
				},
			},
		}

		patches := []JSONPatchOperation{
			{Op: "remove", Path: "/spec/template/metadata/labels/tier"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		labels := result["spec"].(map[string]interface{})["template"].(map[string]interface{})["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
		assert.Equal(t, "myapp", labels["app"])
		_, exists := labels["tier"]
		assert.False(t, exists)
	})
}

// TestJSONPatchEngine_BehavioralBDD_Move tests the MOVE operation
func TestJSONPatchEngine_BehavioralBDD_Move(t *testing.T) {
	contract := BehavioralContractJSONPatch{
		Behavior:  "RFC 6902 MOVE operation - move value from source to destination",
		Operation: "move",
		Example: `
patches := []JSONPatchOperation{
    {Op: "move", Path: "/new_field", From: "/old_field"},
}
result, err := engine.Apply(config, patches)
// Result: "new_field" in result and "old_field" not in result
`,
		Description: "Move operation relocates value, removing from source",
	}

	_ = contract

	// Test 1: Move field
	t.Run("BDD-Move-Field", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"old_field": "value",
			"other":     "data",
		}

		patches := []JSONPatchOperation{
			{Op: "move", Path: "/new_field", From: "/old_field"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		assert.Equal(t, "value", result["new_field"])
		_, exists := result["old_field"]
		assert.False(t, exists)
		assert.Equal(t, "data", result["other"])
	})

	// Test 2: Move nested
	t.Run("BDD-Move-Nested", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"config": map[string]interface{}{
					"old_name": "value",
				},
			},
		}

		patches := []JSONPatchOperation{
			{Op: "move", Path: "/spec/renamed", From: "/spec/config"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		assert.Equal(t, "value", result["spec"].(map[string]interface{})["renamed"].(map[string]interface{})["old_name"])
		_, exists := result["spec"].(map[string]interface{})["config"]
		assert.False(t, exists)
	})
}

// TestJSONPatchEngine_BehavioralBDD_Copy tests the COPY operation
func TestJSONPatchEngine_BehavioralBDD_Copy(t *testing.T) {
	contract := BehavioralContractJSONPatch{
		Behavior:  "RFC 6902 COPY operation - copy value from source to destination",
		Operation: "copy",
		Example: `
patches := []JSONPatchOperation{
    {Op: "copy", Path: "/copy_field", From: "/original"},
}
result, err := engine.Apply(config, patches)
// Result: "copy_field" in result and "original" still in result (both present)
`,
		Description: "Copy operation duplicates value, source remains",
	}

	_ = contract

	// Test 1: Copy field
	t.Run("BDD-Copy-Field", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"field": "value",
		}

		patches := []JSONPatchOperation{
			{Op: "copy", Path: "/copy_field", From: "/field"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		assert.Equal(t, "value", result["copy_field"])
		assert.Equal(t, "value", result["field"])
	})

	// Test 2: Copy creates independent copy
	t.Run("BDD-Copy-IndependentCopy", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"original": map[string]interface{}{
				"nested": map[string]interface{}{
					"value": float64(1),
				},
			},
		}

		patches := []JSONPatchOperation{
			{Op: "copy", Path: "/duplicate", From: "/original"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)
		assert.Equal(t, float64(1), result["duplicate"].(map[string]interface{})["nested"].(map[string]interface{})["value"])
		assert.Equal(t, float64(1), result["original"].(map[string]interface{})["nested"].(map[string]interface{})["value"])
	})
}

// TestJSONPatchEngine_BehavioralBDD_Test tests the TEST operation
func TestJSONPatchEngine_BehavioralBDD_Test(t *testing.T) {
	contract := BehavioralContractJSONPatch{
		Behavior:  "RFC 6902 TEST operation - assert value at path",
		Operation: "test",
		Example: `
patches := []JSONPatchOperation{
    {Op: "test", Path: "/spec/replicas", Value: float64(1)},
}
err := engine.Test(config, patches)
// err == nil if value matches
// err != nil if value doesn't match
`,
		Description: "Test operation validates value at path",
	}

	_ = contract

	// Test 1: Test passes
	t.Run("BDD-Test-Passes", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": float64(1),
			},
		}

		result := engine.Test(content, "/spec/replicas", float64(1))
		assert.True(t, result)
	})

	// Test 2: Test fails
	t.Run("BDD-Test-Fails", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": float64(1),
			},
		}

		result := engine.Test(content, "/spec/replicas", float64(999))
		assert.False(t, result)
	})
}

// TestJSONPatchEngine_BehavioralBDD_Sequence tests multiple operations in sequence
func TestJSONPatchEngine_BehavioralBDD_Sequence(t *testing.T) {
	contract := BehavioralContractJSONPatch{
		Behavior:  "RFC 6902 SEQUENCE - apply multiple operations in order",
		Operation: "multiple",
		Example: `
patches := []JSONPatchOperation{
    {Op: "replace", Path: "/spec/replicas", Value: float64(5)},
    {Op: "add", Path: "/spec/template/metadata/labels/tier", Value: "backend"},
    {Op: "remove", Path: "/spec/deprecated"},
}
result, err := engine.Apply(config, patches)
// Each operation sees the result of previous operations
`,
		Description: "Operations are applied sequentially with each seeing modified state",
	}

	_ = contract

	// Test 1: Multiple patches in sequence
	t.Run("BDD-Sequence-MultipleOperations", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas":   float64(1),
				"deprecated": "field",
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "myapp",
						},
					},
				},
			},
		}

		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/spec/replicas", Value: float64(5)},
			{Op: "add", Path: "/spec/template/metadata/labels/tier", Value: "backend"},
			{Op: "remove", Path: "/spec/deprecated"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)

		spec := result["spec"].(map[string]interface{})
		assert.Equal(t, float64(5), spec["replicas"])
		_, exists := spec["deprecated"]
		assert.False(t, exists)

		labels := spec["template"].(map[string]interface{})["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
		assert.Equal(t, "backend", labels["tier"])
		assert.Equal(t, "myapp", labels["app"])
	})

	// Test 2: Interdependent operations
	t.Run("BDD-Sequence-Interdependent", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"value": float64(1),
		}

		patches := []JSONPatchOperation{
			{Op: "add", Path: "/newfield", Value: "data"},
			{Op: "move", Path: "/moved", From: "/newfield"},
			{Op: "copy", Path: "/copied", From: "/moved"},
		}

		result, err := engine.Apply(content, patches)
		require.NoError(t, err)

		assert.Equal(t, "data", result["moved"])
		assert.Equal(t, "data", result["copied"])
		_, exists := result["newfield"]
		assert.False(t, exists)
	})
}

// TestJSONPatchEngine_BehavioralBDD_AtomicStrict tests strict mode (atomic operations)
func TestJSONPatchEngine_BehavioralBDD_AtomicStrict(t *testing.T) {
	contract := BehavioralContractJSONPatch{
		Behavior:  "RFC 6902 STRICT MODE - atomic test-then-apply semantics",
		Operation: "strict",
		Example: `
patches := []JSONPatchOperation{
    {Op: "test", Path: "/spec/replicas", Value: float64(1)},
    {Op: "replace", Path: "/spec/replicas", Value: float64(5)},
}
result, err := engine.ApplyStrict(config, patches)
// All tests run first. If any fail, no changes applied.
// If all pass, mutations applied.
`,
		Description: "Strict mode validates all tests before applying any mutations",
	}

	_ = contract

	// Test 1: Strict mode passes
	t.Run("BDD-Strict-AllTestsPass", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": float64(1),
			},
		}

		patches := []JSONPatchOperation{
			{Op: "test", Path: "/spec/replicas", Value: float64(1)},
			{Op: "replace", Path: "/spec/replicas", Value: float64(5)},
		}

		result, err := engine.ApplyStrict(content, patches)
		require.NoError(t, err)
		assert.Equal(t, float64(5), result["spec"].(map[string]interface{})["replicas"])
	})

	// Test 2: Strict mode fails - changes not applied
	t.Run("BDD-Strict-TestFails-NoChangesApplied", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		originalContent := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": float64(1),
			},
		}

		patches := []JSONPatchOperation{
			{Op: "test", Path: "/spec/replicas", Value: float64(999)},
			{Op: "replace", Path: "/spec/replicas", Value: float64(5)},
		}

		_, err := engine.ApplyStrict(originalContent, patches)
		require.Error(t, err)

		// Original should be unchanged
		assert.Equal(t, float64(1), originalContent["spec"].(map[string]interface{})["replicas"])
	})
}

// TestJSONPatchEngine_BehavioralBDD_ErrorHandling tests error cases
func TestJSONPatchEngine_BehavioralBDD_ErrorHandling(t *testing.T) {
	// Test 1: Invalid operation
	t.Run("BDD-Error-InvalidOperation", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		patches := []JSONPatchOperation{{Op: "invalid", Path: "/test"}}

		_, err := engine.Apply(map[string]interface{}{}, patches)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid operation")
	})

	// Test 2: Missing required field for move (no "from")
	t.Run("BDD-Error-MissingRequiredField", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		patches := []JSONPatchOperation{{Op: "move", Path: "/test"}}

		_, err := engine.Apply(map[string]interface{}{}, patches)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires 'from'")
	})

	// Test 3: Path not found
	t.Run("BDD-Error-PathNotFound", func(t *testing.T) {
		engine := NewJSONPatchEngine()
		content := map[string]interface{}{"spec": map[string]interface{}{}}
		patches := []JSONPatchOperation{{Op: "remove", Path: "/nonexistent"}}

		_, err := engine.Apply(content, patches)
		require.Error(t, err)
	})
}
