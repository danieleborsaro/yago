package schema

import (
	"strings"
	"testing"
)

// =============================================================================
// TIME-BASED BDD: SchemaStore Behavioral Contracts
// =============================================================================
// These tests document the CURRENT behavior of SchemaStore at time T.
// They serve as:
// 1. Characterization tests (Michael Feathers, "Working Effectively with Legacy Code")
// 2. Regression detection for refactoring safety
// 3. Behavioral specification derived from working code
//
// Pattern: GoBehavioralContract
// Migrated from: store_test.go (211 lines, 5 test functions)
// Migration Date: October 13, 2025
// =============================================================================

// =============================================================================
// PHASE 1: SCHEMA REGISTRATION AND RETRIEVAL
// =============================================================================

func TestSchemaStore_RegisterSchema_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaStore registers schemas with composite key (namespace:version:kind) and provides retrieval via HasSchema() and GetSchema()",

		CurrentImpl: `
Go: internal/schema/store.go (SchemaStore)

type SchemaStore struct {
    schemas map[string]map[string]interface{}
    mu      sync.RWMutex
}

func NewSchemaStore() *SchemaStore {
    return &SchemaStore{
        schemas: make(map[string]map[string]interface{}),
    }
}

func (s *SchemaStore) RegisterSchema(namespace, version, kind string, schema map[string]interface{}) {
    s.mu.Lock()
    defer s.mu.Unlock()

    key := s.makeKey(namespace, version, kind)
    s.schemas[key] = schema
}

func (s *SchemaStore) HasSchema(namespace, version, kind string) bool {
    s.mu.RLock()
    defer s.mu.RUnlock()

    key := s.makeKey(namespace, version, kind)
    _, exists := s.schemas[key]
    return exists
}

func (s *SchemaStore) GetSchema(namespace, version, kind string) (map[string]interface{}, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    key := s.makeKey(namespace, version, kind)
    schema, exists := s.schemas[key]
    if !exists {
        return nil, fmt.Errorf("schema not found: %s:%s:%s", namespace, version, kind)
    }
    return schema, nil
}

func (s *SchemaStore) makeKey(namespace, version, kind string) string {
    return fmt.Sprintf("%s:%s:%s", namespace, version, kind)
}

Key features:
- Composite key: "namespace:version:kind" for unique identification
- In-memory map: Fast O(1) lookups
- Thread-safe: Read/write locks for concurrent access
- Simple registration: Store schema by composite key
- Existence check: HasSchema() returns bool
- Retrieval: GetSchema() returns schema or error
- No validation: Schemas stored as-is (validation elsewhere)
`,

		ExpectedOutcome: `
Registration:
- MUST store schema with composite key (namespace:version:kind)
- MUST accept any map[string]interface{} as schema
- MUST NOT validate schema content during registration
- MUST overwrite existing schema with same key (see DuplicateSchemaOverwrite test)

Existence check (HasSchema):
- MUST return true after registration
- MUST return false for non-existent schemas
- MUST check exact namespace:version:kind match

Retrieval (GetSchema):
- MUST return registered schema (not nil)
- MUST return same schema that was registered
- MUST return error for non-existent schemas
- MUST provide schema reference (not deep copy)

Thread safety:
- MUST support concurrent reads (RLock)
- MUST support concurrent writes (Lock)
- MUST prevent race conditions
`,

		TestScenario: `
GIVEN: New empty SchemaStore

WHEN: RegisterSchema("yago", "1.0.0", "DesiredState", schema1)
  - namespace: "yago"
  - version: "1.0.0"
  - kind: "DesiredState"
  - schema: {$schema: "...", type: "object", version: "1.0.0"}

THEN:
  - HasSchema("yago", "1.0.0", "DesiredState") == true
  - GetSchema("yago", "1.0.0", "DesiredState") returns schema (no error)
  - Retrieved schema is not nil
  - Retrieved schema matches registered schema

Test implementation:
1. Create new SchemaStore
2. Register schema with composite key
3. Verify HasSchema returns true
4. Verify GetSchema returns schema without error
5. Verify retrieved schema is not nil
`,

		Rationale: `
Why this behavior exists:
- In-memory storage: Fast access for runtime schema validation
- Composite key: Uniquely identifies schemas (multi-tenancy support)
- Simple API: RegisterSchema, HasSchema, GetSchema
- Thread safety: Multiple goroutines can access store safely
- No validation: Separation of concerns (validate elsewhere)

Composite key design (namespace:version:kind):
- Namespace: Tenant/organization isolation (yago, custom-org, etc.)
- Version: Schema evolution (1.0.0, 2.0.0, 4.2.0, etc.)
- Kind: Document type (DesiredState, Configuration)
- Uniqueness: All three needed for unique identification

SchemaStore use cases:
- Runtime: Validate documents against registered schemas
- Discovery: Check if schema version exists
- Multi-tenancy: Different organizations use different schemas
- Evolution: Multiple versions coexist in same store
- Fast access: O(1) lookup by composite key

Why map[string]interface{}:
- JSON schemas: Represented as nested maps
- Generic: Works with any JSON Schema structure
- No marshaling: Direct storage without serialization
- Fast: No parsing overhead on retrieval

Thread safety rationale:
- RWMutex: Multiple readers, single writer
- Read-heavy: Schema lookups more frequent than writes
- Startup registration: Schemas loaded at app start
- Runtime reads: Frequent GetSchema during validation
`,

		RegressionRisk: `
HIGH RISK if changed:
- Composite key format: "namespace:version:kind" is contract
- Thread safety: Removing locks causes race conditions
- HasSchema/GetSchema API: Code throughout system depends on this
- Schema storage: Must preserve exact schema content

MEDIUM RISK:
- Key generation: makeKey() format changes break lookups
- Error messages: "schema not found" expected by callers
- Schema reference: Callers may mutate retrieved schemas

LOW RISK:
- Internal map structure: Can change if API maintained
- Lock granularity: Could optimize locking strategy

What breaks if this changes:
1. Change key format → can't find registered schemas
2. Remove thread safety → race conditions, crashes
3. Validate during registration → performance hit, breaks valid uses
4. Deep copy schemas → performance impact, memory overhead
5. Change HasSchema signature → breaks all callers
6. Change GetSchema error → error handling breaks
`,
	}

	// Execute the behavioral test
	t.Run("Register and retrieve schema by composite key", func(t *testing.T) {
		store := NewSchemaStore()

		// Register a schema
		schema1 := map[string]interface{}{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type":    "object",
			"version": "1.0.0",
		}

		store.RegisterSchema("yago", "1.0.0", "DesiredState", schema1)

		// Verify it was registered
		if !store.HasSchema("yago", "1.0.0", "DesiredState") {
			t.Error("Schema should be registered")
		}

		// Verify we can retrieve it
		retrieved, err := store.GetSchema("yago", "1.0.0", "DesiredState")
		if err != nil {
			t.Errorf("Failed to get schema: %v", err)
		}

		if retrieved == nil {
			t.Error("Retrieved schema should not be nil")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 2: DUPLICATE SCHEMA OVERWRITE BEHAVIOR
// =============================================================================

func TestSchemaStore_DuplicateSchemaOverwrite_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaStore overwrites existing schema when same namespace:version:kind is registered again (last-write-wins semantics)",

		CurrentImpl: `
Go: internal/schema/store.go (RegisterSchema)

func (s *SchemaStore) RegisterSchema(namespace, version, kind string, schema map[string]interface{}) {
    s.mu.Lock()
    defer s.mu.Unlock()

    key := s.makeKey(namespace, version, kind)
    // Direct map assignment: overwrites if key exists
    s.schemas[key] = schema
}

Key features:
- No duplicate check: Doesn't check if key already exists
- Direct overwrite: s.schemas[key] = schema (replaces existing)
- Last-write-wins: Most recent RegisterSchema call wins
- No warning/error: Silent overwrite (no indication to caller)
- Idempotent: Safe to call multiple times with same data

Overwrite semantics:
- First registration: Creates entry in map
- Second registration: Replaces entry (no append, no merge)
- Schema content: Completely replaced (not updated/merged)
- No history: Previous schema lost (no versioning within store)
`,

		ExpectedOutcome: `
- MUST overwrite existing schema with same composite key
- MUST NOT merge with existing schema
- MUST NOT append to existing schema
- MUST NOT throw error on duplicate registration
- MUST NOT log warning on overwrite
- MUST use last registered schema (last-write-wins)
- MUST completely replace previous schema content

Retrieved schema after duplicate registration:
- MUST match most recent RegisterSchema call
- MUST NOT contain data from first registration
- MUST NOT be a merge of both registrations
`,

		TestScenario: `
GIVEN: Empty SchemaStore

WHEN:
  1. RegisterSchema("yago", "4.2.0", "DesiredState", schema1)
     - schema1: {description: "First schema"}
  2. RegisterSchema("yago", "4.2.0", "DesiredState", schema2)
     - schema2: {description: "Second schema - should overwrite"}

THEN:
  - GetSchema("yago", "4.2.0", "DesiredState") returns schema2
  - Retrieved schema has description: "Second schema - should overwrite"
  - schema1 is completely gone (not merged, not accessible)

Test implementation:
1. Create new SchemaStore
2. Register first schema with specific description
3. Register second schema with same key, different description
4. Retrieve schema
5. Verify retrieved schema matches second registration (not first)
6. Verify description field matches schema2 (overwrite confirmed)
`,

		Rationale: `
Why this behavior exists:
- Simplicity: No complex merge logic needed
- Reload support: Can reload schemas by re-registering
- Update mechanism: Allows schema updates during runtime
- No versioning: Store doesn't track schema history
- Developer control: Caller responsible for avoiding duplicates

Last-write-wins rationale:
- Common pattern: Maps naturally overwrite on duplicate keys
- Reload scenarios: Re-loading schemas from disk should work
- Testing: Tests can reset schemas between runs
- Update without delete: Don't need DeleteSchema + RegisterSchema
- Deterministic: Clear behavior (last one wins)

When overwrite is desired:
- Schema reload: Re-reading from filesystem
- Hot reload: Update schemas without restart
- Testing: Reset to known state between tests
- Development: Quick iteration on schema changes

When overwrite is problematic:
- Accidental duplicate: Two loads from different sources
- Concurrent writes: Race condition (though locks prevent corruption)
- Plugin conflicts: Two plugins register same namespace:version:kind
- Debugging: Hard to detect when overwrite happens silently

Alternative designs (not used):
- Error on duplicate: Would break reload scenarios
- Merge schemas: Complex, unclear semantics
- Version history: Memory overhead, complexity
- Warning log: Would spam logs during normal reload
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Overwrite semantics: Reload scenarios depend on this
- Silent behavior: Code expects no error on duplicate
- Last-write-wins: Changing to first-write-wins breaks updates

HIGH RISK:
- Adding error on duplicate → breaks schema reload
- Changing to merge → unpredictable schema content
- Adding version history → memory leaks, complexity

LOW RISK:
- Adding optional warning log: Minimal impact
- Making overwrite explicit (flag): Backward compatible

What breaks if this changes:
1. Error on duplicate → reload fails, tests break
2. Merge schemas → validation results unpredictable
3. First-write-wins → can't update schemas at runtime
4. Track history → memory growth, no cleanup mechanism
5. Require explicit overwrite flag → all registration sites need update
`,
	}

	// Execute the behavioral test
	t.Run("Second registration overwrites first (last-write-wins)", func(t *testing.T) {
		store := NewSchemaStore()

		// Register first schema
		schema1 := map[string]interface{}{
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "First schema",
		}
		store.RegisterSchema("yago", "4.2.0", "DesiredState", schema1)

		// Register second schema with same version and kind
		schema2 := map[string]interface{}{
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "Second schema - should overwrite",
		}
		store.RegisterSchema("yago", "4.2.0", "DesiredState", schema2)

		// Verify the second schema overwrote the first
		retrieved, err := store.GetSchema("yago", "4.2.0", "DesiredState")
		if err != nil {
			t.Errorf("Failed to get schema: %v", err)
		}

		if description, ok := retrieved["description"].(string); !ok || description != "Second schema - should overwrite" {
			t.Errorf("Expected second schema to overwrite first, got description: %v", retrieved["description"])
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 3: MULTIPLE KINDS WITH SAME VERSION
// =============================================================================

func TestSchemaStore_DifferentKindsSameVersion_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaStore stores DesiredState and Configuration schemas independently even with same namespace and version (kind is part of composite key)",

		CurrentImpl: `
Go: internal/schema/store.go (composite key includes kind)

Composite key format: "namespace:version:kind"

Examples:
- "yago:4.2.0:DesiredState"    → Stores DesiredState schema
- "yago:4.2.0:Configuration"   → Stores Configuration schema

These are DIFFERENT keys, so stored independently.

Key features:
- Kind discrimination: Different kinds = different keys
- Same version allowed: version + kind combination is unique
- Independent schemas: No relationship between kinds
- Separate retrieval: Must specify kind to retrieve
- Count correctly: Each kind counts as separate schema

Schema kinds in yago:
- DesiredState: Infrastructure desired state documents
- Configuration: Configuration parameter documents
- Both use same version scheme (1.0.0, 2.0.0, etc.)
- Both can coexist in same version
- Both independently registered and retrieved
`,

		ExpectedOutcome: `
Registration:
- MUST store DesiredState and Configuration independently
- MUST allow same namespace + version with different kinds
- MUST NOT overwrite when only kind differs
- MUST create separate entries in store

Existence checks:
- MUST HasSchema("yago", "4.2.0", "DesiredState") return true
- MUST HasSchema("yago", "4.2.0", "Configuration") return true
- MUST both return true independently

Retrieval:
- MUST retrieve correct schema for specified kind
- MUST NOT return Configuration when DesiredState requested
- MUST NOT return DesiredState when Configuration requested

Counting:
- MUST GetSchemaCount() return 2 (both kinds counted)
- MUST each kind count as separate schema
`,

		TestScenario: `
GIVEN: Empty SchemaStore

WHEN:
  1. RegisterSchema("yago", "4.2.0", "DesiredState", desiredStateSchema)
     - desiredStateSchema: {type: "DesiredState"}
  2. RegisterSchema("yago", "4.2.0", "Configuration", configSchema)
     - configSchema: {type: "Configuration"}

THEN:
  - HasSchema("yago", "4.2.0", "DesiredState") == true
  - HasSchema("yago", "4.2.0", "Configuration") == true
  - GetSchemaCount() == 2
  - Both schemas independently retrievable
  - Schemas don't conflict or overwrite each other

Test implementation:
1. Create new SchemaStore
2. Register DesiredState schema for version 4.2.0
3. Register Configuration schema for same version 4.2.0
4. Verify both HasSchema calls return true
5. Verify schema count is 2 (both stored)
6. Verify no overwrite occurred (kind makes key unique)
`,

		Rationale: `
Why this behavior exists:
- Document types: DesiredState and Configuration are different document types
- Same version: Both can evolve together (v4.2.0 for both)
- Independent validation: Different JSON schemas for each kind
- Composite key: Kind included to enable this independence
- No ambiguity: Always specify kind when retrieving

Kind as part of key rationale:
- Type safety: DesiredState and Configuration have different structures
- Version parity: Both kinds can share version numbers
- Clear semantics: "4.2.0" is version, "DesiredState" is type
- Validation: Each kind validated against its own schema
- Document identification: Documents specify both version and kind

Real-world scenario:
- Version 4.2.0 release: Includes both schema types
- Namespace: Organization (yago, custom-org, etc.)
- Version: Schema format version (4.2.0)
- Kind: Document type (DesiredState or Configuration)
- Example keys:
  - "yago:4.2.0:DesiredState"
  - "yago:4.2.0:Configuration"
  - "custom-org:1.0.0:DesiredState"
  - "custom-org:1.0.0:Configuration"

Benefits:
- Clear organization: Kinds logically separated
- No collisions: Kind prevents accidental overwrites
- Version alignment: Both kinds can use same version number
- Independent evolution: Can update one kind without affecting other
- Multi-tenancy: Namespace isolates organizations
`,

		RegressionRisk: `
HIGH RISK if changed:
- Composite key must include kind: Removing breaks independence
- Kind discrimination: Critical for correct schema retrieval
- Count semantics: Each kind must count separately

MEDIUM RISK:
- Key format: Changing to "namespace:version" (no kind) breaks system
- Kind validation: Must preserve exact kind values

LOW RISK:
- Additional kinds: System supports any kind value
- Kind naming: "DesiredState", "Configuration" are conventions

What breaks if this changes:
1. Remove kind from key → configurations overwrite desired states
2. Merge kinds → can't distinguish document types
3. Change kind values → all schemas must re-register
4. Add kind validation → restricts extensibility
5. Count only versions → GetSchemaCount() misleading
`,
	}

	// Execute the behavioral test
	t.Run("Store DesiredState and Configuration independently for same version", func(t *testing.T) {
		store := NewSchemaStore()

		// Register DesiredState schema
		desiredStateSchema := map[string]interface{}{
			"type": "DesiredState",
		}
		store.RegisterSchema("yago", "4.2.0", "DesiredState", desiredStateSchema)

		// Register Configuration schema with same version
		configSchema := map[string]interface{}{
			"type": "Configuration",
		}
		store.RegisterSchema("yago", "4.2.0", "Configuration", configSchema)

		// Both should exist independently
		if !store.HasSchema("yago", "4.2.0", "DesiredState") {
			t.Error("DesiredState schema should exist")
		}

		if !store.HasSchema("yago", "4.2.0", "Configuration") {
			t.Error("Configuration schema should exist")
		}

		// Count should be 2
		if store.GetSchemaCount() != 2 {
			t.Errorf("Expected 2 schemas, got %d", store.GetSchemaCount())
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 4: NON-EXISTENT SCHEMA ERROR HANDLING
// =============================================================================

func TestSchemaStore_GetNonExistentSchema_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaStore returns descriptive error when attempting to retrieve schema that was never registered (GetSchema fails, HasSchema returns false)",

		CurrentImpl: `
Go: internal/schema/store.go (GetSchema error handling)

func (s *SchemaStore) GetSchema(namespace, version, kind string) (map[string]interface{}, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    key := s.makeKey(namespace, version, kind)
    schema, exists := s.schemas[key]
    if !exists {
        // Return error with context
        return nil, fmt.Errorf("schema not found: %s:%s:%s", namespace, version, kind)
    }
    return schema, nil
}

Key features:
- Existence check: Check map key existence before returning
- Nil + error: Return pattern for missing schema
- Error message: Includes namespace:version:kind for debugging
- "schema not found" prefix: Consistent error prefix
- No panic: Graceful error handling (not fatal)

Error message format:
"schema not found: {namespace}:{version}:{kind}"

Examples:
- "schema not found: yago:99.0.0:NonExistent"
- "schema not found: custom-org:1.0.0:DesiredState"
`,

		ExpectedOutcome: `
GetSchema for non-existent schema:
- MUST return error (err != nil)
- MUST return nil schema
- MUST error message contain "schema not found"
- SHOULD error message include namespace, version, kind
- MUST NOT panic
- MUST NOT return empty schema

HasSchema for non-existent schema:
- MUST return false
- MUST NOT error (returns bool, not error)
- MUST NOT panic

Error handling:
- MUST be consistent across all non-existent lookups
- MUST provide enough info for debugging
- MUST NOT leak internal implementation details
`,

		TestScenario: `
GIVEN: Empty SchemaStore (no schemas registered)

WHEN: GetSchema("yago", "99.0.0", "NonExistent")
  - namespace: "yago"
  - version: "99.0.0" (doesn't exist)
  - kind: "NonExistent"

THEN:
  - Returns error (err != nil)
  - Error message contains "schema not found"
  - Returned schema is nil
  - No panic occurs

Test implementation:
1. Create new SchemaStore (don't register any schemas)
2. Attempt to get non-existent schema
3. Assert err != nil
4. Assert error message contains "schema not found"
5. Verify graceful error handling (no panic)
`,

		Rationale: `
Why this behavior exists:
- Fail gracefully: Missing schema is expected scenario
- Clear errors: User knows exactly what's missing
- Debuggability: Error message shows what was requested
- No panic: Service remains stable on missing schema
- Caller control: Caller decides how to handle error

Error handling rationale:
- Return error: Go idiom for optional/failing operations
- Include key: Shows exactly what was requested
- Consistent format: Predictable error parsing
- No schema default: Don't guess what schema to use
- Explicit failure: Better than silent wrong behavior

When schemas might be missing:
- Wrong version: User requests unsupported version
- Wrong namespace: Typo in namespace name
- Wrong kind: Requests invalid document type
- Not loaded: Plugin schemas not loaded yet
- Unregistered: Schema exists but not in this store

Error recovery options (caller's responsibility):
- Fallback version: Try different version
- Default schema: Use built-in schema
- Fail validation: Return error to user
- Retry: Wait for plugin to load
- Log warning: Inform about missing schema

Alternative designs (not used):
- Return empty schema: Misleading, validation would pass incorrectly
- Panic: Too severe for missing schema
- Return default: No sensible default for arbitrary schemas
- Silent nil: Caller wouldn't know why it failed
- Error code: Go uses error messages, not codes
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Error message format: Callers may parse "schema not found"
- Return values: (nil, error) pattern expected
- Error clarity: Message must help debugging

HIGH RISK:
- Panic instead of error → service crashes
- Return empty schema → validation breaks
- Remove error details → debugging harder

LOW RISK:
- Error message wording: Can improve as long as "schema not found" present
- Error type: Could use custom error type

What breaks if this changes:
1. Panic on missing → service instability
2. Return empty schema → incorrect validation results
3. Change error message → error parsing breaks
4. Remove key from error → debugging harder
5. Return (schema, false) instead → API change breaks callers
`,
	}

	// Execute the behavioral test
	t.Run("Return error for non-existent schema", func(t *testing.T) {
		store := NewSchemaStore()

		_, err := store.GetSchema("yago", "99.0.0", "NonExistent")
		if err == nil {
			t.Error("Expected error when getting non-existent schema")
		}

		expectedMsg := "schema not found"
		if err != nil && !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error to contain '%s', got: %s", expectedMsg, err.Error())
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 5: AVAILABLE VERSIONS DISCOVERY
// =============================================================================

func TestSchemaStore_GetAvailableVersions_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaStore provides list of unique schema versions across all namespaces and kinds (version deduplication for discovery)",

		CurrentImpl: `
Go: internal/schema/store.go (GetAvailableVersions)

func (s *SchemaStore) GetAvailableVersions() []string {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Use map to deduplicate versions
    versionSet := make(map[string]bool)

    // Iterate all schemas
    for key := range s.schemas {
        // Parse key: "namespace:version:kind"
        parts := strings.Split(key, ":")
        if len(parts) >= 2 {
            version := parts[1]
            versionSet[version] = true
        }
    }

    // Convert map to slice
    versions := make([]string, 0, len(versionSet))
    for version := range versionSet {
        versions = append(versions, version)
    }

    return versions
}

Key features:
- Deduplication: Same version appears once even if in multiple schemas
- Cross-namespace: Includes versions from all namespaces
- Cross-kind: Includes versions from all kinds
- Unordered: No sorting (random map iteration order)
- Version extraction: Parses composite keys to extract version

Deduplication example:
- Registered: yago:4.2.0:DesiredState
- Registered: yago:4.2.0:Configuration
- Result: ["4.2.0"] (appears once, not twice)

Use case: Version discovery for UI/CLI
- Show available versions to user
- Version dropdown in UI
- Help text: "Available versions: 1.0.0, 2.0.0, 4.2.0"
`,

		ExpectedOutcome: `
Version deduplication:
- MUST return each unique version once
- MUST NOT return duplicates even if multiple kinds use same version
- MUST NOT return duplicates even if multiple namespaces use same version

Version collection:
- MUST include versions from all registered schemas
- MUST extract version from composite key
- MUST handle any namespace and kind

Return value:
- MUST return []string (slice of versions)
- MAY return in any order (no sort requirement)
- MUST return empty slice if no schemas registered
- MUST include all unique versions

Expected behavior for test scenario:
- Register: yago:1.0.0:DesiredState
- Register: yago:2.1.0:DesiredState
- Register: yago:4.2.0:DesiredState
- Register: yago:4.2.0:Configuration
- Result: 3 unique versions (1.0.0, 2.1.0, 4.2.0)
- 4.2.0 appears only once despite two registrations
`,

		TestScenario: `
GIVEN: Empty SchemaStore

WHEN: Register multiple schemas
  - RegisterSchema("yago", "1.0.0", "DesiredState", ...)
  - RegisterSchema("yago", "2.1.0", "DesiredState", ...)
  - RegisterSchema("yago", "4.2.0", "DesiredState", ...)
  - RegisterSchema("yago", "4.2.0", "Configuration", ...)

THEN: GetAvailableVersions() returns:
  - Length: 3 (unique versions)
  - Contains: "1.0.0"
  - Contains: "2.1.0"
  - Contains: "4.2.0"
  - Does NOT contain duplicates
  - 4.2.0 appears once (not twice)

Test implementation:
1. Create new SchemaStore
2. Register 4 schemas with 3 unique versions
3. Call GetAvailableVersions()
4. Verify length is 3 (not 4)
5. Verify each expected version present
6. Verify 4.2.0 deduplication worked
`,

		Rationale: `
Why this behavior exists:
- Version discovery: Users need to know what versions are available
- UI/CLI: Populate version dropdowns, help text
- Deduplication: Same version used by multiple kinds (normal)
- Simple API: Just return list of versions
- No filtering: Return all versions (caller filters if needed)

Deduplication rationale:
- User perspective: Version is version, regardless of kind
- Common pattern: DesiredState and Configuration share versions
- Simplicity: User wants "what versions exist", not "what version+kind combos"
- Discovery: "Can I use v4.2.0?" should be simple yes/no

Use cases:
- CLI help: "yago validate --version 4.2.0" (show available versions)
- UI dropdown: Select version for validation
- Documentation: List supported versions
- Migration: "What versions can I migrate from?"
- Testing: Iterate over all available versions

Why no sorting:
- Performance: Sorting adds overhead
- Caller's choice: Caller may want different sort order
- Semantic versioning: Sorting versions requires semver logic
- Simple implementation: Just return what's there

Alternative designs (not used):
- Return map[version][]kind: More info but more complex
- Return sorted list: Adds dependency on version parsing
- Return version objects: Overkill for simple list
- Return map[namespace][]version: Namespace not relevant for discovery
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Deduplication: Callers expect unique versions
- Return type: []string expected throughout system
- Version extraction: Must correctly parse composite keys

LOW RISK:
- Sort order: No order guaranteed currently
- Empty slice: Correct behavior for no schemas
- Version format: Versions are opaque strings

HIGH RISK:
- Return duplicates → UI shows duplicates, confusing
- Include namespace/kind → breaks simple version list use case
- Change return type → all callers break

What breaks if this changes:
1. Return duplicates → UI dropdowns show "4.2.0" twice
2. Add sorting → performance impact, version parsing complexity
3. Return map instead of slice → API change breaks callers
4. Filter versions → discovery incomplete
5. Include namespace in result → version list polluted
`,
	}

	// Execute the behavioral test
	t.Run("Return unique versions with deduplication", func(t *testing.T) {
		store := NewSchemaStore()

		// Register multiple schemas
		store.RegisterSchema("yago", "1.0.0", "DesiredState", map[string]interface{}{})
		store.RegisterSchema("yago", "2.1.0", "DesiredState", map[string]interface{}{})
		store.RegisterSchema("yago", "4.2.0", "DesiredState", map[string]interface{}{})
		store.RegisterSchema("yago", "4.2.0", "Configuration", map[string]interface{}{})

		versions := store.GetAvailableVersions()

		// Should have 3 unique versions (4.2.0 appears for both kinds but counts once)
		if len(versions) != 3 {
			t.Errorf("Expected 3 versions, got %d: %v", len(versions), versions)
		}

		// Check each expected version exists
		expectedVersions := []string{"1.0.0", "2.1.0", "4.2.0"}
		for _, expected := range expectedVersions {
			found := false
			for _, v := range versions {
				if v == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected version %s not found in: %v", expected, versions)
			}
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}
