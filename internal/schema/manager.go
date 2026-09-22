package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/danieleborsaro/yago/assets"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// PropertyPaths holds all the property paths needed for document processing
// It uses a dynamic map-based approach to allow schema plugins to define custom paths
type PropertyPaths struct {
	paths map[string]string
}

// NewPropertyPaths creates a new PropertyPaths instance
func NewPropertyPaths() *PropertyPaths {
	return &PropertyPaths{
		paths: make(map[string]string),
	}
}

// Get retrieves any property path by key
func (p *PropertyPaths) Get(key string) (string, error) {
	if val, ok := p.paths[key]; ok {
		return val, nil
	}
	return "", errors.Newf(errors.ErrParam, "property path '%s' not defined in schema", key)
}

// Has checks if a property path exists
func (p *PropertyPaths) Has(key string) bool {
	_, ok := p.paths[key]
	return ok
}

// GetAll returns all property paths (copy to prevent external modification)
func (p *PropertyPaths) GetAll() map[string]string {
	result := make(map[string]string, len(p.paths))
	for k, v := range p.paths {
		result[k] = v
	}
	return result
}

// Set sets a property path (used during discovery)
func (p *PropertyPaths) set(key, value string) {
	p.paths[key] = value
}

// Core path convenience getters (required paths)
// These getters provide type-safe access to required property paths.
// Since DiscoverPropertyPaths validates these exist before returning PropertyPaths,
// these getters return the validated error from Get() which should never occur in normal operation.
// Callers should handle errors appropriately - typically by propagating them up the stack.

func (p *PropertyPaths) Root() (string, error) {
	return p.Get("root")
}

func (p *PropertyPaths) Meta() (string, error) {
	return p.Get("meta")
}

func (p *PropertyPaths) MetaRootPath() (string, error) {
	return p.Get("metaRootPath")
}

func (p *PropertyPaths) MetaEcosystem() (string, error) {
	return p.Get("metaEcosystem")
}

func (p *PropertyPaths) ContentEcosystem() (string, error) {
	return p.Get("contentEcosystem")
}

func (p *PropertyPaths) RepoToLoad() (string, error) {
	return p.Get("repoToLoad")
}

func (p *PropertyPaths) PartsToLoad() (string, error) {
	return p.Get("partsToLoad")
}

func (p *PropertyPaths) MasterPipelineIsSelfUpdating() (string, error) {
	return p.Get("masterPipelineIsSelfUpdating")
}

func (p *PropertyPaths) SlavePipelinesRepoList() (string, error) {
	return p.Get("slavePipelinesRepoList")
}

// Repo field path template methods
// These methods construct paths to repository fields using templates defined in the schema
// The schema defines field names like "url", "ref", "branch", "tag", "path"
// These are combined with the path to the repo object to form the complete path
// Example: if repoPath is "metadata.repo" and url field is "url", returns "metadata.repo.url"

// GetRepoUrlPath returns the path template to a repository's URL field
// Takes the path to the repo object and returns the full path to the URL field
func (p *PropertyPaths) GetRepoUrlPath(repoPath string) string {
	// Get the URL field name from schema (defaults to "url")
	urlField := "url"
	if val, err := p.Get("repoUrlField"); err == nil {
		urlField = val
	}

	if repoPath == "" {
		return urlField
	}
	return repoPath + "." + urlField
}

// GetRepoRefPath returns the path template to a repository's ref field
func (p *PropertyPaths) GetRepoRefPath(repoPath string) string {
	// Get the ref field name from schema (defaults to "ref")
	refField := "ref"
	if val, err := p.Get("repoRefField"); err == nil {
		refField = val
	}

	if repoPath == "" {
		return refField
	}
	return repoPath + "." + refField
}

// GetRepoTagPath returns the path template to a repository's tag field
func (p *PropertyPaths) GetRepoTagPath(repoPath string) string {
	// Get the tag field name from schema (defaults to "tag")
	tagField := "tag"
	if val, err := p.Get("repoTagField"); err == nil {
		tagField = val
	}

	if repoPath == "" {
		return tagField
	}
	return repoPath + "." + tagField
}

// GetRepoBranchPath returns the path template to a repository's branch field
func (p *PropertyPaths) GetRepoBranchPath(repoPath string) string {
	// Get the branch field name from schema (defaults to "branch")
	branchField := "branch"
	if val, err := p.Get("repoBranchField"); err == nil {
		branchField = val
	}

	if repoPath == "" {
		return branchField
	}
	return repoPath + "." + branchField
}

// GetRepoPathPath returns the path template to a repository's path field (subdirectory within repo)
func (p *PropertyPaths) GetRepoPathPath(repoPath string) string {
	// Get the path field name from schema (defaults to "path")
	pathField := "path"
	if val, err := p.Get("repoPathField"); err == nil {
		pathField = val
	}

	if repoPath == "" {
		return pathField
	}
	return repoPath + "." + pathField
}

// GetRepoWatchPath returns the path template to a repository's watch list field
func (p *PropertyPaths) GetRepoWatchPath(repoPath string) string {
	// Get the watch field name from schema (defaults to "watch")
	watchField := "watch"
	if val, err := p.Get("repoWatchField"); err == nil {
		watchField = val
	}

	if repoPath == "" {
		return watchField
	}
	return repoPath + "." + watchField
}

