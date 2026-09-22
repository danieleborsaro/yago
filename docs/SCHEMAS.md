# YAGO Schema System

YAGO uses a flexible JSON schema system that supports plugins, allowing you to customize and extend GitOps schema definitions without modifying YAGO's source code.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Provider Support](#provider-support)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [Creating Custom Schemas](#creating-custom-schemas)
- [Advanced Usage](#advanced-usage)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

### What Does YAGO's Schema System Provide?

- **Embedded Core Schemas**: Built-in schemas compiled into the binary
- **Plugin Architecture**: Add custom schemas without modifying source code
- **Flexible Configuration**: Control schema loading through `schema-config.json`
- **Multiple Schema Paths**: Load schemas from default and custom locations
- **Version Management**: Support multiple schema versions with automatic resolution
- **Manifest-based Discovery**: Simple configuration through manifest files

### Schema Types

1. **Core Schemas** (Embedded)
   - Built into YAGO binary at compile time
   - Always available, no external files needed
   - Located in `assets/schemas/gitops/` (source code)
   - Versions: 1.0.0, 2.0.0

2. **Plugin Schemas** (Runtime)
   - Loaded at runtime from filesystem
   - Customizable per project/organization
   - Support for multiple custom locations
   - Can extend or replace core schemas

## Quick Start

### 1. Default Setup

By default, YAGO works with embedded core schemas. No configuration needed!

```bash
# Validate a document (uses embedded schemas)
yago ds validate -d my-desiredstate.yaml
```

### 2. Enable Plugins

Create a `schema-config.json` in your project root:

```json
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": [],
  "enablePlugins": true,
  "cacheSchemas": true
}
```

Create a plugin schema:

```bash
mkdir -p schemas/gitops
```

Add your custom schemas (see [Creating Custom Schemas](#creating-custom-schemas)).

### 3. Multi-Location Setup

For organization-wide schemas:

```json
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": [
    "/opt/company/yago-schemas",
    "~/.yago/schemas",
    "./project-schemas"
  ],
  "enablePlugins": true,
  "cacheSchemas": true
}
```

## Namespace Support

YAGO includes a namespace system for organizing and namespacing schemas. The namespace field enables extensibility while maintaining backward compatibility with existing documents.

### Current Implementation

**Flexible Namespaces**: YAGO supports any namespace identifier. All manifests must specify a namespace:

```json
{
  "manifest": {
    "namespace": "yago"
  }
}
```

The namespace can be any alphanumeric string with hyphens, underscores, dots, and colons, such as:

- `"yago"` (core YAGO schemas)
- `"my-org"` (organization-specific schemas)
- `"aws_provider"` (provider-specific schemas)
- `"io.k8s.api"` (Kubernetes-style namespaces)
- `"org:team:project"` (hierarchical namespaces)

**Schema Indexing**: Internally, schemas are indexed using a composite key format:

```
<namespace>-<version>-<kind>
```

Examples:

- `yago-4.2.0-DesiredState`
- `my-org-1.0.0-Configuration`
- `aws_provider-2.1.0-DesiredState`

This indexing system ensures schemas can be uniquely identified and enables support for multiple schema namespaces from different sources.

### Namespace Field in Documents

Documents can optionally include a `provider` field at the root level:

**Example with explicit provider:**

```yaml
provider: yago        # Explicitly specify the provider
schema: 4.2.0
kind: DesiredState
meta:
  metaEcosystem: production
  # ... rest of document
```

**Example without provider (backward compatible):**

```yaml
# No provider field - will be auto-added
schema: 4.2.0
kind: DesiredState
meta:
  metaEcosystem: production
  # ... rest of document
```

### Backward Compatibility

YAGO ensures seamless backward compatibility for documents that don't specify a provider:

1. **Automatic Provider Injection**: When loading a document without a `provider` field, YAGO automatically adds `provider: yago`
2. **Warning Message**: A warning is logged to inform you that the default provider was added:

   ```
   [WARN] Added default provider 'yago' to document (was not specified)
   ```

3. **No Breaking Changes**: All existing documents continue to work without modification

**Recommendation**: While not required, it's recommended to add `provider: yago` to your documents explicitly to:

- Make the provider dependency explicit
- Avoid warning messages
- Prepare for potential future multi-provider support

### Future Extensibility

The provider system is designed to support future ecosystem extensions:

**Potential Use Cases:**

- Cloud provider-specific schemas (AWS, Azure, GCP)
- Organization-specific schema namespaces
- Third-party plugin ecosystems
- Custom tooling integrations

**Example Future Scenario:**

```yaml
# AWS provider extension (future)
provider: aws-yago
schema: 1.0.0
kind: DesiredState
# ... AWS-specific fields
```

**Note**: Multi-provider support is not currently implemented. This documentation describes the current single-provider implementation and the architectural foundation for future extensibility.

## Architecture

### Directory Structure

```
yago-project/
├── schema-config.json              # Configuration file (optional)
├── assets/                         # Build-time schemas (in source)
│   └── schemas/
│       └── core/                   # Embedded core schemas
│           ├── manifest.json
│           └── v2.0.0-configuration.json
│           ├── v1.0.0-desiredstate.json
│           └── v2.0.0-configuration.json
│           ├── v2.0.0-desiredstate.json
├── schemas/                        # Runtime plugin schemas
│   └── plugins/
│       ├── custom-manifest.json
│       └── v1.0.0-custom-configuration.json
│       └── v1.0.0-custom-desiredstate.json
└── project-schemas/                # Custom location (example)
    └── plugins/
        ├── project-manifest.json
        └── v2.0.0-project-configuration.json
        └── v2.0.0-project-desiredstate.json
```

### How Schema Loading Works

1. **Build Time**: Core schemas are embedded in YAGO binary
2. **Runtime**:
   - Load embedded core schemas (always available)
   - If `enablePlugins: true`:
     - Load from default path: `{defaultSchemaPath}/`
     - Load from each custom path: `{customPath}/`
   - Merge all schemas into unified store
   - Later paths can override earlier ones

### Path Resolution

- **Absolute paths**: `/opt/yago/custom-schemas` → used as-is
- **Relative paths**: `./project-schemas` → resolved from project root
- **Home directory**: `~/.yago/schemas` → expanded to `/home/user/.yago/schemas`

## Configuration

### Configuration File Location

YAGO searches for `schema-config.json` in the following order (highest priority first):

1. **Command-line flag**: `--schema-config /path/to/config.json`
2. **Environment variable**: `YAGO_SCHEMA_CONFIG=/path/to/config.json`
3. **User home directory**: `~/.yago/schema-config.json`
4. **Project root**: `./schema-config.json` (default)

This allows you to:

- Override configuration per-command using `--schema-config`
- Set a global default using `YAGO_SCHEMA_CONFIG` environment variable
- Use user-specific configuration in `~/.yago/schema-config.json`
- Keep project-specific configuration in the project root

### Specifying Configuration Location

**Using Command-Line Flag:**

```bash
yago ds validate -d my-desiredstate.yaml --schema-config=/path/to/schema-config.json
```

**Using Environment Variable:**

```bash
export YAGO_SCHEMA_CONFIG=/opt/yago/schema-config.json
yago ds validate -d my-desiredstate.yaml
```

**Using User Home Directory:**

```bash
# Create user-specific config
mkdir -p ~/.yago
cat > ~/.yago/schema-config.json << 'EOF'
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": ["~/.yago/custom-schemas"],
  "enablePlugins": true,
  "cacheSchemas": true
}
EOF

# Now all yago commands will use this config unless overridden
yago ds validate -d my-desiredstate.yaml
```

**Using Project Root (Default):**

```bash
# Create project-specific config
cat > schema-config.json << 'EOF'
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": [],
  "enablePlugins": true,
  "cacheSchemas": true
}
EOF

yago ds validate -d my-desiredstate.yaml
```

### Configuration File Format

Place `schema-config.json` in one of the supported locations:

```json
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": [
    "/opt/yago/custom-schemas",
    "~/.yago/schemas",
    "./project-schemas"
  ],
  "enablePlugins": true,
  "cacheSchemas": true
}
```

### Configuration Options

#### `defaultSchemaPath`

- **Type:** `string`
- **Default:** `"schemas"`
- **Description:** The default path where YAGO looks for plugin schemas. Can be absolute or relative to the project root.

#### `customSchemaPaths`

- **Type:** `array of strings`
- **Default:** `[]`
- **Description:** Additional paths to search for custom schemas. Supports:
  - Absolute paths: `/opt/yago/custom-schemas`
  - Home directory expansion: `~/.yago/schemas`
  - Relative paths: `./project-schemas`

#### `enablePlugins`

- **Type:** `boolean`
- **Default:** `true`
- **Description:** Whether to load plugin schemas at runtime. Set to `false` to only use embedded core schemas.

#### `cacheSchemas`

- **Type:** `boolean`
- **Default:** `true`
- **Description:** Whether to cache loaded schemas for better performance.

### Configuration Examples

**Example 1: Default (Project-Only Schemas)**

```json
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": [],
  "enablePlugins": true,
  "cacheSchemas": true
}
```

Loads plugins only from `schemas/gitops/`

**Example 2: Organization Schemas**

```json
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": [
    "/opt/company/yago-schemas"
  ],
  "enablePlugins": true,
  "cacheSchemas": true
}
```

Loads plugins from:

1. `schemas/gitops/` (project-specific)
2. `/opt/company/yago-schemas/gitops/` (organization-wide)

**Example 3: Multi-Environment Setup**

```json
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": [
    "~/.yago/schemas",
    "/opt/yago/global-schemas",
    "./env-specific-schemas"
  ],
  "enablePlugins": true,
  "cacheSchemas": true
}
```

Loads plugins from:

1. `schemas/gitops/` (default)
2. `~/.yago/schemas/gitops/` (user-specific)
3. `/opt/yago/global-schemas/gitops/` (global)
4. `./env-specific-schemas/gitops/` (environment-specific)

**Example 4: Core Schemas Only**

```json
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": [],
  "enablePlugins": false,
  "cacheSchemas": true
}
```

Uses only embedded core schemas, no plugins loaded

## Creating Custom Schemas

### Step 1: Create Schema Directory

Create a directory for your custom schemas:

```bash
mkdir -p schemas/gitops
cd schemas/gitops
```

### Step 2: Write JSON Schema

Create your schema file (e.g., `v1.0.0-custom-desiredstate.json`):

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Custom Organization DesiredState v1.0.0",
  "description": "Custom DesiredState schema with organizational fields",
  "type": "object",
  "required": ["schemaVersion", "kind", "metadata", "spec"],
  "properties": {
    "schemaVersion": {
      "type": "string",
      "const": "1.0.0-custom",
      "description": "API version of the schema"
    },
    "kind": {
      "type": "string",
      "const": "DesiredState",
      "description": "Type of document"
    },
    "metadata": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": {
          "type": "string",
          "description": "Name of the desired state"
        },
        "costCenter": {
          "type": "string",
          "description": "Cost center for billing"
        },
        "businessUnit": {
          "type": "string",
          "description": "Business unit owning this resource"
        }
      }
    },
    "spec": {
      "type": "object",
      "description": "Specification of the desired state"
    }
  }
}
```

### Step 3: Create Manifest

Create a manifest file (e.g., `custom-org-manifest.json`):

```json
{
  "manifest": {
    "name": "Custom Organization Schemas",
    "description": "Organization-specific schema extensions",
    "version": "1.0.0",
    "namespace": "my-org"
  },
  "schemas": [
    {
      "version": "1.0.0-custom",
      "kind": "DesiredState",
      "file": "v1.0.0-custom-desiredstate.json",
      "description": "Custom DesiredState schema with organization fields"
    }
  ]
}
```

#### Manifest Structure

- `manifest.name`: Human-readable name for the schema collection
- `manifest.description`: Description of what these schemas provide
- `manifest.version`: Version of the manifest itself
- `manifest.namespace`: Schema namespace identifier
  - Can be any alphanumeric string with hyphens, underscores, dots, and colons
  - Examples: `"yago"` (core schemas), `"my-org"`, `"custom-schemas"`, `"aws_provider"`, `"io.k8s.api"`
  - Schemas are indexed as `<namespace>-<version>-<kind>` internally
  - Documents can specify a `namespace` field at root level to use schemas from specific namespaces
- `schemas[]`: Array of available schemas
  - `version`: Schema version (used for lookup)
  - `kind`: Document kind this schema validates (DesiredState, Configuration, etc.)
  - `file`: Filename of the JSON schema
  - `description`: Human-readable description

### Step 4: Test Your Schema

```bash
# List all available schemas
yago ds printschema -l

