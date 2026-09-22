package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStrategicMergeEngine_BehavioralBDD tests Strategic Merge operations with behavioral contracts
func TestStrategicMergeEngine_BehavioralBDD_Replace(t *testing.T) {
	// Behavioral contract: Default REPLACE strategy
	_ = BehavioralContractJSONPatch{
		Behavior:    "Strategic Merge with REPLACE strategy (default)",
		Operation:   "merge-replace",
		Description: "When no strategy is specified, merge uses REPLACE strategy: source values override base values",
	}

	t.Run("BDD-Replace-SimpleValues", func(t *testing.T) {
		engine := NewStrategicMergeEngine(nil)
		base := map[string]interface{}{
			"app": map[string]interface{}{
				"version": "1.0",
				"name":    "myapp",
			},
		}
		source := map[string]interface{}{
			"app": map[string]interface{}{
				"version": "2.0",
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		assert.Equal(t, "2.0", result["app"].(map[string]interface{})["version"])
		assert.Equal(t, "myapp", result["app"].(map[string]interface{})["name"])
		// Original unchanged
		assert.Equal(t, "1.0", base["app"].(map[string]interface{})["version"])
	})

	t.Run("BDD-Replace-NestedStructure", func(t *testing.T) {
		engine := NewStrategicMergeEngine(nil)
		base := map[string]interface{}{
			"spec": map[string]interface{}{
				"config": map[string]interface{}{
					"debug": true,
					"level": "info",
				},
			},
		}
		source := map[string]interface{}{
			"spec": map[string]interface{}{
				"config": map[string]interface{}{
					"level": "debug",
				},
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		config := result["spec"].(map[string]interface{})["config"].(map[string]interface{})
		assert.Equal(t, "debug", config["level"])
		assert.Equal(t, true, config["debug"])
	})

	t.Run("BDD-Replace-ArrayReplacement", func(t *testing.T) {
		engine := NewStrategicMergeEngine(nil)
		base := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": "1", "name": "item1"},
				map[string]interface{}{"id": "2", "name": "item2"},
			},
		}
		source := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": "3", "name": "item3"},
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		items := result["items"].([]interface{})
		// Replace strategy means source array replaces base array
		assert.Equal(t, 1, len(items))
		assert.Equal(t, "3", items[0].(map[string]interface{})["id"])
	})
}

// TestStrategicMergeEngine_BehavioralBDD_Append tests APPEND merge strategy
func TestStrategicMergeEngine_BehavioralBDD_Append(t *testing.T) {
	// Behavioral contract: APPEND strategy
	_ = BehavioralContractJSONPatch{
		Behavior:    "Strategic Merge with APPEND strategy",
		Operation:   "merge-append",
		Description: "APPEND strategy appends source array items to base array items instead of replacing",
	}

	t.Run("BDD-Append-SimpleArrays", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/items": "APPEND",
		})
		base := map[string]interface{}{
			"items": []interface{}{
				"item1",
				"item2",
			},
		}
		source := map[string]interface{}{
			"items": []interface{}{
				"item3",
				"item4",
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		items := result["items"].([]interface{})
		assert.Equal(t, 4, len(items))
		assert.Equal(t, "item1", items[0])
		assert.Equal(t, "item4", items[3])
		// Original unchanged
		assert.Equal(t, 2, len(base["items"].([]interface{})))
	})

	t.Run("BDD-Append-ObjectArrays", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/volumes": "APPEND",
		})
		base := map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{"name": "vol1"},
			},
		}
		source := map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{"name": "vol2"},
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		volumes := result["volumes"].([]interface{})
		assert.Equal(t, 2, len(volumes))
		assert.Equal(t, "vol1", volumes[0].(map[string]interface{})["name"])
		assert.Equal(t, "vol2", volumes[1].(map[string]interface{})["name"])
	})
}

// TestStrategicMergeEngine_BehavioralBDD_Union tests UNION merge strategy
func TestStrategicMergeEngine_BehavioralBDD_Union(t *testing.T) {
	// Behavioral contract: UNION strategy
	_ = BehavioralContractJSONPatch{
		Behavior:    "Strategic Merge with UNION strategy",
		Operation:   "merge-union",
		Description: "UNION strategy creates union of arrays (unique values only)",
	}

	t.Run("BDD-Union-RemovesDuplicates", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/tags": "UNION",
		})
		base := map[string]interface{}{
			"tags": []interface{}{
				"tag1",
				"tag2",
				"tag3",
			},
		}
		source := map[string]interface{}{
			"tags": []interface{}{
				"tag2",
				"tag4",
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		tags := result["tags"].([]interface{})
		assert.Equal(t, 4, len(tags))
		// Should contain all unique values
		tagStrs := make([]string, len(tags))
		for i, t := range tags {
			tagStrs[i] = t.(string)
		}
		assert.Contains(t, tagStrs, "tag1")
		assert.Contains(t, tagStrs, "tag2")
		assert.Contains(t, tagStrs, "tag3")
		assert.Contains(t, tagStrs, "tag4")
	})

	t.Run("BDD-Union-ObjectArrays", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/labels": "UNION",
		})
		base := map[string]interface{}{
			"labels": []interface{}{
				map[string]interface{}{"key": "env", "value": "prod"},
				map[string]interface{}{"key": "tier", "value": "frontend"},
			},
		}
		source := map[string]interface{}{
			"labels": []interface{}{
				map[string]interface{}{"key": "env", "value": "prod"},
				map[string]interface{}{"key": "team", "value": "backend"},
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		labels := result["labels"].([]interface{})
		// Should have 3 unique items (duplicate "env/prod" removed)
		assert.Equal(t, 3, len(labels))
	})
}