// SchemaManager manages external JSON schemas with full version support
type SchemaManager struct {
	// Unified schema store (shared by all providers)
	store *SchemaStore

	// Embedded core schema provider (loaded at build time)
	embeddedProvider *EmbeddedSchemaProvider

	// External schema provider (for runtime plugin loading)
	externalProvider *ExternalSchemaProvider

	// Configuration
	config *SchemaConfig

	// Working directory for schema resolution
	workDir string

	// PropertyPaths cache: key format: "namespace:version:isDesiredState"
	propertyPathsCache map[string]*PropertyPaths
	propertyPathsMutex sync.RWMutex

	// Wrappers cache: key format: "namespace:version"
	wrappersCache map[string][]string
	wrappersMutex sync.RWMutex
}

// schemaManagerFactory implements a singleton pattern for schema managers
type schemaManagerFactory struct {
	cache             map[string]*SchemaManager
	mutex             sync.RWMutex
	configPath        string // Optional override for schema config path
	namespaceOverride string // Optional override for schema namespace
}

// Global factory instance
var globalFactory = &schemaManagerFactory{
	cache: make(map[string]*SchemaManager),
}

// SetSchemaConfigPath sets the global schema config path override
// This should be called early in the application lifecycle (e.g., from CLI initialization)
func SetSchemaConfigPath(path string) {
	globalFactory.mutex.Lock()
	defer globalFactory.mutex.Unlock()
	globalFactory.configPath = path
	// Clear cache when config path changes to ensure new config is loaded
	if len(globalFactory.cache) > 0 {
		logging.Debug("Clearing schema manager cache due to config path change")
		globalFactory.cache = make(map[string]*SchemaManager)
	}
}

// GetSchemaConfigPath returns the current global schema config path override
func GetSchemaConfigPath() string {
	globalFactory.mutex.RLock()
	defer globalFactory.mutex.RUnlock()
	return globalFactory.configPath
}

// SetNamespaceOverride sets a global namespace override for schema validation.
// When set, this value takes priority over the namespace field in the document.
// This should be called early in the application lifecycle (e.g., from CLI initialization).
func SetNamespaceOverride(ns string) {
	globalFactory.mutex.Lock()
	defer globalFactory.mutex.Unlock()
	globalFactory.namespaceOverride = ns
}

// GetNamespaceOverride returns the current global namespace override
func GetNamespaceOverride() string {
	globalFactory.mutex.RLock()
	defer globalFactory.mutex.RUnlock()
	return globalFactory.namespaceOverride
}

// GetOrCreateSchemaManager returns a cached schema manager or creates a new one
func GetOrCreateSchemaManager(workDir string) (*SchemaManager, error) {
	return globalFactory.getOrCreate(workDir)
}

// GetOrCreateSchemaManagerAuto returns a cached schema manager with auto-detection or creates a new one
func GetOrCreateSchemaManagerAuto() (*SchemaManager, error) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to find project root")
	}

	return globalFactory.getOrCreate(projectRoot)
}

// getOrCreate implements the core factory logic with caching
func (f *schemaManagerFactory) getOrCreate(workDir string) (*SchemaManager, error) {
	// Normalize the path for consistent caching
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		absWorkDir = workDir
	}

	// Check cache first (read lock)
	f.mutex.RLock()
	if manager, exists := f.cache[absWorkDir]; exists {
		f.mutex.RUnlock()
		logging.Debug("Using cached schema manager for work directory: %s", absWorkDir)
		return manager, nil
	}
	f.mutex.RUnlock()

	// Create new manager (write lock)
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Double-check in case another goroutine created it while we were waiting
	if manager, exists := f.cache[absWorkDir]; exists {
		logging.Debug("Using cached schema manager for work directory: %s", absWorkDir)
		return manager, nil
	}

	// Create new manager
	logging.Info("Creating new schema manager for work directory: %s", absWorkDir)
	manager, err := NewSchemaManagerWithConfig(absWorkDir, f.configPath)
	if err != nil {
		return nil, err
	}

	// Cache the manager
	f.cache[absWorkDir] = manager
	logging.Debug("Cached schema manager for work directory: %s", absWorkDir)

	return manager, nil
}

// NewSchemaManager creates a new schema manager
func NewSchemaManager(config *SchemaConfig, workDir string) (*SchemaManager, error) {
	scMan := &SchemaManager{
		config:  config,
		workDir: workDir,
		store:   NewSchemaStore(), // Create shared schema store
	}

	// Initialize manifest validator with embedded manifest schemas
	manifestValidator := NewManifestValidator(assets.ManifestSchemas)

	// Initialize embedded provider with build-time core schemas
	scMan.embeddedProvider = NewEmbeddedSchemaProvider(assets.CoreSchemas, scMan.store)
	scMan.embeddedProvider.SetManifestValidator(manifestValidator)
	if err := scMan.embeddedProvider.LoadEmbeddedSchemas(); err != nil {
		logging.Error("Failed to load embedded core schemas: %v", err)
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to load embedded core schemas")
	}

	// Initialize external provider for runtime plugin loading
	schemaPath := config.ResolveSchemaPath(workDir)
	scMan.externalProvider = NewExternalSchemaProvider(schemaPath, scMan.store)
	scMan.externalProvider.SetManifestValidator(manifestValidator)

	// Load plugin schemas from filesystem
	if err := scMan.loadPluginSchemas(); err != nil {
		// Plugins are optional, so just warn
		logging.Warn("Failed to load plugin schemas: %v", err)
	}

	return scMan, nil
}