# Should show your custom schema version

# Use your custom schema in a document
cat > my-document.yaml <<EOF
schema: 1.0.0-custom
kind: DesiredState
metadata:
  name: my-app
  costCenter: CC-12345
  businessUnit: Engineering
spec:
  environment: production
EOF

# Validate
yago ds validate -d my-document.yaml
```

### Schema File Requirements

Each plugin directory must contain:

1. **At least one manifest file**: `*-manifest.json` or `manifest.json`
2. **Schema files**: Referenced in the manifest
3. **Valid JSON**: All files must be valid JSON

#### Multiple Manifests Per Directory

YAGO supports multiple manifest files in a single directory. Any file ending with `manifest.json` or `-manifest.json` will be discovered and loaded.

**Namespace Support:**

YAGO fully supports custom namespaces for organizing schemas. The namespace field in manifests serves to:

- Enable multi-namespace schema ecosystems
- Facilitate schema organization and discovery from different sources
- Support backward compatibility (defaults to `"yago"` if not specified in documents)

**Current Behavior:**

- Each manifest must specify a `"namespace"` field (e.g., `"yago"`, `"my-org"`, `"custom-schemas"`)
- Schemas are indexed internally using `namespace-version-kind` composite keys
- Multiple manifests in a directory are loaded sequentially
- Schemas from all manifests are merged into a single store
- If multiple manifests define the same `namespace + version + kind`, the last loaded wins (with a warning)
- Use multiple manifests to organize schemas logically, but ensure unique `namespace + version + kind` combinations

**Document Namespace Field:**

Documents can optionally specify a `namespace` field at the root level:

```yaml
namespace: yago        # Optional - defaults to "yago" if not specified
schema: 4.2.0
kind: DesiredState
meta:
  # ... rest of document
