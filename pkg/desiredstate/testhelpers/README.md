# Test Helpers Package

Comprehensive testing utilities for yago's behavioral BDD test suite. This package extracts common patterns from test files to reduce duplication and improve maintainability.

---

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [File Management](#file-management)
- [YAML Generators](#yaml-generators)
- [Assertions](#assertions)
- [Service Setup](#service-setup)
- [Logging Helpers](#logging-helpers)
- [Migration Guide](#migration-guide)
- [Examples](#examples)

---

## Overview

### Purpose

The `testhelpers` package provides:

- **File Management**: Simplified temporary file creation and cleanup
- **YAML Generators**: Reusable YAML content generators for various test scenarios
- **Assertions**: Fluent assertion helpers for common test validations
- **Service Setup**: Utilities for setting up service instances with test data
- **Logging**: Consistent logging for behavioral contract testing

### Benefits

- ✅ **Reduced Code Duplication**: Common patterns extracted into reusable functions
- ✅ **Improved Readability**: Tests focus on behavior, not boilerplate
- ✅ **Consistent Patterns**: Standardized approach across all test files
- ✅ **Easier Maintenance**: Changes to test utilities in one place
- ✅ **Better Error Messages**: Contextual error reporting with helper methods

---

## Installation

Import the package in your test files:

```go
import "github.com/danieleborsaro/yago/pkg/desiredstate/testhelpers"
```

---

## File Management

### WriteYAMLFile

Writes YAML content to a temporary file and returns the absolute path. The file is automatically cleaned up when the test completes.

```go
func WriteYAMLFile(t *testing.T, content string) string
```

**Example:**

```go
content := `schema: v1
kind: DesiredState
metadata:
  name: test`
filePath := testhelpers.WriteYAMLFile(t, content)
// Use filePath in your tests
```

### WriteFile

Writes content to a temporary file with a custom filename.

```go
func WriteFile(t *testing.T, filename string, content string) string
```

**Example:**

```go
jsonContent := `{"key": "value"}`
filePath := testhelpers.WriteFile(t, "config.json", jsonContent)
```

### WriteFileInDir

Writes content to a file in a specific directory (doesn't create a temporary directory).

```go
func WriteFileInDir(t *testing.T, dir string, filename string, content string) string
```

**Example:**

```go
dir := t.TempDir()
file1 := testhelpers.WriteFileInDir(t, dir, "file1.yaml", content1)
file2 := testhelpers.WriteFileInDir(t, dir, "file2.yaml", content2)
```

### CreateTempDir

Creates a temporary directory for testing with automatic cleanup.

```go
func CreateTempDir(t *testing.T) string
```

**Example:**

```go
dir := testhelpers.CreateTempDir(t)
// Create multiple files in this directory
```

---

## YAML Generators

### Basic Generators

#### GenerateDesiredStateYAML

Generates a basic DesiredState YAML file with default options.

```go
func GenerateDesiredStateYAML() string
```

**Example:**

```go
content := testhelpers.GenerateDesiredStateYAML()
filePath := testhelpers.WriteYAMLFile(t, content)
```

**Generated Output:**

```yaml
schema: v1
kind: DesiredState
metadata:
  name: test-desiredstate
  namespace: yago
```

#### GenerateConfigurationYAML

Generates a basic Configuration YAML file with default options.

```go
func GenerateConfigurationYAML() string
```

**Example:**

```go
content := testhelpers.GenerateConfigurationYAML()
filePath := testhelpers.WriteYAMLFile(t, content)
```

### Advanced Generators

#### GenerateDesiredStateYAMLWithOptions

Generates a DesiredState YAML file with custom options.

```go
func GenerateDesiredStateYAMLWithOptions(opts *YAMLGeneratorOptions) string
```

**YAMLGeneratorOptions Fields:**

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `SchemaVersion` | string | API version | "v1" |
| `Kind` | string | Resource kind | "DesiredState" |
| `Namespace` | string | Namespace | "yago" |
| `Name` | string | Resource name | "test-desiredstate" |
| `ComponentCount` | int | Number of components to generate | 0 |
| `IncludeMetadata` | bool | Include metadata section | true |
| `CustomFields` | map[string]string | Additional custom fields | empty map |

**Example:**

```go
opts := testhelpers.DefaultDesiredStateOptions()
opts.Name = "my-custom-test"
opts.ComponentCount = 5
opts.Namespace = "production"
opts.CustomFields = map[string]string{
    "environment": "prod",
}

content := testhelpers.GenerateDesiredStateYAMLWithOptions(opts)
```

**Generated Output:**

```yaml
schema: v1
kind: DesiredState
metadata:
  name: my-custom-test
  namespace: production
environment: prod
spec:
  components:
    - name: component-0
      type: service
      version: 1.0.0
    - name: component-1
      type: service
      version: 1.0.1
    # ... (3 more components)
```

### Load Testing Generators

#### GenerateLargeDesiredStateYAML

Generates a large DesiredState YAML for load testing (1MB-10MB files).

```go
func GenerateLargeDesiredStateYAML(componentCount int) string
```

**Component Count Guidelines:**

- **1,000 components** ≈ 1.3 MB
- **5,000 components** ≈ 6.7 MB
- **10,000 components** ≈ 13 MB

**Example:**

```go
// Generate ~1MB file for load testing
content := testhelpers.GenerateLargeDesiredStateYAML(1000)
filePath := testhelpers.WriteYAMLFile(t, content)

// Test processing time
start := time.Now()
err := service.ProcessFile(filePath)
elapsed := time.Since(start)

if elapsed > 500*time.Millisecond {
    t.Errorf("Processing too slow: %v", elapsed)
}
```

#### GenerateDesiredStateWithComponents

Generates a DesiredState with minimal components for scalability testing (smaller files).

```go
func GenerateDesiredStateWithComponents(count int) string
```

**Example:**

```go
// Test with 100 minimal components
content := testhelpers.GenerateDesiredStateWithComponents(100)
filePath := testhelpers.WriteYAMLFile(t, content)
```

---

## Assertions

### Error Assertions

#### AssertNoError

Fails the test if err is not nil.

```go
func AssertNoError(t *testing.T, err error, message string)
```

**Example:**

```go
err := service.Validate(request)
testhelpers.AssertNoError(t, err, "validation should succeed")
```

#### AssertError

Fails the test if err is nil.

```go
func AssertError(t *testing.T, err error, message string)
```

**Example:**

```go
err := service.ValidateInvalid(request)
testhelpers.AssertError(t, err, "invalid request should fail")
```

#### AssertErrorContains

Fails the test if err is nil or doesn't contain the expected substring.

```go
func AssertErrorContains(t *testing.T, err error, expectedSubstring string, message string)
```

**Example:**

```go
err := service.Validate(invalidRequest)
testhelpers.AssertErrorContains(t, err, "PARAM_ERROR", "should return parameter error")
```

### Value Assertions

#### AssertEqual / AssertNotEqual

Fails the test if values don't match expectations.

```go
func AssertEqual(t *testing.T, actual, expected interface{}, message string)
func AssertNotEqual(t *testing.T, actual, unexpected interface{}, message string)
```

**Example:**

```go
testhelpers.AssertEqual(t, response.Version, "1.0.0", "version should match")
testhelpers.AssertNotEqual(t, response.Content, "", "content should not be empty")
```

### Boolean Assertions

#### AssertTrue / AssertFalse

Fails the test if condition doesn't match expectation.

```go
func AssertTrue(t *testing.T, condition bool, message string)
func AssertFalse(t *testing.T, condition bool, message string)
```

**Example:**

```go
testhelpers.AssertTrue(t, response.IsValid, "response should be valid")
testhelpers.AssertFalse(t, response.HasErrors, "response should not have errors")
```

### String Assertions

#### AssertContains / AssertNotContains

Fails the test if string containment doesn't match expectation.

```go
func AssertContains(t *testing.T, haystack, needle string, message string)
func AssertNotContains(t *testing.T, haystack, needle string, message string)
```

**Example:**

```go
testhelpers.AssertContains(t, output, "SUCCESS", "output should contain success message")
testhelpers.AssertNotContains(t, output, "ERROR", "output should not contain errors")
```

---

## Service Setup

### ServiceSetup

Provides a configured service instance with temporary files for testing.

```go
type ServiceSetup struct {
    TempDir       string // Temporary directory path
    DSFile        string // DesiredState file path
    ConfigFile    string // Configuration file path
    DSContent     string // DesiredState content
    ConfigContent string // Configuration content
}
```

### NewServiceSetup

Creates a new service setup with temporary files.

```go
func NewServiceSetup(t *testing.T, dsContent, configContent string) *ServiceSetup
```

**Example:**

```go
setup := testhelpers.NewServiceSetup(t,
    testhelpers.GenerateDesiredStateYAML(),
    testhelpers.GenerateConfigurationYAML())

// Use setup.DSFile and setup.ConfigFile in service calls
service := desiredstate.NewService(setup.TempDir, true)
req := desiredstate.ValidateRequest{
    DesiredStateFile: setup.DSFile,
    ConfigFile:       setup.ConfigFile,
    Environment:      "test",
}
response, err := service.ValidateDesiredState(req)
```

---

## Logging Helpers

### LogContract

Logs a behavioral contract for BDD tests with visual indicators.

```go
func LogContract(t *testing.T, behavior string, match bool, differences string)
```

**Example:**

```go
testhelpers.LogContract(t, "Validate YAML files", true, "Python and Go match")
// Output: ✓ Contract: Validate YAML files - Match: true

testhelpers.LogContract(t, "Parse schema", false, "Go uses different parser")
// Output: ⚠️  Contract: Parse schema - Match: false - Differences: Go uses different parser
```

### LogSuccess / LogWarning / LogInfo

Logs test steps with visual indicators.

```go
func LogSuccess(t *testing.T, message string)
func LogWarning(t *testing.T, message string)
func LogInfo(t *testing.T, message string)
```

**Example:**

```go
testhelpers.LogSuccess(t, "Validation completed successfully")
// Output: ✓ Validation completed successfully

testhelpers.LogWarning(t, "Performance below target but acceptable")
// Output: ⚠️  Performance below target but acceptable

testhelpers.LogInfo(t, "Processing 1000 components")
// Output: ℹ️  Processing 1000 components
```

---

## Migration Guide

### Before (Without testhelpers)

```go
func TestSomething(t *testing.T) {
    // Lots of boilerplate
    tmpDir := t.TempDir()
    dsFile := filepath.Join(tmpDir, "test.yaml")
    content := `schema: v1
kind: DesiredState
metadata:
  name: test
  namespace: yago`
    
    if err := os.WriteFile(dsFile, []byte(content), 0644); err != nil {
        t.Fatalf("Failed to write file: %v", err)
    }
    
    // Test logic
    err := service.Validate(dsFile)
    if err != nil {
        t.Fatalf("Expected no error, got: %v", err)
    }
    
    if !response.IsValid {
        t.Fatal("Expected response to be valid")
    }
}
```

### After (With testhelpers)

```go
func TestSomething(t *testing.T) {
    // Clean and focused
    content := testhelpers.GenerateDesiredStateYAML()
    dsFile := testhelpers.WriteYAMLFile(t, content)
    
    // Test logic
    err := service.Validate(dsFile)
    testhelpers.AssertNoError(t, err, "validation should succeed")
    testhelpers.AssertTrue(t, response.IsValid, "response should be valid")
}
```

**Benefits:**

- 60% less boilerplate code
- Clearer test intent
- Consistent error messages
- Automatic cleanup

---

## Examples

### Example 1: Simple Validation Test

```go
func TestValidation(t *testing.T) {
    content := testhelpers.GenerateDesiredStateYAML()
    filePath := testhelpers.WriteYAMLFile(t, content)
    
    service := desiredstate.NewService(".", true)
    req := desiredstate.ValidateRequest{
        DesiredStateFile: filePath,
        Environment:      "test",
    }
    
    response, err := service.ValidateDesiredState(req)
    testhelpers.AssertNoError(t, err, "validation should succeed")
    testhelpers.AssertTrue(t, response.DesiredStateValidated, "should validate")
}
```

### Example 2: Custom YAML with Components

```go
func TestWithComponents(t *testing.T) {
    opts := testhelpers.DefaultDesiredStateOptions()
    opts.ComponentCount = 10
    opts.Name = "multi-component-test"
    
    content := testhelpers.GenerateDesiredStateYAMLWithOptions(opts)
    filePath := testhelpers.WriteYAMLFile(t, content)
    
    // Test with 10 components
    service := desiredstate.NewService(".", true)
    // ... rest of test
}
```

### Example 3: Load Testing

```go
func TestLoadProcessing(t *testing.T) {
    // Generate ~1MB file
    content := testhelpers.GenerateLargeDesiredStateYAML(1000)
    filePath := testhelpers.WriteYAMLFile(t, content)
    
    start := time.Now()
    service := desiredstate.NewService(".", true)
    req := desiredstate.ValidateRequest{
        DesiredStateFile: filePath,
        Environment:      "test",
    }
    
    response, err := service.ValidateDesiredState(req)
    elapsed := time.Since(start)
    
    testhelpers.AssertNoError(t, err, "should process large file")
    
    if elapsed > 500*time.Millisecond {
        testhelpers.LogWarning(t, fmt.Sprintf("Processing took %v (target: <500ms)", elapsed))
    } else {
        testhelpers.LogSuccess(t, fmt.Sprintf("Processed in %v", elapsed))
    }
}
```

### Example 4: Service Setup

```go
func TestServiceOperations(t *testing.T) {
    setup := testhelpers.NewServiceSetup(t,
        testhelpers.GenerateDesiredStateYAML(),
        testhelpers.GenerateConfigurationYAML())
    
    service := desiredstate.NewService(setup.TempDir, true)
    
    // Validation
    validateReq := desiredstate.ValidateRequest{
        DesiredStateFile: setup.DSFile,
        ConfigFile:       setup.ConfigFile,
        Environment:      "test",
    }
    validateResp, err := service.ValidateDesiredState(validateReq)
    testhelpers.AssertNoError(t, err, "validation should succeed")
    
    // Assembly
    assembleReq := desiredstate.AssembleRequest{
        DesiredStateFile: setup.DSFile,
        ConfigFile:       setup.ConfigFile,
        Environment:      "test",
    }
    assembleResp, err := service.AssembleDesiredState(assembleReq)
    testhelpers.AssertNoError(t, err, "assembly should succeed")
}
```

### Example 5: Behavioral Contract Testing

```go
func TestBehavioralContract(t *testing.T) {
    contract := BehavioralContract{
        Behavior: "Validate YAML schema",
        PythonImpl: "Uses jsonschema library for validation",
        GoImpl: "Uses gojsonschema for validation",
        ContractMatch: true,
        Differences: "Different libraries, same behavior",
        Rationale: "Schema validation is critical for data integrity",
    }
    
    content := testhelpers.GenerateDesiredStateYAML()
    filePath := testhelpers.WriteYAMLFile(t, content)
    
    service := desiredstate.NewService(".", true)
    req := desiredstate.ValidateRequest{
        DesiredStateFile: filePath,
        Environment:      "test",
    }
    
    response, err := service.ValidateDesiredState(req)
    testhelpers.AssertNoError(t, err, "validation should succeed")
    testhelpers.AssertTrue(t, response.DesiredStateValidated, "schema should validate")
    
    testhelpers.LogContract(t, contract.Behavior, contract.ContractMatch, contract.Differences)
}
```

---

## Best Practices

### 1. Use Generators for Consistency

✅ **Good:**

```go
content := testhelpers.GenerateDesiredStateYAML()
```

❌ **Avoid:**

```go
content := `schema: v1
kind: DesiredState
...` // Hardcoded YAML
```

### 2. Use Assertions for Clarity

✅ **Good:**

```go
testhelpers.AssertNoError(t, err, "validation should succeed")
testhelpers.AssertTrue(t, response.IsValid, "response should be valid")
```

❌ **Avoid:**

```go
if err != nil {
    t.Fatalf("Expected no error, got: %v", err)
}
if !response.IsValid {
    t.Fatal("Expected valid response")
}
```

### 3. Use ServiceSetup for Complex Tests

✅ **Good:**

```go
setup := testhelpers.NewServiceSetup(t, dsContent, configContent)
service := desiredstate.NewService(setup.TempDir, true)
```

❌ **Avoid:**

```go
tmpDir := t.TempDir()
dsFile := filepath.Join(tmpDir, "ds.yaml")
os.WriteFile(dsFile, []byte(dsContent), 0644)
configFile := filepath.Join(tmpDir, "config.yaml")
os.WriteFile(configFile, []byte(configContent), 0644)
```

### 4. Use Logging Helpers for Visibility

✅ **Good:**

```go
testhelpers.LogSuccess(t, "Processed 1000 components in 50ms")
testhelpers.LogWarning(t, "Performance below target")
```

❌ **Avoid:**

```go
t.Logf("Processed 1000 components in 50ms")
t.Logf("Performance below target")
```

---

## API Reference Summary

### File Management

- `WriteYAMLFile(t, content) → path`
- `WriteFile(t, filename, content) → path`
- `WriteFileInDir(t, dir, filename, content) → path`
- `CreateTempDir(t) → path`

### YAML Generators

- `GenerateDesiredStateYAML() → yaml`
- `GenerateConfigurationYAML() → yaml`
- `GenerateDesiredStateYAMLWithOptions(opts) → yaml`
- `GenerateConfigurationYAMLWithOptions(opts) → yaml`
- `GenerateLargeDesiredStateYAML(componentCount) → yaml`
- `GenerateDesiredStateWithComponents(count) → yaml`

### Assertions

- `AssertNoError(t, err, msg)`
- `AssertError(t, err, msg)`
- `AssertErrorContains(t, err, substring, msg)`
- `AssertEqual(t, actual, expected, msg)`
- `AssertNotEqual(t, actual, unexpected, msg)`
- `AssertTrue(t, condition, msg)`
- `AssertFalse(t, condition, msg)`
- `AssertContains(t, haystack, needle, msg)`
- `AssertNotContains(t, haystack, needle, msg)`

### Service Setup

- `NewServiceSetup(t, dsContent, configContent) → *ServiceSetup`

### Logging

- `LogContract(t, behavior, match, differences)`
- `LogSuccess(t, message)`
- `LogWarning(t, message)`
- `LogInfo(t, message)`

---

## Contributing

When adding new test utilities:

1. **Extract Common Patterns**: Look for code repeated 3+ times across test files
2. **Document with Examples**: Add godoc comments and usage examples
3. **Keep Focused**: Each function should do one thing well
4. **Use t.Helper()**: Mark all helpers with `t.Helper()` for better error reporting
5. **Update This Documentation**: Add new utilities to this README

---

## Related Documentation

- [Behavioral BDD Catalog](../docs/testing/BEHAVIORAL_BDD_CATALOG.md) - Complete test catalog
- [Quick Reference](../docs/testing/QUICK_REFERENCE.md) - Quick command reference
- [Tutorial](../docs/testing/TUTORIAL.md) - Step-by-step testing guide
- [Service API Reference](../docs/SERVICE_API_REFERENCE.md) - Service layer API documentation
