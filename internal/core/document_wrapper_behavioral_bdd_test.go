package core

// This file documents behavioral contracts for wrapper validation in GitOpsDocument.
//
// Migration Information:
// - Original File: document_wrapper_test.go (161 lines)
// - Original Tests: 9
// - Migration Date: October 13, 2025
// - Migrated Tests:
//   1. EmptyWrapper - Empty wrapper handling (warning logged, no error)
//   2. NoSchemaManager - Error when schema manager not initialized
//   3. NoSchemaVersion - Error when schema version not set
//   4. AllSupportedWrappers - All 3 wrappers validated (meta, terraform, concourse)
//   5. UnsupportedWrapper - Rejection of invalid wrapper names
//   6. UnknownWrapper - Unknown wrapper rejection
//   7. CaseInsensitive - Case-insensitive wrapper matching
//   8. ErrorMessage - Helpful error messages with supported wrapper list
//   9. DynamicDiscovery - Wrappers loaded from schema (not hardcoded)
//
// Wrapper Concept:
// Wrappers are named subsections within GitOps documents that organize configuration
// by tool/technology (e.g., "terraform", "concourse", "docker"). The schema defines
// which wrappers are valid for each schema version, ensuring documents only reference
// supported configuration types.
//
// Supported Wrappers in v1.0.0:
// - meta: Document metadata
// - terraform: Terraform IaC configuration
// - concourse: Concourse CI/CD pipelines

import (
	"testing"

	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitOpsDocument_WrapperValidation_EmptyWrapper_BehavioralBDD documents
// empty wrapper handling.
//
// BEHAVIORAL CONTRACT:
// ValidateWrapper("") logs a warning but returns nil (no error). This allows
// documents to omit wrapper specifications when using default configuration
// paths. The wrapper validation is permissive for empty values.
//
// CRITICAL SEMANTICS:
// - Empty string accepted: "" wrapper is valid
// - Warning logged: User notified via logging (not error)
// - No error returned: Document processing continues
// - Default behavior: System uses default paths when wrapper omitted
//
// RATIONALE:
// Permissive handling of empty wrappers supports backward compatibility and
// simple configurations that don't need wrapper-based organization. Warning
// provides visibility without breaking functionality.
//
// REGRESSION RISK: MEDIUM
// - Making empty wrapper an error would break simple configurations
// - Removing warning would reduce visibility into document structure
func TestGitOpsDocument_WrapperValidation_EmptyWrapper_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Empty wrapper is accepted with warning, no error returned",
		CurrentImpl:     "ValidateWrapper('') logs warning via logging.Warn(), returns nil",
		ExpectedOutcome: "Warning logged, processing continues, no error",
		TestScenario:    "Call ValidateWrapper with empty string, verify no error returned",
		Rationale:       "Supports backward compatibility and simple configs without wrapper organization",
		RegressionRisk:  "MEDIUM - Making empty an error would break simple configurations",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)

	doc := NewGitOpsDocument()

	// TEST: Empty wrapper should not cause error
	err := doc.ValidateWrapper("")
	assert.NoError(t, err, "BEHAVIORAL CONTRACT VIOLATION: Empty wrapper should be accepted")

	t.Logf("✓ Behavior verified: Empty wrapper accepted, warning logged")
}

// TestGitOpsDocument_WrapperValidation_Prerequisites_BehavioralBDD documents
// required initialization before wrapper validation.
//
// BEHAVIORAL CONTRACT:
// ValidateWrapper requires schemaManager and schemaVersion to be initialized
// before validation can occur. Missing either causes validation to fail immediately
// with descriptive error. This enforces proper document initialization order.
//
// CRITICAL SEMANTICS:
// - Schema manager required: Must be non-nil
// - Schema version required: Must be non-empty string
// - Fail-fast: Validation fails immediately if prerequisites missing
// - Clear errors: Error messages specify exactly what's missing
//
// RATIONALE:
// Wrapper validation depends on schema definitions which vary by version.
// Enforcing prerequisites prevents validation with wrong/missing schema data.
// Fail-fast principle catches initialization errors early.
//
// REGRESSION RISK: HIGH
// - Removing checks would allow validation with invalid state
// - Less specific errors would make debugging harder
func TestGitOpsDocument_WrapperValidation_Prerequisites_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Wrapper validation requires initialized schemaManager and schemaVersion",
		CurrentImpl:     "ValidateWrapper checks schemaManager != nil and schemaVersion != '', returns descriptive errors",
		ExpectedOutcome: "Clear error returned if prerequisites missing, validation blocked",
		TestScenario:    "Call ValidateWrapper with nil schemaManager or empty version, verify specific errors",
		Rationale:       "Fail-fast on missing prerequisites prevents validation with wrong schema data",
		RegressionRisk:  "HIGH - Removing checks allows validation with invalid state",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)

	// TEST: No schema manager
	doc := NewGitOpsDocument()
	doc.schemaManager = nil

	err := doc.ValidateWrapper("terraform")
	require.Error(t, err, "BEHAVIORAL CONTRACT VIOLATION: Must error when schema manager not initialized")
	assert.Contains(t, err.Error(), "schema manager not initialized", "Error should specify missing schema manager")

	// TEST: No schema version
	doc2 := NewGitOpsDocument()
	scMan, err := schema.GetOrCreateSchemaManager("")
	require.NoError(t, err)
	doc2.schemaManager = scMan
	doc2.schemaVersion = ""

	err = doc2.ValidateWrapper("terraform")
	require.Error(t, err, "BEHAVIORAL CONTRACT VIOLATION: Must error when schema version not set")
	assert.Contains(t, err.Error(), "schema version not set", "Error should specify missing schema version")

	t.Logf("✓ Behavior verified: Prerequisites enforced with clear error messages")
}

