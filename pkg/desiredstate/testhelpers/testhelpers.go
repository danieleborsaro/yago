// Package testhelpers provides common utilities and helpers for behavioral BDD tests.
// This package extracts common patterns from test files to reduce duplication and
// improve maintainability.
package testhelpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// FILE MANAGEMENT HELPERS
// =============================================================================

// WriteYAMLFile writes YAML content to a temporary file and returns the absolute path.
// The file is automatically cleaned up when the test completes.
//
// Example:
//
//	content := `schema: v1
//	kind: DesiredState`
//	filePath := testhelpers.WriteYAMLFile(t, content)
func WriteYAMLFile(t *testing.T, content string) string {
	t.Helper()
	return WriteFile(t, "test.yaml", content)
}

// WriteFile writes content to a temporary file with the given filename and returns the absolute path.
// The file is created in a temporary directory that is automatically cleaned up when the test completes.
//
// Example:
//
//	filePath := testhelpers.WriteFile(t, "config.json", `{"key": "value"}`)
func WriteFile(t *testing.T, filename string, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, filename)

	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file %s: %v", filename, err)
	}

	return tmpFile
}

// WriteFileInDir writes content to a file in the specified directory.
// Unlike WriteFile, this does not create a temporary directory.
//
// Example:
//
//	dir := t.TempDir()
//	filePath := testhelpers.WriteFileInDir(t, dir, "test.yaml", content)
func WriteFileInDir(t *testing.T, dir string, filename string, content string) string {
	t.Helper()

	filePath := filepath.Join(dir, filename)

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file %s in %s: %v", filename, dir, err)
	}

	return filePath
}

// CreateTempDir creates a temporary directory for testing.
// The directory is automatically cleaned up when the test completes.
//
// Example:
//
//	dir := testhelpers.CreateTempDir(t)
//	// Use dir for test operations
func CreateTempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// =============================================================================
// YAML CONTENT GENERATORS
// =============================================================================

// YAMLGeneratorOptions configures YAML content generation
type YAMLGeneratorOptions struct {
	SchemaVersion   string
	Kind            string
	Namespace       string
	Name            string
	ComponentCount  int
	IncludeMetadata bool
	CustomFields    map[string]string
}

// DefaultDesiredStateOptions returns default options for a DesiredState YAML file
func DefaultDesiredStateOptions() *YAMLGeneratorOptions {
	return &YAMLGeneratorOptions{
		SchemaVersion:   "v1",
		Kind:            "DesiredState",
		Namespace:       "yago",
		Name:            "test-desiredstate",
		ComponentCount:  0,
		IncludeMetadata: true,
		CustomFields:    make(map[string]string),
	}
}

// DefaultConfigurationOptions returns default options for a Configuration YAML file
func DefaultConfigurationOptions() *YAMLGeneratorOptions {
	return &YAMLGeneratorOptions{
		SchemaVersion:   "v1",
		Kind:            "Configuration",
		Namespace:       "yago",
		Name:            "test-configuration",
		ComponentCount:  0,
		IncludeMetadata: true,
		CustomFields:    make(map[string]string),
	}
}

// GenerateDesiredStateYAML generates a basic DesiredState YAML file
//
// Example:
//
//	content := testhelpers.GenerateDesiredStateYAML()
//	filePath := testhelpers.WriteYAMLFile(t, content)
func GenerateDesiredStateYAML() string {
	opts := DefaultDesiredStateOptions()
	return GenerateDesiredStateYAMLWithOptions(opts)
}

// GenerateDesiredStateYAMLWithOptions generates a DesiredState YAML file with custom options
//
// Example:
//
//	opts := testhelpers.DefaultDesiredStateOptions()
//	opts.ComponentCount = 5
//	opts.Name = "my-test"
//	content := testhelpers.GenerateDesiredStateYAMLWithOptions(opts)
func GenerateDesiredStateYAMLWithOptions(opts *YAMLGeneratorOptions) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("schema: %s\n", opts.SchemaVersion))
	sb.WriteString(fmt.Sprintf("kind: %s\n", opts.Kind))

	if opts.IncludeMetadata {
		sb.WriteString("metadata:\n")
		sb.WriteString(fmt.Sprintf("  name: %s\n", opts.Name))
		if opts.Namespace != "" {
			sb.WriteString(fmt.Sprintf("  namespace: %s\n", opts.Namespace))
		}
	}

	// Add custom fields
	for key, value := range opts.CustomFields {
		sb.WriteString(fmt.Sprintf("%s: %s\n", key, value))
	}

	// Add components if requested
	if opts.ComponentCount > 0 {
		sb.WriteString("spec:\n")
		sb.WriteString("  components:\n")
		for i := 0; i < opts.ComponentCount; i++ {
			sb.WriteString(fmt.Sprintf("    - name: component-%d\n", i))
			sb.WriteString("      type: service\n")
			sb.WriteString(fmt.Sprintf("      version: 1.0.%d\n", i%10))
		}
	}

	return sb.String()
}

