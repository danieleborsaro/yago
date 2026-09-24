package secretsmanager

import (
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"github.com/spf13/cobra"
)

// SecretManagerFactory implements the WrapperFactory interface for the secretsmanager wrapper.
// This allows secretsmanager to be auto-discovered and registered with the wrapper registry.
type SecretManagerFactory struct{}

// Name returns the unique name of this wrapper.
func (f *SecretManagerFactory) Name() string {
	return "secretsmanager"
}

// CreateParser creates a new secretsmanager parser instance.
func (f *SecretManagerFactory) CreateParser(env string, envVars map[string]string) wrapper.Parser {
	return wrapper.NewBaseParser(env, envVars)
}

// CreateService creates a new secretsmanager service instance.
func (f *SecretManagerFactory) CreateService(baseDir string, enableInterpolation bool) wrapper.Service {
	return NewService(baseDir, enableInterpolation)
}

// CreateCLICommand creates the Cobra command tree for secretsmanager operations.
func (f *SecretManagerFactory) CreateCLICommand() *cobra.Command {
	return NewSecretManagerCommand()
}

// init registers the secretsmanager factory with the global wrapper registry.
// This is called automatically when the package is imported.
func init() {
	wrapper.Register(&SecretManagerFactory{})
}
