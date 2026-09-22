# Behavioral BDD Test Quick Reference

Quick reference guide for finding and running specific behavioral tests.

---

## Quick Search Index

### By Feature Area

| Feature | Test File | Function | Sub-tests |
|---------|-----------|----------|-----------|
| **YAML Parsing** | desiredstate_behavioral_bdd_test.go | TestParser_LoadDesiredState_BehavioralBDD | 3 |
| **Configuration Loading** | desiredstate_behavioral_bdd_test.go | TestParser_LoadConfiguration_BehavioralBDD | 3 |
| **Schema Detection** | desiredstate_behavioral_bdd_test.go | TestParser_SchemaDetection_BehavioralBDD | 4 |
| **Error Handling** | desiredstate_behavioral_bdd_test.go | TestParser_ErrorHandling_BehavioralBDD | 8 |
| **Wrapper Filtering** | desiredstate_behavioral_bdd_test.go | TestParser_ConfigurationFiltering_BehavioralBDD | 4 |
| **Service Operations** | commands_behavioral_bdd_test.go | TestService_ValidateOperation_BehavioralBDD | 2 |
| **Assembly** | commands_behavioral_bdd_test.go | TestService_AssembleOperation_BehavioralBDD | 2 |
| **Promotion** | commands_behavioral_bdd_test.go | TestService_PromoteOperation_BehavioralBDD | 3 |
| **CLI Parsing** | cli_behavioral_bdd_test.go | TestCLI_CommandParsing_BehavioralBDD | 5 |
| **CLI Flags** | cli_behavioral_bdd_test.go | TestCLI_FlagHandling_BehavioralBDD | 6 |
| **Version Command** | cli_integration_behavioral_bdd_test.go | TestCLI_VersionCommand_BehavioralBDD | 2 |
| **Help Command** | cli_integration_behavioral_bdd_test.go | TestCLI_HelpCommand_BehavioralBDD | 3 |
| **Exit Codes** | cli_integration_behavioral_bdd_test.go | TestCLI_ExitCodes_BehavioralBDD | 3 |
| **Performance** | cli_integration_behavioral_bdd_test.go | TestCLI_PerformanceRequirements_BehavioralBDD | 5 |
| **Validation Workflow** | workflow_e2e_behavioral_bdd_test.go | TestWorkflow_ValidateDesiredState_BehavioralBDD | 4 |
| **Assembly Workflow** | workflow_e2e_behavioral_bdd_test.go | TestWorkflow_AssembleDesiredState_BehavioralBDD | 4 |
| **Error Propagation** | workflow_e2e_behavioral_bdd_test.go | TestWorkflow_ErrorPropagation_BehavioralBDD | 4 |
| **Multi-Environment** | multienv_errorrecovery_behavioral_bdd_test.go | TestMultiEnv_ConfigurationInheritance_BehavioralBDD | 3 |
| **Environment Promotion** | multienv_errorrecovery_behavioral_bdd_test.go | TestMultiEnv_CrossEnvironmentPromotion_BehavioralBDD | 3 |
| **Error Recovery** | multienv_errorrecovery_behavioral_bdd_test.go | TestErrorRecovery_PartialFailures_BehavioralBDD | 4 |
| **Idempotency** | multienv_errorrecovery_behavioral_bdd_test.go | TestErrorRecovery_RetryResilience_BehavioralBDD | 3 |
| **Large File Processing** | loadstress_behavioral_bdd_test.go | TestLoadStress_LargeFileProcessing_BehavioralBDD | 3 |
| **Many Components** | loadstress_behavioral_bdd_test.go | TestLoadStress_ManyComponents_BehavioralBDD | 3 |
| **Concurrent Operations** | loadstress_behavioral_bdd_test.go | TestLoadStress_ConcurrentOperations_BehavioralBDD | 3 |
| **Memory Pressure** | loadstress_behavioral_bdd_test.go | TestLoadStress_MemoryPressure_BehavioralBDD | 2 |

---

## Quick Commands

### Run by Test Name

```bash
# Specific test function
go test -v ./pkg/desiredstate -run "TestParser_LoadDesiredState_BehavioralBDD"

# Specific sub-test
go test -v ./pkg/desiredstate -run "TestParser_LoadDesiredState_BehavioralBDD/load_valid_desiredstate"

# Multiple related tests with regex
go test -v ./pkg/desiredstate -run "TestParser_(LoadDesiredState|LoadConfiguration)_BehavioralBDD"
```