// ResolveSchemaConfigPath determines the path to schema-config.json with the following precedence:
// 1. ~/.yago/schema-config.json (if it exists) - user default
// 2. workDir/schema-config.json (if it exists) - project/current directory
// 3. YAGO_SCHEMA_CONFIG environment variable - environment override
// 4. Explicit path from CLI flag (highest priority) - command-line override
//
// This order prioritizes development workflow:
// - User defaults are checked first
// - Then local project config
// - Environment variables can override for specific contexts
// - CLI flag provides ultimate override capability
func ResolveSchemaConfigPath(explicitPath, workDir string) string {
	// Priority 4 (Highest): Explicit path from CLI flag
	if explicitPath != "" {
		// Expand home directory if needed
		if strings.HasPrefix(explicitPath, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				explicitPath = filepath.Join(home, explicitPath[2:])
			}
		}
		return explicitPath
	}

	// Priority 3: Environment variable
	if envPath := os.Getenv("YAGO_SCHEMA_CONFIG"); envPath != "" {
		// Expand home directory if needed
		if strings.HasPrefix(envPath, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				envPath = filepath.Join(home, envPath[2:])
			}
		}
		return envPath
	}

	// Priority 2: Current/project directory config
	workDirConfigPath := filepath.Join(workDir, "schema-config.json")
	if _, err := os.Stat(workDirConfigPath); err == nil {
		return workDirConfigPath
	}

	// Priority 1 (Lowest): User home directory config (fallback default)
	if home, err := os.UserHomeDir(); err == nil {
		homeConfigPath := filepath.Join(home, ".yago", "schema-config.json")
		// Return this path whether it exists or not - it's the final fallback
		// If it doesn't exist, LoadSchemaConfig will use default config
		return homeConfigPath
	}

	// Ultimate fallback if home directory cannot be determined
	return workDirConfigPath
}

// NewSchemaManagerWithDefaults creates a schema manager with default configuration
func NewSchemaManagerWithDefaults(workDir string) (*SchemaManager, error) {
	return NewSchemaManagerWithConfig(workDir, "")
}

// NewSchemaManagerWithConfig creates a schema manager with optional config path override
func NewSchemaManagerWithConfig(workDir, configPathOverride string) (*SchemaManager, error) {
	// Resolve the schema config path with proper precedence
	configPath := ResolveSchemaConfigPath(configPathOverride, workDir)
	config, err := LoadSchemaConfig(configPath)
	if err != nil {
		logging.Warn("Failed to load schema config from %s, using defaults: %v", configPath, err)
		config = DefaultSchemaConfig()
	} else {
		logging.Info("Loaded schema configuration from: %s", configPath)
	}

	scMan := &SchemaManager{
		config:  config,
		workDir: workDir,
		store:   NewSchemaStore(), // Create shared schema store
	}

	// Initialize embedded provider with build-time core schemas
	logging.Info("Loading embedded core schemas from build-time resources")
	scMan.embeddedProvider = NewEmbeddedSchemaProvider(assets.CoreSchemas, scMan.store)
	if err := scMan.embeddedProvider.LoadEmbeddedSchemas(); err != nil {
		logging.Error("Failed to load embedded core schemas: %v", err)
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to load embedded core schemas")
	}
	logging.Info("Successfully loaded embedded core schemas")

	// Initialize external provider for runtime plugin loading (only for default path)
	schemaPath := config.ResolveSchemaPath(workDir)
	logging.Info("Initializing plugin schema loader with default path: %s", schemaPath)
	scMan.externalProvider = NewExternalSchemaProvider(schemaPath, scMan.store)

	// Load plugin schemas from filesystem (default + custom paths)
	if config.EnablePlugins {
		logging.Info("Loading plugin schemas from filesystem")
		if err := scMan.loadPluginSchemas(); err != nil {
			// Plugins are optional, so just warn
			logging.Warn("Failed to load plugin schemas: %v", err)
			fmt.Printf("Warning: failed to load plugin schemas: %v\n", err)
		} else {
			logging.Info("Successfully loaded plugin schemas")
		}
	} else {
		logging.Info("Plugin loading is disabled in configuration")
	}

	logging.Info("Successfully initialized schema manager with embedded core schemas and plugin schemas")
	return scMan, nil
}

// findProjectRoot searches for the project root by looking for go.mod or schemas directory
func findProjectRoot() (string, error) {
	// Start from current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get current directory")
	}

	// Walk up the directory tree looking for indicators of project root
	dir := currentDir
	for {
		// Check for go.mod file (indicates Go project root)
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// Found go.mod - this is the Go project root
			// Check if this is the yago project by looking for cmd/yago
			if _, err := os.Stat(filepath.Join(dir, "cmd", "yago")); err == nil {
				logging.Debug("Found yago project root at: %s", dir)
				return dir, nil
			}
			// Found go.mod but not yago - continue searching up
			logging.Debug("Found go.mod at %s but not yago project, continuing search", dir)
		}

		// Check for schemas directory (legacy indicator)
		if _, err := os.Stat(filepath.Join(dir, "schemas")); err == nil {
			// Verify it's a yago-style schemas directory by checking for gitops subdirectory
			if _, err := os.Stat(filepath.Join(dir, "schemas", "gitops")); err == nil {
				logging.Debug("Found yago schemas directory at: %s", dir)
				return dir, nil
			}
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	// Fallback to current directory
	logging.Debug("Could not find yago project root, using current directory: %s", currentDir)
	return currentDir, nil
}

