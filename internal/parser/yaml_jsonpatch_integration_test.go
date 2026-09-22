package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestYAMLHandler_ApplyJSONPatch_BehavioralBDD tests YAMLHandler integration with JSON Patch Engine
func TestYAMLHandler_ApplyJSONPatch_BehavioralBDD(t *testing.T) {
	// Behavioral contract documentation
	_ = BehavioralContractJSONPatch{
		Behavior:  "YAMLHandler convenience methods delegate to JSONPatchEngine",
		Operation: "integration",
		Description: `
YAMLHandler provides convenience methods that wrap the JSONPatchEngine.
These methods allow YAML handler users to apply patches directly without
instantiating the engine separately. Perfect for workflows that load YAML
and need to apply patches before validation or output.
`,
	}

	t.Run("BDD-ApplyJSONPatch-BasicOperations", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		content := map[string]interface{}{
			"metadata": map[string]interface{}{
				"version": "1.0",
				"name":    "myapp",
			},
			"spec": map[string]interface{}{
				"replicas": float64(1),
				"image":    "nginx:latest",
			},
		}

		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/metadata/version", Value: "2.0"},
			{Op: "add", Path: "/spec/port", Value: float64(8080)},
		}

		result, err := handler.ApplyJSONPatch(content, patches)
		require.NoError(t, err)
		assert.Equal(t, "2.0", result["metadata"].(map[string]interface{})["version"])
		assert.Equal(t, float64(8080), result["spec"].(map[string]interface{})["port"])
	})

	t.Run("BDD-ApplyJSONPatch-NestedOperations", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		content := map[string]interface{}{
			"config": map[string]interface{}{
				"database": map[string]interface{}{
					"host": "localhost",
					"port": float64(5432),
				},
			},
		}

		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/config/database/host", Value: "db.example.com"},
			{Op: "add", Path: "/config/database/ssl", Value: true},
		}

		result, err := handler.ApplyJSONPatch(content, patches)
		require.NoError(t, err)
		db := result["config"].(map[string]interface{})["database"].(map[string]interface{})
		assert.Equal(t, "db.example.com", db["host"])
		assert.Equal(t, true, db["ssl"])
	})

	t.Run("BDD-ApplyJSONPatch-RemoveOperation", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"deprecated": "value",
				"active":     true,
			},
		}

		patches := []JSONPatchOperation{
			{Op: "remove", Path: "/spec/deprecated"},
		}

		result, err := handler.ApplyJSONPatch(content, patches)
		require.NoError(t, err)
		spec := result["spec"].(map[string]interface{})
		assert.NotContains(t, spec, "deprecated")
		assert.Equal(t, true, spec["active"])
	})
}

// TestYAMLHandler_ApplyJSONPatchStrict_BehavioralBDD tests strict (atomic) mode through YAMLHandler
func TestYAMLHandler_ApplyJSONPatchStrict_BehavioralBDD(t *testing.T) {
	// Behavioral contract documentation
	_ = BehavioralContractJSONPatch{
		Behavior:  "YAMLHandler ApplyJSONPatchStrict provides atomic application",
		Operation: "strict-integration",
		Description: `
Strict mode ensures atomicity: either all patches are applied or none are.
This is perfect for scenarios where you need to ensure consistency,
such as version upgrades where all changes must succeed together.
`,
	}

	t.Run("BDD-Strict-AllTestsPass", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		content := map[string]interface{}{
			"metadata": map[string]interface{}{"version": "1.0"},
			"spec":     map[string]interface{}{"replicas": float64(1)},
		}

		patches := []JSONPatchOperation{
			{Op: "test", Path: "/metadata/version", Value: "1.0"},
			{Op: "replace", Path: "/metadata/version", Value: "2.0"},
		}

		result, err := handler.ApplyJSONPatchStrict(content, patches)
		require.NoError(t, err)
		assert.Equal(t, "2.0", result["metadata"].(map[string]interface{})["version"])
	})

	t.Run("BDD-Strict-TestFails-NoChangesApplied", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		content := map[string]interface{}{
			"metadata": map[string]interface{}{"version": "1.0"},
		}

		patches := []JSONPatchOperation{
			{Op: "test", Path: "/metadata/version", Value: "2.0"}, // This will fail
			{Op: "replace", Path: "/metadata/version", Value: "3.0"},
		}

		result, err := handler.ApplyJSONPatchStrict(content, patches)
		require.Error(t, err)
		// When test fails, result is nil and content is unchanged
		if result != nil {
			assert.Equal(t, "1.0", result["metadata"].(map[string]interface{})["version"])
		}
		// Original should still be 1.0
		assert.Equal(t, "1.0", content["metadata"].(map[string]interface{})["version"])
	})

	t.Run("BDD-Strict-OriginalUnchanged", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		original := map[string]interface{}{
			"data": map[string]interface{}{"value": "original"},
		}

		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/data/value", Value: "modified"},
		}

		result, err := handler.ApplyJSONPatchStrict(original, patches)
		require.NoError(t, err)
		// Original should be unchanged
		assert.Equal(t, "original", original["data"].(map[string]interface{})["value"])
		// Result should be modified
		assert.Equal(t, "modified", result["data"].(map[string]interface{})["value"])
	})
}