### Run by Package

```bash
# Parser + Service layer
go test -v ./pkg/desiredstate -run "BehavioralBDD"

# CLI layer
go test -v ./internal/cli -run "BehavioralBDD"

# Integration + Workflow layer
go test -v ./cmd/yago -run "BehavioralBDD"
```

### Run by Test Layer

```bash
# Parser layer only
go test -v ./pkg/desiredstate -run "TestParser.*BehavioralBDD"

# Service layer only
go test -v ./pkg/desiredstate -run "TestService.*BehavioralBDD"

# CLI layer only
go test -v ./internal/cli -run "TestCLI.*BehavioralBDD"

# Integration layer only
go test -v ./cmd/yago -run "TestCLI.*BehavioralBDD"

# Workflow layer only
go test -v ./cmd/yago -run "TestWorkflow.*BehavioralBDD"

# Multi-env & error recovery
go test -v ./cmd/yago -run "Test(MultiEnv|ErrorRecovery).*BehavioralBDD"

# Load & stress testing
go test -v ./pkg/desiredstate -run "TestLoadStress.*BehavioralBDD"
```

---

## Troubleshooting Scenarios

### I need to test YAML parsing

```bash
go test -v ./pkg/desiredstate -run "TestParser_(LoadDesiredState|LoadConfiguration)_BehavioralBDD"
```

### I need to test performance under load

```bash
# All load & stress tests (short mode - skips 10MB, 1000 components, 100 concurrent)
go test -v -short ./pkg/desiredstate -run "TestLoadStress.*BehavioralBDD"

# Large file processing only
go test -v ./pkg/desiredstate -run "TestLoadStress_LargeFileProcessing_BehavioralBDD"

# Many components scalability
go test -v ./pkg/desiredstate -run "TestLoadStress_ManyComponents_BehavioralBDD"

# Concurrent operations safety
go test -v ./pkg/desiredstate -run "TestLoadStress_ConcurrentOperations_BehavioralBDD"

# Memory stability
go test -v ./pkg/desiredstate -run "TestLoadStress_MemoryPressure_BehavioralBDD"

# Run full suite including large tests (may take 1-2 minutes)
go test -v ./pkg/desiredstate -run "TestLoadStress.*BehavioralBDD" -timeout 2m
```

### I need to test error handling

```bash
# Parser error handling
go test -v ./pkg/desiredstate -run "TestParser_ErrorHandling_BehavioralBDD"

# Workflow error propagation
go test -v ./cmd/yago -run "TestWorkflow_ErrorPropagation_BehavioralBDD"

# Error recovery
go test -v ./cmd/yago -run "TestErrorRecovery.*BehavioralBDD"
```

### I need to test command-line interface

```bash
# CLI parsing
go test -v ./internal/cli -run "TestCLI_CommandParsing_BehavioralBDD"

# CLI integration (actual binary)
go test -v ./cmd/yago -run "TestCLI_(VersionCommand|HelpCommand)_BehavioralBDD"
```

### I need to test performance

```bash
# Performance behavioral tests
go test -v ./cmd/yago -run "TestCLI_PerformanceRequirements_BehavioralBDD"

# Benchmarks
go test -bench=. -benchmem ./pkg/desiredstate
```

### I need to test multi-environment workflows

```bash
# All multi-env tests
go test -v ./cmd/yago -run "TestMultiEnv.*BehavioralBDD"

# Specific scenarios
go test -v ./cmd/yago -run "TestMultiEnv_ConfigurationInheritance_BehavioralBDD"
go test -v ./cmd/yago -run "TestMultiEnv_CrossEnvironmentPromotion_BehavioralBDD"
```

### I need to test specific workflows

```bash
# Validation workflow
go test -v ./cmd/yago -run "TestWorkflow_ValidateDesiredState_BehavioralBDD"

# Assembly workflow
go test -v ./cmd/yago -run "TestWorkflow_AssembleDesiredState_BehavioralBDD"

# Complete pipeline
go test -v ./cmd/yago -run "TestWorkflow_CompletePipeline_BehavioralBDD"
```