// NewSchemaManagerAuto creates a schema manager with automatic project root detection
func NewSchemaManagerAuto() (*SchemaManager, error) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to find project root")
	}

	return NewSchemaManagerWithDefaults(projectRoot)
}

// loadPluginSchemas loads plugin schemas from filesystem
// Loads from all schema paths configured in schema-config.json
func (scMan *SchemaManager) loadPluginSchemas() error {
	if scMan.externalProvider == nil {
		return errors.New(errors.ErrFail, "external provider not initialized")
	}

	if !scMan.config.EnablePlugins {
		logging.Info("Plugin loading is disabled in configuration")
		return nil
	}

	// Get all schema paths from configuration (default + custom)
	allPaths := scMan.config.GetAllSchemaPaths(scMan.workDir)

	logging.Info("Loading plugin schemas from %d configured path(s)", len(allPaths))

	// Load schemas from all configured paths
	// Paths are treated as optional - missing directories just generate warnings
	if err := scMan.externalProvider.LoadSchemasFromPaths(allPaths, true); err != nil {
		logging.Warn("Error loading plugin schemas: %v", err)
	}

	return nil
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

// ValidateSchema validates content against specified schema version and type
func (scMan *SchemaManager) ValidateSchema(content map[string]interface{}, version SchemaVersion, schemaType SchemaType) error {
	normalizedVersion := scMan.normalizeVersion(string(version))

	// Extract namespace: CLI override takes priority, then document field, then default "yago"
	namespace := "yago"
	namespaceSpecified := false
	if override := GetNamespaceOverride(); override != "" {
		namespace = override
		namespaceSpecified = true
		logging.Debug("Using namespace override from CLI: %s", namespace)
	} else if namespaceVal, ok := content["namespace"]; ok {
		if namespaceStr, ok := namespaceVal.(string); ok {
			namespace = namespaceStr
			namespaceSpecified = true
		}
	}

	// Inject the resolved namespace into the document
	if !namespaceSpecified {
		content["namespace"] = namespace
		logging.Warn("Document did not specify 'namespace' field - added default value: %s", namespace)
	} else {
		content["namespace"] = namespace
	}

	logging.Debug("Starting schema validation: namespace=%s, version=%s->%s, type=%s", namespace, string(version), normalizedVersion, schemaType)

	// Map schema type to appropriate kind for validation
	kind := "DesiredState" // Default to DesiredState for most cases

	// Handle different schema types
	switch schemaType {
	case SchemaTypeDesiredStateMeta, SchemaTypeDesiredStateAssembled:
		kind = "DesiredState"
		logging.Debug("Using DesiredState schema kind for type: %s", schemaType)
	case SchemaTypeConfigurationMeta, SchemaTypeConfigurationContent:
		kind = "Configuration"
		logging.Debug("Using Configuration schema kind for type: %s", schemaType)
	}

	// Check if schema exists in the unified store
	if !scMan.store.HasSchema(namespace, normalizedVersion, kind) {
		logging.Error("No schema found for namespace %s, version %s and type %s", namespace, normalizedVersion, schemaType)
		return errors.Newf(errors.ErrFail, "no schema found for namespace %s, version %s and type %s", namespace, normalizedVersion, schemaType)
	}

	// For now, just verify that the schema exists
	// Future enhancements could include full JSON Schema validation
	logging.Debug("Document validation successful: namespace=%s, version=%s, kind=%s", namespace, normalizedVersion, kind)
	return nil
}

// ValidateSchemaAutoDetect validates content with auto-detected schema version
func (scMan *SchemaManager) ValidateSchemaAutoDetect(content map[string]interface{}, schemaType SchemaType) error {
	version, err := scMan.DetectSchemaVersion(content)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to detect schema version")
	}

	return scMan.ValidateSchema(content, SchemaVersion(version), schemaType)
}

// extractVersionString extracts a version string from various types with consistent type handling
func (scMan *SchemaManager) extractVersionString(value interface{}, fieldName string) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		logging.Warn("Unsupported %s type: %T", fieldName, value)
		return ""
	}
}

// DetectSchemaVersion detects schema version from document content with comprehensive alias support
func (scMan *SchemaManager) DetectSchemaVersion(content map[string]interface{}) (string, error) {
	logging.Debug("Detecting schema version from document content")

	// Look for schema field first (current format)
	if schemaVersion, exists := content["schema"]; exists {
		if version := scMan.extractVersionString(schemaVersion, "schema"); version != "" {
			normalizedVersion := scMan.normalizeVersion(version)
			logging.Info("Detected schema version from 'schema' field: %s -> %s", version, normalizedVersion)
			return normalizedVersion, nil
		}
	}

	// Look for schemaVersion field (legacy format)
	if schemaVersion, exists := content["schemaVersion"]; exists {
		if version := scMan.extractVersionString(schemaVersion, "schemaVersion"); version != "" {
			normalizedVersion := scMan.normalizeVersion(version)
			logging.Info("Detected schema version from 'schemaVersion' field (legacy format): %s -> %s", version, normalizedVersion)
			return normalizedVersion, nil
		}
	}

	// Fail fast - no default assumptions
	return "", errors.New(errors.ErrParse, "no schema version found in document: missing 'schema' or 'schemaVersion' field. GitOps documents must explicitly specify their schema version")
}

