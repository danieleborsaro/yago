package desiredstate

import (
	"testing"

	"github.com/danieleborsaro/yago/pkg/aws"
)

// TestS3Handler_Type tests that the handler returns the correct type.
func TestS3Handler_Type(t *testing.T) {
	handler := NewS3Handler(nil)
	if handler.Type() != ComponentTypeS3 {
		t.Errorf("Expected type %s, got %s", ComponentTypeS3, handler.Type())
	}
}

// TestS3Handler_ParseComponent tests parsing S3 components from YAML.
func TestS3Handler_ParseComponent(t *testing.T) {
	handler := NewS3Handler(nil)

	tests := []struct {
		name     string
		data     map[string]interface{}
		wantErr  bool
		wantURL  string
		wantVer  string
		wantLock bool
	}{
		{
			name: "basic s3 object with version",
			data: map[string]interface{}{
				"bucket":  "my-bucket",
				"key":     "path/to/object.zip",
				"version": "abc123def456",
			},
			wantErr:  false,
			wantURL:  "s3://my-bucket/path/to/object.zip",
			wantVer:  "abc123def456",
			wantLock: true,
		},
		{
			name: "s3 object with version_id field",
			data: map[string]interface{}{
				"bucket":     "my-bucket",
				"key":        "data.json",
				"version_id": "xyz789ghi012",
			},
			wantErr:  false,
			wantURL:  "s3://my-bucket/data.json",
			wantVer:  "xyz789ghi012",
			wantLock: true,
		},
		{
			name: "s3 object with latest",
			data: map[string]interface{}{
				"bucket":  "my-bucket",
				"key":     "config.yaml",
				"version": "latest",
			},
			wantErr:  false,
			wantURL:  "s3://my-bucket/config.yaml",
			wantVer:  "latest",
			wantLock: false,
		},
		{
			name: "s3 object without version defaults to latest",
			data: map[string]interface{}{
				"bucket": "test-bucket",
				"key":    "file.txt",
			},
			wantErr:  false,
			wantURL:  "s3://test-bucket/file.txt",
			wantVer:  "latest",
			wantLock: false,
		},
		{
			name: "missing bucket field",
			data: map[string]interface{}{
				"key": "file.txt",
			},
			wantErr: true,
		},
		{
			name: "missing key field",
			data: map[string]interface{}{
				"bucket": "my-bucket",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, err := handler.ParseComponent(tt.data, "test-component", "test.yaml")
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if comp.URL != tt.wantURL {
				t.Errorf("URL: expected %s, got %s", tt.wantURL, comp.URL)
			}

			if comp.Version != tt.wantVer {
				t.Errorf("Version: expected %s, got %s", tt.wantVer, comp.Version)
			}

			if comp.IsLocked != tt.wantLock {
				t.Errorf("IsLocked: expected %t, got %t", tt.wantLock, comp.IsLocked)
			}

			if comp.Type != ComponentTypeS3 {
				t.Errorf("Type: expected %s, got %s", ComponentTypeS3, comp.Type)
			}
		})
	}
}

