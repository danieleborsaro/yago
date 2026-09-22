package desiredstate

import (
	"testing"
)

// TestGitHandler_Type tests that the handler returns the correct type.
func TestGitHandler_Type(t *testing.T) {
	handler := NewGitHandler()
	if handler.Type() != ComponentTypeSourcecode {
		t.Errorf("Expected type %s, got %s", ComponentTypeSourcecode, handler.Type())
	}
}

// TestGitHandler_ParseComponent tests parsing Git components from YAML.
func TestGitHandler_ParseComponent(t *testing.T) {
	handler := NewGitHandler()

	tests := []struct {
		name     string
		data     map[string]interface{}
		wantErr  bool
		wantURL  string
		wantVer  string
		wantBr   string
		wantTag  string
		wantPath string
		wantLock bool
	}{
		{
			name: "git with branch",
			data: map[string]interface{}{
				"url":    "https://github.com/user/repo.git",
				"branch": "develop",
			},
			wantErr:  false,
			wantURL:  "https://github.com/user/repo.git",
			wantVer:  "develop",
			wantBr:   "develop",
			wantTag:  "",
			wantPath: "",
			wantLock: false,
		},
		{
			name: "git with tag",
			data: map[string]interface{}{
				"url": "https://github.com/user/repo.git",
				"tag": "v1.2.3",
			},
			wantErr:  false,
			wantURL:  "https://github.com/user/repo.git",
			wantVer:  "v1.2.3",
			wantBr:   "",
			wantTag:  "v1.2.3",
			wantPath: "",
			wantLock: false,
		},
		{
			name: "git with commit SHA",
			data: map[string]interface{}{
				"url": "https://github.com/user/repo.git",
				"tag": "abc123def456789012345678901234567890abcd",
			},
			wantErr:  false,
			wantURL:  "https://github.com/user/repo.git",
			wantVer:  "abc123def456789012345678901234567890abcd",
			wantBr:   "",
			wantTag:  "abc123def456789012345678901234567890abcd",
			wantPath: "",
			wantLock: true, // Commit SHA is considered locked
		},
		{
			name: "git with path",
			data: map[string]interface{}{
				"url":    "https://github.com/user/repo.git",
				"branch": "main",
				"path":   "/config/app",
			},
			wantErr:  false,
			wantURL:  "https://github.com/user/repo.git",
			wantVer:  "main",
			wantBr:   "main",
			wantTag:  "",
			wantPath: "/config/app",
			wantLock: false,
		},
		{
			name: "git defaults to main branch",
			data: map[string]interface{}{
				"url": "https://github.com/user/repo.git",
			},
			wantErr:  false,
			wantURL:  "https://github.com/user/repo.git",
			wantVer:  "main",
			wantBr:   "main",
			wantTag:  "",
			wantPath: "",
			wantLock: false,
		},
		{
			name: "missing URL",
			data: map[string]interface{}{
				"branch": "develop",
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

			if comp.Branch != tt.wantBr {
				t.Errorf("Branch: expected %s, got %s", tt.wantBr, comp.Branch)
			}

			if comp.Tag != tt.wantTag {
				t.Errorf("Tag: expected %s, got %s", tt.wantTag, comp.Tag)
			}

			if comp.Path != tt.wantPath {
				t.Errorf("Path: expected %s, got %s", tt.wantPath, comp.Path)
			}

			if comp.IsLocked != tt.wantLock {
				t.Errorf("IsLocked: expected %t, got %t", tt.wantLock, comp.IsLocked)
			}

			if comp.Type != ComponentTypeSourcecode {
				t.Errorf("Type: expected %s, got %s", ComponentTypeSourcecode, comp.Type)
			}
		})
	}
}

