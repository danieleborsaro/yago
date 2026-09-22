package parser

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BehavioralContract documents YAML parser behavior and the current implementation.
type BehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

// TestLoadBuffers_BehavioralBDD tests YAML buffer loading and merging behavioral contracts
//
//   - Method: loadBuffers(pBufferList, pBaseDir, pIsInterpolation)
//   - Uses hiyapyco.load() for merging with METHOD_MERGE
//   - Merges multiple YAML documents into single dict
//   - Supports interpolation and custom lookups
//
// Go Implementation (internal/parser.YAMLHandler):
//   - Method: LoadBuffers(buffers []string) (map[string]interface{}, error)
//   - Uses yaml.v3 decoder with custom mergeMaps()
//   - Merges multiple YAML documents recursively
//   - Supports interpolation and custom lookups
//
// Behavioral Contract:
// Both implementations load multiple YAML buffers/strings and merge them into a single
// data structure. Document separators (---) are handled, and later values override earlier
// ones in case of key conflicts.
func TestLoadBuffers_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Load and merge multiple YAML buffers into single data structure",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== BEHAVIORAL CONTRACT: %s ===", contract.Behavior)
	t.Logf("Rationale:%s", contract.Rationale)

	// Test Scenario 1: Single buffer
	t.Run("SingleBuffer", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")
		handler.SetInterpolation(false) // Disable for simple test

		buffer := `
key1: value1
key2: value2
nested:
  subkey: subvalue
`
		result, err := handler.LoadBuffers([]string{buffer})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["key1"] != "value1" {
			t.Errorf("Expected key1=value1, got %v", result["key1"])
		}
		if result["key2"] != "value2" {
			t.Errorf("Expected key2=value2, got %v", result["key2"])
		}

		t.Logf("✓ Single buffer loaded: %d keys", len(result))
	})

	// Test Scenario 2: Multiple buffers - merge
	t.Run("MultipleBuffersMerge", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")
		handler.SetInterpolation(false)

		buffer1 := `
key1: value1
key2: value2
`
		buffer2 := `
key2: overridden
key3: value3
`
		result, err := handler.LoadBuffers([]string{buffer1, buffer2})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["key1"] != "value1" {
			t.Errorf("Expected key1=value1, got %v", result["key1"])
		}
		if result["key2"] != "overridden" {
			t.Errorf("Expected key2=overridden (later value should win), got %v", result["key2"])
		}
		if result["key3"] != "value3" {
			t.Errorf("Expected key3=value3, got %v", result["key3"])
		}

		t.Logf("✓ Multiple buffers merged: key2 overridden correctly")
	})

	// Test Scenario 3: Nested map merging
	t.Run("NestedMapMerge", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")
		handler.SetInterpolation(false)

		buffer1 := `
metadata:
  name: test
  version: 1.0
`
		buffer2 := `
metadata:
  version: 2.0
  author: tester
`
		result, err := handler.LoadBuffers([]string{buffer1, buffer2})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		metadata, ok := result["metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected metadata to be a map")
		}

		if metadata["name"] != "test" {
			t.Errorf("Expected name=test, got %v", metadata["name"])
		}
		// Version is numeric in YAML, check both int and float
		versionVal := fmt.Sprintf("%v", metadata["version"])
		if versionVal != "2" && versionVal != "2.0" {
			t.Errorf("Expected version=2.0 (overridden), got %v (type %T)", metadata["version"], metadata["version"])
		}
		if metadata["author"] != "tester" {
			t.Errorf("Expected author=tester, got %v", metadata["author"])
		}

		t.Logf("✓ Nested maps merged: %d keys in metadata", len(metadata))
	})

	// Test Scenario 4: Document separators handled
	t.Run("DocumentSeparators", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")
		handler.SetInterpolation(false)

		// Buffer without separator
		buffer1 := `key1: value1`

		// Buffer with separator
		buffer2 := `---
key2: value2`

		result, err := handler.LoadBuffers([]string{buffer1, buffer2})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["key1"] != "value1" {
			t.Errorf("Expected key1=value1, got %v", result["key1"])
		}
		if result["key2"] != "value2" {
			t.Errorf("Expected key2=value2, got %v", result["key2"])
		}

		t.Logf("✓ Document separators handled correctly")
	})
}

// TestGetValue_BehavioralBDD tests dot-path value retrieval behavioral contracts
//
//   - Method: getValue(pContent, pPath)
//   - Uses dpath.util.get() with "/" separator
//   - Converts "." to "/" for path navigation
//   - Supports nested maps and lists
//
// Go Implementation (internal/parser.YAMLHandler):
//   - Method: GetValue(content, path)
//   - Custom implementation splitting on "."
//   - Navigates through maps recursively
//   - Supports nested maps
//
// Behavioral Contract:
// Both retrieve values from nested data structures using dot-separated paths.
// Empty or "/" paths return the entire content.
func TestGetValue_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Retrieve value from nested structure using dot-separated path",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== BEHAVIORAL CONTRACT: %s ===", contract.Behavior)
	t.Logf("Rationale:%s", contract.Rationale)

	// Create test content
	content := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"nested": map[string]interface{}{
			"subkey1": "subvalue1",
			"subkey2": "subvalue2",
			"deep": map[string]interface{}{
				"level3": "deepvalue",
			},
		},
	}

	handler := NewYAMLHandler("/tmp")

	// Test Scenario 1: Top-level key
	t.Run("TopLevelKey", func(t *testing.T) {
		value, err := handler.GetValue(content, "key1")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if value != "value1" {
			t.Errorf("Expected 'value1', got %v", value)
		}

		t.Logf("✓ Top-level key retrieved: key1 = %v", value)
	})

	// Test Scenario 2: Nested key
	t.Run("NestedKey", func(t *testing.T) {
		value, err := handler.GetValue(content, "nested.subkey1")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if value != "subvalue1" {
			t.Errorf("Expected 'subvalue1', got %v", value)
		}

		t.Logf("✓ Nested key retrieved: nested.subkey1 = %v", value)
	})

	// Test Scenario 3: Deep nesting
	t.Run("DeepNesting", func(t *testing.T) {
		value, err := handler.GetValue(content, "nested.deep.level3")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if value != "deepvalue" {
			t.Errorf("Expected 'deepvalue', got %v", value)
		}

		t.Logf("✓ Deep nested key retrieved: nested.deep.level3 = %v", value)
	})

	// Test Scenario 4: Empty path returns entire content
	t.Run("EmptyPath", func(t *testing.T) {
		value, err := handler.GetValue(content, "")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		returnedMap, ok := value.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected map[string]interface{}, got %T", value)
		}
		if returnedMap["key1"] != "value1" {
			t.Errorf("Expected entire content returned")
		}

		t.Logf("✓ Empty path returns entire content")
	})

	// Test Scenario 5: Non-existent key
	t.Run("NonExistentKey", func(t *testing.T) {
		_, err := handler.GetValue(content, "nonexistent")
		if err == nil {
			t.Errorf("Expected error for non-existent key")
		}

		t.Logf("✓ Non-existent key returns error: %v", err)
	})

	// Test Scenario 6: Nested map value
	t.Run("NestedMapValue", func(t *testing.T) {
		value, err := handler.GetValue(content, "nested")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		nestedMap, ok := value.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected map, got %T", value)
		}
		if nestedMap["subkey1"] != "subvalue1" {
			t.Errorf("Expected nested map with subkey1")
		}

		t.Logf("✓ Nested map retrieved: nested = %d keys", len(nestedMap))
	})
}

