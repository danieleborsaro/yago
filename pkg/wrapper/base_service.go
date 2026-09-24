package wrapper

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// BaseService provides shared business logic for all wrappers.
// Wrappers embed this struct and add their specific operations.
type BaseService struct {
	baseDir               string
	isInterpolation       bool
	handler               *parser.YAMLHandler
	document              *core.GitOpsDocument
	configurationDocument *core.GitOpsDocument // Last loaded configuration document
	assembleHooks         AssembleHooks
}

// AssembleContext carries shared state for assemble lifecycle hooks.
type AssembleContext struct {
	Document              *core.GitOpsDocument
	ConfigurationDocument *core.GitOpsDocument
	BaseDir               string
	IsInterpolation       bool
}

// CacheContext carries cache-specific state for post-cache hooks.
type CacheContext struct {
	Document              *core.GitOpsDocument
	ConfigurationDocument *core.GitOpsDocument
	CacheDirectory        string
	OutputFormat          string
}

// AssembleHooks defines extension points for wrapper-specific orchestration.
// Default implementation is no-op to preserve behavior for wrappers that do not specialize.
type AssembleHooks interface {
	PreAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error
	PostDesiredStateAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error
	PostConfigurationAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error
	PostAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error
	PostCache(req AssembleRequest, response *AssembleResponse, ctx *CacheContext) error
}

type noOpAssembleHooks struct{}

func (h noOpAssembleHooks) PreAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error {
	return nil
}

func (h noOpAssembleHooks) PostDesiredStateAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error {
	return nil
}

func (h noOpAssembleHooks) PostConfigurationAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error {
	return nil
}

func (h noOpAssembleHooks) PostAssemble(req AssembleRequest, response *AssembleResponse, ctx *AssembleContext) error {
	return nil
}

func (h noOpAssembleHooks) PostCache(req AssembleRequest, response *AssembleResponse, ctx *CacheContext) error {
	return nil
}

// NewBaseService creates a new base service instance.
func NewBaseService(baseDir string, enableInterpolation bool) *BaseService {
	handler := parser.NewYAMLHandler(baseDir)
	handler.SetInterpolation(enableInterpolation)
	document := core.NewGitOpsDocument()

	return &BaseService{
		baseDir:         baseDir,
		isInterpolation: enableInterpolation,
		handler:         handler,
		document:        document,
		assembleHooks:   noOpAssembleHooks{},
	}
}

// SetAssembleHooks configures wrapper-specific assemble/cache hooks.
// Passing nil resets hooks to default no-op behavior.
func (s *BaseService) SetAssembleHooks(hooks AssembleHooks) {
	if hooks == nil {
		s.assembleHooks = noOpAssembleHooks{}
		return
	}

	s.assembleHooks = hooks
}

// ValidateRequest contains parameters for validation operations.
type ValidateRequest struct {
	DesiredStateFile  string
	ConfigFile        string
	ConfigRepoWorkdir string
	Environment       string
	SchemaVersion     string
	Wrapper           string
}

// ValidateResponse contains the results of validation operations.
type ValidateResponse struct {
	IsValid                bool
	DesiredStateValidated  bool
	ConfigurationValidated bool
	SchemaVersion          string
	ErrorMessage           string
}

// AssembleRequest contains parameters for assembly operations.
type AssembleRequest struct {
	DesiredStateFile  string
	ConfigFile        string
	ConfigRepoWorkdir string
	Environment       string
	Wrapper           string
	CacheDirectory    string
	OutputFormat      string
}

// AssembleResponse contains the results of assembly operations.
type AssembleResponse struct {
	IsValid                       bool
	DesiredStateAssembled         bool
	ConfigurationAssembled        bool
	DesiredStateFile              string
	ConfigurationFile             string
	SchemaVersion                 string
	ErrorMessage                  string
	AssembledConfigurationContent map[string]interface{}
}

