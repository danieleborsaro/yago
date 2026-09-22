package desiredstate

import (
	"testing"

	"github.com/danieleborsaro/yago/pkg/aws"
)

// TestComponentManager_LockUnlockIntegration tests the lock/unlock flow
// through the handler registry, verifying that ComponentManager correctly
// delegates to handlers.
func TestComponentManager_LockUnlockIntegration(t *testing.T) {
	// Create a Docker component through the parser
	parser := NewComponentParser(false, false)
	yamlData := map[string]interface{}{
		"desiredstate": map[string]interface{}{
			"content": map[string]interface{}{
				"components": map[string]interface{}{
					"artifacts": map[string]interface{}{
						"test-app": map[string]interface{}{
							"docker": map[string]interface{}{
								"us-east-1": map[string]interface{}{
									"image": "123456789.dkr.ecr.us-east-1.amazonaws.com/test-app",
									"tag":   "latest",
								},
							},
						},
					},
				},
			},
		},
	}

	err := parser.ParseFromYAML(yamlData, "test.yaml")
	if err != nil {
		t.Fatalf("Failed to parse component: %v", err)
	}

	// Get the parsed component
	components := parser.GetManager().GetAllComponents()
	if len(components) != 1 {
		t.Fatalf("Expected 1 component, got %d", len(components))
	}

	comp := components[0]

	// Verify initial state
	if comp.IsLocked {
		t.Error("Component should initially be unlocked")
	}
	if comp.Version != "latest" {
		t.Errorf("Expected version 'latest', got '%s'", comp.Version)
	}

	// Test unlock on already unlocked component (should be no-op)
	err = comp.Unlock(nil, nil)
	if err != nil {
		t.Errorf("Unlock on unlocked component should not error: %v", err)
	}

	// Verify state unchanged
	if comp.IsLocked {
		t.Error("Component should still be unlocked")
	}
}

// TestComponentManager_HandlerFallback tests that components without
// registered handlers fall back to legacy behavior.
func TestComponentManager_HandlerFallback(t *testing.T) {
	// Test 1: Component with unregistered type and "latest" version
	// This should trigger the legacy switch statement which returns error for unknown type
	comp1 := &VersionedComponent{
		PartID:               "test.unknown1",
		Type:                 ComponentTypeUnknown,
		Version:              "latest", // This triggers version resolution
		IsLocked:             false,
		IsFailOnUnableToLock: false, // Should log warning but not error
		LatestIdentifier:     "latest",
		Metadata:             make(map[string]interface{}),
	}

	// Try to lock - should fall back to legacy, warn, and return nil (no error)
	err := comp1.Lock(nil, nil)
	if err != nil {
		t.Errorf("Expected no error with IsFailOnUnableToLock=false, got: %v", err)
	}
	if comp1.IsLocked {
		t.Error("Component should not be locked after failed lock attempt")
	}

	// Test 2: Component with unregistered type, "latest", and IsFailOnUnableToLock = true
	comp2 := &VersionedComponent{
		PartID:               "test.unknown2",
		Type:                 ComponentTypeUnknown,
		Version:              "latest", // This triggers version resolution
		IsLocked:             false,
		IsFailOnUnableToLock: true, // Should return error
		LatestIdentifier:     "latest",
		Metadata:             make(map[string]interface{}),
	}

	err = comp2.Lock(nil, nil)
	if err == nil {
		t.Error("Expected error for unknown component type with IsFailOnUnableToLock=true")
	}
	if comp2.IsLocked {
		t.Error("Component should not be locked after failed lock attempt")
	}

	// Test 3: Component with specific version already set (not "latest")
	// Legacy behavior will treat this as already locked and return the version
	comp3 := &VersionedComponent{
		PartID:           "test.unknown3",
		Type:             ComponentTypeUnknown,
		Version:          "1.0.0", // Specific version, not "latest"
		IsLocked:         false,
		LatestIdentifier: "latest",
		Metadata:         make(map[string]interface{}),
	}

	err = comp3.Lock(nil, nil)
	if err != nil {
		t.Errorf("Lock with specific version should succeed: %v", err)
	}
	if !comp3.IsLocked {
		t.Error("Component with specific version should be locked after Lock()")
	}
	if comp3.Version != "1.0.0" {
		t.Errorf("Version should remain 1.0.0, got %s", comp3.Version)
	}
}