// normalizeVersion resolves partial version specifications to exact versions using semver semantics
func (scMan *SchemaManager) normalizeVersion(version string) string {
	// Remove common prefixes
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "gitops.tools.io/v")
	version = strings.TrimPrefix(version, "gitops.tools.io/")

	// Get available versions from the unified store
	availableVersions := scMan.store.GetAvailableVersions()

	// Handle special aliases first
	if version == "latest" {
		highest := scMan.findHighestSemverVersion(availableVersions)
		if highest != "" {
			logging.Debug("Normalized 'latest' to highest available version: %s", highest)
			return highest
		}
		logging.Error("No schemas loaded, cannot resolve 'latest' version")
		return "latest" // Return as-is to let validation fail later with a clear error
	}

	// Handle x-wildcards (e.g., "4.x", "3.x")
	if strings.HasSuffix(version, ".x") {
		baseVersion := strings.TrimSuffix(version, ".x")
		resolved := scMan.resolvePartialVersion(baseVersion, availableVersions)
		if resolved != "" {
			logging.Debug("Normalized '%s' to latest matching version: %s", version, resolved)
			return resolved
		}
	}

	// Check if exact version exists
	for _, v := range availableVersions {
		if v == version {
			logging.Debug("Exact version '%s' found in loaded schemas", version)
			return version
		}
	}

	// Try to resolve partial version (e.g., "4" -> "4.2.0", "4.2" -> "4.2.5")
	resolved := scMan.resolvePartialVersion(version, availableVersions)
	if resolved != "" {
		logging.Debug("Resolved partial version '%s' to latest matching: %s", version, resolved)
		return resolved
	}

	// If version not found in loaded schemas, return as-is and let validation handle the error
	logging.Warn("Version '%s' not found in loaded schemas, returning as-is", version)
	return version
}

// resolvePartialVersion finds the highest semver version that matches a partial specification
func (scMan *SchemaManager) resolvePartialVersion(partial string, availableVersions []string) string {
	var candidates []string

	// Find all versions that match the partial specification
	for _, version := range availableVersions {
		if scMan.matchesPartialVersion(version, partial) {
			candidates = append(candidates, version)
		}
	}

	// Return the highest matching version
	return scMan.findHighestSemverVersion(candidates)
}

// matchesPartialVersion checks if a full version matches a partial specification
func (scMan *SchemaManager) matchesPartialVersion(fullVersion, partial string) bool {
	// Split versions into parts
	fullParts := strings.Split(fullVersion, ".")
	partialParts := strings.Split(partial, ".")

	// Check if all partial parts match the corresponding full parts
	for i, partialPart := range partialParts {
		if i >= len(fullParts) {
			return false
		}
		if fullParts[i] != partialPart {
			return false
		}
	}

	return true
}

// findHighestSemverVersion finds the highest semantic version from a list
func (scMan *SchemaManager) findHighestSemverVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}

	highest := versions[0]
	for _, version := range versions[1:] {
		if scMan.compareSemanticVersions(version, highest) > 0 {
			highest = version
		}
	}

	return highest
}

// compareSemanticVersions compares two semantic version strings (returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal)
func (scMan *SchemaManager) compareSemanticVersions(v1, v2 string) int {
	if v1 == v2 {
		return 0
	}

	// Parse version parts
	parts1 := scMan.parseVersionParts(v1)
	parts2 := scMan.parseVersionParts(v2)

	// Compare major, minor, patch in order
	for i := 0; i < 3; i++ {
		if parts1[i] > parts2[i] {
			return 1
		}
		if parts1[i] < parts2[i] {
			return -1
		}
	}

	return 0
}

// parseVersionParts parses a version string into [major, minor, patch] integers
func (scMan *SchemaManager) parseVersionParts(version string) [3]int {
	parts := [3]int{0, 0, 0} // default to 0.0.0

	// Handle non-standard versions (like "1.0.0-custom") by taking only the numeric part
	versionParts := strings.Split(strings.Split(version, "-")[0], ".")

	for i, part := range versionParts {
		if i >= 3 {
			break
		}
		// Simple integer parsing - if it fails, part remains 0
		if num := scMan.parseIntSafe(part); num >= 0 {
			parts[i] = num
		}
	}

	return parts
}

// parseIntSafe safely parses an integer, returning -1 on error
func (scMan *SchemaManager) parseIntSafe(s string) int {
	result := 0
	for _, char := range s {
		if char < '0' || char > '9' {
			return -1
		}
		result = result*10 + int(char-'0')
	}
	return result
}

