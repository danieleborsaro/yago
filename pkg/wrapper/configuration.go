package wrapper

import (
	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// BaseConfiguration provides common configuration functionality for all wrappers.
// Specific wrappers (terraform, docker, etc.) embed this and add wrapper-specific methods.
type BaseConfiguration struct {
	document      *core.GitOpsDocument
	handler       *parser.YAMLHandler
	environment   string
	wrapper       string
	metaFile      string
	tmpFileSuffix string

	// Cached file path after Cache() is called
	cachedFile string
}

// NewBaseConfiguration creates a base configuration instance.
func NewBaseConfiguration(environment, wrapper string, envVars map[string]string) *BaseConfiguration {
	doc := core.NewGitOpsDocument()

	// Set environment variables
	if envVars != nil {
		doc.SetEnvironmentVariables(envVars)
	}

	// Set cache filename base (extension will be added by Cache method based on format)
	doc.SetCacheFilename("configuration.tfvars")

	return &BaseConfiguration{
		document:      doc,
		environment:   environment,
		wrapper:       wrapper,
		tmpFileSuffix: "configuration.yaml",
	}
}

// LoadGitOpsFile loads the configuration from file.
func (cfg *BaseConfiguration) LoadGitOpsFile(schemaVersionOverride string) error {
	if cfg.metaFile == "" {
		return errors.NewParamError("meta file path not set - call SetMetaFile() first")
	}

	logging.Debug("Loading configuration from: %s", cfg.metaFile)

	// Load metadata and ecosystem WITHOUT assembling parts yet.
	// Parts must be loaded AFTER SetWrapperForConfiguration so the correct wrapper-specific
	// parts path is used (e.g., configuration.content.wrappers.terraform.parts).
	// TODO: Wrapper layer doesn't have access to desiredstate document, so passing nil
	// This means wrappers can't use the override feature currently
	if err := cfg.document.LoadGitOpsFile(cfg.metaFile, false, nil); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load configuration")
	}

	// Set wrapper-specific parts path before loading parts.
	// For wrapper="terraform" this changes propertyPartsToLoad from "meta.parts" to
	// "configuration.content.wrappers.terraform.parts", ensuring the actual tfvars
	// part files (global, env, proj, eco, secrets, etc.) are loaded and merged.
	cfg.document.SetWrapperForConfiguration(cfg.wrapper)

	// Now load and assemble parts with the correct parts path.
	if err := cfg.document.LoadParts(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load configuration parts")
	}

	// Apply schema override manually if provided
	if schemaVersionOverride != "" {
		cfg.document.SetSchema(schema.SchemaVersion(schemaVersionOverride))
	}

	// Update schema version after loading
	if schemaVersionOverride != "" {
		cfg.document.SetSchema(schema.SchemaVersion(schemaVersionOverride))
	} else {
		cfg.document.SetSchema(schema.SchemaVersion(cfg.document.GetSchemaVersion()))
	}

	logging.Info("Loaded configuration for wrapper '%s' in environment '%s'",
		cfg.wrapper, cfg.environment)

	return nil
}

// SetMetaFile sets the path to the configuration meta file.
func (cfg *BaseConfiguration) SetMetaFile(path string) {
	cfg.metaFile = path
}

// CloneRepoAndLoad clones a configuration repository and loads it.
func (cfg *BaseConfiguration) CloneRepoAndLoad(req CloneRequest) error {
	logging.Debug("Cloning configuration repository...")

	// Clone/load metadata only first; wrapper-specific parts must be loaded using
	// SetWrapperForConfiguration + LoadParts (same flow as LoadGitOpsFile).
	// TODO: Wrapper layer doesn't have access to desiredstate document, so passing nil.
	err := cfg.document.CloneRepoAndLoad(
		req.DesiredStateContent,
		req.MetaRepoLocatorPath,
		req.CloneDir,
		false, // isAssembleParts
		"",    // refOverride (empty = use desiredstate value)
		nil,   // desiredStateDoc - wrapper doesn't have access to it
	)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to clone and load configuration")
	}

	cfg.document.SetWrapperForConfiguration(cfg.wrapper)
	if err := cfg.document.LoadParts(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load configuration parts")
	}

	logging.Info("Cloned and loaded configuration for wrapper '%s'", cfg.wrapper)

	return nil
}

// GetDocument returns the underlying GitOpsDocument.
func (cfg *BaseConfiguration) GetDocument() *core.GitOpsDocument {
	return cfg.document
}

// GetEnvironment returns the environment name.
func (cfg *BaseConfiguration) GetEnvironment() string {
	return cfg.environment
}

// GetWrapper returns the wrapper type.
func (cfg *BaseConfiguration) GetWrapper() string {
	return cfg.wrapper
}

// GetSchemaVersion returns the API version.
func (cfg *BaseConfiguration) GetSchemaVersion() string {
	return cfg.document.GetSchemaVersion()
}

// Validate validates the configuration document.
// Validation is performed automatically during LoadGitOpsFile, so this is a no-op.
func (cfg *BaseConfiguration) Validate() error {
	// Validation already done in LoadGitOpsFile
	return nil
}

// Cache saves the configuration to a file.
func (cfg *BaseConfiguration) Cache(buildDir, format string) (string, error) {
	isJSON := (format == "json")

	filePath, err := cfg.document.Cache(buildDir, isJSON)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to cache configuration")
	}

	cfg.cachedFile = filePath
	logging.Debug("Cached configuration to: %s", filePath)

	return filePath, nil
}

// GetCachedFile returns the path to the cached file.
func (cfg *BaseConfiguration) GetCachedFile() string {
	return cfg.cachedFile
}

// GetMetaFile returns the path to the original meta file.
func (cfg *BaseConfiguration) GetMetaFile() string {
	return cfg.metaFile
}