// GenerateConfigurationYAML generates a basic Configuration YAML file
//
// Example:
//
//	content := testhelpers.GenerateConfigurationYAML()
//	filePath := testhelpers.WriteYAMLFile(t, content)
func GenerateConfigurationYAML() string {
	opts := DefaultConfigurationOptions()
	return GenerateConfigurationYAMLWithOptions(opts)
}

// GenerateConfigurationYAMLWithOptions generates a Configuration YAML file with custom options
//
// Example:
//
//	opts := testhelpers.DefaultConfigurationOptions()
//	opts.Name = "prod-config"
//	content := testhelpers.GenerateConfigurationYAMLWithOptions(opts)
func GenerateConfigurationYAMLWithOptions(opts *YAMLGeneratorOptions) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("schema: %s\n", opts.SchemaVersion))
	sb.WriteString(fmt.Sprintf("kind: %s\n", opts.Kind))

	if opts.IncludeMetadata {
		sb.WriteString("metadata:\n")
		sb.WriteString(fmt.Sprintf("  name: %s\n", opts.Name))
		if opts.Namespace != "" {
			sb.WriteString(fmt.Sprintf("  namespace: %s\n", opts.Namespace))
		}
	}

	// Add custom fields
	for key, value := range opts.CustomFields {
		sb.WriteString(fmt.Sprintf("%s: %s\n", key, value))
	}

	return sb.String()
}

// GenerateLargeDesiredStateYAML generates a large DesiredState YAML for load testing.
// componentCount should be 1000-10000 for 1MB-10MB files.
//
// Example:
//
//	content := testhelpers.GenerateLargeDesiredStateYAML(1000) // ~1MB
//	filePath := testhelpers.WriteYAMLFile(t, content)
func GenerateLargeDesiredStateYAML(componentCount int) string {
	var sb strings.Builder

	sb.WriteString(`schema: v1
kind: DesiredState
metadata:
  name: large-test
  namespace: stress-test
  annotations:
    description: "Stress test file for large YAML processing"
    generated: "automated"
spec:
  components:
`)

	for i := 0; i < componentCount; i++ {
		portNum := 8000 + (i % 1000)
		sb.WriteString(fmt.Sprintf(`    - name: component-%d
      type: service
      version: 1.0.%d
      description: "This is component number %d for stress testing large file processing in yago"
      properties:
        image: registry.example.com/organization/application-name:v%d
        port: %d
        replicas: 3
        env:
          - name: ENV_VAR_%d
            value: "value-with-long-string-to-increase-file-size-%d-padding-data-here"
          - name: DEBUG
            value: "false"
          - name: LOG_LEVEL
            value: "info"
          - name: ADDITIONAL_CONFIG_%d
            value: "extra-configuration-data-to-pad-file-size-component-%d"
        resources:
          cpu: "500m"
          memory: "512Mi"
          storage: "10Gi"
        labels:
          app: component-%d
          tier: backend
          environment: production
          version: v%d
          team: platform-engineering
        annotations:
          prometheus.io/scrape: "true"
          prometheus.io/port: "%d"
          deployment.kubernetes.io/revision: "%d"
        healthChecks:
          liveness:
            httpGet:
              path: /health
              port: %d
            initialDelaySeconds: 30
            periodSeconds: 10
          readiness:
            httpGet:
              path: /ready
              port: %d
            initialDelaySeconds: 10
            periodSeconds: 5
`, i, i%100, i, i%50, portNum, i, i, i, i, i, i, portNum, i%200, portNum, portNum))
	}

	return sb.String()
}

// GenerateDesiredStateWithComponents generates a DesiredState with specific component count
// for scalability testing. This generates minimal components (smaller files than GenerateLargeDesiredStateYAML).
//
// Example:
//
//	content := testhelpers.GenerateDesiredStateWithComponents(100)
//	filePath := testhelpers.WriteYAMLFile(t, content)
func GenerateDesiredStateWithComponents(count int) string {
	var sb strings.Builder

	sb.WriteString(`schema: v1
kind: DesiredState
metadata:
  name: multi-component-test
  namespace: default
spec:
  components:
`)

	for i := 0; i < count; i++ {
		sb.WriteString(fmt.Sprintf(`    - name: comp-%d
      type: service
      config:
        value: data-%d
`, i, i))
	}

	return sb.String()
}

// =============================================================================
// ASSERTION HELPERS
// =============================================================================

// AssertNoError fails the test if err is not nil
//
// Example:
//
//	err := someOperation()
//	testhelpers.AssertNoError(t, err, "operation should succeed")
func AssertNoError(t *testing.T, err error, message string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: got error: %v", message, err)
	}
}

// AssertError fails the test if err is nil
//
// Example:
//
//	err := invalidOperation()
//	testhelpers.AssertError(t, err, "invalid operation should fail")
func AssertError(t *testing.T, err error, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error but got nil", message)
	}
}