---

## Common Test Patterns

### Run with Coverage

```bash
# Generate coverage
go test -v -coverprofile=coverage.out ./... -run "BehavioralBDD"

# View HTML coverage report
go tool cover -html=coverage.out

# View coverage summary
go tool cover -func=coverage.out
```

### Run with Race Detection

```bash
# All tests with race detector
go test -race ./... -run "BehavioralBDD"

# Specific package with race detector
go test -race ./cmd/yago -run "TestErrorRecovery_RetryResilience_BehavioralBDD"
```

### Run Specific Sub-test

```bash
# Format: TestFunction/subtest_name
go test -v ./pkg/desiredstate -run "TestParser_ErrorHandling_BehavioralBDD/invalid_request_type"
```

### Run Multiple Specific Tests

```bash
# Use regex alternation
go test -v ./cmd/yago -run "TestCLI_(Version|Help)Command_BehavioralBDD"
```

---

## Test File Locations

```
yago/
├── pkg/desiredstate/
│   ├── desiredstate_behavioral_bdd_test.go     # Parser layer (42 contracts)
│   ├── commands_behavioral_bdd_test.go         # Service layer (7 contracts)
│   └── benchmarks_test.go                      # Performance benchmarks (10)
├── internal/cli/
│   └── cli_behavioral_bdd_test.go              # CLI layer (5 contracts)
└── cmd/yago/
    ├── cli_integration_behavioral_bdd_test.go  # Integration layer (7 contracts)
    ├── workflow_e2e_behavioral_bdd_test.go     # Workflow layer (5 contracts)
    └── multienv_errorrecovery_behavioral_bdd_test.go  # Multi-env + error recovery (4 contracts)
```

---

## Performance Targets

Quick reference for performance thresholds:

| Command | Target | Test Function |
|---------|--------|---------------|
| `yago version` | < 100ms | TestCLI_PerformanceRequirements_BehavioralBDD/version_command_performance |
| `yago --help` | < 500ms | TestCLI_PerformanceRequirements_BehavioralBDD/help_command_performance |
| `yago validate` (small) | < 2s | TestCLI_PerformanceRequirements_BehavioralBDD/validate_small_file_performance |
| Service initialization | N/A | BenchmarkService_Initialize |
| Validate operation | N/A | BenchmarkService_Validate |
| Assemble operation | N/A | BenchmarkService_Assemble |

---

## CI/CD Integration

### GitHub Actions Workflows

```yaml
# .github/workflows/behavioral-bdd-tests.yml
# Runs on: push to main, develop; pull requests
# Jobs:
#   - behavioral-tests (Go 1.21, 1.22, 1.23)
#   - behavioral-contract-validation
#   - performance-check
#   - test-quality-gate

# .github/workflows/pr-check.yml
# Runs on: pull requests
# Quick validation (< 10 minutes)
```

### Local Pre-commit

```bash
#!/bin/bash
# Run before committing behavioral test changes

echo "Running behavioral BDD tests..."
go test -v ./... -run "BehavioralBDD" || exit 1

echo "Running race detection..."
go test -race ./... -run "BehavioralBDD" || exit 1

echo "Checking coverage..."
go test -coverprofile=coverage.out ./... -run "BehavioralBDD"
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')

if (( $(echo "$COVERAGE < 70" | bc -l) )); then
    echo "Coverage too low: ${COVERAGE}% (minimum: 70%)"
    exit 1
fi

echo "✅ All pre-commit checks passed!"
```

---

## Getting Help

- **Full Documentation**: [BEHAVIORAL_BDD_CATALOG.md](BEHAVIORAL_BDD_CATALOG.md)
- **Contributing Guide**: [BEHAVIORAL_BDD_CATALOG.md#contributing](BEHAVIORAL_BDD_CATALOG.md#contributing)
- **Testing Patterns**: [BEHAVIORAL_BDD_CATALOG.md#testing-patterns](BEHAVIORAL_BDD_CATALOG.md#testing-patterns)
- **Phase 3 Report**: [../PHASE3_COMPLETION_REPORT.md](../PHASE3_COMPLETION_REPORT.md)

---

**Quick Tip**: Use `go test -list "BehavioralBDD"` to list all behavioral BDD tests without running them.
