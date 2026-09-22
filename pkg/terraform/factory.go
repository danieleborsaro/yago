package terraform

import (
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"github.com/spf13/cobra"
)

// TerraformFactory implements the WrapperFactory interface for the terraform wrapper.
// This allows terraform to be auto-discovered and registered with the wrapper registry.
type TerraformFactory struct{}

// Name returns the unique name of this wrapper.
func (f *TerraformFactory) Name() string {
	return "terraform"
}

// CreateParser creates a new terraform parser instance.
func (f *TerraformFactory) CreateParser(env string, envVars map[string]string) wrapper.Parser {
	return NewParser(env, envVars)
}

// CreateService creates a new terraform service instance.
func (f *TerraformFactory) CreateService(baseDir string, enableInterpolation bool) wrapper.Service {
	return NewService(baseDir, enableInterpolation)
}

// CreateCLICommand creates the Cobra command tree for terraform operations.
func (f *TerraformFactory) CreateCLICommand() *cobra.Command {
	return NewTerraformCommand()
}

// init registers the terraform factory with the global wrapper registry.
// This is called automatically when the package is imported.
func init() {
	wrapper.Register(&TerraformFactory{})
}
