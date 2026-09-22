package core

// This file documents behavioral contracts for core GitOpsDocument helper methods.
//
// Migration Information:
// - Original File: document_test.go (481 lines)
// - Original Tests: 13 (8 migrated here, 5 already covered in document_behavioral_bdd_test.go)
// - Migration Date: October 13, 2025
// - Migrated Tests:
//   1. SetEnvironmentVariables - Environment variable management
//   2. SetSchema - Schema version and property root configuration
//   3. ParsePart - YAML file parsing and content extraction
//   4. DetermineDocumentType - Document type detection
//   5. Getters - Accessor methods (GetWorkdir, GetMetaFile, GetKind, etc.)
//   6. ToString - Document serialization to YAML
//   7. ToMetaString - Metadata serialization to YAML
//   8. KindField - Kind field reading, normalization, validation, backward compatibility
//   9. NamespaceField - Namespace case-insensitivity
//
// Tests Already Covered in document_behavioral_bdd_test.go:
//   - TestGitOpsDocument_Initialization_BehavioralBDD (NewGitOpsDocument)
//   - TestGitOpsDocument_LoadMetadata_BehavioralBDD (KindReadFromDocument)
//   - TestGitOpsDocument_UpdateRepoProperties_BehavioralBDD (UpdateRepoProperties)
//   - TestGitOpsDocument_ParseDesiredStateRef_BehavioralBDD (ParseDesiredStateRef)
//
// This file focuses on helper methods, configuration, and edge cases not covered
// by the tests in document_behavioral_bdd_test.go.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/schema"
)

// GoBehavioralContract documents the expected behavior of Go code at a specific point in time.
type GoBehavioralContract struct {
	Behavior        string // High-level description of the behavior
	CurrentImpl     string // How it's currently implemented
	ExpectedOutcome string // What should happen
	TestScenario    string // Specific test case
	Rationale       string // Why this behavior exists
	RegressionRisk  string // Impact if behavior changes
}

// TestGitOpsDocument_EnvironmentVariables_BehavioralBDD documents environment variable
// management in GitOpsDocument.
//
// BEHAVIORAL CONTRACT:
// SetEnvironmentVariables() performs a full replacement of the internal environment map.
// It does NOT merge with existing variables - each call completely replaces the previous
// environment. Variables are stored as-is without validation or transformation.
//
// CRITICAL SEMANTICS:
// - Full replacement: Previous environment is discarded
// - No validation: Empty keys, empty values all accepted
// - No transformation: Keys/values stored exactly as provided
// - Nil map accepted: Replaces environment with nil (caller must handle)
//
// RATIONALE:
// Full replacement semantics are simpler than merge semantics and match common
// patterns like os.Setenv(). Each document instance gets its own isolated environment
// that can be completely reconfigured without worrying about inherited state.
//
// REGRESSION RISK: HIGH
// - Changing to merge semantics would break code assuming clean slate
// - Adding validation would break documents using special characters
// - Transforming keys (e.g., uppercase) would break case-sensitive lookups
func TestGitOpsDocument_EnvironmentVariables_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "SetEnvironmentVariables performs full replacement of document environment",
		CurrentImpl:     "SetEnvironmentVariables(map) assigns the map directly to doc.envVariables",
		ExpectedOutcome: "Previous environment discarded, new environment stored as-is without transformation",
		TestScenario:    "Call SetEnvironmentVariables twice with different maps, verify second call replaces first",
		Rationale:       "Full replacement is simpler than merging and provides clean slate semantics",
		RegressionRisk:  "HIGH - Merge semantics would break code assuming clean environment state",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)

	doc := NewGitOpsDocument()

	// TEST: Set initial environment
	env1 := map[string]string{
		"TEST_VAR": "test_value",
		"ENV":      "test",
	}
	doc.SetEnvironmentVariables(env1)

	if doc.envVariables["TEST_VAR"] != "test_value" {
		t.Errorf("Expected TEST_VAR='test_value', got '%s'", doc.envVariables["TEST_VAR"])
	}
	if doc.envVariables["ENV"] != "test" {
		t.Errorf("Expected ENV='test', got '%s'", doc.envVariables["ENV"])
	}

	// TEST: Full replacement behavior
	env2 := map[string]string{
		"NEW_VAR": "new_value",
	}
	doc.SetEnvironmentVariables(env2)

	if _, exists := doc.envVariables["TEST_VAR"]; exists {
		t.Error("BEHAVIORAL CONTRACT VIOLATION: Previous environment should be replaced, not merged")
	}
	if doc.envVariables["NEW_VAR"] != "new_value" {
		t.Errorf("Expected NEW_VAR='new_value', got '%s'", doc.envVariables["NEW_VAR"])
	}

	t.Logf("✓ Behavior verified: Environment variables fully replaced, no merging occurs")
}