// Validate validates a GitOps desired state file and optionally a configuration file.
// This is the base implementation that all wrappers can use or override.
func (s *BaseService) Validate(req ValidateRequest) (*ValidateResponse, error) {
	logging.Debug("Starting validation with request: DesiredStateFile=%s, Environment=%s, ConfigFile=%s",
		req.DesiredStateFile, req.Environment, req.ConfigFile)

	response := &ValidateResponse{}

	// Validate input parameters
	if req.DesiredStateFile == "" {
		return nil, errors.NewParamError("desiredstate file must be specified")
	}

	// Check if desiredstate file exists
	if _, err := os.Stat(req.DesiredStateFile); os.IsNotExist(err) {
		return response, errors.NewDesiredStateMissingError(req.DesiredStateFile)
	}

	// Check config file if provided
	if req.ConfigFile != "" {
		if _, err := os.Stat(req.ConfigFile); os.IsNotExist(err) {
			return response, errors.NewConfigurationMissingError(req.ConfigFile)
		}
	}

	// Load and validate desiredstate file using GitOpsDocument
	logging.Debug("Loading file: %s", req.DesiredStateFile)
	// GitOpsDocument automatically validates the file during loading
	// Desiredstate files don't need override from another desiredstate, so pass nil
	err := s.document.LoadGitOpsFile(req.DesiredStateFile, false, nil)
	if err != nil {
		return response, errors.Wrapf(errors.ErrParse, err, "failed to load file %s", req.DesiredStateFile)
	}

	// Get the raw content from the loaded document (use meta since parts loading is disabled)
	content := s.document.GetMeta().Data
	if content == nil {
		return response, errors.NewDesiredStateMalformedError(fmt.Sprintf("file %s contains no valid YAML content", req.DesiredStateFile))
	}

	logging.Debug("Successfully loaded and parsed YAML content from %s", req.DesiredStateFile)
	// GitOpsDocument has already validated the content during LoadGitOpsFile
	response.DesiredStateValidated = true
	logging.Info("DesiredState validation successful: %s (schema: %s)", req.DesiredStateFile, s.document.GetSchemaVersion())

	// Detect and store the schema version used
	schemaVersion, err := s.document.ExtractSchemaVersionFromFile("", req.DesiredStateFile)
	if err != nil {
		logging.Debug("Could not detect schema version, using default: %v", err)
		response.SchemaVersion = "unknown"
	} else {
		response.SchemaVersion = schemaVersion
	}
	response.DesiredStateValidated = true

	// Validate configuration file if provided
	if req.ConfigFile != "" {
		logging.Info("Loading configuration file: %s", req.ConfigFile)

		// Load configuration through GitOpsDocument to get repo context
		// Pass desiredstate document so it can extract and apply overrides
		configDoc := core.NewGitOpsDocument()
		err := configDoc.LoadGitOpsFile(req.ConfigFile, false, s.document)
		if err != nil {
			response.ErrorMessage = fmt.Sprintf("failed to load config file %s: %v", req.ConfigFile, err)
			return response, errors.Wrapf(errors.ErrParse, err, "failed to load config file %s", req.ConfigFile)
		}

		// Get the configuration content
		configContent := configDoc.GetMeta().Data
		if configContent != nil {
			// Apply wrapper filtering to configuration content
			filteredConfigContent := s.filterConfigurationByWrapper(configContent, req.Wrapper)

			// Validate filtered configuration content using already-detected schema
			err = s.handler.ValidateSchema(filteredConfigContent, schema.SchemaVersion(configDoc.GetSchemaVersion()), schema.SchemaTypeConfigurationMeta)
			if err != nil {
				response.ErrorMessage = fmt.Sprintf("config validation failed for file %s: %v", req.ConfigFile, err)
				return response, errors.Wrapf(errors.ErrParse, err, "config validation failed for file %s", req.ConfigFile)
			}
			response.ConfigurationValidated = true
			logging.Info("Configuration validation successful: %s (schema: %s)", req.ConfigFile, configDoc.GetSchemaVersion())
		}
	} else {
		// No config file provided - need to assemble desiredstate parts first,
		// then clone configuration from the assembled content
		logging.Info("No configuration file provided, cloning from desiredstate specification")

		// Load and assemble desiredstate parts
		logging.Debug("Assembling desiredstate parts to extract configuration repository info")
		err = s.document.LoadParts()
		if err != nil {
			response.ErrorMessage = fmt.Sprintf("failed to load and assemble desiredstate parts: %v", err)
			return response, errors.Wrapf(errors.ErrParse, err, "failed to load and assemble desiredstate parts")
		}

		// Get the assembled desiredstate content (which now includes all parts)
		dsContent := s.document.GetContent().Data
		if dsContent == nil {
			return response, errors.NewDesiredStateMalformedError("assembled desiredstate content is empty, cannot clone configuration")
		}

		// Clone and load the configuration from the assembled content
		// Pass the desiredstate document for override extraction
		configDoc, err := s.cloneAndLoadConfiguration(dsContent, req.Wrapper, req.Environment, req.ConfigRepoWorkdir, s.document)
		if err != nil {
			response.ErrorMessage = fmt.Sprintf("failed to clone configuration: %v", err)
			return response, errors.Wrapf(errors.ErrParse, err, "failed to clone configuration from desiredstate")
		}

		configContent := configDoc.GetContent().Data
		configSchemaVersion := configDoc.GetSchemaVersion()

		// Apply wrapper filtering to cloned configuration content
		filteredConfigContent := s.filterConfigurationByWrapper(configContent, req.Wrapper)

		// Validate filtered configuration content using the already-detected schema version
		err = s.handler.ValidateSchema(filteredConfigContent, schema.SchemaVersion(configSchemaVersion), schema.SchemaTypeConfigurationMeta)
		if err != nil {
			response.ErrorMessage = fmt.Sprintf("cloned config validation failed: %v", err)
			return response, errors.Wrapf(errors.ErrParse, err, "cloned config validation failed")
		}

		response.ConfigurationValidated = true
		logging.Info("Configuration validation successful (cloned from desiredstate, schema: %s)", configSchemaVersion)
	}

	response.IsValid = true
	logging.Debug("Validation completed successfully")

	return response, nil
}