```

**Backward Compatibility:**

- If a document does not specify a `namespace` field, YAGO automatically defaults to `namespace: yago`
- A warning message is logged when the default namespace is added
- This ensures all existing documents continue to work without modification

## Advanced Usage

### Multiple Schema Kinds

Support different document types by adding schemas for different `kind` values:

```json
{
  "manifest": {
    "name": "Multi-Kind Schemas",
    "namespace": "my-org"
  },
  "schemas": [
    {
      "version": "1.0.0",
      "kind": "DesiredState",
      "file": "v1.0.0-desiredstate.json",
      "description": "DesiredState schema"
    },
    {
      "version": "1.0.0",
      "kind": "Configuration",
      "file": "v1.0.0-configuration.json",
      "description": "Configuration schema"
    }
  ]
}
```

### Version Resolution

YAGO supports flexible version resolution:

- **Exact versions**: `1.0.0`, `2.1.0`, `4.0.0`
- **Custom versions**: `1.0.0-custom`, `2.1.0-enterprise`, `2.0.0-project`
- **Version aliases**: Future support for `1.x` → latest 1.x version

### Dynamic Property Paths

**New in YAGO**: Schemas can define custom property paths using the `x-gitops-paths` extension. This allows plugins to extend YAGO with organization-specific fields without code changes.

#### What are Property Paths?

Property paths define dot-notation paths to important fields in your GitOps documents. YAGO uses these to navigate and validate document structure.

#### Required Core Paths

Every schema MUST define these 9 core paths:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "x-gitops-paths": {
    "desiredstate": {
      "root": "desiredstate",
      "meta": "desiredstate.meta",
      "metaRootPath": "desiredstate.meta.parts.self",
      "metaEcosystem": "desiredstate.meta.ecosystem",
      "contentEcosystem": "desiredstate.ecosystem",
      "repoToLoad": "desiredstate.meta.repo",
      "partsToLoad": "meta.parts",
      "masterPipelineIsSelfUpdating": "desiredstate.meta.master_pipeline.master.is_self_updating",
      "slavePipelinesRepoList": "desiredstate.meta.master_pipeline.slaves"
    },
    "configuration": {
      "root": "configuration",
      "meta": "configuration.meta",
      "metaRootPath": "configuration.meta.parts.self",
      "metaEcosystem": "configuration.meta.ecosystem",
      "contentEcosystem": "configuration.ecosystem",
      "repoToLoad": "configuration.meta.repo",
      "partsToLoad": "meta.parts",
      "masterPipelineIsSelfUpdating": "configuration.meta.master_pipeline.master.is_self_updating",
      "slavePipelinesRepoList": "configuration.meta.master_pipeline.slaves"
    }
  }
}
```

