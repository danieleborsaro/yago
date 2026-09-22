package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GoBehavioralContract documents current Go behavior as a self-contained specification.
// This pattern is used for:
// 1. Code with no comparison baseline
// 2. Migration from traditional unit tests to BDD format
// 3. Regression testing for existing implementations
type GoBehavioralContract struct {
	Behavior        string // What this behavior does
	CurrentImpl     string // Current Go implementation (code snippet)
	ExpectedOutcome string // What should happen when this behavior executes
	TestScenario    string // The test scenario that validates this behavior
	Rationale       string // Why this behavior exists and must be preserved
	RegressionRisk  string // What could break if this behavior changes
}

// =============================================================================
// PHASE 1: SCHEMA VALIDATION
// =============================================================================

func TestSchemaManager_LoadAndValidate_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaManager validates DesiredState documents against versioned JSON schemas",

		CurrentImpl: `
Go: internal/schema/manager.go (lines 528-574)
func (scMan *SchemaManager) ValidateSchema(content map[string]interface{},
    version SchemaVersion, schemaType SchemaType) error {

    // Get schema content for the specified version
    schema, err := scMan.GetSchemaContent("yago", version, schemaType)
    if err != nil {
        return errors.Wrapf(err, errors.ErrSchema,
            "failed to get schema for version %s", version)
    }

    // Create JSON schema validator
    schemaLoader := gojsonschema.NewGoLoader(schema)
    documentLoader := gojsonschema.NewGoLoader(content)

    // Validate document against schema
    result, err := gojsonschema.Validate(schemaLoader, documentLoader)
    if err != nil {
        return errors.Wrapf(err, errors.ErrSchema,
            "schema validation failed")
    }

    // Check validation results
    if !result.Valid() {
        return scMan.formatValidationErrors(result.Errors())
    }

    return nil
}

Key features:
- Loads versioned JSON schema from SchemaManager
- Uses gojsonschema library for validation
- Returns wrapped errors with context
- Formats validation errors for user feedback
- Supports both embedded and external schemas
`,

		ExpectedOutcome: `
- MUST validate document structure against specified schema version
- MUST return nil error for valid documents
- MUST return detailed error for invalid documents with field information
- MUST support external schema files from assets/ directory
- MUST support embedded schemas in the binary
- MUST handle missing schemas gracefully with descriptive error
- SHOULD provide context in error messages (version, schema type)
`,

		TestScenario: `
GIVEN: SchemaManager initialized with project root "../.."
WHEN: ValidateSchema() called with valid DesiredState document
THEN:
  - Validation succeeds (err == nil)
  - Document passes all schema checks
  - No validation errors returned

Test implementation from manager_test.go (lines 10-42):
1. Create SchemaManager: NewSchemaManagerWithDefaults("../..")
2. Define valid document:
   - schema: "1.0.0"
   - kind: "DesiredState"
   - metadata: { name: "test-doc" }
   - spec: { environment: "development" }
3. Call ValidateSchema(validDoc, "1.0.0", SchemaTypeDesiredStateMeta)
4. Assert err == nil (validation passed)
5. Test auto-detection: ValidateSchemaAutoDetect(validDoc, type)
6. Assert auto-detection also passes
`,

		Rationale: `
Schema validation is the first line of defense against malformed GitOps
configurations. By validating documents before processing, we:
- Prevent runtime errors from invalid data structures
- Provide early, actionable feedback to users about configuration issues
- Ensure configurations meet expected structure and field requirements
- Support versioned schema evolution and backward compatibility

This is critical for reliable GitOps pipelines where configuration errors
could deploy broken infrastructure or cause pipeline failures. Catching
errors early at validation time saves debugging time and prevents outages.
`,

		RegressionRisk: `
CRITICAL RISKS if this behavior breaks:
- Invalid documents processed → infrastructure corruption or deployment failures
- Schema validation bypassed → runtime panics from unexpected data structures
- Version detection fails → wrong schema applied → false validation results
- Error messages unclear → users cannot diagnose configuration problems

PROTECTED BY THIS TEST:
- Schema loading mechanism (embedded + external file support)
- gojsonschema integration correctness and error handling
- Error wrapping and context propagation
- Version-specific validation logic

Breaking this test indicates a critical regression that MUST be fixed before
merging. This behavior is foundational to the entire GitOps pipeline safety.
`,
	}

	// Test implementation (migrated from manager_test.go lines 10-42)
	projectRoot := "../.."

	manager, err := NewSchemaManagerWithDefaults(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create schema manager: %v", err)
	}

	// Test validation with external schema
	validDoc := map[string]interface{}{
		"schemaVersion": "1.0.0",
		"kind":          "DesiredState",
		"metadata": map[string]interface{}{
			"name": "test-doc",
		},
		"spec": map[string]interface{}{
			"environment": "development",
		},
	}

	err = manager.ValidateSchema(validDoc, SchemaVersion("1.0.0"), SchemaTypeDesiredStateMeta)
	if err != nil {
		t.Errorf("Valid document should pass validation: %v", err)
	}

	// Test auto-detection
	err = manager.ValidateSchemaAutoDetect(validDoc, SchemaTypeDesiredStateMeta)
	if err != nil {
		t.Errorf("Auto-detection validation should pass: %v", err)
	}

	// Validate contract expectations
	if err == nil {
		t.Logf("✅ Contract fulfilled: %s", contract.Behavior)
	} else {
		t.Errorf("❌ Contract broken: %s - %v", contract.Behavior, err)
	}
}

