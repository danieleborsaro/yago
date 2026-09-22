package desiredstate

import (
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"github.com/spf13/cobra"
)

// DesiredStateFactory implements the WrapperFactory interface for the desiredstate wrapper.
// This allows desiredstate to be auto-discovered and registered with the wrapper registry.
type DesiredStateFactory struct{}

// Name returns the unique name of this wrapper.
func (f *DesiredStateFactory) Name() string {
	return "desiredstate"
}

// CreateParser creates a new desiredstate parser instance.
func (f *DesiredStateFactory) CreateParser(env string, envVars map[string]string) wrapper.Parser {
	return NewParser(env, envVars)
}

// CreateService creates a new desiredstate service instance.
func (f *DesiredStateFactory) CreateService(baseDir string, enableInterpolation bool) wrapper.Service {
	return NewService(baseDir, enableInterpolation)
}

// CreateCLICommand creates the Cobra command tree for desiredstate operations.
func (f *DesiredStateFactory) CreateCLICommand() *cobra.Command {
	return NewDesiredStateCommand()
}

// init registers the desiredstate factory with the global wrapper registry.
// This is called automatically when the package is imported.
func init() {
	wrapper.Register(&DesiredStateFactory{})
}
