package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/property"
	"github.com/danieleborsaro/yago/internal/repo"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// GitOpsDocument represents a GitOps document with metadata and content handling.
//
// The struct uses TWO PropertyWrappers to separate source from output:
//
// 1. assembledMeta
//   - The original YAML file as loaded from disk
//   - NEVER modified after initial load
//   - Contains the complete root document (namespace, schema, structure)
//   - Used as reference for metadata extraction and validation
//
// 2. contentForConsumption
//   - The consumable output that wrapper tools use
//   - Modified during assembly operations
//   - Written to cache files via Cache() function
//   - Content varies by document type:
//   - DesiredState: Full assembled document (namespace + schema + desiredstate.meta + desiredstate.content)
//   - Configuration with wrapper: ONLY wrapper parts (raw terraform vars, no meta/structure)
//   - Configuration without wrapper: Full configuration structure
//
// This design provides clear separation between "what was in the file" (assembledMeta)
// and "what wrapper tools consume" (contentForConsumption).
type GitOpsDocument struct {
	// Core properties
	handler       *parser.YAMLHandler
	schemaManager *schema.SchemaManager

	// assembledMeta holds the original YAML file as loaded from disk (never modified).
	// Usage: Reference for validation, metadata extraction, starting point for assembly
	assembledMeta *property.PropertyWrapper

	// contentForConsumption holds the consumable output that gets cached and used by wrappers.
	// Usage: What gets written by Cache(), consumed by wrapper tools (terraform, concourse, etc.)
	// Content varies by document type - see struct documentation above for details.
	contentForConsumption *property.PropertyWrapper

	workdir       string
	metaFile      string
	dirname       string
	filename      string
	schemaVersion string
	kind          string
	envVariables  map[string]string

	// Repository properties (cached from repo object)
	repo             *repo.Repo
	repoOrganisation string
	repoName         string
	repoUrl          string

	// Schema and type properties
	isDesiredState  bool
	isConfiguration bool
	tmpFileSuffix   string

	// Override properties (set before loading metadata)
	namespaceOverride string
	childLoadMode     bool

	// Recursive child loading state shared across a desiredstate tree.
	recursionInProgress map[string]bool
	recursionLoaded     map[string]map[string]interface{}

	// Property paths (configurable based on schema version)
	propertyRoot                         string
	propertyMeta                         string
	propertyMetaRootPath                 string
	propertyMetaEcosystem                string
	propertyContentEcosystem             string
	propertyRepoToLoad                   string
	propertyPartsToLoad                  string
	propertyMasterPipelineIsSelfUpdating string
	propertySlavePipelinesRepoList       string
	propertyWrappers                     string
}

// NewGitOpsDocument creates a new GitOpsDocument instance
func NewGitOpsDocument() *GitOpsDocument {
	schemaManager, err := schema.GetOrCreateSchemaManagerAuto()
	if err != nil {
		// Fallback to legacy schema manager
		return &GitOpsDocument{
			handler:               parser.NewYAMLHandler("/"),
			schemaManager:         nil, // Will be set in the legacy path
			assembledMeta:         property.NewPropertyWrapper(nil, ""),
			contentForConsumption: property.NewPropertyWrapper(nil, ""),
			envVariables:          make(map[string]string),
			tmpFileSuffix:         "gitops-assembled.yaml",
		}
	}

	// Get the project root directory from the schema manager
	projectRoot := schemaManager.GetWorkDir()

	return &GitOpsDocument{
		handler:               parser.NewYAMLHandlerWithSchemaManager(projectRoot, schemaManager),
		schemaManager:         schemaManager,
		assembledMeta:         property.NewPropertyWrapperFromPath(nil, "", projectRoot),
		contentForConsumption: property.NewPropertyWrapperFromPath(nil, "", projectRoot),
		envVariables:          make(map[string]string),
		tmpFileSuffix:         "gitops-assembled.yaml",
		recursionInProgress:   make(map[string]bool),
		recursionLoaded:       make(map[string]map[string]interface{}),
	}
}

// NewGitOpsDocumentWithPath creates a new GitOpsDocument with a specific base directory.
// IMPORTANT: This always uses the main yago project's schema manager (auto-detected),
// NOT a schema manager for the baseDir. This ensures schemas are loaded only once
// from the main yago installation, not separately for each target directory.
func NewGitOpsDocumentWithPath(baseDir string) *GitOpsDocument {
	// Always use the main yago schema manager, not one specific to baseDir
	// This prevents creating multiple schema manager instances and re-loading schemas
	schemaManager, err := schema.GetOrCreateSchemaManagerAuto()
	if err != nil {
		// Fallback to legacy schema manager
		return &GitOpsDocument{
			handler:               parser.NewYAMLHandler(baseDir),
			schemaManager:         nil, // Will be set in the legacy path
			assembledMeta:         property.NewPropertyWrapperFromPath(nil, "", baseDir),
			contentForConsumption: property.NewPropertyWrapperFromPath(nil, "", baseDir),
			envVariables:          make(map[string]string),
			tmpFileSuffix:         "gitops-assembled.yaml",
			workdir:               baseDir,
		}
	}

	return &GitOpsDocument{
		handler:               parser.NewYAMLHandlerWithSchemaManager(baseDir, schemaManager),
		schemaManager:         schemaManager,
		assembledMeta:         property.NewPropertyWrapperFromPath(nil, "", baseDir),
		contentForConsumption: property.NewPropertyWrapperFromPath(nil, "", baseDir),
		envVariables:          make(map[string]string),
		tmpFileSuffix:         "gitops-assembled.yaml",
		workdir:               baseDir,
		recursionInProgress:   make(map[string]bool),
		recursionLoaded:       make(map[string]map[string]interface{}),
	}
}

// SetEnvironmentVariables sets the environment variables for this document
func (doc *GitOpsDocument) SetEnvironmentVariables(envVars map[string]string) {
	doc.envVariables = envVars
}

// GetMeta returns the assembled metadata PropertyWrapper.
// This contains the document as it was loaded from disk and is never modified after loading.
//
// For DesiredState: Contains namespace, schema, desiredstate.meta (before assembly)
// For Configuration: Contains namespace, schema, configuration.meta, configuration.content.wrappers
//
// Usage: Reference for validation, metadata extraction, source of truth
func (doc *GitOpsDocument) GetMeta() *property.PropertyWrapper {
	return doc.assembledMeta
}

// GetContent returns the consumable output used by wrapper tools and caching.
// This is what gets written by Cache() and consumed by wrapper tools (terraform, concourse, etc.).
//
// Content varies by document type:
//
// For DesiredState:
//   - Full assembled document including:
//   - namespace: yago
//   - schema: 4.2.0
//   - desiredstate.content (assembled from all parts)
//   - desiredstate.meta (metadata about parts)
//   - Example: Used by concourse to generate pipelines with full state context
//
// For Configuration with wrapper (e.g., --wrapper terraform):
//   - ONLY the wrapper-specific parts:
//   - Raw terraform variables (tfvars)
//   - NO configuration: structure wrapper
//   - NO namespace, NO schema, NO meta
//   - Example: Terraform can directly consume the output as .tfvars
//
// For Configuration without wrapper:
//   - Full configuration structure (same as assembledMeta)
//   - Used for validation and introspection
//
// Usage: Pass to wrapper tools, write to cache files, final assembly output
func (doc *GitOpsDocument) GetContent() *property.PropertyWrapper {
	return doc.contentForConsumption
}

// GetWorkdir returns the working directory
func (doc *GitOpsDocument) GetWorkdir() string {
	return doc.workdir
}

// SetCacheFilename sets the filename to use when caching the document.
// This allows customization of the output filename for different document types.
// Example: "desiredstate.tfvars.json", "configuration.tfvars.json"
func (doc *GitOpsDocument) SetCacheFilename(filename string) {
	doc.tmpFileSuffix = filename
}

// GetMetaFile returns the metadata file path
func (doc *GitOpsDocument) GetMetaFile() string {
	return doc.metaFile
}

// GetSchemaVersion returns the schema version of the document
func (doc *GitOpsDocument) GetSchemaVersion() string {
	return doc.schemaVersion
}

// GetKind returns the kind of the document
func (doc *GitOpsDocument) GetKind() string {
	return doc.kind
}

// IsDesiredState returns true if this is a desired state document
func (doc *GitOpsDocument) IsDesiredState() bool {
	return doc.isDesiredState
}

// IsConfiguration returns true if this is a configuration document
func (doc *GitOpsDocument) IsConfiguration() bool {
	return doc.isConfiguration
}

// DetectSchemaVersion detects the schema version from the loaded content
func (doc *GitOpsDocument) DetectSchemaVersion(content map[string]interface{}) (schema.SchemaVersion, error) {
	if doc.schemaManager == nil {
		return "", errors.New(errors.ErrFail, "schema manager not initialized")
	}

	version, err := doc.schemaManager.DetectSchemaVersion(content)
	if err != nil {
		return "", err
	}

	return schema.SchemaVersion(version), nil
}

// ExtractOverridesFromDesiredState extracts schema version and namespace overrides from a desiredstate document
// Returns (schemaVersionOverride, namespaceOverride, shouldOverride)
// shouldOverride is determined by the is_override_schema_in_config field (defaults to true)
func (doc *GitOpsDocument) ExtractOverridesFromDesiredState(desiredStateDoc *GitOpsDocument) (string, string, bool) {
	if desiredStateDoc == nil {
		return "", "", false
	}

	// Get the desiredstate metadata
	content := desiredStateDoc.GetMeta().Data
	if content == nil {
		return "", "", false
	}

	// Check is_override_schema_in_config field (defaults to true if missing)
	shouldOverride := true
	if dsContent, ok := content["desiredstate"].(map[string]interface{}); ok {
		if metaContent, ok := dsContent["meta"].(map[string]interface{}); ok {
			if overrideVal, exists := metaContent["is_override_schema_in_config"]; exists {
				if overrideBool, ok := overrideVal.(bool); ok {
					shouldOverride = overrideBool
				}
			}
		}
	}

	if !shouldOverride {
		return "", "", false
	}

	// Extract schema version and namespace from desiredstate
	schemaVersionOverride := desiredStateDoc.GetSchemaVersion()
	namespaceOverride := ""
	if ns, ok := content["namespace"].(string); ok {
		namespaceOverride = ns
	}

	logging.Info("Desiredstate requests override: schema=%s, namespace=%s", schemaVersionOverride, namespaceOverride)
	return schemaVersionOverride, namespaceOverride, true
}