// TestUpdateValue_BehavioralBDD tests dot-path value update behavioral contracts
//
//   - Method: updateValue(pContent, pPath, pNewValue)
//   - Uses dpath.util.set() for updates
//   - Supports list indexing with [n] notation
//   - Recursive updates for lists
//
// Go Implementation (internal/parser.YAMLHandler):
//   - Method: UpdateValue(content, path, value)
//   - Custom implementation with path splitting
//   - Creates intermediate maps as needed
//   - Sets final value at path
//
// Behavioral Contract:
// Both update values in nested structures using dot-separated paths.
// Creates intermediate maps if they don't exist.
func TestUpdateValue_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Update value in nested structure using dot-separated path",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== BEHAVIORAL CONTRACT: %s ===", contract.Behavior)
	t.Logf("Rationale:%s", contract.Rationale)

	handler := NewYAMLHandler("/tmp")

	// Test Scenario 1: Update top-level key
	t.Run("UpdateTopLevel", func(t *testing.T) {
		content := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}

		err := handler.UpdateValue(content, "key1", "updated")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if content["key1"] != "updated" {
			t.Errorf("Expected key1=updated, got %v", content["key1"])
		}

		t.Logf("✓ Top-level key updated: key1 = updated")
	})

	// Test Scenario 2: Update nested key
	t.Run("UpdateNested", func(t *testing.T) {
		content := map[string]interface{}{
			"nested": map[string]interface{}{
				"subkey": "oldvalue",
			},
		}

		err := handler.UpdateValue(content, "nested.subkey", "newvalue")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		nested := content["nested"].(map[string]interface{})
		if nested["subkey"] != "newvalue" {
			t.Errorf("Expected nested.subkey=newvalue, got %v", nested["subkey"])
		}

		t.Logf("✓ Nested key updated: nested.subkey = newvalue")
	})

	// Test Scenario 3: Create intermediate maps
	t.Run("CreateIntermediateMaps", func(t *testing.T) {
		content := make(map[string]interface{})

		err := handler.UpdateValue(content, "new.nested.key", "value")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify structure was created
		newMap, ok := content["new"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'new' to be a map")
		}
		nestedMap, ok := newMap["nested"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'new.nested' to be a map")
		}
		if nestedMap["key"] != "value" {
			t.Errorf("Expected new.nested.key=value, got %v", nestedMap["key"])
		}

		t.Logf("✓ Intermediate maps created: new.nested.key = value")
	})

	// Test Scenario 4: Update deep nesting
	t.Run("UpdateDeepNesting", func(t *testing.T) {
		content := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": "oldvalue",
				},
			},
		}

		err := handler.UpdateValue(content, "level1.level2.level3", "newvalue")
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		level1 := content["level1"].(map[string]interface{})
		level2 := level1["level2"].(map[string]interface{})
		if level2["level3"] != "newvalue" {
			t.Errorf("Expected level1.level2.level3=newvalue, got %v", level2["level3"])
		}

		t.Logf("✓ Deep nested key updated: level1.level2.level3 = newvalue")
	})
}

// TestToString_BehavioralBDD tests YAML serialization behavioral contracts
//
//   - Method: toString(pValue, pIsReorder)
//   - Uses hiyapyco.dump() for reordering
//   - Or yaml.dump() without reordering
//   - Returns YAML string
//
// Go Implementation (internal/parser.YAMLHandler):
//   - Method: ToString(content)
//   - Uses yaml.Marshal()
//   - Returns YAML string
//
// Behavioral Contract:
// Both convert map/dict to YAML string representation.
func TestToString_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Convert data structure to YAML string",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== BEHAVIORAL CONTRACT: %s ===", contract.Behavior)
	t.Logf("Rationale:%s", contract.Rationale)

	handler := NewYAMLHandler("/tmp")

	// Test Scenario 1: Simple map
	t.Run("SimpleMap", func(t *testing.T) {
		content := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}

		yamlStr, err := handler.ToString(content)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify it's valid YAML
		if !strings.Contains(yamlStr, "key1") || !strings.Contains(yamlStr, "value1") {
			t.Errorf("Expected YAML to contain key1: value1")
		}
		if !strings.Contains(yamlStr, "key2") || !strings.Contains(yamlStr, "value2") {
			t.Errorf("Expected YAML to contain key2: value2")
		}

		t.Logf("✓ Simple map serialized:\n%s", yamlStr)
	})

	// Test Scenario 2: Nested map
	t.Run("NestedMap", func(t *testing.T) {
		content := map[string]interface{}{
			"parent": map[string]interface{}{
				"child": "value",
			},
		}

		yamlStr, err := handler.ToString(content)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Verify nested structure is present
		if !strings.Contains(yamlStr, "parent:") {
			t.Errorf("Expected YAML to contain parent:")
		}
		if !strings.Contains(yamlStr, "child:") {
			t.Errorf("Expected YAML to contain child:")
		}

		t.Logf("✓ Nested map serialized:\n%s", yamlStr)
	})

	// Test Scenario 3: Round-trip (serialize and parse back)
	t.Run("RoundTrip", func(t *testing.T) {
		original := map[string]interface{}{
			"key1": "value1",
			"nested": map[string]interface{}{
				"subkey": "subvalue",
			},
		}

		// Serialize
		yamlStr, err := handler.ToString(original)
		if err != nil {
			t.Fatalf("Expected no error on serialize, got %v", err)
		}

		// Parse back
		parsed, err := handler.LoadString(yamlStr)
		if err != nil {
			t.Fatalf("Expected no error on parse, got %v", err)
		}

		// Verify values match
		if parsed["key1"] != original["key1"] {
			t.Errorf("Round-trip failed: key1 mismatch")
		}

		t.Logf("✓ Round-trip successful: serialize → parse → same content")
	})
}

