# GitOps Schema Extensions

This directory contains JSON schemas for GitOps desiredstate and configuration documents.

## Custom Schema Extensions

The schemas use custom JSON properties (prefixed with `x-gitops-`) to extend the standard JSON Schema specification with GitOps-specific metadata.

### x-gitops-paths

Defines path mappings for navigating the document structure. These paths are used by the schema manager to extract specific properties without hardcoding JSON paths in the application code.

**Example:**

```json
{
  "x-gitops-paths": {
    "desiredstate": {
      "root": "desiredstate",
      "meta": "desiredstate.meta",
      "metaRootPath": "desiredstate.meta.parts.self",
      "partsToLoad": "meta.parts"
    }
  }
}
```

**Usage:** The `GetPropertyPaths()` method in SchemaManager reads these paths to navigate documents dynamically.

### x-gitops-wrappers

Defines the list of supported wrapper types for a schema version. Wrappers are configuration groupings (terraform, concourse, docker, etc.) that organize infrastructure-as-code definitions.

**Example:**

```json
{
  "x-gitops-wrappers": [
    "meta",
    "terraform",
    "concourse",
    "packer",
    "docker",
    "mysql",
    "vtex",
    "ory",
    "github",
    "postgresql",
    "msbuild"
  ]
}
```

**Wrapper Evolution:**

- **Embedded schema (v1.0.0)**: Contains all 11 wrappers for backward compatibility
- **External schemas**: Define version-specific wrappers
  - v1.0.0: `["meta"]` (base)
  - v2.x: `["meta", "terraform"]` (adds terraform)
  - v3.x: 9 wrappers (adds concourse, packer, docker, mysql, vtex, ory, github)
  - v4.x: 11 wrappers (adds postgresql, msbuild)

**Usage:**

- The `DiscoverWrappers()` method extracts this list at runtime
- The `ValidateWrapper()` method validates user input against this list
- Case-insensitive validation allows "terraform", "Terraform", or "TERRAFORM"

## Schema Architecture

### Embedded vs External Schemas

**Embedded Schemas** (`assets/schemas/`):

- Compiled into the Go binary
- Used as fallback when external schemas are unavailable
- Contain complete wrapper definitions for backward compatibility
- Namespace: `"yago"`

**External Schemas** (`schemas/gitops/`):

- Loaded from filesystem at runtime
- Allow schema updates without recompilation
- Define incremental wrappers per version
- Namespace: `"custom"` or user-defined

### Schema Discovery

The SchemaManager implements dynamic schema discovery:

1. **Loading**: Checks external schemas first, falls back to embedded
2. **Caching**: Parsed schemas are cached per namespace:version
3. **Wrapper Discovery**: Extracts x-gitops-wrappers at first access
4. **Path Discovery**: Extracts x-gitops-paths for document navigation

## Python Parity

The Go implementation achieves 100% parity with the Python implementation:

**Python (Class-based):**

```python
class DesiredStateV1(DesiredStateInterface):
    SUPPORTED_WRAPPERS_LIST = ["meta", "terraform", ...]
    
    @classmethod
    def getSupportedWrappers(cls):
        return cls.SUPPORTED_WRAPPERS_LIST
```

**Go (Schema-based):**

```go
// Reads from schema's x-gitops-wrappers property
wrappers, err := schemaManager.DiscoverWrappers("yago", "1.0.0")
// Returns: ["meta", "terraform", ...]
```

The key difference: Python uses hardcoded class attributes, Go uses dynamic schema discovery. Both achieve the same runtime behavior.

## Thread Safety

All schema operations are thread-safe:

- Read operations use `sync.RWMutex.RLock()`
- Write operations use `sync.RWMutex.Lock()`
- Caches are protected by mutexes

## Validation

Schemas enforce strict validation:

- All required properties must be present
- Wrapper names must be in x-gitops-wrappers list
- Paths must reference valid JSON paths in the document
- Type validation follows JSON Schema Draft 2020-12 specification

## Adding New Wrappers

To add a new wrapper to a schema version:

1. **Update the schema JSON:**

   ```json
   {
     "x-gitops-wrappers": [
       "meta",
       "terraform",
       "your-new-wrapper"
     ]
   }
   ```

2. **Add wrapper configuration definition:**

   ```json
   {
     "$defs": {
       "configPartYourWrapper": {
         "type": "object",
         "properties": {
           "parts": {"$ref": "#/$defs/configWrapperParts"}
         },
         "required": ["parts"]
       }
     }
   }
   ```

3. **No code changes required** - the Go implementation discovers wrappers dynamically

## References

- JSON Schema Specification: <https://json-schema.org/draft/2020-12/schema>
- Custom Properties: <https://json-schema.org/understanding-json-schema/reference/generic.html#annotations>
