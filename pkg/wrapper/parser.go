package wrapper

import (
	"os"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// BaseParser provides common functionality for all wrapper parsers.
// Specific wrappers (terraform, docker, etc.) embed this and override methods as needed.
type BaseParser struct {
	environment     string
	workspaceDir    string
	workspacePrefix string
	buildDirPrefix  string
	buildDirPath    string
	envVariables    map[string]string

	// These are set by specific wrappers
	desiredState  DesiredState
	configuration Configuration
}

// NewBaseParser creates a base parser instance.
func NewBaseParser(environment string, envVars map[string]string) *BaseParser {
	if envVars == nil {
		envVars = make(map[string]string)
	}

	return &BaseParser{
		environment:     environment,
		envVariables:    envVars,
		workspacePrefix: "gitops",
		buildDirPrefix:  ".gitops",
	}
}

// GetDesiredState returns the desiredstate document.
func (p *BaseParser) GetDesiredState() DesiredState {
	return p.desiredState
}

// GetConfiguration returns the configuration document.
func (p *BaseParser) GetConfiguration() Configuration {
	return p.configuration
}

// GetEnvironment returns the environment name.
func (p *BaseParser) GetEnvironment() string {
	return p.environment
}

// GetEnvVariables returns the environment variables map.
func (p *BaseParser) GetEnvVariables() map[string]string {
	return p.envVariables
}

// GetWorkspaceDir returns the workspace directory.
func (p *BaseParser) GetWorkspaceDir() string {
	return p.workspaceDir
}

// GetBuildDir returns the build directory path.
func (p *BaseParser) GetBuildDir() string {
	return p.buildDirPath
}

// SetDesiredState sets the desiredstate document (for subclasses).
func (p *BaseParser) SetDesiredState(ds DesiredState) {
	p.desiredState = ds
}

// SetConfiguration sets the configuration document (for subclasses).
func (p *BaseParser) SetConfiguration(cfg Configuration) {
	p.configuration = cfg
}

// GetConfigurationMeta returns the configuration metadata (meta section).
// This contains the raw configuration structure including wrapper-specific paths.
// Common pattern used by wrappers to access configuration metadata like backend paths, etc.
func (p *BaseParser) GetConfigurationMeta() map[string]interface{} {
	if p.configuration == nil {
		return nil
	}
	doc := p.configuration.GetDocument()
	if doc == nil {
		return nil
	}
	return doc.GetMeta().Data
}

// GetDesiredStateMeta returns the desiredstate metadata (meta section).
// This contains the raw desiredstate structure.
func (p *BaseParser) GetDesiredStateMeta() map[string]interface{} {
	if p.desiredState == nil {
		return nil
	}
	doc := p.desiredState.GetDocument()
	if doc == nil {
		return nil
	}
	return doc.GetMeta().Data
}

// SetWorkspace sets up the workspace directory for cloned assets.
func (p *BaseParser) SetWorkspace(dir string) error {
	logging.Debug("Creating workspace for cloned assets...")

	if dir != "" {
		// Override with provided directory
		p.workspaceDir = dir
		if err := os.MkdirAll(p.workspaceDir, 0755); err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to create workspace directory")
		}
		logging.Info("Workspace directory: %s", p.workspaceDir)
	} else if p.workspaceDir == "" {
		// Create temporary workspace
		tmpDir, err := os.MkdirTemp("", p.workspacePrefix+"-")
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to create temp workspace")
		}
		p.workspaceDir = tmpDir
		logging.Info("Workspace directory: %s", p.workspaceDir)
	} else {
		logging.Debug("Workspace dir already created")
	}

	return nil
}

