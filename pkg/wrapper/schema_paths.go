package wrapper

import (
	"fmt"
	"strings"

	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// NamespaceFromMeta returns normalized namespace from document metadata.
// Defaults to yago when namespace is absent.
func NamespaceFromMeta(meta map[string]interface{}) string {
	if meta == nil {
		return "yago"
	}

	ns, ok := meta["namespace"].(string)
	if !ok || ns == "" {
		return "yago"
	}

	return strings.ToLower(ns)
}

// DiscoverPropertyPath resolves one x-gitops-paths key for a document.
func DiscoverPropertyPath(documentMeta map[string]interface{}, schemaVersion string, isDesiredState bool, key string) (string, error) {
	schemaManager, err := schema.GetOrCreateSchemaManagerAuto()
	if err != nil {
		return "", fmt.Errorf("failed to initialize schema manager: %w", err)
	}

	paths, err := schemaManager.DiscoverPropertyPaths(NamespaceFromMeta(documentMeta), schema.SchemaVersion(schemaVersion), isDesiredState)
	if err != nil {
		return "", fmt.Errorf("failed to discover property paths for schema %s: %w", schemaVersion, err)
	}

	path, err := paths.Get(key)
	if err != nil {
		return "", fmt.Errorf("failed to resolve property path %q: %w", key, err)
	}

	if path == "" {
		return "", fmt.Errorf("property path %q resolved to empty value", key)
	}

	return path, nil
}

// DiscoverPropertyPathOrFallback resolves one x-gitops-paths key for a document
// and returns fallback when schema-based resolution is unavailable.
func DiscoverPropertyPathOrFallback(documentMeta map[string]interface{}, schemaVersion string, isDesiredState bool, key string, fallback string) string {
	path, err := DiscoverPropertyPath(documentMeta, schemaVersion, isDesiredState, key)
	if err != nil {
		logging.Warn("Using fallback path for %s: x-gitops-paths resolution failed (%v)", key, err)
		return fallback
	}

	return path
}
