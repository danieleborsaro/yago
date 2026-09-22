package schema

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/xeipuuv/gojsonschema"
)

// ManifestValidator validates manifest.json files against their schema version
type ManifestValidator struct {
	manifestSchemas embed.FS
	schemaCache     map[string]*gojsonschema.Schema
}

// NewManifestValidator creates a new manifest validator with embedded schemas
func NewManifestValidator(manifestSchemas embed.FS) *ManifestValidator {
	return &ManifestValidator{
		manifestSchemas: manifestSchemas,
		schemaCache:     make(map[string]*gojsonschema.Schema),
	}
}

// ValidateManifest validates a manifest against its declared version schema
func (mv *ManifestValidator) ValidateManifest(manifestData []byte) error {
	// First, parse the manifest to get its version
	var manifest SchemaManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to parse manifest JSON")
	}

	version := manifest.Manifest.Version
	if version == "" {
		return errors.New(errors.ErrParse, "manifest.version is required")
	}

	// Get or load the schema for this version
	schema, err := mv.getSchemaForVersion(version)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load schema for version %s", version)
	}

	// Validate the manifest against the schema
	documentLoader := gojsonschema.NewBytesLoader(manifestData)
	result, err := schema.Validate(documentLoader)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "validation error")
	}

	if !result.Valid() {
		// Collect all validation errors
		errMsg := fmt.Sprintf("manifest validation failed for version %s:", version)
		for _, desc := range result.Errors() {
			errMsg += fmt.Sprintf("\n  - %s", desc)
		}
		return errors.New(errors.ErrParse, errMsg)
	}

	return nil
}

// getSchemaForVersion retrieves the JSON schema for a specific manifest version
func (mv *ManifestValidator) getSchemaForVersion(version string) (*gojsonschema.Schema, error) {
	// Check cache first
	if schema, exists := mv.schemaCache[version]; exists {
		return schema, nil
	}

	// Load schema from embedded FS
	schemaPath := fmt.Sprintf("manifests/v%s.json", version)
	schemaData, err := mv.manifestSchemas.ReadFile(schemaPath)
	if err != nil {
		return nil, errors.Newf(errors.ErrParse, "unsupported manifest version '%s': schema not found", version)
	}

	// Parse and compile the schema
	schemaLoader := gojsonschema.NewBytesLoader(schemaData)
	schema, err := gojsonschema.NewSchema(schemaLoader)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to compile schema")
	}

	// Cache for future use
	mv.schemaCache[version] = schema

	return schema, nil
}
