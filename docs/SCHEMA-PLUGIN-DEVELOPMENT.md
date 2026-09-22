# Schema Plugin Development Guide

## Overview

Starting with the mandatory property path implementation, all schema plugins **MUST** specify property paths using the `x-gitops-paths` extension in their JSON schema files. The system will fail if these paths are not provided - there are no fallback defaults.

## Required Extension: x-gitops-paths

Every schema plugin must include an `x-gitops-paths` extension at the root level of the JSON schema with the following structure:

```json
{
  "x-gitops-paths": {
    "desiredstate": {
      "root": "path.to.desiredstate.root",
      "meta": "path.to.desiredstate.meta",
      "metaRootPath": "path.to.desiredstate.meta.parts.self",
      "metaEcosystem": "path.to.desiredstate.meta.ecosystem",
      "contentEcosystem": "path.to.desiredstate.runtime.ecosystem",
      "repoToLoad": "path.to.repository.config",
      "partsToLoad": "relative.path.to.parts",
      "masterPipelineIsSelfUpdating": "path.to.pipeline.self.updating.flag",
      "slavePipelinesRepoList": "path.to.slave.pipelines.list"
    },
    "configuration": {
      "root": "path.to.configuration.root",
      "meta": "path.to.configuration.meta",
      "metaRootPath": "path.to.configuration.meta.parts.self",
      "metaEcosystem": "path.to.configuration.meta.ecosystem",
      "contentEcosystem": "path.to.configuration.runtime.ecosystem",
      "repoToLoad": "path.to.repository.config",
      "partsToLoad": "relative.path.to.parts",
      "masterPipelineIsSelfUpdating": "path.to.pipeline.self.updating.flag",
      "slavePipelinesRepoList": "path.to.slave.pipelines.list"
    }
  }
}
```

## Required Property Paths

Each document type (desiredstate/configuration) section must define ALL of the following paths:

### Core Document Structure

- **`root`**: Path to the document root section (e.g., "desiredstate", "configuration")
- **`meta`**: Path to the metadata section (e.g., "desiredstate.meta")
- **`metaRootPath`**: Path to the self-reference file (e.g., "desiredstate.meta.parts.self")

### Ecosystem Management

- **`metaEcosystem`**: Path to ecosystem definition in metadata (e.g., "desiredstate.meta.ecosystem")
- **`contentEcosystem`**: Path where runtime ecosystem content is stored (e.g., "desiredstate.ecosystem")

### Repository and Parts

- **`repoToLoad`**: Path to repository configuration (e.g., "desiredstate.meta.repo")
- **`partsToLoad`**: Relative path to parts definition (e.g., "meta.parts")

### Pipeline Management

- **`masterPipelineIsSelfUpdating`**: Path to self-updating flag (e.g., "desiredstate.meta.master_pipeline.master.is_self_updating")
- **`slavePipelinesRepoList`**: Path to slave pipelines list (e.g., "desiredstate.meta.master_pipeline.slaves")

## Schema Plugin Structure

1. **File Location**: Place schema files in the `schemas/` directory of your project
2. **Naming Convention**: Use descriptive names like `gitops-schema-v4.0.0.json`
3. **JSON Schema Compliance**: Use JSON Schema Draft 7 or compatible
4. **Extension Placement**: The `x-gitops-paths` extension must be at the root level of the schema

## Example Schema Plugin

See `examples/example-schema-plugin.json` for a complete working example.

## Error Handling

If your schema plugin is missing the `x-gitops-paths` extension or any required paths, the system will fail with clear error messages:

- `"schema plugin for version X does not specify required 'x-gitops-paths' extension"`
- `"schema plugin for version X does not define required 'x-gitops-paths.desiredstate' section"`
- `"schema plugin for version X is missing required property paths: [list of missing paths]"`

## Migration from Hardcoded Paths

If you're migrating from the old hardcoded approach:

1. **Identify Current Paths**: Look at how your documents are structured
2. **Map to Extension**: Create the `x-gitops-paths` extension with your actual paths
3. **Test Thoroughly**: Ensure all document processing still works correctly
4. **No Fallback**: Remember that there are no default paths - all must be specified

## Custom Document Structures

The property path system allows for complete flexibility in document structure. You can:

- Use different root section names (not just "desiredstate"/"configuration")
- Organize metadata differently
- Place ecosystem content at custom locations
- Define custom pipeline management structures

Just ensure your `x-gitops-paths` extension accurately reflects your document structure.

## Validation

The system validates that:

1. The `x-gitops-paths` extension exists
2. Both `desiredstate` and `configuration` sections are defined
3. All required property paths are present and are strings
4. The paths can be resolved in actual documents

## Best Practices

1. **Consistent Naming**: Use consistent path naming across schema versions
2. **Clear Documentation**: Document what each path represents in your schema
3. **Version Management**: Update paths appropriately when changing schema versions
4. **Testing**: Test with real documents to ensure paths resolve correctly