// TestStrategicMergeEngine_BehavioralBDD_MergeByKey tests MERGE_BY_KEY strategy
func TestStrategicMergeEngine_BehavioralBDD_MergeByKey(t *testing.T) {
	// Behavioral contract: MERGE_BY_KEY strategy
	_ = BehavioralContractJSONPatch{
		Behavior:    "Strategic Merge with MERGE_BY_KEY strategy",
		Operation:   "merge-by-key",
		Description: "MERGE_BY_KEY merges arrays by matching a key field, recursively merging matched items",
	}

	t.Run("BDD-MergeByKey-ContainersMerge", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/spec/containers": "MERGE_BY_KEY:name",
		})
		base := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":    "web",
						"image":   "nginx:1.0",
						"ports":   []interface{}{float64(80)},
						"enabled": true,
					},
					map[string]interface{}{
						"name":  "db",
						"image": "postgres:11",
					},
				},
			},
		}
		source := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "web",
						"image": "nginx:2.0",
					},
					map[string]interface{}{
						"name":  "cache",
						"image": "redis:6",
					},
				},
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)

		containers := result["spec"].(map[string]interface{})["containers"].([]interface{})
		assert.Equal(t, 3, len(containers))

		// Find merged web container
		var webContainer map[string]interface{}
		for _, c := range containers {
			if c.(map[string]interface{})["name"] == "web" {
				webContainer = c.(map[string]interface{})
				break
			}
		}

		require.NotNil(t, webContainer)
		assert.Equal(t, "nginx:2.0", webContainer["image"])
		// Properties from base that weren't in source should be preserved
		assert.Equal(t, true, webContainer["enabled"])

		// Find db container (only in base)
		var dbContainer map[string]interface{}
		for _, c := range containers {
			if c.(map[string]interface{})["name"] == "db" {
				dbContainer = c.(map[string]interface{})
				break
			}
		}
		require.NotNil(t, dbContainer)

		// Find cache container (only in source)
		var cacheContainer map[string]interface{}
		for _, c := range containers {
			if c.(map[string]interface{})["name"] == "cache" {
				cacheContainer = c.(map[string]interface{})
				break
			}
		}
		require.NotNil(t, cacheContainer)
	})

	t.Run("BDD-MergeByKey-EnvironmentVariables", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/env": "MERGE_BY_KEY:name",
		})
		base := map[string]interface{}{
			"env": []interface{}{
				map[string]interface{}{"name": "LOG_LEVEL", "value": "info"},
				map[string]interface{}{"name": "DATABASE_URL", "value": "localhost"},
			},
		}
		source := map[string]interface{}{
			"env": []interface{}{
				map[string]interface{}{"name": "LOG_LEVEL", "value": "debug"},
				map[string]interface{}{"name": "API_KEY", "value": "secret"},
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)

		env := result["env"].([]interface{})
		assert.Equal(t, 3, len(env))

		// Find merged LOG_LEVEL
		var logLevel map[string]interface{}
		for _, e := range env {
			if e.(map[string]interface{})["name"] == "LOG_LEVEL" {
				logLevel = e.(map[string]interface{})
				break
			}
		}
		require.NotNil(t, logLevel)
		assert.Equal(t, "debug", logLevel["value"])
	})

	t.Run("BDD-MergeByKey-PreservesNonMatched", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/items": "MERGE_BY_KEY:id",
		})
		base := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": "1", "status": "active", "data": "old"},
				map[string]interface{}{"id": "2", "status": "inactive"},
			},
		}
		source := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": "1", "data": "new"},
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)

		items := result["items"].([]interface{})
		assert.Equal(t, 2, len(items))

		// Item 1 should be merged
		item1 := items[0].(map[string]interface{})
		assert.Equal(t, "new", item1["data"])
		assert.Equal(t, "active", item1["status"])

		// Item 2 should be preserved
		item2 := items[1].(map[string]interface{})
		assert.Equal(t, "2", item2["id"])
	})
}