// SetSchemaAndNamespaceOverrides sets the schema and namespace overrides for this document.
// This should be called BEFORE LoadMetadata() to ensure the overrides are applied during metadata loading.
// schemaValue and namespaceValue can be any type - they will be converted to strings as needed.
func (doc *GitOpsDocument) SetSchemaAndNamespaceOverrides(schemaValue interface{}, namespaceValue interface{}) {
	if schemaValue != nil {
		if schemaStr, ok := schemaValue.(string); ok && schemaStr != "" {
			doc.schemaVersion = schemaStr
			logging.Debug("Set schema override: %s", schemaStr)
		}
	}
	if namespaceValue != nil {
		if nsStr, ok := namespaceValue.(string); ok && nsStr != "" {
			doc.namespaceOverride = nsStr
			logging.Debug("Set namespace override: %s", nsStr)
		}
	}
}

// SetSchema configures the document properties based on the schema version
func (doc *GitOpsDocument) SetSchema(version schema.SchemaVersion) error {
	doc.schemaVersion = string(version)

	// Validate schema manager is available
	if doc.schemaManager == nil {
		return errors.New(errors.ErrFail, "schema manager not initialized - cannot discover property paths")
	}

	// Extract namespace from document if specified, otherwise default to "yago"
	// Namespace is case-insensitive - normalize to lowercase
	// Priority: CLI global override > document-level override > document field > default "yago"
	namespace := ""

	if !doc.childLoadMode {
		if cliOverride := schema.GetNamespaceOverride(); cliOverride != "" {
			// CLI --namespace flag takes highest priority
			namespace = strings.ToLower(cliOverride)
			logging.Info("Applied CLI namespace override: %s", namespace)
			// Write back so ValidateSchema and later readers see the overridden value
			if doc.assembledMeta != nil && doc.assembledMeta.Data != nil {
				doc.assembledMeta.Data["namespace"] = namespace
			}
		} else if doc.namespaceOverride != "" {
			// Document-level override (e.g. from a parent desiredstate doc)
			namespace = strings.ToLower(doc.namespaceOverride)
			logging.Info("Applied namespace override: %s", namespace)
		} else if doc.assembledMeta != nil && doc.assembledMeta.Data != nil {
			if ns, ok := doc.assembledMeta.Data["namespace"].(string); ok && ns != "" {
				originalNamespace := ns
				namespace = strings.ToLower(ns)
				logging.Info("Read namespace from document: %s -> %s", originalNamespace, namespace)
			}
		}
	} else if doc.assembledMeta != nil && doc.assembledMeta.Data != nil {
		// Child mode: child namespace is authoritative. If missing, inherit fallback.
		if ns, ok := doc.assembledMeta.Data["namespace"].(string); ok && ns != "" {
			originalNamespace := ns
			namespace = strings.ToLower(ns)
			logging.Info("Read namespace from child document: %s -> %s", originalNamespace, namespace)
		} else if doc.namespaceOverride != "" {
			namespace = strings.ToLower(doc.namespaceOverride)
			logging.Info("Child document missing namespace; inherited namespace: %s", namespace)
			doc.assembledMeta.Data["namespace"] = namespace
		}
	}
	// If namespace is still empty, DiscoverPropertyPaths will default to "yago"

	// Add kind to document if not already present (for backward compatibility)
	// If kind was present, it was already validated and set in determineDocumentTypeFromData
	if doc.kind == "" {
		// Kind was not in the document - determine it from document type and add it
		if doc.isDesiredState {
			doc.kind = "DesiredState"
		} else if doc.isConfiguration {
			doc.kind = "Configuration"
		} else {
			return errors.New(errors.ErrFail, "document type not determined - cannot set kind")
		}

		logging.Info("Kind not found in document - adding default kind: %s", doc.kind)
		// Add kind to the document data (after validation, so it won't fail)
		if doc.assembledMeta != nil && doc.assembledMeta.Data != nil {
			doc.assembledMeta.Data["kind"] = doc.kind
		}
	}

	// Discover property paths from the schema plugin
	propertyPaths, err := doc.schemaManager.DiscoverPropertyPaths(namespace, version, doc.isDesiredState)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to discover property paths from schema")
	}

	// Set all property paths from the schema definition using getter methods
	// DiscoverPropertyPaths has already validated these paths exist
	if doc.propertyRoot, err = propertyPaths.Root(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to get root property path")
	}
	if doc.propertyMeta, err = propertyPaths.Meta(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to get meta property path")
	}
	if doc.propertyMetaRootPath, err = propertyPaths.MetaRootPath(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to get metaRootPath property path")
	}
	if doc.isDesiredState {
		if doc.propertyMetaEcosystem, err = propertyPaths.MetaEcosystem(); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to get metaEcosystem property path")
		}
		if doc.propertyContentEcosystem, err = propertyPaths.ContentEcosystem(); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to get contentEcosystem property path")
		}
	}
	if doc.propertyRepoToLoad, err = propertyPaths.RepoToLoad(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to get repoToLoad property path")
	}
	if doc.propertyPartsToLoad, err = propertyPaths.PartsToLoad(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to get partsToLoad property path")
	}
	if doc.isDesiredState {
		if doc.propertyMasterPipelineIsSelfUpdating, err = propertyPaths.MasterPipelineIsSelfUpdating(); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to get masterPipelineIsSelfUpdating property path")
		}
		if doc.propertySlavePipelinesRepoList, err = propertyPaths.SlavePipelinesRepoList(); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to get slavePipelinesRepoList property path")
		}
	}

	// wrappers is optional — not all schemas define it; fall back to {root}.content.wrappers
	if wrappersPath, pathErr := propertyPaths.Get("wrappers"); pathErr == nil {
		doc.propertyWrappers = wrappersPath
	} else {
		doc.propertyWrappers = fmt.Sprintf("%s.content.wrappers", doc.propertyRoot)
	}

	return nil
}

// LoadMetadata loads the root metadata file
func (doc *GitOpsDocument) LoadMetadata(pathToRoot string) error {
	// Normalize the path
	pathToRoot = filepath.Clean(pathToRoot)

	logging.Debug("LoadMetadata called for: %s", pathToRoot)

	// Check if file exists
	if _, err := os.Stat(pathToRoot); os.IsNotExist(err) {
		return errors.Newf(errors.ErrDesiredStateMissing, "metadata file not found: %s", pathToRoot)
	}

	// Load the file into meta wrapper
	err := doc.assembledMeta.LoadFile(pathToRoot, doc.envVariables)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load metadata file %s", pathToRoot)
	}

	// Determine document type BEFORE validation (check for kind field or keys in loaded data)
	err = doc.determineDocumentTypeFromData(doc.assembledMeta.Data)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to determine document type")
	}

	// Detect schema version if not already set
	var schemaVersion schema.SchemaVersion
	if doc.schemaVersion == "" {
		version, err := doc.DetectSchemaVersion(doc.assembledMeta.Data)
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to detect schema version")
		}
		schemaVersion = version
	} else {
		// Schema version was set earlier (e.g., via override)
		schemaVersion = schema.SchemaVersion(doc.schemaVersion)
	}

	// Always call SetSchema after loading metadata to ensure namespace is read correctly
	// This is important because namespace is in the loaded document data
	err = doc.SetSchema(schemaVersion)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to set schema")
	}

	// Validate the metadata using the new validation methods
	logging.Debug("Validating metadata (isDesiredState: %v)", doc.isDesiredState)
	if doc.isDesiredState {
		err = doc.ValidateDesiredStateMeta()
	} else {
		err = doc.ValidateConfigurationMeta()
	}
	if err != nil {
		return err // Error already logged in validation method
	}

	// Set file information
	doc.metaFile = pathToRoot
	// NOTE: workdir should remain the git repo root, not be overridden to the file's directory
	doc.dirname = filepath.Dir(pathToRoot)
	doc.filename = filepath.Base(pathToRoot)

	// Compute workdir by comparing actual path with meta.parts.self
	err = doc.computeWorkdir(pathToRoot)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to compute workdir")
	}

	// NOTE: Repo initialization moved to LoadGitOpsFile() to avoid duplication.
	// LoadGitOpsFile() always calls LoadMetadata() first, then creates the Repo.
	// Standalone LoadMetadata() calls (e.g., for configuration loading) don't need repo.

	return nil
}

// SetWrapperForConfiguration sets the wrapper-specific parts path for configuration documents
func (doc *GitOpsDocument) SetWrapperForConfiguration(wrapper string) {
	if !doc.isConfiguration {
		return // Only applies to configuration documents
	}

	if wrapper != "" && wrapper != "meta" {
		// Build the wrapper parts path from the schema-defined wrappers path
		doc.propertyPartsToLoad = fmt.Sprintf("%s.%s.parts", doc.propertyWrappers, wrapper)
	} else {
		// For meta or no wrapper, use default parts location
		doc.propertyPartsToLoad = "meta.parts"
	}
}

// GetPropertyPartsToLoad returns the current parts path being used
func (doc *GitOpsDocument) GetPropertyPartsToLoad() string {
	return doc.propertyPartsToLoad
}

