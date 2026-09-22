package secretsmanager

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// Parser implements the wrapper.Parser interface for secretsmanager operations.
// It handles parsing of secretsmanager desired state and configuration files.
type Parser struct {
	baseParser             wrapper.Parser
	environment            string
	envVars                map[string]string
	desiredState           wrapper.DesiredState
	configuration          wrapper.Configuration
	workspaceDir           string
	buildDir               string
	desiredStateSpecs      []*SecretManagerDesiredStateSpec
	configSpecs            []*SecretManagerConfigSpec
	cachedDesiredStateFile string
	cachedConfigFile       string
}

// NewParser creates a new secretsmanager parser instance.
func NewParser(env string, envVars map[string]string) wrapper.Parser {
	logging.Debug("Creating new SecretManager parser for environment: %s", env)

	parser := &Parser{
		environment:  env,
		envVars:      envVars,
		workspaceDir: ".",
	}

	return parser
}

// GetDesiredState returns the loaded desired state document.
func (p *Parser) GetDesiredState() wrapper.DesiredState {
	return p.desiredState
}

// GetConfiguration returns the loaded configuration document.
func (p *Parser) GetConfiguration() wrapper.Configuration {
	return p.configuration
}

// GetEnvironment returns the environment name this parser is configured for.
func (p *Parser) GetEnvironment() string {
	return p.environment
}

// GetEnvVariables returns the environment variables available for interpolation.
func (p *Parser) GetEnvVariables() map[string]string {
	return p.envVars
}

// LoadGitOpsFiles loads both desired state and configuration files.
func (p *Parser) LoadGitOpsFiles(req wrapper.LoadRequest) error {
	if req == nil {
		return errors.New(errors.ErrParam, "LoadRequest cannot be nil")
	}

	logging.Debug("Loading GitOps files for secretsmanager: env=%s", req.GetEnvironment())

	// Load desired state
	desiredStateRoot := req.GetDesiredStateRoot()
	if desiredStateRoot == "" {
		desiredStateRoot = filepath.Join(p.workspaceDir, "desiredstate")
	}

	logging.Debug("Loading desired state from: %s", desiredStateRoot)

	if err := p.loadDesiredStateFiles(desiredStateRoot); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to load desired state files")
	}

	// Load configuration
	configRoot := req.GetConfigRoot()
	if configRoot == "" {
		configRoot = filepath.Join(p.workspaceDir, "configuration")
	}

	logging.Debug("Loading configuration from: %s", configRoot)

	if err := p.loadConfigurationFiles(configRoot); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to load configuration files")
	}

	logging.Debug("Successfully loaded all GitOps files for secretsmanager")
	return nil
}

// loadDesiredStateFiles loads all desired state YAML files from the specified directory.
func (p *Parser) loadDesiredStateFiles(rootDir string) error {
	logging.Debug("Scanning for desired state files in: %s", rootDir)

	// Check if directory exists
	info, err := os.Stat(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Debug("Desired state directory does not exist, creating empty spec list")
			p.desiredStateSpecs = []*SecretManagerDesiredStateSpec{}
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("desired state path is not a directory: %s", rootDir)
	}

	// Find all YAML files in the directory
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !isYAMLFile(filename) {
			continue
		}

		filePath := filepath.Join(rootDir, filename)
		logging.Debug("Loading desired state from file: %s", filePath)

		spec, err := p.parseDesiredStateFile(filePath)
		if err != nil {
			logging.Warn("Failed to parse desired state file %s: %v", filePath, err)
			continue
		}

		p.desiredStateSpecs = append(p.desiredStateSpecs, spec)
	}

	logging.Debug("Loaded %d desired state files", len(p.desiredStateSpecs))
	return nil
}

// loadConfigurationFiles loads all configuration YAML files from the specified directory.
func (p *Parser) loadConfigurationFiles(rootDir string) error {
	logging.Debug("Scanning for configuration files in: %s", rootDir)

	// Check if directory exists
	info, err := os.Stat(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Debug("Configuration directory does not exist, using empty config")
			p.configSpecs = []*SecretManagerConfigSpec{}
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("configuration path is not a directory: %s", rootDir)
	}

	// Find all YAML files in the directory
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !isYAMLFile(filename) {
			continue
		}

		filePath := filepath.Join(rootDir, filename)
		logging.Debug("Loading configuration from file: %s", filePath)

		spec, err := p.parseConfigFile(filePath)
		if err != nil {
			logging.Warn("Failed to parse configuration file %s: %v", filePath, err)
			continue
		}

		p.configSpecs = append(p.configSpecs, spec)
	}

	logging.Debug("Loaded %d configuration files", len(p.configSpecs))
	return nil
}

