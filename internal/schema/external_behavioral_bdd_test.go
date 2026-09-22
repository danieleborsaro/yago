package schema

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// TIME-BASED BDD: ExternalSchemaProvider Behavioral Contracts
// =============================================================================
// These tests document the CURRENT behavior of ExternalSchemaProvider at time T.
// They serve as:
// 1. Characterization tests (Michael Feathers, "Working Effectively with Legacy Code")
// 2. Regression detection for refactoring safety
// 3. Behavioral specification derived from working code
//
// Pattern: GoBehavioralContract
// Migrated from: external_test.go (277 lines, 6 test functions)
// Migration Date: October 13, 2025
// =============================================================================

// =============================================================================
// PHASE 1: EXTERNAL SCHEMA LOADING
// =============================================================================

func TestExternalSchemaProvider_LoadSchemas_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ExternalSchemaProvider loads JSON schemas from filesystem directories and registers them in SchemaStore",

		CurrentImpl: `
Go: internal/schema/external.go (ExternalSchemaProvider.LoadSchemasFromPaths)

func (esp *ExternalSchemaProvider) LoadSchemasFromPaths(paths []string,
    embedded bool) error {

    for _, path := range paths {
        // Find all manifest files in the directory
        manifestFiles := esp.findManifestFiles(path)

        for _, manifestFile := range manifestFiles {
            // Load manifest and register schemas
            err := esp.loadManifest(manifestFile, path)
            if err != nil {
                return err
            }
        }
    }
    return nil
}

func (esp *ExternalSchemaProvider) loadManifest(manifestPath string,
    basePath string) error {

    // Parse manifest JSON
    manifest := parseManifestJSON(manifestPath)

    // Extract namespace and version from manifest
    namespace := manifest["namespace"]

    // Load each schema defined in manifest
    for _, schemaEntry := range manifest["schemas"] {
        version := schemaEntry["version"]
        schemaType := schemaEntry["type"]
        schemaFile := schemaEntry["file"]

        // Read schema content
        schemaPath := filepath.Join(basePath, schemaFile)
        schemaContent := readJSONFile(schemaPath)

        // Register in store
        esp.store.AddSchema(namespace, version, schemaType, schemaContent)
    }
    return nil
}

Key features:
- Scans directory for *-manifest.json files
- Parses manifest to get namespace, versions, schema types
- Loads referenced JSON schema files
- Registers schemas in SchemaStore for later retrieval
- Supports both embedded (assets/) and external (custom) schemas
- Uses filepath operations for cross-platform compatibility
`,

		ExpectedOutcome: `
- MUST load all *-manifest.json files from specified paths
- MUST parse each manifest to extract namespace and schema definitions
- MUST load referenced JSON schema files from filesystem
- MUST register schemas in SchemaStore with (namespace, version, type) keys
- MUST support loading from assets/schemas/gitops/ directory
- MUST make schemas retrievable via store.GetSchema(namespace, version, type)
- MUST properly load v1.0.0 DesiredState schema with 'schema' and 'desiredstate' properties
- SHOULD validate that loaded schemas have expected structure
- SHOULD handle nested directory structures
`,

		TestScenario: `
GIVEN:
  - Project root at "../.." relative to internal/schema/
  - Schema files in assets/schemas/gitops/ directory
  - SchemaStore initialized (empty)
  - ExternalSchemaProvider created with schemaPath and store

WHEN: LoadSchemasFromPaths([]string{gitopsPath}, true) is called

THEN:
  1. Provider scans gitopsPath for manifest files
  2. Manifests are parsed and schemas loaded
  3. Schemas are registered in store
  4. store.GetSchema("yago", "1.0.0", "DesiredState") returns valid schema
  5. Schema has "properties" field
  6. Properties contain "schema" field (current format, not legacy "schemaVersion")
  7. Properties contain "desiredstate" field (kind deduced from root property, not "kind" field)
  8. err == nil (successful load)

Test implementation:
1. Get current directory: os.Getwd()
2. Navigate to project root: filepath.Join(currentDir, "..", "..")
3. Build schema path: filepath.Join(projectRoot, "assets", "schemas")
4. Create SchemaStore: NewSchemaStore()
5. Create provider: NewExternalSchemaProvider(schemaPath, store)
6. Load schemas: provider.LoadSchemasFromPaths([]string{gitopsPath}, true)
7. Assert err == nil
8. Retrieve schema: store.GetSchema("yago", "1.0.0", "DesiredState")
9. Assert schema != nil
10. Assert schema has expected properties
`,

		Rationale: `
Why this behavior exists:
- Yago needs to validate documents against versioned JSON schemas
- Schemas must be loadable from filesystem for extensibility
- External schema loading enables custom schema support
- Manifest files provide metadata (namespace, versions) for schema organization
- Schema store provides centralized registry for retrieval by validation logic
- Current schema version uses 'schema' field (not legacy 'schemaVersion')
- Document kind is deduced from root property ('desiredstate' or 'configuration'), not from explicit 'kind' field

Historical context:
- Assets directory structure: assets/schemas/gitops/
- Manifest file naming: *-manifest.json
- Schema version format: semantic versioning (1.0.0)
- Namespace support for multi-tenant schema management
`,

		RegressionRisk: `
HIGH RISK if changed without care:
- Document validation will break if schema loading fails
- Custom schemas won't load if manifest parsing changes
- Schema versioning logic depends on manifest structure
- Breaking changes to schema structure breaks all document parsing
- Path resolution must work across dev/test/production environments

MEDIUM RISK:
- Changes to manifest format require migration of all existing manifests
- Schema file naming conventions are part of the contract
- Error handling affects user experience during schema load failures

What breaks if this changes:
1. Schema validation fails → documents can't be processed
2. Version detection breaks → wrong schema applied
3. Custom schema loading fails → extensibility lost
4. Path resolution breaks → schemas not found in CI/CD
`,
	}

	// Execute the behavioral test
	t.Run("Load schemas from gitops directory", func(t *testing.T) {
		// Get the project root by going up from internal/schema
		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get current directory: %v", err)
		}

		// Navigate to project root
		projectRoot := filepath.Join(currentDir, "..", "..")
		schemaPath := filepath.Join(projectRoot, "assets", "schemas")

		// Create a schema store
		store := NewSchemaStore()
		provider := NewExternalSchemaProvider(schemaPath, store)

		// Load schemas from the schemas directory
		gitopsPath := schemaPath
		err = provider.LoadSchemasFromPaths([]string{gitopsPath}, true)
		if err != nil {
			t.Fatalf("Failed to load schemas: %v", err)
		}

		// Test that core schemas were loaded using the store
		schema, err := store.GetSchema("yago", "1.0.0", "DesiredState")
		if err != nil {
			t.Errorf("Failed to get v1.0.0 DesiredState schema: %v", err)
		}

		if schema == nil {
			t.Error("Schema should not be nil")
		}

		// Check that schema has expected properties
		properties, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Error("Schema should have properties")
		}

		// Current schema version uses 'schema' field (not legacy 'schemaVersion')
		if _, exists := properties["schema"]; !exists {
			t.Error("Schema should have schema property")
		}

		// Documents should have either "desiredstate" or "configuration" root property
		// The kind is deduced from which root property exists, NOT from a "kind" field
		if _, exists := properties["desiredstate"]; !exists {
			t.Error("DesiredState schema should have desiredstate property (not 'kind')")
		}
	})

	// Log the contract for documentation
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 2: SCHEMA VERSION MANAGEMENT
// =============================================================================

