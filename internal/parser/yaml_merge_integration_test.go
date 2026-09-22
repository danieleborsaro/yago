package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestYAMLHandler_StrategicMerge_BehavioralBDD tests YAMLHandler strategic merge integration
func TestYAMLHandler_StrategicMerge_BehavioralBDD(t *testing.T) {
	// Behavioral contract: YAMLHandler strategic merge integration
	_ = BehavioralContractJSONPatch{
		Behavior:    "YAMLHandler.StrategicMerge integration",
		Operation:   "integration-merge",
		Description: "YAMLHandler provides convenient methods for field-aware merging",
	}

	t.Run("BDD-YAMLHandler-BasicMerge", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		base := map[string]interface{}{
			"app": map[string]interface{}{
				"name":    "myapp",
				"version": "1.0",
			},
		}
		source := map[string]interface{}{
			"app": map[string]interface{}{
				"version": "2.0",
			},
		}

		engine := NewStrategicMergeEngine(nil)
		result, err := handler.StrategicMerge(base, source, engine)
		require.NoError(t, err)
		assert.Equal(t, "2.0", result["app"].(map[string]interface{})["version"])
		assert.Equal(t, "myapp", result["app"].(map[string]interface{})["name"])
	})

	t.Run("BDD-YAMLHandler-MergeWithConfig", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		base := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "web", "image": "nginx:1.0"},
				},
			},
		}
		source := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "web", "image": "nginx:2.0"},
					map[string]interface{}{"name": "sidecar", "image": "sidecar:1.0"},
				},
			},
		}

		config := StrategicMergeConfig{
			"/spec/containers": "MERGE_BY_KEY:name",
		}
		result, err := handler.StrategicMergeWithConfig(base, source, config)
		require.NoError(t, err)

		containers := result["spec"].(map[string]interface{})["containers"].([]interface{})
		assert.Equal(t, 2, len(containers))
	})

	t.Run("BDD-YAMLHandler-NestedMergeConfig", func(t *testing.T) {
		handler := NewYAMLHandler(".")
		base := map[string]interface{}{
			"spec": map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "LOG_LEVEL", "value": "info"},
				},
				"volumes": []interface{}{
					map[string]interface{}{"name": "vol1"},
				},
			},
		}
		source := map[string]interface{}{
			"spec": map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "LOG_LEVEL", "value": "debug"},
					map[string]interface{}{"name": "API_KEY", "value": "secret"},
				},
				"volumes": []interface{}{
					map[string]interface{}{"name": "vol2"},
				},
			},
		}

		config := StrategicMergeConfig{
			"/spec/env":     "MERGE_BY_KEY:name",
			"/spec/volumes": "APPEND",
		}
		result, err := handler.StrategicMergeWithConfig(base, source, config)
		require.NoError(t, err)

		// Check env (MERGE_BY_KEY)
		env := result["spec"].(map[string]interface{})["env"].([]interface{})
		assert.Equal(t, 2, len(env))

		// Check volumes (APPEND)
		volumes := result["spec"].(map[string]interface{})["volumes"].([]interface{})
		assert.Equal(t, 2, len(volumes))
	})
}

