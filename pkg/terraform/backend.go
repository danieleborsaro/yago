package terraform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultBackendFileSuffix is the default suffix for backend configuration files
	DefaultBackendFileSuffix = "backend.tfvars.json"
)

// BackendConfig holds terraform backend configuration (S3/DynamoDB).
type BackendConfig struct {
	// S3 backend configuration
	Bucket        string `yaml:"bucket" json:"bucket"`
	Key           string `yaml:"key" json:"key"`
	Region        string `yaml:"region" json:"region"`
	DynamoDBTable string `yaml:"dynamodb_table" json:"dynamodb_table"`
	Encrypt       bool   `yaml:"encrypt" json:"encrypt"`

	// Additional backend options
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
	RoleARN string `yaml:"role_arn,omitempty" json:"role_arn,omitempty"`

	// Raw config for additional fields
	raw map[string]interface{}
}

// BackendManager handles loading and caching of terraform backend configuration.
type BackendManager struct {
	config         *BackendConfig
	bufferFile     string
	fileSuffix     string
	workdir        string
	schemaBasePath string
}

// NewBackendManager creates a new backend manager instance.
func NewBackendManager(workdir string) *BackendManager {
	return &BackendManager{
		fileSuffix: DefaultBackendFileSuffix,
		workdir:    workdir,
	}
}

// LoadBackendConfig loads backend configuration from the schema path.
// schemaPath is the relative path from workdir to the backend config file
// (e.g., "cicd/concourse-dev/devops-dev/cluster/terraform/backends/eu-west-1.tfvars.json")
func (bm *BackendManager) LoadBackendConfig(schemaPath string, awsRegion string) error {
	logging.Debug("Loading backend configuration from schema path: %s, region: %s", schemaPath, awsRegion)

	// Construct the full path to backend config
	configPath := filepath.Join(bm.workdir, schemaPath)

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return errors.Newf(errors.ErrConfigurationMissing, "backend configuration file not found at %s", configPath)
	}

	// Read and parse the file (could be JSON or YAML)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return errors.Wrap(errors.ErrConfigurationMissing, fmt.Sprintf("failed to read backend configuration file: %s", configPath), err)
	}

	config := &BackendConfig{}

	// Try JSON first (since the file is .tfvars.json)
	if strings.HasSuffix(configPath, ".json") {
		if err := json.Unmarshal(data, config); err != nil {
			return errors.Wrap(errors.ErrConfigurationMalformed, "failed to parse backend configuration JSON", err)
		}
		// Also parse as map for raw access
		var rawConfig map[string]interface{}
		if err := json.Unmarshal(data, &rawConfig); err != nil {
			return errors.Wrap(errors.ErrConfigurationMalformed, "failed to parse backend configuration JSON as map", err)
		}
		config.raw = rawConfig
	} else {
		// Try YAML
		if err := yaml.Unmarshal(data, config); err != nil {
			return errors.Wrap(errors.ErrConfigurationMalformed, "failed to parse backend configuration YAML", err)
		}
		// Also parse as map for raw access
		var rawConfig map[string]interface{}
		if err := yaml.Unmarshal(data, &rawConfig); err != nil {
			return errors.Wrap(errors.ErrConfigurationMalformed, "failed to parse backend configuration YAML as map", err)
		}
		config.raw = rawConfig
	}

	bm.config = config
	logging.Info("Successfully loaded backend configuration for region: %s from %s", awsRegion, configPath)

	return nil
}

// Cache generates the backend.tfvars.json file from the loaded configuration.
// - Creates tempfile or file in buildDirectory
// - Writes JSON (if generateJSON) or YAML
// - Sets backendBufferFile path
func (bm *BackendManager) Cache(buildDirectory string, generateJSON bool) error {
	if bm.config == nil {
		return errors.New(errors.ErrConfigurationMissing, "backend configuration not loaded, call LoadBackendConfig first")
	}

	logging.Debug("Caching backend configuration to build directory: %s", buildDirectory)

	var outputPath string
	var data []byte
	var err error

	// Determine output path
	if buildDirectory == "" || !dirExists(buildDirectory) {
		// Use temp file
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("*.%s", bm.fileSuffix))
		if err != nil {
			return errors.Wrap(errors.ErrFail, "failed to create temporary backend file", err)
		}
		outputPath = tmpFile.Name()
		defer tmpFile.Close()
	} else {
		// Use buildDirectory
		outputPath = filepath.Join(buildDirectory, bm.fileSuffix)
	}

	// Serialize to JSON or YAML
	if generateJSON {
		data, err = json.MarshalIndent(bm.config.raw, "", "  ")
		if err != nil {
			return errors.Wrap(errors.ErrFail, "failed to marshal backend config to JSON", err)
		}
	} else {
		data, err = yaml.Marshal(bm.config.raw)
		if err != nil {
			return errors.Wrap(errors.ErrFail, "failed to marshal backend config to YAML", err)
		}
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return errors.Wrap(errors.ErrFail, "failed to write backend configuration file", err)
	}

	bm.bufferFile = outputPath
	logging.Info("Backend configuration cached to: %s", outputPath)

	return nil
}

// GetBackendFilePath returns the path to the cached backend configuration file.
func (bm *BackendManager) GetBackendFilePath() string {
	return bm.bufferFile
}

// GetConfig returns the loaded backend configuration.
func (bm *BackendManager) GetConfig() *BackendConfig {
	return bm.config
}

// Helper function to check if directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
