package parser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// JSONPatchOperation represents a single RFC 6902 JSON Patch operation
type JSONPatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
	From  string      `json:"from,omitempty"`
}

// JSONPatchEngine provides RFC 6902 JSON Patch operations for YAML/JSON manipulation
// RFC 6902 defines operations: add, remove, replace, move, copy, test
type JSONPatchEngine struct {
	stats map[string]int
}

// NewJSONPatchEngine creates a new JSON Patch Engine instance
func NewJSONPatchEngine() *JSONPatchEngine {
	return &JSONPatchEngine{
		stats: map[string]int{
			"patches_applied":  0,
			"patches_failed":   0,
			"total_operations": 0,
		},
	}
}

// Apply applies a series of JSON patches to content
// This function validates patches and applies them in sequence
func (e *JSONPatchEngine) Apply(content map[string]interface{}, patches []JSONPatchOperation) (map[string]interface{}, error) {
	if err := e.validatePatches(patches); err != nil {
		e.stats["patches_failed"]++
		return nil, err
	}

	result := deepCopyMap(content)

	for _, patch := range patches {
		var err error
		switch patch.Op {
		case "replace":
			result, err = e.applyReplace(result, patch)
		case "add":
			result, err = e.applyAdd(result, patch)
		case "remove":
			result, err = e.applyRemove(result, patch)
		case "move":
			result, err = e.applyMove(result, patch)
		case "copy":
			result, err = e.applyCopy(result, patch)
		case "test":
			err = e.applyTest(result, patch)
		default:
			err = fmt.Errorf("unsupported operation: %s", patch.Op)
		}

		if err != nil {
			e.stats["patches_failed"]++
			return nil, errors.Wrapf(errors.ErrParse, err, "failed to apply patch operation %s at path %s", patch.Op, patch.Path)
		}
	}

	e.stats["patches_applied"] += len(patches)
	e.stats["total_operations"] += len(patches)
	return result, nil
}

// ApplyStrict applies patches with strict validation
// All test operations are validated before any mutations are applied (atomic)
func (e *JSONPatchEngine) ApplyStrict(content map[string]interface{}, patches []JSONPatchOperation) (map[string]interface{}, error) {
	// First, run all test operations
	for _, patch := range patches {
		if patch.Op == "test" {
			if err := e.applyTest(content, patch); err != nil {
				return nil, errors.Wrapf(errors.ErrParse, err, "test assertion failed at path %s", patch.Path)
			}
		}
	}

	// If all tests pass, apply mutations
	mutations := []JSONPatchOperation{}
	for _, patch := range patches {
		if patch.Op != "test" {
			mutations = append(mutations, patch)
		}
	}

	if len(mutations) > 0 {
		return e.Apply(content, mutations)
	}

	return content, nil
}

// Test checks if a value matches at a given path
func (e *JSONPatchEngine) Test(content map[string]interface{}, path string, expectedValue interface{}) bool {
	value, err := e.getValueAtPath(content, path)
	if err != nil {
		return false
	}
	return e.valuesEqual(value, expectedValue)
}

// GetStats returns engine statistics
func (e *JSONPatchEngine) GetStats() map[string]int {
	return e.stats
}

// ResetStats resets engine statistics
func (e *JSONPatchEngine) ResetStats() {
	e.stats = map[string]int{
		"patches_applied":  0,
		"patches_failed":   0,
		"total_operations": 0,
	}
}

// Private methods

func (e *JSONPatchEngine) validatePatches(patches []JSONPatchOperation) error {
	for i, patch := range patches {
		if patch.Op == "" {
			return fmt.Errorf("patch %d: missing 'op' field", i)
		}

		validOps := map[string]bool{"add": true, "remove": true, "replace": true, "move": true, "copy": true, "test": true}
		if !validOps[patch.Op] {
			return fmt.Errorf("patch %d: invalid operation '%s'", i, patch.Op)
		}

		if patch.Path == "" {
			return fmt.Errorf("patch %d: missing 'path' field", i)
		}

		// Operation-specific validation
		switch patch.Op {
		case "move", "copy":
			if patch.From == "" {
				return fmt.Errorf("patch %d: operation '%s' requires 'from' field", i, patch.Op)
			}
		}
	}
	return nil
}

func (e *JSONPatchEngine) applyReplace(content map[string]interface{}, patch JSONPatchOperation) (map[string]interface{}, error) {
	return e.setValueAtPath(content, patch.Path, patch.Value)
}

func (e *JSONPatchEngine) applyAdd(content map[string]interface{}, patch JSONPatchOperation) (map[string]interface{}, error) {
	return e.setValueAtPath(content, patch.Path, patch.Value)
}

