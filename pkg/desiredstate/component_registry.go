package desiredstate

import (
	"fmt"
	"sync"
)

// ComponentTypeHandler defines the interface that all component type handlers must implement.
// This enables extensibility - custom component types can be registered via external packages.
//
// Example usage:
//
//	type ComposerHandler struct {
//	    packagistClient *PackagistClient
//	}
//
//	func (h *ComposerHandler) Type() ComponentType {
//	    return ComponentType("composer")
//	}
//
//	func init() {
//	    RegisterComponentType(&ComposerHandler{})
//	}
type ComponentTypeHandler interface {
	// Type returns the component type identifier (e.g., "docker", "s3", "composer")
	Type() ComponentType

	// ParseComponent extracts component data from YAML structure.
	// data: raw YAML data for this component (as map[string]interface{})
	// partID: unique identifier within the part file
	// partFile: path to the part file containing this component
	// Returns: parsed VersionedComponent or error
	ParseComponent(data map[string]interface{}, partID, partFile string) (*VersionedComponent, error)

	// ResolveVersion resolves "latest" or unlocked version to a specific version.
	// comp: component to resolve
	// ctx: resolution context (AWS profile, region, credentials, etc.)
	// Returns: resolved version string or error
	ResolveVersion(comp *VersionedComponent, ctx *ResolveContext) (string, error)

	// CompareVersions compares two version strings.
	// v1: first version
	// v2: second version
	// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2, or error
	CompareVersions(v1, v2 string) (int, error)

	// LockVersion locks a component to its current version.
	// comp: component to lock (modified in-place)
	// Returns: error if locking fails
	LockVersion(comp *VersionedComponent) error

	// UnlockVersion unlocks a component to track latest.
	// comp: component to unlock (modified in-place)
	// Returns: error if unlocking fails
	UnlockVersion(comp *VersionedComponent) error

	// ValidateComponent performs component-specific validation.
	// comp: component to validate
	// Returns: error if component is invalid
	ValidateComponent(comp *VersionedComponent) error
}

// ResolveContext provides context for version resolution operations.
// Different component types may use different fields (AWS, API keys, etc.).
type ResolveContext struct {
	// AWS configuration
	AWSProfile string
	AWSRegion  string

	// Generic credentials/API keys for custom handlers
	Credentials map[string]string

	// Handler-specific options
	Options map[string]interface{}
}

// NewResolveContext creates a new resolve context with AWS parameters.
func NewResolveContext(awsProfile, awsRegion string) *ResolveContext {
	return &ResolveContext{
		AWSProfile:  awsProfile,
		AWSRegion:   awsRegion,
		Credentials: make(map[string]string),
		Options:     make(map[string]interface{}),
	}
}

// componentRegistry holds all registered component type handlers
type componentRegistry struct {
	handlers map[ComponentType]ComponentTypeHandler
	mutex    sync.RWMutex
}

// Global component registry instance
var globalComponentRegistry = &componentRegistry{
	handlers: make(map[ComponentType]ComponentTypeHandler),
}

// RegisterComponentType registers a component type handler with the global registry.
// This should be called from the handler's init() function.
// Panics if a handler for the same component type is already registered.
//
// Example:
//
//	func init() {
//	    RegisterComponentType(&DockerHandler{})
//	}
func RegisterComponentType(handler ComponentTypeHandler) {
	globalComponentRegistry.mutex.Lock()
	defer globalComponentRegistry.mutex.Unlock()

	componentType := handler.Type()

	if _, exists := globalComponentRegistry.handlers[componentType]; exists {
		panic(fmt.Sprintf("component type '%s' is already registered", componentType))
	}

	globalComponentRegistry.handlers[componentType] = handler
}

// GetComponentHandler retrieves a registered component type handler by type.
// Returns an error if the component type is not found.
//
// Example:
//
//	handler, err := GetComponentHandler(ComponentTypeDocker)
//	if err != nil {
//	    return fmt.Errorf("unknown component type: %w", err)
//	}
//	version, err := handler.ResolveVersion(comp, ctx)
func GetComponentHandler(componentType ComponentType) (ComponentTypeHandler, error) {
	globalComponentRegistry.mutex.RLock()
	defer globalComponentRegistry.mutex.RUnlock()

	handler, exists := globalComponentRegistry.handlers[componentType]
	if !exists {
		availableTypes := ListComponentTypes()
		return nil, fmt.Errorf("component type '%s' not found. Available types: %v", componentType, availableTypes)
	}

	return handler, nil
}

// HasComponentType checks if a component type is registered.
// Returns true if a handler for the type exists, false otherwise.
func HasComponentType(componentType ComponentType) bool {
	globalComponentRegistry.mutex.RLock()
	defer globalComponentRegistry.mutex.RUnlock()

	_, exists := globalComponentRegistry.handlers[componentType]
	return exists
}

// ListComponentTypes returns a sorted list of all registered component type names.
// Useful for debugging and help messages.
func ListComponentTypes() []ComponentType {
	globalComponentRegistry.mutex.RLock()
	defer globalComponentRegistry.mutex.RUnlock()

	types := make([]ComponentType, 0, len(globalComponentRegistry.handlers))
	for componentType := range globalComponentRegistry.handlers {
		types = append(types, componentType)
	}

	// Simple bubble sort to avoid importing "sort" package
	for i := 0; i < len(types); i++ {
		for j := i + 1; j < len(types); j++ {
			if string(types[i]) > string(types[j]) {
				types[i], types[j] = types[j], types[i]
			}
		}
	}

	return types
}

// GetComponentTypeCount returns the number of registered component types.
// Useful for testing and diagnostics.
func GetComponentTypeCount() int {
	globalComponentRegistry.mutex.RLock()
	defer globalComponentRegistry.mutex.RUnlock()

	return len(globalComponentRegistry.handlers)
}

// ClearComponentRegistry clears all registered component types.
// WARNING: This should only be used in tests to reset state between test cases.
func ClearComponentRegistry() {
	globalComponentRegistry.mutex.Lock()
	defer globalComponentRegistry.mutex.Unlock()

	globalComponentRegistry.handlers = make(map[ComponentType]ComponentTypeHandler)
}