// GetSupportedSchemaVersions returns all supported schema versions
func (scMan *SchemaManager) GetSupportedSchemaVersions() []SchemaVersion {
	versionSet := make(map[SchemaVersion]bool)

	// Add all versions from the unified store
	for _, versionStr := range scMan.store.GetAvailableVersions() {
		versionSet[SchemaVersion(versionStr)] = true
	}

	// Add common aliases
	aliases := []SchemaVersion{"1", "1.0", "1.x", "2", "2.0", "2.1", "2.x", "3", "3.0", "3.11", "3.x", "4", "4.0", "4.1", "4.2", "4.x", "latest"}
	for _, alias := range aliases {
		versionSet[alias] = true
	}

	// Convert to slice
	versions := make([]SchemaVersion, 0, len(versionSet))
	for version := range versionSet {
		versions = append(versions, version)
	}

	return versions
}

// GetSchemaContent returns the JSON schema definition for a specific namespace, version and schema type
// If namespace is empty string, defaults to "yago"
func (scMan *SchemaManager) GetSchemaContent(namespace string, version SchemaVersion, schemaType SchemaType) (map[string]interface{}, error) {
	if namespace == "" {
		namespace = "yago" // Default namespace
	}

	normalizedVersion := scMan.normalizeVersion(string(version))

	// Map schema type to schema kind
	var schemaKind string
	switch schemaType {
	case SchemaTypeDesiredStateMeta, SchemaTypeDesiredStateAssembled:
		schemaKind = "DesiredState"
	case SchemaTypeConfigurationMeta, SchemaTypeConfigurationContent:
		schemaKind = "Configuration"
	default:
		return nil, errors.Newf(errors.ErrParam, "unsupported schema type: %s", schemaType)
	}

	// Get schema from the unified store
	return scMan.store.GetSchema(namespace, normalizedVersion, schemaKind)
}

// GetSchemaManifests returns information about loaded schema providers
func (scMan *SchemaManager) GetSchemaManifests() []SchemaManifest {
	return scMan.store.GetManifests()
}

// ReloadSchemas reloads all external schemas
func (scMan *SchemaManager) ReloadSchemas() error {
	if scMan.externalProvider != nil {
		// Recreate the provider to clear cache
		schemaPath := scMan.config.ResolveSchemaPath(scMan.workDir)
		scMan.externalProvider = NewExternalSchemaProvider(schemaPath, scMan.store)
		return scMan.loadPluginSchemas()
	}
	return errors.New(errors.ErrFail, "external provider not initialized")
}

// GetWorkDir returns the working directory used by this schema manager
func (scMan *SchemaManager) GetWorkDir() string {
	return scMan.workDir
}

// DiscoverPropertyPaths discovers property paths from the schema's x-gitops-paths extension
// This method requires the schema to explicitly define property paths - no fallback is provided
// If namespace is empty, defaults to "yago"
func (scMan *SchemaManager) DiscoverPropertyPaths(namespace string, version SchemaVersion, isDesiredState bool) (*PropertyPaths, error) {
	if namespace == "" {
		namespace = "yago" // Default namespace for backward compatibility
	}

	// Create cache key
	cacheKey := fmt.Sprintf("%s:%s:%t", namespace, version, isDesiredState)

	// Check cache first
	scMan.propertyPathsMutex.RLock()
	if cached, exists := scMan.propertyPathsCache[cacheKey]; exists {
		scMan.propertyPathsMutex.RUnlock()
		logging.Debug("Using cached property paths for namespace=%s, version=%s, isDesiredState=%t", namespace, version, isDesiredState)
		return cached, nil
	}
	scMan.propertyPathsMutex.RUnlock()

	logging.Debug("Discovering property paths for namespace=%s, version=%s, isDesiredState=%t", namespace, version, isDesiredState)

	// Get the schema content
	var schemaType SchemaType
	if isDesiredState {
		schemaType = SchemaTypeDesiredStateMeta
	} else {
		schemaType = SchemaTypeConfigurationMeta
	}

	schemaContent, err := scMan.GetSchemaContent(namespace, version, schemaType)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to get schema content for version %s", version)
	}

	// Look for the x-gitops-paths extension
	gitopsPathsRaw, exists := schemaContent["x-gitops-paths"]
	if !exists {
		return nil, errors.Newf(errors.ErrFail, "schema plugin for namespace %s, version %s does not specify required 'x-gitops-paths' extension. Schema plugins must explicitly define property paths", namespace, version)
	}

	gitopsPaths, ok := gitopsPathsRaw.(map[string]interface{})
	if !ok {
		return nil, errors.Newf(errors.ErrParse, "x-gitops-paths extension in schema %s is not a valid object", version)
	}

	// Determine which document type paths to use
	var pathsSection map[string]interface{}
	if isDesiredState {
		if desiredStateSection, exists := gitopsPaths["desiredstate"]; exists {
			if dsSection, ok := desiredStateSection.(map[string]interface{}); ok {
				pathsSection = dsSection
			} else {
				return nil, errors.Newf(errors.ErrParse, "x-gitops-paths.desiredstate in schema %s is not a valid object", version)
			}
		} else {
			return nil, errors.Newf(errors.ErrFail, "schema plugin for version %s does not define required 'x-gitops-paths.desiredstate' section", version)
		}
	} else {
		if configSection, exists := gitopsPaths["configuration"]; exists {
			if cfgSection, ok := configSection.(map[string]interface{}); ok {
				pathsSection = cfgSection
			} else {
				return nil, errors.Newf(errors.ErrParse, "x-gitops-paths.configuration in schema %s is not a valid object", version)
			}
		} else {
			return nil, errors.Newf(errors.ErrFail, "schema plugin for version %s does not define required 'x-gitops-paths.configuration' section", version)
		}
	}

	// Create new PropertyPaths instance
	paths := NewPropertyPaths()

	// Load ALL paths from schema (including custom plugin-specific paths)
	for key, value := range pathsSection {
		if pathStr, ok := value.(string); ok {
			paths.set(key, pathStr)
			logging.Debug("Loaded property path: %s = %s", key, pathStr)
		} else {
			return nil, errors.Newf(errors.ErrParse, "property path '%s' in schema %s is not a string (got %T)", key, version, value)
		}
	}

	// Validate that all REQUIRED core paths are present
	// Note: Schemas can define additional custom paths beyond these
	requiredPaths := []string{
		"root",
		"meta",
		"metaRootPath",
		"repoToLoad",
		"partsToLoad",
	}
	if isDesiredState {
		requiredPaths = append(requiredPaths,
			"metaEcosystem",
			"contentEcosystem",
			"masterPipelineIsSelfUpdating",
			"slavePipelinesRepoList",
		)
	}

	var missingPaths []string
	for _, pathName := range requiredPaths {
		if !paths.Has(pathName) {
			missingPaths = append(missingPaths, pathName)
		}
	}

	if len(missingPaths) > 0 {
		return nil, errors.Newf(errors.ErrFail, "schema plugin for version %s is missing required property paths in x-gitops-paths: %s", version, strings.Join(missingPaths, ", "))
	}

	logging.Info("Successfully discovered %d property paths from schema %s (%d required, %d custom)",
		len(paths.GetAll()), version, len(requiredPaths), len(paths.GetAll())-len(requiredPaths))

	// Cache the discovered paths
	scMan.propertyPathsMutex.Lock()
	if scMan.propertyPathsCache == nil {
		scMan.propertyPathsCache = make(map[string]*PropertyPaths)
	}
	scMan.propertyPathsCache[cacheKey] = paths
	scMan.propertyPathsMutex.Unlock()

	return paths, nil
}

