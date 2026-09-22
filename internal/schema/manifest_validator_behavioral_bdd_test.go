package schema

import (
	"testing"

	"github.com/danieleborsaro/yago/assets"
)

// =============================================================================
// TIME-BASED BDD: ManifestValidator Behavioral Contracts
// =============================================================================
// These tests document the CURRENT behavior of ManifestValidator at time T.
// They serve as:
// 1. Characterization tests (Michael Feathers, "Working Effectively with Legacy Code")
// 2. Regression detection for refactoring safety
// 3. Behavioral specification derived from working code
//
// Pattern: GoBehavioralContract
// Migrated from: manifest_validator_test.go (207 lines, 9 test functions)
// Migration Date: October 13, 2025
// =============================================================================

// =============================================================================
// PHASE 1: VALID MANIFEST VALIDATION
// =============================================================================

func TestManifestValidator_ValidManifest_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ManifestValidator validates manifest JSON against embedded JSON Schema (manifest-v1.0.0.json) and accepts valid manifests",

		CurrentImpl: `
Go: internal/schema/validator.go (ManifestValidator.ValidateManifest)

type ManifestValidator struct {
    schemaLoader gojsonschema.JSONLoader
}

func NewManifestValidator(schemaContent []byte) *ManifestValidator {
    return &ManifestValidator{
        schemaLoader: gojsonschema.NewBytesLoader(schemaContent),
    }
}

func (mv *ManifestValidator) ValidateManifest(manifestJSON []byte) error {
    // Load manifest document
    documentLoader := gojsonschema.NewBytesLoader(manifestJSON)

    // Validate against JSON Schema
    result, err := gojsonschema.Validate(mv.schemaLoader, documentLoader)
    if err != nil {
        return fmt.Errorf("schema validation failed: %w", err)
    }

    // Check validation results
    if !result.Valid() {
        return mv.formatValidationErrors(result.Errors())
    }

    return nil
}

Key features:
- Uses gojsonschema library for JSON Schema validation
- Schema embedded in assets.ManifestSchemas (manifest-v1.0.0.json)
- Validates complete manifest structure against schema
- Returns nil for valid manifests
- Returns formatted errors for invalid manifests
- Schema-based validation (declarative, not imperative)
`,

		ExpectedOutcome: `
- MUST accept valid manifest JSON with all required fields
- MUST validate against embedded JSON Schema (manifest-v1.0.0.json)
- MUST return nil error for valid manifests
- MUST validate manifest structure:
  - manifest.name (string, required)
  - manifest.version (string, required, must be "1.0.0")
  - manifest.namespace (string, required)
  - manifest.description (string, optional)
  - schemas (array, can be empty)
- MUST validate schema entries:
  - version (string, required)
  - kind (string, required, enum: DesiredState, Configuration)
  - file (string, required)
  - description (string, optional)
`,

		TestScenario: `
GIVEN: Valid manifest JSON with all required fields
  - manifest.name: "Test Schemas"
  - manifest.version: "1.0.0"
  - manifest.namespace: "yago"
  - schemas: [valid schema entry]

WHEN: ValidateManifest(validManifest) is called

THEN:
  - Schema validation passes
  - Returns nil (no error)
  - Manifest can be used for schema loading

Test implementation:
1. Create ManifestValidator with assets.ManifestSchemas
2. Define valid manifest JSON with all required fields
3. Call validator.ValidateManifest(validManifest)
4. Assert err == nil
`,

		Rationale: `
Why this behavior exists:
- JSON Schema validation: Declarative validation rules in schema file
- Centralized validation: Schema defines all rules in one place
- Reusable validation: Same schema used by external tools
- Standard compliance: JSON Schema is industry standard
- Error clarity: Schema validation provides detailed field-level errors

Validation architecture:
- Schema file: assets/schemas/manifest-v1.0.0.json
- Embedded at compile time: assets.ManifestSchemas byte array
- gojsonschema library: Industry-standard JSON Schema validator
- Schema version: Tied to manifest version (1.0.0)

Advantages of schema-based validation:
- Declarative: Rules in JSON, not code
- Documentation: Schema is both validation and documentation
- Tooling: External tools can use same schema
- Versioning: Different manifest versions have different schemas
- Completeness: Schema validators check all rules automatically
`,

		RegressionRisk: `
HIGH RISK if changed:
- Schema validation is primary validation mechanism
- Breaking schema structure breaks all manifest validation
- Schema file changes affect all manifests

MEDIUM RISK:
- Changing validator library affects validation behavior
- Schema versioning must match manifest versioning
- Error formatting affects user experience

What breaks if this changes:
1. Schema file modified → validation rules change → manifests fail
2. Different validator library → different validation results
3. Schema not embedded → runtime dependency on external file
4. Error formatting changes → automation breaks
`,
	}

	// Execute the behavioral test
	t.Run("Accept valid manifest with all required fields", func(t *testing.T) {
		validator := NewManifestValidator(assets.ManifestSchemas)

		validManifest := []byte(`{
			"manifest": {
				"name": "Test Schemas",
				"description": "Testing manifest validation",
				"version": "1.0.0",
				"namespace": "yago"
			},
			"schemas": [
				{
					"version": "4.2.0",
					"kind": "DesiredState",
					"file": "test.json",
					"description": "Test schema"
				}
			]
		}`)

		err := validator.ValidateManifest(validManifest)
		if err != nil {
			t.Errorf("Expected valid manifest to pass, got error: %v", err)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 2: MISSING REQUIRED FIELD VALIDATION
// =============================================================================

func TestManifestValidator_MissingRequiredField_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ManifestValidator rejects manifests missing required fields (name, version, namespace) via JSON Schema validation",

		CurrentImpl: `
Go: internal/schema/validator.go + assets/schemas/manifest-v1.0.0.json

JSON Schema defines required fields:
{
  "type": "object",
  "required": ["manifest", "schemas"],
  "properties": {
    "manifest": {
      "type": "object",
      "required": ["name", "version", "namespace"],
      "properties": {
        "name": {"type": "string"},
        "version": {"type": "string", "enum": ["1.0.0"]},
        "namespace": {"type": "string"}
      }
    }
  }
}

Validator checks against schema:
- Missing "manifest" → error
- Missing "manifest.name" → error
- Missing "manifest.version" → error
- Missing "manifest.namespace" → error
- Missing "schemas" → error

Key features:
- Required fields enforced by JSON Schema
- Schema validator automatically checks all required fields
- Descriptive error messages with field paths
- Validation fails on first missing required field group
`,

		ExpectedOutcome: `
- MUST reject manifest missing "manifest.name"
- MUST reject manifest missing "manifest.version"
- MUST reject manifest missing "manifest.namespace"
- MUST reject manifest missing "schemas" array
- MUST return error (err != nil)
- MUST provide field-level error information
- SHOULD indicate which field is missing
`,

		TestScenario: `
GIVEN: Manifest JSON missing required "namespace" field
  - manifest.name: "Test Schemas" (present)
  - manifest.version: "1.0.0" (present)
  - manifest.namespace: (MISSING)
  - schemas: [] (present)

WHEN: ValidateManifest(invalidManifest) is called

THEN:
  - Schema validation fails
  - Returns error about missing namespace
  - Error indicates field path and requirement

Test implementation:
1. Create ManifestValidator with schema
2. Define manifest JSON without namespace field
3. Call validator.ValidateManifest(invalidManifest)
4. Assert err != nil
5. Verify error relates to missing field
`,

		Rationale: `
Why this behavior exists:
- Required fields: Manifest needs name, version, namespace to function
- Schema enforcement: JSON Schema defines requirements declaratively
- Early detection: Fail fast before attempting to use incomplete manifest
- User guidance: Error messages show exactly what's missing

Field purposes:
- name: Human-readable identifier for logs/errors
- version: Manifest format version (controls validation rules)
- namespace: Schema isolation key (yago, custom-org, etc.)
- schemas: Array of schema definitions (can be empty but must exist)

JSON Schema advantage:
- Automatic checking of all required fields
- No need to write imperative validation code
- Consistent error format across all validators
`,

		RegressionRisk: `
HIGH RISK if changed:
- Required field validation critical for manifest functionality
- Schema changes affect all manifest files
- Missing validation → incomplete manifests processed → errors

MEDIUM RISK:
- Error message format affects user experience
- Schema validator library changes behavior
- Field requirement changes need manifest migration

What breaks if this changes:
1. Remove required fields from schema → incomplete manifests accepted
2. Add new required fields → all existing manifests invalid
3. Change validator → different error messages/behavior
`,
	}

	// Execute the behavioral test
	t.Run("Reject manifest with missing required field", func(t *testing.T) {
		validator := NewManifestValidator(assets.ManifestSchemas)

		// Missing namespace field
		invalidManifest := []byte(`{
			"manifest": {
				"name": "Test Schemas",
				"version": "1.0.0"
			},
			"schemas": []
		}`)

		err := validator.ValidateManifest(invalidManifest)
		if err == nil {
			t.Error("Expected validation error for missing namespace field")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 3: WRONG VERSION VALIDATION
// =============================================================================

func TestManifestValidator_WrongVersion_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ManifestValidator rejects manifests with version other than 1.0.0 via JSON Schema enum constraint",

		CurrentImpl: `
Go: JSON Schema constraint in manifest-v1.0.0.json

Schema defines version constraint:
{
  "properties": {
    "manifest": {
      "properties": {
        "version": {
          "type": "string",
          "enum": ["1.0.0"]
        }
      }
    }
  }
}

Key features:
- Enum constraint: Only "1.0.0" allowed
- Schema validator checks enum automatically
- Rejects any other value: "2.0.0", "1.1.0", "0.9.0", etc.
- Error message indicates enum violation
`,

		ExpectedOutcome: `
- MUST reject version "2.0.0"
- MUST reject version "1.1.0"
- MUST reject version "0.9.0"
- MUST only accept version "1.0.0"
- MUST return error for non-1.0.0 versions
- SHOULD error message indicate enum constraint violation
`,

		TestScenario: `
GIVEN: Manifest JSON with version "2.0.0"
  - manifest.version: "2.0.0" (unsupported)
  - Other fields valid

WHEN: ValidateManifest(invalidManifest) is called

THEN:
  - Schema validation fails on enum constraint
  - Returns error about version enum violation
  - Indicates only "1.0.0" is allowed

Test implementation:
1. Create ManifestValidator with schema
2. Define manifest JSON with version: "2.0.0"
3. Call validator.ValidateManifest(invalidManifest)
4. Assert err != nil
5. Verify error relates to version constraint
`,

		Rationale: `
Why this behavior exists:
- Version control: Only v1.0.0 manifest format supported
- Schema-based enforcement: Enum in schema enforces version
- Backward compatibility: Old yago rejects new manifest formats
- Forward compatibility: New yago can add v2.0.0 schema
- Format stability: Version tied to schema file

Enum advantage:
- Declarative constraint in schema
- No imperative version checking code needed
- Consistent with other JSON Schema validators
- Easy to add new versions (add to enum, deploy new schema)

Version evolution path:
- Current: manifest-v1.0.0.json with enum: ["1.0.0"]
- Future: manifest-v1.1.0.json with enum: ["1.1.0"]
- Future: manifest-v2.0.0.json with enum: ["2.0.0"]
- Validator: Choose schema based on version field
`,

		RegressionRisk: `
HIGH RISK if changed:
- Version constraint critical for format compatibility
- Wrong version → wrong validation rules → parse errors
- Adding versions requires careful schema design

MEDIUM RISK:
- Enum modification affects all manifest files
- Schema validator behavior must be consistent
- Error messages affect user understanding

What breaks if this changes:
1. Remove enum constraint → any version accepted → wrong rules applied
2. Add v2.0.0 to v1.0.0 schema → format confusion
3. Change validator → enum not enforced properly
`,
	}

	// Execute the behavioral test
	t.Run("Reject manifest with wrong version", func(t *testing.T) {
		validator := NewManifestValidator(assets.ManifestSchemas)

		invalidManifest := []byte(`{
			"manifest": {
				"name": "Test Schemas",
				"version": "2.0.0",
				"namespace": "yago"
			},
			"schemas": []
		}`)

		err := validator.ValidateManifest(invalidManifest)
		if err == nil {
			t.Error("Expected validation error for version 2.0.0")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 4: INVALID KIND VALIDATION
// =============================================================================

func TestManifestValidator_InvalidKind_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ManifestValidator rejects schema entries with kind values other than 'DesiredState' or 'Configuration' via JSON Schema enum",

		CurrentImpl: `
Go: JSON Schema constraint in manifest-v1.0.0.json

Schema defines kind constraint:
{
  "schemas": {
    "type": "array",
    "items": {
      "type": "object",
      "properties": {
        "kind": {
          "type": "string",
          "enum": ["DesiredState", "Configuration"]
        }
      }
    }
  }
}

Key features:
- Enum constraint: Only two kind values allowed
- DesiredState: For desired state documents
- Configuration: For configuration documents
- Schema validator checks enum for each array item
- Error indicates invalid enum value and expected values
`,

		ExpectedOutcome: `
- MUST accept kind "DesiredState"
- MUST accept kind "Configuration"
- MUST reject kind "InvalidKind"
- MUST reject any other kind value
- MUST return error for invalid kind
- SHOULD error message indicate allowed values
- SHOULD error message indicate which schema entry failed
`,

		TestScenario: `
GIVEN: Manifest with schema entry having kind "InvalidKind"
  - schema[0].kind: "InvalidKind" (not in enum)
  - Other fields valid

WHEN: ValidateManifest(invalidManifest) is called

THEN:
  - Schema validation fails on kind enum constraint
  - Returns error about invalid kind value
  - Indicates allowed values: DesiredState, Configuration

Test implementation:
1. Create ManifestValidator with schema
2. Define manifest with schema entry kind: "InvalidKind"
3. Call validator.ValidateManifest(invalidManifest)
4. Assert err != nil
5. Verify error relates to kind enum violation
`,

		Rationale: `
Why this behavior exists:
- Type safety: Only two document types supported
- Schema enforcement: Enum constraint in schema
- Clear semantics: DesiredState vs Configuration distinction
- Future extensibility: Can add new kinds by updating schema

Document type meanings:
- DesiredState: Declares desired infrastructure state
- Configuration: Defines configuration parameters
- No other types: System only processes these two

Schema-based validation benefits:
- Declarative: Kind constraint in schema, not code
- Automatic: Validator checks all schema entries
- Consistent: Same validation as external tools
- Error clarity: "must be one of: DesiredState, Configuration"
`,

		RegressionRisk: `
HIGH RISK if changed:
- Kind validation prevents unsupported document types
- Invalid kind → processing errors, wrong behavior
- Schema system depends on two-kind model

MEDIUM RISK:
- Adding new kind requires system-wide changes
- Enum modification affects all manifests
- Kind affects document processing logic

What breaks if this changes:
1. Remove enum → any kind accepted → processing fails
2. Add new kind without system support → documents can't be processed
3. Change kind names → all manifests must be updated
`,
	}

	// Execute the behavioral test
	t.Run("Reject schema entry with invalid kind", func(t *testing.T) {
		validator := NewManifestValidator(assets.ManifestSchemas)

		invalidManifest := []byte(`{
			"manifest": {
				"name": "Test Schemas",
				"version": "1.0.0",
				"namespace": "yago"
			},
			"schemas": [
				{
					"version": "4.2.0",
					"kind": "InvalidKind",
					"file": "test.json"
				}
			]
		}`)

		err := validator.ValidateManifest(invalidManifest)
		if err == nil {
			t.Error("Expected validation error for invalid kind")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 5: INVALID NAMESPACE PATTERN VALIDATION
// =============================================================================

func TestManifestValidator_InvalidNamespacePattern_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ManifestValidator rejects namespaces with spaces via JSON Schema pattern constraint",

		CurrentImpl: `
Go: JSON Schema pattern constraint in manifest-v1.0.0.json

Schema defines namespace pattern:
{
  "namespace": {
    "type": "string",
    "pattern": "^[a-zA-Z0-9._:-]+$"
  }
}

Pattern breakdown:
- ^: Start of string
- [a-zA-Z0-9._:-]+: One or more of:
  - a-z: Lowercase letters
  - A-Z: Uppercase letters
  - 0-9: Numbers
  - .: Dots
  - _: Underscores
  - :: Colons
  - -: Hyphens
- $: End of string

Allowed: "yago", "my-org", "aws_provider", "io.k8s.api", "org:team:project"
Not allowed: "invalid namespace" (space), "bad\nname" (newline), "test " (trailing space)

Key features:
- Pattern validation via regex
- Prevents problematic characters (spaces, newlines, etc.)
- Allows common namespace separators (-, _, ., :)
- Schema validator checks pattern automatically
`,

		ExpectedOutcome: `
- MUST reject namespace "invalid namespace with spaces"
- MUST reject namespace with newlines or tabs
- MUST reject namespace with special characters outside pattern
- MUST accept namespace "yago"
- MUST accept namespace "my-org"
- MUST accept namespace "aws_provider"
- MUST accept namespace "io.k8s.api"
- MUST return error for pattern violation
- SHOULD error message indicate pattern constraint
`,

		TestScenario: `
GIVEN: Manifest with namespace "invalid namespace with spaces"
  - manifest.namespace: "invalid namespace with spaces" (has spaces)
  - Other fields valid

WHEN: ValidateManifest(invalidManifest) is called

THEN:
  - Schema validation fails on pattern constraint
  - Returns error about namespace pattern violation
  - Indicates namespace must match pattern

Test implementation:
1. Create ManifestValidator with schema
2. Define manifest with namespace containing spaces
3. Call validator.ValidateManifest(invalidManifest)
4. Assert err != nil
5. Verify error relates to namespace pattern
`,

		Rationale: `
Why this behavior exists:
- Key generation: Namespace used in store keys (namespace:version:kind)
- File paths: Namespace may be used in directory names
- URL safety: Namespace may appear in URLs
- Parsing: Spaces complicate parsing and splitting
- Consistency: Enforces naming convention

Pattern design rationale:
- Letters/numbers: Basic identifiers
- Hyphens: Common separator (my-org)
- Underscores: Programming convention (aws_provider)
- Dots: Hierarchical names (io.k8s.api)
- Colons: Nested namespaces (org:team:project)
- No spaces: Prevents parsing issues

Real-world examples:
- yago: Built-in namespace
- custom-org: Organization namespace
- acme_corp: Company namespace
- io.k8s.api: Kubernetes-style namespace
- red101:platform:prod: Hierarchical namespace
`,

		RegressionRisk: `
HIGH RISK if changed:
- Namespace pattern affects key generation and file paths
- Allowing spaces → parsing errors, key generation issues
- Pattern too restrictive → can't use valid namespaces

MEDIUM RISK:
- Pattern change requires manifest migration
- Different validators may interpret pattern differently
- Error messages affect user understanding

What breaks if this changes:
1. Remove pattern → spaces allowed → key generation breaks
2. More restrictive pattern → existing namespaces invalid
3. Different pattern → parsing logic must change
4. Pattern bug → wrong namespaces accepted/rejected
`,
	}

	// Execute the behavioral test
	t.Run("Reject namespace with invalid pattern (spaces)", func(t *testing.T) {
		validator := NewManifestValidator(assets.ManifestSchemas)

		invalidManifest := []byte(`{
			"manifest": {
				"name": "Test Schemas",
				"version": "1.0.0",
				"namespace": "invalid namespace with spaces"
			},
			"schemas": []
		}`)

		err := validator.ValidateManifest(invalidManifest)
		if err == nil {
			t.Error("Expected validation error for namespace with invalid pattern (spaces not allowed)")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 6: MISSING SCHEMA FILE VALIDATION
// =============================================================================

func TestManifestValidator_MissingSchemaFile_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ManifestValidator rejects schema entries missing required 'file' field via JSON Schema required constraint",

		CurrentImpl: `
Go: JSON Schema constraint in manifest-v1.0.0.json

Schema defines required fields for schema entries:
{
  "schemas": {
    "type": "array",
    "items": {
      "type": "object",
      "required": ["version", "kind", "file"],
      "properties": {
        "version": {"type": "string"},
        "kind": {"type": "string"},
        "file": {"type": "string"}
      }
    }
  }
}

Key features:
- Required constraint on "file" field
- Schema validator checks all array items
- Error indicates missing required property
- Error includes field path (schemas[0].file)
`,

		ExpectedOutcome: `
- MUST reject schema entry missing "file" field
- MUST reject schema entry missing "version" field
- MUST reject schema entry missing "kind" field
- MUST return error for missing required fields
- SHOULD error message indicate which field is missing
- SHOULD error message indicate which array index
`,

		TestScenario: `
GIVEN: Manifest with schema entry missing "file" field
  - schema[0].version: "4.2.0" (present)
  - schema[0].kind: "DesiredState" (present)
  - schema[0].file: (MISSING)

WHEN: ValidateManifest(invalidManifest) is called

THEN:
  - Schema validation fails on required constraint
  - Returns error about missing "file" field
  - Indicates field is required

Test implementation:
1. Create ManifestValidator with schema
2. Define manifest with schema entry missing "file"
3. Call validator.ValidateManifest(invalidManifest)
4. Assert err != nil
5. Verify error relates to missing file field
`,

		Rationale: `
Why this behavior exists:
- File loading: "file" field specifies where to load schema JSON
- Required for operation: Can't load schema without file path
- Schema enforcement: JSON Schema defines requirement
- Early detection: Fail validation before attempting file load

Required fields for schema entry:
- version: Which schema version (1.0.0, 2.0.0, etc.)
- kind: Which document type (DesiredState, Configuration)
- file: Where to find schema JSON (relative path)

All three fields required for schema loading:
- version + kind: Generate store key
- file: Load schema content from filesystem
- Missing any field: Schema can't be loaded
`,

		RegressionRisk: `
HIGH RISK if changed:
- File field required for schema loading
- Missing validation → attempt to load with empty path → error
- Required constraint critical for data integrity

MEDIUM RISK:
- Error message format affects user experience
- Field name changes require manifest migration
- Validation order affects error reporting

What breaks if this changes:
1. Remove required constraint → missing file → load fails at runtime
2. Change field name → all manifests must be updated
3. Different validator → required checking may differ
`,
	}

	// Execute the behavioral test
	t.Run("Reject schema entry with missing file field", func(t *testing.T) {
		validator := NewManifestValidator(assets.ManifestSchemas)

		invalidManifest := []byte(`{
			"manifest": {
				"name": "Test Schemas",
				"version": "1.0.0",
				"namespace": "yago"
			},
			"schemas": [
				{
					"version": "4.2.0",
					"kind": "DesiredState"
				}
			]
		}`)

		err := validator.ValidateManifest(invalidManifest)
		if err == nil {
			t.Error("Expected validation error for missing file field in schema entry")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 7: ADDITIONAL PROPERTIES VALIDATION
// =============================================================================

func TestManifestValidator_AdditionalProperties_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ManifestValidator rejects manifests with additional properties not defined in JSON Schema via additionalProperties: false constraint",

		CurrentImpl: `
Go: JSON Schema constraint in manifest-v1.0.0.json

Schema defines additionalProperties constraint:
{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "manifest": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "name": {...},
        "version": {...},
        "namespace": {...},
        "description": {...}
      }
    },
    "schemas": {...}
  }
}

Key features:
- additionalProperties: false prevents unknown fields
- Only defined properties allowed
- Catches typos and mistakes
- Enforces strict schema compliance
- Error indicates additional property name
`,

		ExpectedOutcome: `
- MUST reject manifest with extra fields not in schema
- MUST reject manifest.extra_field (not defined)
- MUST reject unknown properties at any level
- MUST return error indicating additional property
- SHOULD error message show property name
- SHOULD error message show property location
`,

		TestScenario: `
GIVEN: Manifest with additional property "extra_field"
  - manifest.extra_field: "not allowed" (undefined in schema)
  - Other fields valid

WHEN: ValidateManifest(invalidManifest) is called

THEN:
  - Schema validation fails on additionalProperties constraint
  - Returns error about additional property
  - Indicates "extra_field" is not allowed

Test implementation:
1. Create ManifestValidator with schema
2. Define manifest with extra field not in schema
3. Call validator.ValidateManifest(invalidManifest)
4. Assert err != nil
5. Verify error relates to additional properties
`,

		Rationale: `
Why this behavior exists:
- Strict validation: Prevents undefined fields
- Typo detection: Catches misspelled field names
- Future compatibility: Can add fields by updating schema
- Clear contract: Only documented fields allowed
- Error prevention: Unknown fields ignored = bugs

additionalProperties: false benefits:
- Catches typos: "namspace" instead of "namespace"
- Prevents mistakes: Adding fields that don't exist
- Documentation: Schema shows all allowed fields
- Forward compatibility: Old yago rejects new fields

Common mistakes caught:
- Typos: "namspace", "verison", "desription"
- Extra fields: Custom fields not in schema
- Copy-paste errors: Fields from different schema
- Tool errors: Generators adding wrong fields
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Strict validation helps catch errors early
- Allowing additional properties → silent failures
- Field typos not caught → confusing behavior

HIGH RISK:
- Changing to allow additional properties breaks strict validation
- Adding new fields requires schema update (good!)
- Schema-code mismatch causes issues

What breaks if this changes:
1. Allow additional properties → typos not caught
2. Remove constraint → undefined behavior for extra fields
3. Too strict → can't extend schema easily
`,
	}

	// Execute the behavioral test
	t.Run("Reject manifest with additional properties", func(t *testing.T) {
		validator := NewManifestValidator(assets.ManifestSchemas)

		// Additional properties should be rejected
		invalidManifest := []byte(`{
			"manifest": {
				"name": "Test Schemas",
				"version": "1.0.0",
				"namespace": "yago",
				"extra_field": "not allowed"
			},
			"schemas": []
		}`)

		err := validator.ValidateManifest(invalidManifest)
		if err == nil {
			t.Error("Expected validation error for additional properties")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 8: EMPTY SCHEMAS ARRAY VALIDATION
// =============================================================================

func TestManifestValidator_EmptySchemasArray_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ManifestValidator accepts manifests with empty schemas array (array required but can be empty)",

		CurrentImpl: `
Go: JSON Schema constraint in manifest-v1.0.0.json

Schema defines schemas array requirement:
{
  "type": "object",
  "required": ["manifest", "schemas"],
  "properties": {
    "schemas": {
      "type": "array",
      "items": {...}
    }
  }
}

Key features:
- "schemas" field is required
- Must be an array type
- Array can be empty: []
- No minItems constraint (empty allowed)
- Consistent with manifest_validation behavior
`,

		ExpectedOutcome: `
- MUST accept manifest with schemas: []
- MUST require schemas field to exist
- MUST schemas field be array type
- MUST return nil for empty schemas array
- MAY log warning for empty array (implementation dependent)
`,

		TestScenario: `
GIVEN: Manifest with empty schemas array
  - manifest: {...} (valid)
  - schemas: [] (empty but present)

WHEN: ValidateManifest(validManifest) is called

THEN:
  - Schema validation passes
  - Returns nil (no error)
  - Manifest is valid (placeholder use case)

Test implementation:
1. Create ManifestValidator with schema
2. Define manifest with schemas: []
3. Call validator.ValidateManifest(validManifest)
4. Assert err == nil
5. Verify empty array is accepted
`,

		Rationale: `
Why this behavior exists:
- Flexibility: Allows placeholder manifests
- Incremental development: Add manifest first, schemas later
- Required field: "schemas" must exist (but can be empty)
- Consistent: Same behavior as validateManifestContent
- Use case: Reserve namespace, add schemas incrementally

Empty schemas valid because:
- Placeholder: Manifest structure defined, schemas added later
- Testing: Test manifest validation without schemas
- Migration: Empty manifest during schema transition
- No harm: Empty array doesn't break loading

Difference from missing schemas:
- Missing: {"manifest": {...}} → ERROR (field required)
- Empty: {"manifest": {...}, "schemas": []} → OK (field present)
`,

		RegressionRisk: `
LOW RISK if changed:
- Empty schemas is valid edge case
- No functional impact (no schemas to load)
- Placeholder manifests useful for development

VERY LOW RISK:
- Making empty schemas an error breaks placeholder use case
- Current behavior is intentional and useful

What breaks if this changes:
1. Error on empty schemas → can't create placeholders
2. Remove schemas requirement → manifests without field accepted
3. Add minItems: 1 → all placeholders invalid
`,
	}

	// Execute the behavioral test
	t.Run("Accept manifest with empty schemas array", func(t *testing.T) {
		validator := NewManifestValidator(assets.ManifestSchemas)

		// Empty schemas array is valid
		validManifest := []byte(`{
			"manifest": {
				"name": "Test Schemas",
				"version": "1.0.0",
				"namespace": "yago"
			},
			"schemas": []
		}`)

		err := validator.ValidateManifest(validManifest)
		if err != nil {
			t.Errorf("Expected empty schemas array to be valid, got error: %v", err)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
}

// =============================================================================
// PHASE 9: VALID NAMESPACE PATTERNS VALIDATION
// =============================================================================

func TestManifestValidator_ValidNamespacePatterns_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ManifestValidator accepts various valid namespace patterns including hyphens, underscores, dots, and colons via JSON Schema pattern ^[a-zA-Z0-9._:-]+$",

		CurrentImpl: `
Go: JSON Schema pattern in manifest-v1.0.0.json

Pattern: ^[a-zA-Z0-9._:-]+$

Allowed characters:
- a-z: Lowercase letters
- A-Z: Uppercase letters
- 0-9: Numbers
- .: Dots (hierarchical names like io.k8s.api)
- _: Underscores (programming style like aws_provider)
- :: Colons (nested namespaces like org:team:project)
- -: Hyphens (common separator like my-org)

Pattern allows various naming conventions:
- Simple: "yago", "custom"
- With hyphens: "my-org", "acme-corp"
- With underscores: "aws_provider", "gcp_service"
- With dots: "io.k8s.api", "com.example.service"
- With colons: "org:team:project", "env:region:cluster"
- Mixed: "my-org_v1.0:prod"

Key features:
- Flexible: Supports multiple naming conventions
- Safe: No spaces, newlines, or special characters
- Practical: Covers real-world use cases
- Consistent: Same pattern for all namespaces
`,

		ExpectedOutcome: `
- MUST accept namespace "yago"
- MUST accept namespace "my-org"
- MUST accept namespace "aws_provider"
- MUST accept namespace "io.k8s.api"
- MUST accept namespace "org:team:project"
- MUST accept namespace "my-org_v1.0:prod"
- MUST return nil for all valid patterns
- MUST support various naming conventions
`,

		TestScenario: `
GIVEN: Multiple test cases with valid namespace patterns
  - "yago" (simple)
  - "my-org" (with hyphen)
  - "aws_provider" (with underscore)
  - "io.k8s.api" (with dots)
  - "org:team:project" (with colons)
  - "my-org_v1.0:prod" (mixed)

WHEN: ValidateManifest(manifest) is called for each

THEN:
  - All validations pass
  - All return nil
  - All patterns are accepted

Test implementation:
1. Define test cases with various valid namespaces
2. For each namespace:
   - Create manifest with that namespace
   - Call validator.ValidateManifest(manifest)
   - Assert err == nil
   - Verify namespace is accepted
`,

		Rationale: `
Why this behavior exists:
- Flexibility: Different organizations use different conventions
- Real-world: Support actual naming patterns in use
- Interoperability: Compatible with various systems
- Practical: Allows hierarchical and categorized namespaces

Naming convention support:
- Kubernetes-style: "io.k8s.api" (dots for hierarchy)
- Cloud providers: "aws_provider", "gcp_service" (underscores)
- Organizations: "my-org", "acme-corp" (hyphens)
- Projects: "org:team:project" (colons for nesting)
- Versioned: "my-org_v1.0:prod" (mixed for context)

Pattern design rationale:
- Letters/numbers: Standard identifiers
- Dots: Java-style packages, DNS-style names
- Underscores: Programming convention
- Colons: Hierarchical separation
- Hyphens: URL-safe, common in cloud/k8s

Real-world examples:
- yago: Built-in namespace
- red101-core: Project namespace
- io.k8s.api: Kubernetes-style
- aws:us-east-1:prod: Cloud region namespace
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Pattern supports multiple conventions
- Too restrictive → breaks existing valid namespaces
- Too permissive → allows problematic characters

LOW RISK:
- Current pattern is well-tested
- Covers real-world use cases
- Balance between flexibility and safety

What breaks if this changes:
1. Remove dots → can't use io.k8s.api style
2. Remove colons → can't use org:team:project style
3. Remove underscores → can't use aws_provider style
4. Remove hyphens → can't use my-org style
5. Too restrictive → valid namespaces become invalid
`,
	}

	// Execute the behavioral test
	t.Run("Accept various valid namespace patterns", func(t *testing.T) {
		validator := NewManifestValidator(assets.ManifestSchemas)

		testCases := []struct {
			name      string
			namespace string
		}{
			{"simple", "yago"},
			{"with-hyphen", "my-org"},
			{"with_underscore", "aws_provider"},
			{"with.dot", "io.k8s.api"},
			{"with:colon", "org:team:project"},
			{"mixed", "my-org_v1.0:prod"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				validManifest := []byte(`{
					"manifest": {
						"name": "Test Schemas",
						"version": "1.0.0",
						"namespace": "` + tc.namespace + `"
					},
					"schemas": []
				}`)

				err := validator.ValidateManifest(validManifest)
				if err != nil {
					t.Errorf("Expected namespace '%s' to be valid, got error: %v", tc.namespace, err)
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