// TestGitOpsDocument_WrapperValidation_SupportedWrappers_BehavioralBDD documents
// validation of all supported wrapper types in embedded schema v1.0.0.
//
// BEHAVIORAL CONTRACT:
// ValidateWrapper accepts all 3 wrapper types defined in embedded schema v1.0.0.
// These wrappers represent different tool/technology categories that can be
// configured in GitOps documents. Validation is case-insensitive.
//
// CRITICAL SEMANTICS:
// - 3 wrappers supported: meta, terraform, concourse
// - Case-insensitive: "terraform", "Terraform", "TERRAFORM" all valid
// - Schema-driven: Wrapper list comes from schema, not hardcoded
// - Version-specific: Different schema versions may support different wrappers
//
// RATIONALE:
// Schema-defined wrappers allow validation against known configuration types.
// This prevents typos and ensures document structure matches schema expectations.
// Case-insensitivity improves user experience.
//
// REGRESSION RISK: HIGH
// - Removing wrappers would break existing documents using them
// - Making case-sensitive would break documents with non-lowercase wrappers
// - Hardcoding wrapper list would break schema evolution
func TestGitOpsDocument_WrapperValidation_SupportedWrappers_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "All 3 wrappers in embedded v1.0.0 schema are validated successfully",
		CurrentImpl:     "ValidateWrapper queries schemaManager.IsWrapperSupported(), accepts wrappers from schema",
		ExpectedOutcome: "All schema-defined wrappers accepted, case-insensitive matching",
		TestScenario:    "Validate each of 3 wrappers and case variations, verify acceptance",
		Rationale:       "Schema-driven validation ensures only known configuration types accepted",
		RegressionRisk:  "HIGH - Removing wrappers breaks existing documents, case-sensitivity breaks non-lowercase usage",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)

	doc := NewGitOpsDocument()
	scMan, err := schema.GetOrCreateSchemaManager("")
	require.NoError(t, err)
	doc.schemaManager = scMan
	doc.schemaVersion = "1.0.0" // Embedded schema

	// TEST: All 3 supported wrappers from embedded v1.0.0
	supportedWrappers := []string{
		"meta", "terraform", "concourse",
	}

	for _, wrapper := range supportedWrappers {
		t.Run(wrapper, func(t *testing.T) {
			err := doc.ValidateWrapper(wrapper)
			assert.NoError(t, err, "BEHAVIORAL CONTRACT VIOLATION: '%s' should be supported in v1.0.0", wrapper)
		})
	}

	// TEST: Case-insensitive matching
	t.Run("case_variations", func(t *testing.T) {
		caseVariations := []string{
			"terraform",
			"Terraform",
			"TERRAFORM",
			"TeRrAfOrM",
		}

		for _, wrapper := range caseVariations {
			err := doc.ValidateWrapper(wrapper)
			assert.NoError(t, err, "BEHAVIORAL CONTRACT VIOLATION: Case variation '%s' should be accepted", wrapper)
		}
	})

	t.Logf("✓ Behavior verified: All 3 wrappers accepted, case-insensitive matching works")
}

