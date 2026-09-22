package desiredstate

import (
	"testing"
)

func TestCompareSemanticVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{"Equal versions", "1.0.0", "1.0.0", 0},
		{"V1 less than V2", "1.0.0", "2.0.0", -1},
		{"V1 greater than V2", "2.0.0", "1.0.0", 1},
		{"With v prefix", "v1.2.3", "v1.2.3", 0},
		{"Mixed prefixes", "v1.0.0", "1.0.0", 0},
		{"Different patch", "1.0.0", "1.0.1", -1},
		{"Different minor", "1.1.0", "1.2.0", -1},
		{"Short versions", "1.0", "1.1", -1},
		{"Very short", "2", "1", 1},
		{"With suffixes", "1.0.0-alpha", "1.0.0-beta", 0}, // Both extract to "1.0.0"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CompareSemanticVersions(tt.v1, tt.v2)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("CompareSemanticVersions(%s, %s) = %d, expected %d",
					tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestCompareTimestampVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{"Equal timestamps", "1688899028633", "1688899028633", 0},
		{"V1 earlier", "1688899028633", "1688899029000", -1},
		{"V1 later", "1688899029000", "1688899028633", 1},
		{"Date-time format equal", "20231215-120000", "20231215-120000", 0},
		{"Date-time v1 earlier", "20231215-110000", "20231215-120000", -1},
		{"Date-time v1 later", "20231215-130000", "20231215-120000", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CompareTimestampVersions(tt.v1, tt.v2)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("CompareTimestampVersions(%s, %s) = %d, expected %d",
					tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestValidateVersionFormat(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		wantError bool
	}{
		{"Semantic version", "1.0.0", false},
		{"Semantic with v prefix", "v2.1.3", false},
		{"Short semantic", "1.0", false},
		{"Latest", "latest", false},
		{"Timestamp", "1688899028633", false},
		{"Date-time", "20231215-120000", false},
		{"Git SHA-1", "abc123def456789012345678901234567890abcd", false},
		{"Git SHA-256", "abc123def456789012345678901234567890abcdef123456789012345678901234", false},
		{"Empty string", "", true},
		{"With suffix", "1.0.0-alpha", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersionFormat(tt.version)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateVersionFormat(%s) error = %v, wantError %v",
					tt.version, err, tt.wantError)
			}
		})
	}
}

func TestParseYAMLString(t *testing.T) {
	data := map[string]interface{}{
		"name":  "test-component",
		"count": 123,
	}

	// Test existing required field
	result, err := ParseYAMLString(data, "name", true)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "test-component" {
		t.Errorf("Expected 'test-component', got '%s'", result)
	}

	// Test missing required field
	_, err = ParseYAMLString(data, "missing", true)
	if err == nil {
		t.Error("Expected error for missing required field")
	}

	// Test missing optional field
	result, err = ParseYAMLString(data, "optional", false)
	if err != nil {
		t.Errorf("Unexpected error for optional field: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty string for missing optional field, got '%s'", result)
	}

	// Test wrong type
	_, err = ParseYAMLString(data, "count", true)
	if err == nil {
		t.Error("Expected error for wrong type")
	}
}

func TestParseYAMLBool(t *testing.T) {
	data := map[string]interface{}{
		"enabled": true,
		"name":    "test",
	}

	// Test existing bool
	result := ParseYAMLBool(data, "enabled", false)
	if !result {
		t.Error("Expected true for existing bool field")
	}

	// Test missing field with default true
	result = ParseYAMLBool(data, "missing", true)
	if !result {
		t.Error("Expected default value true")
	}

	// Test missing field with default false
	result = ParseYAMLBool(data, "missing", false)
	if result {
		t.Error("Expected default value false")
	}

	// Test wrong type (should return default)
	result = ParseYAMLBool(data, "name", true)
	if !result {
		t.Error("Expected default value for wrong type")
	}
}

