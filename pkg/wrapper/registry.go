package wrapper

import (
	"fmt"
	"sort"
	"sync"

	"github.com/spf13/cobra"
)

// WrapperFactory is the interface that all wrappers must implement to be discoverable.
// This enables the plugin architecture where wrappers can auto-register and be discovered at runtime.
type WrapperFactory interface {
	// Name returns the unique name of this wrapper (e.g., "desiredstate", "terraform", "kubernetes")
	Name() string

	// CreateParser creates a new parser instance for this wrapper
	// env: environment name (e.g., "dev", "prod")
	// envVars: environment variables to use for interpolation
	CreateParser(env string, envVars map[string]string) Parser

	// CreateService creates a new service instance for this wrapper
	// baseDir: base directory for file operations
	// enableInterpolation: whether to enable variable interpolation
	CreateService(baseDir string, enableInterpolation bool) Service

	// CreateCLICommand creates the Cobra command tree for this wrapper
	// This allows the wrapper to define its own CLI subcommands
	CreateCLICommand() *cobra.Command
}

// Service is the interface that all wrapper services must implement.
// This defines the common operations available across all wrappers.
type Service interface {
	// Validate validates a GitOps file and optionally a configuration file
	Validate(req ValidateRequest) (*ValidateResponse, error)

	// Assemble assembles a GitOps file with parts and optionally caches it
	Assemble(req AssembleRequest) (*AssembleResponse, error)

	// GetSupportedSchemaVersions returns a list of all supported schema versions
	GetSupportedSchemaVersions() []string
}

// Global registry for wrapper factories
var (
	registry   = make(map[string]WrapperFactory)
	registryMu sync.RWMutex
)

// Register registers a wrapper factory with the global registry.
// This should be called from the wrapper's init() function.
// Panics if a wrapper with the same name is already registered.
func Register(factory WrapperFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()

	name := factory.Name()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("wrapper '%s' is already registered", name))
	}

	registry[name] = factory
}

// GetWrapper retrieves a registered wrapper factory by name.
// Returns an error if the wrapper is not found.
func GetWrapper(name string) (WrapperFactory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	factory, exists := registry[name]
	if !exists {
		// Use the unlocked helper: taking RLock again here can deadlock if a writer is waiting.
		return nil, fmt.Errorf("wrapper '%s' not found. Available wrappers: %v", name, sortedWrapperNames())
	}

	return factory, nil
}

// ListWrappers returns a list of all registered wrapper names.
// The list is sorted alphabetically for consistent output.
func ListWrappers() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	return sortedWrapperNames()
}

// sortedWrapperNames expects the caller to hold registryMu.
func sortedWrapperNames() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// UnregisterAll clears all registered wrappers.
// This is primarily useful for testing.
func UnregisterAll() {
	registryMu.Lock()
	defer registryMu.Unlock()

	registry = make(map[string]WrapperFactory)
}
