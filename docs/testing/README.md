# yago Testing Documentation Hub

Welcome to the comprehensive testing documentation for yago. This hub provides everything you need to understand, run, and contribute to yago's behavioral BDD test suite.

---

## 📚 Documentation Index

### For Users

- **[Quick Reference Guide](QUICK_REFERENCE.md)** ⚡
  - Quick search index by feature
  - Common test commands
  - Troubleshooting scenarios
  - Performance targets
  - **Start here if you want to run specific tests**

### For Contributors

- **[Tutorial: Writing Behavioral BDD Tests](TUTORIAL.md)** 📖
  - Step-by-step guide for new contributors
  - 4 comprehensive tutorials covering:
    - Writing your first behavioral contract
    - Testing error handling
    - Integration testing
    - Performance testing
  - Best practices and common pitfalls
  - **Start here if you want to write new tests**

- **[Test Helpers Package](../../pkg/desiredstate/testhelpers/README.md)** 🛠️
  - Reusable test utilities and helpers
  - File management, YAML generators, assertions
  - Service setup utilities, logging helpers
  - Reduces test boilerplate by ~60%
  - [Migration Examples](../../pkg/desiredstate/testhelpers/MIGRATION_EXAMPLES.md)
  - **Use this to write cleaner, more maintainable tests**

### For Reference

- **[Behavioral BDD Test Catalog](BEHAVIORAL_BDD_CATALOG.md)** 📋
  - Complete catalog of all 68 behavioral contracts
  - Organized by test layer:
    - Parser Layer (42 contracts)
    - Service Layer (7 contracts)
    - CLI Layer (5 contracts)
    - Integration Layer (7 contracts)
    - Workflow Layer (5 contracts)
    - Multi-Environment & Error Recovery (4 contracts)
    - Load & Stress Testing (4 contracts)
  - Testing patterns and best practices
  - Performance baselines
  - **Start here for comprehensive test documentation**

---

## 🎯 Quick Start

### Run All Behavioral BDD Tests

```bash
cd /path/to/yago
go test -v ./... -run "BehavioralBDD"
```

### Run Tests with Coverage

```bash
go test -v -coverprofile=coverage.out ./... -run "BehavioralBDD"
go tool cover -html=coverage.out
```

### Run Specific Test Layer

```bash
# Parser layer
go test -v ./pkg/desiredstate -run "TestParser.*BehavioralBDD"

# Service layer
go test -v ./pkg/desiredstate -run "TestService.*BehavioralBDD"

# CLI layer
go test -v ./internal/cli -run "BehavioralBDD"

# Integration layer
go test -v ./cmd/yago -run "TestCLI.*BehavioralBDD"

# Workflow layer
go test -v ./cmd/yago -run "TestWorkflow.*BehavioralBDD"

# Multi-environment & error recovery
go test -v ./cmd/yago -run "Test(MultiEnv|ErrorRecovery).*BehavioralBDD"
```

### Run Performance Benchmarks

```bash
go test -bench=. -benchmem ./pkg/desiredstate
```

---

## 📊 Test Suite Overview

### Statistics

- **Total Test Files**: 6
- **Total Lines of Test Code**: 5,927 lines
- **Total Behavioral Contracts**: 64
- **Total Sub-tests**: 150+
- **Pass Rate**: 100% ✅

### Test Files

```
yago/
├── pkg/desiredstate/
│   ├── desiredstate_behavioral_bdd_test.go     # Parser (42 contracts, 2,209 lines)
│   ├── commands_behavioral_bdd_test.go         # Service (7 contracts, 624 lines)
│   └── benchmarks_test.go                      # Benchmarks (10 benchmarks, 431 lines)
├── internal/cli/
│   └── cli_behavioral_bdd_test.go              # CLI (5 contracts, 868 lines)
└── cmd/yago/
    ├── cli_integration_behavioral_bdd_test.go  # Integration (7 contracts, 824 lines)
    ├── workflow_e2e_behavioral_bdd_test.go     # Workflows (5 contracts, 804 lines)
    └── multienv_errorrecovery_behavioral_bdd_test.go  # Multi-env (4 contracts, 598 lines)
```