#### Adding Custom Paths

Plugin schemas can add **custom property paths** beyond the required ones:

```json
{
  "x-gitops-paths": {
    "desiredstate": {
      "_comment": "Required core paths (9 total - see above)",
      "root": "desiredstate",
      "meta": "desiredstate.meta",
      "metaRootPath": "desiredstate.meta.parts.self",
      "metaEcosystem": "desiredstate.meta.ecosystem",
      "contentEcosystem": "desiredstate.ecosystem",
      "repoToLoad": "desiredstate.meta.repo",
      "partsToLoad": "meta.parts",
      "masterPipelineIsSelfUpdating": "desiredstate.meta.master_pipeline.master.is_self_updating",
      "slavePipelinesRepoList": "desiredstate.meta.master_pipeline.slaves",
      
      "_comment_custom": "Custom paths for your organization",
      "orgCostCenter": "desiredstate.meta.organization.cost_center",
      "orgOwner": "desiredstate.meta.organization.owner",
      "complianceTags": "desiredstate.meta.compliance.tags",
      "projectMetrics": "desiredstate.meta.metrics.project",
      "securityLevel": "desiredstate.meta.security.level"
    }
  }
}
```

#### Using Custom Paths in Code

If you're extending YAGO with custom Go code, you can access custom paths:

#### Reference Example: `test-custom-paths` Fixture

The repository ships a working, runnable example of this extension mechanism at
`schemas/test-custom-paths-manifest.json` (+ its two referenced schema files,
`schemas/test-custom-paths-v1.0.0-desiredstate-meta.json` and
`schemas/test-custom-paths-v1.0.0-configuration-meta.json`), registered under the
dedicated namespace `test-custom-paths`.

This is a **test fixture**, not a schema meant for production use. It exists to prove,
via `internal/schema/property_paths_behavioral_bdd_test.go` and
`pkg/wrapper/base_service_phase3_test.go`, that:

