package concourse

import (
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"github.com/spf13/cobra"
)

// ConcourseFactory implements the WrapperFactory interface for the concourse wrapper.
// This allows concourse to be auto-discovered and registered with the wrapper registry.
type ConcourseFactory struct{}

// Name returns the unique name of this wrapper.
func (f *ConcourseFactory) Name() string {
	return "concourse"
}

// CreateParser creates a new concourse parser instance.
func (f *ConcourseFactory) CreateParser(env string, envVars map[string]string) wrapper.Parser {
	return NewParser(env, envVars)
}

// CreateService creates a new concourse service instance.
func (f *ConcourseFactory) CreateService(baseDir string, enableInterpolation bool) wrapper.Service {
	return NewService(baseDir, enableInterpolation)
}

// CreateCLICommand creates the Cobra command tree for concourse operations.
func (f *ConcourseFactory) CreateCLICommand() *cobra.Command {
	return NewConcourseCommand()
}

// init registers the concourse factory with the global wrapper registry.
// This is called automatically when the package is imported.
func init() {
	wrapper.Register(&ConcourseFactory{})
}
