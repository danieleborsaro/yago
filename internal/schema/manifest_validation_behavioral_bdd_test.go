package schema

import (
	"strings"
	"testing"
)

// =============================================================================
// TIME-BASED BDD: Manifest Validation Behavioral Contracts
// =============================================================================
// These tests document the CURRENT behavior of manifest validation at time T.
// They serve as:
// 1. Characterization tests (Michael Feathers, "Working Effectively with Legacy Code")
// 2. Regression detection for refactoring safety
// 3. Behavioral specification derived from working code
//
// Pattern: GoBehavioralContract
// Migrated from: manifest_validation_test.go (274 lines, 9 test functions)
// Migration Date: October 13, 2025
// =============================================================================

// =============================================================================
// PHASE 1: MANIFEST VERSION VALIDATION
// =============================================================================

func TestValidateManifestVersion_Valid_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "validateManifestVersion accepts manifest version 1.0.0 as the only supported version",

		CurrentImpl: `
Go: internal/schema/validation.go (validateManifestVersion)

func validateManifestVersion(manifest *SchemaManifest) error {
    version := manifest.Manifest.Version

    // Check version is not empty
    if version == "" {
        return errors.New("manifest version is required")
    }

    // Only version 1.0.0 is currently supported
    if version != "1.0.0" {
        return fmt.Errorf("unsupported manifest version '%s': only version 1.0.0 is currently supported", version)
    }

    return nil
}

Key features:
- Single supported version: "1.0.0"
- Rejects empty/missing version
- Rejects any other version (future or past)
- Returns descriptive error with actual version
- Fail-fast validation before processing schemas
`,

		ExpectedOutcome: `
- MUST accept version "1.0.0" without error
- MUST return nil for valid manifest with version "1.0.0"
- MUST be called before manifest content validation
- SHOULD be first validation check (version controls format)
`,

		TestScenario: `
GIVEN: SchemaManifest with valid structure
  - Manifest.Name: "Test Manifest"
  - Manifest.Version: "1.0.0"
  - Manifest.Namespace: "test"
  - Schemas: [] (can be empty for version check)

WHEN: validateManifestVersion(manifest) is called

THEN:
  - Returns nil (no error)
  - Validation passes
  - Manifest can proceed to content validation

Test implementation:
1. Create SchemaManifest with version "1.0.0"
2. Call validateManifestVersion(manifest)
3. Assert err == nil
4. Verify no error message
`,

		Rationale: `
Why this behavior exists:
- Version control: Manifest format may evolve over time
- Format stability: v1.0.0 defines current manifest structure
- Forward compatibility: Future yago versions may support v1.1.0, v2.0.0
- Backward compatibility: Older manifests require migration
- Fail-fast principle: Detect version mismatch before parsing

Manifest format defined by version 1.0.0:
- manifest.name: string (required)
- manifest.namespace: string (required)
- manifest.version: "1.0.0" (required)
- manifest.description: string (optional)
- schemas: array of SchemaEntry (can be empty)

Version evolution strategy:
- v1.0.0: Current stable format
- v1.1.0: Potential minor additions (backward compatible)
- v2.0.0: Breaking changes (requires manifest migration)
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Adding support for new versions (1.1.0) is safe with proper validation
- Removing version check would allow incompatible manifests → parse errors
- Changing version string format breaks all existing manifests

LOW RISK:
- This is a simple string comparison
- No complex logic or side effects
- Easy to test and verify

What breaks if this changes:
1. Removing "1.0.0" requirement → wrong format manifests load → runtime errors
2. Accepting any version → future manifests load in old yago → undefined behavior
3. Poor error messages → users can't diagnose version issues
`,
	}

	// Execute the behavioral test
	t.Run("Accept valid manifest version 1.0.0", func(t *testing.T) {
		manifest := &SchemaManifest{
			Manifest: SchemaManifestInfo{
				Name:        "Test Manifest",
				Description: "Test",
				Version:     "1.0.0",
				Namespace:   "test",
			},
			Schemas: []SchemaEntry{},
		}

		err := validateManifestVersion(manifest)
		if err != nil {
			t.Errorf("Expected no error for valid manifest version 1.0.0, got: %v", err)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// PHASE 2: MANIFEST VERSION VALIDATION - MISSING VERSION
// =============================================================================

func TestValidateManifestVersion_Missing_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "validateManifestVersion rejects manifests with missing or empty version field",

		CurrentImpl: `
Go: internal/schema/validation.go (validateManifestVersion)

func validateManifestVersion(manifest *SchemaManifest) error {
    version := manifest.Manifest.Version

    // Check version is not empty - FIRST check
    if version == "" {
        return errors.New("manifest version is required")
    }

    // Version validation continues...
}

Key features:
- First validation: checks for empty/missing version before value check
- Treats empty string as missing (Go zero value)
- Simple error message: "manifest version is required"
- Prevents further processing of unversioned manifests
`,

		ExpectedOutcome: `
- MUST return error when Version field is empty string ("")
- MUST error message contain "manifest version is required"
- MUST fail before checking version value (1.0.0, 2.0.0, etc.)
- SHOULD be the first validation check
`,

		TestScenario: `
GIVEN: SchemaManifest with missing version
  - Manifest.Version: "" (empty string)
  - Other fields valid

WHEN: validateManifestVersion(manifest) is called

THEN:
  - Returns error (err != nil)
  - Error message contains "manifest version is required"
  - No further validation performed

Test implementation:
1. Create manifest with Version: ""
2. Call validateManifestVersion(manifest)
3. Assert err != nil
4. Assert error contains "manifest version is required"
`,

		Rationale: `
Why this behavior exists:
- Version is mandatory: All manifests must declare their format version
- Format identification: Version tells yago how to parse manifest
- Error clarity: "required" is clearer than "unsupported version ''"
- User guidance: Missing field error helps users fix manifests

Common causes of missing version:
- Hand-written manifest without version field
- Manifest generated by old tooling
- Copy-paste error from template
- JSON parsing issue (field renamed)
`,

		RegressionRisk: `
LOW RISK if changed:
- Simple required field check
- No complex logic
- Clear error message

MEDIUM RISK:
- Removing this check would allow unversioned manifests → unclear format
- Changed error message affects user experience

What breaks if this changes:
1. No required check → unversioned manifests proceed → "unsupported version ''" error (confusing)
2. Different error message → automation/scripts that parse errors break
`,
	}

	// Execute the behavioral test
	t.Run("Reject manifest with missing version", func(t *testing.T) {
		manifest := &SchemaManifest{
			Manifest: SchemaManifestInfo{
				Name:        "Test Manifest",
				Description: "Test",
				Version:     "", // Missing version
				Namespace:   "test",
			},
			Schemas: []SchemaEntry{},
		}

		err := validateManifestVersion(manifest)
		if err == nil {
			t.Error("Expected error for missing manifest version, got none")
		}

		expectedMsg := "manifest version is required"
		if err != nil && !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, err.Error())
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// PHASE 3: MANIFEST VERSION VALIDATION - UNSUPPORTED VERSIONS
// =============================================================================

func TestValidateManifestVersion_Unsupported_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "validateManifestVersion rejects any manifest version other than 1.0.0 (past, future, or modified versions)",

		CurrentImpl: `
Go: internal/schema/validation.go (validateManifestVersion)

func validateManifestVersion(manifest *SchemaManifest) error {
    version := manifest.Manifest.Version

    if version == "" {
        return errors.New("manifest version is required")
    }

    // Exact string match required - ONLY "1.0.0" accepted
    if version != "1.0.0" {
        return fmt.Errorf("unsupported manifest version '%s': only version 1.0.0 is currently supported", version)
    }

    return nil
}

Key features:
- Strict equality check: version != "1.0.0"
- No semantic version parsing (1.0.0 != 1.0 != v1.0.0)
- Rejects ALL other versions:
  - Future: 2.0.0, 1.1.0, 1.0.1
  - Past: 0.9.0
  - Malformed: v1.0.0, 1.0, 1
- Error includes actual version for debugging
- Suggests migration: "only version 1.0.0 is currently supported"
`,

		ExpectedOutcome: `
- MUST reject version "2.0.0" (future major version)
- MUST reject version "1.1.0" (future minor version)
- MUST reject version "1.0.1" (future patch version)
- MUST reject version "0.9.0" (past version)
- MUST error message contain "unsupported manifest version"
- MUST error message include the actual version string
- SHOULD suggest supported version in error message
`,

		TestScenario: `
GIVEN: Multiple test cases with unsupported versions
  - "2.0.0" (future major)
  - "0.9.0" (old version)
  - "1.1.0" (minor update)
  - "1.0.1" (patch update)

WHEN: validateManifestVersion(manifest) is called for each

THEN:
  - All return error (err != nil)
  - All errors contain "unsupported manifest version"
  - Error message includes actual version string
  - No manifest is accepted

Test implementation:
1. Define test cases with various unsupported versions
2. For each version:
   - Create manifest with that version
   - Call validateManifestVersion(manifest)
   - Assert err != nil
   - Assert error contains "unsupported manifest version"
`,

		Rationale: `
Why this behavior exists:
- Format control: Different versions = different manifest structures
- Migration enforcement: Forces users to update manifests for new yago
- Backward compatibility: Old yago can't parse new manifest formats
- Forward compatibility: New yago can add support for new versions
- Clear versioning: Semantic versioning for manifest format evolution

Version rejection strategy:
- Future versions (2.0.0, 1.1.0): May have new required fields → parse errors
- Past versions (0.9.0): May lack required fields → validation errors
- Patch versions (1.0.1): Strict policy, even patches require manifest updates
- Malformed (v1.0.0, 1.0): Prevents ambiguity and parsing issues

Real-world scenarios:
- User downloads new manifest from docs (v2.0.0) → old yago rejects it
- Old manifest (v0.9.0) → new yago rejects it, user must migrate
- Typo in manifest (1.1.0 instead of 1.0.0) → clear error message
`,

		RegressionRisk: `
HIGH RISK if changed without care:
- Adding version support requires careful validation of new format
- Removing version check → incompatible manifests load → runtime errors
- Version string format changes break all existing manifests

MEDIUM RISK:
- Error message changes affect user experience and automation
- Version comparison logic must be exact (no fuzzy matching)

What breaks if this changes:
1. Accept any version → future manifests on old yago → undefined behavior
2. Semantic version parsing (1.0 == 1.0.0) → ambiguity in manifest format
3. Accept past versions (0.9.0) → missing required fields → crashes
4. Poor error messages → users can't understand what version to use
`,
	}

	// Execute the behavioral test
	t.Run("Reject all unsupported versions", func(t *testing.T) {
		testCases := []struct {
			version string
			name    string
		}{
			{"2.0.0", "future version"},
			{"0.9.0", "old version"},
			{"1.1.0", "minor update"},
			{"1.0.1", "patch update"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				manifest := &SchemaManifest{
					Manifest: SchemaManifestInfo{
						Name:        "Test Manifest",
						Description: "Test",
						Version:     tc.version,
						Namespace:   "test",
					},
					Schemas: []SchemaEntry{},
				}

				err := validateManifestVersion(manifest)
				if err == nil {
					t.Errorf("Expected error for unsupported manifest version %s, got none", tc.version)
				}

				expectedMsg := "unsupported manifest version"
				if err != nil && !strings.Contains(err.Error(), expectedMsg) {
					t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, err.Error())
				}
			})
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 4: MANIFEST CONTENT VALIDATION - VALID CONTENT
// =============================================================================

func TestValidateManifestContent_Valid_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "validateManifestContent validates manifest metadata and schema entries for completeness and correctness",

		CurrentImpl: `
Go: internal/schema/validation.go (validateManifestContent)

func validateManifestContent(manifest *SchemaManifest) error {
    // Validate manifest info
    if manifest.Manifest.Name == "" {
        return errors.New("manifest.name is required in manifest v1.0.0")
    }

    if manifest.Manifest.Namespace == "" {
        return errors.New("manifest.namespace is required in manifest v1.0.0")
    }

    // Validate each schema entry
    for i, schema := range manifest.Schemas {
        if err := validateSchemaEntry(&schema, i); err != nil {
            return err
        }
    }

    return nil
}

Key features:
- Validates manifest metadata (name, namespace)
- Validates each schema entry in array
- Required fields enforced
- Returns first error encountered (fail-fast)
- Descriptive error messages with field names
`,

		ExpectedOutcome: `
- MUST accept manifest with valid name and namespace
- MUST accept manifest with valid schema entries
- MUST validate all required fields
- MUST return nil for completely valid manifest
- SHOULD validate entries in order (fail on first error)
`,

		TestScenario: `
GIVEN: Complete valid manifest
  - Manifest.Name: "Test Manifest"
  - Manifest.Namespace: "test-provider"
  - Manifest.Version: "1.0.0"
  - Schemas: [valid SchemaEntry]
    - Version: "1.0.0"
    - Kind: "DesiredState"
    - File: "v1.0.0-desiredstate.json"

WHEN: validateManifestContent(manifest) is called

THEN:
  - Returns nil (no error)
  - All validations pass
  - Manifest ready for schema loading

Test implementation:
1. Create complete valid manifest with all fields
2. Include valid schema entry
3. Call validateManifestContent(manifest)
4. Assert err == nil
`,

		Rationale: `
Why this behavior exists:
- Data integrity: Ensure manifest has all required information
- Schema loading: Metadata needed for schema registration
- Error prevention: Catch incomplete manifests before processing
- User guidance: Clear field names in errors help users fix manifests

Required manifest fields (v1.0.0):
- name: Human-readable manifest name
- namespace: Unique identifier for schema set (yago, custom-org, etc.)
- version: Manifest format version (always "1.0.0")
- description: Optional human-readable description

Schema entry validation:
- version: Schema version (1.0.0, 2.0.0, etc.)
- kind: Schema type (DesiredState, Configuration)
- file: Relative path to schema JSON file
- description: Optional schema description
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Required field validation is critical for schema loading
- Missing validation → incomplete manifests → runtime errors
- Adding required fields breaks existing manifests (migration needed)

LOW RISK:
- Validation logic is straightforward
- Easy to test and verify
- Error messages are clear

What breaks if this changes:
1. Missing name validation → schema registration without identifier
2. Missing namespace validation → key generation fails
3. Skipping schema entry validation → invalid schemas load
4. Changed error messages → automation breaks
`,
	}

	// Execute the behavioral test
	t.Run("Accept valid manifest content", func(t *testing.T) {
		manifest := &SchemaManifest{
			Manifest: SchemaManifestInfo{
				Name:        "Test Manifest",
				Description: "Test Description",
				Version:     "1.0.0",
				Namespace:   "test-provider",
			},
			Schemas: []SchemaEntry{
				{
					Version:     "1.0.0",
					Kind:        "DesiredState",
					File:        "v1.0.0-desiredstate.json",
					Description: "Test schema",
				},
			},
		}

		err := validateManifestContent(manifest)
		if err != nil {
			t.Errorf("Expected no error for valid manifest content, got: %v", err)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// PHASE 5: MANIFEST CONTENT VALIDATION - MISSING NAME
// =============================================================================

func TestValidateManifestContent_MissingName_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "validateManifestContent rejects manifests with missing or empty name field",

		CurrentImpl: `
Go: internal/schema/validation.go (validateManifestContent)

func validateManifestContent(manifest *SchemaManifest) error {
    // First validation: check name
    if manifest.Manifest.Name == "" {
        return errors.New("manifest.name is required in manifest v1.0.0")
    }

    // Continue with other validations...
}

Key features:
- First content validation (after version check)
- Treats empty string as missing
- Error message includes manifest version (v1.0.0)
- Simple, clear error: "manifest.name is required"
`,

		ExpectedOutcome: `
- MUST return error when Name field is empty string
- MUST error message contain "manifest.name is required"
- MUST validate before namespace check
- SHOULD mention manifest version in error
`,

		TestScenario: `
GIVEN: Manifest with missing name
  - Manifest.Name: ""
  - Other fields valid

WHEN: validateManifestContent(manifest) is called

THEN:
  - Returns error (err != nil)
  - Error contains "manifest.name is required"
  - No further validation performed

Test implementation:
1. Create manifest with Name: ""
2. Call validateManifestContent(manifest)
3. Assert err != nil
4. Assert error contains "manifest.name is required"
`,

		Rationale: `
Why this behavior exists:
- Identification: Name provides human-readable manifest identifier
- Documentation: Name appears in logs and error messages
- User experience: Name helps users identify which manifest failed
- Required for v1.0.0: Part of manifest format specification

Use cases for name:
- Logging: "Loading manifest 'GitOps Core Schemas'..."
- Errors: "Manifest 'Custom Org Schemas' validation failed"
- Debugging: Identify which of multiple manifests has issues
- Documentation: Reference manifests by name in docs
`,

		RegressionRisk: `
LOW RISK if changed:
- Simple required field check
- No complex logic
- Clear error message

MEDIUM RISK:
- Removing check → logs and errors lack context
- Name used for user-facing messages

What breaks if this changes:
1. No name validation → logs show empty manifest names
2. Error messages less helpful → harder to debug
3. Manifest identification in multi-manifest setups unclear
`,
	}

	// Execute the behavioral test
	t.Run("Reject manifest with missing name", func(t *testing.T) {
		manifest := &SchemaManifest{
			Manifest: SchemaManifestInfo{
				Name:        "", // Missing
				Description: "Test",
				Version:     "1.0.0",
				Namespace:   "test",
			},
			Schemas: []SchemaEntry{},
		}

		err := validateManifestContent(manifest)
		if err == nil {
			t.Error("Expected error for missing manifest.name, got none")
		}

		expectedMsg := "manifest.name is required"
		if err != nil && !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, err.Error())
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// PHASE 6: MANIFEST CONTENT VALIDATION - MISSING NAMESPACE
// =============================================================================

func TestValidateManifestContent_MissingNamespace_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "validateManifestContent rejects manifests with missing or empty namespace field",

		CurrentImpl: `
Go: internal/schema/validation.go (validateManifestContent)

func validateManifestContent(manifest *SchemaManifest) error {
    if manifest.Manifest.Name == "" {
        return errors.New("manifest.name is required in manifest v1.0.0")
    }

    // Second validation: check namespace
    if manifest.Manifest.Namespace == "" {
        return errors.New("manifest.namespace is required in manifest v1.0.0")
    }

    // Continue with schema entries...
}

Key features:
- Second content validation (after name)
- Critical for schema store key generation
- Namespace format: lowercase, hyphen-separated (yago, custom-org)
- Error message includes manifest version
`,

		ExpectedOutcome: `
- MUST return error when Namespace field is empty string
- MUST error message contain "manifest.namespace is required"
- MUST validate after name check but before schema entries
- SHOULD mention manifest version in error
`,

		TestScenario: `
GIVEN: Manifest with missing namespace
  - Manifest.Name: "Test" (valid)
  - Manifest.Namespace: "" (missing)
  - Other fields valid

WHEN: validateManifestContent(manifest) is called

THEN:
  - Returns error (err != nil)
  - Error contains "manifest.namespace is required"
  - Name validation already passed

Test implementation:
1. Create manifest with Namespace: ""
2. Call validateManifestContent(manifest)
3. Assert err != nil
4. Assert error contains "manifest.namespace is required"
`,

		Rationale: `
Why this behavior exists:
- Schema storage: Namespace is part of composite key (namespace:version:kind)
- Multi-tenancy: Different orgs use different namespaces
- Collision prevention: Namespace isolates schema sets
- Required for v1.0.0: Part of manifest format specification

Namespace usage:
- Built-in: "yago" for core schemas
- Custom: "custom-org", "acme-corp" for organization schemas
- Store key: "yago:1.0.0:DesiredState"
- Isolation: Different namespaces = different schema sets

This field was validated in external_test.go as well, showing its criticality.
`,

		RegressionRisk: `
HIGH RISK if changed:
- Namespace required for schema store keys
- Missing namespace → schema registration fails
- Namespace isolation critical for multi-tenancy

MEDIUM RISK:
- Error message affects user experience
- Validation order matters (after name, before schemas)

What breaks if this changes:
1. No namespace validation → key generation fails → panic
2. Empty namespace → schema overwrites
3. Namespace collision → security/isolation issues
`,
	}

	// Execute the behavioral test
	t.Run("Reject manifest with missing namespace", func(t *testing.T) {
		manifest := &SchemaManifest{
			Manifest: SchemaManifestInfo{
				Name:        "Test",
				Description: "Test",
				Version:     "1.0.0",
				Namespace:   "", // Missing
			},
			Schemas: []SchemaEntry{},
		}

		err := validateManifestContent(manifest)
		if err == nil {
			t.Error("Expected error for missing manifest.namespace, got none")
		}

		expectedMsg := "manifest.namespace is required"
		if err != nil && !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, err.Error())
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 7: MANIFEST CONTENT VALIDATION - INVALID SCHEMA ENTRIES
// =============================================================================

func TestValidateManifestContent_InvalidSchemaEntry_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "validateManifestContent validates each schema entry for required fields (version, kind, file) and valid kind values",

		CurrentImpl: `
Go: internal/schema/validation.go (validateSchemaEntry)

func validateSchemaEntry(entry *SchemaEntry, index int) error {
    // Check required fields
    if entry.Version == "" {
        return fmt.Errorf("schema entry [%d]: missing required field 'version'", index)
    }

    if entry.Kind == "" {
        return fmt.Errorf("schema entry [%d]: missing required field 'kind'", index)
    }

    if entry.File == "" {
        return fmt.Errorf("schema entry [%d]: missing required field 'file'", index)
    }

    // Validate kind value
    validKinds := []string{"DesiredState", "Configuration"}
    if !strings.Contains(validKinds, entry.Kind) {
        return fmt.Errorf("schema entry [%d]: invalid kind '%s', must be one of: %v",
            index, entry.Kind, validKinds)
    }

    return nil
}

Key features:
- Validates THREE required fields: version, kind, file
- Validates kind against allowed values: ["DesiredState", "Configuration"]
- Error messages include entry index for multi-schema manifests
- Fail-fast: returns on first error
- Clear error format: "schema entry [0]: missing required field 'version'"
`,

		ExpectedOutcome: `
- MUST reject entry with missing version field
- MUST reject entry with missing kind field
- MUST reject entry with missing file field
- MUST reject entry with invalid kind value
- MUST only accept kind values: "DesiredState" or "Configuration"
- MUST include entry index in error message
- SHOULD validate in order: version → kind → file → kind value
`,

		TestScenario: `
GIVEN: Multiple test cases with invalid schema entries
  - Missing version: Version = ""
  - Missing kind: Kind = ""
  - Missing file: File = ""
  - Invalid kind: Kind = "InvalidKind"

WHEN: validateManifestContent(manifest) is called for each

THEN:
  - All return error (err != nil)
  - Error message contains specific field name
  - Error message indicates what's wrong
  - For invalid kind: suggests valid values

Test implementation:
1. Define test cases with various invalid entries
2. For each invalid entry:
   - Create manifest with that entry
   - Call validateManifestContent(manifest)
   - Assert err != nil
   - Assert error contains expected message
`,

		Rationale: `
Why this behavior exists:
- Schema loading: All three fields required to load schema file
  - version: Which version of schema (1.0.0, 2.0.0, etc.)
  - kind: Which type (DesiredState or Configuration)
  - file: Where to find schema JSON
- Store registration: version + kind form part of lookup key
- Type safety: Only two kinds supported (DesiredState, Configuration)
- User guidance: Clear error messages with field names

Schema entry structure:
{
  "version": "1.0.0",           // Required: schema version
  "kind": "DesiredState",        // Required: DesiredState | Configuration
  "file": "v1.0.0-ds.json",     // Required: relative path to schema
  "description": "Optional"      // Optional: human-readable description
}

Valid kind values:
- "DesiredState": For desired state documents
- "Configuration": For configuration documents
- No other values accepted (strict validation)
`,

		RegressionRisk: `
HIGH RISK if changed:
- Required field validation critical for schema loading
- Missing fields → file not found, key generation fails
- Kind validation prevents loading unsupported schema types

MEDIUM RISK:
- Adding new kind values requires schema system updates
- Error message format affects automation/parsing
- Entry index helps debug multi-schema manifests

What breaks if this changes:
1. Missing version validation → can't determine schema version
2. Missing kind validation → can't generate store key
3. Missing file validation → file not found errors at load time
4. Accept invalid kinds → schema type confusion
5. No index in error → hard to debug which entry failed
`,
	}

	// Execute the behavioral test
	t.Run("Reject schema entries with invalid fields", func(t *testing.T) {
		testCases := []struct {
			name          string
			schema        SchemaEntry
			expectedError string
		}{
			{
				name: "missing version",
				schema: SchemaEntry{
					Version:     "",
					Kind:        "DesiredState",
					File:        "test.json",
					Description: "Test",
				},
				expectedError: "missing required field 'version'",
			},
			{
				name: "missing kind",
				schema: SchemaEntry{
					Version:     "1.0.0",
					Kind:        "",
					File:        "test.json",
					Description: "Test",
				},
				expectedError: "missing required field 'kind'",
			},
			{
				name: "missing file",
				schema: SchemaEntry{
					Version:     "1.0.0",
					Kind:        "DesiredState",
					File:        "",
					Description: "Test",
				},
				expectedError: "missing required field 'file'",
			},
			{
				name: "invalid kind",
				schema: SchemaEntry{
					Version:     "1.0.0",
					Kind:        "InvalidKind",
					File:        "test.json",
					Description: "Test",
				},
				expectedError: "invalid kind 'InvalidKind'",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				manifest := &SchemaManifest{
					Manifest: SchemaManifestInfo{
						Name:        "Test",
						Description: "Test",
						Version:     "1.0.0",
						Namespace:   "test",
					},
					Schemas: []SchemaEntry{tc.schema},
				}

				err := validateManifestContent(manifest)
				if err == nil {
					t.Errorf("Expected error for %s, got none", tc.name)
				}

				if err != nil && !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected error message to contain '%s', got: %s", tc.expectedError, err.Error())
				}
			})
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 8: MANIFEST CONTENT VALIDATION - EMPTY SCHEMAS
// =============================================================================

func TestValidateManifestContent_EmptySchemas_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "validateManifestContent accepts manifests with empty schemas array (warning only, not error)",

		CurrentImpl: `
Go: internal/schema/validation.go (validateManifestContent)

func validateManifestContent(manifest *SchemaManifest) error {
    // Validate manifest info (name, namespace)
    // ... validations ...

    // Validate schema entries
    for i, schema := range manifest.Schemas {
        if err := validateSchemaEntry(&schema, i); err != nil {
            return err
        }
    }

    // Empty schemas: loop never executes, returns nil
    // Note: May log warning but doesn't return error

    return nil
}

Key features:
- Empty schemas array is VALID (not an error)
- Loop doesn't execute for empty array
- May log warning for user awareness
- Manifest loads successfully
- Use case: Manifest placeholder or future schemas
`,

		ExpectedOutcome: `
- MUST accept manifest with empty schemas array
- MUST return nil (no error)
- SHOULD log warning for user awareness (implementation dependent)
- MAY be used for manifest placeholders
`,

		TestScenario: `
GIVEN: Manifest with empty schemas array
  - Manifest.Name: "Test"
  - Manifest.Namespace: "test"
  - Schemas: [] (empty array)

WHEN: validateManifestContent(manifest) is called

THEN:
  - Returns nil (no error)
  - Validation passes
  - Manifest loads successfully
  - May see warning in logs (not enforced by test)

Test implementation:
1. Create manifest with Schemas: []
2. Call validateManifestContent(manifest)
3. Assert err == nil
4. Verify validation passes
`,

		Rationale: `
Why this behavior exists:
- Flexibility: Allows manifest creation before schemas are ready
- Placeholder: Reserve namespace while schemas are developed
- Incremental development: Add manifest first, schemas later
- No harm: Empty schemas array doesn't break anything
- User awareness: Warning (not error) informs but doesn't block

Use cases:
- New organization: Create manifest, add schemas incrementally
- Testing: Manifest structure validation without schemas
- Placeholder: Reserve namespace for future use
- Migration: Empty manifest while migrating schemas

Design decision: Warning vs Error
- Warning: "Manifest has no schemas (empty array)"
- Not error: Empty array is technically valid JSON
- User informed: Logs show awareness of empty state
- Not blocking: Allows incremental manifest creation
`,

		RegressionRisk: `
LOW RISK if changed:
- Empty schemas is an edge case
- No functional impact (no schemas to load)
- Changing to error would break placeholder manifests

VERY LOW RISK:
- Adding warning doesn't break anything
- Removing warning loses user awareness

What breaks if this changes:
1. Error on empty schemas → can't create placeholder manifests
2. Block incremental development → worse UX
3. Testing manifests require dummy schemas → complexity
`,
	}

	// Execute the behavioral test
	t.Run("Accept manifest with empty schemas array", func(t *testing.T) {
		manifest := &SchemaManifest{
			Manifest: SchemaManifestInfo{
				Name:        "Test",
				Description: "Test",
				Version:     "1.0.0",
				Namespace:   "test",
			},
			Schemas: []SchemaEntry{}, // Empty schemas (should warn but not fail)
		}

		err := validateManifestContent(manifest)
		if err != nil {
			t.Errorf("Expected no error for empty schemas list (should only warn), got: %v", err)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// PHASE 9: MANIFEST CONTENT VALIDATION - MULTIPLE VALID SCHEMAS
// =============================================================================

func TestValidateManifestContent_MultipleValidSchemas_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "validateManifestContent validates all schema entries in array, accepting manifests with multiple valid schemas",

		CurrentImpl: `
Go: internal/schema/validation.go (validateManifestContent)

func validateManifestContent(manifest *SchemaManifest) error {
    // ... manifest info validation ...

    // Iterate through ALL schema entries
    for i, schema := range manifest.Schemas {
        if err := validateSchemaEntry(&schema, i); err != nil {
            return err  // Fail-fast on first error
        }
    }

    // All entries valid: return nil
    return nil
}

Key features:
- Validates ALL entries in array
- Fail-fast: stops on first error
- Supports multiple schemas per manifest:
  - Multiple versions (1.0.0, 2.0.0, 3.0.0)
  - Multiple kinds per version (DesiredState, Configuration)
  - Different files for each schema
- Entry index used for error reporting
- All entries must be valid for success
`,

		ExpectedOutcome: `
- MUST validate all schema entries in array
- MUST accept manifest with multiple valid entries
- MUST validate entries in order
- MUST stop on first invalid entry (fail-fast)
- SHOULD support common patterns:
  - Same version, different kinds
  - Different versions, same kind
  - Mix of versions and kinds
`,

		TestScenario: `
GIVEN: Manifest with multiple valid schema entries
  - Entry 1: version=1.0.0, kind=DesiredState
  - Entry 2: version=1.0.0, kind=Configuration
  - Entry 3: version=2.0.0, kind=DesiredState

WHEN: validateManifestContent(manifest) is called

THEN:
  - All three entries validated
  - All validations pass
  - Returns nil (no error)
  - Manifest ready for schema loading

Test implementation:
1. Create manifest with 3 valid schema entries
2. Different combinations of version/kind
3. Call validateManifestContent(manifest)
4. Assert err == nil
5. Verify all entries accepted
`,

		Rationale: `
Why this behavior exists:
- Multi-version support: Manifests typically have multiple schema versions
- Multi-kind support: Each version has DesiredState + Configuration
- Common pattern: Manifest contains version history
- Schema evolution: New versions added while old versions remain
- Complete validation: All entries must be valid for successful load

Typical manifest structure:
{
  "manifest": {...},
  "schemas": [
    {"version": "1.0.0", "kind": "DesiredState", "file": "v1-ds.json"},
    {"version": "1.0.0", "kind": "Configuration", "file": "v1-cfg.json"},
    {"version": "2.0.0", "kind": "DesiredState", "file": "v2-ds.json"},
    {"version": "2.0.0", "kind": "Configuration", "file": "v2-cfg.json"}
  ]
}

Real-world example (yago manifest):
- Multiple versions: 1.0.0, 2.0.0, 3.0.0, ..., 4.0.0
- Two kinds per version: DesiredState, Configuration
- Total: 2N schemas for N versions
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Multi-schema support is core functionality
- Breaking validation loop → only first schema validated
- Changed validation order → different error behavior

LOW RISK:
- Logic is straightforward iteration
- Easy to test and verify
- Well-defined success criteria

What breaks if this changes:
1. Skip validation loop → invalid entries load → runtime errors
2. Stop after first entry → rest of manifest not validated
3. Change validation order → different failure modes
4. No fail-fast → multiple errors confusing (current is better)
`,
	}

	// Execute the behavioral test
	t.Run("Accept manifest with multiple valid schemas", func(t *testing.T) {
		manifest := &SchemaManifest{
			Manifest: SchemaManifestInfo{
				Name:        "Test",
				Description: "Test",
				Version:     "1.0.0",
				Namespace:   "test",
			},
			Schemas: []SchemaEntry{
				{
					Version:     "1.0.0",
					Kind:        "DesiredState",
					File:        "v1.0.0-desiredstate.json",
					Description: "Test schema 1",
				},
				{
					Version:     "1.0.0",
					Kind:        "Configuration",
					File:        "v1.0.0-configuration.json",
					Description: "Test schema 2",
				},
				{
					Version:     "2.0.0",
					Kind:        "DesiredState",
					File:        "v2.0.0-desiredstate.json",
					Description: "Test schema 3",
				},
			},
		}

		err := validateManifestContent(manifest)
		if err != nil {
			t.Errorf("Expected no error for multiple valid schemas, got: %v", err)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}