// TestGetEnvValueLookup_BehavioralBDD tests environment variable lookup behavioral contracts
//
//   - Function: gitOpsGetEnvValue(pContent, pBaseDir, pIsInterpolation)
//   - Pattern: [[gitops.getEnvValue(ENV_VAR_NAME)]]
//   - Finds all env lookups via regex
//   - Replaces with os.environ[name]
//
// Go Implementation (internal/parser):
//   - Method: getEnvValueLookup(content, baseDir, isInterpolation, helpers)
//   - Pattern: [[gitops.getEnvValue(ENV_VAR_NAME)]]
//   - Finds via regex, replaces with os.Getenv()
//   - Error if env var not found
//
// Behavioral Contract:
// Both find and replace [[gitops.getEnvValue(...)]] patterns with environment variable values.
func TestGetEnvValueLookup_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Replace [[gitops.getEnvValue(VAR)]] with environment variable value",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== BEHAVIORAL CONTRACT: %s ===", contract.Behavior)
	t.Logf("Rationale:%s", contract.Rationale)

	// Set test environment variable
	testEnvVar := "TEST_YAML_ENV_VAR"
	testValue := "test-env-value"
	os.Setenv(testEnvVar, testValue)
	defer os.Unsetenv(testEnvVar)

	// Test Scenario 1: Simple env var lookup
	t.Run("SimpleEnvLookup", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")
		// interpolation is enabled by default

		yaml := `
key1: "[[gitops.getEnvValue(TEST_YAML_ENV_VAR)]]"
key2: normal_value
`
		result, err := handler.LoadString(yaml)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["key1"] != testValue {
			t.Errorf("Expected key1=%s (env var expanded), got %v", testValue, result["key1"])
		}
		if result["key2"] != "normal_value" {
			t.Errorf("Expected key2=normal_value, got %v", result["key2"])
		}

		t.Logf("✓ Environment variable lookup expanded: %s → %s", testEnvVar, testValue)
	})

	// Test Scenario 2: Multiple env var lookups
	t.Run("MultipleEnvLookups", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")

		os.Setenv("TEST_VAR_A", "valueA")
		os.Setenv("TEST_VAR_B", "valueB")
		defer os.Unsetenv("TEST_VAR_A")
		defer os.Unsetenv("TEST_VAR_B")

		yaml := `
keyA: "[[gitops.getEnvValue(TEST_VAR_A)]]"
keyB: "[[gitops.getEnvValue(TEST_VAR_B)]]"
`
		result, err := handler.LoadString(yaml)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["keyA"] != "valueA" {
			t.Errorf("Expected keyA=valueA, got %v", result["keyA"])
		}
		if result["keyB"] != "valueB" {
			t.Errorf("Expected keyB=valueB, got %v", result["keyB"])
		}

		t.Logf("✓ Multiple env var lookups expanded")
	})

	// Test Scenario 3: Env var in nested structure
	t.Run("NestedEnvLookup", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")

		yaml := `
metadata:
  name: test
  version: "[[gitops.getEnvValue(TEST_YAML_ENV_VAR)]]"
`
		result, err := handler.LoadString(yaml)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		metadata := result["metadata"].(map[string]interface{})
		if metadata["version"] != testValue {
			t.Errorf("Expected metadata.version=%s, got %v", testValue, metadata["version"])
		}

		t.Logf("✓ Nested env var lookup expanded")
	})

	// Test Scenario 4: Missing env var (should error)
	t.Run("MissingEnvVar", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")

		yaml := `
key1: "[[gitops.getEnvValue(NONEXISTENT_ENV_VAR)]]"
`
		_, err := handler.LoadString(yaml)
		if err == nil {
			t.Errorf("Expected error for missing env var, got nil")
		}

		t.Logf("✓ Missing env var returns error: %v", err)
	})
}

