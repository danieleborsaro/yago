# PropertyWrappers Design in YAGO

> **Quick Reference**: For a condensed cheat sheet, see [PropertyWrappers Quick Reference](PROPERTY-WRAPPERS-QUICK-REF.md)

## Table of Contents

- [Overview](#overview)
- [The Two PropertyWrappers](#the-two-propertywrappers)
- [Comparison: Python vs Go](#comparison-python-vs-go)
- [Why This Design?](#why-this-design)
- [Code Flow Examples](#code-flow-examples)
- [Code Documentation](#code-documentation)
- [Deprecated Methods](#deprecated-methods)
- [Key Takeaway](#key-takeaway)

## Overview

The `GitOpsDocument` struct uses **two PropertyWrappers** to separate the original source file from the consumable output. This design matches the original Python implementation.

**Core Principle**: Separate "what was in the file" (`assembledMeta`) from "what wrapper tools consume" (`contentForConsumption`).

## The Two PropertyWrappers

### 1. `assembledMeta` (Python: `meta`)

**Purpose**: Holds the original YAML file as loaded from disk

**Characteristics**:

- **Never modified** after initial load
- Contains the complete root document with namespace, schema, and structure
- For DesiredState: `desiredstate.meta` and `desiredstate.content` (before assembly)
- For Configuration: `configuration.meta` and `configuration.content.wrappers`
- Source of truth for metadata extraction

**Usage**:

- Reference for validating document structure
- Source for extracting metadata paths
- Starting point for assembly operations

### 2. `contentForConsumption` (Python: `content`)

**Purpose**: Holds the consumable output that wrapper tools use

**Characteristics**:

- **Modified during assembly** - contains the final output
- **What gets cached/written to files** via `Cache()` function
- Content varies by document type and context

**For DesiredState**:

- Contains the **full assembled document**:
  - `namespace: yago`
  - `schema: 4.2.0`
  - `desiredstate.content` (assembled from all parts)
  - `desiredstate.meta` (metadata about parts)
- This is what wrapper tools consume - the complete picture

**For Configuration with wrapper** (e.g., `--wrapper terraform`):

- Contains **ONLY the wrapper parts**:
  - Raw terraform variables (tfvars)
  - NO `configuration:` structure
  - NO `namespace`, NO `schema`, NO `meta`
- This is what wrapper tools (terraform) directly consume

**For Configuration without wrapper**:

- Contains the full configuration structure (same as assembledMeta)
- Used for validation and introspection

## Comparison: Python vs Go

| Aspect | Python | Go |
|--------|--------|-----|
| **Original file** | `self.meta` | `assembledMeta` |
| **Consumable output** | `self.content` | `contentForConsumption` |
| **Cache writes** | `self.content.data` | `contentForConsumption.Data` |
| **Assembly target** | Modifies `self.content` | Modifies `contentForConsumption` |

## Why This Design?

### Separation of Concerns

- **assembledMeta**: "What was in the file?" - immutable reference
- **contentForConsumption**: "What should wrapper tools use?" - mutable output

### Flexibility

Different document types need different outputs:

- DesiredState: Full document (meta + content) for comprehensive state
- Configuration: Just wrapper parts for tool consumption

### Clarity

The naming makes intent explicit:

- `assembledMeta` = source document
- `contentForConsumption` = what wrappers consume

## Code Flow Example

### DesiredState Assembly

```go
// 1. Load root file
doc.assembledMeta.LoadFile(pathToRoot)  // namespace, schema, desiredstate.meta

// 2. Start with root file copy
assembled := deepCopy(doc.assembledMeta.Data)

// 3. Load and merge parts at root level
for each part {
    partContent := LoadPart(partFile)
    MergeKeys(assembled, partContent)  // desiredstate.content grows
}

// 4. Set as output
doc.contentForConsumption.Data = assembled

// 5. Cache writes contentForConsumption
Cache() -> writes doc.contentForConsumption.Data  // Full document
```

### Configuration Assembly (with wrapper)

```go
// 1. Load root file
doc.assembledMeta.LoadFile(pathToRoot)  // configuration.meta, wrappers structure

// 2. Load wrapper-specific parts
for each wrapperPart {
    fileList.append(wrapperPart)
}
mergedContent := LoadFiles(fileList)  // Raw terraform vars

// 3. Set ONLY wrapper parts as output
doc.contentForConsumption.Data = mergedContent  // NO structure, NO meta

// 4. Cache writes contentForConsumption
Cache() -> writes doc.contentForConsumption.Data  // Just terraform vars
```

## Code Documentation

### In-Code Documentation Standards

All key functions and struct fields related to PropertyWrappers are documented with:

1. **Struct Field Comments** (`internal/core/document.go`):
   - `assembledMeta`: Documented as "Original YAML file (never modified), Python equivalent: self.meta"
   - `contentForConsumption`: Documented as "Consumable output for wrappers, Python equivalent: self.content"

2. **Function Documentation**:
   - `GetRootFile()`: Full docstring explaining what it returns and when to use it
   - `GetContentForWrapper()`: Detailed explanation of content variations by document type
   - `LoadParts()`: Comprehensive documentation of assembly logic for both document types
   - `Cache()`: Documents what gets written (contentForConsumption.Data) and format variations

3. **Struct-Level Documentation**:
   - `GitOpsDocument` struct includes detailed multi-paragraph comment explaining:
     - The two-PropertyWrapper pattern
     - Purpose of each PropertyWrapper
     - Content variations by document type
     - Python equivalents for reference

### Documentation Files

- **[README.md](../README.md)**: References this design document in the Documentation section
- **[PROPERTY-WRAPPERS-DESIGN.md](PROPERTY-WRAPPERS-DESIGN.md)** (this file):
  - Architectural overview
  - Comparison with Python implementation
  - Code flow examples
  - Best practices

### Quick Reference

| Want to... | Use... | Returns... |
|------------|--------|------------|
| Get original file | `GetRootFile()` | Unmodified source YAML |
| Get wrapper output | `GetContentForWrapper()` | Assembled/processed data for tools |
| Write to cache | `Cache()` | Writes `contentForConsumption.Data` |
| Validate structure | Check `assembledMeta.Data` | Original document structure |

## Deprecated Methods

For backwards compatibility, we maintain deprecated getter methods:

```go
// Deprecated: Use GetRootFile() for clarity
func (doc *GitOpsDocument) GetMeta() *property.PropertyWrapper {
    return doc.assembledMeta
}

// Deprecated: Use GetContentForWrapper() for clarity
func (doc *GitOpsDocument) GetContent() *property.PropertyWrapper {
    return doc.contentForConsumption
}
```

## Key Takeaway

**The current two-PropertyWrapper design (`assembledMeta` + `contentForConsumption`) perfectly matches the original Python implementation (`meta` + `content`) and provides the necessary separation between source files and consumable output.**