// Assemble assembles a GitOps desired state file and optionally caches it to the filesystem.
// This is the base implementation that all wrappers can use or override.
func (s *BaseService) Assemble(req AssembleRequest) (*AssembleResponse, error) {
	logging.Debug("Starting assembly with request: DesiredStateFile=%s, Environment=%s, ConfigFile=%s, Wrapper=%s",
		req.DesiredStateFile, req.Environment, req.ConfigFile, req.Wrapper)

	response := &AssembleResponse{}
	if earlyResponse, err := s.assembleValidateRequest(req, response); err != nil {
		if earlyResponse != nil {
			return earlyResponse, err
		}
		return nil, err
	}

	if err := s.invokePreAssembleHook(req, response); err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "pre-assemble hook failed")
	}

	if err := s.assembleDesiredState(req, response); err != nil {
		return response, err
	}

	if err := s.invokePostDesiredStateAssembleHook(req, response); err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "post-desiredstate-assemble hook failed")
	}

	if err := s.assembleConfiguration(req, response); err != nil {
		return response, err
	}

	if err := s.invokePostConfigurationAssembleHook(req, response); err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "post-configuration-assemble hook failed")
	}

	if err := s.assembleFinalizeAndCache(req, response); err != nil {
		return response, err
	}

	if err := s.invokePostAssembleHook(req, response); err != nil {
		return response, errors.Wrapf(errors.ErrFail, err, "post-assemble hook failed")
	}

	response.IsValid = true
	logging.Debug("Assembly completed successfully")

	return response, nil
}

func (s *BaseService) assembleValidateRequest(req AssembleRequest, response *AssembleResponse) (*AssembleResponse, error) {
	// Validate input parameters
	if req.DesiredStateFile == "" {
		return nil, errors.NewParamError("desiredstate file must be specified")
	}

	// Validate output format
	if req.OutputFormat != "" && req.OutputFormat != "yaml" && req.OutputFormat != "json" {
		return nil, errors.Newf(errors.ErrParam, "output format must be 'yaml' or 'json', got '%s'", req.OutputFormat)
	}

	// Check if desiredstate file exists
	if _, err := os.Stat(req.DesiredStateFile); os.IsNotExist(err) {
		return response, errors.NewDesiredStateMissingError(req.DesiredStateFile)
	}

	// Check config file if provided
	if req.ConfigFile != "" {
		if _, err := os.Stat(req.ConfigFile); os.IsNotExist(err) {
			return response, errors.NewConfigurationMissingError(req.ConfigFile)
		}
	}

	return nil, nil
}

func (s *BaseService) assembleDesiredState(req AssembleRequest, response *AssembleResponse) error {
	// Load and validate desiredstate file with parts assembly enabled
	logging.Debug("Loading and assembling desiredstate file: %s", req.DesiredStateFile)
	// Desiredstate files don't need override from another desiredstate, so pass nil
	err := s.document.LoadGitOpsFile(req.DesiredStateFile, true, nil) // Enable parts assembly
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load and assemble file %s", req.DesiredStateFile)
	}

	// Get the assembled content from the loaded document
	content := s.document.GetContent().Data
	if content == nil {
		return errors.NewDesiredStateMalformedError(fmt.Sprintf("file %s contains no valid assembled content", req.DesiredStateFile))
	}

	logging.Debug("Successfully loaded and assembled YAML content from %s", req.DesiredStateFile)
	response.DesiredStateAssembled = true
	logging.Info("DesiredState assembly and validation successful: %s (schema: %s)", req.DesiredStateFile, s.document.GetSchemaVersion())

	if err := s.filterDesiredStateConfigurationRefsByEnvironment(req.Environment); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to filter desiredstate configuration refs by environment")
	}

	// Detect and store the schema version used
	schemaVersion, err := s.document.ExtractSchemaVersionFromFile("", req.DesiredStateFile)
	if err != nil {
		logging.Debug("Could not detect schema version, using default: %v", err)
		response.SchemaVersion = "unknown"
	} else {
		response.SchemaVersion = schemaVersion
	}

	return nil
}