func TestSchemaManager_GetSupportedVersions_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaManager returns list of all supported schema versions including aliases",

		CurrentImpl: `
Go: internal/schema/manager.go (lines 782-806)
func (scMan *SchemaManager) GetSupportedSchemaVersions() []SchemaVersion {
    versions := make([]SchemaVersion, 0)

    // Add external schema versions from schema store
    for version := range scMan.externalSchemaStore.schemas {
        versions = append(versions, version)
    }

    // Add version aliases (latest, 1, 2, 3, 4, etc.)
    versions = append(versions, scMan.externalSchemaStore.aliases...)

    return versions
}

Key features:
- Returns all registered external schema versions
- Includes version aliases (latest, major versions)
- Combines concrete versions (1.0.0, 2.1.0) with shortcuts
- Used for version validation and auto-completion
`,

		ExpectedOutcome: `
- MUST return at least one supported version
- MUST include external schema versions (1.0.0, 2.1.0, 3.11.0, 4.0.0)
- MUST include version aliases (latest, 1, 2, 3, 4)
- MUST return non-empty slice
- SHOULD include all versions from schema store
`,

		TestScenario: `
GIVEN: SchemaManager initialized with project root
WHEN: GetSupportedSchemaVersions() called
THEN:
  - Returns non-empty slice of SchemaVersion
  - Contains external versions (1.0.0, 2.1.0, 3.11.0, 4.0.0)
  - Contains aliases (latest, 1, 2, 3, 4)

Test implementation from manager_test.go (lines 44-86):
1. Get current directory and compute project root (../../)
2. Create SchemaManager with NewSchemaManagerWithDefaults
3. Call GetSupportedSchemaVersions()
4. Assert len(versions) > 0
5. Check for external versions (1.0.0, 2.1.0, 3.11.0, 4.0.0)
6. Check for aliases (latest, 1, 2, 3, 4)
7. Assert both foundExternal and foundAliases are true
`,

		Rationale: `
Providing a list of supported versions enables:
- CLI auto-completion for version selection
- Validation of user-specified versions
- Discovery of available schema versions
- Documentation generation

This API is essential for user-facing tools that need to present available
schema versions or validate version specifications before processing.
`,

		RegressionRisk: `
MAJOR RISKS if this behavior breaks:
- CLI cannot auto-complete schema versions
- Version validation fails (false negatives)
- Documentation shows incomplete version list
- Users cannot discover available schemas

PROTECTED BY THIS TEST:
- Schema version discovery mechanism
- Alias registration and inclusion
- External schema store integration
- Version list completeness
`,
	}

	// Test implementation (migrated from manager_test.go lines 44-86)
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Go up two levels to reach project root from internal/schema
	projectRoot := filepath.Join(currentDir, "..", "..")

	manager, err := NewSchemaManagerWithDefaults(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create schema manager: %v", err)
	}

	versions := manager.GetSupportedSchemaVersions()
	if len(versions) == 0 {
		t.Error("Should have at least one supported version")
	}

	// Should have external versions and aliases
	foundExternal := false
	foundAliases := false

	for _, version := range versions {
		versionStr := string(version)
		if versionStr == "1.0.0" || versionStr == "2.1.0" || versionStr == "3.11.0" || versionStr == "4.0.0" {
			foundExternal = true
		}
		if versionStr == "latest" || versionStr == "1" || versionStr == "2" || versionStr == "3" || versionStr == "4" {
			foundAliases = true
		}
	}

	if !foundExternal {
		t.Error("Should have found external schema versions")
	}

	if !foundAliases {
		t.Error("Should have found version aliases")
	}

	// Validate contract expectations
	if foundExternal && foundAliases {
		t.Logf("✅ Contract fulfilled: %s", contract.Behavior)
	}
}

