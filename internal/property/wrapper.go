package property

import (
	"fmt"
	"os"
	"strings"

	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// PropertyWrapper provides a wrapper around YAML content with convenient methods
// for accessing and manipulating values using dot-separated paths.
// PropertyWrapper wraps a document's content for dot-path get/set access.
type PropertyWrapper struct {
	Data    map[string]interface{}
	handler *parser.YAMLHandler
	path    string
}

// NewPropertyWrapper creates a new PropertyWrapper instance
func NewPropertyWrapper(content map[string]interface{}, path string) *PropertyWrapper {
	if content == nil {
		content = make(map[string]interface{})
	}

	return &PropertyWrapper{
		Data:    content,
		handler: parser.NewYAMLHandler("/"),
		path:    path,
	}
}

// NewPropertyWrapperFromPath creates a new PropertyWrapper with a base directory
func NewPropertyWrapperFromPath(content map[string]interface{}, path string, baseDir string) *PropertyWrapper {
	if content == nil {
		content = make(map[string]interface{})
	}

	return &PropertyWrapper{
		Data:    content,
		handler: parser.NewYAMLHandler(baseDir),
		path:    path,
	}
}

// LoadFile loads a single YAML file into the wrapper
func (pw *PropertyWrapper) LoadFile(filePath string, envVariables map[string]string) error {
	content, err := pw.handler.LoadFile(filePath, envVariables)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load file %s", filePath)
	}

	pw.Data = content
	return nil
}

// LoadBuffer loads YAML content from a string buffer
func (pw *PropertyWrapper) LoadBuffer(buffer string, envVariables map[string]string) error {
	// Set environment variables
	for key, value := range envVariables {
		os.Setenv(key, value)
	}

	content, err := pw.handler.LoadString(buffer)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load buffer")
	}

	pw.Data = content
	return nil
}

// MergeKeys merges the provided content into the current data
func (pw *PropertyWrapper) MergeKeys(mergingContent map[string]interface{}) {
	pw.Data = pw.mergeMaps(pw.Data, mergingContent)
}

// GetValue retrieves a value from the data using a dot-separated path
func (pw *PropertyWrapper) GetValue(path string) (interface{}, error) {
	return pw.handler.GetValue(pw.Data, path)
}

// AddKey adds a new key-value pair at the specified path
func (pw *PropertyWrapper) AddKey(path string, value interface{}) error {
	return pw.handler.UpdateValue(pw.Data, path, value)
}

// FindValueByKey searches for values that match a pattern (using glob-like patterns)
func (pw *PropertyWrapper) FindValueByKey(pattern string) ([]interface{}, error) {
	var results []interface{}
	err := pw.findValueByKeyRecursive(pw.Data, pattern, "", &results)
	return results, err
}

// ToString converts the wrapper data to a YAML string
func (pw *PropertyWrapper) ToString() (string, error) {
	return pw.handler.ToString(pw.Data)
}

// GetPath returns the base path for this wrapper
func (pw *PropertyWrapper) GetPath() string {
	return pw.path
}

// SetPath sets the base path for this wrapper
func (pw *PropertyWrapper) SetPath(path string) {
	pw.path = path
}

// mergeMaps recursively merges two maps, with values from the second map taking precedence
func (pw *PropertyWrapper) mergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		return src
	}
	if src == nil {
		return dst
	}

	result := make(map[string]interface{})

	// Copy all values from dst
	for k, v := range dst {
		result[k] = v
	}

	// Merge values from src
	for k, v := range src {
		if dstVal, exists := result[k]; exists {
			// If both values are maps, merge them recursively
			if dstMap, dstOk := dstVal.(map[string]interface{}); dstOk {
				if srcMap, srcOk := v.(map[string]interface{}); srcOk {
					result[k] = pw.mergeMaps(dstMap, srcMap)
					continue
				}
			}
		}
		// Otherwise, src value takes precedence
		result[k] = v
	}

	return result
}

// findValueByKeyRecursive recursively searches for keys matching a pattern
func (pw *PropertyWrapper) findValueByKeyRecursive(data interface{}, pattern, currentPath string, results *[]interface{}) error {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			newPath := key
			if currentPath != "" {
				newPath = currentPath + "." + key
			}

			// Check if the current path matches the pattern
			if pw.matchPattern(newPath, pattern) {
				*results = append(*results, map[string]interface{}{
					newPath: value,
				})
			}

			// Recursively search in nested structures
			if err := pw.findValueByKeyRecursive(value, pattern, newPath, results); err != nil {
				return err
			}
		}
	case map[interface{}]interface{}:
		for k, value := range v {
			if key, ok := k.(string); ok {
				newPath := key
				if currentPath != "" {
					newPath = currentPath + "." + key
				}

				// Check if the current path matches the pattern
				if pw.matchPattern(newPath, pattern) {
					*results = append(*results, map[string]interface{}{
						newPath: value,
					})
				}

				// Recursively search in nested structures
				if err := pw.findValueByKeyRecursive(value, pattern, newPath, results); err != nil {
					return err
				}
			}
		}
	case []interface{}:
		for i, item := range v {
			indexPath := fmt.Sprintf("%s[%d]", currentPath, i)
			if err := pw.findValueByKeyRecursive(item, pattern, indexPath, results); err != nil {
				return err
			}
		}
	}

	return nil
}