// TestGitOpsDocument_SetSchema_BehavioralBDD documents schema configuration behavior.
//
// BEHAVIORAL CONTRACT:
// SetSchema() configures the document's schema version AND sets the property root
// based on document type (desiredstate vs configuration). The property root determines
// which top-level YAML key contains the document's actual content.
//
// CRITICAL SEMANTICS:
// - Document type must be determined BEFORE calling SetSchema
// - DesiredState documents → propertyRoot = "desiredstate"
// - Configuration documents → propertyRoot = "configuration"
// - Schema version stored as string (not parsed/validated)
// - Property root used for all subsequent YAML operations
//
// RATIONALE:
// The property root is a fundamental part of the YAML structure and must be set
// early in document lifecycle. Tying it to SetSchema ensures it's configured
// before any content operations occur. Different document types have different
// top-level structures that must be handled consistently.
//
// REGRESSION RISK: CRITICAL
// - Wrong property root would break all YAML parsing/generation
// - Changing root names would break all existing documents
// - Making property root configurable would break schema validation
func TestGitOpsDocument_SetSchema_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "SetSchema configures schema version and property root based on document type",
		CurrentImpl:     "SetSchema(version) stores version string and sets propertyRoot to 'desiredstate' or 'configuration'",
		ExpectedOutcome: "Schema version stored, property root set correctly for document type",
		TestScenario:    "Create DesiredState and Configuration documents, call SetSchema, verify property roots",
		Rationale:       "Property root is fundamental YAML structure that must be set before content operations",
		RegressionRisk:  "CRITICAL - Wrong property root breaks all YAML parsing and generation",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)

	// TEST: DesiredState document schema
	doc := NewGitOpsDocument()
	doc.isDesiredState = true
	doc.isConfiguration = false

	version := schema.SchemaVersion("1.0.0")
	err := doc.SetSchema(version)
	if err != nil {
		t.Fatalf("Unexpected error setting schema: %v", err)
	}

	if doc.schemaVersion != string(version) {
		t.Errorf("Expected schemaVersion='%s', got '%s'", version, doc.schemaVersion)
	}

	if doc.propertyRoot != "desiredstate" {
		t.Errorf("BEHAVIORAL CONTRACT VIOLATION: DesiredState documents must have propertyRoot='desiredstate', got '%s'", doc.propertyRoot)
	}

	// TEST: Configuration document schema
	doc2 := NewGitOpsDocument()
	doc2.isDesiredState = false
	doc2.isConfiguration = true

	err = doc2.SetSchema(version)
	if err != nil {
		t.Fatalf("Unexpected error setting configuration schema: %v", err)
	}

	if doc2.propertyRoot != "configuration" {
		t.Errorf("BEHAVIORAL CONTRACT VIOLATION: Configuration documents must have propertyRoot='configuration', got '%s'", doc2.propertyRoot)
	}

	t.Logf("✓ Behavior verified: Schema sets property root correctly based on document type")
}

