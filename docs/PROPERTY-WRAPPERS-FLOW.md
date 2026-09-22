# PropertyWrappers Data Flow Diagram

## Document Loading and Assembly Flow

### DesiredState Document

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. LOAD ROOT FILE                                               │
├─────────────────────────────────────────────────────────────────┤
│ desiredstate.yaml                                               │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ namespace: yago                                             │ │
│ │ schema: 4.2.0                                               │ │
│ │ desiredstate:                                               │ │
│ │   meta:                                                     │ │
│ │     parts:                                                  │ │
│ │       terraform: terraform/terraform.yaml                   │ │
│ │       concourse: concourse/concourse.yaml                   │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          ↓                                      │
│                   doc.assembledMeta.Data                             │
│                 (NEVER MODIFIED)                                │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ 2. ASSEMBLY (LoadParts)                                         │
├─────────────────────────────────────────────────────────────────┤
│ Copy assembledMeta → contentForConsumption                               │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ namespace: yago                                             │ │
│ │ schema: 4.2.0                                               │ │
│ │ desiredstate:                                               │ │
│ │   meta: { ... }                                             │ │
│ │   content: {}  ← EMPTY, will be filled                      │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          ↓                                      │
│ Load terraform/terraform.yaml                                   │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ desiredstate:                                               │ │
│ │   content:                                                  │ │
│ │     terraform:                                              │ │
│ │       backend: s3                                           │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          ↓                                      │
│ Merge at ROOT level (desiredstate merges with desiredstate)     │
│                          ↓                                      │
│ Load concourse/concourse.yaml and merge                         │
│                          ↓                                      │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ namespace: yago                                             │ │
│ │ schema: 4.2.0                                               │ │
│ │ desiredstate:                                               │ │
│ │   meta: { parts: ... }                                      │ │
│ │   content:                                                  │ │
│ │     terraform: { backend: s3 }  ← FROM PART                 │ │
│ │     concourse: { ... }          ← FROM PART                 │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          ↓                                      │
│              doc.contentForConsumption.Data                         │
│                 (MODIFIED)                                      │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ 3. CACHE (Cache)                                                │
├─────────────────────────────────────────────────────────────────┤
│ Write doc.contentForConsumption.Data to file                        │
│                          ↓                                      │
│ desiredstate-assembled.yaml                                     │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ namespace: yago                                             │ │
│ │ schema: 4.2.0                                               │ │
│ │ desiredstate:                                               │ │
│ │   meta: { ... }                                             │ │
│ │   content:                                                  │ │
│ │     terraform: { ... }                                      │ │
│ │     concourse: { ... }                                      │ │
│ └─────────────────────────────────────────────────────────────┘ │
│          ↓                                                      │
│ Used by wrapper tools (concourse, etc.)                         │
└─────────────────────────────────────────────────────────────────┘
```

### Configuration Document (with wrapper: terraform)

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. LOAD ROOT FILE                                               │
├─────────────────────────────────────────────────────────────────┤
│ configuration.yaml                                              │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ namespace: yago                                             │ │
│ │ schema: 4.2.0                                               │ │
│ │ configuration:                                              │ │
│ │   meta:                                                     │ │
│ │     parts:                                                  │ │
│ │       self: configuration.yaml                              │ │
│ │   content:                                                  │ │
│ │     wrappers:                                               │ │
│ │       terraform:                                            │ │
│ │         parts:                                              │ │
│ │           self: terraform/terraform.yaml                    │ │
│ │           vars: terraform/terraform2.yaml                   │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          ↓                                      │
│                   doc.assembledMeta.Data                             │
│                 (NEVER MODIFIED)                                │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ 2. ASSEMBLY (LoadParts) - Wrapper: terraform                    │
├─────────────────────────────────────────────────────────────────┤
│ Extract terraform parts only:                                   │
│   - terraform/terraform.yaml                                    │
│   - terraform/terraform2.yaml                                   │
│                          ↓                                      │
│ Load terraform/terraform.yaml                                   │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ terraform:                                                  │ │
│ │   region: eu-west-1                                         │ │
│ │   instance_type: t2.micro                                   │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          ↓                                      │
│ Load terraform/terraform2.yaml and merge                        │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ terraform:                                                  │ │
│ │   vpc_id: vpc-12345                                         │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          ↓                                      │
│ Merged result (RAW wrapper data):                               │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ terraform:                                                  │ │
│ │   region: eu-west-1                                         │ │
│ │   instance_type: t2.micro                                   │ │
│ │   vpc_id: vpc-12345                                         │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                          ↓                                      │
│              doc.contentForConsumption.Data                         │
│         (ONLY WRAPPER PARTS - NO STRUCTURE)                     │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ 3. CACHE (Cache)                                                │
├─────────────────────────────────────────────────────────────────┤
│ Write doc.contentForConsumption.Data to file                        │
│                          ↓                                      │
│ configuration-assembled.yaml                                    │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ terraform:                                                  │ │
│ │   region: eu-west-1                                         │ │
│ │   instance_type: t2.micro                                   │ │
│ │   vpc_id: vpc-12345                                         │ │
│ │                                                             │ │
│ │ ❌ NO namespace                                              │ │
│ │ ❌ NO schema                                                 │ │
│ │ ❌ NO configuration: wrapper                                 │ │
│ │ ❌ NO meta                                                   │ │
│ └─────────────────────────────────────────────────────────────┘ │
│          ↓                                                      │
│ Used directly by terraform (as .tfvars)                         │
└─────────────────────────────────────────────────────────────────┘
```

