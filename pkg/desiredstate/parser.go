package desiredstate

import (
	"os"

	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// DesiredState extends BaseDesiredState for desiredstate-specific functionality.
// This provides a concrete type for the desiredstate wrapper.
type DesiredState struct {
	*wrapper.BaseDesiredState
}

// NewDesiredState creates a desiredstate-specific instance.
func NewDesiredState(environment string, envVars map[string]string) *DesiredState {
	base := wrapper.NewBaseDesiredState(environment, "meta", envVars)

	return &DesiredState{
		BaseDesiredState: base,
	}
}

// Configuration extends BaseConfiguration for desiredstate-specific functionality.
// This provides a concrete type for the desiredstate wrapper.
type Configuration struct {
	*wrapper.BaseConfiguration

	// Desiredstate-specific fields
	wrapperFilter string
}

// NewConfiguration creates a configuration instance for desiredstate.
func NewConfiguration(environment string, envVars map[string]string) *Configuration {
	base := wrapper.NewBaseConfiguration(environment, "meta", envVars)

	return &Configuration{
		BaseConfiguration: base,
	}
}

// SetWrapperFilter sets the wrapper type for filtering configuration content.
func (cfg *Configuration) SetWrapperFilter(wrapper string) {
	cfg.wrapperFilter = wrapper
}

// GetWrapperFilter returns the wrapper filter.
func (cfg *Configuration) GetWrapperFilter() string {
	return cfg.wrapperFilter
}

// Parser extends BaseParser for desiredstate-specific operations.
// This is the main parser for the desiredstate wrapper.
type Parser struct {
	*wrapper.BaseParser

	// Override with concrete types for type safety
	desiredState  *DesiredState
	configuration *Configuration
}

// NewParser creates a desiredstate parser.
func NewParser(environment string, envVars map[string]string) *Parser {
	base := wrapper.NewBaseParser(environment, envVars)

	return &Parser{
		BaseParser: base,
	}
}

// LoadRequest contains parameters for loading desiredstate and configuration files.
type LoadRequest struct {
	DesiredStateRoot string
	ConfigRoot       string
	Environment      string
	Wrapper          string
}

// Implement wrapper.LoadRequest interface
func (req LoadRequest) GetEnvironment() string {
	return req.Environment
}

func (req LoadRequest) GetDesiredStateRoot() string {
	return req.DesiredStateRoot
}

func (req LoadRequest) GetConfigRoot() string {
	return req.ConfigRoot
}

// LoadGitOpsFiles loads desiredstate and configuration files.
// This implements the wrapper.Parser interface.
func (p *Parser) LoadGitOpsFiles(req wrapper.LoadRequest) error {
	// Type assert to get concrete request
	loadReq, ok := req.(LoadRequest)
	if !ok {
		return errors.New(errors.ErrParam, "invalid request type for desiredstate parser")
	}

	// Validate input
	if loadReq.DesiredStateRoot == "" {
		return errors.NewParamError("desiredstate file must be specified")
	}

	// Check if desiredstate file exists
	if _, err := os.Stat(loadReq.DesiredStateRoot); os.IsNotExist(err) {
		return errors.NewDesiredStateMissingError(loadReq.DesiredStateRoot)
	}

	// Create and load desiredstate
	p.desiredState = NewDesiredState(loadReq.Environment, p.GetEnvVariables())
	if err := p.desiredState.LoadGitOpsFile(loadReq.DesiredStateRoot); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate")
	}

	// Update environment from loaded desiredstate if needed
	if p.desiredState.GetEnvironment() != "" {
		p.BaseParser.SetDesiredState(p.desiredState)
	}

	// Load configuration if provided
	if loadReq.ConfigRoot != "" {
		// Check if config file exists
		if _, err := os.Stat(loadReq.ConfigRoot); os.IsNotExist(err) {
			return errors.NewConfigurationMissingError(loadReq.ConfigRoot)
		}

		p.configuration = NewConfiguration(p.desiredState.GetEnvironment(), p.GetEnvVariables())
		p.configuration.SetMetaFile(loadReq.ConfigRoot)
		p.configuration.SetWrapperFilter(loadReq.Wrapper)

		// Load with API version from desiredstate
		schemaVersion := p.desiredState.GetSchemaVersion()
		if err := p.configuration.LoadGitOpsFile(schemaVersion); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to load configuration")
		}

		p.BaseParser.SetConfiguration(p.configuration)
	}

	logging.Debug("Successfully loaded desiredstate and configuration")
	return nil
}

// GetDesiredState returns the desiredstate document (interface compliance).
func (p *Parser) GetDesiredState() wrapper.DesiredState {
	return p.desiredState
}