// ValidateWrapper validates that a specified wrapper is supported by the current schema version.
//
// This method implements FAIL-FAST validation: wrapper validation failures immediately
// return an error with detailed diagnostic information, preventing further processing
// with invalid configurations.
//
// The method performs case-insensitive validation against the schema's x-gitops-wrappers list,
// ensuring that user-provided wrapper names (in any case) are properly validated.
//
// Parameters:
//   - wrapper: The wrapper name to validate (e.g., "terraform", "concourse", "meta")
//     Empty string is allowed and skips validation (uses default behavior)
//
// Returns:
//   - nil: Wrapper is valid or empty (empty wrapper uses defaults)
//   - error: Wrapper is invalid, schema manager not initialized, or schema version not set
//
// Validation Rules:
//  1. Empty wrapper: Returns nil (uses default "meta.parts" path)
//  2. Schema manager missing: Returns error (cannot validate without schema)
//  3. Schema version missing: Returns error (version required for validation)
//  4. Wrapper not in schema's x-gitops-wrappers list: Returns detailed error with supported list
//  5. Wrapper supported: Returns nil and logs success
//
// Case Handling:
// The validation is case-insensitive:
//   - Input "Terraform" → normalized to "terraform" → validated
//   - Input "TERRAFORM" → normalized to "terraform" → validated
//   - Input "terraform" → no change → validated
//
// Error Messages:
// Error messages include the full list of supported wrappers to help users correct their input:
//
//	"wrapper 'invalid' is not supported by schema 1.0.0. Supported wrappers: [meta terraform concourse]"
//
// Example Usage:
//
//	// Valid wrapper
//	err := doc.ValidateWrapper("terraform")
//	if err != nil {
//	    return fmt.Errorf("wrapper validation failed: %w", err)
//	}
//
//	// Invalid wrapper (returns error)
//	err := doc.ValidateWrapper("unsupported")
//	// Error: "wrapper 'unsupported' is not supported by schema 1.0.0. Supported wrappers: [...]"
//
//	// Empty wrapper (uses defaults)
//	err := doc.ValidateWrapper("")
//	// Returns nil, no error
//
// Side Effects:
//   - Logs warning for empty wrapper
//   - Logs debug message for successful validation
//   - Logs error with supported wrapper list for validation failures
//
// Thread Safety:
// This method is safe for concurrent use as it only reads from the schema manager,
// which implements thread-safe caching for wrapper discovery.
func (doc *GitOpsDocument) ValidateWrapper(wrapper string) error {
	if wrapper == "" {
		logging.Warn("Wrapper not specified - using default")
		return nil
	}

	if doc.schemaManager == nil {
		return errors.New(errors.ErrFail, "VALIDATION ERROR: schema manager not initialized, cannot validate wrapper")
	}

	// Get schema version
	if doc.schemaVersion == "" {
		return errors.New(errors.ErrFail, "VALIDATION ERROR: schema version not set, cannot validate wrapper")
	}

	// Normalize wrapper to lowercase
	wrapperLower := strings.ToLower(wrapper)

	// Check if wrapper is supported by this schema version
	if !doc.schemaManager.IsWrapperSupported(schema.SchemaVersion(doc.schemaVersion), wrapperLower) {
		supportedWrappers := doc.schemaManager.GetSupportedWrappers(schema.SchemaVersion(doc.schemaVersion))
		logging.Error("Wrapper '%s' is not supported by schema %s. Supported wrappers: %v",
			wrapper, doc.schemaVersion, supportedWrappers)
		return errors.Newf(errors.ErrParam, "wrapper '%s' is not supported by schema %s. Supported wrappers: %v",
			wrapper, doc.schemaVersion, supportedWrappers)
	}

	logging.Debug("Wrapper '%s' validated successfully for schema %s", wrapperLower, doc.schemaVersion)
	return nil
}

// ValidateDesiredStateMeta validates the desiredstate metadata against the schema
// Returns error on validation failure, which propagates up to fail-fast at main()
func (doc *GitOpsDocument) ValidateDesiredStateMeta() error {
	if !doc.isDesiredState {
		logging.Error("Validation failed: document is not a desiredstate (schema: %s)", doc.schemaVersion)
		return errors.New(errors.ErrParam, "document is not a desiredstate")
	}

	if doc.assembledMeta == nil || doc.assembledMeta.Data == nil {
		logging.Error("Validation failed: desiredstate metadata not loaded")
		return errors.NewDesiredStateMalformedError("desiredstate metadata not loaded")
	}

	if doc.schemaManager == nil {
		logging.Error("Validation failed: schema manager not initialized")
		return errors.New(errors.ErrFail, "schema manager not initialized")
	}

	// Validate using schema manager (will log its own errors if validation fails)
	err := doc.schemaManager.ValidateSchema(
		doc.assembledMeta.Data,
		schema.SchemaVersion(doc.schemaVersion),
		schema.SchemaTypeDesiredStateMeta,
	)

	if err != nil {
		// Schema manager already logged details, just return wrapped error
		return errors.Wrapf(errors.ErrParse, err, "desiredstate meta validation failed")
	}

	logging.Info("✓ Desiredstate metadata validated (schema: %s)", doc.schemaVersion)
	return nil
}

// ValidateDesiredStateAssembled validates the assembled desiredstate document against the schema
// Returns error on validation failure, which propagates up to fail-fast at main()
func (doc *GitOpsDocument) ValidateDesiredStateAssembled() error {
	if !doc.isDesiredState {
		logging.Error("Validation failed: document is not a desiredstate (schema: %s)", doc.schemaVersion)
		return errors.New(errors.ErrParam, "document is not a desiredstate")
	}

	if doc.contentForConsumption == nil || doc.contentForConsumption.Data == nil {
		logging.Error("Validation failed: assembled desiredstate content not loaded")
		return errors.NewDesiredStateMalformedError("assembled content not loaded")
	}

	if doc.schemaManager == nil {
		logging.Error("Validation failed: schema manager not initialized")
		return errors.New(errors.ErrFail, "schema manager not initialized")
	}

	// Validate using schema manager (will log its own errors if validation fails)
	err := doc.schemaManager.ValidateSchema(
		doc.contentForConsumption.Data,
		schema.SchemaVersion(doc.schemaVersion),
		schema.SchemaTypeDesiredStateAssembled,
	)

	if err != nil {
		// Schema manager already logged details, just return wrapped error
		return errors.Wrapf(errors.ErrParse, err, "desiredstate assembled validation failed")
	}

	logging.Info("✓ Desiredstate assembled document validated (schema: %s)", doc.schemaVersion)
	return nil
}

// ValidateConfigurationMeta validates the configuration metadata against the schema
// Returns error on validation failure, which propagates up to fail-fast at main()
func (doc *GitOpsDocument) ValidateConfigurationMeta() error {
	if !doc.isConfiguration {
		logging.Error("Validation failed: document is not a configuration (schema: %s)", doc.schemaVersion)
		return errors.New(errors.ErrParam, "document is not a configuration")
	}

	if doc.assembledMeta == nil || doc.assembledMeta.Data == nil {
		logging.Error("Validation failed: configuration metadata not loaded")
		return errors.NewConfigurationMalformedError("configuration metadata not loaded")
	}

	if doc.schemaManager == nil {
		logging.Error("Validation failed: schema manager not initialized")
		return errors.New(errors.ErrFail, "schema manager not initialized")
	}

	// Validate using schema manager (will log its own errors if validation fails)
	err := doc.schemaManager.ValidateSchema(
		doc.assembledMeta.Data,
		schema.SchemaVersion(doc.schemaVersion),
		schema.SchemaTypeConfigurationMeta,
	)

	if err != nil {
		// Schema manager already logged details, just return wrapped error
		return errors.Wrapf(errors.ErrParse, err, "configuration meta validation failed")
	}

	logging.Info("✓ Configuration metadata validated (schema: %s)", doc.schemaVersion)
	return nil
}

// ValidateConfigurationContent validates the configuration content against the schema
// Returns error on validation failure, which propagates up to fail-fast at main()
func (doc *GitOpsDocument) ValidateConfigurationContent() error {
	if !doc.isConfiguration {
		logging.Error("Validation failed: document is not a configuration (schema: %s)", doc.schemaVersion)
		return errors.New(errors.ErrParam, "document is not a configuration")
	}

	if doc.contentForConsumption == nil || doc.contentForConsumption.Data == nil {
		logging.Error("Validation failed: configuration content not loaded")
		return errors.NewConfigurationMalformedError("configuration content not loaded")
	}

	if doc.schemaManager == nil {
		logging.Error("Validation failed: schema manager not initialized")
		return errors.New(errors.ErrFail, "schema manager not initialized")
	}

	// Validate using schema manager (will log its own errors if validation fails)
	err := doc.schemaManager.ValidateSchema(
		doc.contentForConsumption.Data,
		schema.SchemaVersion(doc.schemaVersion),
		schema.SchemaTypeConfigurationContent,
	)

	if err != nil {
		// Schema manager already logged details, just return wrapped error
		return errors.Wrapf(errors.ErrParse, err, "configuration content validation failed")
	}

	logging.Info("✓ Configuration content validated (schema: %s)", doc.schemaVersion)
	return nil
}

