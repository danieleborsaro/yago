package desiredstate

import (
	"sync"
	"testing"
)

// MockComponentHandler is a test implementation of ComponentTypeHandler
type MockComponentHandler struct {
	componentType ComponentType
	parseFunc     func(map[string]interface{}, string, string) (*VersionedComponent, error)
	resolveFunc   func(*VersionedComponent, *ResolveContext) (string, error)
}

func (m *MockComponentHandler) Type() ComponentType {
	return m.componentType
}

func (m *MockComponentHandler) ParseComponent(data map[string]interface{}, partID, partFile string) (*VersionedComponent, error) {
	if m.parseFunc != nil {
		return m.parseFunc(data, partID, partFile)
	}
	return &VersionedComponent{
		PartID:  partID,
		Type:    m.componentType,
		Version: "1.0.0",
	}, nil
}

func (m *MockComponentHandler) ResolveVersion(comp *VersionedComponent, ctx *ResolveContext) (string, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(comp, ctx)
	}
	return "1.0.0", nil
}

func (m *MockComponentHandler) CompareVersions(v1, v2 string) (int, error) {
	return CompareSemanticVersions(v1, v2)
}

func (m *MockComponentHandler) LockVersion(comp *VersionedComponent) error {
	comp.IsLocked = true
	return nil
}

func (m *MockComponentHandler) UnlockVersion(comp *VersionedComponent) error {
	comp.IsLocked = false
	comp.Version = "latest"
	return nil
}

func (m *MockComponentHandler) ValidateComponent(comp *VersionedComponent) error {
	return nil
}

func TestRegisterComponentType(t *testing.T) {
	// Save the current registry state
	globalComponentRegistry.mutex.Lock()
	savedHandlers := make(map[ComponentType]ComponentTypeHandler)
	for k, v := range globalComponentRegistry.handlers {
		savedHandlers[k] = v
	}
	globalComponentRegistry.mutex.Unlock()

	// Cleanup: restore registry after test
	t.Cleanup(func() {
		globalComponentRegistry.mutex.Lock()
		globalComponentRegistry.handlers = savedHandlers
		globalComponentRegistry.mutex.Unlock()
	})

	// Clear registry before test
	ClearComponentRegistry()

	handler := &MockComponentHandler{
		componentType: ComponentType("test-type"),
	}

	RegisterComponentType(handler)

	// Verify registration
	retrieved, err := GetComponentHandler(ComponentType("test-type"))
	if err != nil {
		t.Fatalf("Failed to get registered handler: %v", err)
	}

	if retrieved.Type() != ComponentType("test-type") {
		t.Errorf("Expected type 'test-type', got '%s'", retrieved.Type())
	}
}

func TestRegisterComponentType_Duplicate(t *testing.T) {
	// Save the current registry state
	globalComponentRegistry.mutex.Lock()
	savedHandlers := make(map[ComponentType]ComponentTypeHandler)
	for k, v := range globalComponentRegistry.handlers {
		savedHandlers[k] = v
	}
	globalComponentRegistry.mutex.Unlock()

	// Cleanup: restore registry after test
	t.Cleanup(func() {
		globalComponentRegistry.mutex.Lock()
		globalComponentRegistry.handlers = savedHandlers
		globalComponentRegistry.mutex.Unlock()
	})

	// Clear registry before test
	ClearComponentRegistry()

	handler1 := &MockComponentHandler{
		componentType: ComponentType("duplicate"),
	}
	handler2 := &MockComponentHandler{
		componentType: ComponentType("duplicate"),
	}

	RegisterComponentType(handler1)

	// Attempt to register duplicate should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when registering duplicate component type")
		}
	}()

	RegisterComponentType(handler2)
}

func TestGetComponentHandler_NotFound(t *testing.T) {
	// Save the current registry state
	globalComponentRegistry.mutex.Lock()
	savedHandlers := make(map[ComponentType]ComponentTypeHandler)
	for k, v := range globalComponentRegistry.handlers {
		savedHandlers[k] = v
	}
	globalComponentRegistry.mutex.Unlock()

	// Cleanup: restore registry after test
	t.Cleanup(func() {
		globalComponentRegistry.mutex.Lock()
		globalComponentRegistry.handlers = savedHandlers
		globalComponentRegistry.mutex.Unlock()
	})

	// Clear registry before test
	ClearComponentRegistry()

	_, err := GetComponentHandler(ComponentType("nonexistent"))
	if err == nil {
		t.Error("Expected error when getting nonexistent handler")
	}
}

func TestHasComponentType(t *testing.T) {
	// Save the current registry state
	globalComponentRegistry.mutex.Lock()
	savedHandlers := make(map[ComponentType]ComponentTypeHandler)
	for k, v := range globalComponentRegistry.handlers {
		savedHandlers[k] = v
	}
	globalComponentRegistry.mutex.Unlock()

	// Cleanup: restore registry after test
	t.Cleanup(func() {
		globalComponentRegistry.mutex.Lock()
		globalComponentRegistry.handlers = savedHandlers
		globalComponentRegistry.mutex.Unlock()
	})

	// Clear registry before test
	ClearComponentRegistry()

	handler := &MockComponentHandler{
		componentType: ComponentType("exists"),
	}

	RegisterComponentType(handler)

	if !HasComponentType(ComponentType("exists")) {
		t.Error("Expected HasComponentType to return true for registered type")
	}

	if HasComponentType(ComponentType("nonexistent")) {
		t.Error("Expected HasComponentType to return false for unregistered type")
	}
}