// parseDesiredStateFile parses a single desired state YAML file.
func (p *Parser) parseDesiredStateFile(filePath string) (*SecretManagerDesiredStateSpec, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	spec := &SecretManagerDesiredStateSpec{}
	if err := yaml.Unmarshal(content, spec); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate the spec
	if err := ValidateDesiredStateSpec(spec); err != nil {
		return nil, err
	}

	return spec, nil
}

// parseConfigFile parses a single configuration YAML file.
func (p *Parser) parseConfigFile(filePath string) (*SecretManagerConfigSpec, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	spec := &SecretManagerConfigSpec{}
	if err := yaml.Unmarshal(content, spec); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return spec, nil
}

// isYAMLFile checks if a filename is a YAML file.
func isYAMLFile(filename string) bool {
	ext := filepath.Ext(filename)
	return ext == ".yaml" || ext == ".yml"
}

// GetDesiredStateSpecs returns the parsed desired state specifications.
func (p *Parser) GetDesiredStateSpecs() []*SecretManagerDesiredStateSpec {
	return p.desiredStateSpecs
}

// GetConfigSpecs returns the parsed configuration specifications.
func (p *Parser) GetConfigSpecs() []*SecretManagerConfigSpec {
	return p.configSpecs
}

// Cache caches the loaded documents.
func (p *Parser) Cache(req wrapper.CacheRequest) (*wrapper.CacheResponse, error) {
	logging.Debug("Caching secretsmanager documents")

	buildDir := req.BuildDirectory
	if buildDir == "" {
		buildDir = p.GetBuildDir()
	}

	// Create build directory if it doesn't exist
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create build directory: %s", buildDir)
	}

	// Cache desired state specs if available
	var desiredStateFile string
	if len(p.desiredStateSpecs) > 0 {
		desiredStateFile = filepath.Join(buildDir, "desired-state.yaml")
		content, err := yaml.Marshal(map[string]interface{}{
			"specs": p.desiredStateSpecs,
		})
		if err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to marshal desired state")
		}

		if err := os.WriteFile(desiredStateFile, content, 0644); err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to write cached desired state file")
		}

		logging.Debug("Cached desired state to: %s", desiredStateFile)
		p.cachedDesiredStateFile = desiredStateFile
	}

	// Cache configuration specs if available
	var configFile string
	if len(p.configSpecs) > 0 {
		configFile = filepath.Join(buildDir, "config.yaml")
		content, err := yaml.Marshal(map[string]interface{}{
			"specs": p.configSpecs,
		})
		if err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to marshal configuration")
		}

		if err := os.WriteFile(configFile, content, 0644); err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to write cached config file")
		}

		logging.Debug("Cached configuration to: %s", configFile)
		p.cachedConfigFile = configFile
	}

	return &wrapper.CacheResponse{
		BuildDir:          buildDir,
		DesiredStateFile:  desiredStateFile,
		ConfigurationFile: configFile,
		AdditionalFiles:   map[string]string{},
	}, nil
}

// Clear clears all loaded data and caches.
func (p *Parser) Clear() error {
	logging.Debug("Clearing secretsmanager parser state")

	p.desiredState = nil
	p.configuration = nil

	return nil
}

// SetWorkspace sets the workspace directory for file operations.
func (p *Parser) SetWorkspace(dir string) error {
	if dir == "" {
		return errors.New(errors.ErrParam, "workspace directory cannot be empty")
	}

	// Verify directory exists
	info, err := os.Stat(dir)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to access workspace directory: %s", dir)
	}

	if !info.IsDir() {
		return errors.New(errors.ErrParam, fmt.Sprintf("workspace path is not a directory: %s", dir))
	}

	p.workspaceDir = dir
	logging.Debug("Workspace set to: %s", dir)

	return nil
}

// GetWorkspaceDir returns the current workspace directory.
func (p *Parser) GetWorkspaceDir() string {
	return p.workspaceDir
}

// GetBuildDir returns the build directory for cached files.
func (p *Parser) GetBuildDir() string {
	if p.buildDir == "" {
		p.buildDir = filepath.Join(p.workspaceDir, ".yago", "secretsmanager")
	}
	return p.buildDir
}