// Cache saves assembled content to temporary files.
// Specific wrappers can override this to add wrapper-specific caching logic.
func (p *BaseParser) Cache(req CacheRequest) (*CacheResponse, error) {
	logging.Debug("Caching to file(s)")

	var buildDirPath string

	if req.BuildDirectory != "" {
		// Use provided build directory
		buildDirPath = req.BuildDirectory
		if err := os.MkdirAll(buildDirPath, 0755); err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to create build directory")
		}
	} else {
		// Create temporary build directory
		tmpDir, err := os.MkdirTemp("", p.buildDirPrefix+"-")
		if err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to create temp build directory")
		}
		buildDirPath = tmpDir
	}

	p.buildDirPath = buildDirPath
	logging.Debug("Created build directory: %s", p.buildDirPath)

	response := &CacheResponse{
		BuildDir:        buildDirPath,
		AdditionalFiles: make(map[string]string),
	}

	// Determine which documents to cache
	documents := req.Documents
	if len(documents) == 0 {
		// Default: cache both desiredstate and configuration if they exist
		if p.desiredState != nil {
			documents = append(documents, p.desiredState)
		}
		if p.configuration != nil {
			documents = append(documents, p.configuration)
		}
	}

	// Determine format
	format := "yaml"
	if req.GenerateJSON {
		format = "json"
	}

	// Cache each document
	for _, doc := range documents {
		switch d := doc.(type) {
		case DesiredState:
			file, err := d.Cache(buildDirPath, format)
			if err != nil {
				return nil, errors.Wrapf(errors.ErrFail, err, "failed to cache desiredstate")
			}
			response.DesiredStateFile = file

		case Configuration:
			file, err := d.Cache(buildDirPath, format)
			if err != nil {
				return nil, errors.Wrapf(errors.ErrFail, err, "failed to cache configuration")
			}
			response.ConfigurationFile = file

		default:
			logging.Warn("Unknown document type in cache request, skipping")
		}
	}

	return response, nil
}

// Clear removes cached files.
// Specific wrappers can override this to clear additional files.
func (p *BaseParser) Clear() error {
	logging.Debug("Clearing cached file(s)")

	// Remove build directory if it exists
	if p.buildDirPath != "" {
		if err := os.RemoveAll(p.buildDirPath); err != nil {
			logging.Warn("Unable to delete build directory: %s", err)
			return err
		}
		p.buildDirPath = ""
	}

	return nil
}

// LoadGitOpsFiles loads desiredstate and configuration documents.
// This is the base implementation that loads both documents and stores them in the parser.
// Specific wrappers can call this method and then load additional wrapper-specific documents.
func (p *BaseParser) LoadGitOpsFiles(req LoadRequest) error {
	logging.Debug("BaseParser: Loading GitOps files")

	// Validate required parameters
	if req.GetDesiredStateRoot() == "" {
		return errors.New(errors.ErrParam, "desiredstate root is required")
	}

	// Load desiredstate
	// Note: Using empty wrapper string for base loading - specific wrappers can override
	logging.Debug("Loading desiredstate from: %s", req.GetDesiredStateRoot())
	ds := NewBaseDesiredState(req.GetEnvironment(), "", p.envVariables)
	if err := ds.LoadGitOpsFile(req.GetDesiredStateRoot()); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate from %s", req.GetDesiredStateRoot())
	}
	p.desiredState = ds
	logging.Info("Desiredstate loaded successfully")

	// Load configuration if provided
	if req.GetConfigRoot() != "" {
		logging.Debug("Loading configuration from: %s", req.GetConfigRoot())
		cfg := NewBaseConfiguration(req.GetEnvironment(), "", p.envVariables)

		// Get schema override from desiredstate for configuration
		dsDoc := ds.GetDocument()
		var schemaVersionOverride string
		if dsDoc != nil {
			if meta := dsDoc.GetMeta(); meta != nil && meta.Data != nil {
				if schema, ok := meta.Data["schema"].(string); ok {
					schemaVersionOverride = schema
				}
			}
		}

		if err := cfg.LoadGitOpsFile(schemaVersionOverride); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to load configuration from %s", req.GetConfigRoot())
		}
		p.configuration = cfg
		logging.Info("Configuration loaded successfully")
	}

	return nil
}
