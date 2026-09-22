package desiredstate

import (
	"sync"
	"testing"
	"time"
)

// TestMultiHandlerIntegration tests using multiple handlers in the same scenario.
// This simulates a real desiredstate with Docker, S3, and Git components together.
func TestMultiHandlerIntegration(t *testing.T) {
	// Create components of different types
	dockerComp := &VersionedComponent{
		PartID:   "api-service",
		PartFile: "test.yaml",
		Type:     ComponentTypeDocker,
		URL:      "123456789.dkr.ecr.us-east-1.amazonaws.com/my-app",
		Version:  "1.2.3",
		IsLocked: false,
		Metadata: make(map[string]interface{}),
	}

	s3Comp := &VersionedComponent{
		PartID:           "config-data",
		PartFile:         "test.yaml",
		Type:             ComponentTypeS3,
		URL:              "s3://my-bucket/config.zip",
		Version:          "abc123def456",
		IsLocked:         true,
		LatestIdentifier: "latest",
		Metadata:         make(map[string]interface{}),
	}

	gitComp := &VersionedComponent{
		PartID:           "infrastructure",
		PartFile:         "test.yaml",
		Type:             ComponentTypeSourcecode,
		URL:              "https://github.com/user/infra.git",
		Branch:           "main",
		Version:          "main",
		IsLocked:         false,
		LatestIdentifier: "latest",
		Metadata:         make(map[string]interface{}),
	}

	// Test that each handler can be retrieved
	dockerHandler, err := GetComponentHandler(ComponentTypeDocker)
	if err != nil {
		t.Fatalf("Failed to get Docker handler: %v", err)
	}

	s3Handler, err := GetComponentHandler(ComponentTypeS3)
	if err != nil {
		t.Fatalf("Failed to get S3 handler: %v", err)
	}

	gitHandler, err := GetComponentHandler(ComponentTypeSourcecode)
	if err != nil {
		t.Fatalf("Failed to get Git handler: %v", err)
	}

	// Test validation of all components
	if err := dockerHandler.ValidateComponent(dockerComp); err != nil {
		t.Errorf("Docker component validation failed: %v", err)
	}

	if err := s3Handler.ValidateComponent(s3Comp); err != nil {
		t.Errorf("S3 component validation failed: %v", err)
	}

	if err := gitHandler.ValidateComponent(gitComp); err != nil {
		t.Errorf("Git component validation failed: %v", err)
	}

	// Test unlocking all components
	if err := dockerHandler.UnlockVersion(dockerComp); err != nil {
		t.Errorf("Docker unlock failed: %v", err)
	}
	if dockerComp.Version != "latest" {
		t.Errorf("Docker version should be 'latest', got '%s'", dockerComp.Version)
	}

	if err := s3Handler.UnlockVersion(s3Comp); err != nil {
		t.Errorf("S3 unlock failed: %v", err)
	}
	if s3Comp.Version != "latest" {
		t.Errorf("S3 version should be 'latest', got '%s'", s3Comp.Version)
	}

	if err := gitHandler.UnlockVersion(gitComp); err != nil {
		t.Errorf("Git unlock failed: %v", err)
	}
	if gitComp.Version != "main" {
		t.Errorf("Git version should be 'main', got '%s'", gitComp.Version)
	}
}

// TestConcurrentHandlerAccess tests thread-safe access to the component registry.
func TestConcurrentHandlerAccess(t *testing.T) {
	const numGoroutines = 100
	const numIterations = 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numIterations*3)

	// Launch multiple goroutines that access handlers concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numIterations; j++ {
				// Get Docker handler
				handler, err := GetComponentHandler(ComponentTypeDocker)
				if err != nil {
					errors <- err
					continue
				}
				if handler.Type() != ComponentTypeDocker {
					errors <- err
				}

				// Get S3 handler
				handler, err = GetComponentHandler(ComponentTypeS3)
				if err != nil {
					errors <- err
					continue
				}
				if handler.Type() != ComponentTypeS3 {
					errors <- err
				}

				// Get Git handler
				handler, err = GetComponentHandler(ComponentTypeSourcecode)
				if err != nil {
					errors <- err
					continue
				}
				if handler.Type() != ComponentTypeSourcecode {
					errors <- err
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for any errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent access error: %v", err)
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Encountered %d errors during concurrent access", errorCount)
	}

	t.Logf("Successfully completed %d concurrent operations", numGoroutines*numIterations*3)
}