// UpdateRepoProperties caches repository properties from repo object into document fields
// This allows quick access to repo metadata without going through the repo object
func (doc *GitOpsDocument) UpdateRepoProperties() error {
	if doc.repo == nil {
		return errors.New(errors.ErrFail, "repo object not initialized")
	}

	// Verify workdir exists
	if doc.repo.WorkDir == "" {
		return errors.New(errors.ErrFail, "repo workdir not set")
	}

	if _, err := os.Stat(doc.repo.WorkDir); os.IsNotExist(err) {
		logging.Error("Unable to read workdir: '%s'", doc.repo.WorkDir)
		return errors.Newf(errors.ErrFail, "unable to read workdir: '%s'", doc.repo.WorkDir)
	}

	// Cache repo properties
	doc.workdir = doc.repo.WorkDir
	doc.repoOrganisation = doc.repo.Organisation
	doc.repoName = doc.repo.Name
	doc.repoUrl = doc.repo.URL

	logging.Debug("Updated repo properties: workdir=%s, org=%s, name=%s, url=%s",
		doc.workdir, doc.repoOrganisation, doc.repoName, doc.repoUrl)

	return nil
}

// ParseDesiredStateRef parses a desiredstate reference from a repository configuration dictionary
// and returns the absolute path to the desiredstate file and the relative path within the repo
//
// Input dictionary format:
//
//	{
//	  "url": "git@github.com:Org/repo.git",
//	  "ref": "main",
//	  "path": "path/to/desiredstate.yaml",
//	  "watch": ["path/**/*"]
//	}
//
// Returns: (absolutePath, relativePath, error)
func ParseDesiredStateRef(dictionary map[string]interface{}, schemaManager *schema.SchemaManager) (string, string, error) {
	logging.Debug("Parsing desiredstate reference from dictionary")

	// Create repo instance - this automatically parses URL, ref, path and clones the repo
	thisRepo, err := repo.NewRepoFromDesiredState(dictionary, "", "", "", nil)
	if err != nil {
		return "", "", errors.Wrapf(errors.ErrParse, err, "failed to create and clone repo")
	}

	// Get the relative path (path field from dictionary)
	relativePath := thisRepo.Path
	if relativePath == "" {
		return "", "", errors.New(errors.ErrParam, "path not specified in desiredstate reference")
	}

	// Construct absolute path
	absolutePath := filepath.Join(thisRepo.WorkDir, relativePath)

	logging.Debug("Parsed desiredstate ref: absolute=%s, relative=%s", absolutePath, relativePath)

	return absolutePath, relativePath, nil
}

// computeWorkdir computes the working directory by comparing the actual file path
// with the meta.parts.self reference
func (doc *GitOpsDocument) computeWorkdir(pathToRoot string) error {
	// Get the meta.parts.self value
	metaRootFile, err := doc.GetMetaRootFile()
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to get meta root file")
	}

	// Compute relative path from meta.parts.self
	relPath := filepath.Dir(metaRootFile)

	// Compute absolute path from actual file
	absPath := filepath.Dir(pathToRoot)

	// Compute workdir by removing the relative path from the absolute path
	if len(relPath) > 0 && strings.HasSuffix(absPath, relPath) {
		doc.workdir = strings.TrimSuffix(absPath, relPath)
		doc.workdir = strings.TrimRight(doc.workdir, "/")
		// Ensure we don't end up with an empty workdir
		if doc.workdir == "" {
			doc.workdir = "."
		}
	} else {
		// Fallback: use the directory containing the file
		doc.workdir = absPath
	}

	return nil
}

// assembleChildDesiredStateMeta loads a child desiredstate root file and merges all its referenced
// parts (excluding "self"), returning a fully assembled map for watch list traversal.
func (doc *GitOpsDocument) assembleChildDesiredStateMeta(rootFile string) (map[string]interface{}, error) {
	rootContent, err := ParsePart(doc.workdir, rootFile, doc.envVariables)
	if err != nil {
		return nil, err
	}

	assembled := property.NewPropertyWrapperFromPath(rootContent, "", doc.workdir)

	partsVal, err := assembled.GetValue("meta.parts")
	if err != nil {
		return assembled.Data, nil
	}

	partsMap, ok := partsVal.(map[string]interface{})
	if !ok || len(partsMap) == 0 {
		return assembled.Data, nil
	}

	for mountPoint, partFileVal := range partsMap {
		if mountPoint == "self" {
			continue
		}
		partFile, ok := partFileVal.(string)
		if !ok {
			continue
		}
		partContent, err := ParsePart(doc.workdir, partFile, doc.envVariables)
		if err != nil {
			logging.Debug("Skipping child part file %s during watch list assembly: %v", partFile, err)
			continue
		}
		assembled.MergeKeys(partContent)
	}

	return assembled.Data, nil
}

// ParsePart parses a single part file and returns its content
func ParsePart(workdir, partFile string, envVariables map[string]string) (map[string]interface{}, error) {
	fullPath := filepath.Join(workdir, partFile)

	// Create a temporary handler for this operation
	handler := parser.NewYAMLHandler(workdir)

	content, err := handler.LoadFile(fullPath, envVariables)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to parse part file %s", fullPath)
	}

	return content, nil
}

// ResolveRelativePath resolves a relative path from a document to an absolute path.
// This is used for parts files, backend files, and any other relative references in GitOps documents.
// The path is resolved relative to the directory containing the root document file.
//
// Parameters:
//   - rootDocumentPath: absolute path to the root document file (desiredstate.yaml or configuration.yaml)
//   - relativePath: relative path from the document (e.g., "cicd/terraform/backends/eu-west-1.tfvars.json")
//
// Returns the absolute path by joining the document's directory with the relative path.
//
// Example:
//
//	rootDocumentPath = "/path/to/gitops-configurations-shared/service/configuration.yaml"
//	relativePath = "cicd/terraform/backends/eu-west-1.tfvars.json"
//	returns: "/path/to/gitops-configurations-shared/service/cicd/terraform/backends/eu-west-1.tfvars.json"
func ResolveRelativePath(rootDocumentPath string, relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	documentDir := filepath.Dir(rootDocumentPath)
	return filepath.Join(documentDir, relativePath)
}

// GetTargetRootFile determines the target root file from metadata
func (doc *GitOpsDocument) GetTargetRootFile() (string, error) {
	// Get the root file path from metadata
	rootPath, err := doc.assembledMeta.GetValue(doc.propertyMetaRootPath)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to get target root file path")
	}

	if rootPathStr, ok := rootPath.(string); ok {
		return rootPathStr, nil
	}

	return "", errors.New(errors.ErrParse, "target root file path is not a string")
}

// GetMetaRootFile returns the metadata root file path
func (doc *GitOpsDocument) GetMetaRootFile() (string, error) {
	return doc.GetTargetRootFile()
}

// FindAllPartsToLoad finds all parts that need to be loaded
func (doc *GitOpsDocument) FindAllPartsToLoad(yamlPathToParts string) ([]map[string]interface{}, error) {
	var fullPath string

	// Handle different path types based on where parts are located
	if strings.HasPrefix(yamlPathToParts, doc.propertyRoot+".") {
		// Path is already absolute (includes the document root prefix)
		fullPath = yamlPathToParts
	} else if strings.HasPrefix(yamlPathToParts, "content.") {
		// Parts are in the content section (e.g., configuration.content.wrappers.terraform.parts)
		fullPath = fmt.Sprintf("%s.%s", doc.propertyRoot, yamlPathToParts)
	} else if strings.HasPrefix(yamlPathToParts, "meta.") {
		// Parts are in the meta section (standard case)
		// Remove the "meta." prefix since doc.propertyMeta already includes "meta"
		partsSuffix := strings.TrimPrefix(yamlPathToParts, "meta.")
		fullPath = fmt.Sprintf("%s.%s", doc.propertyMeta, partsSuffix)
	} else {
		// Assume it's a meta-relative path
		fullPath = fmt.Sprintf("%s.%s", doc.propertyMeta, yamlPathToParts)
	}

	// Get the parts section directly
	partsValue, err := doc.assembledMeta.GetValue(fullPath)
	if err != nil {
		// Check if this is a wrapper not found error and provide helpful suggestions
		if doc.isConfiguration && strings.Contains(fullPath, "wrappers.") {
			return nil, doc.createWrapperNotFoundError(fullPath, err)
		}
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to find parts at path %s", fullPath)
	}

	// The parts should be a map[string]interface{} where keys are mount points and values are file paths
	if partsMap, ok := partsValue.(map[string]interface{}); ok {
		result := []map[string]interface{}{partsMap}
		return result, nil
	}

	return nil, errors.Newf(errors.ErrParse, "parts section at %s is not a valid map structure", fullPath)
}

// createWrapperNotFoundError creates a user-friendly error when a wrapper is not found
func (doc *GitOpsDocument) createWrapperNotFoundError(fullPath string, originalErr error) error {
	// Extract the wrapper name from the path (e.g., configuration.content.wrappers.nonexistent.parts)
	pathParts := strings.Split(fullPath, ".")
	var wrapperName string
	for i, part := range pathParts {
		if part == "wrappers" && i+1 < len(pathParts) {
			wrapperName = pathParts[i+1]
			break
		}
	}

	// Get available wrappers
	availableWrappers := doc.getAvailableWrappers()

	if len(availableWrappers) > 0 {
		return errors.Newf(errors.ErrParam, "wrapper '%s' not found in configuration. Available wrappers: [%s]",
			wrapperName, strings.Join(availableWrappers, ", "))
	}

	return errors.Newf(errors.ErrParam, "wrapper '%s' not found in configuration. No wrappers are defined in this configuration", wrapperName)
}

// getAvailableWrappers returns a list of available wrapper names from the configuration
func (doc *GitOpsDocument) getAvailableWrappers() []string {
	var wrappers []string

	// Try to get the wrappers section
	wrappersValue, err := doc.assembledMeta.GetValue(doc.propertyWrappers)
	if err != nil {
		return wrappers
	}

	if wrappersMap, ok := wrappersValue.(map[string]interface{}); ok {
		for wrapperName := range wrappersMap {
			wrappers = append(wrappers, wrapperName)
		}
	}

	return wrappers
}