func (e *JSONPatchEngine) applyRemove(content map[string]interface{}, patch JSONPatchOperation) (map[string]interface{}, error) {
	parts := strings.Split(strings.TrimPrefix(patch.Path, "/"), "/")
	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		return nil, fmt.Errorf("cannot remove root element")
	}

	current := content
	for i := 0; i < len(parts)-1; i++ {
		key := parts[i]
		if val, exists := current[key]; exists {
			if nextMap, ok := val.(map[string]interface{}); ok {
				current = nextMap
			} else {
				return nil, fmt.Errorf("cannot traverse path %s: not a map", patch.Path)
			}
		} else {
			return nil, fmt.Errorf("path not found: %s", patch.Path)
		}
	}

	lastKey := parts[len(parts)-1]
	lastKey = unescapeJSONPointer(lastKey)

	// Check if the key exists before attempting deletion
	if _, exists := current[lastKey]; !exists {
		return nil, fmt.Errorf("path not found: %s", patch.Path)
	}

	delete(current, lastKey)
	return content, nil
}

func (e *JSONPatchEngine) applyMove(content map[string]interface{}, patch JSONPatchOperation) (map[string]interface{}, error) {
	// Get value from source path
	value, err := e.getValueAtPath(content, patch.From)
	if err != nil {
		return nil, fmt.Errorf("move operation: cannot get value at 'from' path %s: %v", patch.From, err)
	}

	// Remove from source
	content, err = e.applyRemove(content, JSONPatchOperation{Op: "remove", Path: patch.From})
	if err != nil {
		return nil, fmt.Errorf("move operation: cannot remove from source path %s: %v", patch.From, err)
	}

	// Add to destination
	return e.setValueAtPath(content, patch.Path, value)
}

func (e *JSONPatchEngine) applyCopy(content map[string]interface{}, patch JSONPatchOperation) (map[string]interface{}, error) {
	// Get value from source path
	value, err := e.getValueAtPath(content, patch.From)
	if err != nil {
		return nil, fmt.Errorf("copy operation: cannot get value at 'from' path %s: %v", patch.From, err)
	}

	// Deep copy the value to avoid reference issues
	copiedValue := deepCopy(value)

	// Add to destination
	return e.setValueAtPath(content, patch.Path, copiedValue)
}

func (e *JSONPatchEngine) applyTest(content map[string]interface{}, patch JSONPatchOperation) error {
	value, err := e.getValueAtPath(content, patch.Path)
	if err != nil {
		return err
	}

	if !e.valuesEqual(value, patch.Value) {
		return fmt.Errorf("test failed at path %s: expected %v, got %v", patch.Path, patch.Value, value)
	}
	return nil
}

func (e *JSONPatchEngine) getValueAtPath(content map[string]interface{}, path string) (interface{}, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		return content, nil
	}

	current := interface{}(content)

	for _, part := range parts {
		part = unescapeJSONPointer(part)

		switch v := current.(type) {
		case map[string]interface{}:
			if val, exists := v[part]; exists {
				current = val
			} else {
				return nil, fmt.Errorf("path not found: %s", path)
			}
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid array index: %s", part)
			}
			if idx < 0 || idx >= len(v) {
				return nil, fmt.Errorf("array index out of bounds: %d", idx)
			}
			current = v[idx]
		default:
			return nil, fmt.Errorf("cannot traverse path %s: value is not a map or array", path)
		}
	}

	return current, nil
}

func (e *JSONPatchEngine) setValueAtPath(content map[string]interface{}, path string, value interface{}) (map[string]interface{}, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		// Setting root - return new value if it's a map
		if newRoot, ok := value.(map[string]interface{}); ok {
			return newRoot, nil
		}
		return nil, fmt.Errorf("cannot set root to non-map value")
	}

	current := content

	// Navigate to parent and create intermediate objects if needed
	for i := 0; i < len(parts)-1; i++ {
		key := unescapeJSONPointer(parts[i])

		if val, exists := current[key]; exists {
			if nextMap, ok := val.(map[string]interface{}); ok {
				current = nextMap
			} else {
				return nil, fmt.Errorf("cannot traverse path: %s is not a map", key)
			}
		} else {
			// Create intermediate object
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
		}
	}

	// Set the final value
	lastKey := unescapeJSONPointer(parts[len(parts)-1])
	current[lastKey] = value

	return content, nil
}

func (e *JSONPatchEngine) valuesEqual(a, b interface{}) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

// Helper functions

// unescapeJSONPointer handles JSON Pointer escaping
// According to RFC 6901, ~ must be escaped as ~0 and / must be escaped as ~1
func unescapeJSONPointer(token string) string {
	token = strings.ReplaceAll(token, "~1", "/")
	token = strings.ReplaceAll(token, "~0", "~")
	return token
}

// escapeJSONPointer escapes a token for use in JSON Pointer
func escapeJSONPointer(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return token
}

// deepCopy creates a deep copy of any value
func deepCopy(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return deepCopyMap(v)
	case []interface{}:
		return deepCopySlice(v)
	default:
		// For primitive types, return as-is (they're immutable)
		return v
	}
}

// deepCopyMap creates a deep copy of a map
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = deepCopy(v)
	}
	return result
}

// deepCopySlice creates a deep copy of a slice
func deepCopySlice(s []interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		result[i] = deepCopy(v)
	}
	return result
}
