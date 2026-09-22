package testhelpers_test

import (
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/pkg/desiredstate/testhelpers"
)

// TestWriteYAMLFile verifies YAML file writing functionality
func TestWriteYAMLFile(t *testing.T) {
	content := `schema: v1
kind: DesiredState`

	filePath := testhelpers.WriteYAMLFile(t, content)

	if filePath == "" {
		t.Fatal("WriteYAMLFile should return non-empty path")
	}

	if !strings.HasSuffix(filePath, "test.yaml") {
		t.Errorf("Expected path to end with 'test.yaml', got: %s", filePath)
	}
}

// TestGenerateDesiredStateYAML verifies basic YAML generation
func TestGenerateDesiredStateYAML(t *testing.T) {
	content := testhelpers.GenerateDesiredStateYAML()

	testhelpers.AssertContains(t, content, "schema: v1", "should contain schemaVersion")
	testhelpers.AssertContains(t, content, "kind: DesiredState", "should contain kind")
	testhelpers.AssertContains(t, content, "metadata:", "should contain metadata")
}

// TestGenerateDesiredStateWithComponents verifies component generation
func TestGenerateDesiredStateWithComponents(t *testing.T) {
	content := testhelpers.GenerateDesiredStateWithComponents(5)

	testhelpers.AssertContains(t, content, "components:", "should contain components")
	testhelpers.AssertContains(t, content, "comp-0", "should contain comp-0")
	testhelpers.AssertContains(t, content, "comp-4", "should contain comp-4")
}

// TestGenerateWithOptions verifies custom options
func TestGenerateWithOptions(t *testing.T) {
	opts := testhelpers.DefaultDesiredStateOptions()
	opts.Name = "custom-test"
	opts.Namespace = "production"
	opts.ComponentCount = 3

	content := testhelpers.GenerateDesiredStateYAMLWithOptions(opts)

	testhelpers.AssertContains(t, content, "name: custom-test", "should contain custom name")
	testhelpers.AssertContains(t, content, "namespace: production", "should contain namespace")
	testhelpers.AssertContains(t, content, "component-0", "should contain first component")
	testhelpers.AssertContains(t, content, "component-2", "should contain third component")
}

// TestAssertions verifies assertion helpers
func TestAssertions(t *testing.T) {
	t.Run("AssertEqual", func(t *testing.T) {
		testhelpers.AssertEqual(t, "value", "value", "values should be equal")
	})

	t.Run("AssertNotEqual", func(t *testing.T) {
		testhelpers.AssertNotEqual(t, "value1", "value2", "values should not be equal")
	})

	t.Run("AssertTrue", func(t *testing.T) {
		testhelpers.AssertTrue(t, true, "should be true")
	})

	t.Run("AssertFalse", func(t *testing.T) {
		testhelpers.AssertFalse(t, false, "should be false")
	})

	t.Run("AssertContains", func(t *testing.T) {
		testhelpers.AssertContains(t, "hello world", "world", "should contain 'world'")
	})

	t.Run("AssertNotContains", func(t *testing.T) {
		testhelpers.AssertNotContains(t, "hello world", "xyz", "should not contain 'xyz'")
	})
}

// TestServiceSetup verifies service setup utility
func TestServiceSetup(t *testing.T) {
	dsContent := testhelpers.GenerateDesiredStateYAML()
	configContent := testhelpers.GenerateConfigurationYAML()

	setup := testhelpers.NewServiceSetup(t, dsContent, configContent)

	testhelpers.AssertNotEqual(t, setup.TempDir, "", "TempDir should not be empty")
	testhelpers.AssertNotEqual(t, setup.DSFile, "", "DSFile should not be empty")
	testhelpers.AssertNotEqual(t, setup.ConfigFile, "", "ConfigFile should not be empty")
	testhelpers.AssertEqual(t, setup.DSContent, dsContent, "DSContent should match input")
	testhelpers.AssertEqual(t, setup.ConfigContent, configContent, "ConfigContent should match input")
}

// TestLargeYAMLGeneration verifies large file generation
func TestLargeYAMLGeneration(t *testing.T) {
	content := testhelpers.GenerateLargeDesiredStateYAML(100)

	// Should contain many components
	testhelpers.AssertContains(t, content, "component-0", "should contain first component")
	testhelpers.AssertContains(t, content, "component-99", "should contain last component")
	testhelpers.AssertContains(t, content, "healthChecks:", "should contain health checks")

	// File should be reasonably large (>10KB for 100 components)
	if len(content) < 10000 {
		t.Errorf("Expected large content (>10KB), got %d bytes", len(content))
	}

	testhelpers.LogSuccess(t, "Large YAML generation works correctly")
}

// TestLoggingHelpers verifies logging functionality
func TestLoggingHelpers(t *testing.T) {
	testhelpers.LogSuccess(t, "This is a success message")
	testhelpers.LogWarning(t, "This is a warning message")
	testhelpers.LogInfo(t, "This is an info message")
	testhelpers.LogContract(t, "Test contract", true, "No differences")
}
