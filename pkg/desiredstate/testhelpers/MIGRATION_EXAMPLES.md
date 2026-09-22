# Test Helpers Migration Examples

This document shows before/after examples of migrating existing tests to use the testhelpers package.

---

## Example 1: Simple File Writing

### Before (Without testhelpers)

```go
func TestValidation(t *testing.T) {
    tmpDir := t.TempDir()
    testFile := filepath.Join(tmpDir, "test.yaml")
    
    content := `schema: v1
kind: DesiredState
metadata:
  name: test
  namespace: yago`
    
    if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
        t.Fatalf("Failed to write test file: %v", err)
    }
    
    // Rest of test...
}
```

**Line Count**: 14 lines of boilerplate

### After (With testhelpers)

```go
func TestValidation(t *testing.T) {
    content := testhelpers.GenerateDesiredStateYAML()
    testFile := testhelpers.WriteYAMLFile(t, content)
    
    // Rest of test...
}
```

**Line Count**: 4 lines total

**Reduction**: 71% less code (14 → 4 lines)

---

## Example 2: Custom YAML Generation

### Before (Without testhelpers)

```go
func TestWithComponents(t *testing.T) {
    tmpDir := t.TempDir()
    testFile := filepath.Join(tmpDir, "test.yaml")
    
    content := fmt.Sprintf(`schema: v1
kind: DesiredState
metadata:
  name: %s
  namespace: %s
spec:
  components:
    - name: component-0
      type: service
      version: 1.0.0
    - name: component-1
      type: service
      version: 1.0.1
    - name: component-2
      type: service
      version: 1.0.2`, "my-test", "production")
    
    if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
        t.Fatalf("Failed to write test file: %v", err)
    }
    
    // Rest of test...
}
```

**Line Count**: 25 lines

### After (With testhelpers)

```go
func TestWithComponents(t *testing.T) {
    opts := testhelpers.DefaultDesiredStateOptions()
    opts.Name = "my-test"
    opts.Namespace = "production"
    opts.ComponentCount = 3
    
    content := testhelpers.GenerateDesiredStateYAMLWithOptions(opts)
    testFile := testhelpers.WriteYAMLFile(t, content)
    
    // Rest of test...
}
```

**Line Count**: 9 lines

**Reduction**: 64% less code (25 → 9 lines)

---

## Example 3: Error Assertions

### Before (Without testhelpers)

```go
func TestErrorHandling(t *testing.T) {
    err := service.ValidateInvalid(req)
    
    if err == nil {
        t.Fatal("Expected error but got nil")
    }
    
    if !strings.Contains(err.Error(), "PARAM_ERROR") {
        t.Fatalf("Expected PARAM_ERROR in error message, got: %v", err)
    }
    
    response, err := service.ValidateValid(req)
    
    if err != nil {
        t.Fatalf("Expected no error, got: %v", err)
    }
    
    if !response.IsValid {
        t.Fatal("Expected response.IsValid to be true")
    }
}
```

**Line Count**: 17 lines

### After (With testhelpers)

```go
func TestErrorHandling(t *testing.T) {
    err := service.ValidateInvalid(req)
    testhelpers.AssertErrorContains(t, err, "PARAM_ERROR", "should return parameter error")
    
    response, err := service.ValidateValid(req)
    testhelpers.AssertNoError(t, err, "validation should succeed")
    testhelpers.AssertTrue(t, response.IsValid, "response should be valid")
}
```

**Line Count**: 7 lines

**Reduction**: 59% less code (17 → 7 lines)

---

## Example 4: Service Setup

### Before (Without testhelpers)

```go
func TestServiceOperations(t *testing.T) {
    tmpDir := t.TempDir()
    
    dsContent := `schema: v1
kind: DesiredState
metadata:
  name: test
  namespace: yago`
    
    configContent := `schema: v1
