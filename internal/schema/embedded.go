package schema

import (
	"embed"
	"encoding/json"
	"path/filepath"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// EmbeddedSchemaProvider loads core schemas from embedded files
// The actual embedded FS is passed from the caller
type EmbeddedSchemaProvider struct {
	fs                embed.FS
	store             *SchemaStore
	manifestValidator *ManifestValidator
}

// NewEmbeddedSchemaProvider creates a new embedded schema provider
func NewEmbeddedSchemaProvider(fs embed.FS, store *SchemaStore) *EmbeddedSchemaProvider {
	return &EmbeddedSchemaProvider{
		fs:    fs,
		store: store,
	}
}

// SetManifestValidator sets the manifest validator for this provider
func (esp *EmbeddedSchemaProvider) SetManifestValidator(validator *ManifestValidator) {
	esp.manifestValidator = validator
}

// LoadEmbeddedSchemas loads all embedded core schemas from the provided FS
func (esp *EmbeddedSchemaProvider) LoadEmbeddedSchemas() error {
	logging.Info("Loading embedded core schemas from build-time resources")

	// First, load the manifest
	manifestPath := "schemas/manifest.json"
	manifestData, err := esp.fs.ReadFile(manifestPath)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to read embedded manifest")
	}

	// Use JSON schema validation if validator is available
	if esp.manifestValidator != nil {
		if err := esp.manifestValidator.ValidateManifest(manifestData); err != nil {
			logging.Error("Embedded manifest JSON schema validation failed: %v", err)
			return errors.Wrapf(errors.ErrParse, err, "embedded manifest validation failed")
		}
		logging.Debug("Embedded manifest passed JSON schema validation")
	}

	var manifest SchemaManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to parse embedded manifest")
	}

	logging.Info("Parsed embedded manifest '%s' with %d schema definitions",
		manifest.Manifest.Name, len(manifest.Schemas))

	// Validate manifest format version (fallback validation)
	if err := validateManifestVersion(&manifest); err != nil {
		logging.Error("Embedded manifest validation failed: %v", err)
		return errors.Wrapf(errors.ErrParse, err, "invalid embedded manifest")
	}

	// Validate manifest content (v1.0.0 structure) - fallback validation
	if err := validateManifestContent(&manifest); err != nil {
		logging.Error("Embedded manifest content validation failed: %v", err)
		return errors.Wrapf(errors.ErrParse, err, "invalid embedded manifest content")
	}

	// Load each schema file referenced in the manifest
	namespace := manifest.Manifest.Namespace
	if namespace == "" {
		namespace = "yago" // Default namespace if not specified
	}

	for _, schemaEntry := range manifest.Schemas {
		schemaPath := filepath.Join("schemas", schemaEntry.File)

		logging.Debug("Loading embedded schema file: %s (namespace: %s, version: %s, kind: %s)",
			schemaEntry.File, namespace, schemaEntry.Version, schemaEntry.Kind)

		schemaData, err := esp.fs.ReadFile(schemaPath)
		if err != nil {
			logging.Error("Failed to read embedded schema file %s: %v", schemaPath, err)
			return errors.Wrapf(errors.ErrParse, err, "failed to read embedded schema file %s", schemaEntry.File)
		}

		var schemaContent map[string]interface{}
		if err := json.Unmarshal(schemaData, &schemaContent); err != nil {
			logging.Error("Failed to parse embedded schema %s: %v", schemaEntry.File, err)
			return errors.Wrapf(errors.ErrParse, err, "failed to parse embedded schema %s", schemaEntry.File)
		}

		// Register the schema in the common store with namespace
		esp.store.RegisterSchema(namespace, schemaEntry.Version, schemaEntry.Kind, schemaContent)
		logging.Info("Successfully loaded embedded schema: namespace=%s, version=%s, kind=%s from %s",
			namespace, schemaEntry.Version, schemaEntry.Kind, schemaEntry.File)
	}

	// Register the manifest in the common store
	esp.store.RegisterManifest(manifest)

	logging.Info("Successfully loaded %d embedded core schemas", esp.store.GetSchemaCount())
	return nil
}