// TestHandlerRegistryLookupPerformance benchmarks handler lookup performance.
func TestHandlerRegistryLookupPerformance(t *testing.T) {
	const numLookups = 10000

	start := time.Now()

	for i := 0; i < numLookups; i++ {
		_, err := GetComponentHandler(ComponentTypeDocker)
		if err != nil {
			t.Fatalf("Handler lookup failed: %v", err)
		}

		_, err = GetComponentHandler(ComponentTypeS3)
		if err != nil {
			t.Fatalf("Handler lookup failed: %v", err)
		}

		_, err = GetComponentHandler(ComponentTypeSourcecode)
		if err != nil {
			t.Fatalf("Handler lookup failed: %v", err)
		}
	}

	duration := time.Since(start)
	avgPerLookup := duration / (numLookups * 3)

	t.Logf("Performed %d handler lookups in %v", numLookups*3, duration)
	t.Logf("Average time per lookup: %v", avgPerLookup)

	// Verify reasonable performance (should be very fast - under 1µs per lookup)
	if avgPerLookup > time.Microsecond {
		t.Errorf("Handler lookup too slow: %v per lookup (expected < 1µs)", avgPerLookup)
	}
}

// TestAllHandlersRegistered verifies all expected handlers are registered.
func TestAllHandlersRegistered(t *testing.T) {
	expectedTypes := []ComponentType{
		ComponentTypeDocker,
		ComponentTypeS3,
		ComponentTypeSourcecode,
	}

	registeredTypes := ListComponentTypes()

	// Check that all expected types are present
	for _, expectedType := range expectedTypes {
		found := false
		for _, registeredType := range registeredTypes {
			if registeredType == expectedType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected handler type %s not registered", expectedType)
		}
	}

	t.Logf("Successfully verified %d registered handlers: %v", len(registeredTypes), registeredTypes)
}

