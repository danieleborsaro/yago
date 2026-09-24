package terraform

import (
	"fmt"
	"path/filepath"

	yamlparser "github.com/danieleborsaro/yago/internal/parser"
	gitrepo "github.com/danieleborsaro/yago/internal/repo"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

const (
	defaultTerraformSourceCodeRepo = "desiredstate.content.components.sourcecode.terraform"
	defaultTerraformYagoRepo       = "desiredstate.components.yago.git"
)

// Parser extends BaseParser for terraform-specific parsing operations.
type Parser struct {
	*wrapper.BaseParser

	// Terraform-specific fields
	terraformVersion   string
	backendManager     *BackendManager
	sourceCodeDir      string
	isClonedSourceCode bool
	configWorkdir      string
	desiredStatePath   string
}

// NewParser creates a new terraform parser instance.
func NewParser(env string, envVars map[string]string) *Parser {
	baseParser := wrapper.NewBaseParser(env, envVars)
	return &Parser{
		BaseParser:       baseParser,
		terraformVersion: DefaultTerraformVersion,
		backendManager:   NewBackendManager(baseParser.GetWorkspaceDir()),
	}
}

// LoadGitOpsFiles loads and parses terraform-specific GitOps files.
func (p *Parser) LoadGitOpsFiles(req wrapper.LoadRequest) error {
	logging.Debug("Loading terraform GitOps files")

	// Store desiredstate path for later use
	if req.GetDesiredStateRoot() != "" {
		p.desiredStatePath = req.GetDesiredStateRoot()
	}

	// First delegate to base implementation
	if err := p.BaseParser.LoadGitOpsFiles(req); err != nil {
		return err
	}

	// Load terraform-specific version from desiredstate if available
	if err := p.loadTerraformVersion(); err != nil {
		logging.Warn("Could not load terraform version from desiredstate, using default: %s", DefaultTerraformVersion)
	}

	logging.Info("Terraform version set to: %s", p.terraformVersion)

	return nil
}

// LoadGitOpsFilesExtended loads GitOps files with all terraform-specific parameters.
// Supports:
// - desiredStateRoot: root path for desiredstate files
// - awsRegion: AWS region for backend config
// - configRoot: root path for configuration files
// - terraformSourceCodeDir: optional directory for terraform source code
func (p *Parser) LoadGitOpsFilesExtended(
	desiredStateRoot string,
	awsRegion string,
	configRoot string,
	terraformSourceCodeDir string,
) error {
	logging.Debug("Loading terraform GitOps files (extended) - desiredstate: %s, region: %s, config: %s, sourcecode: %s",
		desiredStateRoot, awsRegion, configRoot, terraformSourceCodeDir)

	// Store desiredstate path
	p.desiredStatePath = desiredStateRoot

	// Load desiredstate
	ds := wrapper.NewBaseDesiredState(p.GetEnvironment(), "terraform", p.GetEnvVariables())
	if err := ds.LoadGitOpsFile(desiredStateRoot); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate from %s", desiredStateRoot)
	}
	p.SetDesiredState(ds)

	// Load configuration if provided
	if configRoot != "" {
		cfg := wrapper.NewBaseConfiguration(p.GetEnvironment(), "terraform", p.GetEnvVariables())
		cfg.SetMetaFile(configRoot)

		// Get schema version from desiredstate for override
		dsDoc := ds.GetDocument()
		schemaVersionOverride := ""
		namespaceOverride := ""
		if dsDoc != nil && dsDoc.GetMeta() != nil && dsDoc.GetMeta().Data != nil {
			if schema, ok := dsDoc.GetMeta().Data["schema"].(string); ok {
				schemaVersionOverride = schema
			}
			if namespace, ok := dsDoc.GetMeta().Data["namespace"].(string); ok {
				namespaceOverride = namespace
			}
		}

		// Load configuration with schema override
		if err := cfg.LoadGitOpsFile(schemaVersionOverride); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to load configuration from %s", configRoot)
		}

		// Apply namespace override if provided
		if namespaceOverride != "" {
			cfgDoc := cfg.GetDocument()
			if cfgDoc != nil {
				cfgDoc.SetSchemaAndNamespaceOverrides(schemaVersionOverride, namespaceOverride)
			}
		}

		p.SetConfiguration(cfg)
		logging.Info("Loaded configuration with schema override: %s, namespace override: %s", schemaVersionOverride, namespaceOverride)
	} else {
		dsDoc := ds.GetDocument()
		dsMeta := map[string]interface{}{}
		if dsDoc.GetMeta() != nil && dsDoc.GetMeta().Data != nil {
			dsMeta = dsDoc.GetMeta().Data
		}
		locators, err := wrapper.UsableConfigRepoLocators("terraform", p.GetEnvironment(), dsDoc.GetContent().Data, dsMeta, dsDoc.GetSchemaVersion())
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to resolve configuration repo locator from desiredstate")
		}

		var cfg *wrapper.BaseConfiguration
		for _, locator := range locators {
			cfg = wrapper.NewBaseConfiguration(p.GetEnvironment(), "terraform", p.GetEnvVariables())
			err = cfg.CloneRepoAndLoad(wrapper.CloneRequest{
				CloneDir:              p.configWorkdir,
				MetaRepoLocatorPath:   locator,
				DesiredStateContent:   dsDoc.GetContent().Data,
				SchemaVersionOverride: ds.GetSchemaVersion(),
			})
			if err == nil {
				break
			}
			logging.Debug("Configuration repo locator %s did not resolve: %v", locator, err)
		}
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to clone and load configuration from desiredstate")
		}
		p.SetConfiguration(cfg)
		logging.Info("Cloned and loaded configuration from desiredstate")
	}

	if err := p.loadBackendForRegion(awsRegion); err != nil {
		return err
	}

	if err := p.loadTerraformSourceCode(terraformSourceCodeDir); err != nil {
		return err
	}

	return nil
}

