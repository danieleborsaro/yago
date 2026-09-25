# YAGO (Yet Another GitOps)

[![Behavioral BDD Tests](https://github.com/danieleborsaro/yago/actions/workflows/behavioral-bdd-tests.yml/badge.svg)](https://github.com/danieleborsaro/yago/actions/workflows/behavioral-bdd-tests.yml)
[![Coverage](https://github.com/danieleborsaro/yago/actions/workflows/coverage.yml/badge.svg)](https://github.com/danieleborsaro/yago/actions/workflows/coverage.yml)
[![PR Quality Check](https://github.com/danieleborsaro/yago/actions/workflows/pr-check.yml/badge.svg)](https://github.com/danieleborsaro/yago/actions/workflows/pr-check.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/danieleborsaro/yago)](https://goreportcard.com/report/github.com/danieleborsaro/yago)
[![codecov](https://codecov.io/gh/danieleborsaro/yago/branch/main/graph/badge.svg)](https://codecov.io/gh/danieleborsaro/yago)

> **Note**: Once coverage badges are configured with a GitHub Gist, the static workflow badge above can be replaced with dynamic coverage percentage badges. See [Coverage Tracking Documentation](docs/COVERAGE_TRACKING.md#coverage-badges) for setup instructions.

A Go implementation providing tools for handling desired state configuration in the context of a delivery pipeline in the context of a GitOps framework.

## Features

- **YAML Processing**: Load, merge, and validate YAML files with custom lookup functions
- **Schema Validation**: Support for GitOps schema versions 1.0.0 and 2.0.0
- **CLI Interface**: Command-line interface
- **Desiredstate Management**: Validate, assemble, promote, and compare desired state configurations
- **Custom Lookups**: Environment variable and YAML value lookups with `[[...]]` syntax
- **Modular Design**: Clean separation of concerns with utility packages
- **Quality Assurance**: Comprehensive linting with golangci-lint, yamllint, and pre-commit hooks
- **CI/CD Integration**: GitHub Actions workflows for automated testing and quality checks

## Installation

### From Binary Releases

Download the latest binary for your platform from the [releases page](https://github.com/danieleborsaro/yago/releases).

### From Source

```bash
git clone git@github.com-personal:danieleborsaro/yago.git
cd yago
make deps
make build
```

## Releases and GoReleaser

Release artifacts are built and published by GoReleaser from version tags. A tag such as `v0.1.0` becomes the application version and is injected into the binary together with the commit and build date.

The release workflow runs automatically when a tag matching `v*` is pushed:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Each release contains archives for Linux, macOS, and Windows on amd64 and arm64, a SHA-256 checksum file, and a Cosign signature bundle for that checksum file.

### Local GoReleaser commands

GoReleaser is the source of truth for release builds. Make remains available as a local convenience wrapper:

```bash
make release-check     # Validate .goreleaser.yaml
make release-build     # Build release binaries without publishing
make release-snapshot  # Build local archives and checksums without publishing
```

Local snapshot signing requires `cosign` to be installed. GitHub Actions uses keyless Cosign signing through GitHub's OIDC identity and does not require a private signing key.

### Verify a release

Download the checksum file and its `.sigstore.json` bundle from the GitHub release, then verify the signature:

```bash
cosign verify-blob \
  --certificate-identity "https://github.com/danieleborsaro/yago/.github/workflows/release.yml@refs/tags/v0.1.0" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --bundle yago_checksums.txt.sigstore.json \
  yago_checksums.txt
```

After the signature verifies, validate the downloaded archives:

```bash
sha256sum --check --ignore-missing yago_checksums.txt
```

<!-- ### Using Docker

```bash
docker pull ghcr.io/danieleborsaro/yago:latest
docker run --rm yago:latest --help
``` -->

## Usage

### CLI Commands

```bash
# Show help
yago --help

# List supported schema versions
yago desiredstate printschema --list

# Validate a YAML file
yago desiredstate validate myfile.yaml

# Validate with specific schema version
yago desiredstate validate --schema-version 2.0.0 myfile.yaml

# Assemble multiple YAML files
yago desiredstate assemble file1.yaml file2.yaml --output merged.yaml

# Promote configuration between environments
yago desiredstate promote --source-file dev.yaml --destination-file prod.yaml

# Compare two configurations
yago desiredstate compare dev.yaml prod.yaml

# Check component versions across environments
yago desiredstate checkversions myfile.yaml

# Lock / unlock a desired state
yago desiredstate lock myfile.yaml
yago desiredstate unlock myfile.yaml

# Show status of a desired state
yago desiredstate status myfile.yaml

# AWS Secrets Manager
yago sm plan -r eu-west-1 -d my-app/secrets/create/desiredstate.yaml
yago sm create -r eu-west-1 -d my-app/secrets/create/desiredstate.yaml
yago sm validate -r eu-west-1 -d my-app/secrets/create/desiredstate.yaml

# Enable verbose logging
yago --verbose desiredstate validate myfile.yaml
```

### Secrets Manager

`yago sm` (alias `secretsmanager`) creates and checks the secrets described by the `secrets` block of a desired
state's Terraform configuration, keyed by AWS region. Terraform root modules read the same block. It only describes the
secrets; their values are never in it:

```yaml
secrets:
  eu-west-1:
    database:
      is_created_here: true       # this configuration creates the secret; false means it must already exist
      name: my-app/database       # the secret's name in Secrets Manager
      description: Database credentials
      encryption: kms             # optional: kms (the default), or sse for the AWS managed key
      kms_key_id: alias/my-app    # optional, with kms: an existing key (ID, ARN or alias) instead of a new one
      versions:                   # version IDs (or stages) in use
        version_a: 6324c1f6-e0eb-4066-af51-29059f772d48
      keys:                       # the fields of the secret's JSON value; none means plain text
        username_path: username
        certificate_path: certificate_b64
      permissions:                # optional: only these principals can read or change the secret
        restrict_to_roles: [my-app, my-deployer]
```

To add a secret: add it with `is_created_here: true`, run `plan` then `create`, set its real value in Secrets Manager,
record the new version ID under `versions`, then run `validate`.

`permissions` gives a secret created here a resource policy that denies reading, changing and deleting the secret, and
changing its policy, to every principal not in `restrict_to_users`, `restrict_to_roles`, `restrict_to_assumed_roles`
or `restrict_to_sso_policies` (permission set names). `extra_policy_statements` are added to the policy as they are.
List whoever runs yago and reads the secret, such as the Terraform role: anyone else loses access, and only a listed
principal can change the policy. `restrict_to_groups` is refused, as IAM groups aren't principals and a policy can't
match their members. `create` also applies the policy to existing secrets it created, but doesn't remove a policy when
`permissions` is removed.

Every command takes the desired state (`-d`) and the AWS region (`-r`), and reads the desired state's `terraform`
configuration: `-c`, or cloned as per the desired state. `-p` sets the AWS profile (default `$AWS_PROFILE`) and `-e`
the environment (default `all`).

| Command | What it does |
|---|---|
| `assemble -C <dir>` | Writes the assembled desired state and configuration to `<dir>`. Doesn't contact AWS. |
| `plan` | Shows the secrets `create` would create, and the resource policies it would apply. |
| `create` | Creates each of the region's `is_created_here` secrets that doesn't exist yet: first a KMS key of its own (`alias/<secret name>` with dots replaced by dashes, rotation on), unless `kms_key_id` names one or `encryption` is `sse`, then the secret, with the value `placeholder` in every field (base64-encoded for fields ending in `_b64`), and a resource policy if `permissions` is set. The alias of a key it creates must be one KMS accepts, so those secrets' names can't use `+`, `=` or `@`, and two of them can't share an alias. Existing secrets are left as they are, apart from their resource policy. Secrets not created here must already exist. `-n`, or `IS_DRY_RUN=1`, only shows what it would do. |
| `validate` | Reads each secret at the versions in `versions`, and fails if its fields don't match `keys`. |
| `destroy` | Asks, then immediately deletes the region's `is_created_here` secrets, with their replicas, KMS key and alias. It only deletes secrets `create` made, which have its `isCreatedHere` tag, and keeps a KMS key that isn't the secret's own. `-f` doesn't ask; `--dry-run` shows what it would do. |

`create` tags the secrets and their KMS keys as the awstagging Terraform provider would, from `project_properties`,
which merges `project_properties_global`, `_eco`, `_proj` and `_env` (each overriding the ones before). It needs
`company_name_short` (the tag prefix), `accounts_coding` (with the account's ID), `owner`, `cost_centre`,
`compliance`, `infra_environment`, `project_name_long` and `resource_set_long`, and also uses `app_environment`,
`app_ecosystem`, `role`, `description`, `custom_tags` and `custom_tags_verbatim`. As no Terraform is involved, there
are no TerraformModule or TerraformWorkspace tags.

### Examples

#### Basic Validation

```bash
yago ds validate configs/app.yaml
```

#### Assembling Multiple Files

```bash
yago ds assemble \
  configs/base.yaml \
  configs/environment.yaml \
  --output final-config.yaml
```

#### Promotion with Dry Run

```bash
yago ds promote \
  --source-file staging/app.yaml \
  --destination-file production/app.yaml \
  --dry-run
```

## YAML Lookup Functions

The tool supports custom lookup functions in YAML files:

```yaml
schema: 2.0.0
namespace: yago
kind: DesiredState
desiredstate:
  meta:
    repo:
      git:
        url: "[[gitops.getEnvValue(REPO_URL, git@github.com:org/repo.git)]]"
        branch: "[[gitops.getEnvValue(BRANCH, main)]]"
        tag: ""
        watch: []
```

### Available Lookups

- `gitops.getEnvValue(VAR_NAME, default)`: Get environment variable with fallback
- `gitops.getYamlValue(path.to.value)`: Get value from YAML document

## Terraform Secret Inputs

A Terraform configuration part can set Terraform variables from existing AWS Secrets Manager secrets with
`secret_variables`. The configuration holds references only; yago reads the values when Terraform runs.

```yaml
secret_variables:
  db_password:                # Terraform variable name
    secret_id: prod/app/db    # secret name or ARN (required)
    json_key: password        # optional: take one field of a JSON secret
  api_token:
    secret_id: prod/app/api-token
    version_stage: AWSPREVIOUS  # optional: version_stage or version_id, not both
```

- `yago tf assemble` removes `secret_variables` from `.gitops/configuration.tfvars.json` and writes the references
  to `.gitops/terraform-secrets.json`. Secret values are never written to `.gitops`.
- `plan`, `apply`, `destroy` and `import` read the secrets with `--aws-profile` and `--aws-region`, and pass each one as
  `TF_VAR_<name>`. Dry runs don't read them. Secret values are removed from the Terraform output yago shows.
- A saved plan keeps the references it was made with in `<plan file>.secrets.json`, and applying that plan uses them.
  If a saved plan has no references file but the configuration has secret inputs, `apply` stops. Plan again with yago.
- A JSON field that isn't a string is passed as compact JSON, for Terraform variables of list, map or object type.
- Variable names may use letters, digits and underscores, and can't start with a digit. A variable can't be set both
  in the configuration and in `secret_variables`.

Terraform stores input variable values in saved plan files, and in state when resources use them. Declare these
variables `sensitive = true`, or `ephemeral = true` if nothing needs to keep the value.

## Configuration

### Logging

Set log level with `--log-level` or `-v/--verbose`:

```bash
yago --log-level debug ds validate myfile.yaml
yago -v ds validate myfile.yaml  # Same as debug level
```

### Schema Validation

Specify schema version explicitly or let the tool auto-detect:

```bash
# Auto-detect (default)
yago ds validate myfile.yaml

# Explicit version
yago ds validate --schema-version 2.0.0 myfile.yaml
```

## Development

YAGO includes comprehensive development tools and quality assurance:

### Quick Setup

```bash
# Install dependencies and development tools
make deps
make pre-commit-install

# Run all linters
make lint

# Run tests
make test
```

### Building

Make is still used for local development and CI checks. GoReleaser is the source of truth for release builds and cross-platform release artifacts.

```bash
# Build for the current platform
make build

# Validate the GoReleaser configuration
make release-check

# Build release-style binaries without publishing
make release-build

# Build release archives and checksums without publishing
make release-snapshot
```

Do not use `make build-cross` for releases. It is retained for compatibility with the existing CI build job; release artifacts must be produced by GoReleaser so they receive the Git tag metadata, checksums, and Cosign signature bundle.

### Testing

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage
```

### Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Available Commands

```bash
make lint          # Run all linters (Go + YAML)
make lint-go       # Run Go-specific linters
make lint-yaml     # Run YAML linter
make lint-fix      # Auto-fix common issues
make security-scan # Run security vulnerability scan
make pre-commit-run # Run pre-commit hooks
```

### Dependencies

The pre-commit hooks shell out to the following tools, which must be installed and on `PATH`:

- [pre-commit](https://pre-commit.com/) - hook runner (`pip install pre-commit`)
- [golangci-lint](https://golangci-lint.run/) - Go meta-linter
- [goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) - Go import formatting (`go install golang.org/x/tools/cmd/goimports@latest`)
- [hadolint](https://github.com/hadolint/hadolint) - Dockerfile linting (runs via Docker)
- [yamlfmt](https://github.com/google/yamlfmt) - a tool for linting and formatting YAML files
- [detect-secrets](https://github.com/Yelp/detect-secrets) - secret scanning against `.secrets.baseline` (`pip install detect-secrets`)
- [gitleaks](https://github.com/gitleaks/gitleaks) - secret scanning
- Node.js - required by the `markdownlint-cli` hook (managed automatically by pre-commit's `nodeenv`)

### GitHub Actions release configuration

The release workflow requires no custom repository secret or environment variable. It uses the automatically provided `GITHUB_TOKEN` to publish the GitHub Release and GitHub OIDC to create a keyless Cosign signature.

The workflow declares these permissions itself:

- `contents: write` - publish release artifacts
- `id-token: write` - obtain the OIDC identity used by Cosign

If repository or organization policy restricts GitHub Actions, allow Actions to run and allow the workflow to use write permissions. Do not add `COSIGN_PRIVATE_KEY`, a Cosign password, or a `COSIGN_EXPERIMENTAL` variable for this setup.

For detailed development setup, see [DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Documentation

- **[Schema System](docs/SCHEMAS.md)** - Complete guide to YAGO's schema system, including:
  - Core embedded schemas and plugin architecture
  - Creating custom schemas for your organization
  - Configuring schema paths with `schema-config.json`
  - Best practices and troubleshooting
  
- **PropertyWrappers Architecture**:
  - **[Quick Reference](docs/PROPERTY-WRAPPERS-QUICK-REF.md)** - TL;DR cheat sheet for `assembledMeta` vs `contentForConsumption`
  - **[Design Document](docs/PROPERTY-WRAPPERS-DESIGN.md)** - Detailed architecture guide and rationale
  - **[Data Flow Diagrams](docs/PROPERTY-WRAPPERS-FLOW.md)** - Visual flow of how documents are loaded and assembled
  
- **[Development Guide](docs/DEVELOPMENT.md)** - Development setup, tools, and workflows

## Architecture

```
yago/
├── cmd/yago/               # Main CLI entry point
├── internal/
│   ├── cli/                # CLI framework setup
│   ├── core/               # Core business logic
│   ├── parser/             # YAML parsing and lookups
│   ├── property/           # Property access utilities
│   ├── repo/               # Git repository operations
│   ├── schema/             # Schema validation and discovery
│   └── utils/              # Utility packages
│       ├── command/        # Command execution
│       ├── errors/         # Error handling
│       └── logging/        # Custom logging
├── pkg/
│   ├── desiredstate/       # Desiredstate commands and service
│   ├── wrapper/            # Wrapper orchestration
│   ├── terraform/          # Terraform integration
│   ├── concourse/          # Concourse CI integration
│   ├── secretsmanager/     # AWS Secrets Manager integration
│   └── aws/                # AWS clients (ECR, S3, Secrets Manager)
├── assets/
│   └── schemas/            # Embedded JSON schemas (1.0.0, 2.0.0)
├── tests/                  # Integration test assets
├── docs/                   # Documentation
├── .github/workflows/      # CI/CD pipelines
│   ├── behavioral-bdd-tests.yml  # Comprehensive test suite
│   └── pr-check.yml        # Quick PR validation
├── .golangci.yml          # Go linting configuration
├── .yamllint.yaml         # YAML linting configuration
├── .pre-commit-config.yaml # Pre-commit hooks
├── Makefile              # Build automation
└── go.mod                # Go module definition
```

## Testing

### Behavioral BDD Tests

This project uses comprehensive behavioral BDD (Behavior-Driven Development) tests following a Cucumber-like contract pattern.

**Test Coverage:**

- **68 behavioral contracts** across all application layers (including 4 load/stress tests)
- **160+ sub-tests** with 100% pass rate
- **6,500+ lines** of behavioral documentation
- **10 performance benchmarks** tracking memory and execution time
- **Test helpers package** with 24 utility functions reducing boilerplate by 62%

**Test Layers:**

- Parser Layer (42 contracts): Core parsing, error handling, configuration filtering
- Service Layer (7 contracts): Business logic, operations, command integration
- CLI Layer (5 contracts): Command parsing, flag handling, environment selection
- Integration Layer (7 contracts): Actual binary execution, version/help commands, performance
- Workflow Layer (5 contracts): End-to-end workflows, error propagation, complete pipelines
- Multi-Environment & Error Recovery (4 contracts): Environment inheritance, promotion, error recovery
- **Load & Stress Testing (4 contracts)**: Large files (1-10MB), scalability (100-1000 components), concurrency (10-100 operations), memory pressure

**📚 Complete Testing Documentation Hub:** [docs/testing/README.md](docs/testing/README.md)

**Quick Start:**

```bash
# All behavioral contracts
go test -v ./... -run "BehavioralBDD"

# Specific layer
go test -v ./pkg/desiredstate -run "TestParser.*BehavioralBDD"   # Parser
go test -v ./pkg/desiredstate -run "TestService.*BehavioralBDD"  # Service
go test -v ./internal/cli -run "BehavioralBDD"                   # CLI
go test -v ./cmd/yago -run "TestCLI.*BehavioralBDD"              # Integration
go test -v ./cmd/yago -run "TestWorkflow.*BehavioralBDD"         # Workflows
go test -v ./cmd/yago -run "Test(MultiEnv|ErrorRecovery).*BehavioralBDD"  # Multi-env
go test -v ./pkg/desiredstate -run "TestLoadStress.*BehavioralBDD"  # Load/Stress

# With coverage
go test -v -coverprofile=coverage.out ./... -run "BehavioralBDD"
go tool cover -html=coverage.out

# Performance benchmarks
go test -bench=. -benchmem ./pkg/desiredstate
```

**Documentation:**

- **[Testing Hub](docs/testing/README.md)** - Complete testing documentation index
- **[Quick Reference](docs/testing/QUICK_REFERENCE.md)** - Common commands and troubleshooting
- **[Tutorial](docs/testing/TUTORIAL.md)** - Step-by-step guide for writing tests
- **[Test Catalog](docs/testing/BEHAVIORAL_BDD_CATALOG.md)** - Complete catalog of all 68 contracts
- **[Test Helpers](pkg/desiredstate/testhelpers/README.md)** - Reusable test utilities (62% code reduction)
- **[Coverage Tracking](docs/COVERAGE_TRACKING.md)** - CI/CD coverage integration and quality gates

### Code Coverage

yago maintains comprehensive code coverage with automated tracking and quality gates:

**Coverage Goals:**

- **Overall Coverage**: ≥ 70% (enforced via CI/CD)
- **Critical Packages**: ≥ 80% (desiredstate, parser)
- **Daily Trend Tracking**: Historical coverage data in `.coverage-history/`
- **Multiple Strategies**: Full, behavioral BDD, package-specific, trend analysis

**View Coverage Locally:**

```bash
# Generate full coverage report
go test -v -coverprofile=coverage.out -covermode=atomic ./...

# View summary
go tool cover -func=coverage.out | grep total:

# Interactive HTML report
go tool cover -html=coverage.out -o coverage.html
open coverage.html  # macOS / xdg-open coverage.html on Linux
```

**CI/CD Integration:**

- **Automated Coverage Workflow**: Runs on every push/PR
- **Quality Gate**: PRs blocked if coverage < 70%
- **Codecov Integration**: Coverage visualization and PR comments
- **Coverage Badges**: Dynamic badges showing current coverage percentage
- **Trend Tracking**: Daily coverage history stored in repository

**📊 Complete Coverage Documentation:** [docs/COVERAGE_TRACKING.md](docs/COVERAGE_TRACKING.md)

## License

This project is licensed under the same terms outlined in the [LICNSE](license) file.

## Support

For questions and support, please open an issue in the [GitHub repository](https://github.com/danieleborsaro/yago/issues).