kind: Configuration
metadata:
  name: test-config
  namespace: yago`
    
    dsFile := filepath.Join(tmpDir, "desiredstate.yaml")
    if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
        t.Fatalf("Failed to write desiredstate file: %v", err)
    }
    
    configFile := filepath.Join(tmpDir, "configuration.yaml")
    if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
        t.Fatalf("Failed to write configuration file: %v", err)
    }
    
    service := desiredstate.NewService(tmpDir, true)
    
    // Rest of test...
}
```

**Line Count**: 27 lines

### After (With testhelpers)

```go
func TestServiceOperations(t *testing.T) {
    setup := testhelpers.NewServiceSetup(t,
        testhelpers.GenerateDesiredStateYAML(),
        testhelpers.GenerateConfigurationYAML())
    
    service := desiredstate.NewService(setup.TempDir, true)
    
    // Use setup.DSFile and setup.ConfigFile
    // Rest of test...
}
```

**Line Count**: 8 lines

**Reduction**: 70% less code (27 → 8 lines)

---

## Example 5: Load Testing

### Before (Without testhelpers)

```go
func TestLargeFileProcessing(t *testing.T) {
    tmpDir := t.TempDir()
    testFile := filepath.Join(tmpDir, "large.yaml")
    
    var sb strings.Builder
    sb.WriteString("schema: v1\nkind: DesiredState\n")
    sb.WriteString("metadata:\n  name: large-test\n")
    sb.WriteString("spec:\n  components:\n")
    
    for i := 0; i < 1000; i++ {
        sb.WriteString(fmt.Sprintf(`    - name: component-%d
      type: service
      version: 1.0.%d
      properties:
        image: registry.example.com/app:v%d
        port: %d
        replicas: 3
`, i, i%100, i%50, 8000+(i%1000)))
    }
    
    content := sb.String()
    if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
        t.Fatalf("Failed to write test file: %v", err)
    }
    
    start := time.Now()
    // ... processing
    elapsed := time.Since(start)
    
    if elapsed > 500*time.Millisecond {
        t.Logf("Processing took %v (target: <500ms)", elapsed)
    }
}
```

**Line Count**: 30 lines

### After (With testhelpers)

```go
func TestLargeFileProcessing(t *testing.T) {
    content := testhelpers.GenerateLargeDesiredStateYAML(1000)
    testFile := testhelpers.WriteYAMLFile(t, content)
    
    start := time.Now()
    // ... processing
    elapsed := time.Since(start)
    
    if elapsed > 500*time.Millisecond {
        testhelpers.LogWarning(t, fmt.Sprintf("Processing took %v (target: <500ms)", elapsed))
    } else {
        testhelpers.LogSuccess(t, fmt.Sprintf("Processed in %v", elapsed))
    }
}
```

**Line Count**: 12 lines

**Reduction**: 60% less code (30 → 12 lines)

---

## Example 6: Behavioral Contract Testing

### Before (Without testhelpers)

```go
func TestBehavioralContract(t *testing.T) {
    contract := BehavioralContract{
        Behavior: "Validate YAML schema",
        // ... contract details
    }
    
    tmpDir := t.TempDir()
    testFile := filepath.Join(tmpDir, "test.yaml")
    
    content := `schema: v1
kind: DesiredState
metadata:
  name: test
  namespace: yago`
    
    if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
        t.Fatalf("Failed to write test file: %v", err)
    }
    
    service := desiredstate.NewService(tmpDir, true)
    req := desiredstate.ValidateRequest{
        DesiredStateFile: testFile,
        Environment:      "test",
    }
    
    response, err := service.ValidateDesiredState(req)
    
    if err != nil {
        t.Fatalf("Validation should succeed, got error: %v", err)
    }
    
    if !response.DesiredStateValidated {
        t.Fatal("Schema should validate")
    }
    
    if contract.ContractMatch {
        t.Logf("✓ Contract: %s - Match: %v", contract.Behavior, contract.ContractMatch)
    } else {
        t.Logf("⚠️  Contract: %s - Match: %v - Differences: %s", 
            contract.Behavior, contract.ContractMatch, contract.Differences)
    }
}
```

**Line Count**: 39 lines

