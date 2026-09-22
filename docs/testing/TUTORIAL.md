# Tutorial: Writing Behavioral BDD Tests for yago

This tutorial will guide you through writing effective behavioral BDD tests for yago, step by step.

---

## Table of Contents

1. [Introduction](#introduction)
2. [Prerequisites](#prerequisites)
3. [Tutorial 1: Your First Behavioral Contract](#tutorial-1-your-first-behavioral-contract)
4. [Tutorial 2: Testing Error Handling](#tutorial-2-testing-error-handling)
5. [Tutorial 3: Integration Testing](#tutorial-3-integration-testing)
6. [Tutorial 4: Performance Testing](#tutorial-4-performance-testing)
7. [Best Practices](#best-practices)
8. [Common Pitfalls](#common-pitfalls)

---

## Introduction

Behavioral BDD tests in yago document the behavioral equivalence between the Python gitops implementation and the Go yago rewrite. Each test serves as both executable documentation and a contract that ensures consistency.

### What You'll Learn

- How to structure behavioral contracts
- How to write effective sub-tests
- How to test error scenarios
- How to write integration and performance tests
- Best practices for maintainable tests

---

## Prerequisites

```bash
# Clone the repository
git clone https://github.com/yourusername/yago.git
cd yago

# Ensure Go is installed (1.21+)
go version

# Run existing tests to ensure setup works
go test -v ./... -run "BehavioralBDD"
```

---

## Tutorial 1: Your First Behavioral Contract

Let's write a simple behavioral contract for a new feature: loading metadata from a desiredstate file.

### Step 1: Understand the Feature

**Feature**: Extract metadata (name, namespace, kind) from a desiredstate YAML file.

**Python Implementation**:

```python
def extract_metadata(yaml_content):
    data = yaml.safe_load(yaml_content)
    return {
        'name': data['metadata']['name'],
        'namespace': data['metadata']['namespace'],
        'kind': data['kind']
    }
```

**Go Implementation**:

```go
func ExtractMetadata(yamlContent []byte) (Metadata, error) {
    var data DesiredState
    if err := yaml.Unmarshal(yamlContent, &data); err != nil {
        return Metadata{}, err
    }
    return data.Metadata, nil
}
```

### Step 2: Create the Test File

```go
// pkg/desiredstate/metadata_behavioral_bdd_test.go
package desiredstate_test

import (
    "testing"
)

type MetadataBehavioralContract struct {
    Behavior      string
    PythonImpl    string
    GoImpl        string
    ContractMatch bool
    Differences   string
    Rationale     string
}
```

### Step 3: Define the Contract

```go
func TestMetadata_Extraction_BehavioralBDD(t *testing.T) {
    contract := MetadataBehavioralContract{
        Behavior: "Extract metadata (name, namespace, kind) from desiredstate YAML",
        PythonImpl: `
Python implementation (gitops):
- Load YAML with yaml.safe_load()
- Access data['metadata']['name']
- Access data['metadata']['namespace']
- Access data['kind']
- Return dict with name, namespace, kind
`,
        GoImpl: `
Go implementation (yago):
- Unmarshal YAML to DesiredState struct
- Access data.Metadata.Name
- Access data.Metadata.Namespace
- Access data.Kind
- Return Metadata struct
`,
        ContractMatch: true,
        Differences: "Python uses dict, Go uses struct; otherwise identical behavior",
        Rationale: "Metadata extraction is fundamental for identifying resources",
    }

    // Test implementation follows below
}
```

### Step 4: Write Sub-tests

```go
func TestMetadata_Extraction_BehavioralBDD(t *testing.T) {
    // ... contract definition ...

    t.Run("extract_from_valid_yaml", func(t *testing.T) {
        // Setup
        yamlContent := `
schema: v1
kind: DesiredState
metadata:
  name: my-app
  namespace: production
spec:
  # ... more fields ...
`
        // Execute
        service := desiredstate.NewService("/base", map[string]string{})
        metadata, err := service.ExtractMetadata([]byte(yamlContent))

        // Verify
        if err != nil {
            t.Fatalf("Unexpected error: %v", err)
        }

        if metadata.Name != "my-app" {
            t.Errorf("Expected name 'my-app', got '%s'", metadata.Name)
        }

        if metadata.Namespace != "production" {
            t.Errorf("Expected namespace 'production', got '%s'", metadata.Namespace)
        }

        t.Log("✓ Metadata extracted successfully from valid YAML")
    })

    t.Run("handle_missing_metadata", func(t *testing.T) {
        yamlContent := `
schema: v1
kind: DesiredState
spec:
  # Missing metadata section
`
        service := desiredstate.NewService("/base", map[string]string{})
        _, err := service.ExtractMetadata([]byte(yamlContent))

        if err == nil {
            t.Error("Expected error for missing metadata, got nil")
        }

        t.Logf("✓ Missing metadata detected correctly: %v", err)
    })

    t.Run("handle_invalid_yaml", func(t *testing.T) {
        yamlContent := `
invalid: yaml: content:
  - missing: brackets
`
        service := desiredstate.NewService("/base", map[string]string{})
        _, err := service.ExtractMetadata([]byte(yamlContent))

        if err == nil {
            t.Error("Expected error for invalid YAML, got nil")
        }

        t.Logf("✓ Invalid YAML detected correctly: %v", err)
    })

    // Log the contract
    t.Logf("Contract: %s - Match: %v", contract.Behavior, contract.ContractMatch)
}
```

### Step 5: Run Your Test

```bash
go test -v ./pkg/desiredstate -run "TestMetadata_Extraction_BehavioralBDD"
```

**Expected Output**:

```
=== RUN   TestMetadata_Extraction_BehavioralBDD
=== RUN   TestMetadata_Extraction_BehavioralBDD/extract_from_valid_yaml
    metadata_behavioral_bdd_test.go:XX: ✓ Metadata extracted successfully from valid YAML
=== RUN   TestMetadata_Extraction_BehavioralBDD/handle_missing_metadata
    metadata_behavioral_bdd_test.go:XX: ✓ Missing metadata detected correctly: ...
=== RUN   TestMetadata_Extraction_BehavioralBDD/handle_invalid_yaml
    metadata_behavioral_bdd_test.go:XX: ✓ Invalid YAML detected correctly: ...
    metadata_behavioral_bdd_test.go:XX: Contract: Extract metadata ... - Match: true
--- PASS: TestMetadata_Extraction_BehavioralBDD (0.05s)
PASS
```

---

## Tutorial 2: Testing Error Handling

Error handling is critical. Let's test comprehensive error scenarios.

### Step 1: Define Error Scenarios

```go
func TestParser_ValidationErrors_BehavioralBDD(t *testing.T) {
    contract := BehavioralContract{
        Behavior: "Validate desiredstate files and report errors clearly",
        PythonImpl: "Python raises exceptions with specific messages",
        GoImpl: "Go returns errors with detailed context",
        ContractMatch: true,
        Differences: "Error format differs (exception vs error), but information equivalent",
        Rationale: "Clear error messages help users fix issues quickly",
    }

    // Sub-tests follow
}
```

### Step 2: Test Each Error Type

```go
func TestParser_ValidationErrors_BehavioralBDD(t *testing.T) {
    // ... contract ...

    t.Run("missing_required_field", func(t *testing.T) {
        yamlContent := `
schema: v1
kind: DesiredState
# Missing metadata - required field
spec:
  components: []
`
        service := desiredstate.NewService("/base", map[string]string{})
        err := service.Validate([]byte(yamlContent))

        if err == nil {
            t.Fatal("Expected validation error for missing required field")
        }

        // Check that error message is helpful
        errMsg := err.Error()
        if !strings.Contains(strings.ToLower(errMsg), "metadata") {
            t.Errorf("Error message should mention 'metadata': %s", errMsg)
        }

        if !strings.Contains(strings.ToLower(errMsg), "required") {
            t.Errorf("Error message should indicate field is 'required': %s", errMsg)
        }

        t.Logf("✓ Missing required field error: %v", err)
    })

    t.Run("invalid_field_type", func(t *testing.T) {
        yamlContent := `
schema: v1
kind: DesiredState
metadata:
  name: test
  namespace: default
spec:
  components: "should be array not string"
`
        service := desiredstate.NewService("/base", map[string]string{})
        err := service.Validate([]byte(yamlContent))

        if err == nil {
            t.Fatal("Expected validation error for invalid field type")
        }

        t.Logf("✓ Invalid field type error: %v", err)
    })

    t.Run("schema_mismatch", func(t *testing.T) {
        yamlContent := `
schema: v999
kind: DesiredState
metadata:
  name: test
`
        service := desiredstate.NewService("/base", map[string]string{})
        err := service.Validate([]byte(yamlContent))

        if err == nil {
            t.Fatal("Expected schema version error")
        }

        // Error should mention schema version
        if !strings.Contains(err.Error(), "v999") || 
           !strings.Contains(strings.ToLower(err.Error()), "schema") {
            t.Errorf("Error should mention schema version: %v", err)
        }

        t.Logf("✓ Schema mismatch error: %v", err)
    })

    t.Logf("Contract: %s - Match: %v", contract.Behavior, contract.ContractMatch)
}
```

### Step 3: Test Error Recovery Suggestions

```go
t.Run("error_includes_recovery_suggestion", func(t *testing.T) {
    yamlContent := `
schema: v1
kind: DesiredState
metadata:
  name: test
  # Missing namespace
spec:
  components: []
`
    service := desiredstate.NewService("/base", map[string]string{})
    err := service.Validate([]byte(yamlContent))

    if err == nil {
        t.Fatal("Expected validation error")
    }

    errMsg := err.Error()
    
    // Check for helpful suggestions
    hasHelpfulInfo := strings.Contains(strings.ToLower(errMsg), "add") ||
                      strings.Contains(strings.ToLower(errMsg), "provide") ||
                      strings.Contains(strings.ToLower(errMsg), "specify")

    if !hasHelpfulInfo {
        t.Logf("⚠️  Error could be more helpful: %s", errMsg)
    } else {
        t.Logf("✓ Error includes recovery suggestion: %s", errMsg)
    }
})
```

---

## Tutorial 3: Integration Testing

Integration tests execute the actual compiled binary to test real-world behavior.

### Step 1: Build Helper Functions

```go
// cmd/yago/test_helpers_test.go
package main_test

import (
    "bytes"
    "os"
    "os/exec"
    "path/filepath"
    "testing"
)

func buildYagoBinary(t *testing.T) string {
    t.Helper()
    
    tmpDir := t.TempDir()
    binaryPath := filepath.Join(tmpDir, "yago")
    
    cmd := exec.Command("go", "build", "-o", binaryPath, ".")
    if output, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("Failed to build yago binary: %v\nOutput: %s", err, output)
    }
    
    return binaryPath
}

func runYago(t *testing.T, binary string, args ...string) (stdout, stderr string, exitCode int) {
    t.Helper()
    
    cmd := exec.Command(binary, args...)
    
    var stdoutBuf, stderrBuf bytes.Buffer
    cmd.Stdout = &stdoutBuf
    cmd.Stderr = &stderrBuf
    
    err := cmd.Run()
    
    stdout = stdoutBuf.String()
    stderr = stderrBuf.String()
    
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            exitCode = exitErr.ExitCode()
        } else {
            t.Fatalf("Failed to run command: %v", err)
        }
    } else {
        exitCode = 0
    }
    
    return stdout, stderr, exitCode
}
```

### Step 2: Write Integration Test

```go
func TestCLI_ValidateCommand_Integration_BehavioralBDD(t *testing.T) {
    contract := BehavioralContract{
        Behavior: "Validate command executes and reports results via CLI",
        PythonImpl: "gitops validate <file> - exits 0 on success, 1 on failure",
        GoImpl: "yago validate <file> - same exit code behavior",
        ContractMatch: true,
        Differences: "Output format may differ, but exit codes identical",
        Rationale: "CLI exit codes are critical for CI/CD integration",
    }

    // Build binary once for all sub-tests
    yagoBinary := buildYagoBinary(t)

    t.Run("validate_success_exit_code", func(t *testing.T) {
        // Create valid test file
        testDir := t.TempDir()
        validFile := filepath.Join(testDir, "valid.yaml")
        
        validContent := `
schema: v1
kind: DesiredState
metadata:
  name: test
  namespace: default
spec:
  components: []
`
        if err := os.WriteFile(validFile, []byte(validContent), 0644); err != nil {
            t.Fatalf("Failed to create test file: %v", err)
        }

        // Run validate command
        stdout, stderr, exitCode := runYago(t, yagoBinary, "validate", validFile)

        // Verify
        if exitCode != 0 {
            t.Errorf("Expected exit code 0, got %d\nStdout: %s\nStderr: %s", 
                     exitCode, stdout, stderr)
        }

        t.Logf("✓ Validate command succeeded with exit code 0")
    })

    t.Run("validate_failure_exit_code", func(t *testing.T) {
        // Create invalid test file
        testDir := t.TempDir()
        invalidFile := filepath.Join(testDir, "invalid.yaml")
        
        invalidContent := `
invalid: yaml: structure
metadata:
  - this is wrong
`
        if err := os.WriteFile(invalidFile, []byte(invalidContent), 0644); err != nil {
            t.Fatalf("Failed to create test file: %v", err)
        }

        // Run validate command
        stdout, stderr, exitCode := runYago(t, yagoBinary, "validate", invalidFile)

        // Verify non-zero exit code
        if exitCode == 0 {
            t.Errorf("Expected non-zero exit code for invalid file\nStdout: %s\nStderr: %s", 
                     stdout, stderr)
        }

        // Verify error appears in stderr
        if len(stderr) == 0 {
            t.Error("Expected error message in stderr")
        }

        t.Logf("✓ Validate command failed with exit code %d", exitCode)
    })

    t.Run("validate_missing_file", func(t *testing.T) {
        stdout, stderr, exitCode := runYago(t, yagoBinary, "validate", "/nonexistent/file.yaml")

        if exitCode == 0 {
            t.Error("Expected non-zero exit code for missing file")
        }

        if !strings.Contains(strings.ToLower(stderr), "not found") &&
           !strings.Contains(strings.ToLower(stderr), "no such file") {
            t.Errorf("Expected 'not found' error in stderr: %s", stderr)
        }

        t.Logf("✓ Missing file error reported correctly")
    })

    t.Logf("Contract: %s - Match: %v", contract.Behavior, contract.ContractMatch)
}
```

---

## Tutorial 4: Performance Testing

Let's add a performance behavioral contract.

### Step 1: Performance Behavioral Test

```go
func TestPerformance_SmallFileProcessing_BehavioralBDD(t *testing.T) {
    contract := BehavioralContract{
        Behavior: "Process small files (< 1KB) within 2 seconds",
        PythonImpl: "Python gitops processes small files in ~50-200ms",
        GoImpl: "Go yago should match or exceed Python performance",
        ContractMatch: true,
        Differences: "Go may be faster due to compilation",
        Rationale: "Fast processing enables quick feedback in development",
    }

    yagoBinary := buildYagoBinary(t)

    t.Run("small_file_validation_performance", func(t *testing.T) {
        // Create small test file
        testDir := t.TempDir()
        smallFile := filepath.Join(testDir, "small.yaml")
        
        smallContent := `
schema: v1
kind: DesiredState
metadata:
  name: small-test
  namespace: default
spec:
  components:
    - name: component1
      type: service
`
        if err := os.WriteFile(smallFile, []byte(smallContent), 0644); err != nil {
            t.Fatalf("Failed to create test file: %v", err)
        }

        // Measure performance
        start := time.Now()
        _, _, exitCode := runYago(t, yagoBinary, "validate", smallFile)
        elapsed := time.Since(start)

        // Verify
        if exitCode != 0 {
            t.Error("Validation failed")
        }

        if elapsed > 2*time.Second {
            t.Errorf("Performance target missed: %v (target: < 2s)", elapsed)
        } else {
            t.Logf("✓ Validation completed in %v (target: < 2s)", elapsed)
        }
    })

    t.Logf("Contract: %s - Match: %v", contract.Behavior, contract.ContractMatch)
}
```

### Step 2: Benchmark Test

```go
// pkg/desiredstate/benchmarks_test.go
func BenchmarkValidate_SmallFile(b *testing.B) {
    // Setup
    service := desiredstate.NewService("/base", map[string]string{})
    
    yamlContent := []byte(`
schema: v1
kind: DesiredState
metadata:
  name: benchmark-test
  namespace: default
spec:
  components: []
`)

    // Track memory allocations
    b.ReportAllocs()
    b.ResetTimer()

    // Run benchmark
    for i := 0; i < b.N; i++ {
        if err := service.Validate(yamlContent); err != nil {
            b.Fatalf("Validation failed: %v", err)
        }
    }
}
```

Run benchmark:

```bash
go test -bench=BenchmarkValidate_SmallFile -benchmem ./pkg/desiredstate
```

---

## Best Practices

### 1. **Use Descriptive Names**

✅ Good:

```go
t.Run("validate_rejects_invalid_yaml_with_clear_error", func(t *testing.T) {
```

❌ Bad:

```go
t.Run("test1", func(t *testing.T) {
```

### 2. **Log Successes with ✓**

```go
t.Log("✓ Feature works as expected")
```

### 3. **Use t.TempDir() for File Operations**

✅ Good:

```go
testDir := t.TempDir()  // Auto-cleanup
testFile := filepath.Join(testDir, "test.yaml")
```

❌ Bad:

```go
testFile := "/tmp/test.yaml"  // No cleanup, potential conflicts
```

### 4. **Test Independent Execution**

Each sub-test should be runnable independently:

```bash
# Should work
go test -run "TestMyFeature_BehavioralBDD/specific_subtest"
```

### 5. **Accept Expected Failures Gracefully**

```go
_, err := service.SchemaVersion()
if err != nil && strings.Contains(err.Error(), "configuration not loaded") {
    t.Log("✓ Expected error: configuration not loaded yet")
    return  // Expected failure
}
```

### 6. **Document Contracts Thoroughly**

Include all contract fields:

- **Behavior**: What is tested
- **PythonImpl**: How Python does it
- **GoImpl**: How Go does it
- **ContractMatch**: Whether they match
- **Differences**: Key differences
- **Rationale**: Why this matters

### 7. **Test Real Behavior, Not Mocks**

✅ Good:

```go
yagoBinary := buildYagoBinary(t)
stdout, _, _ := runYago(t, yagoBinary, "version")
```

❌ Bad:

```go
mockCLI := &MockCLI{}
mockCLI.SetVersion("1.0.0")
```

---

## Common Pitfalls

### Pitfall 1: Shared State Between Sub-tests

❌ **Bad**:

```go
func TestBad(t *testing.T) {
    sharedService := desiredstate.NewService("/base", nil)
    
    t.Run("test1", func(t *testing.T) {
        sharedService.Load(...)  // Modifies shared state
    })
    
    t.Run("test2", func(t *testing.T) {
        // Expects fresh service, but gets modified one
    })
}
```

✅ **Good**:

```go
func TestGood(t *testing.T) {
    t.Run("test1", func(t *testing.T) {
        service := desiredstate.NewService("/base", nil)  // Fresh instance
        service.Load(...)
    })
    
    t.Run("test2", func(t *testing.T) {
        service := desiredstate.NewService("/base", nil)  // Fresh instance
    })
}
```

### Pitfall 2: Not Cleaning Up Resources

❌ **Bad**:

```go
file, _ := os.Create("/tmp/test.yaml")
defer file.Close()  // Might not run if test fails
```

✅ **Good**:

```go
testDir := t.TempDir()  // Auto-cleanup
testFile := filepath.Join(testDir, "test.yaml")
```

### Pitfall 3: Ignoring Errors

❌ **Bad**:

```go
result, _ := service.Validate(content)  // Ignoring error
```

✅ **Good**:

```go
result, err := service.Validate(content)
if err != nil {
    t.Fatalf("Unexpected error: %v", err)
}
```

### Pitfall 4: Testing Implementation Instead of Behavior

❌ **Bad** (tests implementation):

```go
t.Run("uses_yaml_v3", func(t *testing.T) {
    // Testing internal library choice
})
```

✅ **Good** (tests behavior):

```go
t.Run("parses_valid_yaml_correctly", func(t *testing.T) {
    // Testing observable behavior
})
```

---

## Summary Checklist

When writing a new behavioral BDD test:

- [ ] Define complete behavioral contract with all fields
- [ ] Use descriptive sub-test names (snake_case)
- [ ] Test both success and error scenarios
- [ ] Use t.TempDir() for file operations
- [ ] Log successes with ✓ prefix
- [ ] Ensure tests are independent
- [ ] Test real behavior, not mocks
- [ ] Clean up resources properly
- [ ] Run tests locally before committing
- [ ] Run with race detector: `go test -race`

---

## Next Steps

1. **Read existing tests**: Start with simpler tests in `pkg/desiredstate/desiredstate_behavioral_bdd_test.go`
2. **Run tests**: `go test -v ./... -run "BehavioralBDD"`
3. **Pick a feature**: Choose an untested feature to add coverage
4. **Write your test**: Follow the patterns in this tutorial
5. **Submit PR**: Include test in your pull request

---

## Additional Resources

- [Behavioral BDD Catalog](BEHAVIORAL_BDD_CATALOG.md) - Complete test documentation
- [Quick Reference](QUICK_REFERENCE.md) - Common test commands
- [Phase 3 Report](../PHASE3_COMPLETION_REPORT.md) - Background on testing strategy

Happy testing! 🎉