func (s *BaseService) assembleConfiguration(req AssembleRequest, response *AssembleResponse) error {
	// Handle configuration file if provided, with wrapper filtering and parts assembly
	if req.ConfigFile != "" {
		err := s.assembleConfigurationFromFile(req, response)
		if err != nil {
			return err
		}
	} else {
		err := s.assembleConfigurationFromDesiredState(req, response)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *BaseService) assembleFinalizeAndCache(req AssembleRequest, response *AssembleResponse) error {
	// Cache files if cache directory is specified
	if req.CacheDirectory != "" {
		err := s.cacheAssembledFiles(req, response)
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to cache files")
		}
	}

	return nil
}

func (s *BaseService) newAssembleContext() *AssembleContext {
	return &AssembleContext{
		Document:              s.document,
		ConfigurationDocument: s.configurationDocument,
		BaseDir:               s.baseDir,
		IsInterpolation:       s.isInterpolation,
	}
}

func (s *BaseService) invokePreAssembleHook(req AssembleRequest, response *AssembleResponse) error {
	return s.assembleHooks.PreAssemble(req, response, s.newAssembleContext())
}

func (s *BaseService) invokePostDesiredStateAssembleHook(req AssembleRequest, response *AssembleResponse) error {
	return s.assembleHooks.PostDesiredStateAssemble(req, response, s.newAssembleContext())
}

func (s *BaseService) invokePostConfigurationAssembleHook(req AssembleRequest, response *AssembleResponse) error {
	return s.assembleHooks.PostConfigurationAssemble(req, response, s.newAssembleContext())
}

func (s *BaseService) invokePostAssembleHook(req AssembleRequest, response *AssembleResponse) error {
	return s.assembleHooks.PostAssemble(req, response, s.newAssembleContext())
}

func (s *BaseService) invokePostCacheHook(req AssembleRequest, response *AssembleResponse) error {
	ctx := &CacheContext{
		Document:              s.document,
		ConfigurationDocument: s.configurationDocument,
		CacheDirectory:        req.CacheDirectory,
		OutputFormat:          req.OutputFormat,
	}

	return s.assembleHooks.PostCache(req, response, ctx)
}

