package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// SchemaConfig holds configuration for schema loading
type SchemaConfig struct {
	// DefaultSchemaPath is the path to built-in schemas
	DefaultSchemaPath string `json:"defaultSchemaPath"`

	// CustomSchemaPaths are additional paths to search for schemas
	CustomSchemaPaths []string `json:"customSchemaPaths"`

	// EnablePlugins determines whether to load plugin schemas
	EnablePlugins bool `json:"enablePlugins"`

	// CacheSchemas determines whether to cache loaded schemas
	CacheSchemas bool `json:"cacheSchemas"`
}

// DefaultSchemaConfig returns the default configuration
func DefaultSchemaConfig() *SchemaConfig {
	return &SchemaConfig{
		DefaultSchemaPath: "schemas",
		CustomSchemaPaths: []string{},
		EnablePlugins:     true,
		CacheSchemas:      true,
	}
}

// LoadSchemaConfig loads schema configuration from a file
func LoadSchemaConfig(configPath string) (*SchemaConfig, error) {
	// Start with defaults
	config := DefaultSchemaConfig()

	// If config file doesn't exist, use defaults
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return config, nil
	}

	// Load config file
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to read schema config")
	}

	if err := json.Unmarshal(configData, config); err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to parse schema config")
	}

	return config, nil
}

// SaveSchemaConfig saves configuration to a file
func (sc *SchemaConfig) SaveSchemaConfig(configPath string) error {
	configData, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to marshal schema config")
	}

	// Ensure directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to create config directory")
	}

	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to write schema config")
	}

	return nil
}

// ResolveSchemaPath resolves a schema path relative to the working directory
func (sc *SchemaConfig) ResolveSchemaPath(workDir string) string {
	if filepath.IsAbs(sc.DefaultSchemaPath) {
		return sc.DefaultSchemaPath
	}
	return filepath.Join(workDir, sc.DefaultSchemaPath)
}

// GetAllSchemaPaths returns all schema paths to search
func (sc *SchemaConfig) GetAllSchemaPaths(workDir string) []string {
	paths := []string{sc.ResolveSchemaPath(workDir)}

	for _, customPath := range sc.CustomSchemaPaths {
		// Expand ~ to home directory
		expandedPath := customPath
		if strings.HasPrefix(customPath, "~/") {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				expandedPath = filepath.Join(homeDir, customPath[2:])
			}
		}

		// Resolve path
		if filepath.IsAbs(expandedPath) {
			paths = append(paths, expandedPath)
		} else {
			paths = append(paths, filepath.Join(workDir, expandedPath))
		}
	}

	return paths
}