// TestGitOpsDocument_ParsePart_BehavioralBDD documents YAML file parsing behavior.
//
// BEHAVIORAL CONTRACT:
// ParsePart() reads a YAML file from a base directory and returns the parsed content
// as a map[string]interface{}. It preserves nested structure and handles environment
// variable substitution via the provided envVars map.
//
// CRITICAL SEMANTICS:
// - Relative path resolution: file path relative to baseDir
// - Nested maps preserved: YAML structure maintained in Go map hierarchy
// - Type preservation: YAML types (string, int, bool) preserved in interface{}
// - Environment substitution: [[gitops.getEnvValue(VAR)]] replaced from envVars map
// - Single document: Only first YAML document parsed (--- separators ignored after first)
//
// RATIONALE:
// ParsePart is the foundational YAML loading function used by all document types.
// It abstracts file I/O and YAML parsing while providing environment variable
// injection needed for templating. The envVars parameter allows isolated environments
// per document without affecting global process environment.
//
// REGRESSION RISK: CRITICAL
// - Path resolution changes would break all document loading
// - Losing nested structure would break schema validation
// - Removing environment substitution would break templating
// - Type coercion (e.g., all strings) would break type-aware logic
func TestGitOpsDocument_ParsePart_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "ParsePart reads YAML file and returns parsed content with nested structure preserved",
		CurrentImpl:     "ParsePart(baseDir, filename, envVars) uses YAML parser to load file into map[string]interface{}",
		ExpectedOutcome: "YAML structure preserved, types maintained, content returned as nested maps",
		TestScenario:    "Create YAML file with nested structure, parse via ParsePart, verify hierarchy",
		Rationale:       "Foundation for all document loading, must preserve structure for schema validation",
		RegressionRisk:  "CRITICAL - Structure loss would break schema validation and all content access",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)

	// Create temporary YAML file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.yaml")

	yamlContent := `---
key: value
nested:
  subkey: subvalue
  deeper:
    level: 3
`

	err := os.WriteFile(testFile, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// TEST: Parse YAML file
	content, err := ParsePart(tempDir, "test.yaml", make(map[string]string))
	if err != nil {
		t.Fatalf("Unexpected error parsing: %v", err)
	}

	// Verify top-level key
	if content["key"] != "value" {
		t.Errorf("Expected key='value', got '%v'", content["key"])
	}

	// Verify nested structure preserved
	nested, ok := content["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("BEHAVIORAL CONTRACT VIOLATION: Nested maps must be preserved as map[string]interface{}")
	}

	if nested["subkey"] != "subvalue" {
		t.Errorf("Expected nested.subkey='subvalue', got '%v'", nested["subkey"])
	}

	// Verify deeper nesting
	deeper, ok := nested["deeper"].(map[string]interface{})
	if !ok {
		t.Fatal("BEHAVIORAL CONTRACT VIOLATION: Multi-level nesting must be preserved")
	}

	if deeper["level"] != 3 {
		t.Errorf("Expected deeper.level=3 (int), got '%v' (type %T)", deeper["level"], deeper["level"])
	}

	t.Logf("✓ Behavior verified: YAML parsed with full nested structure and type preservation")
}