func TestExternalSchemaProvider_GetAvailableVersions_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaStore provides list of all loaded schema versions after external provider loads schemas",

		CurrentImpl: `
Go: internal/schema/store.go (SchemaStore.GetAvailableVersions)

func (ss *SchemaStore) GetAvailableVersions() []string {
    ss.mu.RLock()
    defer ss.mu.RUnlock()

    versionsMap := make(map[string]bool)

    // Iterate through all registered schemas
    for key := range ss.schemas {
        // Extract version from composite key
        // Key format: "namespace:version:type"
        parts := strings.Split(key, ":")
        if len(parts) >= 2 {
            version := parts[1]
            versionsMap[version] = true
        }
    }

    // Convert map to sorted list
    versions := make([]string, 0, len(versionsMap))
    for version := range versionsMap {
        versions = append(versions, version)
    }

    sort.Strings(versions)
    return versions
}

Key features:
- Extracts unique versions from all registered schemas
- Returns deduplicated list of versions
- Thread-safe with read lock
- Returns sorted list for consistency
- Works across all namespaces and schema types
`,

		ExpectedOutcome: `
- MUST return non-empty list after schemas are loaded
- MUST include version "1.0.0" (embedded yago schema version)
- MUST deduplicate versions across different schema types
- MUST work after LoadSchemasFromPaths() completes
- SHOULD return sorted list for predictable behavior
- MAY include additional versions if custom schemas loaded
`,

		TestScenario: `
GIVEN:
  - SchemaStore initialized
  - ExternalSchemaProvider created with store
  - Schemas loaded from gitops directory via LoadSchemasFromPaths()

WHEN: store.GetAvailableVersions() is called

THEN:
  1. Returns non-empty list of version strings
  2. List includes "1.0.0" (expected yago schema version)
  3. List may include other versions from loaded schemas
  4. No duplicates in the list

Test implementation:
1. Setup: Load schemas from gitops directory (same as Phase 1)
2. Call: versions := store.GetAvailableVersions()
3. Assert: len(versions) > 0
4. Assert: "1.0.0" is in versions list
5. Verify: All expected versions present
`,

		Rationale: `
Why this behavior exists:
- Version discovery: Clients need to know what schema versions are available
- Validation logic: Document validation needs to check if requested version exists
- User feedback: CLI/API can display available versions to users
- Schema compatibility: Enables version negotiation and compatibility checks

Use cases:
- Document processing: Check if document's schema version is supported
- Error messages: "Version X not found, available versions: [...]"
- Schema management: List all loaded schema versions for diagnostics
- Testing: Verify expected schemas are loaded correctly
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Version detection logic depends on accurate version listing
- Error messages will show incorrect available versions
- Schema compatibility checks may fail

LOW RISK:
- This is a read-only query operation
- No side effects on schema storage
- Can be refactored without breaking validation logic

What breaks if this changes:
1. Version validation errors show wrong information
2. CLI/API version listing becomes inaccurate
3. Testing/diagnostics lose visibility into loaded schemas
`,
	}

	// Execute the behavioral test
	t.Run("Get available versions after loading schemas", func(t *testing.T) {
		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get current directory: %v", err)
		}

		projectRoot := filepath.Join(currentDir, "..", "..")
		schemaPath := filepath.Join(projectRoot, "assets", "schemas")

		// Create a schema store
		store := NewSchemaStore()
		provider := NewExternalSchemaProvider(schemaPath, store)

		// Load schemas from the schemas directory
		gitopsPath := schemaPath
		err = provider.LoadSchemasFromPaths([]string{gitopsPath}, true)
		if err != nil {
			t.Fatalf("Failed to load schemas: %v", err)
		}

		// Get versions from the store
		versions := store.GetAvailableVersions()
		if len(versions) == 0 {
			t.Error("Should have at least one version available")
		}

		// Check if we have the expected version (embedded yago schema is 1.0.0)
		expectedVersions := []string{"1.0.0"}
		for _, expected := range expectedVersions {
			found := false
			for _, version := range versions {
				if version == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected version %s not found in available versions: %v", expected, versions)
			}
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// PHASE 3: SCHEMA VALIDATION
// =============================================================================

func TestExternalSchemaProvider_ValidateSchema_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaStore validates schema existence by checking if namespace/version/type combination is registered",

		CurrentImpl: `
Go: internal/schema/store.go (SchemaStore.HasSchema)

func (ss *SchemaStore) HasSchema(namespace string, version string,
    schemaType string) bool {

    ss.mu.RLock()
    defer ss.mu.RUnlock()

    // Build composite key
    key := ss.buildKey(namespace, version, schemaType)

    // Check if key exists in schemas map
    _, exists := ss.schemas[key]
    return exists
}

func (ss *SchemaStore) buildKey(namespace, version, schemaType string) string {
    return fmt.Sprintf("%s:%s:%s", namespace, version, schemaType)
}

Key features:
- Simple existence check (not structural validation)
- Uses composite key: "namespace:version:type"
- Thread-safe with read lock
- Fast O(1) lookup via map
- Returns boolean (true = exists, false = not found)
`,

		ExpectedOutcome: `
- MUST return true for schemas that exist in store
- MUST return false for schemas that don't exist
- MUST work for loaded schemas (e.g., "yago", "1.0.0", "DesiredState")
- MUST return false for non-existent namespaces
- MUST return false for non-existent versions (e.g., "99.0.0")
- MUST return false for non-existent types
- SHOULD be fast (O(1) map lookup)
`,

		TestScenario: `
GIVEN:
  - SchemaStore with loaded schemas from gitops directory
  - Known schema: namespace="yago", version="1.0.0", type="DesiredState"

WHEN: HasSchema() is called with various combinations

THEN:
  - HasSchema("yago", "1.0.0", "DesiredState") returns true
  - HasSchema("yago", "99.0.0", "NonExistent") returns false

Test implementation:
1. Setup: Load schemas from gitops directory
2. Positive test: Assert HasSchema("yago", "1.0.0", "DesiredState") == true
3. Negative test: Assert HasSchema("yago", "99.0.0", "NonExistent") == false
4. Verify: Existence check is accurate
`,

		Rationale: `
Why this behavior exists:
- Pre-validation: Check if schema exists before attempting to load
- Error prevention: Avoid errors during GetSchema() by checking first
- User feedback: Provide clear error when requested schema not found
- Fast existence check: Avoid expensive schema loading for non-existent schemas

Note: "Validation" here means existence check, not structural schema validation.
Structural validation happens later via gojsonschema library.
`,

		RegressionRisk: `
LOW RISK if changed:
- Simple boolean existence check
- No side effects
- Can be refactored without breaking functionality

MEDIUM RISK:
- Error handling logic depends on accurate existence checks
- Validation workflow expects reliable HasSchema() results

What breaks if this changes:
1. GetSchema() may be called on non-existent schemas → errors
2. Error messages become less helpful (can't distinguish "not found" from other errors)
3. Performance may degrade if existence check becomes expensive
`,
	}

	// Execute the behavioral test
	t.Run("Validate schema existence", func(t *testing.T) {
		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get current directory: %v", err)
		}

		projectRoot := filepath.Join(currentDir, "..", "..")
		schemaPath := filepath.Join(projectRoot, "assets", "schemas")

		// Create a schema store
		store := NewSchemaStore()
		provider := NewExternalSchemaProvider(schemaPath, store)

		// Load schemas from the schemas directory
		gitopsPath := schemaPath
		err = provider.LoadSchemasFromPaths([]string{gitopsPath}, true)
		if err != nil {
			t.Fatalf("Failed to load schemas: %v", err)
		}

		// Check schema exists in store (validation is now basic - just checks existence)
		if !store.HasSchema("yago", "1.0.0", "DesiredState") {
			t.Error("Schema 1.0.0 DesiredState should exist")
		}

		// Test with a non-existent schema
		if store.HasSchema("yago", "99.0.0", "NonExistent") {
			t.Error("Non-existent schema should not be found")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// PHASE 4: CUSTOM SCHEMA LOADING
// =============================================================================

func TestExternalSchemaProvider_LoadCustomSchemas_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ExternalSchemaProvider loads custom schemas with custom namespaces and versions, extending base schema functionality",

		CurrentImpl: `
Go: internal/schema/external.go (ExternalSchemaProvider.LoadSchemasFromPaths)

Custom schema loading workflow:
1. Load built-in schemas first (embedded=true)
   provider.LoadSchemasFromPaths([]string{builtinPath}, true)

2. Load custom schemas second (embedded=false)
   provider.LoadSchemasFromPaths([]string{customPath}, false)

3. Custom manifest structure:
   {
     "namespace": "custom-org",  // Custom namespace (not "yago")
     "version": "1.0.0",
     "manifestVersion": "1.0.0",
     "schemas": [
       {
         "version": "1.0.0-custom",  // Custom version string
         "type": "DesiredState",
         "file": "custom-desiredstate-v1.0.0.json"
       }
     ]
   }

4. Custom schemas can extend base schemas with additional properties:
   - Standard metadata fields (name, namespace, labels)
   - Custom metadata fields (costCenter, businessUnit, etc.)
   - Custom validation rules

Key features:
- Supports multiple schema namespaces (yago, custom-org, etc.)
- Custom version strings (1.0.0-custom, 2.0.0-enterprise, etc.)
- Custom schema files with extended properties
- Schemas registered with unique keys (namespace:version:type)
- Custom schemas don't overwrite built-in schemas
`,

		ExpectedOutcome: `
- MUST load built-in schemas first without errors
- MUST load custom schemas second without errors
- MUST support custom namespace (e.g., "custom-org")
- MUST support custom version strings (e.g., "1.0.0-custom")
- MUST register custom schemas in store separately from built-in
- MUST make custom schemas retrievable via GetSchema()
- MUST preserve custom schema properties (costCenter, businessUnit)
- SHOULD support schema extension patterns (adding custom fields)
- SHOULD not overwrite existing schemas with same namespace
`,

		TestScenario: `
GIVEN:
  - Built-in schemas in assets/schemas/gitops/
  - Custom schemas in schemas/ (project root)
  - Custom manifest with namespace="custom-org", version="1.0.0-custom"
  - Custom schema with additional metadata fields

WHEN:
  1. Load built-in schemas: LoadSchemasFromPaths([builtinPath], true)
  2. Load custom schemas: LoadSchemasFromPaths([customPath], false)

THEN:
  1. Both loads succeed (err == nil)
  2. Custom schema retrievable: GetSchema("custom-org", "1.0.0-custom", "DesiredState")
  3. Custom schema has "properties" field
  4. Custom schema has "metadata" with "properties"
  5. Metadata includes custom fields: "costCenter", "businessUnit"
  6. Built-in and custom schemas coexist in store

Test implementation:
1. Load built-in: provider.LoadSchemasFromPaths([builtinGitopsPath], true)
2. Load custom: provider.LoadSchemasFromPaths([gitopsPath], false)
3. Retrieve: schema, err := store.GetSchema("custom-org", "1.0.0-custom", "DesiredState")
4. Assert: schema != nil && err == nil
5. Assert: schema["properties"]["metadata"]["properties"]["costCenter"] exists
6. Assert: schema["properties"]["metadata"]["properties"]["businessUnit"] exists
`,

		Rationale: `
Why this behavior exists:
- Extensibility: Organizations need custom schema fields for their specific needs
- Multi-tenancy: Multiple namespaces enable schema isolation per organization
- Versioning: Custom version strings enable independent evolution
- Schema composition: Custom schemas can extend base schemas

Real-world use cases:
- Custom metadata: costCenter, businessUnit, complianceLevel
- Organization-specific fields: budget, approver, tags
- Industry-specific extensions: PCI compliance, HIPAA requirements
- Environment-specific fields: development, staging, production distinctions

Why load order matters:
- Built-in schemas provide foundation
- Custom schemas extend functionality
- No overwrites: custom schemas use different namespaces/versions
`,

		RegressionRisk: `
HIGH RISK if changed without care:
- Custom schema loading is critical for enterprise deployments
- Breaking manifest format breaks all custom schema deployments
- Namespace/version collision handling must be robust

MEDIUM RISK:
- Custom schema validation depends on proper loading
- Schema extension patterns rely on property merging behavior
- Multi-organization setups depend on namespace isolation

What breaks if this changes:
1. Custom schemas fail to load → organizations can't use custom fields
2. Namespace collision → schemas overwrite each other
3. Version handling breaks → wrong schema versions applied
4. Property merging breaks → custom fields lost
5. Multi-tenant isolation breaks → security/compliance issues
`,
	}

	// Execute the behavioral test
	t.Run("Load custom schemas with extended properties", func(t *testing.T) {
		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get current directory: %v", err)
		}

		projectRoot := filepath.Join(currentDir, "..", "..")
		schemaPath := filepath.Join(projectRoot, "assets", "schemas")

		// Create a schema store
		store := NewSchemaStore()
		provider := NewExternalSchemaProvider(schemaPath, store)

		// Load built-in schemas first
		builtinGitopsPath := schemaPath
		err = provider.LoadSchemasFromPaths([]string{builtinGitopsPath}, true)
		if err != nil {
			t.Fatalf("Failed to load schemas: %v", err)
		}

		// Load custom schemas from the actual schemas directory
		gitopsPath := filepath.Join(projectRoot, "schemas")
		err = provider.LoadSchemasFromPaths([]string{gitopsPath}, false)
		if err != nil {
			t.Fatalf("Failed to load custom schemas: %v", err)
		}

		// Check if custom schema was loaded using the store
		// Note: The custom schema has namespace "custom-org" (from custom-org-manifest.json)
		schema, err := store.GetSchema("custom-org", "1.0.0-custom", "DesiredState")
		if err != nil {
			t.Errorf("Failed to get custom schema: %v", err)
		}

		if schema == nil {
			t.Error("Custom schema should not be nil")
		}

		// Check custom schema properties
		properties, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Error("Custom schema should have properties")
		}

		metadata, ok := properties["metadata"].(map[string]interface{})
		if !ok {
			t.Error("Custom schema should have metadata")
		}

		metadataProps, ok := metadata["properties"].(map[string]interface{})
		if !ok {
			t.Error("Metadata should have properties")
		}

		// Check for custom fields
		if _, exists := metadataProps["costCenter"]; !exists {
			t.Error("Custom schema should have costCenter field")
		}

		if _, exists := metadataProps["businessUnit"]; !exists {
			t.Error("Custom schema should have businessUnit field")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 5: ERROR HANDLING - INVALID MANIFEST VERSION
// =============================================================================

func TestExternalSchemaProvider_InvalidManifestVersion_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ExternalSchemaProvider rejects manifests with unsupported manifestVersion field",

		CurrentImpl: `
Go: internal/schema/external.go (loadManifest validation)

func (esp *ExternalSchemaProvider) loadManifest(manifestPath string,
    basePath string) error {

    // Parse manifest
    manifest := parseJSON(manifestPath)

    // Validate manifestVersion
    manifestVersion := manifest["manifestVersion"]
    if manifestVersion != "1.0.0" {
        return fmt.Errorf("unsupported manifest version '%s'", manifestVersion)
    }

    // Continue with schema loading...
}

Key features:
- Validates manifestVersion field before processing
- Only supports manifestVersion "1.0.0" currently
- Returns descriptive error with unsupported version
- Prevents loading incompatible manifest formats
- Fail-fast behavior: errors immediately on version mismatch
`,

		ExpectedOutcome: `
- MUST return error when manifestVersion != "1.0.0"
- MUST include unsupported version in error message
- MUST error message contain "unsupported manifest version"
- MUST prevent schema loading when version is invalid
- SHOULD fail fast (check version before parsing schemas)
- MAY support additional versions in future (1.1.0, 2.0.0)
`,

		TestScenario: `
GIVEN:
  - Test manifest with manifestVersion "2.0.0" (unsupported)
  - Manifest located in testdata/invalid-version-manifest.json

WHEN: loadManifest() is called with this manifest

THEN:
  1. Error returned (err != nil)
  2. Error message contains "unsupported manifest version '2.0.0'"
  3. No schemas loaded into store
  4. Provider state unchanged

Test implementation:
1. Create temp directory with test manifest
2. Copy testdata/invalid-version-manifest.json to temp dir
3. Create provider: NewExternalSchemaProvider(tempDir, store)
4. Call: err := provider.loadManifest(manifestPath, testdataDir)
5. Assert: err != nil
6. Assert: err.Error() contains "unsupported manifest version '2.0.0'"
`,

		Rationale: `
Why this behavior exists:
- Version control: Manifest format may change over time
- Backward compatibility: Prevent loading incompatible manifest formats
- Forward compatibility: Newer yago versions may require newer manifest formats
- Error prevention: Fail fast rather than parse incorrectly
- User feedback: Clear error message helps users upgrade manifests

Version evolution:
- Current: Only 1.0.0 supported
- Future: May support 1.1.0 (backward compatible additions)
- Future: May support 2.0.0 (breaking changes)
- Migration path: Manifest format converter tool may be needed
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Version validation is critical for format compatibility
- Removing validation could allow incompatible manifests to load
- Error message format affects user experience

LOW RISK:
- Adding support for new versions (1.1.0) is low risk
- Backward compatibility can be maintained

What breaks if this changes:
1. Unsupported manifests load incorrectly → parse errors
2. Missing version check → runtime errors during schema processing
3. Poor error messages → users can't diagnose issues
4. Forward compatibility lost → old yago can't reject new manifests
`,
	}

	// Execute the behavioral test
	t.Run("Reject manifest with unsupported version", func(t *testing.T) {
		// Create a temporary directory for test
		tempDir := t.TempDir()

		// Create testdata directory if it doesn't exist
		testdataDir := filepath.Join(tempDir, "testdata")
		err := os.MkdirAll(testdataDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create testdata directory: %v", err)
		}

		// Copy invalid manifest to temp directory
		currentDir, _ := os.Getwd()
		srcManifest := filepath.Join(currentDir, "testdata", "invalid-version-manifest.json")
		dstManifest := filepath.Join(testdataDir, "invalid-manifest.json")

		data, err := os.ReadFile(srcManifest)
		if err != nil {
			t.Fatalf("Failed to read test manifest: %v", err)
		}

		err = os.WriteFile(dstManifest, data, 0644)
		if err != nil {
			t.Fatalf("Failed to write test manifest: %v", err)
		}

		// Try to load the manifest - should fail
		store := NewSchemaStore()
		provider := NewExternalSchemaProvider(tempDir, store)

		err = provider.loadManifest(dstManifest, testdataDir)
		if err == nil {
			t.Error("Expected error when loading manifest with unsupported version, got none")
		}

		expectedMsg := "unsupported manifest version '2.0.0'"
		if err != nil && !stringContains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, err.Error())
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// PHASE 6: ERROR HANDLING - MISSING NAMESPACE
// =============================================================================

func TestExternalSchemaProvider_MissingNamespaceField_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ExternalSchemaProvider rejects manifests missing required 'namespace' field",

		CurrentImpl: `
Go: internal/schema/external.go (loadManifest validation)

func (esp *ExternalSchemaProvider) loadManifest(manifestPath string,
    basePath string) error {

    // Parse manifest
    manifest := parseJSON(manifestPath)

    // Validate namespace field exists
    namespace, ok := manifest["namespace"].(string)
    if !ok || namespace == "" {
        return fmt.Errorf("manifest.namespace is required")
    }

    // Continue with schema loading...
}

Key features:
- Validates namespace field exists and is non-empty string
- Namespace is required for schema registration (key: namespace:version:type)
- Returns descriptive error: "manifest.namespace is required"
- Fail-fast behavior: errors before attempting schema loads
- Type checking: namespace must be string (not int, bool, etc.)
`,

		ExpectedOutcome: `
- MUST return error when "namespace" field is missing
- MUST return error when namespace is empty string
- MUST error message contain "manifest.namespace is required"
- MUST prevent schema loading when namespace is invalid
- SHOULD fail fast (validate before schema processing)
- SHOULD handle various invalid cases (null, wrong type, etc.)
`,

		TestScenario: `
GIVEN:
  - Test manifest missing "namespace" field
  - Manifest located in testdata/missing-namespace-manifest.json

WHEN: loadManifest() is called with this manifest

THEN:
  1. Error returned (err != nil)
  2. Error message contains "manifest.namespace is required"
  3. No schemas loaded into store
  4. Provider state unchanged

Test implementation:
1. Create temp directory with test manifest
2. Copy testdata/missing-namespace-manifest.json to temp dir
3. Create provider: NewExternalSchemaProvider(tempDir, store)
4. Call: err := provider.loadManifest(manifestPath, testdataDir)
5. Assert: err != nil
6. Assert: err.Error() contains "manifest.namespace is required"
`,

		Rationale: `
Why this behavior exists:
- Namespace is required: Schemas are keyed by (namespace, version, type)
- Multi-tenancy: Different organizations use different namespaces
- Collision prevention: Namespace prevents schema overwrites
- Error prevention: Fail fast rather than register with invalid key
- User guidance: Clear error helps users fix manifest

Namespace usage:
- Built-in schemas: namespace = "yago"
- Custom schemas: namespace = "custom-org", "acme-corp", etc.
- Schema isolation: Different namespaces = different schemas
- Key structure: "namespace:version:type" (e.g., "yago:1.0.0:DesiredState")
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Namespace validation is critical for schema registration
- Missing validation could cause runtime errors or panics
- Schema store depends on valid namespace keys

LOW RISK:
- Adding better validation (e.g., namespace format rules) is low risk
- Improving error messages is safe

What breaks if this changes:
1. Missing namespace → schema registration fails with cryptic error
2. Empty namespace → schemas overwrite each other
3. Invalid namespace format → key parsing errors later
4. Poor error message → users can't diagnose and fix manifests
`,
	}

	// Execute the behavioral test
	t.Run("Reject manifest missing namespace field", func(t *testing.T) {
		// Create a temporary directory for test
		tempDir := t.TempDir()

		// Create testdata directory
		testdataDir := filepath.Join(tempDir, "testdata")
		err := os.MkdirAll(testdataDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create testdata directory: %v", err)
		}

		// Copy invalid manifest to temp directory
		currentDir, _ := os.Getwd()
		srcManifest := filepath.Join(currentDir, "testdata", "missing-namespace-manifest.json")
		dstManifest := filepath.Join(testdataDir, "missing-namespace-manifest.json")

		data, err := os.ReadFile(srcManifest)
		if err != nil {
			t.Fatalf("Failed to read test manifest: %v", err)
		}

		err = os.WriteFile(dstManifest, data, 0644)
		if err != nil {
			t.Fatalf("Failed to write test manifest: %v", err)
		}

		// Try to load the manifest - should fail
		store := NewSchemaStore()
		provider := NewExternalSchemaProvider(tempDir, store)

		err = provider.loadManifest(dstManifest, testdataDir)
		if err == nil {
			t.Error("Expected error when loading manifest with missing namespace, got none")
		}

		expectedMsg := "manifest.namespace is required"
		if err != nil && !stringContains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, err.Error())
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// stringContains checks if a string contains a substring (avoids naming conflict)
func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && stringContainsHelper(s, substr)))
}

func stringContainsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
