package secretsmanager

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// Service implements the wrapper.Service interface for secretsmanager operations.
// It provides validation, assembly, and schema query capabilities for secrets.
type Service struct {
	baseDir             string
	enableInterpolation bool
}

// NewService creates a new secretsmanager service instance.
func NewService(baseDir string, enableInterpolation bool) wrapper.Service {
	logging.Debug("Creating new SecretManager service (baseDir=%s, interpolation=%v)",
		baseDir, enableInterpolation)

	return &Service{
		baseDir:             baseDir,
		enableInterpolation: enableInterpolation,
	}
}

// Validate validates a secretsmanager GitOps file and optionally a configuration file.
func (s *Service) Validate(req wrapper.ValidateRequest) (*wrapper.ValidateResponse, error) {
	logging.Info("Validating secretsmanager: %s", req.DesiredStateFile)

	response := &wrapper.ValidateResponse{
		IsValid:                true,
		DesiredStateValidated:  true,
		ConfigurationValidated: true,
		SchemaVersion:          req.SchemaVersion,
	}

	// Validate desired state file if specified
	if req.DesiredStateFile != "" {
		if err := s.validateDesiredStateFile(req.DesiredStateFile); err != nil {
			response.IsValid = false
			response.DesiredStateValidated = false
			response.ErrorMessage = err.Error()
			logging.Warn("Desired state validation failed: %v", err)
			return response, nil
		}
	}

	// Validate configuration file if specified
	if req.ConfigFile != "" {
		if err := s.validateConfigurationFile(req.ConfigFile); err != nil {
			response.IsValid = false
			response.ConfigurationValidated = false
			response.ErrorMessage = err.Error()
			logging.Warn("Configuration validation failed: %v", err)
			return response, nil
		}
	}

	logging.Info("Validation complete for secretsmanager: %s (valid=%v)",
		req.DesiredStateFile, response.IsValid)

	return response, nil
}

// validateDesiredStateFile validates a desired state YAML file.
func (s *Service) validateDesiredStateFile(filePath string) error {
	logging.Debug("Validating desired state file: %s", filePath)

	// Check file exists
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("desired state file not found: %s", filePath)
	}

	// Read and parse YAML
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read desired state file: %w", err)
	}

	spec := &SecretManagerDesiredStateSpec{}
	if err := yaml.Unmarshal(content, spec); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate schema
	if err := ValidateDesiredStateSpec(spec); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	logging.Debug("Desired state file validation successful")
	return nil
}

// validateConfigurationFile validates a configuration YAML file.
func (s *Service) validateConfigurationFile(filePath string) error {
	logging.Debug("Validating configuration file: %s", filePath)

	// Check file exists
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("configuration file not found: %s", filePath)
	}

	// Read and parse YAML
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read configuration file: %w", err)
	}

	spec := &SecretManagerConfigSpec{}
	if err := yaml.Unmarshal(content, spec); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	logging.Debug("Configuration file validation successful")
	return nil
}

// Assemble assembles a secretsmanager GitOps file with parts and optionally caches it.
func (s *Service) Assemble(req wrapper.AssembleRequest) (*wrapper.AssembleResponse, error) {
	logging.Info("Assembling secretsmanager: %s", req.DesiredStateFile)

	response := &wrapper.AssembleResponse{
		IsValid:               true,
		DesiredStateAssembled: true,
		DesiredStateFile:      req.DesiredStateFile,
	}

	// Validate the desired state file first
	if err := s.validateDesiredStateFile(req.DesiredStateFile); err != nil {
		response.IsValid = false
		response.DesiredStateAssembled = false
		response.ErrorMessage = err.Error()
		logging.Warn("Assembly failed during validation: %v", err)
		return response, nil
	}

	// Cache if requested
	if req.CacheDirectory != "" {
		cacheDir := req.CacheDirectory
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			response.IsValid = false
			response.DesiredStateAssembled = false
			response.ErrorMessage = fmt.Sprintf("failed to create cache directory: %v", err)
			logging.Warn("Assembly failed during caching: %v", err)
			return response, nil
		}

		// Copy the desired state file to the cache
		cachedFile := filepath.Join(cacheDir, filepath.Base(req.DesiredStateFile))
		content, err := os.ReadFile(req.DesiredStateFile)
		if err != nil {
			response.IsValid = false
			response.DesiredStateAssembled = false
			response.ErrorMessage = fmt.Sprintf("failed to read desired state for caching: %v", err)
			logging.Warn("Assembly failed during file caching: %v", err)
			return response, nil
		}

		if err := os.WriteFile(cachedFile, content, 0644); err != nil {
			response.IsValid = false
			response.DesiredStateAssembled = false
			response.ErrorMessage = fmt.Sprintf("failed to cache desired state file: %v", err)
			logging.Warn("Assembly failed writing cached file: %v", err)
			return response, nil
		}

		response.DesiredStateFile = cachedFile
		logging.Debug("Cached desired state to: %s", cachedFile)
	}

	logging.Info("Assembly complete for secretsmanager: %s", req.DesiredStateFile)

	return response, nil
}

// mergePartFiles merges part files into the desired state file.
func (s *Service) mergePartFiles(mainFile string, parts []string) error {
	logging.Debug("Merging %d part files into: %s", len(parts), mainFile)

	// Read the main file
	mainContent, err := os.ReadFile(mainFile)
	if err != nil {
		return fmt.Errorf("failed to read main file: %w", err)
	}

	mainSpec := &SecretManagerDesiredStateSpec{}
	if err := yaml.Unmarshal(mainContent, mainSpec); err != nil {
		return fmt.Errorf("failed to parse main file YAML: %w", err)
	}

	// Merge each part file
	for _, partPath := range parts {
		logging.Debug("Merging part file: %s", partPath)

		partContent, err := os.ReadFile(partPath)
		if err != nil {
			return fmt.Errorf("failed to read part file %s: %w", partPath, err)
		}

		partSpec := &SecretManagerDesiredStateSpec{}
		if err := yaml.Unmarshal(partContent, partSpec); err != nil {
			return fmt.Errorf("failed to parse part file YAML %s: %w", partPath, err)
		}

		// Add secrets from part to main
		mainSpec.Secrets = append(mainSpec.Secrets, partSpec.Secrets...)
	}

	// Write back the merged content
	mergedContent, err := yaml.Marshal(mainSpec)
	if err != nil {
		return fmt.Errorf("failed to marshal merged content: %w", err)
	}

	if err := os.WriteFile(mainFile, mergedContent, 0644); err != nil {
		return fmt.Errorf("failed to write merged file: %w", err)
	}

	logging.Debug("Successfully merged %d part files", len(parts))
	return nil
}

// GetSupportedSchemaVersions returns a list of all supported secretsmanager schema versions.
func (s *Service) GetSupportedSchemaVersions() []string {
	logging.Debug("Fetching supported secretsmanager schema versions")

	versions := []string{
		"gitops.io/v1",
		"gitops.io/v2",
	}

	return versions
}