// TestStrategicMergeEngine_BehavioralBDD_ComplexScenarios tests complex merge scenarios
func TestStrategicMergeEngine_BehavioralBDD_ComplexScenarios(t *testing.T) {
	// Behavioral contract: Complex multi-strategy merging
	_ = BehavioralContractJSONPatch{
		Behavior:    "Strategic Merge with multiple strategies",
		Operation:   "merge-complex",
		Description: "Different paths can use different strategies in a single merge",
	}

	t.Run("BDD-ComplexScenario-MultipleStrategies", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/spec/containers": "MERGE_BY_KEY:name",
			"/spec/volumes":    "APPEND",
			"/labels":          "UNION",
		})
		base := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "app:1.0"},
				},
				"volumes": []interface{}{
					map[string]interface{}{"name": "vol1"},
				},
			},
			"labels": []interface{}{"prod", "web"},
		}
		source := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "app:2.0", "ports": float64(8080)},
					map[string]interface{}{"name": "sidecar", "image": "sidecar:1.0"},
				},
				"volumes": []interface{}{
					map[string]interface{}{"name": "vol2"},
				},
			},
			"labels": []interface{}{"prod", "api"},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)

		// Check containers (MERGE_BY_KEY)
		containers := result["spec"].(map[string]interface{})["containers"].([]interface{})
		assert.Equal(t, 2, len(containers))

		// Check volumes (APPEND)
		volumes := result["spec"].(map[string]interface{})["volumes"].([]interface{})
		assert.Equal(t, 2, len(volumes))

		// Check labels (UNION)
		labels := result["labels"].([]interface{})
		assert.Equal(t, 3, len(labels))
	})

	t.Run("BDD-ComplexScenario-DeepNesting", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/spec/template/spec/containers": "MERGE_BY_KEY:name",
		})
		base := map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": "app:1.0",
							},
						},
					},
				},
			},
		}
		source := map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": "app:2.0",
								"cpu":   "500m",
							},
						},
					},
				},
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)

		container := result["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["containers"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, "app:2.0", container["image"])
		assert.Equal(t, "500m", container["cpu"])
	})
}

// TestStrategicMergeEngine_BehavioralBDD_EdgeCases tests edge cases
func TestStrategicMergeEngine_BehavioralBDD_EdgeCases(t *testing.T) {
	// Behavioral contract: Edge cases
	_ = BehavioralContractJSONPatch{
		Behavior:    "Strategic Merge handles edge cases",
		Operation:   "merge-edge-cases",
		Description: "Gracefully handles empty arrays, null values, type mismatches",
	}

	t.Run("BDD-EdgeCase-EmptyArrays", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/items": "APPEND",
		})
		base := map[string]interface{}{
			"items": []interface{}{},
		}
		source := map[string]interface{}{
			"items": []interface{}{"item1", "item2"},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		items := result["items"].([]interface{})
		assert.Equal(t, 2, len(items))
	})

	t.Run("BDD-EdgeCase-NullValues", func(t *testing.T) {
		engine := NewStrategicMergeEngine(nil)
		base := map[string]interface{}{
			"field": "value",
		}
		source := map[string]interface{}{
			"field": nil,
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		assert.Nil(t, result["field"])
	})

	t.Run("BDD-EdgeCase-TypeMismatch", func(t *testing.T) {
		engine := NewStrategicMergeEngine(nil)
		base := map[string]interface{}{
			"config": map[string]interface{}{"nested": "value"},
		}
		source := map[string]interface{}{
			"config": "simple_string",
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		// Type mismatch -> replace with source
		assert.Equal(t, "simple_string", result["config"])
	})

	t.Run("BDD-EdgeCase-MissingKeyField", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/items": "MERGE_BY_KEY:id",
		})
		base := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"value": "item1"},
			},
		}
		source := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"value": "item2"},
			},
		}

		result, err := engine.Merge(base, source)
		require.NoError(t, err)
		// Without key field, falls back to append
		items := result["items"].([]interface{})
		assert.Equal(t, 2, len(items))
	})
}

// TestStrategicMergeEngine_BehavioralBDD_Statistics tests statistics tracking
func TestStrategicMergeEngine_BehavioralBDD_Statistics(t *testing.T) {
	// Behavioral contract: Statistics tracking
	_ = BehavioralContractJSONPatch{
		Behavior:    "Strategic Merge tracks merge statistics",
		Operation:   "merge-stats",
		Description: "Engine tracks merges performed, keys updated, arrays merged, conflicts resolved",
	}

	t.Run("BDD-Stats-TracksMerges", func(t *testing.T) {
		engine := NewStrategicMergeEngine(StrategicMergeConfig{
			"/items": "APPEND",
		})
		base := map[string]interface{}{
			"a":     "1",
			"b":     "2",
			"items": []interface{}{"item1"},
		}
		source := map[string]interface{}{
			"a":     "1_updated",
			"c":     "3",
			"items": []interface{}{"item2"},
		}

		_, err := engine.Merge(base, source)
		require.NoError(t, err)

		stats := engine.GetStats()
		assert.Equal(t, 1, stats["merges_performed"])
		assert.Greater(t, stats["keys_updated"], 0)
		assert.Equal(t, 1, stats["arrays_merged"])
	})

	t.Run("BDD-Stats-ResetStats", func(t *testing.T) {
		engine := NewStrategicMergeEngine(nil)
		base := map[string]interface{}{"a": "1"}
		source := map[string]interface{}{"a": "2"}

		_, _ = engine.Merge(base, source)
		stats1 := engine.GetStats()
		assert.Equal(t, 1, stats1["merges_performed"])

		engine.ResetStats()
		stats2 := engine.GetStats()
		assert.Equal(t, 0, stats2["merges_performed"])
	})
}