### After (With testhelpers)

```go
func TestBehavioralContract(t *testing.T) {
    contract := BehavioralContract{
        Behavior: "Validate YAML schema",
        // ... contract details
    }
    
    content := testhelpers.GenerateDesiredStateYAML()
    testFile := testhelpers.WriteYAMLFile(t, content)
    
    service := desiredstate.NewService(filepath.Dir(testFile), true)
    req := desiredstate.ValidateRequest{
        DesiredStateFile: testFile,
        Environment:      "test",
    }
    
    response, err := service.ValidateDesiredState(req)
    testhelpers.AssertNoError(t, err, "validation should succeed")
    testhelpers.AssertTrue(t, response.DesiredStateValidated, "schema should validate")
    
    testhelpers.LogContract(t, contract.Behavior, contract.ContractMatch, contract.Differences)
}
```

**Line Count**: 20 lines

**Reduction**: 49% less code (39 → 20 lines)

---

## Summary of Improvements

| Example | Before | After | Reduction | Benefit |
|---------|--------|-------|-----------|---------|
| Simple File Writing | 14 lines | 4 lines | 71% | Less boilerplate, clearer intent |
| Custom YAML | 25 lines | 9 lines | 64% | Reusable options, type-safe |
| Error Assertions | 17 lines | 7 lines | 59% | Better error messages, cleaner |
| Service Setup | 27 lines | 8 lines | 70% | Encapsulated setup, reusable |
| Load Testing | 30 lines | 12 lines | 60% | Consistent large file generation |
| Behavioral Contract | 39 lines | 20 lines | 49% | Focused on contract, not setup |

**Average Code Reduction**: **62%**

**Additional Benefits**:

- ✅ Automatic cleanup of temp files
- ✅ Consistent error messages
- ✅ Visual logging (✓, ⚠️, ℹ️)
- ✅ Type-safe YAML generation
- ✅ Easier to write new tests
- ✅ Centralized test utilities

---

## Migration Checklist

When migrating existing tests:

- [ ] Replace `t.TempDir()` + `os.WriteFile` with `testhelpers.WriteYAMLFile()`
- [ ] Replace hardcoded YAML with `testhelpers.GenerateDesiredStateYAML()`
- [ ] Replace custom error checks with `testhelpers.AssertNoError()` / `testhelpers.AssertError()`
- [ ] Replace `strings.Contains()` checks with `testhelpers.AssertContains()`
- [ ] Replace boolean checks with `testhelpers.AssertTrue()` / `testhelpers.AssertFalse()`
- [ ] Replace service setup boilerplate with `testhelpers.NewServiceSetup()`
- [ ] Replace `t.Logf()` with `testhelpers.LogSuccess()` / `LogWarning()` / `LogInfo()`
- [ ] Replace contract logging with `testhelpers.LogContract()`

---

## Priority Order for Migration

1. **High Priority**: New tests (use helpers from the start)
2. **Medium Priority**: Frequently failing tests (improve reliability)
3. **Low Priority**: Stable tests (migrate opportunistically)

---

## Example Pull Request Template

```markdown
## Refactor: Migrate tests to use testhelpers package

### Changes
- Migrated `TestValidation` to use testhelpers
- Reduced test code by 65% (42 → 15 lines)
- Improved error messages and readability

### Before
```go
// 42 lines of boilerplate setup...
```

### After

```go
func TestValidation(t *testing.T) {
    content := testhelpers.GenerateDesiredStateYAML()
    testFile := testhelpers.WriteYAMLFile(t, content)
    
    // ... focused test logic ...
    
    testhelpers.AssertNoError(t, err, "validation should succeed")
    testhelpers.LogSuccess(t, "Test passed")
}
```

### Benefits

- Clearer test intent
- Less boilerplate
- Consistent error messages
- Easier maintenance

```

---

## Next Steps

After migrating tests:

1. Run tests to verify behavior unchanged
2. Update documentation if test examples changed
3. Consider extracting additional common patterns
4. Share learnings with team