func TestParseYAMLMap(t *testing.T) {
	data := map[string]interface{}{
		"config": map[string]interface{}{
			"key": "value",
		},
		"name": "test",
	}

	// Test existing map
	result, err := ParseYAMLMap(data, "config", true)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result) != 1 || result["key"] != "value" {
		t.Error("Failed to parse map correctly")
	}

	// Test missing required field
	_, err = ParseYAMLMap(data, "missing", true)
	if err == nil {
		t.Error("Expected error for missing required field")
	}

	// Test missing optional field
	result, err = ParseYAMLMap(data, "optional", false)
	if err != nil {
		t.Errorf("Unexpected error for optional field: %v", err)
	}
	if len(result) != 0 {
		t.Error("Expected empty map for missing optional field")
	}

	// Test wrong type
	_, err = ParseYAMLMap(data, "name", true)
	if err == nil {
		t.Error("Expected error for wrong type")
	}
}

func TestIsVersionLocked(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"1.0.0", true},
		{"latest", false},
		{"v2.1.0", true},
		{"abc123", true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := IsVersionLocked(tt.version)
			if result != tt.expected {
				t.Errorf("IsVersionLocked(%s) = %v, expected %v",
					tt.version, result, tt.expected)
			}
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v1.0.0", "1.0.0"},
		{"  v2.1.3  ", "2.1.3"},
		{"1.0.0", "1.0.0"},
		{"V1.0.0", "1.0.0"},
		{"  1.2.3  ", "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeVersion(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeVersion(%s) = %s, expected %s",
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatVersionForDisplay(t *testing.T) {
	tests := []struct {
		version  string
		isLocked bool
		expected string
	}{
		{"1.0.0", true, "1.0.0 (locked)"},
		{"1.0.0", false, "1.0.0"},
		{"latest", false, "latest (unlocked)"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := FormatVersionForDisplay(tt.version, tt.isLocked)
			if result != tt.expected {
				t.Errorf("FormatVersionForDisplay(%s, %v) = %s, expected %s",
					tt.version, tt.isLocked, result, tt.expected)
			}
		})
	}
}

func TestValidateComponentName(t *testing.T) {
	tests := []struct {
		name      string
		wantError bool
	}{
		{"valid-component", false},
		{"component_123", false},
		{"component.name", false},
		{"", true},
		{"component\nname", true},
		{"component\tname", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateComponentName(tt.name)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateComponentName(%s) error = %v, wantError %v",
					tt.name, err, tt.wantError)
			}
		})
	}
}

func TestComponentMetadata(t *testing.T) {
	meta := NewComponentMetadata()

	// Test Set/Get
	meta.Set("key1", "value1")
	value, exists := meta.Get("key1")
	if !exists || value != "value1" {
		t.Error("Failed to set/get metadata")
	}

	// Test GetString
	meta.Set("stringKey", "stringValue")
	strValue, ok := meta.GetString("stringKey")
	if !ok || strValue != "stringValue" {
		t.Error("Failed to get string from metadata")
	}

	// Test GetInt
	meta.Set("intKey", 42)
	intValue, ok := meta.GetInt("intKey")
	if !ok || intValue != 42 {
		t.Error("Failed to get int from metadata")
	}

	// Test GetBool
	meta.Set("boolKey", true)
	boolValue, ok := meta.GetBool("boolKey")
	if !ok || !boolValue {
		t.Error("Failed to get bool from metadata")
	}

	// Test Has
	if !meta.Has("key1") {
		t.Error("Has() returned false for existing key")
	}
	if meta.Has("nonexistent") {
		t.Error("Has() returned true for nonexistent key")
	}

	// Test GetAll
	all := meta.GetAll()
	if len(all) != 4 {
		t.Errorf("Expected 4 metadata entries, got %d", len(all))
	}

	// Test Clear
	meta.Clear()
	if meta.Has("key1") {
		t.Error("Clear() did not remove metadata")
	}
}
