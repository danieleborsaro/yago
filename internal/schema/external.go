package schema

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// SchemaManifest represents the manifest file for a schema provider
type SchemaManifest struct {
	Manifest SchemaManifestInfo `json:"manifest"`
	Schemas  []SchemaEntry      `json:"schemas"`
}

// SchemaManifestInfo contains metadata about the schema provider
type SchemaManifestInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Namespace   string `json:"namespace"`
}

// SchemaEntry represents a single schema definition
type SchemaEntry struct {
	Version     string `json:"version"`
	Kind        string `json:"kind"`
	File        string `json:"file"`
	Description string `json:"description"`
}

// ExternalSchemaProvider loads schemas from external JSON files (plugins)
type ExternalSchemaProvider struct {
	store             *SchemaStore
	basePath          string
	manifestValidator *ManifestValidator
}

// NewExternalSchemaProvider creates a new external schema provider
func NewExternalSchemaProvider(basePath string, store *SchemaStore) *ExternalSchemaProvider {
	return &ExternalSchemaProvider{
		store:    store,
		basePath: basePath,
	}
}

// SetManifestValidator sets the manifest validator for this provider
func (esp *ExternalSchemaProvider) SetManifestValidator(validator *ManifestValidator) {
	esp.manifestValidator = validator
}

// LoadSchemasFromPaths loads schemas from multiple directory paths
// If optional is true, missing directories only generate warnings; otherwise they cause errors
func (esp *ExternalSchemaProvider) LoadSchemasFromPaths(paths []string, optional bool) error {
	var loadedCount int

	for _, path := range paths {
		if err := esp.loadSchemasFromDirectory(path); err != nil {
			if optional {
				// Optional paths: just log warning and continue
				logging.Warn("Failed to load schemas from %s: %v", path, err)
				continue
			}
			// Required paths: return error
			return errors.Wrapf(errors.ErrParse, err, "failed to load schemas from %s", path)
		}
		loadedCount++
		logging.Info("Successfully loaded schemas from: %s", path)
	}

	if loadedCount > 0 {
		logging.Info("Loaded schemas from %d path(s). Total schemas: %d", loadedCount, esp.store.GetSchemaCount())
	} else if !optional {
		return errors.Newf(errors.ErrParse, "no schemas loaded from any of the %d path(s)", len(paths))
	}

	return nil
}

// loadSchemasFromDirectory loads all schemas from a specific directory
// NOTE: Multiple manifest files are supported per directory. This allows different
// schema namespaces to coexist in the same directory, each with their own manifest file.
// Each manifest can only specify a single namespace value.
func (esp *ExternalSchemaProvider) loadSchemasFromDirectory(dirPath string) error {
	// Check if directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return errors.Newf(errors.ErrParse, "schema directory not found: %s", dirPath)
	}

	// Look for manifest files (supports multiple manifests per directory)
	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Process manifest files
		if strings.HasSuffix(d.Name(), "manifest.json") || strings.HasSuffix(d.Name(), "-manifest.json") {
			return esp.loadManifest(path, dirPath)
		}

		return nil
	})

	return err
}

// validateManifestVersion validates the manifest format version
// Currently only version 1.0.0 is supported
func validateManifestVersion(manifest *SchemaManifest) error {
	const supportedManifestVersion = "1.0.0"

	if manifest.Manifest.Version == "" {
		return errors.New(errors.ErrParse, "manifest version is required but not specified")
	}

	if manifest.Manifest.Version != supportedManifestVersion {
		return errors.Newf(errors.ErrParse, "unsupported manifest version '%s': only version %s is currently supported. "+
			"Please update your manifest file to use version %s or upgrade YAGO to support newer manifest formats",
			manifest.Manifest.Version, supportedManifestVersion, supportedManifestVersion)
	}

	logging.Debug("Manifest version %s validated successfully", manifest.Manifest.Version)
	return nil
}