// DiscoverWrappers discovers and returns the list of supported wrappers for a given schema version.
//
// This method dynamically extracts wrapper definitions from the schema's custom x-gitops-wrappers
// JSON property, providing runtime flexibility for schema evolution without code changes.
//
// The method implements a thread-safe caching mechanism to avoid repeated JSON parsing:
//   - First call: Loads schema, parses x-gitops-wrappers, caches result
//   - Subsequent calls: Returns cached value immediately
//
// Parameters:
//   - namespace: Schema namespace (empty string defaults to "yago" for embedded schemas)
//   - version: Schema version (e.g., "1.0.0", "v3.10.0")
//
// Returns:
//   - []string: List of wrapper names as defined in the schema (typically lowercase)
//   - error: Non-nil if schema cannot be loaded or x-gitops-wrappers is invalid/missing
//
// Schema Requirements:
//   - Must contain x-gitops-wrappers property at root level
//   - Property must be a non-empty JSON array of strings
//   - Each wrapper name should be a valid string identifier
//
// Example Schema Definition:
//
//	{
//	  "$schema": "https://json-schema.org/draft/2020-12/schema",
//	  "x-gitops-wrappers": ["meta", "terraform", "concourse"],
//	  ...
//	}
//
// Example Usage:
//
//	wrappers, err := schemaManager.DiscoverWrappers("yago", "1.0.0")
//	if err != nil {
//	    return fmt.Errorf("wrapper discovery failed: %w", err)
//	}
//	// wrappers = ["meta", "terraform", "concourse", ...]
//
// Design rationale:
// Wrapper support is schema-driven rather than hardcoded, so new wrappers can be
// added by updating the schema definition without any Go code changes.
//
// Thread Safety:
// This method is safe for concurrent use. Read operations use RWMutex.RLock(),
// write operations use RWMutex.Lock().
func (scMan *SchemaManager) DiscoverWrappers(namespace string, version SchemaVersion) ([]string, error) {
	if namespace == "" {
		namespace = "yago" // Default namespace for embedded schemas
	}

	// Create cache key combining namespace and version
	cacheKey := fmt.Sprintf("%s:%s", namespace, version)

	// Check cache first (read lock only)
	scMan.wrappersMutex.RLock()
	if cached, exists := scMan.wrappersCache[cacheKey]; exists {
		scMan.wrappersMutex.RUnlock()
		logging.Debug("Using cached wrappers for namespace=%s, version=%s", namespace, version)
		return cached, nil
	}
	scMan.wrappersMutex.RUnlock()

	logging.Debug("Discovering wrappers for namespace=%s, version=%s", namespace, version)

	// Get the schema content (desiredstate schema contains wrapper definitions)
	schemaContent, err := scMan.GetSchemaContent(namespace, version, SchemaTypeDesiredStateMeta)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to get schema content for version %s", version)
	}

	// Extract the x-gitops-wrappers custom property
	wrappersRaw, exists := schemaContent["x-gitops-wrappers"]
	if !exists {
		return nil, errors.Newf(errors.ErrFail, "schema for namespace %s, version %s does not specify 'x-gitops-wrappers' extension. Schema must define supported wrappers", namespace, version)
	}

	// Validate that x-gitops-wrappers is a JSON array
	wrappersList, ok := wrappersRaw.([]interface{})
	if !ok {
		return nil, errors.Newf(errors.ErrParse, "x-gitops-wrappers extension in schema %s is not a valid array", version)
	}

	// Convert interface{} array to string slice, validating each entry
	wrappers := make([]string, 0, len(wrappersList))
	for i, wrapper := range wrappersList {
		wrapperStr, ok := wrapper.(string)
		if !ok {
			return nil, errors.Newf(errors.ErrParse, "wrapper at index %d in schema %s is not a string (got %T)", i, version, wrapper)
		}
		wrappers = append(wrappers, wrapperStr)
	}

	// Ensure at least one wrapper is defined
	if len(wrappers) == 0 {
		return nil, errors.Newf(errors.ErrFail, "schema %s defines empty wrappers list", version)
	}

	logging.Info("Discovered %d wrappers from schema %s: %v", len(wrappers), version, wrappers)

	// Cache the discovered wrappers (write lock)
	scMan.wrappersMutex.Lock()
	if scMan.wrappersCache == nil {
		scMan.wrappersCache = make(map[string][]string)
	}
	scMan.wrappersCache[cacheKey] = wrappers
	scMan.wrappersMutex.Unlock()

	return wrappers, nil
}