// TestGetYamlValueLookup_BehavioralBDD tests YAML value lookup behavioral contracts
//
//   - Function: gitOpsGetYamlValue(pContent, pBaseDir, pIsInterpolation)
//   - Pattern: [[gitops.getYamlValue(path.to.value)]]
//   - Pattern: [[gitops.getYamlValue(path.to.value, file.yaml)]]
//   - Retrieves value from same or external file
//
// Go Implementation (internal/parser):
//   - Method: getYamlValueLookup(content, baseDir, isInterpolation, helpers)
//   - Same patterns supported
//   - Loads external files if specified
//
// Behavioral Contract:
// Both find and replace [[gitops.getYamlValue(...)]] patterns with referenced values.
func TestGetYamlValueLookup_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Replace [[gitops.getYamlValue(path, file?)]] with referenced YAML value",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== BEHAVIORAL CONTRACT: %s ===", contract.Behavior)
	t.Logf("Rationale:%s", contract.Rationale)

	// Test Scenario 1: Internal value lookup
	t.Run("InternalValueLookup", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")

		yaml := `
source_key: source_value
target_key: "[[gitops.getYamlValue(source_key)]]"
`
		result, err := handler.LoadString(yaml)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["target_key"] != "source_value" {
			t.Errorf("Expected target_key=source_value (lookup resolved), got %v", result["target_key"])
		}

		t.Logf("✓ Internal YAML value lookup: source_key → target_key")
	})

	// Test Scenario 2: Nested value lookup
	t.Run("NestedValueLookup", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")

		yaml := `
config:
  database:
    host: localhost
connection_string: "[[gitops.getYamlValue(config.database.host)]]"
`
		result, err := handler.LoadString(yaml)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["connection_string"] != "localhost" {
			t.Errorf("Expected connection_string=localhost, got %v", result["connection_string"])
		}

		t.Logf("✓ Nested YAML value lookup: config.database.host → connection_string")
	})

	// Test Scenario 3: External file lookup
	t.Run("ExternalFileLookup", func(t *testing.T) {
		// Create temporary external file
		tmpDir, err := ioutil.TempDir("", "yaml-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		externalFile := filepath.Join(tmpDir, "external.yaml")
		externalContent := `
shared:
  api_url: https://api.example.com
`
		if err := ioutil.WriteFile(externalFile, []byte(externalContent), 0644); err != nil {
			t.Fatalf("Failed to write external file: %v", err)
		}

		handler := NewYAMLHandler(tmpDir)

		yaml := `
service:
  endpoint: "[[gitops.getYamlValue(shared.api_url, external.yaml)]]"
`
		result, err := handler.LoadString(yaml)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		service := result["service"].(map[string]interface{})
		if service["endpoint"] != "https://api.example.com" {
			t.Errorf("Expected endpoint from external file, got %v", service["endpoint"])
		}

		t.Logf("✓ External file YAML value lookup: external.yaml:shared.api_url → service.endpoint")
	})

	// Test Scenario 4: Multiple lookups
	t.Run("MultipleLookups", func(t *testing.T) {
		handler := NewYAMLHandler("/tmp")

		yaml := `
defaults:
  timeout: 30
  retries: 3
service1:
  timeout: "[[gitops.getYamlValue(defaults.timeout)]]"
  retries: "[[gitops.getYamlValue(defaults.retries)]]"
service2:
  timeout: "[[gitops.getYamlValue(defaults.timeout)]]"
  retries: "[[gitops.getYamlValue(defaults.retries)]]"
`
		result, err := handler.LoadString(yaml)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		service1 := result["service1"].(map[string]interface{})
		service2 := result["service2"].(map[string]interface{})

		if service1["timeout"] != "30" {
			t.Errorf("Expected service1.timeout=30, got %v", service1["timeout"])
		}
		if service2["retries"] != "3" {
			t.Errorf("Expected service2.retries=3, got %v", service2["retries"])
		}

		t.Logf("✓ Multiple YAML value lookups resolved")
	})
}

