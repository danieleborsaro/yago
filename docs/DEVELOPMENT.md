# Development Setup

This document describes how to set up the development environment for YAGO, including linting tools and pre-commit hooks.

## Prerequisites

- **Go 1.27.1+**: For building and running the application
- **Python 3.8+**: For pre-commit hooks and yamllint
- **Git**: For version control and pre-commit integration

## Quick Start

1. **Clone the repository**:

   ```bash
   git clone git@github.com-personal:danieleborsaro/yago.git
   cd yago
   ```

2. **Install dependencies**:

   ```bash
   make deps
   ```

3. **Install development tools**:

   ```bash
   # Install pre-commit hooks
   make pre-commit-install
   
   # Install linting tools
   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
   pip install yamllint
   ```

4. **Run initial checks**:

   ```bash
   # Run all linters
   make lint
   
   # Run pre-commit hooks
   make pre-commit-run
   ```

## Linting Tools

### Go Linting

YAGO uses **golangci-lint** v2 with the standard linters (errcheck, govet, ineffassign, staticcheck, unused) and the gofmt and goimports formatters. The existing code still has findings, so CI only reports issues that are new since `origin/main`. gosec runs separately (`make security-scan`).

**Configuration**: `.golangci.yml`

**Usage**:

```bash
# Run Go linting
make lint-go

# Run specific linter
golangci-lint run --default=none --enable=gosec

# Auto-fix issues
make lint-fix
```

### YAML Linting

YAGO uses **yamllint** optimized for GitOps configurations:

- **Line Length**: 120 characters (standard for GitOps files)
- **Indentation**: 2 spaces, consistent with Kubernetes YAML
- **Comments**: Enabled with proper spacing
- **Document Structure**: Single document per file
- **Key Ordering**: Disabled (flexibility for GitOps manifests)

**Configuration**: `.yamllint.yaml`

**Usage**:

```bash
# Run YAML linting
make lint-yaml

# Check specific file
yamllint path/to/file.yaml

# Show available rules
yamllint --list-files
```

## Pre-commit Hooks

Pre-commit hooks run automatically before each commit to ensure code quality.

### Installed Hooks

1. **Go Hooks**:
   - `gofmt`: Format Go code
   - `goimports`: Organize imports
   - `go-mod-tidy`: Keep go.mod clean
   - `go-unit-tests`: Run the unit tests

2. **YAML Hooks**:
   - `yamllint`: YAML syntax and style checking

3. **Security Hooks**:
   - `gosec`: Security vulnerability scanning
   - `detect-private-key`: Prevent committing private keys
   - `detect-aws-credentials`: Prevent committing AWS credentials

4. **General Hooks**:
   - `trailing-whitespace`: Remove trailing spaces
   - `end-of-file-fixer`: Ensure files end with newline
   - `check-merge-conflict`: Detect merge conflict markers
   - `check-added-large-files`: Prevent large file commits

### Pre-commit Commands

```bash
# Install hooks (run once)
make pre-commit-install

# Run hooks manually
make pre-commit-run

# Run on specific files
pre-commit run --files src/main.go

# Skip hooks for emergency commits
git commit --no-verify -m "Emergency fix"

# Update hook versions
make pre-commit-update
```

## GitHub Actions Integration

### CI Workflow (`.github/workflows/ci.yml`)

- Runs on push to main/develop branches
- Includes lint job that must pass before tests
- Caches Go modules and dependencies

### Linting Workflow (`.github/workflows/lint.yml`)

- Runs on pull requests
- Separate jobs for Go, YAML, and security scanning
- Provides detailed feedback on code quality issues

### Workflow Features

- **Parallel Execution**: Multiple linting jobs run simultaneously
- **Caching**: Dependencies cached for faster runs
- **Fail Fast**: Stops on first critical error
- **Detailed Reporting**: Clear error messages and suggestions

## Local Development Workflow

### Before Making Changes

1. **Update dependencies**:

   ```bash
   make deps
   ```

2. **Run linters**:

   ```bash
   make lint
   ```

### During Development

1. **Auto-fix common issues**:

   ```bash
   make lint-fix
   ```

2. **Run specific checks**:

   ```bash
   # Only Go linting
   make lint-go
   
   # Only YAML linting  
   make lint-yaml
   
   # Security scan
   make security-scan
   ```

### Before Committing

1. **Run all checks**:

   ```bash
   make pre-commit-run
   ```

2. **Fix any issues** reported by the tools

3. **Commit** (pre-commit hooks will run automatically)

## Troubleshooting

### Common Issues

**1. golangci-lint timeout**:

```bash
# Increase timeout
golangci-lint run --timeout=10m
```

**2. Pre-commit hook failures**:

```bash
# See detailed output
pre-commit run --verbose --all-files

# Run specific hook
pre-commit run go-unit-tests --all-files
```

**3. YAML linting errors**:

```bash
# Check specific file with context
yamllint -f parsable file.yaml

# Disable specific rule for file
# Add to top of YAML file: # yamllint disable rule:line-length
```

**4. Import organization issues**:

```bash
# Fix imports automatically
goimports -w .

# Show what the gofmt and goimports formatters would change
golangci-lint fmt --diff
```

### Performance Tips

1. **Use caching**:
   - Pre-commit automatically caches hook environments
   - golangci-lint caches analysis results

2. **Run targeted checks**:

   ```bash
   # Only check changed files
   pre-commit run --files $(git diff --name-only HEAD~1)
   
   # Only new issues since main
   golangci-lint run --new-from-rev=origin/main
   ```

3. **Parallel execution**:

   ```bash
   # Run linting jobs in parallel
   make lint-go & make lint-yaml & wait
   ```

## Integration with IDEs

### VS Code

1. Install recommended extensions:
   - Go extension (golang.go)
   - YAML extension (redhat.vscode-yaml)
   - golangci-lint extension

2. Configure settings in `.vscode/settings.json`:

   ```json
   {
     "go.lintTool": "golangci-lint",
     "go.lintFlags": ["--fast"],
     "yaml.validate": true,
     "yaml.format.enable": true
   }
   ```

### GoLand/IntelliJ

1. Enable golangci-lint integration:
   - Go to Preferences → Tools → Go Linter
   - Select golangci-lint as linter
   - Point to `.golangci.yml` config

2. Configure YAML support:
   - Install YAML plugin
   - Configure yamllint external tool

## Contributing

When contributing to YAGO:

1. **Follow the linting rules** - they ensure code consistency
2. **Run pre-commit hooks** - they catch issues early
3. **Write tests** for new functionality
4. **Update documentation** when changing behavior
5. **Keep commits focused** - one logical change per commit

For questions about the development setup, please check the existing issues or create a new one.