// GetAllPartFiles retrieves all part file paths for loading
func (doc *GitOpsDocument) GetAllPartFiles(yamlPathToParts string) ([]map[string]string, error) {
	partBlocks, err := doc.FindAllPartsToLoad(yamlPathToParts)
	if err != nil {
		return nil, err
	}

	var finalFilesList []map[string]string

	for _, block := range partBlocks {
		// Each block is the parts map where keys are mount points and values are file paths
		for mountPoint, value := range block {
			if partFile, ok := value.(string); ok {
				// For desiredstate documents, skip the 'self' part as it's the main file we already loaded
				// For configuration documents, include all parts including 'self'
				if doc.isDesiredState && mountPoint == "self" {
					continue
				}

				finalFilesList = append(finalFilesList, map[string]string{
					mountPoint: partFile,
				})
			}
		}
	}

	return finalFilesList, nil
}

// LoadParts assembles and loads data from files specified in the meta configuration.
// This function populates contentForConsumption differently based on document type.
//
// Behavior by document type:
//
// For DesiredState:
//  1. Start with assembledMeta.Data as base (includes namespace, schema, desiredstate.meta)
//  2. Load each part file (which contains full structure: desiredstate.content)
//  3. Merge parts at ROOT level (desiredstate in part merges with desiredstate in base)
//  4. Result: contentForConsumption = full assembled document
//     - namespace: yago
//     - schema: 4.2.0
//     - desiredstate.content (merged from all parts)
//     - desiredstate.meta (from root file)
//  5. Validate assembled document against schema
//
// For Configuration with wrapper parts:
//  1. Collect all wrapper-specific part files
//  2. Load and merge them (raw wrapper data - terraform vars, etc.)
//  3. Result: contentForConsumption = ONLY merged wrapper parts
//     - NO configuration: wrapper
//     - NO namespace, NO schema, NO meta
//     - Just raw data that wrapper tools can directly consume
//  4. Skip validation (wrapper tools validate their own formats)
//
// For Configuration without parts:
//  1. Result: contentForConsumption = full configuration structure (same as assembledMeta)
//  2. Validate against configuration schema
//
// The key difference: DesiredState includes structure and meta, Configuration extracts only parts.
func (doc *GitOpsDocument) LoadParts() error {
	// Reset both content fields
	doc.contentForConsumption = property.NewPropertyWrapperFromPath(nil, "", doc.workdir)
	doc.contentForConsumption = property.NewPropertyWrapperFromPath(nil, "", doc.workdir)

	// Get all part files
	partFiles, err := doc.GetAllPartFiles(doc.propertyPartsToLoad)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to get part files")
	}

	if doc.isDesiredState {
		// DesiredState Assembly:
		// contentForConsumption = full assembled document (namespace + schema + desiredstate.meta + desiredstate.content)
		doc.contentForConsumption.Data = doc.deepCopyMap(doc.assembledMeta.Data)

		// Load and merge each part directly (not under mount point keys)
		for _, partCombo := range partFiles {
			for _, partFile := range partCombo {
				// Parse the part file
				partContent, err := ParsePart(doc.workdir, partFile, doc.envVariables)
				if err != nil {
					return errors.Wrapf(errors.ErrParse, err, "failed to parse part file %s", partFile)
				}

				// Parts contain full structure (desiredstate.content) and merge at root level
				// Example: desiredstate.content in part merges with desiredstate.content in base
				doc.contentForConsumption.MergeKeys(partContent)
			}
		}

		// For DesiredState, contentForConsumption is the same as rootAssembled
		doc.contentForConsumption.Data = doc.deepCopyMap(doc.contentForConsumption.Data)

		// Validate the assembled desired state using validation method
		logging.Debug("Validating assembled desiredstate")
		err = doc.ValidateDesiredStateAssembled()
		if err != nil {
			return err // Error already logged in validation method
		}

		// Finalize assembled desiredstate by removing duplicate ecosystem definition
		// under meta when the assembled ecosystem path differs.
		doc.cleanupAssembledDesiredStateEcosystem()

	} else {
		// Configuration Assembly:
		// contentForConsumption = ONLY wrapper parts (raw terraform vars, etc.)
		// NO configuration structure, NO namespace, NO schema, NO meta
		var fileList []string
		for _, partCombo := range partFiles {
			for _, partFile := range partCombo {
				fullPath := filepath.Join(doc.workdir, partFile)
				fileList = append(fileList, fullPath)
			}
		}

		if len(fileList) > 0 {
			// Load all part files and merge them
			mergedContent, err := doc.handler.LoadFiles(fileList, doc.envVariables)
			if err != nil {
				return errors.Wrapf(errors.ErrParse, err, "failed to load configuration parts")
			}

			// Configuration with wrapper parts:
			// contentForConsumption = ONLY the merged wrapper parts (raw data)
			// This is what wrapper tools (terraform, etc.) directly consume
			doc.contentForConsumption.Data = mergedContent
		} else {
			// No parts found, use the full configuration structure for validation
			doc.contentForConsumption.Data = doc.deepCopyMap(doc.assembledMeta.Data)
		}

		// Validate the configuration structure (metadata + wrapper structure)
		// but skip validation of wrapper parts content since those are wrapper-specific documents
		if len(fileList) == 0 {
			// Only validate when we have the original configuration structure (no parts loaded)
			// This validates the configuration.content.wrappers.*.parts and backends structure
			logging.Debug("Validating configuration content")
			err = doc.ValidateConfigurationContent()
			if err != nil {
				return err // Error already logged in validation method
			}
		}
		// Note: When parts are loaded (len(fileList) > 0), doc.contentForConsumption.Data contains raw
		// wrapper content (terraform vars, etc.) which should be validated by wrapper tools,
		// not against the GitOps configuration schema.
	}

	return nil
}

// LoadGitOpsFile loads a GitOps file with all its parts
// If desiredStateDoc is provided, it will extract and apply schema/namespace overrides from it
func (doc *GitOpsDocument) LoadGitOpsFile(pathToRoot string, isAssembleParts bool, desiredStateDoc *GitOpsDocument) error {
	if doc.recursionInProgress == nil {
		doc.recursionInProgress = make(map[string]bool)
	}
	if doc.recursionLoaded == nil {
		doc.recursionLoaded = make(map[string]map[string]interface{})
	}

	// Normalize path
	pathToRoot = filepath.Clean(pathToRoot)

	// Check if file exists
	if _, err := os.Stat(pathToRoot); os.IsNotExist(err) {
		return errors.Newf(errors.ErrDesiredStateMissing, "resource root file not found: %s", pathToRoot)
	}

	// Extract overrides from desiredstate document if provided
	var schemaVersionOverride, namespaceOverride string
	if desiredStateDoc != nil && !doc.childLoadMode {
		apiOverride, nsOverride, shouldOverride := doc.ExtractOverridesFromDesiredState(desiredStateDoc)
		if shouldOverride {
			schemaVersionOverride = apiOverride
			namespaceOverride = nsOverride
		}
	}

	// Set schema version if override is provided
	if schemaVersionOverride != "" {
		doc.SetSchema(schema.SchemaVersion(schemaVersionOverride))
		logging.Info("Applied schema version override: %s", schemaVersionOverride)
	}

	// Set namespace override if provided (will be used by SetSchema during metadata loading)
	if namespaceOverride != "" {
		doc.namespaceOverride = namespaceOverride
	}

	// Save workdir if already set (e.g., by CloneRepoAndLoad)
	// LoadMetadata will recompute it, but we want to know if it was pre-set
	savedWorkdir := doc.workdir

	// Load metadata
	err := doc.LoadMetadata(pathToRoot)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load metadata")
	}

	// Apply namespace override to assembledMeta if it was set
	// This ensures the metadata has the correct namespace value
	if namespaceOverride != "" {
		if doc.assembledMeta != nil && doc.assembledMeta.Data != nil {
			doc.assembledMeta.Data["namespace"] = namespaceOverride
		}
	}

	// Document type is already determined in LoadMetadata()

	// Create Repo instance only if workdir wasn't pre-set
	// This avoids duplicate repo initialization when called from CloneRepoAndLoad
	if savedWorkdir == "" {
		gitRepo, err := repo.NewRepoFromWorkDir(doc.workdir, "", nil)
		if err != nil {
			// Log warning but don't fail - we can continue without repo info
			logging.Warn("Failed to create Repo instance for workdir %s: %v", doc.workdir, err)
		} else {
			// Successfully created Repo - it will have detected git or non-git
			// Store repo reference and update properties
			doc.repo = gitRepo
			if err := doc.UpdateRepoProperties(); err != nil {
				logging.Warn("Failed to update repo properties: %v", err)
			} else {
				logging.Debug("Repo initialized for workdir: %s (isGit: %v)", doc.workdir, gitRepo.IsGitRepo())
			}
		}
	} else {
		logging.Debug("Skipping repo initialization - workdir was pre-set by caller (e.g., CloneRepoAndLoad)")
	}

	// Calculate relative paths
	relPath, err := filepath.Rel(doc.workdir, pathToRoot)
	if err != nil {
		relPath = pathToRoot
	}
	doc.dirname = filepath.Dir(relPath)
	doc.filename = filepath.Base(pathToRoot)

	// Load ecosystem (sub-desired states)
	err = doc.LoadEcosystem()
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load ecosystem")
	}

	// Assemble parts if requested
	if isAssembleParts {
		err = doc.LoadParts()
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to load parts")
		}

		// Child-load mode is used for embedded ecosystem desiredstates. Keep those
		// documents boundary-isolated by not attaching an extended watch list at
		// child assembly time; only the top-level document computes and sets it.
		if !doc.childLoadMode {
			err = doc.LoadWatchList()
			if err != nil {
				return errors.Wrapf(errors.ErrParse, err, "failed to load watch list")
			}
		}
	}

	return nil
}

