package desiredstate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// BaseComponentHandler provides common functionality for component type handlers.
// Handlers can embed this struct to inherit shared utilities.
type BaseComponentHandler struct{}

// CompareSemanticVersions compares two semantic version strings.
// Supports: X.Y.Z, X.Y, X formats with optional 'v' prefix.
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
//
// Examples:
//   - "1.0.0" vs "2.0.0" → -1
//   - "v1.2.3" vs "v1.2.3" → 0
//   - "3.0" vs "2.9" → 1
func CompareSemanticVersions(v1, v2 string) (int, error) {
	// Remove 'v' prefix if present
	v1Clean := strings.TrimPrefix(v1, "v")
	v2Clean := strings.TrimPrefix(v2, "v")

	// Parse version components
	parts1 := strings.Split(v1Clean, ".")
	parts2 := strings.Split(v2Clean, ".")

	// Pad shorter version with zeros
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for len(parts1) < maxLen {
		parts1 = append(parts1, "0")
	}
	for len(parts2) < maxLen {
		parts2 = append(parts2, "0")
	}

	// Compare each component
	for i := 0; i < maxLen; i++ {
		// Extract numeric part (ignore any suffix like -alpha, -beta)
		num1 := extractNumericVersion(parts1[i])
		num2 := extractNumericVersion(parts2[i])

		n1, err1 := strconv.Atoi(num1)
		n2, err2 := strconv.Atoi(num2)

		if err1 != nil || err2 != nil {
			// Fallback to string comparison if not numeric
			if parts1[i] < parts2[i] {
				return -1, nil
			} else if parts1[i] > parts2[i] {
				return 1, nil
			}
			continue
		}

		if n1 < n2 {
			return -1, nil
		} else if n1 > n2 {
			return 1, nil
		}
	}

	return 0, nil
}

// extractNumericVersion extracts the numeric portion of a version component.
// Examples: "1" → "1", "1-alpha" → "1", "2beta" → "2"
func extractNumericVersion(part string) string {
	re := regexp.MustCompile(`^(\d+)`)
	matches := re.FindStringSubmatch(part)
	if len(matches) > 1 {
		return matches[1]
	}
	return part
}

// CompareTimestampVersions compares two timestamp-based versions.
// Timestamps are expected to be numeric strings (Unix timestamps, build numbers, etc.).
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
//
// Examples:
//   - "1688899028633" vs "1688899029000" → -1
//   - "20231215-120000" vs "20231215-110000" → 1 (lexicographic)
func CompareTimestampVersions(v1, v2 string) (int, error) {
	// Try numeric comparison first
	n1, err1 := strconv.ParseInt(v1, 10, 64)
	n2, err2 := strconv.ParseInt(v2, 10, 64)

	if err1 == nil && err2 == nil {
		// Both are numeric timestamps
		if n1 < n2 {
			return -1, nil
		} else if n1 > n2 {
			return 1, nil
		}
		return 0, nil
	}

	// Fallback to lexicographic comparison (for date-time strings like "20231215-120000")
	if v1 < v2 {
		return -1, nil
	} else if v1 > v2 {
		return 1, nil
	}
	return 0, nil
}

// ValidateVersionFormat checks if a version string is valid.
// Accepts semantic versions (X.Y.Z), timestamps, or custom formats.
func ValidateVersionFormat(version string) error {
	if version == "" {
		return errors.New(errors.ErrParam, "version cannot be empty")
	}

	if version == "latest" {
		return nil // "latest" is always valid (unlocked component)
	}

	// Check for semantic version pattern (X.Y.Z with optional v prefix)
	semanticPattern := regexp.MustCompile(`^v?\d+(\.\d+)*(-[a-zA-Z0-9]+)?$`)
	if semanticPattern.MatchString(version) {
		return nil
	}

	// Check for timestamp pattern (numeric or date-time)
	timestampPattern := regexp.MustCompile(`^\d{10,}$|^\d{8}-\d{6}$`)
	if timestampPattern.MatchString(version) {
		return nil
	}

	// Check for Git SHA pattern (40 or 64 hex characters)
	shaPattern := regexp.MustCompile(`^[a-f0-9]{40}$|^[a-f0-9]{64}$`)
	if shaPattern.MatchString(version) {
		return nil
	}

	// If none of the patterns match, it might still be valid (custom format)
	// Just warn but don't error
	return nil
}

