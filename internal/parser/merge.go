package parser

import (
	"strings"
)

// MergeStrategy defines how to merge values at a specific path
type MergeStrategy string

const (
	// MergeStrategyReplace replaces the entire value (default, standard deep merge)
	MergeStrategyReplace MergeStrategy = "REPLACE"

	// MergeStrategyAppend appends arrays instead of replacing them
	MergeStrategyAppend MergeStrategy = "APPEND"

	// MergeStrategyUnion creates a union of array values (unique only)
	MergeStrategyUnion MergeStrategy = "UNION"

	// MergeStrategyMergeByKey merges arrays by matching a key field
	// Format: "MERGE_BY_KEY:fieldname"
	MergeStrategyMergeByKey MergeStrategy = "MERGE_BY_KEY"
)

// StrategicMergeConfig defines merge strategies for specific paths
// Key: JSON Pointer path (e.g., "/spec/containers")
// Value: Strategy string (e.g., "MERGE_BY_KEY:name" or "APPEND")
type StrategicMergeConfig map[string]string

// StrategicMergeEngine provides field-aware merging for YAML/JSON objects
// This enables intelligent merging that prevents data loss and respects semantic meaning
type StrategicMergeEngine struct {
	config StrategicMergeConfig
	stats  map[string]int
}

// NewStrategicMergeEngine creates a new Strategic Merge Engine
func NewStrategicMergeEngine(config StrategicMergeConfig) *StrategicMergeEngine {
	if config == nil {
		config = make(StrategicMergeConfig)
	}
	return &StrategicMergeEngine{
		config: config,
		stats: map[string]int{
			"merges_performed":   0,
			"keys_updated":       0,
			"arrays_merged":      0,
			"conflicts_resolved": 0,
		},
	}
}

// Merge performs a strategic merge of source into base using configured strategies
// This returns a new merged object without modifying the originals
func (e *StrategicMergeEngine) Merge(base map[string]interface{}, source map[string]interface{}) (map[string]interface{}, error) {
	e.stats["merges_performed"]++
	result := deepCopyMap(base)
	return e.mergeObjects(result, source, "")
}

// mergeObjects recursively merges source into target, respecting merge strategies at each path
func (e *StrategicMergeEngine) mergeObjects(target map[string]interface{}, source map[string]interface{}, basePath string) (map[string]interface{}, error) {
	for key, sourceValue := range source {
		currentPath := basePath + "/" + escapeJSONPointer(key)

		// Check if there's a strategy defined for this path
		strategy := e.getStrategyForPath(currentPath)

		if baseValue, exists := target[key]; exists {
			// Key exists in both - need to merge or apply strategy
			switch {
			case isMap(baseValue) && isMap(sourceValue):
				// Both are maps - recurse
				baseMap := baseValue.(map[string]interface{})
				sourceMap := sourceValue.(map[string]interface{})
				merged, err := e.mergeObjects(baseMap, sourceMap, currentPath)
				if err != nil {
					return nil, err
				}
				target[key] = merged
				e.stats["keys_updated"]++

			case isSlice(baseValue) && isSlice(sourceValue):
				// Both are arrays - apply merge strategy
				merged, err := e.mergeArrays(baseValue, sourceValue, strategy)
				if err != nil {
					return nil, err
				}
				target[key] = merged
				e.stats["arrays_merged"]++

			default:
				// Type mismatch or scalar value - replace with source
				target[key] = deepCopy(sourceValue)
				e.stats["keys_updated"]++
				e.stats["conflicts_resolved"]++
			}
		} else {
			// Key only in source - add it
			target[key] = deepCopy(sourceValue)
			e.stats["keys_updated"]++
		}
	}

	return target, nil
}

// mergeArrays merges two arrays according to the specified strategy
func (e *StrategicMergeEngine) mergeArrays(baseValue interface{}, sourceValue interface{}, strategy string) (interface{}, error) {
	baseArray := baseValue.([]interface{})
	sourceArray := sourceValue.([]interface{})

	// Determine the merge strategy
	mergeStrategy := MergeStrategyReplace
	keyField := ""

	if strategy != "" {
		// Parse strategy string (e.g., "MERGE_BY_KEY:name")
		if strings.Contains(strategy, ":") {
			parts := strings.Split(strategy, ":")
			strategyType := strings.TrimSpace(parts[0])
			if len(parts) > 1 {
				keyField = strings.TrimSpace(parts[1])
			}

			if strategyType == "MERGE_BY_KEY" {
				mergeStrategy = MergeStrategyMergeByKey
			} else if strategyType == "APPEND" {
				mergeStrategy = MergeStrategyAppend
			} else if strategyType == "UNION" {
				mergeStrategy = MergeStrategyUnion
			}
		} else {
			// Simple strategy name
			if strategy == "APPEND" {
				mergeStrategy = MergeStrategyAppend
			} else if strategy == "UNION" {
				mergeStrategy = MergeStrategyUnion
			} else if strategy == "MERGE_BY_KEY" {
				mergeStrategy = MergeStrategyMergeByKey
			}
		}
	}

	// Apply merge strategy
	switch mergeStrategy {
	case MergeStrategyMergeByKey:
		return e.mergeByKey(baseArray, sourceArray, keyField)
	case MergeStrategyAppend:
		return e.appendArrays(baseArray, sourceArray), nil
	case MergeStrategyUnion:
		return e.unionArrays(baseArray, sourceArray), nil
	default:
		// Replace strategy - just return a copy of source array
		return deepCopySlice(sourceArray), nil
	}
}