// determineDocumentType determines if this is a desired state or configuration
func (doc *GitOpsDocument) determineDocumentType() {
	// Check for desired state indicators in the metadata
	if doc.assembledMeta.HasKey("desiredstate") {
		doc.isDesiredState = true
		doc.isConfiguration = false
	} else if doc.assembledMeta.HasKey("configuration") {
		doc.isDesiredState = false
		doc.isConfiguration = true
	}
}

// determineDocumentTypeFromData determines document type from raw YAML data
// Priority: 1) kind field (formal), 2) desiredstate/configuration keys (backward compatible)
func (doc *GitOpsDocument) determineDocumentTypeFromData(data map[string]interface{}) error {
	// First, check if 'kind' field is present (formal document type specification)
	if kindValue, hasKind := data["kind"]; hasKind {
		if kindStr, ok := kindValue.(string); ok && kindStr != "" {
			// Normalize kind to lowercase for case-insensitive comparison
			kindLower := strings.ToLower(kindStr)

			switch kindLower {
			case "desiredstate":
				doc.isDesiredState = true
				doc.isConfiguration = false
				doc.kind = "DesiredState" // Normalize to canonical form
				logging.Info("Read kind from document: %s -> %s", kindStr, doc.kind)
				return nil
			case "configuration":
				doc.isDesiredState = false
				doc.isConfiguration = true
				doc.kind = "Configuration" // Normalize to canonical form
				logging.Info("Read kind from document: %s -> %s", kindStr, doc.kind)
				return nil
			default:
				return errors.Newf(errors.ErrParam, "unsupported kind '%s': must be 'DesiredState' or 'Configuration' (case-insensitive)", kindStr)
			}
		}
	}

	// Fall back to key-based detection (backward compatible)
	if _, hasDesiredState := data["desiredstate"]; hasDesiredState {
		doc.isDesiredState = true
		doc.isConfiguration = false
		logging.Info("Kind not found in document - will determine from document structure (desiredstate)")
		// Kind not specified in document - will be added after validation
		return nil
	} else if _, hasConfiguration := data["configuration"]; hasConfiguration {
		doc.isDesiredState = false
		doc.isConfiguration = true
		logging.Info("Kind not found in document - will determine from document structure (configuration)")
		// Kind not specified in document - will be added after validation
		return nil
	}

	return errors.New(errors.ErrParse, "unable to determine document type: no 'kind' field or 'desiredstate'/'configuration' key found")
}

// LoadEcosystem loads sub-desired states from ecosystem configuration
func (doc *GitOpsDocument) LoadEcosystem() error {
	// Ecosystem is a desiredstate-only concept. Configuration documents do not define
	// an ecosystem path and treating an empty path as root can misinterpret top-level
	// keys (for example 'configuration') as ecosystem mount points.
	if !doc.isDesiredState || doc.propertyMetaEcosystem == "" {
		return nil
	}

	// Get ecosystem configuration
	ecosystem, err := doc.assembledMeta.GetValue(doc.propertyMetaEcosystem)
	if err != nil {
		// No ecosystem configured, which is fine
		return nil
	}

	if ecosystemMap, ok := ecosystem.(map[string]interface{}); ok && len(ecosystemMap) > 0 {
		return doc.loadEcosystemMapOfRepos(ecosystemMap)
	}

	return nil
}

// loadEcosystemMapOfRepos loads ecosystem from repository map
func (doc *GitOpsDocument) loadEcosystemMapOfRepos(subEnvMap map[string]interface{}) error {
	ecosystemContent := make(map[string]interface{})
	effectiveNamespace := doc.getEffectiveNamespace()

	for mountPoint, envRef := range subEnvMap {
		if envRefMap, ok := envRef.(map[string]interface{}); ok {
			childRootPath, err := doc.resolveChildDesiredStatePath(envRefMap)
			if err != nil {
				return errors.Wrapf(errors.ErrParse, err, "failed to resolve ecosystem mount '%s'", mountPoint)
			}

			normalizedRoot := filepath.Clean(childRootPath)
			if doc.recursionInProgress[normalizedRoot] {
				logging.Warn("Recursive loop detected for ecosystem desiredstate %s, skipping", normalizedRoot)
				continue
			}

			if cached, exists := doc.recursionLoaded[normalizedRoot]; exists {
				ecosystemContent[mountPoint] = doc.deepCopyMap(cached)
				continue
			}

			doc.recursionInProgress[normalizedRoot] = true

			childDoc := NewGitOpsDocument()
			childDoc.SetEnvironmentVariables(doc.envVariables)
			childDoc.setChildLoadContext(effectiveNamespace, doc.recursionInProgress)

			loadErr := childDoc.LoadGitOpsFile(normalizedRoot, true, nil)
			delete(doc.recursionInProgress, normalizedRoot)
			if loadErr != nil {
				return errors.Wrapf(errors.ErrParse, loadErr, "failed to fully load ecosystem desiredstate %s", normalizedRoot)
			}

			childContent := childDoc.GetContent().Data
			if childContent == nil {
				return errors.Newf(errors.ErrParse, "fully loaded ecosystem desiredstate is empty: %s", normalizedRoot)
			}

			doc.recursionLoaded[normalizedRoot] = doc.deepCopyMap(childContent)
			ecosystemContent[mountPoint] = doc.deepCopyMap(childContent)
		}
	}

	// Add ecosystem content to metadata
	if len(ecosystemContent) > 0 {
		err := doc.assembledMeta.AddKey(doc.propertyContentEcosystem, ecosystemContent)
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to add ecosystem content")
		}
	}

	return nil
}

func copyRecursionStack(inProgress map[string]bool) map[string]bool {
	if inProgress == nil {
		return make(map[string]bool)
	}

	out := make(map[string]bool, len(inProgress))
	for k, v := range inProgress {
		out[k] = v
	}

	return out
}

func (doc *GitOpsDocument) setChildLoadContext(namespaceOverride string, inProgress map[string]bool) {
	doc.childLoadMode = true
	if namespaceOverride != "" {
		doc.namespaceOverride = namespaceOverride
	}
	// Child desiredstates load with an isolated recursion stack so they cannot
	// mutate or inherit parent traversal state beyond cycle detection ancestry.
	doc.recursionInProgress = copyRecursionStack(inProgress)
	doc.recursionLoaded = make(map[string]map[string]interface{})
}

func (doc *GitOpsDocument) getEffectiveNamespace() string {
	if doc.assembledMeta != nil && doc.assembledMeta.Data != nil {
		if namespace, ok := doc.assembledMeta.Data["namespace"].(string); ok && namespace != "" {
			return strings.ToLower(namespace)
		}
	}

	if doc.namespaceOverride != "" {
		return strings.ToLower(doc.namespaceOverride)
	}

	return "yago"
}

func (doc *GitOpsDocument) getDesiredStatePathWithFallback(key, fallback string) string {
	if doc.schemaManager == nil || doc.schemaVersion == "" {
		return fallback
	}

	paths, err := doc.schemaManager.DiscoverPropertyPaths(
		doc.getEffectiveNamespace(),
		schema.SchemaVersion(doc.schemaVersion),
		true,
	)
	if err != nil {
		return fallback
	}

	path, err := paths.Get(key)
	if err != nil || path == "" {
		return fallback
	}

	return path
}

func (doc *GitOpsDocument) resolveChildDesiredStatePath(envRefMap map[string]interface{}) (string, error) {
	repoRef := envRefMap
	if gitSpec, hasGit := envRefMap["git"].(map[string]interface{}); hasGit && len(gitSpec) > 0 {
		// yago 2.x ecosystem entries are nested under "git"
		repoRef = gitSpec
	}

	absPath, relPath, err := repo.ParseDesiredStateRef(repoRef, nil)
	if err == nil {
		return filepath.Join(absPath, relPath), nil
	}

	// Fallback to local path handling (supports both flat and nested git refs)
	pathStr := ""
	if pathVal, exists := envRefMap["path"]; exists {
		if path, ok := pathVal.(string); ok {
			pathStr = path
		}
	}
	if pathStr == "" {
		if gitSpec, hasGit := envRefMap["git"].(map[string]interface{}); hasGit {
			if path, ok := gitSpec["path"].(string); ok {
				pathStr = path
			}
		}
	}

	if pathStr == "" {
		return "", errors.Newf(errors.ErrParse,
			"unable to resolve repository reference (%v)",
			err,
		)
	}

	return filepath.Join(doc.workdir, pathStr), nil
}

// CloneRepoAndLoad clones a repository and loads GitOps data from it
func (doc *GitOpsDocument) CloneRepoAndLoad(desiredStateContent map[string]interface{}, metaRepoLocatorOverride, cloneDir string, isAssembleParts bool, refOverride string, desiredStateDoc *GitOpsDocument) error {
	// Create repository instance (this also handles cloning/loading)
	gitRepo, err := repo.NewRepoFromDesiredState(desiredStateContent, metaRepoLocatorOverride, cloneDir, refOverride, nil)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to create repository")
	}

	// Store repo reference and update document properties with repository information
	doc.repo = gitRepo
	if err := doc.UpdateRepoProperties(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to update repo properties")
	}

	// Get the full path to the target file
	pathToRoot := gitRepo.GetFilePath()
	if pathToRoot == "" {
		return errors.New(errors.ErrParam, "invalid repository path configuration")
	}

	// Load the GitOps file (passing desiredstate doc for override extraction)
	return doc.LoadGitOpsFile(pathToRoot, isAssembleParts, desiredStateDoc)
}

