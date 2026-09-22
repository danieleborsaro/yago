package wrapper

import (
	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// BaseDesiredState provides common desiredstate functionality for all wrappers.
// Specific wrappers (terraform, docker, etc.) embed this and add wrapper-specific methods.
type BaseDesiredState struct {
	document      *core.GitOpsDocument
	handler       *parser.YAMLHandler
	environment   string
	wrapper       string
	metaFile      string
	tmpFileSuffix string

	// Cached file path after Cache() is called
	cachedFile string
}

// NewBaseDesiredState creates a base desiredstate instance.
func NewBaseDesiredState(environment, wrapper string, envVars map[string]string) *BaseDesiredState {
	doc := core.NewGitOpsDocument()

	// Set environment variables
	if envVars != nil {
		doc.SetEnvironmentVariables(envVars)
	}

	// Set cache filename base (extension will be added by Cache method based on format)
	doc.SetCacheFilename("desiredstate.tfvars")

	return &BaseDesiredState{
		document:      doc,
		environment:   environment,
		wrapper:       wrapper,
		tmpFileSuffix: "desiredstate.yaml",
	}
}

// LoadGitOpsFile loads the desiredstate from file.
func (ds *BaseDesiredState) LoadGitOpsFile(pathToRoot string) error {
	logging.Debug("Loading desiredstate from: %s", pathToRoot)

	ds.metaFile = pathToRoot

	// Load using GitOpsDocument's LoadGitOpsFile method
	// isAssembleParts=true for desiredstate to assemble all parts
	// desiredStateDoc=nil since desiredstate files don't have overrides from another desiredstate
	if err := ds.document.LoadGitOpsFile(pathToRoot, true, nil); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to load desiredstate")
	}

	// Update schema version after loading
	ds.document.SetSchema(schema.SchemaVersion(ds.document.GetSchemaVersion()))

	// Keep only the selected environment configuration refs in assembled desiredstate.
	if err := ds.filterConfigurationRefsByEnvironment(); err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to filter desiredstate configuration refs")
	}

	// Note: The current implementation doesn't have GetDefaultEnvironmentName() or ValidateWrapper()
	// These would need to be added to SchemaManager if needed
	// For now, we'll keep it simple and match what the service.go does

	if ds.wrapper == "meta" || ds.wrapper == "" {
		logging.Debug("Wrapper not specified or set to meta, loading metadata only")
	} else {
		logging.Info("Loading desiredstate for wrapper '%s' in environment '%s'",
			ds.wrapper, ds.environment)
	}

	return nil
}

func (ds *BaseDesiredState) filterConfigurationRefsByEnvironment() error {
	if ds.environment == "" {
		return nil
	}

	if ds.document.GetContent() != nil {
		content := ds.document.GetContent().Data
		if content != nil {
			// Normalize nested YAML maps so pruning can mutate all nested desiredstates,
			// including ecosystem-loaded sub-documents.
			normalized, ok := normalizeYAMLNode(content).(map[string]interface{})
			if ok {
				ds.document.GetContent().Data = normalized
				if err := pruneDesiredStateEnvironmentRefsRecursive(normalized, ds.environment); err != nil {
					return err
				}
			}
		}
	}

	if ds.document.GetMeta() != nil {
		meta := ds.document.GetMeta().Data
		if meta != nil {
			normalized, ok := normalizeYAMLNode(meta).(map[string]interface{})
			if ok {
				ds.document.GetMeta().Data = normalized
				if err := pruneDesiredStateEnvironmentRefsRecursive(normalized, ds.environment); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func toStringMap(value interface{}) (map[string]interface{}, bool) {
	switch m := value.(type) {
	case map[string]interface{}:
		return m, true
	case map[interface{}]interface{}:
		converted := make(map[string]interface{}, len(m))
		for key, val := range m {
			keyStr, ok := key.(string)
			if !ok {
				continue
			}
			converted[keyStr] = val
		}
		return converted, true
	default:
		return nil, false
	}
}

// GetDocument returns the underlying GitOpsDocument.
func (ds *BaseDesiredState) GetDocument() *core.GitOpsDocument {
	return ds.document
}

// GetEnvironment returns the environment name.
func (ds *BaseDesiredState) GetEnvironment() string {
	return ds.environment
}

// GetWrapper returns the wrapper type.
func (ds *BaseDesiredState) GetWrapper() string {
	return ds.wrapper
}

// GetSchema returns the schema manager instance.
// Note: schemaManager is private in GitOpsDocument, so we return nil for now.
// Wrappers should use document methods directly if they need schema operations.
func (ds *BaseDesiredState) GetSchema() *schema.SchemaManager {
	// TODO: Add GetSchemaManager() method to GitOpsDocument if needed
	return nil
}

// GetSchemaVersion returns the API version.
func (ds *BaseDesiredState) GetSchemaVersion() string {
	return ds.document.GetSchemaVersion()
}

// Validate validates the desiredstate document.
// Validation is performed automatically during LoadGitOpsFile, so this is a no-op.
func (ds *BaseDesiredState) Validate() error {
	// Validation already done in LoadGitOpsFile
	return nil
}

// Cache saves the desiredstate to a file.
func (ds *BaseDesiredState) Cache(buildDir, format string) (string, error) {
	isJSON := (format == "json")

	filePath, err := ds.document.Cache(buildDir, isJSON)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to cache desiredstate")
	}

	ds.cachedFile = filePath
	logging.Debug("Cached desiredstate to: %s", filePath)

	return filePath, nil
}

// GetCachedFile returns the path to the cached file.
func (ds *BaseDesiredState) GetCachedFile() string {
	return ds.cachedFile
}

// GetMetaFile returns the path to the original meta file.
func (ds *BaseDesiredState) GetMetaFile() string {
	return ds.metaFile
}