// TestYAMLHandler_TestJSONPatchValue_BehavioralBDD tests value validation through YAMLHandler
func TestYAMLHandler_TestJSONPatchValue_BehavioralBDD(t *testing.T) {
	// Behavioral contract documentation
	_ = BehavioralContractJSONPatch{
		Behavior:  "YAMLHandler TestJSONPatchValue validates values at paths",
		Operation: "test-integration",
		Description: `
The Test method allows you to validate that a value matches before deciding
to apply patches. This is useful for conditional patching based on current state.
`,
	}

	t.Run("BDD-TestValue-Passes", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		content := map[string]interface{}{
			"metadata": map[string]interface{}{
				"version": "1.0",
			},
		}

		result := handler.TestJSONPatchValue(content, "/metadata/version", "1.0")
		assert.True(t, result)
	})

	t.Run("BDD-TestValue-Fails", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		content := map[string]interface{}{
			"metadata": map[string]interface{}{
				"version": "1.0",
			},
		}

		result := handler.TestJSONPatchValue(content, "/metadata/version", "2.0")
		assert.False(t, result)
	})

	t.Run("BDD-TestValue-NestedPath", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		content := map[string]interface{}{
			"config": map[string]interface{}{
				"database": map[string]interface{}{
					"ssl": true,
				},
			},
		}

		result := handler.TestJSONPatchValue(content, "/config/database/ssl", true)
		assert.True(t, result)
	})

	t.Run("BDD-TestValue-NullValue", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		content := map[string]interface{}{
			"data": nil,
		}

		result := handler.TestJSONPatchValue(content, "/data", nil)
		assert.True(t, result)
	})
}

// TestYAMLHandler_JSONPatchIntegration_BehavioralBDD tests full workflow integration
func TestYAMLHandler_JSONPatchIntegration_BehavioralBDD(t *testing.T) {
	// Behavioral contract documentation
	_ = BehavioralContractJSONPatch{
		Behavior:  "YAMLHandler JSON Patch integration enables complete patching workflows",
		Operation: "workflow-integration",
		Description: `
This test demonstrates a complete workflow: load YAML, validate state, apply patches,
verify results, and convert back to YAML. This is the typical usage pattern for
configuration transformation and management operations.
`,
	}

	t.Run("BDD-Workflow-LoadPatchConvert", func(t *testing.T) {
		handler := NewYAMLHandler(".")

		// Start with initial content
		content := map[string]interface{}{
			"app": map[string]interface{}{
				"name":    "myapp",
				"version": "1.0",
			},
			"database": map[string]interface{}{
				"host": "localhost",
				"port": float64(5432),
			},
		}

		// Verify state before patching
		assert.True(t, handler.TestJSONPatchValue(content, "/app/version", "1.0"))

		// Apply patches
		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/app/version", Value: "2.0"},
			{Op: "replace", Path: "/database/host", Value: "db.prod.com"},
		}

		result, err := handler.ApplyJSONPatch(content, patches)
		require.NoError(t, err)

		// Verify patches applied
		assert.Equal(t, "2.0", result["app"].(map[string]interface{})["version"])
		assert.Equal(t, "db.prod.com", result["database"].(map[string]interface{})["host"])

		// Verify original unchanged
		assert.Equal(t, "1.0", content["app"].(map[string]interface{})["version"])

		// Convert to string (for file output)
		yamlStr, err := handler.ToString(result)
		require.NoError(t, err)
		assert.Contains(t, yamlStr, "version: \"2.0\"")
		assert.Contains(t, yamlStr, "host: db.prod.com")
	})

	t.Run("BDD-Workflow-ComplexPatching", func(t *testing.T) {
		handler := NewYAMLHandler(".")

		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"replicas": float64(1),
				"selector": map[string]interface{}{
					"tier": "frontend",
				},
			},
		}

		// Multi-step patch sequence
		patches := []JSONPatchOperation{
			{Op: "add", Path: "/spec/template", Value: map[string]interface{}{}},
			{Op: "add", Path: "/spec/template/metadata", Value: map[string]interface{}{}},
			{Op: "add", Path: "/spec/template/metadata/labels", Value: map[string]interface{}{"tier": "frontend"}},
			{Op: "replace", Path: "/spec/replicas", Value: float64(3)},
		}

		result, err := handler.ApplyJSONPatch(content, patches)
		require.NoError(t, err)

		// Verify structure
		spec := result["spec"].(map[string]interface{})
		assert.Equal(t, float64(3), spec["replicas"])
		assert.NotNil(t, spec["template"])
	})
}
