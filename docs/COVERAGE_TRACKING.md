# Coverage Tracking & CI Integration

Comprehensive guide to yago's code coverage tracking, CI/CD integration, and quality gates.

---

## Table of Contents

- [Overview](#overview)
- [Coverage Workflows](#coverage-workflows)
- [Coverage Thresholds](#coverage-thresholds)
- [Coverage Reports](#coverage-reports)
- [CI/CD Integration](#cicd-integration)
- [Coverage Badges](#coverage-badges)
- [Trend Tracking](#trend-tracking)
- [Local Coverage](#local-coverage)
- [Quality Gates](#quality-gates)
- [Troubleshooting](#troubleshooting)

---

## Overview

### Coverage Strategy

yago implements a **multi-layered coverage strategy**:

1. **Full Coverage** - All tests across all packages
2. **Behavioral BDD Coverage** - Behavioral contract tests only
3. **Package-Specific Coverage** - Individual package analysis
4. **Trend Tracking** - Historical coverage data
5. **Quality Gates** - Automated threshold enforcement

### Coverage Targets

| Type | Target | Current | Status |
|------|--------|---------|--------|
| **Overall Coverage** | ≥ 70% | TBD | 🎯 Target set |
| **Critical Packages** | ≥ 80% | TBD | 🎯 Target set |
| **Behavioral BDD** | - | Tracked | ✅ Monitored |
| **Trend** | Increasing | Tracked | 📈 Monitored |

---

## Coverage Workflows

### 1. Full Coverage Workflow

**File**: `.github/workflows/coverage.yml`

**Triggers**:

- Push to `main`, `develop`, `feature/*`
- Pull requests to `main`, `develop`
- Manual dispatch
- Daily at 2 AM UTC (cron schedule)

**Jobs**:

#### `full-coverage`

Comprehensive coverage analysis across all packages.

```yaml
- Runs all tests with coverage
- Generates multiple report formats (text, HTML, XML, JSON)
- Calculates total coverage percentage
- Creates package-level breakdown
- Uploads to Codecov and Coveralls
- Enforces 70% threshold
```

**Outputs**:

- `coverage.out` - Go coverage profile
- `coverage.html` - Interactive HTML report
- `coverage.xml` - XML report for tools
- `coverage.json` - JSON for analysis
- `coverage-summary.txt` - Text summary
- `package-coverage.md` - Package breakdown

#### `behavioral-coverage`

Tracks coverage from behavioral BDD tests only.

```yaml
- Runs behavioral tests with coverage
- Calculates behavioral-specific coverage
- Uploads separate coverage profile
```

#### `package-coverage`

Matrix job analyzing individual packages.

```yaml
Strategy matrix:
  - pkg/desiredstate
  - pkg/wrapper
  - internal/core
  - internal/parser
  - internal/cli
  - cmd/yago
```

#### `coverage-trends`

Tracks coverage over time (main/develop only).

```yaml
- Downloads coverage data
- Appends to historical CSV
- Generates trend chart
- Commits history back to repo
```

#### `coverage-badge`

Generates coverage badges (main branch only).

```yaml
- Extracts coverage percentage
- Determines badge color (green/yellow/orange/red)
- Updates Gist-based badge
- Updates behavioral tests badge
```

#### `coverage-gate`

Final quality gate check.

```yaml
- Verifies all coverage jobs passed
- Confirms thresholds met
- Blocks merge if failing
```

### 2. Behavioral BDD Tests Workflow

**File**: `.github/workflows/behavioral-bdd-tests.yml`

**Enhanced with Coverage**:

- Runs tests with coverage profile
- Displays coverage summary in PR
- Shows package breakdown
- Uploads to Codecov

---

## Coverage Thresholds

### Global Thresholds

Set in workflow environment variables:

```yaml
env:
  COVERAGE_THRESHOLD: 70  # Minimum overall coverage
```

### Package-Level Thresholds

Recommended thresholds by package type:

| Package Type | Threshold | Rationale |
|--------------|-----------|-----------|
| **Core Logic** (`pkg/desiredstate`) | ≥ 80% | Critical business logic |
| **Parsers** (`internal/parser`) | ≥ 80% | Data validation critical |
| **Utilities** (`pkg/wrapper`) | ≥ 70% | Helper functions |
| **CLI** (`internal/cli`) | ≥ 70% | User-facing interface |
| **Commands** (`cmd/yago`) | ≥ 60% | Integration layer |

### Coverage Badge Colors

Automatic color coding:

- 🟢 **Green** (brightgreen): ≥ 80%
- 🟡 **Yellow**: 70-79%
- 🟠 **Orange**: 60-69%
- 🔴 **Red**: < 60%

### Badge configuration

Coverage badge publishing is opt-in. The `coverage-badge` job runs only when the repository variable `COVERAGE_BADGES_ENABLED` is set to `true`.

When enabling it, configure these repository-level Actions secrets:

- `GIST_TOKEN`: a GitHub token allowed to update the target Gist
- `COVERAGE_GIST_ID`: the ID of an existing Gist accessible by that token

If the Gist ID is invalid or the Gist is not accessible, the badge action returns `404 Not Found`. Leave `COVERAGE_BADGES_ENABLED` unset or set it to `false` until both values are valid.

---

## Coverage Reports

### Report Formats

#### 1. Text Summary (`coverage-summary.txt`)

```
github.com/danieleborsaro/yago/pkg/desiredstate/service.go:82: ValidateDesiredState 75.0%
github.com/danieleborsaro/yago/pkg/desiredstate/service.go:211: AssembleDesiredState 80.5%
...
total:         (statements)  72.3%
```

**Usage**:

- Quick terminal reference
- CI/CD log analysis
- Automated parsing

#### 2. HTML Report (`coverage.html`)

Interactive visual coverage report with:

- Color-coded source files (green/yellow/red)
- Line-by-line coverage
- Package navigation
- Function drill-down

**Access**:

- Download from workflow artifacts
- View locally in browser
- 90-day retention

#### 3. XML Report (`coverage.xml`)

Cobertura-format XML for tool integration:

- SonarQube
- Jenkins
- GitLab CI
- Other CI/CD tools

#### 4. JSON Report (`coverage.json`)

Structured data for custom analysis:

```json
{
  "Packages": [
    {
      "Name": "github.com/danieleborsaro/yago/pkg/desiredstate",
      "Functions": [
        {
          "Name": "ValidateDesiredState",
          "File": "service.go",
          "Coverage": 75.0
        }
      ]
    }
  ]
}
```

### Package Coverage Markdown

Generated in `package-coverage.md`:

```markdown
## Package Coverage Breakdown

| Package | Coverage | Status |
|---------|----------|--------|
| pkg/desiredstate | 75.2% | ⚠️ |
| internal/parser | 82.1% | ✅ |
| internal/cli | 68.5% | ❌ |
```

---

## CI/CD Integration

### GitHub Actions Integration

All coverage workflows integrate with GitHub Actions features:

#### 1. Artifacts

Coverage artifacts retained for 90 days:

```yaml
- name: Archive coverage artifacts
  uses: actions/upload-artifact@v4
  with:
    name: coverage-reports
    path: |
      coverage.out
      coverage.html
      coverage.xml
      coverage.json
      coverage-summary.txt
      package-coverage.md
    retention-days: 90
```

**Access**:

1. Go to Actions tab
2. Select workflow run
3. Download "coverage-reports" artifact

#### 2. Job Summaries

Coverage displayed in GitHub step summary:

```markdown
# 📊 Coverage Report

## Overall Coverage: **72.3%**

✅ **Coverage threshold met** (≥ 70%)

## Package Coverage Breakdown

| Package | Coverage | Status |
|---------|----------|--------|
| pkg/desiredstate | 75.2% | ⚠️ |
```

#### 3. Status Checks

Coverage gates create GitHub status checks:

- ✅ `coverage-gate` - Must pass to merge
- ✅ `full-coverage` - Shows coverage percentage
- ✅ `behavioral-coverage` - Behavioral test coverage

### External Service Integration

#### Codecov

Automatic upload to [codecov.io](https://codecov.io):

```yaml
- name: Upload coverage to Codecov
  uses: codecov/codecov-action@v4
  with:
    files: ./coverage.out,./coverage.xml
    flags: unittests,behavioral-bdd
    token: ${{ secrets.CODECOV_TOKEN }}
```

**Features**:

- Coverage diff in PRs
- Sunburst visualizations
- Historical trends
- File browser

**Setup**:

1. Sign up at codecov.io
2. Add repository
3. Set `CODECOV_TOKEN` secret in GitHub

#### Coveralls

Upload to [coveralls.io](https://coveralls.io):

```yaml
- name: Upload coverage to Coveralls
  uses: coverallsapp/github-action@v2
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    path-to-lcov: ./coverage.out
```

**Features**:

- PR comments with coverage delta
- Badge generation
- Trend charts

---

## Coverage Badges

### Badge Generation

Badges automatically generated on `main` branch:

#### Coverage Badge

![Coverage](https://img.shields.io/badge/coverage-72.3%25-yellow)

**URL**: `https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/USERNAME/GIST_ID/raw/yago-coverage.json`

#### Behavioral Tests Badge

![Behavioral Tests](https://img.shields.io/badge/behavioral%20tests-68%20contracts-brightgreen)

**URL**: `https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/USERNAME/GIST_ID/raw/yago-behavioral-tests.json`

### Setup Instructions

Badge publishing is disabled unless the repository variable `COVERAGE_BADGES_ENABLED` is set to `true`.

1. **Create a public GitHub Gist**:
  - Go to <https://gist.github.com> and create a new **public** gist.
  - Add these two files, each containing valid JSON:

    ```json
    {"schemaVersion": 1, "label": "coverage", "message": "0%", "color": "red"}
    ```

    Use the filenames `yago-coverage.json` and `yago-behavioral-tests.json`.
  - Copy the Gist ID from its URL. For `https://gist.github.com/USERNAME/a1b2c3d4e5f`, the ID is `a1b2c3d4e5f`.

2. **Create a token for Gist updates**:
  - Open <https://github.com/settings/tokens>.
  - Select **Generate new token (classic)**.
  - Give it a descriptive name such as `yago-coverage-badges` and choose an expiration date.
  - Select only the `gist` scope.
  - Generate the token and copy it immediately. GitHub will not show it again.

  A fine-grained token is not used here because the workflow needs the classic token's `gist` scope. Never commit this token or place it in a workflow file.

3. **Configure the repository**:
  - Open the repository's **Settings → Secrets and variables → Actions**.
  - Under **Variables**, create:

    ```text
    COVERAGE_BADGES_ENABLED=true
    ```

  - Under **Secrets**, create:

    ```text
    GIST_TOKEN=<the classic token value>
    COVERAGE_GIST_ID=<the Gist ID, not the full URL>
    ```

4. **Add badge links to README.md**:

   ```markdown
   [![Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/USERNAME/GIST_ID/raw/yago-coverage.json)](https://github.com/danieleborsaro/yago/actions/workflows/coverage.yml)
   [![Behavioral Tests](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/USERNAME/GIST_ID/raw/yago-behavioral-tests.json)](https://github.com/danieleborsaro/yago/actions/workflows/behavioral-bdd-tests.yml)
   ```

---

## Trend Tracking

### Coverage History

Coverage trends tracked in `.coverage-history/coverage.csv`:

```csv
Date,Commit,Coverage
2025-10-01,abc1234,68.5
2025-10-08,def5678,70.2
2025-10-14,ghi9012,72.3
```

**Updated**:

- Daily via cron schedule
- On pushes to `main`/`develop`
- Committed back to repository

### Trend Visualization

GitHub step summary shows last 10 entries:

```markdown
## 📈 Coverage Trend

| Date | Commit | Coverage |
|------|--------|----------|
| 2025-10-01 | abc1234 | 68.5% |
| 2025-10-08 | def5678 | 70.2% |
| 2025-10-14 | ghi9012 | 72.3% |
```

### Analysis

Track coverage improvement:

```bash
# View coverage history
cat .coverage-history/coverage.csv

# Calculate average improvement
awk -F, 'NR>1 {sum+=$3; count++} END {print "Average:", sum/count"%"}' \
  .coverage-history/coverage.csv
```

---

## Local Coverage

### Generate Coverage Locally

#### Full Coverage

```bash
# Run all tests with coverage
go test -v -coverprofile=coverage.out -covermode=atomic ./...

# View summary
go tool cover -func=coverage.out

# View total
go tool cover -func=coverage.out | grep total:

# Generate HTML
go tool cover -html=coverage.out -o coverage.html
open coverage.html  # macOS
xdg-open coverage.html  # Linux
```

#### Package-Specific Coverage

```bash
# Single package
go test -v -coverprofile=coverage.out ./pkg/desiredstate

# Multiple packages
go test -v -coverprofile=coverage.out \
  ./pkg/desiredstate \
  ./internal/parser \
  ./internal/core
```

#### Behavioral BDD Coverage

```bash
# Only behavioral tests
go test -v -coverprofile=behavioral.out -covermode=atomic \
  ./pkg/desiredstate \
  ./internal/cli \
  ./cmd/yago \
  -run "BehavioralBDD"

go tool cover -func=behavioral.out | grep total:
```

### Coverage Tools

Install additional coverage tools:

```bash
# gocov - JSON coverage
go install github.com/axw/gocov/gocov@latest

# gocov-html - HTML from gocov
go install github.com/matm/gocov-html@latest

# gocov-xml - XML from gocov
go install github.com/AlekSi/gocov-xml@latest

# Usage
gocov test ./... | gocov-html > coverage.html
gocov test ./... | gocov-xml > coverage.xml
```

---

## Quality Gates

### Automated Checks

Coverage workflows enforce quality gates:

#### 1. Threshold Enforcement

```yaml
- name: Fail if coverage below threshold
  if: steps.coverage.outputs.status == 'fail'
  run: |
    echo "❌ Coverage ${{ steps.coverage.outputs.total }}% is below threshold $COVERAGE_THRESHOLD%"
    exit 1
```

**Behavior**:

- ❌ Fails if coverage < 70%
- ✅ Passes if coverage ≥ 70%
- Blocks PR merge if failing

#### 2. Race Condition Check

```yaml
- name: Check for race conditions
  run: |
    if go test -race ./... ; then
      echo "✅ No race conditions"
    else
      echo "❌ Race conditions detected"
      exit 1
    fi
```

#### 3. Package Quality Gate

Each package checked individually:

- ❌ Fails if critical package < 80%
- ⚠️ Warns if utility package < 70%
- ✅ Passes if all thresholds met

### Override Quality Gates

For exceptional cases:

```bash
# Skip coverage check (not recommended)
git commit -m "feat: new feature [skip coverage]"

# Or update threshold temporarily
# Edit .github/workflows/coverage.yml
COVERAGE_THRESHOLD: 65  # Temporarily lower
```

---

## Troubleshooting

### Common Issues

#### Coverage Not Uploading

**Symptom**: Coverage reports not appearing in Codecov/Coveralls

**Solutions**:

1. Check `CODECOV_TOKEN` secret is set
2. Verify token has correct permissions
3. Check workflow logs for upload errors
4. Ensure `coverage.out` file exists

```bash
# Debug locally
ls -la coverage.out
go tool cover -func=coverage.out
```

#### Badge Not Updating

**Symptom**: Coverage badge shows old percentage

**Solutions**:

1. Verify `GIST_TOKEN` has `gist` scope
2. Check `COVERAGE_GIST_ID` is correct
3. Ensure workflow runs on `main` branch
4. Clear browser cache (badges cached)

```bash
# Force badge update
curl -X PURGE https://img.shields.io/endpoint?url=...
```

#### Coverage Below Threshold

**Symptom**: CI fails due to low coverage

**Solutions**:

1. Add tests for uncovered code
2. Use `go tool cover -html` to find gaps
3. Focus on critical packages first
4. Consider updating threshold if justified

```bash
# Find least covered files
go tool cover -func=coverage.out | sort -t% -k2 -n | head -20
```

#### Workflow Timeout

**Symptom**: Coverage workflow exceeds time limit

**Solutions**:

1. Reduce test verbosity: remove `-v` flag
2. Run package matrix in parallel
3. Increase timeout in workflow:

```yaml
timeout-minutes: 30  # Default is 10
```

### Getting Help

- **CI/CD Issues**: Check `.github/workflows/coverage.yml` logs
- **Coverage Questions**: Review `go tool cover -help`
- **Badge Problems**: See [shields.io documentation](https://shields.io)
- **Trend Analysis**: Examine `.coverage-history/coverage.csv`

---

## Best Practices

### 1. Write Tests First

✅ **Good**: Write tests as you develop features

```bash
# TDD workflow
go test -v ./pkg/desiredstate -run TestNewFeature
# Add implementation
go test -v -coverprofile=coverage.out ./pkg/desiredstate
```

❌ **Avoid**: Adding tests after feature complete to hit coverage

### 2. Focus on Critical Code

✅ **Good**: Prioritize testing critical business logic

- Data validation
- Error handling
- State management

❌ **Avoid**: Testing trivial getters/setters just for coverage

### 3. Monitor Trends

✅ **Good**: Track coverage over time

```bash
# Check trend
cat .coverage-history/coverage.csv
```

❌ **Avoid**: Ignoring declining coverage

### 4. Use Coverage to Find Gaps

✅ **Good**: Use HTML report to identify untested code paths

```bash
go tool cover -html=coverage.out
```

❌ **Avoid**: Treating coverage as just a number

### 5. Balance Coverage and Quality

✅ **Good**: 70% well-tested critical code
❌ **Avoid**: 95% superficial tests

---

## Integration with Phase 5

Coverage tracking supports other Phase 5 priorities:

- **Priority 1 (API Docs)**: Better documented API → easier to test
- **Priority 2 (Load Tests)**: Performance tests contribute to coverage
- **Priority 3 (Test Helpers)**: Helpers make reaching coverage easier
- **Priority 5 (Performance)**: Coverage ensures optimizations don't break code
- **Priority 6 (Completion)**: Coverage metrics demonstrate quality

---

## Metrics

### Current Coverage Status

| Metric | Value | Status |
|--------|-------|--------|
| **Workflow Files** | 2 | ✅ |
| **Coverage Jobs** | 6 | ✅ |
| **Report Formats** | 5 | ✅ |
| **External Integrations** | 2 (Codecov, Coveralls) | ✅ |
| **Badge Generation** | Automated | ✅ |
| **Trend Tracking** | Daily | ✅ |
| **Quality Gates** | Enforced | ✅ |

### Workflow Statistics

- **Full Coverage Runtime**: ~2-3 minutes
- **Behavioral Coverage Runtime**: ~1-2 minutes
- **Package Matrix Runtime**: ~3-4 minutes (parallel)
- **Total Coverage Overhead**: ~5-7 minutes per run

---

## Next Steps

After coverage integration:

1. **Monitor Coverage**: Watch trends for first few weeks
2. **Adjust Thresholds**: Fine-tune based on actual coverage
3. **Add Package Targets**: Set package-specific goals
4. **Improve Low Areas**: Focus on <70% packages
5. **Document Exceptions**: Explain why some code isn't covered

---

## Related Documentation

- [Behavioral BDD Catalog](testing/BEHAVIORAL_BDD_CATALOG.md) - Test catalog
- [Test Helpers](../pkg/desiredstate/testhelpers/README.md) - Testing utilities
- [CI/CD Workflows](../.github/workflows/) - All workflows
- [Go Coverage Tool](https://go.dev/blog/cover) - Official Go coverage docs