## Key Observations

### assembledMeta vs contentForConsumption

| Aspect | assembledMeta | contentForConsumption |
|--------|----------|-------------------|
| **Lifetime** | Set once, never changed | Modified during assembly |
| **Source** | Loaded from disk | Computed/assembled |
| **Structure** | Always has full document structure | Varies by document type |
| **Purpose** | Validation reference | Wrapper tool consumption |
| **Written to cache?** | ❌ No | ✅ Yes |

### Why Different Content?

**DesiredState**: Wrapper tools (like concourse) need the FULL context:

- Namespace and schema for validation
- Meta for understanding structure and parts
- Content for the actual state data

**Configuration**: Wrapper tools (like terraform) need ONLY their specific data:

- No GitOps structure overhead
- Direct consumption as .tfvars or similar
- Cleaner, focused configuration files

## Python Equivalents

```python
# Python GitOpsDocument
class GitOpsDocument:
    def __init__(self):
        self.meta = PropertyWrapper()        # → Go: assembledMeta
        self.content = PropertyWrapper()     # → Go: contentForConsumption
    
    def loadParts(self):
        if self.isDesiredState:
            self.content.data = copy.deepcopy(self.meta.data)
            # Merge parts...
        else:
            # Load wrapper parts
            self.content.mergeKeys(thisContent)
    
    def cache(self, pBuildDirectory, pIsGenerateJson):
        # Write self.content.data (NOT self.meta.data)
        pYaml.YamlHandler.toFile(self.content.data, configBuffer)
```

```go
// Go GitOpsDocument
type GitOpsDocument struct {
    assembledMeta          *property.PropertyWrapper  // Python: meta
    contentForConsumption *property.PropertyWrapper  // Python: content
}

func (doc *GitOpsDocument) LoadParts() error {
    if doc.isDesiredState {
        doc.contentForConsumption.Data = doc.deepCopyMap(doc.assembledMeta.Data)
        // Merge parts...
    } else {
        // Load wrapper parts
        doc.contentForConsumption.Data = mergedContent
    }
}

func (doc *GitOpsDocument) Cache(buildDirectory string, generateJSON bool) (string, error) {
    // Write doc.contentForConsumption.Data (NOT doc.assembledMeta.Data)
    doc.handler.ToFile(doc.contentForConsumption.Data, tempFile)
}
```