### Test Layers

| Layer | Purpose | Contracts | File |
|-------|---------|-----------|------|
| **Parser** | YAML parsing, schema detection, validation | 42 | desiredstate_behavioral_bdd_test.go |
| **Service** | Business logic, operations, commands | 7 | commands_behavioral_bdd_test.go |
| **CLI** | Command-line parsing, flags, configuration | 5 | cli_behavioral_bdd_test.go |
| **Integration** | Real binary execution, exit codes, output | 7 | cli_integration_behavioral_bdd_test.go |
| **Workflow** | End-to-end workflows, error propagation | 5 | workflow_e2e_behavioral_bdd_test.go |
| **Multi-Env** | Multi-environment, error recovery, idempotency | 4 | multienv_errorrecovery_behavioral_bdd_test.go |

---

## 🔍 Find What You Need

### I want to

**...understand the behavioral BDD approach**
→ Read [Behavioral BDD Catalog - Overview](BEHAVIORAL_BDD_CATALOG.md#overview)

**...run specific tests**
→ Check [Quick Reference - Quick Commands](QUICK_REFERENCE.md#quick-commands)

**...write my first test**
→ Follow [Tutorial - Your First Behavioral Contract](TUTORIAL.md#tutorial-1-your-first-behavioral-contract)

**...test error handling**
→ Follow [Tutorial - Testing Error Handling](TUTORIAL.md#tutorial-2-testing-error-handling)

**...write integration tests**
→ Follow [Tutorial - Integration Testing](TUTORIAL.md#tutorial-3-integration-testing)

**...add performance tests**
→ Follow [Tutorial - Performance Testing](TUTORIAL.md#tutorial-4-performance-testing)

**...find tests for a specific feature**
→ Use [Quick Reference - Quick Search Index](QUICK_REFERENCE.md#quick-search-index)

**...understand test patterns**
→ Read [Behavioral BDD Catalog - Testing Patterns](BEHAVIORAL_BDD_CATALOG.md#testing-patterns)

**...troubleshoot test failures**
→ Check [Quick Reference - Troubleshooting Scenarios](QUICK_REFERENCE.md#troubleshooting-scenarios)

**...see performance baselines**
→ View [Behavioral BDD Catalog - Performance Baselines](BEHAVIORAL_BDD_CATALOG.md#performance-baselines)

---

## 🏗️ Architecture

### Behavioral Contract Structure

Every behavioral test follows this pattern:

```go
type BehavioralContract struct {
    Behavior      string  // What this behavior does
    PythonImpl    string  // Python gitops implementation
    GoImpl        string  // Go yago implementation
    ContractMatch bool    // Whether they match behaviorally
    Differences   string  // Key differences
    Rationale     string  // Why this is important
}
```

### Test Hierarchy

```
Behavioral BDD Test Function (e.g., TestParser_LoadDesiredState_BehavioralBDD)
├── Behavioral Contract Definition
├── Sub-test 1: Specific scenario (e.g., load_valid_desiredstate)
├── Sub-test 2: Another scenario (e.g., load_missing_file)
├── Sub-test 3: Error scenario (e.g., load_invalid_yaml)
└── Contract Log
```

### Coverage by Feature

| Feature Area | Coverage | Test Count |
|--------------|----------|------------|
| YAML Parsing | ✅ Comprehensive | 10+ contracts |
| Validation | ✅ Comprehensive | 8+ contracts |
| Assembly | ✅ Comprehensive | 5+ contracts |
| CLI Interface | ✅ Comprehensive | 12+ contracts |
| Error Handling | ✅ Comprehensive | 10+ contracts |
| Multi-Environment | ✅ Comprehensive | 4 contracts |
| Performance | ✅ Comprehensive | 5+ contracts + 10 benchmarks |

---

## 🚀 CI/CD Integration

### GitHub Actions Workflows

**Behavioral BDD Tests** (`.github/workflows/behavioral-bdd-tests.yml`)

- Runs on: Push to main/develop, pull requests
- Go versions: 1.21, 1.22, 1.23
- Jobs:
  - `behavioral-tests`: Core test execution
  - `behavioral-contract-validation`: Contract structure validation
  - `performance-check`: Performance threshold checks
  - `test-quality-gate`: Final quality gate

**PR Quality Check** (`.github/workflows/pr-check.yml`)

- Runs on: Pull requests
- Fast validation (< 10 minutes)
- Quick feedback for PR authors

### Badges

![Behavioral BDD Tests](https://github.com/yourusername/yago/workflows/Behavioral%20BDD%20Tests/badge.svg)
![PR Quality Check](https://github.com/yourusername/yago/workflows/PR%20Quality%20Check/badge.svg)
![Go Report Card](https://goreportcard.com/badge/github.com/yourusername/yago)
![Codecov](https://codecov.io/gh/yourusername/yago/branch/main/graph/badge.svg)

---

## 📈 Performance Targets

| Operation | Target | Typical | Status |
|-----------|--------|---------|--------|
| `yago version` | < 100ms | ~5ms | ✅ |
| `yago --help` | < 500ms | ~6ms | ✅ |
| Validate small file | < 2s | ~20-50ms | ✅ |
| Service initialization | N/A | ~10-30ms | ✅ |
| Assemble operation | < 2s | ~50-200ms | ✅ |
| Large file (100 components) | < 5s | ~200-500ms | ✅ |

---

## 🤝 Contributing

### Before Writing Tests

1. Read the [Tutorial](TUTORIAL.md)
2. Review existing test patterns in [Behavioral BDD Catalog](BEHAVIORAL_BDD_CATALOG.md#testing-patterns)
3. Check [Quick Reference](QUICK_REFERENCE.md) for similar tests

### Test Writing Checklist

- [ ] Define complete behavioral contract
- [ ] Use descriptive sub-test names (snake_case)
- [ ] Test both success and error scenarios
- [ ] Use `t.TempDir()` for file operations
- [ ] Log successes with ✓ prefix
- [ ] Ensure tests are independent
- [ ] Test real behavior, not mocks
- [ ] Run with race detector: `go test -race`

### Before Submitting PR

```bash
# Run all tests
go test -v ./... -run "BehavioralBDD"

# Check race conditions
go test -race ./... -run "BehavioralBDD"

# Verify coverage
go test -coverprofile=coverage.out ./... -run "BehavioralBDD"
go tool cover -func=coverage.out
```

---

## 📖 Related Documentation

- [README Testing Section](../../README.md#testing) - Project overview
- [Phase 3 Completion Report](../PHASE3_COMPLETION_REPORT.md) - Phase 3 achievements
- [Priority 1 CLI Contracts](../PRIORITY1_CLI_CONTRACTS_COMPLETE.md) - CLI documentation

---

## 🆘 Getting Help

### Documentation Issues

If you find errors or have suggestions for the documentation:

1. Open an issue on GitHub
2. Tag with `documentation` label
3. Provide specific feedback or corrections

### Test Failures

If tests are failing:

1. Check [Quick Reference - Troubleshooting](QUICK_REFERENCE.md#troubleshooting-scenarios)
2. Run with verbose output: `go test -v`
3. Check for race conditions: `go test -race`
4. Review recent changes with `git diff`

### Questions

For questions about testing:

1. Check this documentation hub first
2. Review existing tests for examples
3. Open a discussion on GitHub

---

## 📅 Documentation Status

- **Last Updated**: Phase 4, October 2025
- **Maintained By**: yago development team
- **Status**: ✅ Complete and up-to-date
- **Test Suite Version**: Phase 4
- **Coverage**: 64 behavioral contracts, 150+ sub-tests

---

**Navigate**: [Quick Reference](QUICK_REFERENCE.md) | [Tutorial](TUTORIAL.md) | [Complete Catalog](BEHAVIORAL_BDD_CATALOG.md)
