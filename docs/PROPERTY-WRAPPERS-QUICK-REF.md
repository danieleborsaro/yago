# PropertyWrappers Quick Reference

## TL;DR

`GitOpsDocument` has **two PropertyWrappers**:

| Field | Purpose | Modified? | What's Inside |
|-------|---------|-----------|---------------|
| `assembledMeta` | Source file | ❌ Never | Original YAML as loaded |
| `contentForConsumption` | Output for tools | ✅ Yes | What gets cached/consumed |

## Python → Go Mapping

```python
# Python
self.meta       →  doc.assembledMeta
self.content    →  doc.contentForConsumption
```

## What Gets Written to Cache?

```go
Cache() writes doc.contentForConsumption.Data
```

**NOT** `doc.assembledMeta.Data` ❌

## Content by Document Type

### DesiredState

```yaml
# contentForConsumption contains:
namespace: yago
schema: 4.2.0
desiredstate:
  content:    # ← Assembled from parts
    ...
  meta:       # ← From root file
    ...
```

### Configuration (with wrapper)

```yaml
# contentForConsumption contains ONLY:
terraform:      # ← Just wrapper parts
  variables:
    ...
# NO namespace ❌
# NO schema ❌
# NO meta ❌
# NO configuration: wrapper ❌
```

## Common Operations

### Get Original File (Metadata)

```go
doc.GetMeta()
// Returns: Unmodified source YAML (assembledMeta)
```

### Get Output for Wrappers

```go
doc.GetContent()
// Returns: Assembled data for tools (contentForConsumption)
```

### Write to Cache

```go
filePath, err := doc.Cache(buildDir, false)
// Writes: doc.contentForConsumption.Data
// Format: YAML (or JSON if second param is true)
```

## Assembly Flow

### DesiredState

1. Load `assembledMeta` ← desiredstate.yaml
2. Copy to `contentForConsumption`
3. Merge parts at root level
4. Result: Full assembled document

### Configuration (with wrapper)

1. Load `assembledMeta` ← configuration.yaml
2. Load wrapper part files
3. Merge wrapper parts
4. Set `contentForConsumption` ← merged parts ONLY
5. Result: Raw wrapper data (no structure)

## Key Rules

✅ **DO**:

- Use `GetMeta()` / `assembledMeta` for validation and metadata extraction
- Use `GetContent()` / `contentForConsumption` for wrapper tools and caching
- Modify `contentForConsumption` during assembly
- Keep `assembledMeta` immutable after load

❌ **DON'T**:

- Modify `assembledMeta` after initial load
- Write `assembledMeta` to cache (use `contentForConsumption`)
- Assume `contentForConsumption` always has structure (it varies by type)

## API Summary

```go
// Recommended getters (simplified API)
doc.GetMeta()     // Returns assembledMeta - source file
doc.GetContent()  // Returns contentForConsumption - output
```

## See Also

- [Full Design Documentation](PROPERTY-WRAPPERS-DESIGN.md) - Detailed architecture guide
- [internal/core/document.go](../internal/core/document.go) - Implementation with inline docs