func (s *BaseService) filterDesiredStateConfigurationRefsByEnvironment(environment string) error {
	if environment == "" {
		return nil
	}

	if s.document.GetContent() != nil {
		content := s.document.GetContent().Data
		if content != nil {
			// Normalize YAML map types recursively so nested map[interface{}]interface{}
			// structures can be pruned in-place.
			normalized, ok := normalizeYAMLNode(content).(map[string]interface{})
			if ok {
				s.document.GetContent().Data = normalized
				if err := pruneDesiredStateEnvironmentRefsRecursive(normalized, environment); err != nil {
					return err
				}
			}
		}
	}

	if s.document.GetMeta() != nil {
		meta := s.document.GetMeta().Data
		if meta != nil {
			normalized, ok := normalizeYAMLNode(meta).(map[string]interface{})
			if ok {
				s.document.GetMeta().Data = normalized
				if err := pruneDesiredStateEnvironmentRefsRecursive(normalized, environment); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func pruneDesiredStateEnvironmentRefsRecursive(node interface{}, environment string) error {
	switch v := node.(type) {
	case map[string]interface{}:
		if desiredstate, ok := toStringMap(v["desiredstate"]); ok {
			if err := pruneSingleDesiredStateEnvironmentRefs(desiredstate, environment); err != nil {
				return err
			}
		}

		for _, child := range v {
			if err := pruneDesiredStateEnvironmentRefsRecursive(child, environment); err != nil {
				return err
			}
		}

	case []interface{}:
		for _, item := range v {
			if err := pruneDesiredStateEnvironmentRefsRecursive(item, environment); err != nil {
				return err
			}
		}
	}

	return nil
}

func normalizeYAMLNode(node interface{}) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		for key, child := range v {
			v[key] = normalizeYAMLNode(child)
		}
		return v

	case map[interface{}]interface{}:
		normalized := make(map[string]interface{}, len(v))
		for rawKey, child := range v {
			key, ok := rawKey.(string)
			if !ok {
				continue
			}
			normalized[key] = normalizeYAMLNode(child)
		}
		return normalized

	case []interface{}:
		for index, child := range v {
			v[index] = normalizeYAMLNode(child)
		}
		return v

	default:
		return node
	}
}

func pruneSingleDesiredStateEnvironmentRefs(desiredstate map[string]interface{}, environment string) error {
	if dsContent, ok := toStringMap(desiredstate["content"]); ok {
		if envs, ok := toStringMap(dsContent["environments"]); ok {
			selected, exists := envs[environment]
			if !exists {
				return errors.Newf(errors.ErrParam,
					"environment '%s' is not defined under desiredstate.content.environments",
					environment,
				)
			}
			dsContent["environments"] = map[string]interface{}{environment: selected}
			return nil
		}
	}

	if configs, ok := toStringMap(desiredstate["configuration"]); ok {
		selected, exists := configs[environment]
		if !exists {
			return errors.Newf(errors.ErrParam,
				"environment '%s' is not defined under desiredstate.configuration",
				environment,
			)
		}
		desiredstate["configuration"] = map[string]interface{}{environment: selected}
		return nil
	}

	return nil
}

// assembleConfigurationFromFile handles configuration assembly when a config file is provided.
func (s *BaseService) assembleConfigurationFromFile(req AssembleRequest, response *AssembleResponse) error {
	// Create a separate document for configuration loading and assembly
	configDocument := core.NewGitOpsDocument()

	// Check if we should override schema and namespace from desiredstate BEFORE loading metadata
	useDesiredstateSchemaOverride := true
	if dsContent := s.document.GetContent().Data; dsContent != nil {
		if metaSection, ok := dsContent["desiredstate"].(map[string]interface{}); ok {
			if meta, ok := metaSection["meta"].(map[string]interface{}); ok {
				if override, ok := meta["is_override_schema_in_config"].(bool); ok {
					useDesiredstateSchemaOverride = override
					logging.Debug("Found is_override_schema_in_config in desiredstate: %t", override)
				} else {
					logging.Debug("is_override_schema_in_config not found, using default: true")
				}
			}
		}
	}

	logging.Info("Configuration schema override from desiredstate: %t", useDesiredstateSchemaOverride)

	// If override is enabled, set schema and namespace from desiredstate BEFORE loading config metadata
	if useDesiredstateSchemaOverride {
		dsMetadata := s.document.GetMeta()
		if dsMetadata != nil && dsMetadata.Data != nil {
			schemaValue := dsMetadata.Data["schema"]
			namespaceValue := dsMetadata.Data["namespace"]
			logging.Info("Overriding configuration schema and namespace from desiredstate (schema: %v, namespace: %v)",
				schemaValue, namespaceValue)
			configDocument.SetSchemaAndNamespaceOverrides(schemaValue, namespaceValue)
		} else {
			logging.Warn("Cannot override configuration schema/namespace: desiredstate metadata is nil")
		}
	} else {
		logging.Info("Configuration will use its own schema and namespace (override disabled)")
	}

	// Load configuration metadata first (without parts assembly)
	logging.Debug("Loading configuration metadata file: %s", req.ConfigFile)
	err := configDocument.LoadMetadata(req.ConfigFile)
	if err != nil {
		response.ErrorMessage = fmt.Sprintf("failed to load configuration metadata %s: %v", req.ConfigFile, err)
		return errors.Wrapf(errors.ErrParse, err, "failed to load configuration metadata %s", req.ConfigFile)
	}

	// Determine schema version strategy
	configSchemaVersion := s.determineConfigSchemaVersion(response, configDocument)

	// Canonical sequence: apply wrapper-specific path then load parts.
	err = s.prepareConfigurationDocumentForWrapper(configDocument, req.Wrapper)
	if err != nil {
		response.ErrorMessage = fmt.Sprintf("failed to load and assemble config file %s: %v", req.ConfigFile, err)
		return errors.Wrapf(errors.ErrParse, err, "failed to load and assemble config file %s", req.ConfigFile)
	}

	// Get the assembled configuration content
	configContent := configDocument.GetContent().Data
	if configContent != nil {
		// Store the configuration document for later access (e.g., for backend config)
		s.configurationDocument = configDocument

		// Apply wrapper filtering to configuration content
		filteredConfigContent := s.filterConfigurationByWrapper(configContent, req.Wrapper)

		// Store the assembled config content for caching
		response.AssembledConfigurationContent = filteredConfigContent

		// Skip validation if we have a specific wrapper and parts were loaded
		if req.Wrapper == "" || req.Wrapper == "meta" {
			err = s.handler.ValidateSchema(filteredConfigContent, configSchemaVersion, schema.SchemaTypeConfigurationMeta)
			if err != nil {
				response.ErrorMessage = fmt.Sprintf("config validation failed for file %s: %v", req.ConfigFile, err)
				return errors.Wrapf(errors.ErrParse, err, "config validation failed for file %s", req.ConfigFile)
			}
			logging.Info("Configuration validation successful: %s", req.ConfigFile)
		} else {
			logging.Info("Configuration assembled successfully (wrapper-specific, validation skipped): %s", req.ConfigFile)
		}
		response.ConfigurationAssembled = true
	}

	return nil
}

// assembleConfigurationFromDesiredState handles configuration assembly when cloning from desiredstate.
func (s *BaseService) assembleConfigurationFromDesiredState(req AssembleRequest, response *AssembleResponse) error {
	logging.Info("No configuration file provided, cloning from desiredstate specification")

	// Get the desiredstate content for cloning
	dsContent := s.document.GetContent().Data
	if dsContent == nil {
		return errors.NewDesiredStateMalformedError("desiredstate content is empty, cannot clone configuration")
	}

	// Clone and load the configuration
	configDoc, err := s.cloneAndLoadConfiguration(dsContent, req.Wrapper, req.Environment, req.ConfigRepoWorkdir, s.document)
	if err != nil {
		response.ErrorMessage = fmt.Sprintf("failed to clone configuration: %v", err)
		return errors.Wrapf(errors.ErrParse, err, "failed to clone configuration from desiredstate")
	}

	configContent := configDoc.GetContent().Data
	detectedSchemaVersion := configDoc.GetSchemaVersion()

	// Keep configuration document available for wrapper-specific post-processing hooks.
	s.configurationDocument = configDoc

	// Apply wrapper filtering to cloned configuration content
	filteredConfigContent := s.filterConfigurationByWrapper(configContent, req.Wrapper)

	// Store the assembled config content for caching
	response.AssembledConfigurationContent = filteredConfigContent

	// Validate the cloned configuration
	configSchemaVersion := schema.SchemaVersion(detectedSchemaVersion)
	if string(configSchemaVersion) == "unknown" || string(configSchemaVersion) == "" {
		// Fallback to desiredstate schema version
		configSchemaVersion = schema.SchemaVersion(response.SchemaVersion)
		if string(configSchemaVersion) == "unknown" || string(configSchemaVersion) == "" {
			configSchemaVersion = schema.SchemaVersion("4.2.0")
		}
	}

	if req.Wrapper == "" || req.Wrapper == "meta" {
		err = s.handler.ValidateSchema(filteredConfigContent, configSchemaVersion, schema.SchemaTypeConfigurationMeta)
		if err != nil {
			response.ErrorMessage = fmt.Sprintf("cloned config validation failed: %v", err)
			return errors.Wrapf(errors.ErrParse, err, "cloned config validation failed")
		}
		logging.Info("Cloned configuration assembled and validated successfully against schema %s", string(configSchemaVersion))
	}

	response.ConfigurationAssembled = true
	return nil
}

// prepareConfigurationDocumentForWrapper applies wrapper-specific parts path,
// loads parts, and validates that assembled content is available.
func (s *BaseService) prepareConfigurationDocumentForWrapper(configDoc *core.GitOpsDocument, wrapper string) error {
	if configDoc == nil {
		return errors.New(errors.ErrParam, "configuration document is nil")
	}

	// Set wrapper-specific parts path before loading parts.
	configDoc.SetWrapperForConfiguration(wrapper)

	// Load parts to complete assembly.
	// For configuration documents, LoadParts() updates content for consumption while
	// preserving full metadata in meta for backend and other lookups.
	logging.Debug("Assembling configuration parts with wrapper: %s", wrapper)
	if err := configDoc.LoadParts(); err != nil {
		return err
	}

	if configDoc.GetContent() == nil || configDoc.GetContent().Data == nil {
		return errors.NewConfigurationMalformedError("assembled configuration contains no valid content")
	}

	return nil
}

// determineConfigSchemaVersion determines the schema version to use for configuration validation.
func (s *BaseService) determineConfigSchemaVersion(response *AssembleResponse, configDocument *core.GitOpsDocument) schema.SchemaVersion {
	// Add is_override_schema_in_config property if missing (default: true)
	if dsContent := s.document.GetContent().Data; dsContent != nil {
		if metaSection, ok := dsContent["desiredstate"].(map[string]interface{}); ok {
			if meta, ok := metaSection["meta"].(map[string]interface{}); ok {
				if _, exists := meta["is_override_schema_in_config"]; !exists {
					meta["is_override_schema_in_config"] = true
					logging.Debug("Added default value for desiredstate.meta.is_override_schema_in_config: true")
				}
			}
		}
	}

	// Read the is_override_schema_in_config property
	useDesiredstateSchemaOverride := true
	if dsContent := s.document.GetContent().Data; dsContent != nil {
		if metaSection, ok := dsContent["desiredstate"].(map[string]interface{}); ok {
			if meta, ok := metaSection["meta"].(map[string]interface{}); ok {
				if override, ok := meta["is_override_schema_in_config"].(bool); ok {
					useDesiredstateSchemaOverride = override
				}
			}
		}
	}
	logging.Debug("Reading desiredstate.meta.is_override_schema_in_config value: %t", useDesiredstateSchemaOverride)

	var configSchemaVersion schema.SchemaVersion
	if useDesiredstateSchemaOverride {
		configSchemaVersion = schema.SchemaVersion(response.SchemaVersion)
		logging.Info("Desiredstate schema override enabled - using version %s for configuration validation", string(configSchemaVersion))
	} else {
		detectedVersion, err := configDocument.DetectSchemaVersion(configDocument.GetMeta().Data)
		if err != nil {
			logging.Info("Configuration schema override disabled but could not detect configuration version, falling back to desiredstate version (%s): %v", response.SchemaVersion, err)
			configSchemaVersion = schema.SchemaVersion(response.SchemaVersion)
		} else {
			configSchemaVersion = detectedVersion
			logging.Info("Configuration schema override disabled - using configuration's own version: %s", string(configSchemaVersion))
		}
	}

	// Final fallback
	if string(configSchemaVersion) == "unknown" || string(configSchemaVersion) == "" {
		configSchemaVersion = schema.SchemaVersion("4.2.0")
	}

	return configSchemaVersion
}

// GetSupportedSchemaVersions returns a list of all supported schema versions.
func (s *BaseService) GetSupportedSchemaVersions() []string {
	versions := s.handler.GetSupportedSchemaVersions()
	result := make([]string, len(versions))
	for i, version := range versions {
		result[i] = string(version)
	}
	return result
}

// filterConfigurationByWrapper filters configuration content based on the target wrapper.
func (s *BaseService) filterConfigurationByWrapper(configContent map[string]interface{}, wrapper string) map[string]interface{} {
	logging.Debug("Filtering configuration content by wrapper: %s", wrapper)

	if wrapper == "" {
		logging.Debug("No wrapper specified, returning empty content")
		return make(map[string]interface{})
	}

	logging.Debug("Wrapper specified: %s, returning assembled wrapper content directly", wrapper)
	return configContent
}

// cacheAssembledFiles saves the assembled content to the filesystem.
func (s *BaseService) cacheAssembledFiles(req AssembleRequest, response *AssembleResponse) error {
	logging.Debug("Caching assembled files to directory: %s", req.CacheDirectory)

	// Create cache directory if it doesn't exist
	err := os.MkdirAll(req.CacheDirectory, 0755)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to create cache directory %s", req.CacheDirectory)
	}

	// Determine file extension based on output format
	fileExt := ".yaml"
	if req.OutputFormat == "json" {
		fileExt = ".tfvars.json"
	}

	// Cache desiredstate file
	if response.DesiredStateAssembled {
		desiredStateContent := s.document.GetContent().Data
		if desiredStateContent != nil {
			desiredStateFile, err := s.writeContentToFile(
				req.CacheDirectory,
				"desiredstate"+fileExt,
				desiredStateContent,
				req.OutputFormat,
			)
			if err != nil {
				return errors.Wrapf(errors.ErrFail, err, "failed to cache desiredstate file")
			}
			response.DesiredStateFile = desiredStateFile
			logging.Debug("Cached desiredstate to: %s", desiredStateFile)
		}
	}

	// Cache configuration file if assembled
	if response.ConfigurationAssembled && response.AssembledConfigurationContent != nil {
		configFile, err := s.writeContentToFile(
			req.CacheDirectory,
			"configuration"+fileExt,
			response.AssembledConfigurationContent,
			req.OutputFormat,
		)
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to cache configuration file")
		}
		response.ConfigurationFile = configFile
		logging.Debug("Cached configuration to: %s", configFile)
	}

	if err := s.invokePostCacheHook(req, response); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "post-cache hook failed")
	}

	logging.Debug("Successfully cached all assembled files")
	return nil
}