// GetSupportedWrappers returns the list of wrappers supported by a specific schema version.
//
// This is a convenience method that wraps DiscoverWrappers() with the default "yago" namespace,
// providing a simpler API for the common case of querying embedded schema wrappers.
//
// The method returns wrapper names in their canonical form (typically lowercase) as defined
// in the schema's x-gitops-wrappers property.
//
// Parameters:
//   - version: Schema version to query (e.g., "1.0.0", "v3.10.0")
//
// Returns:
//   - []string: List of supported wrapper names, or empty slice if discovery fails
//
// Error Handling:
// Unlike DiscoverWrappers, this method does not return errors. Instead, it logs errors
// and returns an empty slice, making it safe to use in validation contexts where a
// missing wrapper list effectively means "no wrappers supported".
//
// Example Usage:
//
//	wrappers := schemaManager.GetSupportedWrappers("1.0.0")
//	// wrappers = ["meta", "terraform", "concourse", "packer", ...]
//
//	for _, wrapper := range wrappers {
//	    fmt.Printf("Supported: %s\n", wrapper)
//	}
//
// Caching:
// Results are cached by DiscoverWrappers(), so repeated calls are efficient.
func (scMan *SchemaManager) GetSupportedWrappers(version SchemaVersion) []string {
	// Discover wrappers dynamically from schema (uses default "yago" namespace)
	wrappers, err := scMan.DiscoverWrappers("", version)
	if err != nil {
		logging.Error("Failed to discover wrappers for version %s: %v", version, err)
		return []string{} // Return empty slice on error (indicates no wrappers supported)
	}

	logging.Debug("Schema version %s supports %d wrappers: %v", version, len(wrappers), wrappers)

	return wrappers
}

// IsWrapperSupported checks whether a specific wrapper is supported by a schema version.
//
// This method provides case-insensitive wrapper validation, allowing user input in any case
// (e.g., "terraform", "Terraform", "TERRAFORM") to match schema definitions.
//
// The validation is performed by querying GetSupportedWrappers() and comparing the provided
// wrapper name against the schema's wrapper list.
//
// Parameters:
//   - version: Schema version to check against (e.g., "1.0.0", "v3.10.0")
//   - wrapper: Wrapper name to validate (case-insensitive)
//
// Returns:
//   - bool: true if the wrapper is supported by the schema version, false otherwise
//
// Case Sensitivity:
// The comparison is case-insensitive to improve user experience:
//   - "terraform" matches "terraform"
//   - "Terraform" matches "terraform"
//   - "TERRAFORM" matches "terraform"
//
// Example Usage:
//
//	if schemaManager.IsWrapperSupported("1.0.0", "terraform") {
//	    fmt.Println("Terraform wrapper is supported")
//	}
//
//	if !schemaManager.IsWrapperSupported("1.0.0", "unsupported") {
//	    fmt.Println("Wrapper not available in v1.0.0")
//	}
//
// Performance:
// First call for a version triggers DiscoverWrappers() which caches results.
// Subsequent calls reuse cached data, making repeated validation efficient.
func (scMan *SchemaManager) IsWrapperSupported(version SchemaVersion, wrapper string) bool {
	wrappers := scMan.GetSupportedWrappers(version)

	// Normalize wrapper to lowercase for case-insensitive comparison
	wrapperLower := strings.ToLower(wrapper)

	for _, supportedWrapper := range wrappers {
		if strings.ToLower(supportedWrapper) == wrapperLower {
			logging.Debug("Wrapper '%s' is supported by schema %s", wrapper, version)
			return true
		}
	}

	logging.Debug("Wrapper '%s' is NOT supported by schema %s (supported: %v)", wrapper, version, wrappers)

	return false
}