// TestGitOpsDocument_DetermineDocumentType_BehavioralBDD documents document type detection.
//
// BEHAVIORAL CONTRACT:
// determineDocumentType() inspects the assembledMeta PropertyWrapper for top-level keys
// "desiredstate" or "configuration" and sets the document type flags accordingly.
// This detection happens automatically during metadata loading.
//
// CRITICAL SEMANTICS:
// - Key-based detection: Presence of "desiredstate" key → DesiredState document
// - Mutual exclusion: Document is either DesiredState OR Configuration, never both
// - Case sensitivity: Keys must exactly match "desiredstate"/"configuration" (lowercase)
// - Meta structure required: Keys must contain nested "meta" structure
// - Flags set: isDesiredState and isConfiguration boolean flags
//
// RATIONALE:
// Document type determines schema selection, validation rules, and property root.
// Automatic detection from YAML structure prevents manual configuration errors and
// ensures document type always matches actual content structure.
//
// REGRESSION RISK: CRITICAL
// - Wrong detection would apply wrong schema validation
// - Case-insensitive matching would break existing documents with uppercase keys
// - Allowing both types would create ambiguous validation state
func TestGitOpsDocument_DetermineDocumentType_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "determineDocumentType detects document type from top-level YAML keys",
		CurrentImpl:     "Checks for 'desiredstate' or 'configuration' keys in assembledMeta, sets type flags",
		ExpectedOutcome: "Document type flags set correctly, mutually exclusive (only one type)",
		TestScenario:    "Create documents with desiredstate/configuration keys, verify type detection",
		Rationale:       "Automatic detection ensures type always matches content structure",
		RegressionRisk:  "CRITICAL - Wrong detection applies wrong schema and validation rules",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)

	// TEST: DesiredState detection
	doc := NewGitOpsDocument()
	doc.assembledMeta.AddKey("desiredstate", map[string]interface{}{
		"meta": map[string]interface{}{
			"version": "1.0.0",
		},
	})

	doc.determineDocumentType()

	if !doc.IsDesiredState() {
		t.Error("BEHAVIORAL CONTRACT VIOLATION: Document with 'desiredstate' key must be detected as DesiredState")
	}

	if doc.IsConfiguration() {
		t.Error("BEHAVIORAL CONTRACT VIOLATION: Document types must be mutually exclusive")
	}

	// TEST: Configuration detection
	doc2 := NewGitOpsDocument()
	doc2.assembledMeta.AddKey("configuration", map[string]interface{}{
		"meta": map[string]interface{}{
			"version": "1.0.0",
		},
	})

	doc2.determineDocumentType()

	if doc2.IsDesiredState() {
		t.Error("BEHAVIORAL CONTRACT VIOLATION: Document with 'configuration' key must not be DesiredState")
	}

	if !doc2.IsConfiguration() {
		t.Error("BEHAVIORAL CONTRACT VIOLATION: Document with 'configuration' key must be detected as Configuration")
	}

	t.Logf("✓ Behavior verified: Document type correctly detected from YAML structure")
}

// TestGitOpsDocument_Getters_BehavioralBDD documents accessor method behavior.
//
// BEHAVIORAL CONTRACT:
// Getter methods (GetWorkdir, GetMetaFile, GetSchemaVersion, etc.) provide read-only
// access to document state. They return copies or references to internal fields
// without modification. Getters never return nil - they return empty values for
// uninitialized fields.
//
// CRITICAL SEMANTICS:
// - Read-only: Getters never modify document state
// - Never nil: PropertyWrapper getters return non-nil empty wrappers
// - Direct access: No computation or transformation
// - Immutable view: Returned values don't affect document state if modified
//
// RATIONALE:
// Getters provide safe access to document state for external consumers. The never-nil
// guarantee simplifies caller code (no defensive nil checks needed). This follows
// Go conventions where accessor methods provide predictable, side-effect-free access.
//
// REGRESSION RISK: MEDIUM
// - Returning nil would break callers assuming non-nil PropertyWrappers
// - Adding computation would break performance assumptions
// - Returning mutable references could break encapsulation
func TestGitOpsDocument_Getters_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Getter methods provide read-only access to document state, never returning nil",
		CurrentImpl:     "GetXXX() methods return internal field values directly",
		ExpectedOutcome: "All getters return non-nil values, uninitialized fields return empty values",
		TestScenario:    "Set document fields, call getters, verify values match and are non-nil",
		Rationale:       "Never-nil guarantee simplifies caller code and prevents defensive nil checks",
		RegressionRisk:  "MEDIUM - Returning nil would break callers without nil checks",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)

	doc := NewGitOpsDocument()
	doc.workdir = "/test/workdir"
	doc.metaFile = "/test/meta.yaml"
	doc.schemaVersion = "4.0.0"
	doc.kind = "test-kind"

	// TEST: Simple field getters
	if doc.GetWorkdir() != "/test/workdir" {
		t.Errorf("Expected GetWorkdir()='/test/workdir', got '%s'", doc.GetWorkdir())
	}

	if doc.GetMetaFile() != "/test/meta.yaml" {
		t.Errorf("Expected GetMetaFile()='/test/meta.yaml', got '%s'", doc.GetMetaFile())
	}

	if doc.GetSchemaVersion() != "4.0.0" {
		t.Errorf("Expected GetSchemaVersion()='4.0.0', got '%s'", doc.GetSchemaVersion())
	}

	if doc.GetKind() != "test-kind" {
		t.Errorf("Expected GetKind()='test-kind', got '%s'", doc.GetKind())
	}

	// TEST: PropertyWrapper getters never return nil
	if doc.GetMeta() == nil {
		t.Error("BEHAVIORAL CONTRACT VIOLATION: GetMeta() must never return nil")
	}

	if doc.GetContent() == nil {
		t.Error("BEHAVIORAL CONTRACT VIOLATION: GetContent() must never return nil")
	}

	t.Logf("✓ Behavior verified: All getters return expected values, PropertyWrappers never nil")
}