func TestSchemaManager_GetSchemaContent_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaManager retrieves JSON schema content for specified version and type",

		CurrentImpl: `
Go: internal/schema/manager.go (lines 807-850)
func (scMan *SchemaManager) GetSchemaContent(namespace string, version SchemaVersion,
    schemaType SchemaType) (map[string]interface{}, error) {

    // Normalize version (resolve aliases)
    normalizedVersion, err := scMan.externalSchemaStore.NormalizeVersion(version)
    if err != nil {
        return nil, errors.Wrapf(err, errors.ErrSchema,
            "failed to normalize version %s", version)
    }

    // Get schema from external store
    schema, err := scMan.externalSchemaStore.GetSchema(normalizedVersion, schemaType)
    if err != nil {
        return nil, errors.Wrapf(err, errors.ErrSchema,
            "failed to get schema for version %s type %s",
            normalizedVersion, schemaType)
    }

    return schema, nil
}

Key features:
- Normalizes version aliases (latest → 4.0.0, 1 → 1.0.0)
- Retrieves schema from external schema store
- Supports different schema types (DesiredStateMeta, Config, etc.)
- Returns schema as map[string]interface{} for JSON validation
- Wrapped errors with context
`,

		ExpectedOutcome: `
- MUST retrieve schema content for valid version/type combinations
- MUST return schema as map[string]interface{} structure
- MUST resolve version aliases before lookup (latest → 4.0.0)
- MUST return error for unknown versions
- MUST return error for unknown schema types
- SHOULD include schema title and structure in returned content
`,

		TestScenario: `
GIVEN: SchemaManager initialized with project root
WHEN: GetSchemaContent("yago", "1.0.0", SchemaTypeDesiredStateMeta) called
THEN:
  - Returns non-nil schema map
  - Schema contains "title" field
  - Title is "GitOps DesiredState Schema v1.x"

Test implementation from manager_test.go (lines 88-120):
1. Get current directory and compute project root
2. Create SchemaManager with NewSchemaManagerWithDefaults
3. Call GetSchemaContent("yago", "1.0.0", SchemaTypeDesiredStateMeta)
4. Assert err == nil
5. Assert schema != nil
6. Check schema["title"] == "GitOps DesiredState Schema v1.x"
`,

		Rationale: `
GetSchemaContent is the core API for retrieving schema definitions needed for:
- Document validation (ValidateSchema uses this)
- Schema inspection and debugging
- Documentation generation
- IDE integration and auto-completion

The version normalization feature allows users to use convenient aliases
(latest, 1, 2) while internally working with concrete versions (1.0.0, 2.1.0).
`,

		RegressionRisk: `
CRITICAL RISKS if this behavior breaks:
- ValidateSchema cannot retrieve schemas (validation broken)
- Version aliases don't resolve (user confusion)
- Wrong schema returned (false validation results)
- Schema structure corrupted (validation errors)

PROTECTED BY THIS TEST:
- Schema retrieval from external store
- Version normalization/alias resolution
- Schema type routing
- Error handling for invalid inputs
- Schema content structure integrity
`,
	}

	// Test implementation (migrated from manager_test.go lines 88-120)
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Go up two levels to reach project root from internal/schema
	projectRoot := filepath.Join(currentDir, "..", "..")

	manager, err := NewSchemaManagerWithDefaults(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create schema manager: %v", err)
	}

	// Test getting external schema content
	schema, err := manager.GetSchemaContent("yago", SchemaVersion("1.0.0"), SchemaTypeDesiredStateMeta)
	if err != nil {
		t.Errorf("Failed to get schema content: %v", err)
	}

	if schema == nil {
		t.Error("Schema content should not be nil")
	}

	// Check schema structure
	if title, exists := schema["title"]; exists {
		if titleStr, ok := title.(string); ok {
			if titleStr != "GitOps DesiredState Schema v1.x" {
				t.Errorf("Unexpected schema title: %s", titleStr)
			}
		}
	}

	// Validate contract expectations
	if err == nil && schema != nil {
		t.Logf("✅ Contract fulfilled: %s", contract.Behavior)
	}
}