// TestComponentManager_SetAWSManagers verifies that SetAWSManagers
// correctly propagates managers to handlers.
func TestComponentManager_SetAWSManagers(t *testing.T) {
	manager := NewComponentManager()

	// Get the Docker handler before setting managers
	handler, err := GetComponentHandler(ComponentTypeDocker)
	if err != nil {
		t.Fatalf("Failed to get Docker handler: %v", err)
	}

	dockerHandler, ok := handler.(*DockerHandler)
	if !ok {
		t.Fatal("Handler is not a DockerHandler")
	}

	// Store initial ECR manager state (may be nil or may be set from previous tests)
	initialManager := dockerHandler.ecrManager

	// Create mock ECR manager (will fail to initialize without AWS config, but that's OK)
	ecrManager, ecrErr := aws.NewECRManager("test-profile", "us-east-1")
	if ecrErr != nil {
		// Skip test if we can't create ECR manager (no AWS credentials)
		t.Skipf("Cannot create ECR manager (no AWS credentials): %v", ecrErr)
	}

	// Set AWS managers on component manager
	manager.SetAWSManagers(ecrManager, nil)

	// Verify Docker handler now has ECR manager
	// Since the registry stores a pointer to the handler, the same instance should be updated
	handler2, _ := GetComponentHandler(ComponentTypeDocker)
	dockerHandler2, _ := handler2.(*DockerHandler)

	if dockerHandler2.ecrManager == nil {
		t.Error("Docker handler should have ECR manager after SetAWSManagers")
	}

	// Verify it's the same handler instance (pointer equality)
	if dockerHandler != dockerHandler2 {
		t.Error("Handler instances should be the same (registry should return same pointer)")
	}

	// Restore initial state if needed
	if initialManager == nil {
		dockerHandler.ecrManager = nil
	}
}

// TestVersionedComponent_LockWithDigest tests that components already
// locked with digests are handled correctly.
func TestVersionedComponent_LockWithDigest(t *testing.T) {
	// Create a Docker component already locked with a digest
	comp := &VersionedComponent{
		PartID:   "test.docker",
		Type:     ComponentTypeDocker,
		URL:      "123456789.dkr.ecr.us-east-1.amazonaws.com/test-app",
		Version:  "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		IsLocked: true,
		Metadata: map[string]interface{}{
			"is_digest": true,
		},
	}

	// Try to lock again - should be no-op
	err := comp.Lock(nil, nil)
	if err != nil {
		t.Errorf("Lock on already locked component should not error: %v", err)
	}

	// Verify version unchanged
	if comp.Version != "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Errorf("Version should be unchanged after lock on locked component")
	}
}

// TestVersionedComponent_GetLockWithHandler tests GetLock delegates to handler
func TestVersionedComponent_GetLockWithHandler(t *testing.T) {
	// Create a component with a digest (already locked)
	comp := &VersionedComponent{
		PartID:  "test.docker",
		Type:    ComponentTypeDocker,
		URL:     "123456789.dkr.ecr.us-east-1.amazonaws.com/test-app",
		Version: "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		Region:  "us-east-1",
		Metadata: map[string]interface{}{
			"is_digest": true,
		},
	}

	// GetLock should validate and return the digest
	lockedVersion, err := comp.GetLock(nil, nil)
	if err != nil {
		t.Errorf("GetLock failed: %v", err)
	}

	if lockedVersion != comp.Version {
		t.Errorf("Expected locked version '%s', got '%s'", comp.Version, lockedVersion)
	}
}

// TestComponentManager_ComponentCount verifies component counting works
func TestComponentManager_ComponentCount(t *testing.T) {
	manager := NewComponentManager()

	// Add some test components
	comp1 := &VersionedComponent{
		PartFile: "test1.yaml",
		PartID:   "comp1",
		Type:     ComponentTypeDocker,
		IsLocked: true,
	}
	comp2 := &VersionedComponent{
		PartFile: "test1.yaml",
		PartID:   "comp2",
		Type:     ComponentTypeDocker,
		IsLocked: false,
	}
	comp3 := &VersionedComponent{
		PartFile: "test2.yaml",
		PartID:   "comp3",
		Type:     ComponentTypeS3,
		IsLocked: true,
	}

	manager.AddComponent(comp1)
	manager.AddComponent(comp2)
	manager.AddComponent(comp3)

	// Count components
	total, locked, unlocked := manager.CountComponents()

	if total != 3 {
		t.Errorf("Expected 3 total components, got %d", total)
	}
	if locked != 2 {
		t.Errorf("Expected 2 locked components, got %d", locked)
	}
	if unlocked != 1 {
		t.Errorf("Expected 1 unlocked component, got %d", unlocked)
	}
}