// TestHandlerVersionComparison tests version comparison across different handlers.
func TestHandlerVersionComparison(t *testing.T) {
	tests := []struct {
		name        string
		handlerType ComponentType
		v1          string
		v2          string
		expected    int
	}{
		// Docker semantic versions
		{
			name:        "Docker: v1 < v2",
			handlerType: ComponentTypeDocker,
			v1:          "1.0.0",
			v2:          "2.0.0",
			expected:    -1,
		},
		{
			name:        "Docker: v1 = v2",
			handlerType: ComponentTypeDocker,
			v1:          "1.5.0",
			v2:          "1.5.0",
			expected:    0,
		},
		{
			name:        "Docker: v1 > v2",
			handlerType: ComponentTypeDocker,
			v1:          "3.0.0",
			v2:          "2.9.9",
			expected:    1,
		},

		// S3 semantic versions
		{
			name:        "S3: semantic v1 < v2",
			handlerType: ComponentTypeS3,
			v1:          "1.0.0",
			v2:          "1.1.0",
			expected:    -1,
		},
		{
			name:        "S3: version IDs lexicographic",
			handlerType: ComponentTypeS3,
			v1:          "abc123",
			v2:          "xyz789",
			expected:    -1,
		},

		// Git versions
		{
			name:        "Git: semantic tags v1 < v2",
			handlerType: ComponentTypeSourcecode,
			v1:          "v1.0.0",
			v2:          "v2.0.0",
			expected:    -1,
		},
		{
			name:        "Git: commit SHAs equal",
			handlerType: ComponentTypeSourcecode,
			v1:          "abc123def456",
			v2:          "abc123def456",
			expected:    0,
		},
		{
			name:        "Git: branch names",
			handlerType: ComponentTypeSourcecode,
			v1:          "develop",
			v2:          "main",
			expected:    -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := GetComponentHandler(tt.handlerType)
			if err != nil {
				t.Fatalf("Failed to get handler: %v", err)
			}

			result, err := handler.CompareVersions(tt.v1, tt.v2)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestHandlerParseValidate tests parsing and validation pipeline for all handlers.
func TestHandlerParseValidate(t *testing.T) {
	tests := []struct {
		name        string
		handlerType ComponentType
		data        map[string]interface{}
		wantErr     bool
	}{
		{
			name:        "Docker valid",
			handlerType: ComponentTypeDocker,
			data: map[string]interface{}{
				"image": "my-app",
				"tag":   "1.0.0",
			},
			wantErr: false,
		},
		{
			name:        "Docker invalid - missing image",
			handlerType: ComponentTypeDocker,
			data: map[string]interface{}{
				"tag": "1.0.0",
			},
			wantErr: true,
		},
		{
			name:        "S3 valid",
			handlerType: ComponentTypeS3,
			data: map[string]interface{}{
				"bucket": "my-bucket",
				"key":    "data.zip",
			},
			wantErr: false,
		},
		{
			name:        "S3 invalid - missing bucket",
			handlerType: ComponentTypeS3,
			data: map[string]interface{}{
				"key": "data.zip",
			},
			wantErr: true,
		},
		{
			name:        "Git valid",
			handlerType: ComponentTypeSourcecode,
			data: map[string]interface{}{
				"url":    "https://github.com/user/repo.git",
				"branch": "main",
			},
			wantErr: false,
		},
		{
			name:        "Git invalid - missing URL",
			handlerType: ComponentTypeSourcecode,
			data: map[string]interface{}{
				"branch": "main",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := GetComponentHandler(tt.handlerType)
			if err != nil {
				t.Fatalf("Failed to get handler: %v", err)
			}

			// Parse component
			comp, err := handler.ParseComponent(tt.data, "test-component", "test.yaml")
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected parsing error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected parsing error: %v", err)
				return
			}

			// Validate component
			err = handler.ValidateComponent(comp)
			if err != nil {
				t.Errorf("Validation failed: %v", err)
			}
		})
	}
}

// TestHandlerLockUnlockCycle tests the complete lock/unlock cycle for all handlers.
func TestHandlerLockUnlockCycle(t *testing.T) {
	tests := []struct {
		name            string
		comp            *VersionedComponent
		expectedVersion string
	}{
		{
			name: "Docker component",
			comp: &VersionedComponent{
				Type:     ComponentTypeDocker,
				URL:      "my-app",
				Version:  "1.2.3",
				IsLocked: true,
				Metadata: make(map[string]interface{}),
			},
			expectedVersion: "latest",
		},
		{
			name: "S3 component",
			comp: &VersionedComponent{
				Type:             ComponentTypeS3,
				URL:              "s3://my-bucket/object.zip",
				Version:          "abc123def456",
				IsLocked:         true,
				LatestIdentifier: "latest",
				Metadata:         make(map[string]interface{}),
			},
			expectedVersion: "latest",
		},
		{
			name: "Git component",
			comp: &VersionedComponent{
				Type:             ComponentTypeSourcecode,
				URL:              "https://github.com/user/repo.git",
				Branch:           "develop",
				Version:          "abc123def456",
				IsLocked:         true,
				LatestIdentifier: "latest",
				Metadata:         make(map[string]interface{}),
			},
			expectedVersion: "develop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := GetComponentHandler(tt.comp.Type)
			if err != nil {
				t.Fatalf("Failed to get handler: %v", err)
			}

			// Initial state should be locked
			if !tt.comp.IsLocked {
				t.Errorf("Component should start locked")
			}

			// Unlock
			err = handler.UnlockVersion(tt.comp)
			if err != nil {
				t.Fatalf("Unlock failed: %v", err)
			}

			// Verify unlocked state
			if tt.comp.IsLocked {
				t.Errorf("Component should be unlocked")
			}

			if tt.comp.Version != tt.expectedVersion {
				t.Errorf("Expected version '%s', got '%s'", tt.expectedVersion, tt.comp.Version)
			}
		})
	}
}

// TestHandlerCount verifies the expected number of handlers are registered.
func TestHandlerCount(t *testing.T) {
	types := ListComponentTypes()

	expectedCount := 3 // Docker, S3, Git
	if len(types) < expectedCount {
		t.Errorf("Expected at least %d handlers, got %d", expectedCount, len(types))
	}

	t.Logf("Total handlers registered: %d", len(types))
}