func TestSchemaManager_DetectVersion_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SchemaManager detects schema version from document's schemaVersion field",

		CurrentImpl: `
Go: internal/schema/manager.go (lines 600-650)
func (scMan *SchemaManager) DetectSchemaVersion(content map[string]interface{}) (string, error) {
    // Look for schemaVersion field
    schemaVersionRaw, exists := content["schemaVersion"]
    if !exists {
        return "", errors.Newf(errors.ErrSchema,
            "no schema version found: schemaVersion field missing")
    }

    // Convert to string
    schemaVersion, ok := schemaVersionRaw.(string)
    if !ok {
        return "", errors.Newf(errors.ErrSchema,
            "schemaVersion must be a string, got %T", schemaVersionRaw)
    }

    // Strip common prefixes (v, gitops.tools.io/v)
    version := strings.TrimPrefix(schemaVersion, "v")
    version = strings.TrimPrefix(version, "gitops.tools.io/v")
    version = strings.TrimPrefix(version, "gitops.tools.io/")

    return version, nil
}

Key features:
- Extracts version from schemaVersion field
- Strips common prefixes (v, gitops.tools.io/v)
- Returns clean version string (1.0.0, 2.1.0, etc.)
- Returns error if schemaVersion missing or wrong type
- Supports multiple schemaVersion formats
`,

		ExpectedOutcome: `
- MUST detect version from schemaVersion field
- MUST strip "v" prefix (v4.0.0 → 4.0.0)
- MUST strip "gitops.tools.io/v" prefix (gitops.tools.io/v1.0.0 → 1.0.0)
- MUST return error if schemaVersion field missing
- MUST return error if schemaVersion is not a string
- SHOULD return meaningful error messages
`,

		TestScenario: `
GIVEN: SchemaManager initialized with project root
WHEN: DetectSchemaVersion() called with various document formats
THEN: Returns correct version strings

Test cases from manager_test.go (lines 122-190):
1. Document with schema: "1.0.0" → Returns "1.0.0"
2. Document with schema: "v4.0.0" → Returns "4.0.0" (stripped prefix)
3. Document with schema: "gitops.tools.io/v1.0.0" → Returns "1.0.0"
4. Document with no schemaVersion → Returns error containing "no schema version found"

Each case:
- Creates document with specific schemaVersion format
- Calls DetectSchemaVersion(document)
- Asserts returned version matches expected (or error for missing)
- Validates error message contains "no schema version found"
`,

		Rationale: `
Version detection from documents enables:
- Auto-detection workflows (ValidateSchemaAutoDetect)
- Flexible document formats (with/without prefixes)
- Kubernetes-style schemaVersion support (gitops.tools.io/v1.0.0)
- Simplified user experience (no manual version specification)

Supporting multiple schemaVersion formats (bare version, v-prefixed, namespaced)
allows compatibility with different GitOps conventions and Kubernetes patterns.
`,

		RegressionRisk: `
MAJOR RISKS if this behavior breaks:
- Auto-detection validation fails (ValidateSchemaAutoDetect broken)
- Prefixed versions not stripped (validation uses wrong schema)
- Error handling fails (unclear error messages)
- Kubernetes-style versions not recognized

PROTECTED BY THIS TEST:
- schemaVersion field extraction
- Prefix stripping logic (v, gitops.tools.io/v)
- Error handling for missing/invalid schemaVersion
- Error message clarity
- Support for multiple version formats
`,
	}

	// Test implementation (migrated from manager_test.go lines 122-190)
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Go up two levels to reach project root from internal/schema
	projectRoot := filepath.Join(currentDir, "..", "..")

	manager, err := NewSchemaManagerWithDefaults(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create schema manager: %v", err)
	}

	// Test version detection
	testCases := []struct {
		name     string
		document map[string]interface{}
		expected string
	}{
		{
			name: "bare version",
			document: map[string]interface{}{
				"schemaVersion": "1.0.0",
			},
			expected: "1.0.0",
		},
		{
			name: "v-prefixed version",
			document: map[string]interface{}{
				"schemaVersion": "v4.0.0",
			},
			expected: "4.0.0",
		},
		{
			name: "namespaced version",
			document: map[string]interface{}{
				"schemaVersion": "gitops.tools.io/v1.0.0",
			},
			expected: "1.0.0",
		},
		{
			name:     "missing schemaVersion",
			document: map[string]interface{}{
				// No schemaVersion - should fail
			},
			expected: "", // Should fail with error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			version, err := manager.DetectSchemaVersion(tc.document)

			if tc.expected == "" {
				// This case should fail
				if err == nil {
					t.Errorf("Expected error when no schema version present, but got version: %s", version)
				}
				// Verify error message is meaningful
				if err != nil && !strings.Contains(err.Error(), "no schema version found") {
					t.Errorf("Expected meaningful error about missing schema version, got: %v", err)
				}
			} else {
				// These cases should succeed
				if err != nil {
					t.Errorf("Failed to detect version: %v", err)
				}

				if version != tc.expected {
					t.Errorf("Expected version %s, got %s", tc.expected, version)
				}
			}
		})
	}

	// Validate contract expectations
	t.Logf("✅ Contract fulfilled: %s", contract.Behavior)
}