// mergeByKey merges two arrays of maps by matching a key field
// Elements with matching key values are recursively merged
// Elements only in source are appended; elements only in base are kept
func (e *StrategicMergeEngine) mergeByKey(baseArray []interface{}, sourceArray []interface{}, keyField string) (interface{}, error) {
	if keyField == "" {
		// No key field specified, fall back to append
		return e.appendArrays(baseArray, sourceArray), nil
	}

	result := make([]interface{}, 0)

	// Track which source items we've processed
	processedIndices := make(map[int]bool)

	// First, go through base array and merge with source items where keys match
	for _, baseItem := range baseArray {
		baseMap, ok := baseItem.(map[string]interface{})
		if !ok {
			// Not a map, can't merge by key - keep as is
			result = append(result, deepCopy(baseItem))
			continue
		}

		baseKey, exists := baseMap[keyField]
		if !exists {
			// No key field in base - keep as is
			result = append(result, deepCopy(baseItem))
			continue
		}

		// Find matching item in source array
		merged := baseMap

		for sourceIdx, sourceItem := range sourceArray {
			if processedIndices[sourceIdx] {
				continue
			}

			sourceMap, ok := sourceItem.(map[string]interface{})
			if !ok {
				continue
			}

			sourceKey, exists := sourceMap[keyField]
			if !exists {
				continue
			}

			// Check if keys match
			if e.valuesEqual(baseKey, sourceKey) {
				// Keys match - recursively merge the maps
				var err error
				merged, err = e.mergeObjects(deepCopyMap(baseMap), sourceMap, "")
				if err != nil {
					return nil, err
				}
				processedIndices[sourceIdx] = true
				break
			}
		}

		result = append(result, merged)
	}

	// Add remaining items from source that weren't matched
	for sourceIdx, sourceItem := range sourceArray {
		if !processedIndices[sourceIdx] {
			result = append(result, deepCopy(sourceItem))
		}
	}

	return result, nil
}

// appendArrays appends source array items to base array
func (e *StrategicMergeEngine) appendArrays(baseArray []interface{}, sourceArray []interface{}) interface{} {
	result := deepCopySlice(baseArray)
	for _, item := range sourceArray {
		result = append(result, deepCopy(item))
	}
	return result
}

// unionArrays returns the union of two arrays (unique values only)
// Uses deepEqual comparison to determine uniqueness
func (e *StrategicMergeEngine) unionArrays(baseArray []interface{}, sourceArray []interface{}) interface{} {
	result := deepCopySlice(baseArray)

	for _, sourceItem := range sourceArray {
		// Check if item already exists in result
		found := false
		for _, resultItem := range result {
			if e.valuesEqual(resultItem, sourceItem) {
				found = true
				break
			}
		}

		if !found {
			result = append(result, deepCopy(sourceItem))
		}
	}

	return result
}

// getStrategyForPath returns the merge strategy for a given JSON Pointer path
// Supports exact match and wildcard matching
func (e *StrategicMergeEngine) getStrategyForPath(path string) string {
	// Exact match
	if strategy, exists := e.config[path]; exists {
		return strategy
	}

	// Wildcard match (e.g., "/spec/*/containers" matches "/spec/template/spec/containers")
	for configPath, strategy := range e.config {
		if e.pathMatches(path, configPath) {
			return strategy
		}
	}

	return ""
}

// pathMatches checks if path matches a config pattern (simple wildcard support)
func (e *StrategicMergeEngine) pathMatches(path string, pattern string) bool {
	// Simple implementation: if pattern contains *, treat it as a wildcard
	if !strings.Contains(pattern, "*") {
		return path == pattern
	}

	// Convert pattern to regex-like matching
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(patternParts) != len(pathParts) {
		return false
	}

	for i, patternPart := range patternParts {
		if patternPart == "*" {
			continue
		}
		if pathParts[i] != patternPart {
			return false
		}
	}

	return true
}

// GetStats returns merge statistics
func (e *StrategicMergeEngine) GetStats() map[string]int {
	return e.stats
}

// ResetStats resets merge statistics
func (e *StrategicMergeEngine) ResetStats() {
	e.stats = map[string]int{
		"merges_performed":   0,
		"keys_updated":       0,
		"arrays_merged":      0,
		"conflicts_resolved": 0,
	}
}

// Helper functions

// isMap checks if a value is a map
func isMap(v interface{}) bool {
	_, ok := v.(map[string]interface{})
	return ok
}

// isSlice checks if a value is a slice
func isSlice(v interface{}) bool {
	_, ok := v.([]interface{})
	return ok
}

// valuesEqual compares two values for equality
func (e *StrategicMergeEngine) valuesEqual(a interface{}, b interface{}) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle basic types
	switch aVal := a.(type) {
	case string:
		bVal, ok := b.(string)
		return ok && aVal == bVal
	case float64:
		bVal, ok := b.(float64)
		return ok && aVal == bVal
	case bool:
		bVal, ok := b.(bool)
		return ok && aVal == bVal
	case map[string]interface{}:
		bVal, ok := b.(map[string]interface{})
		if !ok || len(aVal) != len(bVal) {
			return false
		}
		for k, v := range aVal {
			if bv, exists := bVal[k]; !exists || !e.valuesEqual(v, bv) {
				return false
			}
		}
		return true
	case []interface{}:
		bVal, ok := b.([]interface{})
		if !ok || len(aVal) != len(bVal) {
			return false
		}
		for i, v := range aVal {
			if !e.valuesEqual(v, bVal[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
