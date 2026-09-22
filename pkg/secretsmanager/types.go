package secretsmanager

import (
	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// SecretManagerLoadRequest extends the base LoadRequest for secretsmanager-specific operations.
type SecretManagerLoadRequest struct {
	Environment         string
	DesiredStateRoot    string
	ConfigRoot          string
	EnableInterpolation bool
}

// GetEnvironment returns the environment name.
func (r *SecretManagerLoadRequest) GetEnvironment() string {
	return r.Environment
}

// GetDesiredStateRoot returns the root path for desired state files.
func (r *SecretManagerLoadRequest) GetDesiredStateRoot() string {
	return r.DesiredStateRoot
}

// GetConfigRoot returns the root path for configuration files.
func (r *SecretManagerLoadRequest) GetConfigRoot() string {
	return r.ConfigRoot
}

// SecretManagerConfiguration wraps the base Configuration interface with secretsmanager-specific methods.
type SecretManagerConfiguration struct {
	baseConfig wrapper.Configuration
	document   *core.GitOpsDocument
}

// GetDocument returns the underlying GitOps document.
func (c *SecretManagerConfiguration) GetDocument() *core.GitOpsDocument {
	return c.document
}

// LoadGitOpsFile loads a secretsmanager configuration file.
func (c *SecretManagerConfiguration) LoadGitOpsFile(schemaVersionOverride string) error {
	return c.baseConfig.LoadGitOpsFile(schemaVersionOverride)
}

// CloneRepoAndLoad clones a repository and loads the configuration.
func (c *SecretManagerConfiguration) CloneRepoAndLoad(req wrapper.CloneRequest) error {
	return c.baseConfig.CloneRepoAndLoad(req)
}

// Validate validates the secretsmanager configuration.
func (c *SecretManagerConfiguration) Validate() error {
	return c.baseConfig.Validate()
}

// Cache caches the secretsmanager configuration.
func (c *SecretManagerConfiguration) Cache(buildDir string, format string) (string, error) {
	return c.baseConfig.Cache(buildDir, format)
}

// GetCachedFile returns the path to the cached configuration file.
func (c *SecretManagerConfiguration) GetCachedFile() string {
	return c.baseConfig.GetCachedFile()
}

// SecretManagerDesiredState wraps the base DesiredState interface with secretsmanager-specific methods.
type SecretManagerDesiredState struct {
	baseState wrapper.DesiredState
	document  *core.GitOpsDocument
}

// GetDocument returns the underlying GitOps document.
func (ds *SecretManagerDesiredState) GetDocument() *core.GitOpsDocument {
	return ds.document
}

// LoadGitOpsFile loads a secretsmanager desired state file.
func (ds *SecretManagerDesiredState) LoadGitOpsFile(pathToRoot string) error {
	return ds.baseState.LoadGitOpsFile(pathToRoot)
}

// GetEnvironment returns the environment name.
func (ds *SecretManagerDesiredState) GetEnvironment() string {
	return ds.baseState.GetEnvironment()
}

// GetWrapper returns the wrapper name.
func (ds *SecretManagerDesiredState) GetWrapper() string {
	return ds.baseState.GetWrapper()
}

// GetSchema returns the schema manager.
func (ds *SecretManagerDesiredState) GetSchema() *schema.SchemaManager {
	return ds.baseState.GetSchema()
}

// GetSchemaVersion returns the API version.
func (ds *SecretManagerDesiredState) GetSchemaVersion() string {
	return ds.baseState.GetSchemaVersion()
}

// Validate validates the secretsmanager desired state.
func (ds *SecretManagerDesiredState) Validate() error {
	return ds.baseState.Validate()
}

// Cache caches the secretsmanager desired state.
func (ds *SecretManagerDesiredState) Cache(buildDir string, format string) (string, error) {
	return ds.baseState.Cache(buildDir, format)
}

// GetCachedFile returns the path to the cached desired state file.
func (ds *SecretManagerDesiredState) GetCachedFile() string {
	return ds.baseState.GetCachedFile()
}