func TestListComponentTypes(t *testing.T) {
	// Save the current registry state
	globalComponentRegistry.mutex.Lock()
	savedHandlers := make(map[ComponentType]ComponentTypeHandler)
	for k, v := range globalComponentRegistry.handlers {
		savedHandlers[k] = v
	}
	globalComponentRegistry.mutex.Unlock()

	// Cleanup: restore registry after test
	t.Cleanup(func() {
		globalComponentRegistry.mutex.Lock()
		globalComponentRegistry.handlers = savedHandlers
		globalComponentRegistry.mutex.Unlock()
	})

	// Clear registry before test
	ClearComponentRegistry()

	// Register multiple handlers
	RegisterComponentType(&MockComponentHandler{componentType: ComponentType("docker")})
	RegisterComponentType(&MockComponentHandler{componentType: ComponentType("s3")})
	RegisterComponentType(&MockComponentHandler{componentType: ComponentType("git")})

	types := ListComponentTypes()

	if len(types) != 3 {
		t.Errorf("Expected 3 component types, got %d", len(types))
	}

	// Verify sorted order
	expected := []ComponentType{"docker", "git", "s3"}
	for i, expectedType := range expected {
		if types[i] != expectedType {
			t.Errorf("Expected type[%d] = '%s', got '%s'", i, expectedType, types[i])
		}
	}
}

func TestGetComponentTypeCount(t *testing.T) {
	// Save the current registry state
	globalComponentRegistry.mutex.Lock()
	savedHandlers := make(map[ComponentType]ComponentTypeHandler)
	for k, v := range globalComponentRegistry.handlers {
		savedHandlers[k] = v
	}
	globalComponentRegistry.mutex.Unlock()

	// Cleanup: restore registry after test
	t.Cleanup(func() {
		globalComponentRegistry.mutex.Lock()
		globalComponentRegistry.handlers = savedHandlers
		globalComponentRegistry.mutex.Unlock()
	})

	// Clear registry before test
	ClearComponentRegistry()

	if GetComponentTypeCount() != 0 {
		t.Error("Expected empty registry initially")
	}

	RegisterComponentType(&MockComponentHandler{componentType: ComponentType("type1")})
	RegisterComponentType(&MockComponentHandler{componentType: ComponentType("type2")})

	if GetComponentTypeCount() != 2 {
		t.Errorf("Expected 2 component types, got %d", GetComponentTypeCount())
	}
}

func TestConcurrentRegistration(t *testing.T) {
	// Save the current registry state
	globalComponentRegistry.mutex.Lock()
	savedHandlers := make(map[ComponentType]ComponentTypeHandler)
	for k, v := range globalComponentRegistry.handlers {
		savedHandlers[k] = v
	}
	globalComponentRegistry.mutex.Unlock()

	// Cleanup: restore registry after test
	t.Cleanup(func() {
		globalComponentRegistry.mutex.Lock()
		globalComponentRegistry.handlers = savedHandlers
		globalComponentRegistry.mutex.Unlock()
	})

	// Clear registry before test
	ClearComponentRegistry()

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Register 10 handlers concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			handler := &MockComponentHandler{
				componentType: ComponentType("concurrent-" + string(rune(index+'0'))),
			}
			RegisterComponentType(handler)
		}(i)
	}

	wg.Wait()
	close(errors)

	// Verify all handlers were registered
	if GetComponentTypeCount() != 10 {
		t.Errorf("Expected 10 handlers, got %d", GetComponentTypeCount())
	}
}

func TestConcurrentAccess(t *testing.T) {
	// Save the current registry state
	globalComponentRegistry.mutex.Lock()
	savedHandlers := make(map[ComponentType]ComponentTypeHandler)
	for k, v := range globalComponentRegistry.handlers {
		savedHandlers[k] = v
	}
	globalComponentRegistry.mutex.Unlock()

	// Cleanup: restore registry after test
	t.Cleanup(func() {
		globalComponentRegistry.mutex.Lock()
		globalComponentRegistry.handlers = savedHandlers
		globalComponentRegistry.mutex.Unlock()
	})

	// Clear registry before test
	ClearComponentRegistry()

	// Register a handler
	RegisterComponentType(&MockComponentHandler{
		componentType: ComponentType("concurrent-access"),
	})

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// 50 goroutines reading, 50 checking existence
	for i := 0; i < 50; i++ {
		wg.Add(2)

		// Reader goroutine
		go func() {
			defer wg.Done()
			_, err := GetComponentHandler(ComponentType("concurrent-access"))
			if err != nil {
				errors <- err
			}
		}()

		// Checker goroutine
		go func() {
			defer wg.Done()
			if !HasComponentType(ComponentType("concurrent-access")) {
				errors <- nil // Just signal an issue
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for range errors {
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Concurrent access produced %d errors", errorCount)
	}
}

func TestResolveContext(t *testing.T) {
	ctx := NewResolveContext("my-profile", "us-east-1")

	if ctx.AWSProfile != "my-profile" {
		t.Errorf("Expected AWS profile 'my-profile', got '%s'", ctx.AWSProfile)
	}

	if ctx.AWSRegion != "us-east-1" {
		t.Errorf("Expected AWS region 'us-east-1', got '%s'", ctx.AWSRegion)
	}

	if ctx.Credentials == nil {
		t.Error("Expected initialized Credentials map")
	}

	if ctx.Options == nil {
		t.Error("Expected initialized Options map")
	}

	// Test adding credentials
	ctx.Credentials["api_key"] = "secret123"
	if ctx.Credentials["api_key"] != "secret123" {
		t.Error("Failed to set credential")
	}

	// Test adding options
	ctx.Options["timeout"] = 30
	if ctx.Options["timeout"] != 30 {
		t.Error("Failed to set option")
	}
}
