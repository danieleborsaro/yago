package desiredstate

import (
	"testing"

	"github.com/danieleborsaro/yago/pkg/aws"
)

func TestDockerHandler_Type(t *testing.T) {
	handler := NewDockerHandler(nil)
	if handler.Type() != ComponentTypeDocker {
		t.Errorf("Expected type %s, got %s", ComponentTypeDocker, handler.Type())
	}
}

func TestDockerHandler_ParseComponent(t *testing.T) {
	handler := NewDockerHandler(nil)

	tests := []struct {
		name      string
		data      map[string]interface{}
		partID    string
		partFile  string
		wantError bool
		checkFunc func(*testing.T, *VersionedComponent)
	}{
		{
			name: "Valid Docker component with tag",
			data: map[string]interface{}{
				"image": "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app",
				"tag":   "1.5.0",
			},
			partID:    "artifacts.my-app.docker.eu-west-1",
			partFile:  "test.yaml",
			wantError: false,
			checkFunc: func(t *testing.T, comp *VersionedComponent) {
				if comp.URL != "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app" {
					t.Errorf("Expected URL to be set, got %s", comp.URL)
				}
				if comp.Version != "1.5.0" {
					t.Errorf("Expected version 1.5.0, got %s", comp.Version)
				}
				if !comp.IsLocked {
					t.Error("Expected component to be locked")
				}
			},
		},
		{
			name: "Docker component with latest tag",
			data: map[string]interface{}{
				"image": "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app",
				"tag":   "latest",
			},
			partID:    "artifacts.my-app.docker.eu-west-1",
			partFile:  "test.yaml",
			wantError: false,
			checkFunc: func(t *testing.T, comp *VersionedComponent) {
				if comp.Version != "latest" {
					t.Errorf("Expected version latest, got %s", comp.Version)
				}
				if comp.IsLocked {
					t.Error("Expected component to be unlocked")
				}
			},
		},
		{
			name: "Docker component with digest",
			data: map[string]interface{}{
				"image": "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app",
				"tag":   "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			},
			partID:    "artifacts.my-app.docker.eu-west-1",
			partFile:  "test.yaml",
			wantError: false,
			checkFunc: func(t *testing.T, comp *VersionedComponent) {
				if !comp.IsLocked {
					t.Error("Expected digest to be locked")
				}
				isDigest, ok := comp.Metadata["is_digest"].(bool)
				if !ok || !isDigest {
					t.Error("Expected is_digest metadata to be true")
				}
			},
		},
		{
			name: "Docker component without tag (defaults to latest)",
			data: map[string]interface{}{
				"image": "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app",
			},
			partID:    "artifacts.my-app.docker.eu-west-1",
			partFile:  "test.yaml",
			wantError: false,
			checkFunc: func(t *testing.T, comp *VersionedComponent) {
				if comp.Version != "latest" {
					t.Errorf("Expected default version latest, got %s", comp.Version)
				}
			},
		},
		{
			name: "Missing image field",
			data: map[string]interface{}{
				"tag": "1.0.0",
			},
			partID:    "artifacts.my-app.docker.eu-west-1",
			partFile:  "test.yaml",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, err := handler.ParseComponent(tt.data, tt.partID, tt.partFile)
			if (err != nil) != tt.wantError {
				t.Errorf("ParseComponent() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if err == nil && tt.checkFunc != nil {
				tt.checkFunc(t, comp)
			}
		})
	}
}

func TestDockerHandler_ValidateDockerDigest(t *testing.T) {
	handler := NewDockerHandler(nil)

	tests := []struct {
		name      string
		digest    string
		wantError bool
	}{
		{
			name:      "Valid digest",
			digest:    "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			wantError: false,
		},
		{
			name:      "Missing sha256 prefix",
			digest:    "abc123def456789012345678901234567890abcdef123456789012345678901",
			wantError: true,
		},
		{
			name:      "Wrong hash length",
			digest:    "sha256:abc123",
			wantError: true,
		},
		{
			name:      "Invalid characters",
			digest:    "sha256:ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateDockerDigest(tt.digest)
			if (err != nil) != tt.wantError {
				t.Errorf("validateDockerDigest() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestDockerHandler_CompareVersions(t *testing.T) {
	handler := NewDockerHandler(nil)

	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
		wantErr  bool
	}{
		{
			name:     "Semantic versions - v1 < v2",
			v1:       "1.0.0",
			v2:       "2.0.0",
			expected: -1,
			wantErr:  false,
		},
		{
			name:     "Semantic versions - v1 > v2",
			v1:       "3.0.0",
			v2:       "2.0.0",
			expected: 1,
			wantErr:  false,
		},
		{
			name:     "Semantic versions - equal",
			v1:       "1.5.3",
			v2:       "1.5.3",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "Both digests - equal",
			v1:       "sha256:abc123def4567890123456789012345678901234abcdef1234567890123456",
			v2:       "sha256:abc123def4567890123456789012345678901234abcdef1234567890123456",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "Both digests - v1 < v2",
			v1:       "sha256:aaa0000000000000000000000000000000000000000000000000000000000000",
			v2:       "sha256:bbb0000000000000000000000000000000000000000000000000000000000000",
			expected: -1,
			wantErr:  false,
		},
		{
			name:    "Mixed digest and version - error",
			v1:      "sha256:abc123def4567890123456789012345678901234abcdef1234567890123456",
			v2:      "1.0.0",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler.CompareVersions(tt.v1, tt.v2)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompareVersions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("CompareVersions() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestDockerHandler_ValidateComponent(t *testing.T) {
	handler := NewDockerHandler(nil)

	tests := []struct {
		name      string
		comp      *VersionedComponent
		wantError bool
	}{
		{
			name: "Valid component with tag",
			comp: &VersionedComponent{
				PartID:  "test-docker",
				URL:     "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app",
				Version: "1.0.0",
			},
			wantError: false,
		},
		{
			name: "Valid component with digest",
			comp: &VersionedComponent{
				PartID:  "test-docker",
				URL:     "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app",
				Version: "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			},
			wantError: false,
		},
		{
			name: "Missing URL",
			comp: &VersionedComponent{
				PartID:  "test-docker",
				Version: "1.0.0",
			},
			wantError: true,
		},
		{
			name: "Missing version",
			comp: &VersionedComponent{
				PartID: "test-docker",
				URL:    "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app",
			},
			wantError: true,
		},
		{
			name: "Invalid digest",
			comp: &VersionedComponent{
				PartID:  "test-docker",
				URL:     "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app",
				Version: "sha256:invalid",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.ValidateComponent(tt.comp)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateComponent() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestDockerHandler_UnlockVersion(t *testing.T) {
	handler := NewDockerHandler(nil)

	comp := &VersionedComponent{
		PartID:   "test-docker",
		URL:      "1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app",
		Version:  "1.5.0",
		IsLocked: true,
		Metadata: make(map[string]interface{}),
	}

	err := handler.UnlockVersion(comp)
	if err != nil {
		t.Fatalf("UnlockVersion() error = %v", err)
	}

	if comp.Version != "latest" {
		t.Errorf("Expected version to be 'latest', got %s", comp.Version)
	}
	if comp.IsLocked {
		t.Error("Expected IsLocked to be false")
	}
	if comp.OldVersion != "1.5.0" {
		t.Errorf("Expected OldVersion to be '1.5.0', got %s", comp.OldVersion)
	}
}

func TestDockerHandler_SetECRManager(t *testing.T) {
	handler := NewDockerHandler(nil)
	if handler.ecrManager != nil {
		t.Error("Expected initial ECR manager to be nil")
	}

	ecrManager, err := aws.NewECRManager("test-profile", "us-east-1")
	if err != nil {
		t.Skipf("Skipping test: cannot create ECR manager: %v", err)
	}
	handler.SetECRManager(ecrManager)

	if handler.ecrManager == nil {
		t.Error("Expected ECR manager to be set")
	}
}

func TestDockerHandler_Registration(t *testing.T) {
	// The init() function should have registered the Docker handler
	handler, err := GetComponentHandler(ComponentTypeDocker)
	if err != nil {
		t.Fatalf("Docker handler not registered: %v", err)
	}

	if handler.Type() != ComponentTypeDocker {
		t.Errorf("Expected type %s, got %s", ComponentTypeDocker, handler.Type())
	}

	// Verify it's a DockerHandler
	_, ok := handler.(*DockerHandler)
	if !ok {
		t.Error("Registered handler is not a DockerHandler")
	}
}