// AssertErrorContains fails the test if err is nil or doesn't contain the expected substring
//
// Example:
//
//	err := operation()
//	testhelpers.AssertErrorContains(t, err, "PARAM_ERROR", "should return parameter error")
func AssertErrorContains(t *testing.T, err error, expectedSubstring string, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error containing '%s' but got nil", message, expectedSubstring)
	}
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Fatalf("%s: expected error containing '%s', got: %v", message, expectedSubstring, err)
	}
}

// AssertEqual fails the test if actual != expected
//
// Example:
//
//	testhelpers.AssertEqual(t, result, "expected value", "result should match")
func AssertEqual(t *testing.T, actual, expected interface{}, message string) {
	t.Helper()
	if actual != expected {
		t.Fatalf("%s: expected %v, got %v", message, expected, actual)
	}
}

// AssertNotEqual fails the test if actual == unexpected
//
// Example:
//
//	testhelpers.AssertNotEqual(t, result, "", "result should not be empty")
func AssertNotEqual(t *testing.T, actual, unexpected interface{}, message string) {
	t.Helper()
	if actual == unexpected {
		t.Fatalf("%s: expected not %v, got %v", message, unexpected, actual)
	}
}

// AssertTrue fails the test if condition is false
//
// Example:
//
//	testhelpers.AssertTrue(t, response.IsValid, "response should be valid")
func AssertTrue(t *testing.T, condition bool, message string) {
	t.Helper()
	if !condition {
		t.Fatalf("%s: expected true, got false", message)
	}
}

// AssertFalse fails the test if condition is true
//
// Example:
//
//	testhelpers.AssertFalse(t, response.HasErrors, "response should not have errors")
func AssertFalse(t *testing.T, condition bool, message string) {
	t.Helper()
	if condition {
		t.Fatalf("%s: expected false, got true", message)
	}
}

// AssertContains fails the test if haystack doesn't contain needle
//
// Example:
//
//	testhelpers.AssertContains(t, output, "success", "output should contain success message")
func AssertContains(t *testing.T, haystack, needle string, message string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("%s: expected string containing '%s', got: %s", message, needle, haystack)
	}
}

// AssertNotContains fails the test if haystack contains needle
//
// Example:
//
//	testhelpers.AssertNotContains(t, output, "error", "output should not contain error")
func AssertNotContains(t *testing.T, haystack, needle string, message string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("%s: expected string not containing '%s', got: %s", message, needle, haystack)
	}
}

// =============================================================================
// SERVICE SETUP HELPERS
// =============================================================================

// ServiceSetup provides a configured service instance for testing
type ServiceSetup struct {
	TempDir       string
	DSFile        string
	ConfigFile    string
	DSContent     string
	ConfigContent string
}

// NewServiceSetup creates a new service setup with temporary files
//
// Example:
//
//	setup := testhelpers.NewServiceSetup(t,
//	    testhelpers.GenerateDesiredStateYAML(),
//	    testhelpers.GenerateConfigurationYAML())
//	// Use setup.DSFile, setup.ConfigFile in tests
func NewServiceSetup(t *testing.T, dsContent, configContent string) *ServiceSetup {
	t.Helper()

	tmpDir := t.TempDir()

	dsFile := filepath.Join(tmpDir, "desiredstate.yaml")
	configFile := filepath.Join(tmpDir, "configuration.yaml")

	if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
		t.Fatalf("Failed to write desiredstate file: %v", err)
	}

	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write configuration file: %v", err)
	}

	return &ServiceSetup{
		TempDir:       tmpDir,
		DSFile:        dsFile,
		ConfigFile:    configFile,
		DSContent:     dsContent,
		ConfigContent: configContent,
	}
}

// =============================================================================
// LOGGING HELPERS
// =============================================================================

// LogContract logs a behavioral contract for BDD tests
//
// Example:
//
//	testhelpers.LogContract(t, "Validate YAML files", true, "matches expected behavior")
func LogContract(t *testing.T, behavior string, match bool, differences string) {
	t.Helper()
	if match {
		t.Logf("✓ Contract: %s - Match: %v", behavior, match)
	} else {
		t.Logf("⚠️  Contract: %s - Match: %v - Differences: %s", behavior, match, differences)
	}
}

// LogSuccess logs a successful test step
//
// Example:
//
//	testhelpers.LogSuccess(t, "Validation completed successfully")
func LogSuccess(t *testing.T, message string) {
	t.Helper()
	t.Logf("✓ %s", message)
}

// LogWarning logs a warning during test execution
//
// Example:
//
//	testhelpers.LogWarning(t, "Performance below target but acceptable")
func LogWarning(t *testing.T, message string) {
	t.Helper()
	t.Logf("⚠️  %s", message)
}

// LogInfo logs informational message during test execution
//
// Example:
//
//	testhelpers.LogInfo(t, "Processing 1000 components")
func LogInfo(t *testing.T, message string) {
	t.Helper()
	t.Logf("ℹ️  %s", message)
}