// writeContentToFile writes content to a file in the specified format.
func (s *BaseService) writeContentToFile(dir, filename string, content map[string]interface{}, format string) (resultPath string, err error) {
	filepath := fmt.Sprintf("%s/%s", dir, filename)

	file, createErr := os.Create(filepath)
	if createErr != nil {
		return "", errors.Wrapf(errors.ErrFail, createErr, "failed to create file %s", filepath)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if format == "json" {
		// Normalize the content to ensure all map keys are strings
		// YAML allows map[interface{}]interface{} but JSON requires map[string]interface{}
		normalizedContent := NormalizeMapForJSON(content)

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(normalizedContent)
		if err != nil {
			return "", errors.Wrapf(errors.ErrParse, err, "failed to write JSON to file %s", filepath)
		}
	} else {
		err = s.handler.ToFile(content, file)
		if err != nil {
			return "", errors.Wrapf(errors.ErrParse, err, "failed to write YAML to file %s", filepath)
		}
	}

	return filepath, nil
}

// NormalizeMapForJSON recursively converts map[interface{}]interface{} to map[string]interface{}
// This is necessary because YAML parsing can create map[interface{}]interface{} but JSON requires map[string]interface{}
func NormalizeMapForJSON(input interface{}) interface{} {
	switch v := input.(type) {
	case map[interface{}]interface{}:
		// Convert map[interface{}]interface{} to map[string]interface{}
		result := make(map[string]interface{})
		for key, value := range v {
			// Convert key to string
			keyStr := fmt.Sprint(key)
			// Recursively normalize the value
			result[keyStr] = NormalizeMapForJSON(value)
		}
		return result
	case map[string]interface{}:
		// Already correct type, but need to normalize nested values
		result := make(map[string]interface{})
		for key, value := range v {
			result[key] = NormalizeMapForJSON(value)
		}
		return result
	case []interface{}:
		// Normalize all elements in the slice
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = NormalizeMapForJSON(item)
		}
		return result
	default:
		// Primitive types (string, int, bool, etc.) are returned as-is
		return v
	}
}