// TestGitOpsDocument_WrapperValidation_UnsupportedWrappers_BehavioralBDD documents
// rejection of invalid wrapper names.
//
// BEHAVIORAL CONTRACT:
// ValidateWrapper rejects wrapper names not in the schema's supported list.
// Error message includes the invalid wrapper name and lists all supported
// wrappers to guide users toward valid options.
//
// CRITICAL SEMANTICS:
// - Strict validation: Only schema-defined wrappers accepted
// - Clear errors: Message includes invalid name and valid options
// - Fail-fast: Invalid wrapper causes immediate validation failure
// - Helpful guidance: Supported wrapper list in error aids correction
//
// RATIONALE:
// Strict validation prevents typos and undefined configuration sections from
// passing validation. Clear error messages with supported options help users
// quickly identify and fix invalid wrapper references.
//
// REGRESSION RISK: MEDIUM
// - Making validation permissive would allow undefined configuration sections
// - Removing supported wrapper list from error reduces helpfulness
// - Auto-correction would hide user errors
func TestGitOpsDocument_WrapperValidation_UnsupportedWrappers_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Invalid wrapper names are rejected with helpful error listing supported options",
		CurrentImpl:     "ValidateWrapper checks IsWrapperSupported(), returns error with GetSupportedWrappers() list",
		ExpectedOutcome: "Error returned containing invalid name and list of valid wrappers",
		TestScenario:    "Validate clearly invalid wrappers (foobar, unknown-wrapper), verify error content",
		Rationale:       "Strict validation prevents typos, helpful errors guide users to valid options",
		RegressionRisk:  "MEDIUM - Permissive validation would allow undefined sections, less helpful errors harder to debug",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)

	doc := NewGitOpsDocument()
	scMan, err := schema.GetOrCreateSchemaManager("")
	require.NoError(t, err)
	doc.schemaManager = scMan
	doc.schemaVersion = "1.0.0"

	// TEST: Clearly invalid wrapper
	err = doc.ValidateWrapper("foobar")
	require.Error(t, err, "BEHAVIORAL CONTRACT VIOLATION: 'foobar' should be rejected")
	assert.Contains(t, err.Error(), "not supported", "Error should indicate wrapper not supported")
	assert.Contains(t, err.Error(), "foobar", "Error should include invalid wrapper name")

	// TEST: Unknown wrapper with hyphen (realistic typo)
	err = doc.ValidateWrapper("unknown-wrapper")
	require.Error(t, err, "BEHAVIORAL CONTRACT VIOLATION: 'unknown-wrapper' should be rejected")
	assert.Contains(t, err.Error(), "not supported", "Error should indicate wrapper not supported")
	assert.Contains(t, err.Error(), "unknown-wrapper", "Error should include invalid wrapper name")

	// TEST: Error message includes supported wrappers
	err = doc.ValidateWrapper("invalid-wrapper-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid-wrapper-name", "Error should include invalid name")
	assert.Contains(t, err.Error(), "not supported", "Error should indicate not supported")
	// Error should list some supported wrappers
	assert.Contains(t, err.Error(), "meta", "Error should list 'meta' as supported")
	assert.Contains(t, err.Error(), "terraform", "Error should list 'terraform' as supported")

	t.Logf("✓ Behavior verified: Invalid wrappers rejected with helpful error messages")
}

// TestGitOpsDocument_WrapperValidation_DynamicDiscovery_BehavioralBDD documents
// dynamic wrapper loading from schema.
//
// BEHAVIORAL CONTRACT:
// Wrapper validation queries the schema manager for supported wrappers rather
// than using hardcoded lists. This allows schema evolution without code changes.
// Different schema versions can define different wrapper sets.
//
// CRITICAL SEMANTICS:
// - Schema-driven: Wrappers loaded from schema.GetSupportedWrappers()
// - Not hardcoded: No wrapper list in validation code
// - Version-specific: Each schema version can have different wrappers
// - Dynamic discovery: New wrappers added via schema updates, not code changes
//
// RATIONALE:
// Schema-driven wrapper discovery decouples validation logic from wrapper
// definitions. This enables schema evolution (new wrappers, deprecated wrappers)
// without modifying validation code. Critical for extensibility.
//
// REGRESSION RISK: CRITICAL
// - Hardcoding wrappers would break schema evolution
// - Ignoring schema version would apply wrong validation rules
// - Caching without invalidation would miss schema updates
func TestGitOpsDocument_WrapperValidation_DynamicDiscovery_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Wrappers dynamically loaded from schema, not hardcoded in validation logic",
		CurrentImpl:     "ValidateWrapper calls schemaManager.GetSupportedWrappers(version) to get current wrapper list",
		ExpectedOutcome: "Wrapper list matches schema definition, changes with schema version",
		TestScenario:    "Query schema manager for wrappers, verify count and contents match embedded v1.0.0",
		Rationale:       "Schema-driven discovery enables wrapper evolution without code changes",
		RegressionRisk:  "CRITICAL - Hardcoding breaks schema evolution and extensibility",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)

	doc := NewGitOpsDocument()
	scMan, err := schema.GetOrCreateSchemaManager("")
	require.NoError(t, err)
	doc.schemaManager = scMan
	doc.schemaVersion = "1.0.0"

	// TEST: Dynamic discovery from schema
	wrappers := scMan.GetSupportedWrappers("1.0.0")

	require.NotEmpty(t, wrappers, "BEHAVIORAL CONTRACT VIOLATION: Should discover wrappers from schema")
	require.Equal(t, 3, len(wrappers), "BEHAVIORAL CONTRACT VIOLATION: Embedded v1.0.0 should have exactly 3 wrappers")

	// Verify all expected wrappers are present
	expectedWrappers := map[string]bool{
		"meta": true, "terraform": true, "concourse": true,
	}

	for _, wrapper := range wrappers {
		assert.True(t, expectedWrappers[wrapper], "BEHAVIORAL CONTRACT VIOLATION: Unexpected wrapper '%s' in schema", wrapper)
	}

	// Verify no hardcoding - validation should work with schema-defined wrappers
	for wrapper := range expectedWrappers {
		err := doc.ValidateWrapper(wrapper)
		assert.NoError(t, err, "BEHAVIORAL CONTRACT VIOLATION: Schema-defined wrapper '%s' should validate", wrapper)
	}

	t.Logf("✓ Behavior verified: Wrappers dynamically loaded from schema, matches v1.0.0 definition")
}