func (p *Parser) loadBackendForRegion(awsRegion string) error {
	if awsRegion == "" {
		return nil
	}

	cfg := p.GetConfiguration()
	if cfg == nil {
		return nil
	}

	cfgDoc := cfg.GetDocument()
	if cfgDoc == nil {
		return nil
	}

	cfgMeta := cfgDoc.GetMeta()
	if cfgMeta == nil || cfgMeta.Data == nil {
		return nil
	}

	backendPath := p.extractBackendPath(cfgMeta.Data, cfgDoc.GetSchemaVersion(), awsRegion)
	if backendPath == "" {
		logging.Warn("No backend configuration found for region: %s", awsRegion)
		return nil
	}

	configWorkdir := cfgDoc.GetWorkdir()
	absBackendPath := filepath.Join(configWorkdir, backendPath)
	logging.Debug("Resolved absolute backend path: %s (workdir: %s)", absBackendPath, configWorkdir)

	if err := p.LoadBackendConfig(absBackendPath, awsRegion); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load backend configuration for region %s", awsRegion)
	}

	return nil
}

func (p *Parser) loadTerraformSourceCode(terraformSourceCodeDir string) error {
	if terraformSourceCodeDir != "" {
		absSourceDir, err := filepath.Abs(terraformSourceCodeDir)
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to resolve terraform source workdir %s", terraformSourceCodeDir)
		}
		p.sourceCodeDir = absSourceDir
		p.isClonedSourceCode = false
		logging.Info("Using provided terraform source workdir: %s", p.sourceCodeDir)
		return nil
	}

	ds := p.GetDesiredState()
	if ds == nil || ds.GetDocument() == nil || ds.GetDocument().GetContent() == nil || ds.GetDocument().GetContent().Data == nil {
		return errors.New(errors.ErrDesiredStateMissing, "desiredstate content not loaded for terraform source code resolution")
	}

	dsDoc := ds.GetDocument()
	metaData := map[string]interface{}{}
	if dsDoc.GetMeta() != nil && dsDoc.GetMeta().Data != nil {
		metaData = dsDoc.GetMeta().Data
	}

	candidates := []string{}
	if schemaLocator, err := wrapper.DiscoverPropertyPath(metaData, dsDoc.GetSchemaVersion(), true, "terraformSourceCodeRepo"); err == nil && schemaLocator != "" {
		candidates = append(candidates, schemaLocator)
	}
	candidates = append(candidates, defaultTerraformSourceCodeRepo, defaultTerraformYagoRepo)

	var lastErr error
	for _, locator := range candidates {
		repo, err := gitrepo.NewRepoFromDesiredState(dsDoc.GetContent().Data, locator, "", "", nil)
		if err != nil {
			lastErr = err
			continue
		}

		resolvedDir := repo.GetFilePath()
		if resolvedDir == "" {
			lastErr = fmt.Errorf("empty repo path for locator %s", locator)
			continue
		}

		p.sourceCodeDir = resolvedDir
		p.isClonedSourceCode = true
		logging.Info("Resolved terraform source code from desiredstate locator '%s': %s", locator, resolvedDir)
		return nil
	}

	if lastErr != nil {
		return errors.Wrapf(errors.ErrParse, lastErr, "failed to resolve terraform source code repository from desiredstate")
	}

	return errors.New(errors.ErrParse, "failed to resolve terraform source code repository from desiredstate")
}