// GetConfiguration returns the configuration document (interface compliance).
func (p *Parser) GetConfiguration() wrapper.Configuration {
	return p.configuration
}

// GetDesiredStateTyped returns the typed desiredstate for internal use.
func (p *Parser) GetDesiredStateTyped() *DesiredState {
	return p.desiredState
}

// GetConfigurationTyped returns the typed configuration for internal use.
func (p *Parser) GetConfigurationTyped() *Configuration {
	return p.configuration
}

// ValidateFiles validates both desiredstate and configuration files.
// This is a helper method for the service layer.
func (p *Parser) ValidateFiles(wrapperFilter string) error {
	// Validate desiredstate (already done during load)
	if p.desiredState == nil {
		return errors.New(errors.ErrFail, "desiredstate not loaded")
	}

	// Validate configuration if loaded
	if p.configuration != nil {
		if err := p.configuration.Validate(); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "configuration validation failed")
		}
	}

	return nil
}

// AssembleFiles assembles desiredstate and configuration with parts.
// Returns the assembled content for both documents.
func (p *Parser) AssembleFiles() (dsContent, cfgContent map[string]interface{}, err error) {
	if p.desiredState == nil {
		return nil, nil, errors.New(errors.ErrFail, "desiredstate not loaded")
	}

	// Get assembled desiredstate content
	dsContent = p.desiredState.GetDocument().GetContent().Data
	if dsContent == nil {
		return nil, nil, errors.NewDesiredStateMalformedError("desiredstate contains no valid assembled content")
	}

	// Get assembled configuration content if available
	if p.configuration != nil {
		cfgContent = p.configuration.GetDocument().GetContent().Data
	}

	return dsContent, cfgContent, nil
}

// GetSchemaVersion returns the schema version from desiredstate.
func (p *Parser) GetSchemaVersion() string {
	if p.desiredState == nil {
		return "unknown"
	}
	return p.desiredState.GetSchemaVersion()
}

// FilterConfigurationByWrapper filters configuration content by wrapper type.
// This matches the logic from service.go.
func FilterConfigurationByWrapper(configContent map[string]interface{}, wrapperFilter string) map[string]interface{} {
	if wrapperFilter == "" || wrapperFilter == "meta" {
		logging.Debug("No wrapper filter specified, returning full configuration")
		return configContent
	}

	// Navigate to configuration.content.wrappers.<wrapper>
	if config, ok := configContent["configuration"].(map[string]interface{}); ok {
		if content, ok := config["content"].(map[string]interface{}); ok {
			if wrappers, ok := content["wrappers"].(map[string]interface{}); ok {
				if wrapperContent, ok := wrappers[wrapperFilter].(map[string]interface{}); ok {
					logging.Debug("Filtered configuration for wrapper: %s", wrapperFilter)
					return wrapperContent
				}
			}
		}
	}

	logging.Debug("Wrapper '%s' not found in configuration, returning full content", wrapperFilter)
	return configContent
}

// SetWrapperForConfiguration sets the wrapper type in the configuration document.
// This is needed for parts assembly.
func (p *Parser) SetWrapperForConfiguration(wrapper string) error {
	if p.configuration == nil {
		return errors.New(errors.ErrFail, "configuration not loaded")
	}

	// Use the document's method to set wrapper
	p.configuration.GetDocument().SetWrapperForConfiguration(wrapper)
	logging.Debug("Set wrapper '%s' for configuration parts assembly", wrapper)

	return nil
}

// LoadConfigurationParts loads and assembles configuration parts.
// This should be called after SetWrapperForConfiguration.
func (p *Parser) LoadConfigurationParts() error {
	if p.configuration == nil {
		return errors.New(errors.ErrFail, "configuration not loaded")
	}

	if err := p.configuration.GetDocument().LoadParts(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load configuration parts")
	}

	logging.Debug("Successfully loaded configuration parts")
	return nil
}

// ExtractSchemaVersion extracts the schema version from a file.
// This delegates to the document's method.
func (p *Parser) ExtractSchemaVersion(filePath string) (string, error) {
	if p.desiredState == nil {
		// Create temporary document to extract version
		doc := core.NewGitOpsDocument()
		return doc.ExtractSchemaVersionFromFile("", filePath)
	}

	return p.desiredState.GetDocument().ExtractSchemaVersionFromFile("", filePath)
}

// DetectSchemaVersion detects the schema version from configuration content.
func (p *Parser) DetectSchemaVersion(content map[string]interface{}) (schema.SchemaVersion, error) {
	if p.configuration == nil {
		return "", errors.New(errors.ErrFail, "configuration not loaded")
	}

	return p.configuration.GetDocument().DetectSchemaVersion(content)
}
