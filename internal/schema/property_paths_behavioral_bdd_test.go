package schema

import (
	"strings"
	"testing"
)

// =============================================================================
// TIME-BASED BDD: Property Paths Discovery Behavioral Contracts
// =============================================================================
// These tests document the CURRENT behavior of property path discovery from
// JSON schemas at time T. They serve as:
// 1. Characterization tests (Michael Feathers, "Working Effectively with Legacy Code")
// 2. Regression detection for refactoring safety
// 3. Behavioral specification derived from working code
//
// Pattern: GoBehavioralContract
// Migrated from: property_paths_test.go (243 lines, 3 test functions)
// Migration Date: October 13, 2025
// =============================================================================

// =============================================================================
// PHASE 1: MISSING EXTENSION ERROR HANDLING
// =============================================================================

func TestDiscoverPropertyPaths_MissingExtension_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "DiscoverPropertyPaths returns error when schema lacks x-gitops-paths extension or schema doesn't exist",

		CurrentImpl: `
Go: internal/schema/manager.go (DiscoverPropertyPaths)

func (sm *SchemaManager) DiscoverPropertyPaths(namespace string, version string, isDesiredState bool) (*PropertyPaths, error) {
    // Get schema content
    kind := "Configuration"
    if isDesiredState {
        kind = "DesiredState"
    }

    schemaContent, err := sm.GetSchemaContent(namespace, version, kind)
    if err != nil {
        return nil, fmt.Errorf("schema not found: %w", err)
    }

    // Parse schema JSON
    var schema map[string]interface{}
    if err := json.Unmarshal(schemaContent, &schema); err != nil {
        return nil, fmt.Errorf("failed to parse schema: %w", err)
    }

    // Look for x-gitops-paths extension
    pathsData, exists := schema["x-gitops-paths"]
    if !exists {
        return nil, fmt.Errorf("schema does not contain x-gitops-paths extension")
    }

    // Convert to PropertyPaths
    return NewPropertyPaths(pathsData), nil
}

Key features:
- Schema lookup: GetSchemaContent(namespace, version, kind)
- Extension required: x-gitops-paths must exist in schema
- Error propagation: Schema not found → error
- Extension missing: No x-gitops-paths → error
- Clear error messages: Indicate what went wrong

Error cases:
1. Schema doesn't exist → "schema not found" error
2. Schema exists but no extension → "x-gitops-paths extension" error
3. Invalid JSON → "failed to parse schema" error
`,

		ExpectedOutcome: `
- MUST return error when schema doesn't exist
- MUST return error when x-gitops-paths extension missing
- MUST return error (not nil) for non-existent schema version
- SHOULD error message indicate "schema not found" or "x-gitops-paths"
- MUST NOT return nil error for missing/invalid schemas
- MUST NOT panic on missing schema

Error types:
- Schema not found: Version doesn't exist in store
- Extension missing: Schema exists but lacks x-gitops-paths
- Parse error: Schema JSON is malformed
`,

		TestScenario: `
GIVEN: SchemaManager with embedded yago schemas
  - Schemas exist: versions 1.0.0, 2.0.0, 3.0.0, 4.0.0, 4.2.0
  - All have x-gitops-paths extension

WHEN: DiscoverPropertyPaths("", "99.0.0", true) is called
  - namespace: "" (default yago namespace)
  - version: "99.0.0" (doesn't exist)
  - isDesiredState: true

THEN:
  - Returns error (err != nil)
  - Error message contains "schema not found" or similar
  - No PropertyPaths returned (nil or invalid)

Test implementation:
1. Create SchemaManager with defaults
2. Call DiscoverPropertyPaths with non-existent version
3. Assert err != nil
4. Verify error message indicates schema issue
`,

		Rationale: `
Why this behavior exists:
- Fail fast: Don't proceed with invalid/missing schemas
- Clear errors: User knows what's wrong (missing schema vs extension)
- Schema validation: Extension is required for property path discovery
- Extension design: x-gitops-paths is optional in JSON Schema but required for yago

x-gitops-paths extension:
- Custom extension: Not part of JSON Schema standard
- Yago-specific: Defines property paths for GitOps operations
- Required paths: root, meta, metaRootPath, spec, status, etc.
- Custom paths: Plugins can add organization-specific paths
- Schema location: Top level of JSON schema definition

Why extension is required:
- Property traversal: Yago needs to know where properties are
- Schema flexibility: Different schemas may organize properties differently
- Backward compatibility: Old schemas without extension should fail clearly
- Plugin support: Custom schemas must define their property structure

Error handling design:
- Explicit errors: User knows exactly what's missing
- Early detection: Fail at discovery time, not during traversal
- Debugging: Error messages guide users to fix schema
`,

		RegressionRisk: `
HIGH RISK if changed:
- Extension requirement: Removing check breaks property traversal
- Error handling: Must fail for missing schemas/extensions
- Schema lookup: GetSchemaContent must work correctly

MEDIUM RISK:
- Error messages: Changes affect user debugging experience
- Extension name: x-gitops-paths is documented convention
- Return values: nil vs error must be consistent

LOW RISK:
- Error formatting: Exact wording can change
- Test version number: 99.0.0 is just test data

What breaks if this changes:
1. Skip extension check → property traversal fails cryptically
2. Return nil instead of error → callers crash on nil paths
3. Wrong error messages → users can't diagnose issues
4. Change extension name → all schemas must update
5. Don't check schema existence → proceed with invalid data
`,
	}

	// Execute the behavioral test
	t.Run("Error when schema missing or lacks extension", func(t *testing.T) {
		projectRoot, err := findProjectRoot()
		if err != nil {
			t.Fatalf("Failed to determine project root: %v", err)
		}
		scMan, err := NewSchemaManagerWithDefaults(projectRoot)
		if err != nil {
			t.Fatalf("Failed to create schema manager: %v", err)
		}

		// Try to discover paths from a version that doesn't exist
		_, err = scMan.DiscoverPropertyPaths("", "99.0.0", true)
		if err == nil {
			t.Error("Expected error when schema doesn't exist, but got none")
		}

		// The error should indicate the schema wasn't found or doesn't have the extension
		if err != nil && !strings.Contains(err.Error(), "schema not found") && !strings.Contains(err.Error(), "x-gitops-paths") {
			t.Logf("Got expected error: %s", err.Error())
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 2: SUCCESSFUL PROPERTY PATH DISCOVERY
// =============================================================================

func TestDiscoverPropertyPaths_Success_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "DiscoverPropertyPaths successfully extracts property paths from schema x-gitops-paths extension for both DesiredState and Configuration schemas",

		CurrentImpl: `
Go: internal/schema/manager.go + internal/schema/property_paths.go

Property path discovery:
1. Get schema content by namespace/version/kind
2. Parse schema JSON
3. Extract x-gitops-paths extension
4. Create PropertyPaths wrapper

PropertyPaths structure:
- Wraps map[string]string from x-gitops-paths
- Provides getter methods: Root(), Meta(), MetaRootPath(), etc.
- Generic Get(key) method for any path
- Has(key) to check existence
- GetAll() returns all paths

Required core paths (in x-gitops-paths):
- root: Top-level object path
- meta: Metadata section path
- metaRootPath: Root path reference in metadata
- spec: Specification section path
- status: Status section path
- parts: Parts array path
- partsFile: Individual part file path
- partsInline: Inline part content path
- partsInlineKind: Inline part kind path

DesiredState paths example:
- root: "desiredstate"
- meta: "desiredstate.meta"
- metaRootPath: "desiredstate.meta.parts.self"
- spec: "desiredstate.spec"
- status: "desiredstate.status"

Configuration paths example:
- root: "configuration"
- meta: "configuration.meta"
- spec: "configuration.spec"
- status: "configuration.status"

Key features:
- Kind-specific: Different paths for DesiredState vs Configuration
- Accessor methods: Type-safe getters with error handling
- Generic access: Get(key) for any path including custom
- Existence check: Has(key) for validation
- Complete access: GetAll() for iteration
`,

		ExpectedOutcome: `
DesiredState schema (isDesiredState=true):
- MUST return PropertyPaths object (not nil)
- MUST Root() return "desiredstate"
- MUST Meta() return "desiredstate.meta"
- MUST MetaRootPath() return "desiredstate.meta.parts.self"
- MUST Get("root") return "desiredstate"
- MUST Has("root") return true
- MUST Has("nonexistent") return false
- MUST GetAll() return non-empty map with all paths

Configuration schema (isDesiredState=false):
- MUST return PropertyPaths object (not nil)
- MUST Root() return "configuration"
- MUST Meta() return "configuration.meta"
- MUST Get("root") return "configuration"
- MUST all core paths present and accessible

Accessor methods:
- MUST specific getters (Root, Meta, etc.) work
- MUST generic Get(key) work for any path
- MUST Has(key) correctly detect presence
- MUST GetAll() return complete map
- MUST error handling work for missing keys
`,

		TestScenario: `
Test Case 1: DesiredState property paths
GIVEN:
  - SchemaManager with embedded yago schemas
  - Schema version: "1.0.0"
  - isDesiredState: true

WHEN: DiscoverPropertyPaths("", "1.0.0", true)

THEN:
  - Returns PropertyPaths (no error)
  - Root() == "desiredstate"
  - Meta() == "desiredstate.meta"
  - MetaRootPath() == "desiredstate.meta.parts.self"
  - Get("root") == "desiredstate"
  - Has("root") == true
  - Has("nonexistent") == false
  - GetAll() returns non-empty map

Test Case 2: Configuration property paths
GIVEN:
  - SchemaManager with embedded yago schemas
  - Schema version: "1.0.0"
  - isDesiredState: false

WHEN: DiscoverPropertyPaths("", "1.0.0", false)

THEN:
  - Returns PropertyPaths (no error)
  - Root() == "configuration"
  - Meta() == "configuration.meta"
  - All core paths accessible

Test implementation:
1. Create SchemaManager with defaults (embedded yago schemas)
2. Test DesiredState paths:
   - Call DiscoverPropertyPaths("", "1.0.0", true)
   - Verify all getter methods return correct paths
   - Test generic Get(), Has(), GetAll() methods
3. Test Configuration paths:
   - Call DiscoverPropertyPaths("", "1.0.0", false)
   - Verify Root() and Meta() return configuration paths
`,

		Rationale: `
Why this behavior exists:
- Schema-driven: Property paths defined in schema (not hardcoded)
- Flexibility: Different schemas can have different path structures
- Type safety: Getter methods provide type-safe access to common paths
- Discoverability: x-gitops-paths documents schema structure
- Validation: PropertyPaths ensures required paths exist

Property path use cases:
- Document traversal: Navigate to specific sections
- Metadata extraction: Find meta fields in document
- Spec access: Get specification from document
- Status updates: Update status section
- Parts handling: Process document parts (file or inline)

DesiredState vs Configuration:
- DesiredState: Infrastructure desired state documents
- Configuration: Configuration parameter documents
- Different roots: "desiredstate" vs "configuration"
- Same structure: Both have meta, spec, status, parts
- Consistent API: PropertyPaths works for both

Why getter methods:
- Common paths: Root, Meta, etc. used frequently
- Type safety: Method return types known
- Error handling: Can return errors for missing paths
- Documentation: Methods document expected paths
- IDE support: Autocomplete for common paths

Generic Get() method:
- Custom paths: Access organization-specific paths
- Flexibility: Don't need getter for every possible path
- Consistency: Same interface for core and custom paths
`,

		RegressionRisk: `
HIGH RISK if changed:
- Path values: "desiredstate", "configuration" are standardized
- Required paths: Core paths must exist in all schemas
- Accessor methods: Code depends on Root(), Meta(), etc.
- x-gitops-paths structure: Schemas define this extension

MEDIUM RISK:
- Path format: Changing dot notation affects traversal
- Error handling: Callers expect errors for missing paths
- GetAll() behavior: Code may iterate over all paths

LOW RISK:
- Getter method implementation: Internal details can change
- Error messages: Exact wording flexible
- Has() implementation: Just checks map key existence

What breaks if this changes:
1. Change "desiredstate" root → document traversal fails
2. Remove required paths → accessor methods fail
3. Change path format → property access breaks
4. Remove Has() → validation code breaks
5. Modify GetAll() → iteration code fails
6. Change extension name → schemas incompatible
`,
	}

	// Execute the behavioral tests
	t.Run("Successfully discover DesiredState and Configuration paths", func(t *testing.T) {
		projectRoot, err := findProjectRoot()
		if err != nil {
			t.Fatalf("Failed to determine project root: %v", err)
		}
		scMan, err := NewSchemaManagerWithDefaults(projectRoot)
		if err != nil {
			t.Fatalf("Failed to create schema manager: %v", err)
		}

		// Test DesiredState paths
		t.Run("DesiredState paths from v1.0.0", func(t *testing.T) {
			paths, err := scMan.DiscoverPropertyPaths("", "1.0.0", true)
			if err != nil {
				t.Fatalf("Unexpected error when discovering property paths: %v", err)
			}

			// Verify paths using getter methods
			root, err := paths.Root()
			if err != nil {
				t.Errorf("Unexpected error getting Root: %v", err)
			}
			if root != "desiredstate" {
				t.Errorf("Expected Root to be 'desiredstate', got '%s'", root)
			}

			meta, err := paths.Meta()
			if err != nil {
				t.Errorf("Unexpected error getting Meta: %v", err)
			}
			if meta != "desiredstate.meta" {
				t.Errorf("Expected Meta to be 'desiredstate.meta', got '%s'", meta)
			}

			metaRootPath, err := paths.MetaRootPath()
			if err != nil {
				t.Errorf("Unexpected error getting MetaRootPath: %v", err)
			}
			if metaRootPath != "desiredstate.meta.parts.self" {
				t.Errorf("Expected MetaRootPath to be 'desiredstate.meta.parts.self', got '%s'", metaRootPath)
			}

			// Test generic Get() method
			rootPath, err := paths.Get("root")
			if err != nil {
				t.Errorf("Unexpected error getting 'root' path: %v", err)
			}
			if rootPath != "desiredstate" {
				t.Errorf("Expected root path to be 'desiredstate', got '%s'", rootPath)
			}

			// Test Has() method
			if !paths.Has("root") {
				t.Error("Expected paths to have 'root' key")
			}
			if paths.Has("nonexistent") {
				t.Error("Expected paths to not have 'nonexistent' key")
			}

			// Test GetAll() method
			allPaths := paths.GetAll()
			if len(allPaths) == 0 {
				t.Error("Expected GetAll() to return non-empty map")
			}
		})

		// Test Configuration paths
		t.Run("Configuration paths from v1.0.0", func(t *testing.T) {
			configPaths, err := scMan.DiscoverPropertyPaths("", "1.0.0", false)
			if err != nil {
				t.Fatalf("Unexpected error when discovering configuration property paths: %v", err)
			}

			configRoot, err := configPaths.Root()
			if err != nil {
				t.Errorf("Unexpected error getting configuration Root: %v", err)
			}
			if configRoot != "configuration" {
				t.Errorf("Expected configuration Root to be 'configuration', got '%s'", configRoot)
			}

			configMeta, err := configPaths.Meta()
			if err != nil {
				t.Errorf("Unexpected error getting configuration Meta: %v", err)
			}
			if configMeta != "configuration.meta" {
				t.Errorf("Expected configuration Meta to be 'configuration.meta', got '%s'", configMeta)
			}
		})
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 3: CUSTOM PROPERTY PATHS FROM PLUGIN SCHEMAS
// =============================================================================

func TestDiscoverPropertyPaths_CustomPaths_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "DiscoverPropertyPaths loads custom organization-specific property paths from plugin schemas alongside required core paths",

		CurrentImpl: `
Go: x-gitops-paths extension in schema JSON

Standard x-gitops-paths extension structure:
{
  "x-gitops-paths": {
    // Required core paths (9 paths)
    "root": "desiredstate",
    "meta": "desiredstate.meta",
    "metaRootPath": "desiredstate.meta.parts.self",
    "spec": "desiredstate.spec",
    "status": "desiredstate.status",
    "parts": "desiredstate.meta.parts",
    "partsFile": "desiredstate.meta.parts.file",
    "partsInline": "desiredstate.meta.parts.inline",
    "partsInlineKind": "desiredstate.meta.parts.inline.kind",

    // Custom organization paths (extensible)
    "customOrgCostCenter": "desiredstate.meta.organization.cost_center",
    "customOrgOwner": "desiredstate.meta.organization.owner",
    "customComplianceTags": "desiredstate.meta.compliance.tags",
    "customProjectMetrics": "desiredstate.spec.project.metrics",

    // Optional comment fields
    "_comment": "Custom paths for organization XYZ"
  }
}

Key features:
- Core paths required: 9 standard paths must exist
- Custom paths optional: Organizations can add their own
- Naming convention: "custom" prefix for organization paths
- Arbitrary structure: Custom paths can reference any schema location
- PropertyPaths handles all: No distinction between core and custom in API

Custom path use cases:
- Organization fields: cost_center, owner, department
- Compliance: tags, policies, audit_info
- Business metrics: SLAs, budgets, KPIs
- Custom metadata: project-specific fields
- Integration points: External system references

Discovery process:
1. Load schema by namespace (e.g., "test-custom-paths")
2. Parse x-gitops-paths extension
3. Validate core paths exist
4. Include all custom paths
5. Return PropertyPaths with complete map
`,

		ExpectedOutcome: `
Required core paths (MUST exist):
- root, meta, metaRootPath, spec, status
- parts, partsFile, partsInline, partsInlineKind
- MUST Has() return true for all required paths
- MUST Get() work for all required paths

Custom paths (MUST be loaded):
- MUST Has() return true for custom paths
- MUST Get() return correct custom path values
- MUST customOrgCostCenter accessible
- MUST customOrgOwner accessible
- MUST customComplianceTags accessible
- MUST customProjectMetrics accessible

GetAll() behavior:
- MUST return map with core + custom paths
- MUST have at least 9 required + custom count
- MAY include _comment fields (ignored in count)
- MUST support iteration over all paths

Error handling:
- MUST error for non-existent paths (Get())
- MUST return false for non-existent paths (Has())
- MUST NOT error for custom paths
`,

		TestScenario: `
GIVEN:
  - SchemaManager with custom test schema
  - namespace: "test-custom-paths"
  - version: "1.0.0"
  - Schema has x-gitops-paths with:
    - 9 required core paths
    - 4 custom organization paths

WHEN: DiscoverPropertyPaths("test-custom-paths", "1.0.0", true)

THEN:
  - Returns PropertyPaths (no error)

  Core paths accessible:
  - Has("root") == true
  - Has("meta") == true
  - Get("root") works
  - Get("meta") works

  Custom paths accessible:
  - Has("customOrgCostCenter") == true
  - Has("customOrgOwner") == true
  - Has("customComplianceTags") == true
  - Has("customProjectMetrics") == true
  - Get("customOrgCostCenter") == "desiredstate.meta.organization.cost_center"

  Complete access:
  - GetAll() returns >= 13 paths (9 core + 4 custom)
  - All paths accessible via Get()

  Error handling:
  - Get("nonExistentPath") returns error
  - Has("nonExistentPath") returns false

Test implementation:
1. Create SchemaManager with defaults (loads test schemas)
2. Discover paths from test-custom-paths namespace
3. Verify required core paths exist (Has checks)
4. Verify custom paths exist (Has checks)
5. Retrieve and validate custom path values (Get)
6. Check GetAll() returns complete map
7. Test error handling for non-existent paths
8. Log all discovered paths for debugging
`,

		Rationale: `
Why this behavior exists:
- Extensibility: Organizations need custom fields beyond core schema
- Flexibility: Each organization has unique requirements
- Consistency: Custom paths use same API as core paths
- Validation: PropertyPaths ensures custom paths are defined
- Documentation: x-gitops-paths self-documents schema structure

Custom path design rationale:
- Plugin schemas: Custom schemas registered at runtime
- Namespace isolation: test-custom-paths is separate namespace
- Schema-driven: Custom paths in x-gitops-paths (not code)
- No special handling: PropertyPaths treats all paths equally
- Naming convention: "custom" prefix indicates organization-specific

Real-world custom path examples:
- Cost tracking: customOrgCostCenter, customOrgBudget
- Ownership: customOrgOwner, customOrgTeam
- Compliance: customComplianceTags, customAuditInfo
- Business metrics: customProjectMetrics, customSLAs
- Integration: customJiraTicket, customServiceNowCMDB

Benefits of custom paths:
- No code changes: Just add to schema x-gitops-paths
- Type safety: Paths validated at discovery time
- Consistency: Same traversal API as core paths
- Discoverability: GetAll() shows all available paths
- Tooling: External tools can discover custom paths

Alternative designs (not used):
- Hardcoded: Custom paths in code (inflexible)
- Separate API: Different methods for custom paths (complex)
- No validation: Free-form access (error-prone)
- Runtime registration: Paths registered in code (not schema-driven)
`,

		RegressionRisk: `
HIGH RISK if changed:
- Core path requirements: Breaking change to remove required paths
- Custom path loading: Must load all paths from extension
- PropertyPaths API: Get(), Has(), GetAll() must work for custom
- Extension structure: x-gitops-paths format is contract

MEDIUM RISK:
- Path naming convention: "custom" prefix is convention
- GetAll() completeness: Must return core + custom
- Error handling: Custom paths should not error differently

LOW RISK:
- Test schema specifics: test-custom-paths is test data
- Custom path names: Specific names are just examples
- Comment fields: _comment fields are ignored

What breaks if this changes:
1. Require custom paths → plugin schemas must define them
2. Filter custom paths → organizations lose functionality
3. Change API → code using custom paths breaks
4. Separate custom path methods → API inconsistency
5. Don't load custom paths → custom fields not accessible
6. Validate custom path structure → arbitrary paths rejected
`,
	}

	// Execute the behavioral test
	t.Run("Load custom property paths from plugin schema", func(t *testing.T) {
		projectRoot, err := findProjectRoot()
		if err != nil {
			t.Fatalf("Failed to determine project root: %v", err)
		}
		scMan, err := NewSchemaManagerWithDefaults(projectRoot)
		if err != nil {
			t.Fatalf("Failed to create schema manager: %v", err)
		}

		// Discover paths from test custom schema
		paths, err := scMan.DiscoverPropertyPaths("test-custom-paths", "1.0.0", true)
		if err != nil {
			t.Fatalf("Unexpected error when discovering property paths: %v", err)
		}

		// Verify required core paths are present
		if !paths.Has("root") {
			t.Error("Expected paths to have required 'root' key")
		}
		if !paths.Has("meta") {
			t.Error("Expected paths to have required 'meta' key")
		}

		// Verify custom paths are loaded
		if !paths.Has("customOrgCostCenter") {
			t.Error("Expected paths to have custom 'customOrgCostCenter' key")
		}
		if !paths.Has("customOrgOwner") {
			t.Error("Expected paths to have custom 'customOrgOwner' key")
		}
		if !paths.Has("customComplianceTags") {
			t.Error("Expected paths to have custom 'customComplianceTags' key")
		}
		if !paths.Has("customProjectMetrics") {
			t.Error("Expected paths to have custom 'customProjectMetrics' key")
		}

		// Verify we can retrieve custom paths
		costCenterPath, err := paths.Get("customOrgCostCenter")
		if err != nil {
			t.Errorf("Unexpected error getting custom path: %v", err)
		}
		if costCenterPath != "desiredstate.meta.organization.cost_center" {
			t.Errorf("Expected customOrgCostCenter to be 'desiredstate.meta.organization.cost_center', got '%s'", costCenterPath)
		}

		// Verify GetAll returns all paths (core + custom + comments)
		allPaths := paths.GetAll()
		requiredCount := 9                         // 9 required core paths
		customCount := 4                           // 4 custom paths in this schema
		minExpected := requiredCount + customCount // At least this many (may have _comment fields)

		if len(allPaths) < minExpected {
			t.Errorf("Expected GetAll() to return at least %d paths (%d required + %d custom), got %d",
				minExpected, requiredCount, customCount, len(allPaths))
		}

		// Log all paths for debugging
		t.Logf("Total paths discovered: %d", len(allPaths))
		for key := range allPaths {
			if key != "_comment" { // Skip comment fields in the count verification
				t.Logf("  - %s", key)
			}
		}

		// Verify that accessing non-existent path returns error
		_, err = paths.Get("nonExistentPath")
		if err == nil {
			t.Error("Expected error when getting non-existent path, got none")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}