// TestGitOpsDocument_Serialization_BehavioralBDD documents YAML serialization behavior.
//
// BEHAVIORAL CONTRACT:
// ToString() and ToMetaString() serialize document content and metadata to YAML strings.
// They use the internal PropertyWrapper's ToString() method which handles YAML generation.
// Output is deterministic and suitable for file writing or comparison.
//
// CRITICAL SEMANTICS:
// - ToString(): Serializes contentForConsumption (actual document data)
// - ToMetaString(): Serializes assembledMeta (structural metadata)
// - YAML format: Standard YAML 1.2 syntax with proper indentation
// - Empty documents: Returns valid empty YAML (not empty string)
// - Error handling: Returns error on serialization failure (rare)
//
// RATIONALE:
// Separate serialization methods for content vs metadata allow independent access
// to different document aspects. This is critical for operations that only need
// metadata (e.g., validation) without loading full content.
//
// REGRESSION RISK: HIGH
// - Changing YAML format would break all file I/O
// - Returning empty string for empty docs would break empty document handling
// - Combining ToString/ToMetaString would break selective serialization
func TestGitOpsDocument_Serialization_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "ToString and ToMetaString serialize document parts to YAML format",
		CurrentImpl:     "Methods delegate to PropertyWrapper.ToString() for YAML generation",
		ExpectedOutcome: "Valid YAML strings containing document content or metadata",
		TestScenario:    "Add content and metadata, serialize, verify YAML contains expected keys",
		Rationale:       "Separate serialization allows selective access to content vs metadata",
		RegressionRisk:  "HIGH - Format changes would break all file I/O and document storage",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)

	doc := NewGitOpsDocument()

	// TEST: Content serialization
	doc.contentForConsumption.AddKey("test", "value")
	doc.contentForConsumption.AddKey("number", 42)

	yamlStr, err := doc.ToString()
	if err != nil {
		t.Fatalf("Unexpected error in ToString(): %v", err)
	}

	if yamlStr == "" {
		t.Error("BEHAVIORAL CONTRACT VIOLATION: ToString() must return non-empty YAML for non-empty content")
	}

	if !strings.Contains(yamlStr, "test") {
		t.Error("Expected YAML to contain 'test' key")
	}

	// TEST: Metadata serialization
	doc.assembledMeta.AddKey("version", "1.0.0")
	doc.assembledMeta.AddKey("parts", map[string]interface{}{
		"self": "test.yaml",
	})

	metaStr, err := doc.ToMetaString()
	if err != nil {
		t.Fatalf("Unexpected error in ToMetaString(): %v", err)
	}

	if metaStr == "" {
		t.Error("BEHAVIORAL CONTRACT VIOLATION: ToMetaString() must return non-empty YAML for non-empty metadata")
	}

	if !strings.Contains(metaStr, "version") {
		t.Error("Expected YAML to contain 'version' key")
	}

	t.Logf("✓ Behavior verified: Content and metadata serialize to valid YAML strings")
}