- Arbitrary custom keys added to `x-gitops-paths` (e.g. `customOrgCostCenter`,
  `customOrgOwner`, `customComplianceTags`, `customProjectMetrics`) are discovered
  and queryable through the same `Has()`/`Get()`/`GetAll()` API as the required
  core paths — no special-casing between core and custom paths.
- A schema can override the `configRepoLocatorPrimary`/`configRepoLocatorLegacy`
  templates to point somewhere other than the default
  `desiredstate.content.environments.{environment}.configurations.{wrapper}`
  (the fixture uses `desiredstate.meta.locators.primary.{environment}.{wrapper}`
  instead), and that YAGO's config-repo resolution actually honors the
  schema-declared template instead of silently falling back to its own default.

The embedded `yago` schemas (1.0.0/2.0.0) and the `legacy` schema (4.2.0) do **not**
include `customOrg*`/`customCompliance*`/etc. paths, and that's intentional: those
schemas are the generic, tenant-agnostic baseline shipped with YAGO. Organization-specific
fields belong in your own plugin schema (see [Adding Custom Paths](#adding-custom-paths)
above) — `test-custom-paths` is simply a stand-in example of what such a plugin schema
looks like, using illustrative field names rather than a real organization's fields.

If you're extending YAGO with custom Go code, you can access custom paths:

```go
// Get property paths from schema (specify namespace and version)
paths, err := schemaManager.DiscoverPropertyPaths("my-org", "1.0.0-custom", true)
if err != nil {
    log.Fatal(err)
}

// Access required core paths (always available)
rootPath := paths.Root()  // "desiredstate"
metaPath := paths.Meta()  // "desiredstate.meta"

// Access custom paths (check if they exist)
if paths.Has("orgCostCenter") {
    costCenterPath, _ := paths.Get("orgCostCenter")
    // Use costCenterPath to access data
```

    fmt.Println("Cost center path:", costCenterPath)
}

// Get all paths (core + custom)
allPaths := paths.GetAll()
fmt.Printf("Total paths defined: %d\n", len(allPaths))

```

#### Benefits

1. **No Code Changes**: Add custom fields without modifying YAGO source
2. **Schema-Driven**: Paths are defined where the schema is defined
3. **Runtime Discovery**: YAGO automatically loads all paths from schemas
4. **Validation**: YAGO validates that required core paths exist
5. **Extensibility**: Custom paths enable organization-specific tooling

#### Example: Compliance Tracking

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://my-org.com/schemas/v1.0.0-compliance",
  "x-gitops-paths": {
    "desiredstate": {
      "root": "desiredstate",
      "meta": "desiredstate.meta",
      "metaRootPath": "desiredstate.meta.parts.self",
      "metaEcosystem": "desiredstate.meta.ecosystem",
      "contentEcosystem": "desiredstate.ecosystem",
      "repoToLoad": "desiredstate.meta.repo",
      "partsToLoad": "meta.parts",
      "masterPipelineIsSelfUpdating": "desiredstate.meta.master_pipeline.master.is_self_updating",
      "slavePipelinesRepoList": "desiredstate.meta.master_pipeline.slaves",
      
      "complianceLevel": "desiredstate.meta.compliance.level",
      "complianceAuditor": "desiredstate.meta.compliance.auditor",
      "complianceLastReview": "desiredstate.meta.compliance.last_review_date"
    }
  },
  "properties": {
    "desiredstate": {
      "type": "object",
      "properties": {
        "meta": {
          "type": "object",
          "properties": {
            "compliance": {
              "type": "object",
              "properties": {
                "level": {
                  "type": "string",
                  "enum": ["basic", "enhanced", "strict"],
                  "description": "Compliance level required for this resource"
                },
                "auditor": {
                  "type": "string",
                  "description": "Email of designated compliance auditor"
                },
                "last_review_date": {
                  "type": "string",
                  "format": "date",
                  "description": "Date of last compliance review"
                }
              },
              "required": ["level", "auditor"]
            }
          }
        }
      }
    }
  }
}
```

Your GitOps document:

```yaml
schema: my-org.com/v1.0.0-compliance
desiredstate:
  meta:
    parts:
      self: infrastructure/vpc/desiredstate.yaml
    ecosystem: production
    compliance:
      level: strict
      auditor: security@my-org.com
      last_review_date: 2025-10-01
  content: {}
```

### Schema Inheritance

Build upon existing schemas by referencing them with `allOf`:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "allOf": [
    {
      "$ref": "v4.2.0-desiredstate.json"
    },
    {
      "properties": {
        "metadata": {
          "properties": {
            "compliance": {
              "type": "object",
              "properties": {
                "level": {
                  "enum": ["basic", "enhanced", "strict"]
                },
                "auditor": {
                  "type": "string"
                }
              },
              "required": ["level"]
            }
          }
        }
      }
    }
  ]
}
```

### Organizational Distribution

Distribute schemas across your organization:

**1. Central Git Repository**

```bash
# Add organizational schemas as submodule
git submodule add https://github.com/my-org/yago-schemas.git /opt/company/yago-schemas