// validateManifestContent performs structural validation of manifest v1.0.0
// This is a placeholder for future more sophisticated validation
func validateManifestContent(manifest *SchemaManifest) error {
	// Validate required manifest fields
	if manifest.Manifest.Name == "" {
		return errors.New(errors.ErrParse, "manifest.name is required in manifest v1.0.0")
	}

	if manifest.Manifest.Namespace == "" {
		return errors.New(errors.ErrParse, "manifest.namespace is required in manifest v1.0.0")
	}

	// Validate schema entries
	if len(manifest.Schemas) == 0 {
		logging.Warn("Manifest '%s' contains no schema definitions", manifest.Manifest.Name)
	}

	for i, schema := range manifest.Schemas {
		if schema.Version == "" {
			return errors.Newf(errors.ErrParse, "schema entry %d is missing required field 'version'", i)
		}
		if schema.Kind == "" {
			return errors.Newf(errors.ErrParse, "schema entry %d (version %s) is missing required field 'kind'", i, schema.Version)
		}
		if schema.File == "" {
			return errors.Newf(errors.ErrParse, "schema entry %d (version %s) is missing required field 'file'", i, schema.Version)
		}

		// Validate kind values
		validKinds := map[string]bool{
			"DesiredState":  true,
			"Configuration": true,
		}
		if !validKinds[schema.Kind] {
			return errors.Newf(errors.ErrParse, "schema entry %d (version %s) has invalid kind '%s': must be 'DesiredState' or 'Configuration'",
				i, schema.Version, schema.Kind)
		}
	}

	logging.Debug("Manifest content validation passed for '%s'", manifest.Manifest.Name)
	return nil
}

// loadManifest loads a schema manifest and its associated schema files
func (esp *ExternalSchemaProvider) loadManifest(manifestPath, baseDir string) error {
	logging.Info("Loading schema manifest from: %s", manifestPath)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		logging.Error("Failed to read manifest file %s: %v", manifestPath, err)
		return errors.Wrapf(errors.ErrParse, err, "failed to read manifest %s", manifestPath)
	}

	// Use JSON schema validation if validator is available
	if esp.manifestValidator != nil {
		if err := esp.manifestValidator.ValidateManifest(manifestData); err != nil {
			logging.Error("Manifest JSON schema validation failed for %s: %v", manifestPath, err)
			return errors.Wrapf(errors.ErrParse, err, "manifest validation failed for %s", manifestPath)
		}
		logging.Debug("Manifest %s passed JSON schema validation", manifestPath)
	}

	var manifest SchemaManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		logging.Error("Failed to parse manifest file %s: %v", manifestPath, err)
		return errors.Wrapf(errors.ErrParse, err, "failed to parse manifest %s", manifestPath)
	}

	logging.Info("Parsed manifest '%s' from %s with %d schema definitions", manifest.Manifest.Name, manifestPath, len(manifest.Schemas))

	// Validate manifest format version (fallback validation)
	if err := validateManifestVersion(&manifest); err != nil {
		logging.Error("Manifest validation failed for %s: %v", manifestPath, err)
		return errors.Wrapf(errors.ErrParse, err, "invalid manifest %s", manifestPath)
	}

	// Validate manifest content (v1.0.0 structure) - fallback validation
	if err := validateManifestContent(&manifest); err != nil {
		logging.Error("Manifest content validation failed for %s: %v", manifestPath, err)
		return errors.Wrapf(errors.ErrParse, err, "invalid manifest content in %s", manifestPath)
	}

	// Load each schema file referenced in the manifest
	namespace := manifest.Manifest.Namespace
	if namespace == "" {
		namespace = "yago" // Default namespace if not specified
	}

	for _, schemaEntry := range manifest.Schemas {
		schemaFilePath := filepath.Join(baseDir, schemaEntry.File)
		logging.Debug("Loading schema file: %s (namespace: %s, version: %s, kind: %s)", schemaFilePath, namespace, schemaEntry.Version, schemaEntry.Kind)
		if err := esp.loadSchemaFile(schemaFilePath, namespace, schemaEntry.Version, schemaEntry.Kind); err != nil {
			logging.Error("Failed to load schema file %s: %v", schemaFilePath, err)
			return errors.Wrapf(errors.ErrParse, err, "failed to load schema file %s", schemaFilePath)
		}
		logging.Info("Successfully loaded schema: namespace=%s, version=%s, kind=%s from %s", namespace, schemaEntry.Version, schemaEntry.Kind, schemaFilePath)
	}

	// Register the manifest in the common store
	esp.store.RegisterManifest(manifest)
	return nil
}

// loadSchemaFile loads a single JSON schema file
func (esp *ExternalSchemaProvider) loadSchemaFile(filePath, namespace, version, kind string) error {
	logging.Debug("Reading schema file: %s", filePath)
	schemaData, err := os.ReadFile(filePath)
	if err != nil {
		logging.Error("Failed to read schema file %s: %v", filePath, err)
		return errors.Wrapf(errors.ErrParse, err, "failed to read schema file %s", filePath)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		logging.Error("Failed to parse JSON schema file %s: %v", filePath, err)
		return errors.Wrapf(errors.ErrParse, err, "failed to parse schema file %s", filePath)
	}

	logging.Debug("Parsed JSON schema from %s (namespace: %s, version: %s, kind: %s)", filePath, namespace, version, kind)

	// Register the schema in the common store
	esp.store.RegisterSchema(namespace, version, kind, schema)
	return nil
}
