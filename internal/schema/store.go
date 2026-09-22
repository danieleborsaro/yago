package schema

import (
	"fmt"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// SchemaStore provides unified storage and access for schemas regardless of source
type SchemaStore struct {
	schemas    map[string]map[string]interface{} // namespace-version-kind -> schema content
	manifests  []SchemaManifest
	namespaces map[string]bool // track loaded namespaces
}

// NewSchemaStore creates a new schema store
func NewSchemaStore() *SchemaStore {
	return &SchemaStore{
		schemas:    make(map[string]map[string]interface{}),
		manifests:  make([]SchemaManifest, 0),
		namespaces: make(map[string]bool),
	}
}

// RegisterSchema adds a schema to the store with namespace prefix
func (ss *SchemaStore) RegisterSchema(namespace, version, kind string, content map[string]interface{}) {
	key := ss.makeKey(namespace, version, kind)

	// Check if schema already exists and warn about overwrite
	if _, exists := ss.schemas[key]; exists {
		logging.Warn("Schema with namespace '%s', version '%s' and kind '%s' already exists - overwriting with new definition", namespace, version, kind)
	}

	ss.schemas[key] = content
	ss.namespaces[strings.ToLower(namespace)] = true // Track this namespace (lowercase)
	logging.Debug("Registered schema with key '%s'", key)
}

// RegisterManifest adds a manifest to the store and tracks its namespace
func (ss *SchemaStore) RegisterManifest(manifest SchemaManifest) {
	ss.manifests = append(ss.manifests, manifest)
	// Track the namespace from the manifest (lowercase)
	if manifest.Manifest.Namespace != "" {
		ss.namespaces[strings.ToLower(manifest.Manifest.Namespace)] = true
	}
	logging.Debug("Added manifest to collection, total manifests: %d", len(ss.manifests))
}

// GetSchema retrieves a schema by namespace, version and kind
func (ss *SchemaStore) GetSchema(namespace, version, kind string) (map[string]interface{}, error) {
	key := ss.makeKey(namespace, version, kind)
	schema, exists := ss.schemas[key]
	if !exists {
		return nil, errors.Newf(errors.ErrParse, "schema not found for namespace %s, version %s, kind %s", namespace, version, kind)
	}
	return schema, nil
}

// GetAvailableVersions returns all available versions from stored schemas
func (ss *SchemaStore) GetAvailableVersions() []string {
	versionSet := make(map[string]bool)

	for key := range ss.schemas {
		parts := strings.Split(key, "-")
		// Key format: namespace-version-kind
		// Need at least 3 parts: namespace, version (may contain dashes), kind
		if len(parts) >= 3 {
			// Reconstruct version (everything between namespace and kind)
			// Skip first part (namespace) and last part (kind)
			version := strings.Join(parts[1:len(parts)-1], "-")
			versionSet[version] = true
		}
	}

	versions := make([]string, 0, len(versionSet))
	for version := range versionSet {
		versions = append(versions, version)
	}

	return versions
}

// GetManifests returns all stored manifests
func (ss *SchemaStore) GetManifests() []SchemaManifest {
	return ss.manifests
}

// HasSchema checks if a schema exists for the given namespace, version and kind
func (ss *SchemaStore) HasSchema(namespace, version, kind string) bool {
	key := ss.makeKey(namespace, version, kind)
	_, exists := ss.schemas[key]
	return exists
}

// makeKey creates a composite key from namespace, version and kind
// Both namespace and kind are normalized to lowercase for case-insensitive comparison
func (ss *SchemaStore) makeKey(namespace, version, kind string) string {
	return fmt.Sprintf("%s-%s-%s", strings.ToLower(namespace), version, strings.ToLower(kind))
}

// GetSchemaCount returns the total number of schemas in the store
func (ss *SchemaStore) GetSchemaCount() int {
	return len(ss.schemas)
}

// GetLoadedNamespaces returns a list of all loaded schema namespaces
func (ss *SchemaStore) GetLoadedNamespaces() []string {
	namespaces := make([]string, 0, len(ss.namespaces))
	for namespace := range ss.namespaces {
		namespaces = append(namespaces, namespace)
	}
	return namespaces
}

// IsNamespaceLoaded checks if a namespace has been loaded (case-insensitive)
func (ss *SchemaStore) IsNamespaceLoaded(namespace string) bool {
	return ss.namespaces[strings.ToLower(namespace)]
}