// getConfigRepoLocators returns candidate property paths to the configuration repo.
func (s *BaseService) getConfigRepoLocators(wrapper, environment string, desiredStateDoc *core.GitOpsDocument) []string {
	var (
		dsMeta        map[string]interface{}
		schemaVersion string
	)
	if desiredStateDoc != nil && desiredStateDoc.GetSchemaVersion() != "" {
		if meta := desiredStateDoc.GetMeta(); meta != nil && meta.Data != nil {
			dsMeta = meta.Data
		}
		schemaVersion = desiredStateDoc.GetSchemaVersion()
	}

	return ConfigRepoLocatorsFromMeta(wrapper, environment, dsMeta, schemaVersion)
}

// cloneAndLoadConfiguration clones the configuration repository based on desiredstate content.
func (s *BaseService) cloneAndLoadConfiguration(desiredStateContent map[string]interface{}, wrapper, environment, configRepoWorkdir string, desiredStateDoc *core.GitOpsDocument) (*core.GitOpsDocument, error) {
	locators := s.getConfigRepoLocators(wrapper, environment, desiredStateDoc)
	normalized, ok := NormalizeMapForJSON(desiredStateContent).(map[string]interface{})
	if !ok {
		return nil, errors.New(errors.ErrParse, "desiredstate content has invalid structure")
	}

	h := parser.NewYAMLHandler("")
	usableLocators := make([]string, 0, len(locators))
	for _, locator := range locators {
		if _, pathErr := h.GetValue(normalized, locator); pathErr == nil {
			usableLocators = append(usableLocators, locator)
		}
	}
	if len(usableLocators) == 0 {
		return nil, errors.Newf(errors.ErrParse,
			"no configuration repository locator found for wrapper '%s' and environment '%s' (tried locators: %v)",
			wrapper,
			environment,
			locators,
		)
	}

	var (
		configDoc *core.GitOpsDocument
		err       error
	)

	for _, configRepoLocator := range usableLocators {
		logging.Debug("Trying configuration repo locator: %s", configRepoLocator)

		configDoc = core.NewGitOpsDocument()
		err = configDoc.CloneRepoAndLoad(
			normalized,
			configRepoLocator,
			configRepoWorkdir, // workspaceDir
			false,             // isAssembleParts
			"",                // refOverride
			desiredStateDoc,   // desiredStateDoc
		)
		if err == nil {
			logging.Debug("Configuration repo locator resolved: %s", configRepoLocator)
			break
		}
	}

	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err,
			"failed to clone and load configuration (tried locators: %v)", usableLocators)
	}

	logging.Debug("Successfully cloned configuration from repository")

	if err := s.prepareConfigurationDocumentForWrapper(configDoc, wrapper); err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to load and assemble configuration parts")
	}

	return configDoc, nil
}

// GetHandler returns the YAML handler for advanced operations.
// Wrappers can use this to access handler functionality.
func (s *BaseService) GetHandler() *parser.YAMLHandler {
	return s.handler
}

// GetDocument returns the GitOpsDocument for advanced operations.
// Wrappers can use this to access document functionality.
func (s *BaseService) GetDocument() *core.GitOpsDocument {
	return s.document
}

// GetConfigurationDocument returns the last loaded configuration document.
// This is set during Assemble() and contains the full configuration metadata including backends.
func (s *BaseService) GetConfigurationDocument() *core.GitOpsDocument {
	return s.configurationDocument
}