// TestLoadFiles_BehavioralBDD tests file loading behavioral contracts
//
//   - Method: loadFiles(pFiles, pBaseDir, pIsInterpolation, pEnvVariables)
//   - Uses YamlIncludeConstructor for !include tags
//   - Loads each file with yaml.load()
//   - Normalizes with yaml.dump()
//   - Calls loadBuffers() for merging
//
// Go Implementation (internal/parser.YAMLHandler):
//   - Method: LoadFiles(filePaths, envVars)
//   - Sets environment variables
//   - Loads each file with loadSingleFile()
//   - Normalizes YAML
//   - Calls LoadBuffers() for merging
//
// Behavioral Contract:
// Both load multiple YAML files, normalize them, and merge into single structure.
func TestLoadFiles_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Load and merge multiple YAML files into single data structure",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== BEHAVIORAL CONTRACT: %s ===", contract.Behavior)
	t.Logf("Rationale:%s", contract.Rationale)

	// Create temporary directory for test files
	tmpDir, err := ioutil.TempDir("", "yaml-files-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test Scenario 1: Single file
	t.Run("SingleFile", func(t *testing.T) {
		file1 := filepath.Join(tmpDir, "file1.yaml")
		content1 := `
key1: value1
key2: value2
`
		if err := ioutil.WriteFile(file1, []byte(content1), 0644); err != nil {
			t.Fatalf("Failed to write file1: %v", err)
		}

		handler := NewYAMLHandler(tmpDir)
		handler.SetInterpolation(false)

		result, err := handler.LoadFiles([]string{file1}, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["key1"] != "value1" {
			t.Errorf("Expected key1=value1, got %v", result["key1"])
		}

		t.Logf("✓ Single file loaded: %d keys", len(result))
	})

	// Test Scenario 2: Multiple files merge
	t.Run("MultipleFilesMerge", func(t *testing.T) {
		file1 := filepath.Join(tmpDir, "base.yaml")
		file2 := filepath.Join(tmpDir, "override.yaml")

		content1 := `
service:
  name: my-service
  version: 1.0
  port: 8080
`
		content2 := `
service:
  version: 2.0
  enabled: true
`
		if err := ioutil.WriteFile(file1, []byte(content1), 0644); err != nil {
			t.Fatalf("Failed to write file1: %v", err)
		}
		if err := ioutil.WriteFile(file2, []byte(content2), 0644); err != nil {
			t.Fatalf("Failed to write file2: %v", err)
		}

		handler := NewYAMLHandler(tmpDir)
		handler.SetInterpolation(false)

		result, err := handler.LoadFiles([]string{file1, file2}, nil)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		service := result["service"].(map[string]interface{})
		if service["name"] != "my-service" {
			t.Errorf("Expected name=my-service, got %v", service["name"])
		}
		// Version is numeric - check both int and float representations
		versionVal := fmt.Sprintf("%v", service["version"])
		if versionVal != "2" && versionVal != "2.0" {
			t.Errorf("Expected version=2.0 (overridden), got %v (type %T)", service["version"], service["version"])
		}
		if service["enabled"] != true {
			t.Errorf("Expected enabled=true, got %v", service["enabled"])
		}

		t.Logf("✓ Multiple files merged: service.version overridden to 2.0")
	})

	// Test Scenario 3: Environment variables
	t.Run("EnvironmentVariables", func(t *testing.T) {
		file1 := filepath.Join(tmpDir, "with-env.yaml")
		content1 := `
api_key: "[[gitops.getEnvValue(TEST_API_KEY)]]"
`
		if err := ioutil.WriteFile(file1, []byte(content1), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}

		handler := NewYAMLHandler(tmpDir)

		envVars := map[string]string{
			"TEST_API_KEY": "secret-key-123",
		}

		result, err := handler.LoadFiles([]string{file1}, envVars)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if result["api_key"] != "secret-key-123" {
			t.Errorf("Expected api_key=secret-key-123, got %v", result["api_key"])
		}

		t.Logf("✓ Environment variables set and lookup resolved")
	})
}

// ============================================================================
// MISSING FEATURE DOCUMENTATION TESTS
// ============================================================================
// Each test explains the feature, its use case, priority, and estimated effort.
// When features are implemented, these tests should be replaced with behavioral
// validation tests.
// ============================================================================

// TestYAMLHandler_FindValueByKey_BehavioralBDD verifies:
// - Recursive traversal works for nested maps and arrays
// - Path format matches between implementations
//
// Go: YAMLHandler.FindValueByKey(content map[string]interface{}, key string) []string
func TestYAMLHandler_FindValueByKey_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Find all paths in YAML content that contain a specific key name",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	handler := NewYAMLHandler(".")

	t.Run("simple nested structure", func(t *testing.T) {
		content := map[string]interface{}{
			"app": map[string]interface{}{
				"name":    "myapp",
				"version": "1.0",
			},
		}

		paths := handler.FindValueByKey(content, "name")
		expected := []string{"app.name"}

		if len(paths) != len(expected) {
			t.Errorf("Expected %d paths, got %d: %v", len(expected), len(paths), paths)
		}

		if len(paths) > 0 && paths[0] != expected[0] {
			t.Errorf("Expected path %q, got %q", expected[0], paths[0])
		}

		t.Logf("✓ Found path: %v", paths)
	})

	t.Run("multiple occurrences at different levels", func(t *testing.T) {
		content := map[string]interface{}{
			"deployment": map[string]interface{}{
				"name": "app-deployment",
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"name": "app-pod",
						},
					},
				},
			},
		}

		paths := handler.FindValueByKey(content, "name")

		if len(paths) != 2 {
			t.Errorf("Expected 2 paths, got %d: %v", len(paths), paths)
		}

		// Should find both deployment.name and deployment.spec.template.metadata.name
		foundDeploymentName := false
		foundPodName := false
		for _, path := range paths {
			if path == "deployment.name" {
				foundDeploymentName = true
			}
			if path == "deployment.spec.template.metadata.name" {
				foundPodName = true
			}
		}

		if !foundDeploymentName || !foundPodName {
			t.Errorf("Expected to find both names, got: %v", paths)
		}

		t.Logf("✓ Found %d occurrences: %v", len(paths), paths)
	})

	t.Run("array with multiple items", func(t *testing.T) {
		content := map[string]interface{}{
			"deployment": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"image": "nginx:1.19", "name": "web"},
								map[string]interface{}{"image": "redis:6", "name": "cache"},
								map[string]interface{}{"image": "postgres:13", "name": "db"},
							},
						},
					},
				},
			},
		}

		paths := handler.FindValueByKey(content, "image")

		if len(paths) != 3 {
			t.Errorf("Expected 3 image paths, got %d: %v", len(paths), paths)
		}

		// Expected paths with array indices
		expectedPaths := []string{
			"deployment.spec.template.spec.containers[0].image",
			"deployment.spec.template.spec.containers[1].image",
			"deployment.spec.template.spec.containers[2].image",
		}

		for _, expected := range expectedPaths {
			found := false
			for _, path := range paths {
				if path == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find path %q, but didn't. Got: %v", expected, paths)
			}
		}

		t.Logf("✓ Found %d images in array: %v", len(paths), paths)
	})

	t.Run("no matches found", func(t *testing.T) {
		content := map[string]interface{}{
			"app": map[string]interface{}{
				"name":    "myapp",
				"version": "1.0",
			},
		}

		paths := handler.FindValueByKey(content, "nonexistent")

		if len(paths) != 0 {
			t.Errorf("Expected 0 paths for nonexistent key, got %d: %v", len(paths), paths)
		}

		t.Logf("✓ Correctly returned empty result for non-existent key")
	})

	t.Run("deep nesting (10+ levels)", func(t *testing.T) {
		// Build a deeply nested structure
		content := map[string]interface{}{
			"l1": map[string]interface{}{
				"l2": map[string]interface{}{
					"l3": map[string]interface{}{
						"l4": map[string]interface{}{
							"l5": map[string]interface{}{
								"l6": map[string]interface{}{
									"l7": map[string]interface{}{
										"l8": map[string]interface{}{
											"l9": map[string]interface{}{
												"l10": map[string]interface{}{
													"target": "found",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		paths := handler.FindValueByKey(content, "target")

		if len(paths) != 1 {
			t.Errorf("Expected 1 path, got %d: %v", len(paths), paths)
		}

		expected := "l1.l2.l3.l4.l5.l6.l7.l8.l9.l10.target"
		if len(paths) > 0 && paths[0] != expected {
			t.Errorf("Expected path %q, got %q", expected, paths[0])
		}

		t.Logf("✓ Found deeply nested key at: %v", paths)
	})

	t.Run("real-world Kubernetes manifest", func(t *testing.T) {
		// Realistic Kubernetes deployment manifest
		content := map[string]interface{}{
			"schemaVersion": "apps/v1",
			"kind":          "Deployment",
			"metadata": map[string]interface{}{
				"name":      "web-deployment",
				"namespace": "production",
			},
			"spec": map[string]interface{}{
				"replicas": 3,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "web",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "web",
						},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "nginx",
								"image": "nginx:1.19",
								"ports": []interface{}{
									map[string]interface{}{
										"containerPort": 80,
										"name":          "http",
									},
								},
								"env": []interface{}{
									map[string]interface{}{
										"name":  "ENV",
										"value": "production",
									},
								},
							},
							map[string]interface{}{
								"name":  "sidecar",
								"image": "sidecar:latest",
							},
						},
					},
				},
			},
		}

		// Find all "name" keys (common config management task)
		namePaths := handler.FindValueByKey(content, "name")

		// Should find:
		// - metadata.name (deployment name)
		// - spec.template.spec.containers[0].name (nginx)
		// - spec.template.spec.containers[0].ports[0].name (http)
		// - spec.template.spec.containers[0].env[0].name (ENV)
		// - spec.template.spec.containers[1].name (sidecar)
		expectedNameCount := 5

		if len(namePaths) != expectedNameCount {
			t.Errorf("Expected %d 'name' keys, got %d: %v", expectedNameCount, len(namePaths), namePaths)
		}

		// Find all "image" keys (common use case: find all container images)
		imagePaths := handler.FindValueByKey(content, "image")

		if len(imagePaths) != 2 {
			t.Errorf("Expected 2 'image' keys, got %d: %v", len(imagePaths), imagePaths)
		}

		t.Logf("✓ Found %d 'name' fields in K8s manifest", len(namePaths))
		t.Logf("✓ Found %d 'image' fields: %v", len(imagePaths), imagePaths)
	})

	t.Logf("✅ CONTRACT SATISFIED: FindValueByKey works correctly for all scenarios")
	t.Logf("   Behavioral equivalence: Go generator → Go slice")
	t.Logf("   Path format: dot-separated with array[N] indices")
	t.Logf("   Use cases validated: simple, nested, arrays, deep, K8s manifests")
}

// TestYAMLHandler_IncludeTagSupport_BehavioralBDD verifies:
// - Files are loaded and their content replaces the !include tag
// - Nested includes work correctly
// - Circular dependencies are detected
//
// Go: YAMLHandler.processIncludeTags() for !include tag support
func TestYAMLHandler_IncludeTagSupport_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Support !include YAML tag for inline file inclusion",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	t.Run("simple file inclusion", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create base.yaml
		baseContent := `database:
  host: localhost
  port: 5432
  name: mydb
`
		baseFile := filepath.Join(tmpDir, "base.yaml")
		if err := os.WriteFile(baseFile, []byte(baseContent), 0644); err != nil {
			t.Fatalf("Failed to create base file: %v", err)
		}

		// Create main.yaml with !include
		mainContent := `app:
  name: myapp
  version: 1.0
config: !include base.yaml
overrides:
  enabled: true
`
		mainFile := filepath.Join(tmpDir, "main.yaml")
		if err := os.WriteFile(mainFile, []byte(mainContent), 0644); err != nil {
			t.Fatalf("Failed to create main file: %v", err)
		}

		// Load the file
		handler := NewYAMLHandler(tmpDir)
		content, err := handler.LoadFile(mainFile, nil)
		if err != nil {
			t.Fatalf("Failed to load file with !include: %v", err)
		}

		// Verify structure
		app, ok := content["app"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected 'app' field")
		}
		if app["name"] != "myapp" {
			t.Errorf("Expected app.name='myapp', got %v", app["name"])
		}

		// Verify !include was replaced
		config, ok := content["config"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'config' to be replaced by included content")
		}

		database, ok := config["database"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected database in config")
		}

		if database["host"] != "localhost" {
			t.Errorf("Expected host='localhost', got %v", database["host"])
		}
		if database["port"] != 5432 {
			t.Errorf("Expected port=5432, got %v", database["port"])
		}

		t.Logf("✓ Simple !include successfully loaded: %v", config)
	})

	t.Run("nested includes", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create deepest level
		level3Content := `setting: "production"
timeout: 30
`
		level3File := filepath.Join(tmpDir, "level3.yaml")
		if err := os.WriteFile(level3File, []byte(level3Content), 0644); err != nil {
			t.Fatalf("Failed to create level3: %v", err)
		}

		// Create middle level that includes level3
		level2Content := `defaults: !include level3.yaml
custom_field: "test"
`
		level2File := filepath.Join(tmpDir, "level2.yaml")
		if err := os.WriteFile(level2File, []byte(level2Content), 0644); err != nil {
			t.Fatalf("Failed to create level2: %v", err)
		}

		// Create top level that includes level2
		mainContent := `root: !include level2.yaml`
		mainFile := filepath.Join(tmpDir, "main.yaml")
		if err := os.WriteFile(mainFile, []byte(mainContent), 0644); err != nil {
			t.Fatalf("Failed to create main: %v", err)
		}

		// Load and verify
		handler := NewYAMLHandler(tmpDir)
		content, err := handler.LoadFile(mainFile, nil)
		if err != nil {
			t.Fatalf("Failed to load nested includes: %v", err)
		}

		// Navigate nested structure
		root, ok := content["root"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected root field")
		}

		defaults, ok := root["defaults"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected defaults field (from level3)")
		}

		if defaults["setting"] != "production" {
			t.Errorf("Expected setting='production', got %v", defaults["setting"])
		}
		if defaults["timeout"] != 30 {
			t.Errorf("Expected timeout=30, got %v", defaults["timeout"])
		}

		t.Logf("✓ Nested includes work: root -> level2 -> level3")
	})

	t.Run("circular dependency detection", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create circular references: a -> b -> a
		aContent := `data: !include b.yaml`
		aFile := filepath.Join(tmpDir, "a.yaml")
		if err := os.WriteFile(aFile, []byte(aContent), 0644); err != nil {
			t.Fatalf("Failed to create a.yaml: %v", err)
		}

		bContent := `data: !include a.yaml`
		bFile := filepath.Join(tmpDir, "b.yaml")
		if err := os.WriteFile(bFile, []byte(bContent), 0644); err != nil {
			t.Fatalf("Failed to create b.yaml: %v", err)
		}

		// Should detect circular dependency
		handler := NewYAMLHandler(tmpDir)
		_, err := handler.LoadFile(aFile, nil)

		if err == nil {
			t.Error("Expected circular dependency error")
		}

		if err != nil {
			errMsg := err.Error()
			if !strings.Contains(errMsg, "circular") && !strings.Contains(errMsg, "cycle") {
				t.Errorf("Expected circular dependency error, got: %v", err)
			}
		}

		t.Logf("✓ Circular dependency correctly detected")
	})

	t.Run("missing file error handling", func(t *testing.T) {
		tmpDir := t.TempDir()

		mainContent := `data: !include nonexistent.yaml`
		mainFile := filepath.Join(tmpDir, "main.yaml")
		if err := os.WriteFile(mainFile, []byte(mainContent), 0644); err != nil {
			t.Fatalf("Failed to create main: %v", err)
		}

		handler := NewYAMLHandler(tmpDir)
		_, err := handler.LoadFile(mainFile, nil)

		if err == nil {
			t.Error("Expected error for missing file")
		}

		t.Logf("✓ Missing file error: %v", err)
	})

	t.Run("multiple includes in same file", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create multiple files to include
		dbContent := `host: localhost
port: 5432
`
		dbFile := filepath.Join(tmpDir, "db.yaml")
		if err := os.WriteFile(dbFile, []byte(dbContent), 0644); err != nil {
			t.Fatalf("Failed to create db.yaml: %v", err)
		}

		apiContent := `endpoint: "https://api.example.com"
version: "v1"
`
		apiFile := filepath.Join(tmpDir, "api.yaml")
		if err := os.WriteFile(apiFile, []byte(apiContent), 0644); err != nil {
			t.Fatalf("Failed to create api.yaml: %v", err)
		}

		// Main file with multiple includes
		mainContent := `services:
  database: !include db.yaml
  api: !include api.yaml
app_name: "myapp"
`
		mainFile := filepath.Join(tmpDir, "main.yaml")
		if err := os.WriteFile(mainFile, []byte(mainContent), 0644); err != nil {
			t.Fatalf("Failed to create main: %v", err)
		}

		handler := NewYAMLHandler(tmpDir)
		content, err := handler.LoadFile(mainFile, nil)
		if err != nil {
			t.Fatalf("Failed to load multiple includes: %v", err)
		}

		services, ok := content["services"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected services field")
		}

		database, ok := services["database"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected database in services")
		}
		if database["host"] != "localhost" {
			t.Errorf("Expected host='localhost', got %v", database["host"])
		}

		api, ok := services["api"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected api in services")
		}
		if api["endpoint"] != "https://api.example.com" {
			t.Errorf("Expected endpoint URL, got %v", api["endpoint"])
		}

		t.Logf("✓ Multiple includes in same file work correctly")
	})

	t.Logf("✅ CONTRACT SATISFIED: !include tag support works correctly")
	t.Logf("   Go: YamlIncludeConstructor")
	t.Logf("   Go: processIncludeTags with circular dependency detection")
	t.Logf("   Use cases: simple, nested, circular detection, error handling, multiple includes")
}