# Update schemas
git submodule update --remote
```

**2. Shared Network Storage**

```json
{
  "customSchemaPaths": [
    "/mnt/shared/yago-schemas"
  ]
}
```

**3. User Home Directory**

```bash
# Clone to user directory
git clone https://github.com/my-org/yago-schemas.git ~/.yago/schemas
```

Then reference in config:

```json
{
  "customSchemaPaths": [
    "~/.yago/schemas"
  ]
}
```

## Best Practices

### Schema Design

1. **Start Simple**: Begin with minimal required fields, add complexity gradually
2. **Use Semantic Versions**: Follow semver for schema versions
3. **Provide Descriptions**: Add clear `description` fields for all properties
4. **Validate Early**: Test schemas with real documents during development
5. **Document Changes**: Maintain a CHANGELOG for schema updates
6. **Avoid Breaking Changes**: Don't remove required fields in minor versions

### Version Management

1. **Distinct Versions**: Use distinct version numbers for custom schemas
   - Good: `1.0.0-custom`, `2.0.0-project`, `1.0.0-enterprise`
   - Avoid: `1.0.0`, `4.2.0` (conflicts with core versions)

2. **Version Naming**: Include suffix to indicate custom schemas
   - `{version}-{org}`: `1.0.0-acme`, `2.1.0-corp`
   - `{version}-{purpose}`: `1.0.0-dev`, `2.0.0-prod`

3. **Migration Paths**: Provide documentation for schema migrations

### Path Organization

1. **Project-Specific Schemas**: Use default path `schemas/gitops/`
2. **Organization Schemas**: Use custom absolute path `/opt/company/yago-schemas`
3. **User Schemas**: Use home directory `~/.yago/schemas`
4. **Environment-Specific**: Use relative path `./env-schemas`

### Version Control

1. **Commit Configuration**: Always commit `schema-config.json` to repository
2. **Don't Commit External Schemas**: Schemas from custom paths are external dependencies
3. **Document Custom Paths**: Document where custom schemas come from in README
4. **Test Locally**: Ensure schemas work before committing

### Testing

1. **Test with Core Only**: Verify with `enablePlugins: false`
2. **Test Each Custom Path**: Verify each custom path loads correctly
3. **Test Version Resolution**: Ensure correct schema versions are used
4. **Validate Sample Documents**: Keep sample documents for testing

## Troubleshooting

### Configuration File Not Found

**Symptoms**: YAGO warns "Failed to load schema config" or uses default configuration

**Solutions**:

1. Check the configuration file precedence (from lowest to highest priority):

   ```bash
   # Lowest priority - user home directory (personal defaults)
   cat ~/.yago/schema-config.json
   
   # Second priority - project root (project-specific config)
   cat ./schema-config.json
   
   # Third priority - environment variable (context-specific override)
   export YAGO_SCHEMA_CONFIG=/path/to/config.json
   yago ds validate -d document.yaml
   
   # Highest priority - command-line flag (ultimate override)
   yago ds validate -d document.yaml --schema-config=/path/to/config.json
   ```

2. Verify file exists and is readable:

   ```bash
   # Check if file exists
   ls -la ~/.yago/schema-config.json
   ls -la ./schema-config.json
   
   # Verify JSON is valid
   python -m json.tool schema-config.json
   ```

3. Enable verbose logging to see which config is loaded:

   ```bash
   yago ds validate -d document.yaml --verbose
   # Look for: "Loaded schema configuration from: <path>"
   ```

4. Test with explicit path:

   ```bash
   yago ds validate -d document.yaml --schema-config=./schema-config.json --verbose
   ```

### Plugins Not Loading

**Symptoms**: Custom schemas not found, validation uses core schemas only

**Solutions**:

1. Check that `enablePlugins` is `true`
2. Verify paths exist and are readable:

   ```bash
   ls -la schemas/gitops/
   ls -la /opt/company/yago-schemas/gitops/
   ```

3. Check logs for specific error messages:

   ```bash
   yago ds validate -d document.yaml --verbose
   ```

4. Verify manifest file format:

   ```bash
   python -m json.tool schemas/gitops/custom-manifest.json
   ```

### Schema Not Found

**Symptoms**: "Schema not found" error for custom version

**Solutions**:

1. List available schemas:

   ```bash
   yago ds printschema -l
   ```

2. Ensure schema files are in `plugins/` subdirectory
3. Verify manifest.json lists the schema correctly
4. Check file permissions:

   ```bash
   ls -la schemas/gitops/
   ```

5. Verify schema version matches document `schemaVersion`

### Path Resolution Issues

**Symptoms**: Custom paths not loading, home directory not expanded

**Solutions**:

1. Use absolute paths if relative paths aren't working
2. Verify `~/` expansion:

   ```bash
   echo ~
   ```

3. Check that project root is correctly detected
4. Try absolute path first to isolate issue:

   ```json
   {
     "customSchemaPaths": [
       "/home/username/.yago/schemas"
     ]
   }
   ```

### Invalid JSON Schema

**Symptoms**: Schema validation fails, parsing errors

**Solutions**:

1. Validate JSON syntax:

   ```bash
   python -m json.tool schemas/gitops/my-schema.json
   jq . schemas/gitops/my-schema.json
   ```

2. Check JSON Schema syntax against draft-07 spec
3. Ensure `$schema` property is present
4. Verify all `$ref` references are valid
5. Test schema structure:

   ```bash
   yago ds validate -d document.yaml --verbose
   ```

### Debug Mode

Enable detailed logging for schema operations:

```bash
# Enable debug logging
yago ds validate -d document.yaml --verbose --log-level debug
```

Look for these log messages:

- `Loaded schema configuration from: ...`
- `Loading embedded core schemas from build-time resources`
- `Successfully loaded embedded schema: version=X.X.X`
- `Loading plugin schemas from X custom path(s)`
- `Checking custom plugin path: ...`
- `Successfully loaded schema: version=X.X.X-custom`

### Common Error Messages

**"Schema directory not found"**

- The `plugins/` subdirectory doesn't exist in the specified path
- Create it: `mkdir -p schemas/gitops`

**"Failed to parse manifest"**

- Manifest JSON is invalid
- Validate: `python -m json.tool manifest.json`

**"Schema version X.X.X not found"**

- Schema isn't registered or version mismatch
- List available: `yago ds printschema -l`

**"Failed to load plugin schemas"**

- Usually non-fatal - plugins are optional
- Check if path exists and has correct permissions

## Reference

### Supported Schema Versions (Core)

| Version | Kind | Description |
|---------|------|-------------|
| 1.0.0 | DesiredState | Initial schema version |
| 2.1.0 | DesiredState | Enhanced metadata support |
| 3.11.0 | DesiredState | Extended specification |
| 4.0.0 | DesiredState | Major revision |
| 4.2.0 | DesiredState | Current stable version |
| 4.2.0 | Configuration | Configuration document schema |

### File Locations

| Item | Location | Purpose |
|------|----------|---------|
| Core GitOps schemas (source) | `assets/schemas/gitops/` | Embedded in binary |
| Core GitOps schemas (embedded) | Built into binary | Runtime GitOps schemas |
| Manifest schemas (source) | `assets/schemas/manifests/` | Embedded manifest validators |
| Manifest schemas (embedded) | Built into binary | Validates manifest.json structure |
| Default plugins | `schemas/gitops/` | Project-specific plugins |
| Configuration | `schema-config.json` | Schema loading config |
| Custom schemas | Configured in `customSchemaPaths` | Organization/user schemas |

### Manifest File Format

```json
{
  "manifest": {
    "name": "string",           // Required: Display name
    "description": "string",    // Optional: Description
    "version": "string",        // Required: Manifest version (must be "1.0.0")
    "namespace": "string"       // Required: Namespace ID (e.g., "yago", "my-org", "custom-schemas")
  },
  "schemas": [
    {
      "version": "string",      // Required: Schema version
      "kind": "string",         // Required: Document kind
      "file": "string",         // Required: Schema filename
      "description": "string"   // Optional: Description
    }
  ]
}
```

### Manifest Validation

YAGO validates schema manifests at load time to ensure compatibility and correctness.

#### Version Validation

Currently, only manifest version `1.0.0` is supported. YAGO will reject manifests with any other version:

```json
{
  "manifest": {
    "version": "2.0.0"  // ❌ ERROR: unsupported manifest version
  }
}
```

**Error message:**

```
unsupported manifest version '2.0.0': only version 1.0.0 is currently supported. 
Please update your manifest file to use version 1.0.0 or upgrade YAGO to support 
newer manifest formats
```

This validation serves as a placeholder for future extensibility when YAGO may support multiple manifest format versions with different structures or capabilities.

#### Content Validation (v1.0.0)

For manifests declaring `version: "1.0.0"`, YAGO validates:

**Required Fields:**

- `manifest.name`: Display name for the schema collection
- `manifest.namespace`: Schema namespace identifier
  - Can be any alphanumeric string with hyphens, underscores, dots, and colons
  - Examples: `"yago"`, `"my-org"`, `"custom-schemas"`, `"aws_provider"`, `"io.k8s.api"`
  - Schemas are indexed internally using namespace-version-kind composite keys
  - Documents can specify a namespace field at root level to use schemas from specific namespaces
- `manifest.version`: Must be "1.0.0"

**Schema Entry Requirements:**
Each entry in the `schemas` array must have:

- `version`: Schema version (e.g., "4.2.0")
- `kind`: Document kind - must be "DesiredState" or "Configuration"
- `file`: Path to the JSON schema file (relative to manifest)

**Example of Valid Manifest:**

```json
{
  "manifest": {
    "name": "Custom GitOps Schemas",
    "description": "Organization-specific schema extensions",
    "version": "1.0.0",
    "namespace": "my-org"
  },
  "schemas": [
    {
      "version": "4.2.0",
      "kind": "DesiredState",
      "file": "custom-desired-state-4.2.0.json",
      "description": "Extended desired state schema"
    }
  ]
}
```

**Common Validation Errors:**

| Error | Cause | Fix |
|-------|-------|-----|
| "manifest.namespace is required" | Missing `namespace` field | Add `"namespace": "my-org"` |
| "unsupported manifest version" | Wrong version number | Set `"version": "1.0.0"` |
| "schema entry missing 'kind'" | Schema definition incomplete | Add `"kind": "DesiredState"` |
| "invalid kind 'Custom'" | Unsupported document kind | Use "DesiredState" or "Configuration" |
| "schema entry missing 'file'" | No file path specified | Add `"file": "schema.json"` |

#### Validation Behavior

- **Embedded Schemas**: Core schemas are validated at build time
- **External Schemas**: Plugin schemas are validated when loaded
- **Failure Action**: Invalid manifests prevent schema loading and YAGO will log detailed error messages
- **Empty Schemas Array**: Allowed but generates a warning (manifest has no schemas to load)

### Manifest Schema Validation

YAGO validates manifest.json files themselves using JSON Schema to ensure structural correctness beyond the basic field validation described above.

#### How It Works

1. **JSON Schema Definition**: Manifest structure is defined in `assets/schemas/manifests/v1.0.0.json`
2. **Embedded Validation**: The manifest schema is embedded at build time
3. **Automatic Validation**: All manifest files are validated against their declared version's schema
4. **Two-Layer Validation**:
   - **JSON Schema Validation**: Strict structural validation (types, patterns, enums, additional properties)
   - **Fallback Validation**: Basic field presence checks (backward compatibility)

#### What Gets Validated

The JSON Schema enforces:

- **Required Fields**: `manifest.name`, `manifest.version`, `manifest.provider`, `schemas` array
- **Version Constraint**: `manifest.version` must be exactly `"1.0.0"`
- **Provider Constraint**: Must be exactly `"yago"`
  - Schemas are indexed internally using provider-version-kind composite keys
  - Future versions may support additional providers for ecosystem extensions
- **Schema Entry Structure**: Each schema must have `version`, `kind`, `file`
- **Kind Enumeration**: Only `"DesiredState"` or `"Configuration"` allowed
- **Version Format**: Must match semantic versioning pattern `^[0-9]+\.[0-9]+\.[0-9]+$`
- **File Extension**: Schema files must end with `.json`
- **No Additional Properties**: Extra fields in manifest or schema entries are rejected

#### Benefits

- **Early Error Detection**: Catch typos, wrong types, and structural errors before runtime
- **Clear Error Messages**: JSON Schema provides detailed validation errors with field paths
- **IDE Support**: JSON Schema enables autocomplete and validation in modern editors
- **Consistency**: Ensures all manifests follow the same structure
- **Future-Proof**: Easy to add new manifest versions with different schemas

#### Example Validation Errors

```
manifest validation failed: 
  - manifest.namespace: manifest.namespace is required
  - manifest.version: Does not match: "1.0.0"
  - schemas.0.kind: schemas.0.kind must be one of the following: "DesiredState", "Configuration"
  - manifest: Additional property extra_field is not allowed
```

## Migration Guide

### From Hardcoded Schemas

If you're migrating from an older version of YAGO with hardcoded schemas:

1. **No changes required**: The new system is fully backward compatible
2. **Existing documents work**: All core schema versions are embedded
3. **Commands unchanged**: CLI interface remains the same
4. **Optional enhancements**: Add custom schemas if needed

### Adding First Custom Schema

1. Create directory structure:

   ```bash
   mkdir -p schemas/gitops
   ```

2. Create schema and manifest files (see [Creating Custom Schemas](#creating-custom-schemas))

3. Create `schema-config.json`:

   ```json
   {
     "defaultSchemaPath": "schemas",
     "enablePlugins": true
   }
   ```

4. Test:

   ```bash
   yago ds printschema -l
   ```

## Contributing

To contribute schemas back to YAGO core:

1. Create schemas in `schemas/gitops/`
2. Test thoroughly with real documents
3. Submit PR with:
   - Schema files
   - Updated manifest
   - Documentation
   - Test cases
4. Follow YAGO's contribution guidelines

For questions or issues with the schema system, please open an issue in the YAGO repository.