// TestS3Handler_ValidateS3VersionID tests S3 version ID validation.
func TestS3Handler_ValidateS3VersionID(t *testing.T) {
	handler := NewS3Handler(nil)

	tests := []struct {
		name      string
		versionID string
		wantErr   bool
	}{
		{
			name:      "valid version id",
			versionID: "abcdefghij1234567890",
			wantErr:   false,
		},
		{
			name:      "valid long version id",
			versionID: "3sL4kqtJlcpXroDTDmJ.YJKDMQ0Ubu2hAAAAAAAAAAAA",
			wantErr:   false,
		},
		{
			name:      "valid with dots and dashes",
			versionID: "abc-123.def_456",
			wantErr:   false,
		},
		{
			name:      "empty version id",
			versionID: "",
			wantErr:   true,
		},
		{
			name:      "too short",
			versionID: "abc123",
			wantErr:   true,
		},
		{
			name:      "invalid characters",
			versionID: "abc123def456!@#",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateS3VersionID(tt.versionID)
			if tt.wantErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestS3Handler_CompareVersions tests version comparison for S3 version IDs.
func TestS3Handler_CompareVersions(t *testing.T) {
	handler := NewS3Handler(nil)

	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int // -1, 0, 1
	}{
		{
			name:     "semantic versions - v1 older",
			v1:       "1.0.0",
			v2:       "1.1.0",
			expected: -1,
		},
		{
			name:     "semantic versions - equal",
			v1:       "2.5.3",
			v2:       "2.5.3",
			expected: 0,
		},
		{
			name:     "semantic versions - v1 newer",
			v1:       "3.0.0",
			v2:       "2.9.9",
			expected: 1,
		},
		{
			name:     "timestamp versions",
			v1:       "20230101120000",
			v2:       "20230102120000",
			expected: -1,
		},
		{
			name:     "opaque version ids - lexicographic",
			v1:       "abc123def456",
			v2:       "xyz789ghi012",
			expected: -1,
		},
		{
			name:     "opaque version ids - equal",
			v1:       "abc123def456",
			v2:       "abc123def456",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

// TestS3Handler_ValidateComponent tests component validation.
func TestS3Handler_ValidateComponent(t *testing.T) {
	handler := NewS3Handler(nil)

	tests := []struct {
		name    string
		comp    *VersionedComponent
		wantErr bool
	}{
		{
			name: "valid component with version",
			comp: &VersionedComponent{
				URL:     "s3://my-bucket/path/to/object.zip",
				Version: "abc123def456",
				Type:    ComponentTypeS3,
			},
			wantErr: false,
		},
		{
			name: "valid component with latest",
			comp: &VersionedComponent{
				URL:     "s3://my-bucket/data.json",
				Version: "latest",
				Type:    ComponentTypeS3,
			},
			wantErr: false,
		},
		{
			name: "empty URL",
			comp: &VersionedComponent{
				URL:     "",
				Version: "abc123",
				Type:    ComponentTypeS3,
			},
			wantErr: true,
		},
		{
			name: "invalid S3 URL format",
			comp: &VersionedComponent{
				URL:     "http://my-bucket/object",
				Version: "abc123",
				Type:    ComponentTypeS3,
			},
			wantErr: true,
		},
		{
			name: "invalid version ID format",
			comp: &VersionedComponent{
				URL:     "s3://my-bucket/object",
				Version: "short",
				Type:    ComponentTypeS3,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.ValidateComponent(tt.comp)
			if tt.wantErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestS3Handler_UnlockVersion tests unlocking an S3 component.
func TestS3Handler_UnlockVersion(t *testing.T) {
	handler := NewS3Handler(nil)

	comp := &VersionedComponent{
		URL:      "s3://my-bucket/object.zip",
		Version:  "abc123def456",
		IsLocked: true,
		Type:     ComponentTypeS3,
		Metadata: make(map[string]interface{}),
	}
	comp.Metadata["is_version_id"] = true

	err := handler.UnlockVersion(comp)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if comp.Version != "latest" {
		t.Errorf("Expected version 'latest', got '%s'", comp.Version)
	}

	if comp.IsLocked {
		t.Errorf("Expected IsLocked to be false")
	}

	if comp.OldVersion != "abc123def456" {
		t.Errorf("Expected OldVersion 'abc123def456', got '%s'", comp.OldVersion)
	}

	if isVersionID, ok := comp.Metadata["is_version_id"].(bool); !ok || isVersionID {
		t.Errorf("Expected is_version_id to be false")
	}
}

// TestS3Handler_SetS3Manager tests setting the S3 manager.
func TestS3Handler_SetS3Manager(t *testing.T) {
	handler := NewS3Handler(nil)

	if handler.s3Manager != nil {
		t.Errorf("Expected nil S3 manager initially")
	}

	s3Manager, err := aws.NewS3Manager("test-profile", "us-east-1")
	if err != nil {
		t.Skipf("Skipping test - cannot initialize S3 manager: %v", err)
		return
	}
	handler.SetS3Manager(s3Manager)

	if handler.s3Manager == nil {
		t.Errorf("Expected S3 manager to be set")
	}
}

// TestS3Handler_Registration tests that the handler is registered.
func TestS3Handler_Registration(t *testing.T) {
	handler, err := GetComponentHandler(ComponentTypeS3)
	if err != nil {
		t.Fatalf("S3 handler not registered: %v", err)
	}

	if handler.Type() != ComponentTypeS3 {
		t.Errorf("Expected type %s, got %s", ComponentTypeS3, handler.Type())
	}
}

// TestS3Handler_LockVersion tests locking an S3 component.
// This test skips actual S3 operations as it requires AWS credentials.
func TestS3Handler_LockVersion(t *testing.T) {
	handler := NewS3Handler(nil)

	comp := &VersionedComponent{
		URL:      "s3://my-bucket/object.zip",
		Version:  "abc123def456",
		Type:     ComponentTypeS3,
		Metadata: make(map[string]interface{}),
	}

	// Test locking without S3 manager (should fail for "latest")
	comp.Version = "latest"
	err := handler.LockVersion(comp)
	if err == nil {
		t.Errorf("Expected error when locking 'latest' without S3 manager")
	}

	// Test locking with existing version ID (should succeed without S3 manager)
	comp.Version = "abc123def456ghi"
	comp.IsLocked = false
	err = handler.LockVersion(comp)
	if err != nil {
		t.Errorf("Unexpected error when locking with existing version ID: %v", err)
	}
	if !comp.IsLocked {
		t.Errorf("Expected component to be locked")
	}
}

// TestS3Handler_ResolveVersion tests version resolution.
func TestS3Handler_ResolveVersion(t *testing.T) {
	handler := NewS3Handler(nil)

	// Test resolving existing version ID (no S3 manager needed)
	comp := &VersionedComponent{
		URL:      "s3://my-bucket/object.zip",
		Version:  "abc123def456ghi",
		Type:     ComponentTypeS3,
		Metadata: make(map[string]interface{}),
	}

	version, err := handler.ResolveVersion(comp, &ResolveContext{})
	if err != nil {
		t.Errorf("Unexpected error resolving existing version: %v", err)
	}
	if version != "abc123def456ghi" {
		t.Errorf("Expected version 'abc123def456ghi', got '%s'", version)
	}

	// Test resolving "latest" without S3 manager (should fail)
	comp.Version = "latest"
	_, err = handler.ResolveVersion(comp, &ResolveContext{})
	if err == nil {
		t.Errorf("Expected error when resolving 'latest' without S3 manager")
	}
}