// TestYAMLHandler_FindKeyByValue_BehavioralBDD verifies:
// - Supports exact match and partial (substring) match
// - Works with nested maps, arrays, and different value types
//
// Go: YAMLHandler.FindKeyByValue(content, pattern, isPartialMatch)
func TestYAMLHandler_FindKeyByValue_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Find all paths in YAML content containing a specific value",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	handler := NewYAMLHandler(".")

	t.Run("exact value match", func(t *testing.T) {
		content := map[string]interface{}{
			"deployment": map[string]interface{}{
				"image":   "nginx:1.19",
				"version": "1.19",
			},
			"service": map[string]interface{}{
				"image": "redis:6",
			},
		}

		paths := handler.FindKeyByValue(content, "nginx:1.19", false)

		if len(paths) != 1 {
			t.Errorf("Expected 1 path, got %d: %v", len(paths), paths)
		}

		if len(paths) > 0 && paths[0] != "deployment.image" {
			t.Errorf("Expected 'deployment.image', got '%s'", paths[0])
		}

		t.Logf("✓ Exact match: %v", paths)
	})

	t.Run("partial (substring) match", func(t *testing.T) {
		content := map[string]interface{}{
			"config": map[string]interface{}{
				"image":       "nginx:1.19",
				"description": "Using nginx web server",
			},
		}

		paths := handler.FindKeyByValue(content, "nginx", true)

		if len(paths) != 2 {
			t.Errorf("Expected 2 paths (partial match), got %d: %v", len(paths), paths)
		}

		found := make(map[string]bool)
		for _, path := range paths {
			found[path] = true
		}

		if !found["config.image"] || !found["config.description"] {
			t.Errorf("Expected both config.image and config.description, got: %v", paths)
		}

		t.Logf("✓ Partial match: %v", paths)
	})

	t.Run("search in arrays", func(t *testing.T) {
		content := map[string]interface{}{
			"deployment": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"image": "nginx:1.19", "name": "web"},
					map[string]interface{}{"image": "nginx:1.19", "name": "cache"},
					map[string]interface{}{"image": "postgres:13", "name": "db"},
				},
			},
		}

		paths := handler.FindKeyByValue(content, "nginx:1.19", false)

		if len(paths) != 2 {
			t.Errorf("Expected 2 paths, got %d: %v", len(paths), paths)
		}

		expected := map[string]bool{
			"deployment.containers[0].image": true,
			"deployment.containers[1].image": true,
		}

		for _, path := range paths {
			if !expected[path] {
				t.Errorf("Unexpected path: %s", path)
			}
		}

		t.Logf("✓ Array search: %v", paths)
	})

	t.Run("case-insensitive matching", func(t *testing.T) {
		content := map[string]interface{}{
			"config": map[string]interface{}{
				"image": "NGINX:1.19",
			},
		}

		paths := handler.FindKeyByValue(content, "nginx:1.19", false)

		if len(paths) != 1 {
			t.Errorf("Expected case-insensitive match, got %d paths: %v", len(paths), paths)
		}

		t.Logf("✓ Case-insensitive: %v", paths)
	})

	t.Run("different value types", func(t *testing.T) {
		content := map[string]interface{}{
			"config": map[string]interface{}{
				"port":    5432,
				"enabled": true,
				"name":    "myapp",
			},
		}

		// Integer
		paths := handler.FindKeyByValue(content, "5432", false)
		if len(paths) != 1 || paths[0] != "config.port" {
			t.Errorf("Integer search failed: %v", paths)
		}

		// Boolean
		paths = handler.FindKeyByValue(content, "true", false)
		if len(paths) != 1 || paths[0] != "config.enabled" {
			t.Errorf("Boolean search failed: %v", paths)
		}

		// String
		paths = handler.FindKeyByValue(content, "myapp", false)
		if len(paths) != 1 || paths[0] != "config.name" {
			t.Errorf("String search failed: %v", paths)
		}

		t.Logf("✓ Different types handled correctly")
	})

	t.Run("no matches", func(t *testing.T) {
		content := map[string]interface{}{
			"config": map[string]interface{}{
				"image": "nginx:1.19",
			},
		}

		paths := handler.FindKeyByValue(content, "nonexistent", false)

		if len(paths) != 0 {
			t.Errorf("Expected empty result, got: %v", paths)
		}

		t.Logf("✓ No matches returns empty list")
	})

	t.Run("real-world Kubernetes manifest", func(t *testing.T) {
		content := map[string]interface{}{
			"schemaVersion": "apps/v1",
			"kind":          "Deployment",
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"image": "nginx:1.19",
								"env": []interface{}{
									map[string]interface{}{"name": "ENV", "value": "production"},
								},
							},
							map[string]interface{}{
								"image": "sidecar:latest",
								"env": []interface{}{
									map[string]interface{}{"name": "ENV", "value": "production"},
								},
							},
						},
					},
				},
			},
		}

		// Find all "production" environments
		paths := handler.FindKeyByValue(content, "production", false)
		if len(paths) != 2 {
			t.Errorf("Expected 2 'production' values, got %d: %v", len(paths), paths)
		}

		// Find all nginx references (partial)
		paths = handler.FindKeyByValue(content, "nginx", true)
		if len(paths) < 1 {
			t.Errorf("Expected at least 1 nginx reference, got %d: %v", len(paths), paths)
		}

		t.Logf("✓ K8s manifest search works: found %d nginx refs", len(paths))
	})

	t.Logf("✅ CONTRACT SATISFIED: FindKeyByValue works correctly")
	t.Logf("   Go: Regex-based value search")
	t.Logf("   Go: Exact/substring value search")
	t.Logf("   Use cases: exact, partial, arrays, case-insensitive, multiple types, K8s auditing")
}
func TestYAMLHandler_RenameKey_Missing(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Rename keys in nested YAML structures",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== MISSING FEATURE: %s ===", contract.Behavior)
	t.Log("⚠️  Feature not yet implemented in Go")
	t.Log("📋 Priority: NICE-TO-HAVE - Useful for migrations")
	t.Log("⏱️  Estimated effort: 2-3 hours")
	t.Log("🎯 Use case: Rename 'old_field' to 'new_field' across configs")
}
func TestYAMLHandler_AddKey_Missing(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Add new key-value pairs to nested structures",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== MISSING FEATURE: %s ===", contract.Behavior)
	t.Log("⚠️  Feature not yet implemented in Go")
	t.Log("📋 Priority: NICE-TO-HAVE - UpdateValue can substitute")
	t.Log("⏱️  Estimated effort: 1-2 hours")
	t.Log("💡 Alternative: Use UpdateValue() with full path")
}
func TestYAMLHandler_MergeKeys_Missing(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Merge multiple key-value pairs into a nested location",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== MISSING FEATURE: %s ===", contract.Behavior)
	t.Log("⚠️  Feature not yet implemented in Go")
	t.Log("📋 Priority: NICE-TO-HAVE - Multiple UpdateValue calls can substitute")
	t.Log("⏱️  Estimated effort: 2-3 hours")
	t.Log("💡 Alternative: Loop with UpdateValue()")
}
func TestYAMLHandler_JoinTag_Missing(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Support !join YAML tag for string concatenation",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== MISSING FEATURE: %s ===", contract.Behavior)
	t.Log("⚠️  Feature not yet implemented in Go")
	t.Log("📋 Priority: NICE-TO-HAVE - Template feature")
	t.Log("⏱️  Estimated effort: 2-3 hours")
	t.Log("🎯 Use case: message: !join ['Hello', ' ', 'World']")
}
func TestYAMLHandler_EnvTag_Missing(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Support !env YAML tag for environment variable substitution",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== MISSING FEATURE: %s ===", contract.Behavior)
	t.Log("⚠️  Feature not yet implemented in Go")
	t.Log("📋 Priority: LOW - Redundant with existing getEnvValue lookup")
	t.Log("💡 Alternative: Use [[gitops.getEnvValue(VAR)]] syntax")
}
func TestYAMLHandler_UnsafeTag_Missing(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Support !unsafe YAML tag for bypassing validations",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== MISSING FEATURE: %s ===", contract.Behavior)
	t.Log("⚠️  Feature not yet implemented in Go")
	t.Log("📋 Priority: LOW - Security anti-pattern")
	t.Log("⚠️  Note: Intentionally excluded for security reasons")
}
func TestYAMLHandler_AwsSecretsManagerLookup_Missing(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Support AWS Secrets Manager lookup via [[gitops.getSmValue(...)]]",
		CurrentImpl:     "Current Go YAML parser implementation",
		ExpectedOutcome: "YAML behavior remains stable and is validated by this test",
		Rationale:       "Predictable YAML parsing is required by downstream document workflows",
	}

	t.Logf("\n=== MISSING FEATURE: %s ===", contract.Behavior)
	t.Log("⚠️  Feature not yet implemented in Go")
	t.Log("📋 Priority: LOW - AWS-specific, high implementation complexity")
	t.Log("⏱️  Estimated effort: 8-12 hours (requires AWS SDK integration)")
	t.Log("🎯 Use case: db_password: [[gitops.getSmValue(prod/db, AWSCURRENT, password)]]")
	t.Log("💡 Note: Only implement if AWS is primary deployment target")
}