// ParseYAMLString safely extracts a string value from YAML data.
func ParseYAMLString(data map[string]interface{}, key string, required bool) (string, error) {
	value, exists := data[key]
	if !exists {
		if required {
			return "", errors.Newf(errors.ErrParam, "required field '%s' is missing", key)
		}
		return "", nil
	}

	strValue, ok := value.(string)
	if !ok {
		return "", errors.Newf(errors.ErrParam, "field '%s' must be a string, got %T", key, value)
	}

	return strValue, nil
}

// ParseYAMLBool safely extracts a boolean value from YAML data.
func ParseYAMLBool(data map[string]interface{}, key string, defaultValue bool) bool {
	value, exists := data[key]
	if !exists {
		return defaultValue
	}

	boolValue, ok := value.(bool)
	if !ok {
		return defaultValue
	}

	return boolValue
}

// ParseYAMLMap safely extracts a map value from YAML data.
func ParseYAMLMap(data map[string]interface{}, key string, required bool) (map[string]interface{}, error) {
	value, exists := data[key]
	if !exists {
		if required {
			return nil, errors.Newf(errors.ErrParam, "required field '%s' is missing", key)
		}
		return make(map[string]interface{}), nil
	}

	mapValue, ok := value.(map[string]interface{})
	if !ok {
		return nil, errors.Newf(errors.ErrParam, "field '%s' must be a map, got %T", key, value)
	}

	return mapValue, nil
}

// IsVersionLocked checks if a version string represents a locked version.
// "latest" is considered unlocked, everything else is locked.
func IsVersionLocked(version string) bool {
	return version != "latest"
}

// NormalizeVersion normalizes a version string for consistent comparison.
// - Removes 'v' or 'V' prefix if present
// - Trims whitespace
// - Converts to lowercase (for case-insensitive comparison)
func NormalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	return strings.ToLower(version)
}

// FormatVersionForDisplay formats a version string for user-friendly display.
// Adds metadata like lock status, rollback indicators, etc.
func FormatVersionForDisplay(version string, isLocked bool) string {
	if !isLocked && version == "latest" {
		return "latest (unlocked)"
	}
	if isLocked {
		return fmt.Sprintf("%s (locked)", version)
	}
	return version
}

// ValidateComponentName checks if a component name is valid.
// Names should be non-empty and follow common identifier patterns.
func ValidateComponentName(name string) error {
	if name == "" {
		return errors.New(errors.ErrParam, "component name cannot be empty")
	}

	// Check for invalid characters (control characters, etc.)
	if strings.ContainsAny(name, "\x00\n\r\t") {
		return errors.Newf(errors.ErrParam, "component name '%s' contains invalid characters", name)
	}

	return nil
}

// ComponentMetadata provides a consistent way to store handler-specific data.
type ComponentMetadata struct {
	data map[string]interface{}
}

// NewComponentMetadata creates a new metadata instance.
func NewComponentMetadata() *ComponentMetadata {
	return &ComponentMetadata{
		data: make(map[string]interface{}),
	}
}

// Set stores a key-value pair in metadata.
func (m *ComponentMetadata) Set(key string, value interface{}) {
	m.data[key] = value
}

// Get retrieves a value from metadata.
func (m *ComponentMetadata) Get(key string) (interface{}, bool) {
	value, exists := m.data[key]
	return value, exists
}

// GetString retrieves a string value from metadata.
func (m *ComponentMetadata) GetString(key string) (string, bool) {
	value, exists := m.data[key]
	if !exists {
		return "", false
	}
	strValue, ok := value.(string)
	return strValue, ok
}

// GetInt retrieves an integer value from metadata.
func (m *ComponentMetadata) GetInt(key string) (int, bool) {
	value, exists := m.data[key]
	if !exists {
		return 0, false
	}
	intValue, ok := value.(int)
	return intValue, ok
}

// GetBool retrieves a boolean value from metadata.
func (m *ComponentMetadata) GetBool(key string) (bool, bool) {
	value, exists := m.data[key]
	if !exists {
		return false, false
	}
	boolValue, ok := value.(bool)
	return boolValue, ok
}

// GetAll returns all metadata as a map.
func (m *ComponentMetadata) GetAll() map[string]interface{} {
	result := make(map[string]interface{}, len(m.data))
	for k, v := range m.data {
		result[k] = v
	}
	return result
}

// Has checks if a key exists in metadata.
func (m *ComponentMetadata) Has(key string) bool {
	_, exists := m.data[key]
	return exists
}

// Clear removes all metadata.
func (m *ComponentMetadata) Clear() {
	m.data = make(map[string]interface{})
}