// GetWatchList returns the watch list for this document
func (doc *GitOpsDocument) GetWatchList() ([]string, error) {
	// Build watch path based on repository configuration
	watchPath := fmt.Sprintf("%s.watch", doc.propertyRepoToLoad)

	watchListVal, err := doc.assembledMeta.GetValue(watchPath)
	if err != nil {
		return []string{}, nil // No watch list configured
	}

	if watchList, ok := watchListVal.([]interface{}); ok {
		var result []string
		for _, item := range watchList {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result, nil
	}

	return []string{}, nil
}

// SetWatchList updates the watch list for this document
func (doc *GitOpsDocument) SetWatchList(watchList []string) error {
	watchPath := fmt.Sprintf("%s.watch", doc.propertyRepoToLoad)

	sort.Strings(watchList)

	// Convert to interface slice
	interfaceList := make([]interface{}, len(watchList))
	for i, item := range watchList {
		interfaceList[i] = item
	}

	// Update both meta and contentAssembled
	err := doc.assembledMeta.AddKey(watchPath, interfaceList)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to update meta watch list")
	}

	err = doc.contentForConsumption.AddKey(watchPath, interfaceList)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to update contentAssembled watch list")
	}

	return nil
}

// FindWatchList recursively finds and builds watch lists for the document hierarchy
func (doc *GitOpsDocument) FindWatchList(parentDocument map[string]interface{}, parentPath string, isSelfUpdatingMasterPipeline bool, recursionDepth int) ([]string, error) {
	watchList := []string{}

	// Add pipeline's own watch list if it's a self-updating pipeline
	if isSelfUpdatingMasterPipeline {
		ownWatchList, err := doc.getWatchListFromDocument(parentDocument)
		if err == nil {
			watchList = append(watchList, ownWatchList...)
		}
	}

	var repoList []string

	// 1. Process ecosystem (sub-desired states)
	ecosystemRepos, err := doc.getEcosystemRepos(parentDocument)
	if err == nil {
		repoList = append(repoList, ecosystemRepos...)
	}

	// 2. Process master pipeline slaves (if this is not too deep in recursion)
	if recursionDepth == 0 {
		slaveRepos, err := doc.getSlavePipelineRepos(parentDocument)
		if err == nil {
			repoList = append(repoList, slaveRepos...)

			// Add pipelines.yaml to watch list for master pipelines
			if len(slaveRepos) > 0 {
				pipelinesPartPath := doc.getDesiredStatePathWithFallback(
					"pipelinesPartFile",
					"desiredstate.meta.parts.pipelines",
				)
				pipelinesFile, err := doc.assembledMeta.GetValue(pipelinesPartPath)
				if err == nil {
					if pipelinesStr, ok := pipelinesFile.(string); ok {
						watchList = append(watchList, pipelinesStr)
					}
				}
			}
		}
	}

	// Recursively process child repositories
	for _, repoStr := range removeDuplicateStrings(repoList) {
		// Parse repository specification
		repoObj, err := doc.handler.LoadString(repoStr)
		if err != nil {
			logging.Debug("Skipping invalid repository spec during watch list build: %v", err)
			continue
		}

		repoPath, err := doc.getRepoPath(repoObj)
		if err != nil {
			logging.Debug("Skipping repository with invalid path during watch list build: %v", err)
			continue
		}

		// Avoid recursive loops
		if repoPath == parentPath {
			logging.Debug("Skipping repository to avoid recursive loop: %s", repoPath)
			continue
		}

		// Get watch list from child repository
		childWatchList, err := doc.getWatchListFromRepo(repoObj)
		if err == nil {
			watchList = append(watchList, childWatchList...)
		}

		// Recursively process children (but not too deep)
		if recursionDepth < 3 {
			childDocument, err := doc.assembleChildDesiredStateMeta(repoPath)
			if err != nil {
				logging.Debug("Skipping repository that failed to assemble during watch list build: %v", err)
				continue
			}

			childWatchList, err := doc.FindWatchList(childDocument, repoPath, true, recursionDepth+1)
			if err == nil {
				watchList = append(watchList, childWatchList...)
			}
		}
	}

	return removeDuplicateStrings(watchList), nil
}

// LoadWatchList loads and builds the complete watch list for the document
func (doc *GitOpsDocument) LoadWatchList() error {
	if !doc.isDesiredState {
		return nil // Only applicable to desired states
	}

	// Determine if this is a self-updating master pipeline
	isSelfUpdating := doc.getMasterPipelineIsSelfUpdating()

	// Build a merged document for watch list traversal that contains BOTH:
	// - Parts data (e.g. master_pipeline.slaves from pipelines.yaml) from contentForConsumption
	// - Ecosystem data (for recursive processing) from assembledMeta (cleanupAssembledDesiredStateEcosystem
	//   deletes propertyMetaEcosystem from contentForConsumption after LoadParts)
	watchDoc := doc.deepCopyMap(doc.contentForConsumption.Data)
	if doc.propertyMetaEcosystem != "" {
		ecoVal, err := doc.handler.GetValue(doc.assembledMeta.Data, doc.propertyMetaEcosystem)
		if err == nil && ecoVal != nil {
			_ = doc.handler.UpdateValue(watchDoc, doc.propertyMetaEcosystem, ecoVal)
		}
	}

	foundWatchList, err := doc.FindWatchList(watchDoc, doc.metaFile, isSelfUpdating, 0)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to find watch list")
	}

	return doc.SetWatchList(foundWatchList)
}

// Helper functions for watch list management

func (doc *GitOpsDocument) getWatchListFromDocument(document map[string]interface{}) ([]string, error) {
	watchPath := doc.getDesiredStatePathWithFallback("repoWatchList", fmt.Sprintf("%s.watch", doc.propertyRepoToLoad))
	watchVal, err := doc.handler.GetValue(document, watchPath)
	if err != nil {
		return []string{}, err
	}

	if watchList, ok := watchVal.([]interface{}); ok {
		var result []string
		for _, item := range watchList {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result, nil
	}

	return []string{}, nil
}

func (doc *GitOpsDocument) getEcosystemRepos(document map[string]interface{}) ([]string, error) {
	ecosystemVal, err := doc.handler.GetValue(document, doc.propertyMetaEcosystem)
	if err != nil {
		return []string{}, err
	}

	if ecosystem, ok := ecosystemVal.(map[string]interface{}); ok {
		var repoList []string
		for _, ecoRef := range ecosystem {
			yamlStr, err := doc.handler.ToString(map[string]interface{}{"repo": ecoRef})
			if err == nil {
				repoList = append(repoList, yamlStr)
			}
		}
		return repoList, nil
	}

	return []string{}, nil
}

func (doc *GitOpsDocument) getSlavePipelineRepos(document map[string]interface{}) ([]string, error) {
	slavesVal, err := doc.handler.GetValue(document, doc.propertySlavePipelinesRepoList)
	if err != nil {
		return []string{}, err
	}

	if slaves, ok := slavesVal.([]interface{}); ok {
		var repoList []string
		for _, slave := range slaves {
			yamlStr, err := doc.handler.ToString(map[string]interface{}{"slave": slave})
			if err == nil {
				repoList = append(repoList, yamlStr)
			}
		}
		return repoList, nil
	}

	return []string{}, nil
}

func (doc *GitOpsDocument) getRepoPath(repoObj map[string]interface{}) (string, error) {
	if repo, exists := repoObj["repo"]; exists {
		if repoMap, ok := repo.(map[string]interface{}); ok {
			// Flat path (slave-style)
			if path, exists := repoMap["path"]; exists {
				if pathStr, ok := path.(string); ok {
					return pathStr, nil
				}
			}
			// Nested git spec (yago 2.0.0 ecosystem structure: repo.git.path)
			if git, exists := repoMap["git"]; exists {
				if gitMap, ok := git.(map[string]interface{}); ok {
					if path, exists := gitMap["path"]; exists {
						if pathStr, ok := path.(string); ok {
							return pathStr, nil
						}
					}
				}
			}
		}
	}

	if slave, exists := repoObj["slave"]; exists {
		if slaveMap, ok := slave.(map[string]interface{}); ok {
			if path, exists := slaveMap["path"]; exists {
				if pathStr, ok := path.(string); ok {
					return pathStr, nil
				}
			}
		}
	}

	return "", errors.New(errors.ErrFail, "path not found in repository object")
}

func (doc *GitOpsDocument) getWatchListFromRepo(repoObj map[string]interface{}) ([]string, error) {
	// Try to get watch list from repo object
	if repo, exists := repoObj["repo"]; exists {
		if repoMap, ok := repo.(map[string]interface{}); ok {
			// Flat watch (slave-style)
			if watch, exists := repoMap["watch"]; exists {
				if watchList, ok := watch.([]interface{}); ok {
					var result []string
					for _, item := range watchList {
						if str, ok := item.(string); ok {
							result = append(result, str)
						}
					}
					return result, nil
				}
			}
			// Nested git spec (yago 2.0.0 ecosystem structure: repo.git.watch)
			if git, exists := repoMap["git"]; exists {
				if gitMap, ok := git.(map[string]interface{}); ok {
					if watch, exists := gitMap["watch"]; exists {
						if watchList, ok := watch.([]interface{}); ok {
							var result []string
							for _, item := range watchList {
								if str, ok := item.(string); ok {
									result = append(result, str)
								}
							}
							return result, nil
						}
					}
				}
			}
		}
	}

	// Try slave object
	if slave, exists := repoObj["slave"]; exists {
		if slaveMap, ok := slave.(map[string]interface{}); ok {
			if watch, exists := slaveMap["watch"]; exists {
				if watchList, ok := watch.([]interface{}); ok {
					var result []string
					for _, item := range watchList {
						if str, ok := item.(string); ok {
							result = append(result, str)
						}
					}
					return result, nil
				}
			}
		}
	}

	return []string{}, nil
}

func (doc *GitOpsDocument) getMasterPipelineIsSelfUpdating() bool {
	if doc.propertyMasterPipelineIsSelfUpdating != "" {
		// Use contentForConsumption: it is fully assembled with parts, so properties
		// like master_pipeline.master.is_self_updating that live in a part file are visible.
		// assembledMeta is root-file-only and would miss them.
		// Read the property to exercise the plumbing, but the effective value is always true.
		_, _ = doc.contentForConsumption.GetValue(doc.propertyMasterPipelineIsSelfUpdating)
	}
	// Effective value is always true regardless of what is configured.
	return true
}

// removeDuplicateStrings removes duplicate strings from a slice
func removeDuplicateStrings(slice []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}
func (doc *GitOpsDocument) ToString() (string, error) {
	return doc.contentForConsumption.ToString()
}

// ToMetaString converts the document metadata to YAML string
// Deprecated: Use ToRootFileString() for clarity
func (doc *GitOpsDocument) ToMetaString() (string, error) {
	return doc.assembledMeta.ToString()
}

// ToRootFileString converts the root file (initial YAML) to string
func (doc *GitOpsDocument) ToRootFileString() (string, error) {
	return doc.assembledMeta.ToString()
}

// SavePart saves content to a part file
func (doc *GitOpsDocument) SavePart(partFile string, partContent map[string]interface{}, isDryRun bool) error {
	fullPath := filepath.Join(doc.workdir, partFile)

	yamlContent, err := doc.handler.ToString(partContent)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to convert content to YAML")
	}

	if isDryRun {
		fmt.Printf("[Dry-Run] Saving part content to file %s\n", partFile)
		fmt.Println(yamlContent)
		return nil
	}

	fmt.Printf("Saving part content to file %s\n", partFile)

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to create directory %s", dir)
	}

	err = os.WriteFile(fullPath, []byte(yamlContent), 0644)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to write file %s", fullPath)
	}

	return nil
}

