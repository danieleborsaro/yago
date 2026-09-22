// Package assets provides embedded schema resources for the YAGO GitOps CLI
package assets

import (
	"embed"
)

// CoreSchemas embeds all core GitOps schema files from the schemas directory
// This makes the schemas available at build time without requiring external files
// The embed path is relative to this package directory
//
//go:embed schemas
var CoreSchemas embed.FS

// ManifestSchemas embeds manifest validation schemas from the manifests directory
// These schemas define the structure of manifest.json files themselves
//
//go:embed manifests
var ManifestSchemas embed.FS
