package wrapper

import (
	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/schema"
)

// Parser defines the interface for all wrapper parsers.
// This is the contract that terraform, docker, github, etc. must implement.
// Each wrapper embeds BaseParser and can override methods as needed.
type Parser interface {
	// Core document access
	GetDesiredState() DesiredState
	GetConfiguration() Configuration
	GetEnvironment() string
	GetEnvVariables() map[string]string

	// Lifecycle methods
	LoadGitOpsFiles(req LoadRequest) error
	Cache(req CacheRequest) (*CacheResponse, error)
	Clear() error

	// Workspace management
	SetWorkspace(dir string) error
	GetWorkspaceDir() string
	GetBuildDir() string
}

// DesiredState defines the interface for desiredstate documents.
// Each wrapper can extend the base implementation with wrapper-specific methods.
type DesiredState interface {
	// Core GitOps document behavior
	LoadGitOpsFile(pathToRoot string) error
	GetDocument() *core.GitOpsDocument
	GetEnvironment() string
	GetWrapper() string

	// Schema access
	GetSchema() *schema.SchemaManager
	GetSchemaVersion() string

	// Validation
	Validate() error

	// Caching
	Cache(buildDir string, format string) (string, error)
	GetCachedFile() string
}

// Configuration defines the interface for configuration documents.
// Each wrapper can extend the base implementation with wrapper-specific methods.
type Configuration interface {
	// Core loading
	LoadGitOpsFile(schemaVersionOverride string) error
	CloneRepoAndLoad(req CloneRequest) error
	GetDocument() *core.GitOpsDocument

	// Validation
	Validate() error

	// Caching
	Cache(buildDir string, format string) (string, error)
	GetCachedFile() string
}

// LoadRequest is the interface that all wrapper-specific load requests must implement.
// Each wrapper can define its own concrete request type with additional fields.
type LoadRequest interface {
	GetEnvironment() string
	GetDesiredStateRoot() string
	GetConfigRoot() string
}

// CloneRequest contains parameters for cloning configuration repository.
type CloneRequest struct {
	CloneDir              string
	DesiredStateContent   map[string]interface{}
	SchemaVersionOverride string
	NamespaceOverride     string
	MetaRepoLocatorPath   string
}

// CacheRequest contains parameters for caching assembled content.
type CacheRequest struct {
	BuildDirectory string
	GenerateJSON   bool
	Documents      []interface{} // Can be DesiredState or Configuration
}

// CacheResponse contains results of caching operation.
type CacheResponse struct {
	BuildDir          string
	DesiredStateFile  string
	ConfigurationFile string
	AdditionalFiles   map[string]string // For wrapper-specific files (e.g., terraform backend)
}

// PromoteRequest contains parameters for promoting content across environments.
type PromoteRequest struct {
	SourceEnvironment string
	TargetEnvironment string
	IsDryRun          bool
}

// PromoteResponse contains results of promote operation.
type PromoteResponse struct {
	Success        bool
	SourceFile     string
	TargetFile     string
	ChangesApplied []string
	ErrorMessage   string
}