// GetTerraformVersion returns the terraform version from desiredstate or default.
func (p *Parser) GetTerraformVersion() string {
	return p.terraformVersion
}

// loadTerraformVersion loads terraform version from the desiredstate file.
func (p *Parser) loadTerraformVersion() error {
	ds := p.GetDesiredState()
	if ds == nil {
		return errors.New(errors.ErrDesiredStateMissing, "desiredstate not loaded")
	}

	dsDoc := ds.GetDocument()
	if dsDoc == nil || dsDoc.GetContent() == nil || dsDoc.GetContent().Data == nil {
		return errors.New(errors.ErrDesiredStateMalformed, "desiredstate content not available")
	}

	metaData := map[string]interface{}{}
	if dsDoc.GetMeta() != nil && dsDoc.GetMeta().Data != nil {
		metaData = dsDoc.GetMeta().Data
	}

	terraformVersionPath, err := wrapper.DiscoverPropertyPath(metaData, dsDoc.GetSchemaVersion(), true, "terraformVersion")
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to resolve property path 'terraformVersion' from x-gitops-paths")
	}

	versionValue, err := dsDoc.GetContent().GetValue(terraformVersionPath)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "terraform version not found at schema path '%s'", terraformVersionPath)
	}

	version, ok := versionValue.(string)
	if !ok || version == "" {
		return errors.Newf(errors.ErrParse, "terraform version at schema path '%s' is not a non-empty string", terraformVersionPath)
	}

	p.terraformVersion = version
	logging.Debug("Loaded terraform version from desiredstate via x-gitops-paths (%s): %s", terraformVersionPath, version)
	return nil

}

// extractBackendPath extracts the backend file path from configuration metadata.
// Uses schema-defined x-gitops-paths.configuration.wrappers to avoid hardcoded paths.
func (p *Parser) extractBackendPath(configMeta map[string]interface{}, schemaVersion string, awsRegion string) string {
	wrappersPath, err := wrapper.DiscoverPropertyPath(configMeta, schemaVersion, false, "wrappers")
	if err != nil {
		logging.Warn("extractBackendPath: unable to resolve 'wrappers' path from x-gitops-paths: %v", err)
		return ""
	}

	backendLocator := wrappersPath + ".terraform.backends." + awsRegion
	handler := yamlparser.NewYAMLHandler("/")

	backendValue, err := handler.GetValue(configMeta, backendLocator)
	if err != nil {
		logging.Warn("extractBackendPath: backend path not found at '%s': %v", backendLocator, err)
		return ""
	}

	backendPath, ok := backendValue.(string)
	if !ok || backendPath == "" {
		logging.Warn("extractBackendPath: value at '%s' is not a non-empty string", backendLocator)
		return ""
	}

	return backendPath
}

// LoadBackendConfig loads backend configuration for the specified region.
func (p *Parser) LoadBackendConfig(schemaPath string, awsRegion string) error {
	return p.backendManager.LoadBackendConfig(schemaPath, awsRegion)
}

// CacheBackendConfig generates the backend.tfvars.json file.
func (p *Parser) CacheBackendConfig(buildDirectory string, generateJSON bool) error {
	return p.backendManager.Cache(buildDirectory, generateJSON)
}

// GetBackendFilePath returns the path to the cached backend configuration file.
func (p *Parser) GetBackendFilePath() string {
	return p.backendManager.GetBackendFilePath()
}

// GetBackendConfig returns the loaded backend configuration.
func (p *Parser) GetBackendConfig() *BackendConfig {
	return p.backendManager.GetConfig()
}

// IsClonedSourceCode returns whether terraform source code was cloned from a repo.
func (p *Parser) IsClonedSourceCode() bool {
	return p.isClonedSourceCode
}

// GetSourceCodeDir returns the terraform source code directory if set.
func (p *Parser) GetSourceCodeDir() string {
	return p.sourceCodeDir
}

// SetConfigurationWorkdir sets the optional workdir to use when configuration is cloned from desiredstate.
func (p *Parser) SetConfigurationWorkdir(workdir string) {
	p.configWorkdir = workdir
}