// TestYAMLHandler_StrategicMergeWorkflow_BehavioralBDD tests complete merge workflows
func TestYAMLHandler_StrategicMergeWorkflow_BehavioralBDD(t *testing.T) {
	// Behavioral contract: Complete merge workflow
	_ = BehavioralContractJSONPatch{
		Behavior:    "YAMLHandler strategic merge complete workflow",
		Operation:   "workflow-merge",
		Description: "Load YAML → merge with strategy → convert back to YAML",
	}

	t.Run("BDD-Workflow-LoadMergeConvert", func(t *testing.T) {
		handler := NewYAMLHandler(".")

		// Load base configuration
		base := map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":    "myapp",
				"version": "1.0",
				"labels":  []interface{}{"prod", "web"},
			},
			"spec": map[string]interface{}{
				"replicas": float64(1),
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": "myapp:1.0",
						"ports": float64(8080),
					},
				},
				"env": []interface{}{
					map[string]interface{}{"name": "LOG_LEVEL", "value": "info"},
				},
			},
		}

		// Load overlay configuration
		overlay := map[string]interface{}{
			"metadata": map[string]interface{}{
				"version": "2.0",
				"labels":  []interface{}{"prod", "api"},
			},
			"spec": map[string]interface{}{
				"replicas": float64(3),
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": "myapp:2.0",
						"cpu":   "500m",
					},
					map[string]interface{}{
						"name":  "sidecar",
						"image": "sidecar:1.0",
					},
				},
				"env": []interface{}{
					map[string]interface{}{"name": "LOG_LEVEL", "value": "debug"},
					map[string]interface{}{"name": "API_KEY", "value": "secret"},
				},
			},
		}

		// Define merge strategies
		config := StrategicMergeConfig{
			"/metadata/labels": "UNION",
			"/spec/containers": "MERGE_BY_KEY:name",
			"/spec/env":        "MERGE_BY_KEY:name",
		}

		// Merge with strategies
		result, err := handler.StrategicMergeWithConfig(base, overlay, config)
		require.NoError(t, err)

		// Verify metadata
		metadata := result["metadata"].(map[string]interface{})
		assert.Equal(t, "2.0", metadata["version"])
		assert.Equal(t, "myapp", metadata["name"])
		labels := metadata["labels"].([]interface{})
		assert.Equal(t, 3, len(labels)) // Union: prod, web, api

		// Verify spec
		spec := result["spec"].(map[string]interface{})
		assert.Equal(t, float64(3), spec["replicas"])

		// Verify containers (merged by name)
		containers := spec["containers"].([]interface{})
		assert.Equal(t, 2, len(containers))

		var appContainer map[string]interface{}
		for _, c := range containers {
			if c.(map[string]interface{})["name"] == "app" {
				appContainer = c.(map[string]interface{})
				break
			}
		}
		require.NotNil(t, appContainer)
		assert.Equal(t, "myapp:2.0", appContainer["image"])
		assert.Equal(t, "500m", appContainer["cpu"])
		assert.Equal(t, float64(8080), appContainer["ports"]) // Preserved from base

		// Verify env (merged by name)
		env := spec["env"].([]interface{})
		assert.Equal(t, 2, len(env))

		// Convert back to YAML
		yamlStr, err := handler.ToString(result)
		require.NoError(t, err)
		assert.Contains(t, yamlStr, "version: \"2.0\"")
		assert.Contains(t, yamlStr, "replicas: 3")
	})

	t.Run("BDD-Workflow-LayeredConfig", func(t *testing.T) {
		handler := NewYAMLHandler(".")

		// Start with base config
		base := map[string]interface{}{
			"database": map[string]interface{}{
				"host": "localhost",
				"port": float64(5432),
			},
			"cache": map[string]interface{}{
				"enabled": true,
			},
		}

		// Layer 1: Dev overrides
		devConfig := map[string]interface{}{
			"database": map[string]interface{}{
				"host": "localhost-dev",
			},
		}

		// Layer 2: Test overrides
		testConfig := map[string]interface{}{
			"database": map[string]interface{}{
				"host": "test-db",
			},
			"cache": map[string]interface{}{
				"enabled": false,
			},
		}

		// Apply dev config
		result1, _ := handler.StrategicMergeWithConfig(base, devConfig, nil)
		assert.Equal(t, "localhost-dev", result1["database"].(map[string]interface{})["host"])

		// Apply test config on top
		result2, _ := handler.StrategicMergeWithConfig(result1, testConfig, nil)
		assert.Equal(t, "test-db", result2["database"].(map[string]interface{})["host"])
		assert.Equal(t, false, result2["cache"].(map[string]interface{})["enabled"])
		assert.Equal(t, float64(5432), result2["database"].(map[string]interface{})["port"])
	})

	t.Run("BDD-Workflow-PreventDataLoss", func(t *testing.T) {
		handler := NewYAMLHandler(".")

		// Base has complete container spec
		base := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":          "app",
						"image":         "app:1.0",
						"resources":     map[string]interface{}{"cpu": "100m"},
						"livenessProbe": map[string]interface{}{"periodSeconds": float64(10)},
					},
				},
			},
		}

		// Overlay only updates image
		overlay := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": "app:2.0",
					},
				},
			},
		}

		// With MERGE_BY_KEY, other properties are preserved
		config := StrategicMergeConfig{
			"/spec/containers": "MERGE_BY_KEY:name",
		}

		result, _ := handler.StrategicMergeWithConfig(base, overlay, config)
		container := result["spec"].(map[string]interface{})["containers"].([]interface{})[0].(map[string]interface{})

		// Verify data not lost
		assert.Equal(t, "app:2.0", container["image"])
		assert.NotNil(t, container["resources"])
		assert.NotNil(t, container["livenessProbe"])
	})
}