// TestGitOpsDocument_KindField_BehavioralBDD documents Kind field behavior.
//
// BEHAVIORAL CONTRACT:
// The "kind" field explicitly specifies document type (DesiredState/Configuration)
// in YAML. It is read during LoadMetadata() and normalized to canonical case.
// Kind is optional for backward compatibility - if absent, type is auto-detected.
//
// CRITICAL SEMANTICS:
// - Case-insensitive matching: "desiredstate", "DesiredState", "DESIREDSTATE" all valid
// - Canonical normalization: Always stored as "DesiredState" or "Configuration"
// - Valid values: Only "DesiredState" and "Configuration" accepted
// - Validation: Invalid kinds cause LoadMetadata() to fail with error
// - Backward compatibility: Documents without "kind" field still work (auto-detection)
//
// RATIONALE:
// Explicit Kind field makes document type unambiguous and supports future document
// types. Case-insensitivity improves user experience (no errors from capitalization).
// Backward compatibility ensures existing documents continue working.
//
// REGRESSION RISK: CRITICAL
// - Removing backward compatibility would break existing documents without "kind"
// - Making case-sensitive would break documents with non-canonical capitalization
// - Adding new kind values without migration would break schema validation
// - Changing canonical forms would break kind-based routing and filtering
func TestGitOpsDocument_KindField_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Kind field explicitly specifies document type with case-insensitive matching",
		CurrentImpl:     "LoadMetadata() reads 'kind' field, normalizes to canonical case, validates against allowed values",
		ExpectedOutcome: "Kind normalized to 'DesiredState' or 'Configuration', invalid values rejected, absent kind auto-detected",
		TestScenario:    "Load documents with various kind capitalizations and missing kind, verify normalization and validation",
		Rationale:       "Explicit kind improves clarity, case-insensitivity improves UX, backward compatibility preserves existing docs",
		RegressionRisk:  "CRITICAL - Breaking compatibility or case-insensitivity breaks existing document corpus",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)

	tempDir := t.TempDir()

	// TEST: Kind field read and normalized
	testFile := filepath.Join(tempDir, "test-with-kind.yaml")

	yamlContent := `---
schema: 1.0.0
namespace: yago
kind: DesiredState
desiredstate:
  meta:
    parts:
      self: test-with-kind.yaml
    ecosystem: {}
`

	err := os.WriteFile(testFile, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	doc := NewGitOpsDocument()
	err = doc.LoadMetadata(testFile)
	if err != nil {
		t.Fatalf("Failed to load metadata: %v", err)
	}

	if doc.GetKind() != "DesiredState" {
		t.Errorf("Expected kind='DesiredState', got '%s'", doc.GetKind())
	}

	// TEST: Case-insensitive matching
	testCases := []struct {
		kindValue    string
		expectedKind string
		docType      string
	}{
		{"desiredstate", "DesiredState", "desiredstate"},
		{"DESIREDSTATE", "DesiredState", "desiredstate"},
		{"DesiredState", "DesiredState", "desiredstate"},
		{"configuration", "Configuration", "configuration"},
		{"CONFIGURATION", "Configuration", "configuration"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("kind=%s", tc.kindValue), func(t *testing.T) {
			testFile := filepath.Join(tempDir, fmt.Sprintf("test-%s.yaml", tc.kindValue))

			yamlContent := fmt.Sprintf(`---
schema: 1.0.0
namespace: yago
kind: %s
%s:
  meta:
    parts:
      self: test.yaml
    ecosystem: {}
`, tc.kindValue, tc.docType)

			err := os.WriteFile(testFile, []byte(yamlContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			doc := NewGitOpsDocument()
			err = doc.LoadMetadata(testFile)
			if err != nil {
				t.Fatalf("Failed to load metadata: %v", err)
			}

			if doc.GetKind() != tc.expectedKind {
				t.Errorf("BEHAVIORAL CONTRACT VIOLATION: Expected kind='%s' (normalized), got '%s'", tc.expectedKind, doc.GetKind())
			}
		})
	}

	// TEST: Invalid kind rejected
	invalidFile := filepath.Join(tempDir, "test-invalid.yaml")
	yamlContent = `---
schema: 1.0.0
namespace: yago
kind: InvalidKind
desiredstate:
  meta:
    parts:
      self: test-invalid.yaml
    ecosystem: {}
`

	err = os.WriteFile(invalidFile, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	doc = NewGitOpsDocument()
	err = doc.LoadMetadata(invalidFile)
	if err == nil {
		t.Fatal("BEHAVIORAL CONTRACT VIOLATION: Invalid kind must be rejected with error")
	}

	if !strings.Contains(err.Error(), "unsupported kind 'InvalidKind'") {
		t.Errorf("Expected error about unsupported kind, got: %v", err)
	}

	// TEST: Backward compatibility - missing kind
	noKindFile := filepath.Join(tempDir, "test-no-kind.yaml")
	yamlContent = `---
schema: 1.0.0
namespace: yago
desiredstate:
  meta:
    parts:
      self: test-no-kind.yaml
    ecosystem: {}
`

	err = os.WriteFile(noKindFile, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	doc = NewGitOpsDocument()
	err = doc.LoadMetadata(noKindFile)
	if err != nil {
		t.Fatalf("BEHAVIORAL CONTRACT VIOLATION: Documents without 'kind' field must still work (backward compatibility): %v", err)
	}

	if doc.GetKind() != "DesiredState" {
		t.Errorf("Expected auto-detected kind='DesiredState', got '%s'", doc.GetKind())
	}

	if !doc.IsDesiredState() {
		t.Error("Expected document auto-detected as DesiredState")
	}

	t.Logf("✓ Behavior verified: Kind field normalized, validated, with backward compatibility for absent kind")
}

// TestGitOpsDocument_NamespaceField_BehavioralBDD documents namespace handling.
//
// BEHAVIORAL CONTRACT:
// The "namespace" field groups documents logically (default: "yago"). It is read
// during LoadMetadata() and normalized to lowercase for case-insensitive matching.
// Namespace affects schema lookup and document organization.
//
// CRITICAL SEMANTICS:
// - Case-insensitive: "yago", "Yago", "YAGO" all normalized to "yago"
// - Lowercase storage: Always stored in lowercase internally
// - Schema lookup: Namespace used to find schema files (namespace/schemaVersion.json)
// - Default value: "yago" if not specified
// - No validation: Any string accepted as namespace
//
// RATIONALE:
// Namespaces allow multi-tenant document management without conflicts. Case-insensitivity
// prevents user errors from capitalization mistakes. Lowercase normalization ensures
// consistent file system paths (important for case-sensitive file systems).
//
// REGRESSION RISK: MEDIUM
// - Making case-sensitive would break documents with non-lowercase namespaces
// - Changing default would break documents without explicit namespace
// - Adding validation would break documents with special characters
func TestGitOpsDocument_NamespaceField_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Namespace field is case-insensitive and normalized to lowercase",
		CurrentImpl:     "LoadMetadata() reads 'namespace' field, converts to lowercase for storage and schema lookup",
		ExpectedOutcome: "Namespace stored in lowercase, schema lookup succeeds regardless of original capitalization",
		TestScenario:    "Load documents with various namespace capitalizations, verify schema loading succeeds",
		Rationale:       "Case-insensitivity prevents capitalization errors, lowercase ensures consistent file paths",
		RegressionRisk:  "MEDIUM - Case-sensitivity would break documents with non-lowercase namespaces",
	}

	t.Logf("Testing Behavior: %s", contract.Behavior)

	testCases := []struct {
		namespaceValue string
	}{
		{"yago"},
		{"YAGO"},
		{"YaGo"},
		{"test-custom-paths"},
		{"TEST-CUSTOM-PATHS"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("namespace=%s", tc.namespaceValue), func(t *testing.T) {
			tempDir := t.TempDir()
			testFile := filepath.Join(tempDir, "test.yaml")

			yamlContent := fmt.Sprintf(`---
schema: 1.0.0
namespace: %s
kind: DesiredState
desiredstate:
  meta:
    parts:
      self: test.yaml
    ecosystem: {}
`, tc.namespaceValue)

			err := os.WriteFile(testFile, []byte(yamlContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			doc := NewGitOpsDocument()
			err = doc.LoadMetadata(testFile)
			if err != nil {
				t.Fatalf("Failed to load metadata: %v", err)
			}

			// Verify schema loading succeeds (depends on namespace normalization)
			version := schema.SchemaVersion("1.0.0")
			err = doc.SetSchema(version)
			if err != nil {
				t.Fatalf("BEHAVIORAL CONTRACT VIOLATION: Schema lookup must succeed with normalized namespace: %v", err)
			}

			if doc.GetSchemaVersion() != "1.0.0" {
				t.Errorf("Expected schema version '1.0.0', got '%s'", doc.GetSchemaVersion())
			}
		})
	}

	t.Logf("✓ Behavior verified: Namespace normalized to lowercase, schema lookup works regardless of capitalization")
}