// ExtractSchemaVersionFromFile gets the schema version from a file using regex pattern matching
func (doc *GitOpsDocument) ExtractSchemaVersionFromFile(schemaVersionOverride, filePath string) (string, error) {
	if schemaVersionOverride != "" {
		return schemaVersionOverride, nil
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to read file %s", filePath)
	}

	// Common version patterns to try (in priority order)
	versionPatterns := []string{
		`schema:\s*(\S+)`, // Current format
		`schema:\s*(\S+)`, // Legacy format
		`version:\s*(\S+)`,
		`schema_version:\s*(\S+)`,
	}

	for _, pattern := range versionPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindSubmatch(content)
		if len(matches) > 1 {
			version := string(matches[1])
			return version, nil
		}
	}

	return "", errors.Newf(errors.ErrParse, "unable to parse schema version from file %s", filePath)
}

// CompactSlavePipelines removes duplicates from slave pipeline list
func (doc *GitOpsDocument) CompactSlavePipelines() error {
	if doc.propertySlavePipelinesRepoList == "" {
		return nil // No slave pipelines configured
	}

	slaveDsList, err := doc.assembledMeta.GetValue(doc.propertySlavePipelinesRepoList)
	if err != nil {
		// No slave pipelines configured, which is fine
		return nil
	}

	slavesList, ok := slaveDsList.([]interface{})
	if !ok {
		return errors.New(errors.ErrParse, "slave pipelines list is not a slice")
	}

	// Convert to strings for deduplication
	var repoList []string
	for _, slave := range slavesList {
		yamlStr, err := doc.handler.ToString(map[string]interface{}{"slave": slave})
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to marshal slave pipeline")
		}
		repoList = append(repoList, yamlStr)
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var compactList []interface{}
	for _, repoStr := range repoList {
		if !seen[repoStr] {
			seen[repoStr] = true
			// Convert back to object
			repoObj, err := doc.handler.LoadString(repoStr)
			if err != nil {
				return errors.Wrapf(errors.ErrParse, err, "failed to unmarshal slave pipeline")
			}
			if slave, exists := repoObj["slave"]; exists {
				compactList = append(compactList, slave)
			}
		}
	}

	// Update both meta and content with compacted list
	err = doc.assembledMeta.AddKey(doc.propertySlavePipelinesRepoList, compactList)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to update meta slave pipelines")
	}

	// Sync master pipeline object from meta to contentAssembled
	masterPipelinePath := doc.getDesiredStatePathWithFallback(
		"masterPipelineRoot",
		"desiredstate.meta.master_pipeline",
	)
	masterPipeline, err := doc.assembledMeta.GetValue(masterPipelinePath)
	if err == nil {
		doc.contentForConsumption.AddKey(masterPipelinePath, masterPipeline)
	}

	return nil
}

// Cache saves the assembled content to a file for wrapper tools to consume.
// This function writes contentForConsumption.Data (NOT assembledMeta.Data) to disk.
//
// What gets written depends on document type:
//
// For DesiredState:
//   - Full assembled document (namespace + schema + desiredstate.meta + desiredstate.content)
//   - This is what wrapper tools like concourse use to generate pipelines
//
// For Configuration with wrapper:
//   - ONLY the wrapper-specific parts (e.g., raw terraform variables)
//   - NO configuration structure, NO meta, NO namespace/schema
//   - Terraform can directly consume this as .tfvars
//
// For Configuration without wrapper:
//   - Full configuration structure for validation/introspection
//
// Parameters:
//   - buildDirectory: Directory to write the file (or "" for temp file)
//   - generateJSON: If true, write as JSON instead of YAML
//
// Returns: Path to the created cache file
func (doc *GitOpsDocument) Cache(buildDirectory string, generateJSON bool) (filePath string, err error) {
	var tempFile *os.File
	var createErr error

	cacheData := doc.contentForConsumption.Data
	if cacheData != nil {
		cacheData = doc.deepCopyMap(cacheData)
	}

	if buildDirectory != "" {
		// Create file in specified directory
		if _, statErr := os.Stat(buildDirectory); os.IsNotExist(statErr) {
			return "", errors.Newf(errors.ErrFail, "build directory does not exist: %s", buildDirectory)
		}

		fileName := doc.tmpFileSuffix
		if generateJSON {
			fileName = strings.TrimSuffix(fileName, ".yaml") + ".json"
		}

		filePath = filepath.Join(buildDirectory, fileName)
		tempFile, createErr = os.Create(filePath)
		if createErr != nil {
			return "", errors.Wrapf(errors.ErrFail, createErr, "failed to create cache file")
		}
	} else {
		// Create temporary file
		suffix := doc.tmpFileSuffix
		if generateJSON {
			suffix = ".gitops-assembled.json"
		}

		tempFile, createErr = os.CreateTemp("", "*"+suffix)
		if createErr != nil {
			return "", errors.Wrapf(errors.ErrFail, createErr, "failed to create temp file")
		}
	}
	defer func() {
		if closeErr := tempFile.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if generateJSON {
		// Normalize content before JSON encoding: YAML parsing may produce
		// map[interface{}]interface{} for nested maps, which json.Encoder cannot handle.
		normalized := doc.normalizeForJSON(cacheData)
		encoder := json.NewEncoder(tempFile)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(normalized)
		if err != nil {
			os.Remove(tempFile.Name())
			return "", errors.Wrapf(errors.ErrFail, err, "failed to write JSON content")
		}
	} else {
		// Write as YAML
		err = doc.handler.ToFile(cacheData, tempFile)
		if err != nil {
			os.Remove(tempFile.Name())
			return "", errors.Wrapf(errors.ErrFail, err, "failed to write YAML content")
		}
	}

	return tempFile.Name(), nil
}

func (doc *GitOpsDocument) cleanupAssembledDesiredStateEcosystem() {
	if !doc.isDesiredState || doc.contentForConsumption == nil || doc.contentForConsumption.Data == nil {
		return
	}

	if doc.propertyMetaEcosystem == "" || doc.propertyContentEcosystem == "" {
		return
	}

	if doc.propertyMetaEcosystem == doc.propertyContentEcosystem {
		return
	}

	if normalized, ok := doc.normalizeForJSON(doc.contentForConsumption.Data).(map[string]interface{}); ok {
		doc.contentForConsumption.Data = normalized
		deleteDottedPathFromMap(normalized, doc.propertyMetaEcosystem)
	}
}

func deleteDottedPathFromMap(root map[string]interface{}, path string) {
	if root == nil || path == "" {
		return
	}

	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return
	}

	current := root
	for _, part := range parts[:len(parts)-1] {
		nextRaw, exists := current[part]
		if !exists {
			return
		}
		nextMap, ok := nextRaw.(map[string]interface{})
		if !ok {
			return
		}
		current = nextMap
	}

	delete(current, parts[len(parts)-1])
}

// normalizeForJSON recursively converts map[interface{}]interface{} to map[string]interface{}
// so that the result is safe to pass to json.Marshal/json.Encoder.
func (doc *GitOpsDocument) normalizeForJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, vv := range val {
			result[fmt.Sprint(k)] = doc.normalizeForJSON(vv)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, vv := range val {
			result[k] = doc.normalizeForJSON(vv)
		}
		return result
	case []interface{}:
		for i, item := range val {
			val[i] = doc.normalizeForJSON(item)
		}
		return val
	default:
		return v
	}
}

func (doc *GitOpsDocument) deepCopyMap(original map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})

	for key, value := range original {
		switch v := value.(type) {
		case map[string]interface{}:
			copy[key] = doc.deepCopyMap(v)
		case []interface{}:
			copy[key] = doc.deepCopySlice(v)
		default:
			copy[key] = value
		}
	}

	return copy
}

// deepCopySlice creates a deep copy of a slice
func (doc *GitOpsDocument) deepCopySlice(original []interface{}) []interface{} {
	copy := make([]interface{}, len(original))

	for i, value := range original {
		switch v := value.(type) {
		case map[string]interface{}:
			copy[i] = doc.deepCopyMap(v)
		case []interface{}:
			copy[i] = doc.deepCopySlice(v)
		default:
			copy[i] = value
		}
	}

	return copy
}