// TestYAMLHandler_StrategicMergeIntegration_BehavioralBDD tests integration with other features
func TestYAMLHandler_StrategicMergeIntegration_BehavioralBDD(t *testing.T) {
	// Behavioral contract: Integration with JSON Patch
	_ = BehavioralContractJSONPatch{
		Behavior:    "Strategic Merge integrates with JSON Patch",
		Operation:   "integration-patch-merge",
		Description: "Can combine strategic merge with JSON patches in workflows",
	}

	t.Run("BDD-Integration-MergeAndPatch", func(t *testing.T) {
		handler := NewYAMLHandler(".")

		// Start with base config
		base := map[string]interface{}{
			"app": map[string]interface{}{
				"version": "1.0",
				"name":    "myapp",
			},
			"spec": map[string]interface{}{
				"replicas": float64(1),
			},
		}

		// Merge with overlay using strategy
		overlay := map[string]interface{}{
			"app": map[string]interface{}{
				"version": "2.0",
			},
			"spec": map[string]interface{}{
				"replicas": float64(3),
			},
		}

		merged, _ := handler.StrategicMergeWithConfig(base, overlay, nil)

		// Then apply patch to fine-tune
		patches := []JSONPatchOperation{
			{Op: "add", Path: "/metadata/environment", Value: "prod"},
			{Op: "replace", Path: "/spec/replicas", Value: float64(5)},
		}

		final, err := handler.ApplyJSONPatch(merged, patches)
		require.NoError(t, err)

		assert.Equal(t, "2.0", final["app"].(map[string]interface{})["version"])
		assert.Equal(t, float64(5), final["spec"].(map[string]interface{})["replicas"])
		assert.Equal(t, "prod", final["metadata"].(map[string]interface{})["environment"])
	})

	t.Run("BDD-Integration-MergeWithValidation", func(t *testing.T) {
		handler := NewYAMLHandler(".")

		base := map[string]interface{}{
			"config": map[string]interface{}{
				"version":  "1.0",
				"features": []interface{}{"feature1", "feature2"},
			},
		}

		overlay := map[string]interface{}{
			"config": map[string]interface{}{
				"version":  "2.0",
				"features": []interface{}{"feature3"},
			},
		}

		// First validate current state
		versionOK := handler.TestJSONPatchValue(base, "/config/version", "1.0")
		assert.True(t, versionOK)

		// Merge with union strategy for features
		merged, _ := handler.StrategicMergeWithConfig(base, overlay, StrategicMergeConfig{
			"/config/features": "UNION",
		})

		// Verify merged state
		features := merged["config"].(map[string]interface{})["features"].([]interface{})
		assert.Equal(t, 3, len(features))
	})
}