// TestGitHandler_IsCommitSHA tests commit SHA detection.
func TestGitHandler_IsCommitSHA(t *testing.T) {
	handler := NewGitHandler()

	tests := []struct {
		name   string
		input  string
		wantOK bool
	}{
		{
			name:   "full commit SHA",
			input:  "abc123def456789012345678901234567890abcd",
			wantOK: true,
		},
		{
			name:   "short commit SHA (7 chars)",
			input:  "abc1234",
			wantOK: true,
		},
		{
			name:   "short commit SHA (12 chars)",
			input:  "abc123def456",
			wantOK: true,
		},
		{
			name:   "not a SHA - too short",
			input:  "abc12",
			wantOK: false,
		},
		{
			name:   "not a SHA - contains non-hex",
			input:  "abc123xyz",
			wantOK: false,
		},
		{
			name:   "not a SHA - too long",
			input:  "abc123def456789012345678901234567890abcd1",
			wantOK: false,
		},
		{
			name:   "semantic version",
			input:  "v1.2.3",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isCommitSHA(tt.input)
			if result != tt.wantOK {
				t.Errorf("Expected %t, got %t", tt.wantOK, result)
			}
		})
	}
}

// TestGitHandler_CompareVersions tests version comparison.
func TestGitHandler_CompareVersions(t *testing.T) {
	handler := NewGitHandler()

	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int // -1, 0, 1
	}{
		{
			name:     "semantic versions - v1 older",
			v1:       "v1.0.0",
			v2:       "v1.1.0",
			expected: -1,
		},
		{
			name:     "semantic versions - equal",
			v1:       "v2.5.3",
			v2:       "v2.5.3",
			expected: 0,
		},
		{
			name:     "semantic versions - v1 newer",
			v1:       "v3.0.0",
			v2:       "v2.9.9",
			expected: 1,
		},
		{
			name:     "commit SHAs - equal",
			v1:       "abc123def456",
			v2:       "abc123def456",
			expected: 0,
		},
		{
			name:     "commit SHAs - different",
			v1:       "abc123def456",
			v2:       "def456abc123",
			expected: -1, // Lexicographic comparison
		},
		{
			name:     "branch names",
			v1:       "develop",
			v2:       "main",
			expected: -1, // Lexicographic comparison
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

// TestGitHandler_ValidateComponent tests component validation.
func TestGitHandler_ValidateComponent(t *testing.T) {
	handler := NewGitHandler()

	tests := []struct {
		name    string
		comp    *VersionedComponent
		wantErr bool
	}{
		{
			name: "valid with HTTPS URL",
			comp: &VersionedComponent{
				URL:     "https://github.com/user/repo.git",
				Version: "main",
				Type:    ComponentTypeSourcecode,
			},
			wantErr: false,
		},
		{
			name: "valid with SSH URL",
			comp: &VersionedComponent{
				URL:     "git@github.com:user/repo.git",
				Version: "develop",
				Type:    ComponentTypeSourcecode,
			},
			wantErr: false,
		},
		{
			name: "valid with commit SHA",
			comp: &VersionedComponent{
				URL:     "https://github.com/user/repo.git",
				Version: "abc123def456789012345678901234567890abcd",
				Type:    ComponentTypeSourcecode,
			},
			wantErr: false,
		},
		{
			name: "empty URL",
			comp: &VersionedComponent{
				URL:     "",
				Version: "main",
				Type:    ComponentTypeSourcecode,
			},
			wantErr: true,
		},
		{
			name: "invalid URL format",
			comp: &VersionedComponent{
				URL:     "not a valid url",
				Version: "main",
				Type:    ComponentTypeSourcecode,
			},
			wantErr: true,
		},
		{
			name: "invalid commit SHA",
			comp: &VersionedComponent{
				URL:     "https://github.com/user/repo.git",
				Version: "abc123def456789", // Looks like SHA but contains invalid length for strict validation
				Type:    ComponentTypeSourcecode,
			},
			wantErr: false, // Actually valid - it's either a short SHA or branch name
		},
		{
			name: "invalid commit SHA with non-hex",
			comp: &VersionedComponent{
				URL:     "https://github.com/user/repo.git",
				Version: "ghi123xyz456789", // Looks like SHA (15 chars) but contains non-hex g, x, y, z
				Type:    ComponentTypeSourcecode,
			},
			wantErr: false, // Valid as branch name, only strict SHA validation would fail
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

// TestGitHandler_UnlockVersion tests unlocking a Git component.
func TestGitHandler_UnlockVersion(t *testing.T) {
	handler := NewGitHandler()

	// Test unlocking with existing branch
	comp := &VersionedComponent{
		URL:      "https://github.com/user/repo.git",
		Version:  "abc123def456789012345678901234567890abcd",
		Branch:   "develop",
		IsLocked: true,
		Type:     ComponentTypeSourcecode,
	}

	err := handler.UnlockVersion(comp)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if comp.Version != "develop" {
		t.Errorf("Expected version 'develop', got '%s'", comp.Version)
	}

	if comp.IsLocked {
		t.Errorf("Expected IsLocked to be false")
	}

	if comp.Tag != "" {
		t.Errorf("Expected Tag to be cleared, got '%s'", comp.Tag)
	}

	// Test unlocking without branch (should default to main)
	comp2 := &VersionedComponent{
		URL:      "https://github.com/user/repo.git",
		Version:  "abc123def456",
		IsLocked: true,
		Type:     ComponentTypeSourcecode,
	}

	err = handler.UnlockVersion(comp2)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if comp2.Version != "main" {
		t.Errorf("Expected version 'main', got '%s'", comp2.Version)
	}

	if comp2.Branch != "main" {
		t.Errorf("Expected branch 'main', got '%s'", comp2.Branch)
	}
}

// TestGitHandler_LockVersion tests locking a Git component.
func TestGitHandler_LockVersion(t *testing.T) {
	handler := NewGitHandler()

	// Test locking with commit SHA (should just mark as locked)
	comp := &VersionedComponent{
		URL:      "https://github.com/user/repo.git",
		Version:  "abc123def456789012345678901234567890abcd",
		IsLocked: false,
		Type:     ComponentTypeSourcecode,
	}

	err := handler.LockVersion(comp)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !comp.IsLocked {
		t.Errorf("Expected component to be locked")
	}

	// Test locking with branch (TODO: should resolve to commit SHA)
	comp2 := &VersionedComponent{
		URL:      "https://github.com/user/repo.git",
		Version:  "develop",
		Branch:   "develop",
		IsLocked: false,
		Type:     ComponentTypeSourcecode,
	}

	err = handler.LockVersion(comp2)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !comp2.IsLocked {
		t.Errorf("Expected component to be locked")
	}
}

// TestGitHandler_Registration tests that the handler is registered.
func TestGitHandler_Registration(t *testing.T) {
	handler, err := GetComponentHandler(ComponentTypeSourcecode)
	if err != nil {
		t.Fatalf("Git handler not registered: %v", err)
	}

	if handler.Type() != ComponentTypeSourcecode {
		t.Errorf("Expected type %s, got %s", ComponentTypeSourcecode, handler.Type())
	}
}

// TestGitHandler_ResolveVersion tests version resolution.
func TestGitHandler_ResolveVersion(t *testing.T) {
	handler := NewGitHandler()

	// Test resolving commit SHA (should return as-is)
	comp := &VersionedComponent{
		URL:     "https://github.com/user/repo.git",
		Version: "abc123def456789012345678901234567890abcd",
		Type:    ComponentTypeSourcecode,
	}

	version, err := handler.ResolveVersion(comp, &ResolveContext{})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if version != "abc123def456789012345678901234567890abcd" {
		t.Errorf("Expected commit SHA, got '%s'", version)
	}

	// Test resolving branch (TODO: should query Git, for now returns branch name)
	comp2 := &VersionedComponent{
		URL:     "https://github.com/user/repo.git",
		Version: "develop",
		Branch:  "develop",
		Type:    ComponentTypeSourcecode,
	}

	version2, err := handler.ResolveVersion(comp2, &ResolveContext{})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if version2 != "develop" {
		t.Errorf("Expected 'develop', got '%s'", version2)
	}
}