// matchPattern checks if a path matches a glob-like pattern
func (pw *PropertyWrapper) matchPattern(path, pattern string) bool {
	// Handle simple wildcard patterns
	if pattern == "*" || pattern == "**" {
		return true
	}

	// Handle patterns with wildcards
	if strings.Contains(pattern, "*") {
		// Convert glob pattern to regex-like matching
		return pw.matchGlob(path, pattern)
	}

	// Exact match
	return path == pattern
}

// matchGlob performs glob-like pattern matching
func (pw *PropertyWrapper) matchGlob(path, pattern string) bool {
	// Handle ** for recursive matching
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := strings.TrimSuffix(parts[0], "/")
			prefix = strings.TrimSuffix(prefix, ".")
			suffix := strings.TrimPrefix(parts[1], "/")
			suffix = strings.TrimPrefix(suffix, ".")

			if prefix == "" && suffix == "" {
				return true // ** matches everything
			}
			if prefix == "" {
				return strings.Contains(path, suffix) || strings.HasSuffix(path, suffix)
			}
			if suffix == "" {
				return strings.Contains(path, prefix) || strings.HasPrefix(path, prefix)
			}
			return (strings.Contains(path, prefix) || strings.HasPrefix(path, prefix)) &&
				(strings.Contains(path, suffix) || strings.HasSuffix(path, suffix))
		}
	}

	// Handle single * wildcards - more flexible matching
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]

			// Handle case like "*/parts" - should match "metadata.parts"
			if prefix == "" {
				return strings.HasSuffix(path, suffix)
			}
			if suffix == "" {
				return strings.HasPrefix(path, prefix)
			}

			// For patterns like "*/parts", we want to match any single path segment
			// followed by the suffix
			if strings.HasSuffix(prefix, "/") || strings.HasSuffix(prefix, ".") {
				prefix = strings.TrimSuffix(prefix, "/")
				prefix = strings.TrimSuffix(prefix, ".")

				// Check if path has the right structure
				pathParts := strings.Split(path, ".")
				for i := 0; i < len(pathParts); i++ {
					if i > 0 && strings.HasSuffix(strings.Join(pathParts[:i], "."), prefix) {
						remainingPath := strings.Join(pathParts[i:], ".")
						if strings.HasPrefix(remainingPath, suffix) || remainingPath == suffix {
							return true
						}
					}
				}
			}

			return strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix)
		}
	}

	return false
}

// Clone creates a deep copy of the PropertyWrapper
func (pw *PropertyWrapper) Clone() *PropertyWrapper {
	// Deep copy the data
	clonedData := pw.deepCopyMap(pw.Data)

	return &PropertyWrapper{
		Data:    clonedData,
		handler: parser.NewYAMLHandler("/"),
		path:    pw.path,
	}
}

// deepCopyMap creates a deep copy of a map
func (pw *PropertyWrapper) deepCopyMap(original map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})

	for key, value := range original {
		switch v := value.(type) {
		case map[string]interface{}:
			copy[key] = pw.deepCopyMap(v)
		case []interface{}:
			copy[key] = pw.deepCopySlice(v)
		default:
			copy[key] = value
		}
	}

	return copy
}

// deepCopySlice creates a deep copy of a slice
func (pw *PropertyWrapper) deepCopySlice(original []interface{}) []interface{} {
	copy := make([]interface{}, len(original))

	for i, value := range original {
		switch v := value.(type) {
		case map[string]interface{}:
			copy[i] = pw.deepCopyMap(v)
		case []interface{}:
			copy[i] = pw.deepCopySlice(v)
		default:
			copy[i] = value
		}
	}

	return copy
}

// IsEmpty returns true if the wrapper contains no data
func (pw *PropertyWrapper) IsEmpty() bool {
	return len(pw.Data) == 0
}

// HasKey checks if a key exists at the specified path
func (pw *PropertyWrapper) HasKey(path string) bool {
	_, err := pw.GetValue(path)
	return err == nil
}

// UpdateValue updates a value at the specified path (alias for AddKey)
func (pw *PropertyWrapper) UpdateValue(path string, value interface{}) error {
	return pw.AddKey(path, value)
}

// DeleteKey removes a key at the specified path
func (pw *PropertyWrapper) DeleteKey(path string) error {
	if path == "" {
		return errors.NewParamError("empty path not supported for deletion")
	}

	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		// Delete from root level
		delete(pw.Data, parts[0])
		return nil
	}

	// Navigate to parent
	parentPath := strings.Join(parts[:len(parts)-1], ".")
	parent, err := pw.GetValue(parentPath)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "parent path not found")
	}

	if parentMap, ok := parent.(map[string]interface{}); ok {
		delete(parentMap, parts[len(parts)-1])
		return nil
	}

	return errors.New(errors.ErrFail, "parent is not a map, cannot delete key")
}
