package desiredstate

import (
	"testing"
)

// TestGitHandler_WithGitClient tests GitHandler with a real GitClient integration.
// This test uses a public GitHub repository to verify real Git resolution.
func TestGitHandler_WithGitClient(t *testing.T) {
	handler := NewGitHandler()
	gitClient := NewGitClient()
	handler.SetGitClient(gitClient)

	tests := []struct {
		name        string
		url         string
		branch      string
		tag         string
		expectError bool
		description string
	}{
		{
			name:        "resolve main branch",
			url:         "https://github.com/git/git.git",
			branch:      "main",
			expectError: false,
			description: "Should resolve main branch to a commit SHA",
		},
		{
			name:        "resolve master branch",
			url:         "https://github.com/git/git.git",
			branch:      "master",
			expectError: false,
			description: "Should resolve master branch to a commit SHA",
		},
		{
			name:        "resolve tag",
			url:         "https://github.com/git/git.git",
			tag:         "v2.40.0",
			expectError: false,
			description: "Should resolve tag to a commit SHA",
		},
		{
			name:        "invalid branch",
			url:         "https://github.com/git/git.git",
			branch:      "this-branch-does-not-exist-12345",
			expectError: true,
			description: "Should fail for non-existent branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip if we can't reach the network
			t.Skip("Skipping Git integration test - requires network access")

			// Create a test component
			comp := &VersionedComponent{
				URL:    tt.url,
				Branch: tt.branch,
				Tag:    tt.tag,
				Type:   ComponentTypeSourcecode,
			}

			if tt.branch != "" {
				comp.Version = tt.branch
			} else if tt.tag != "" {
				comp.Version = tt.tag
			}

			// Test ResolveVersion
			resolved, err := handler.ResolveVersion(comp, nil)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify we got a commit SHA
			if !handler.isCommitSHA(resolved) {
				t.Errorf("Expected commit SHA but got: %s", resolved)
			}

			t.Logf("✓ %s - Resolved to commit: %s", tt.description, resolved[:8])
		})
	}
}

// TestGitHandler_LockWithGitClient tests locking with GitClient integration.
func TestGitHandler_LockWithGitClient(t *testing.T) {
	handler := NewGitHandler()
	gitClient := NewGitClient()
	handler.SetGitClient(gitClient)

	t.Run("lock branch to commit SHA", func(t *testing.T) {
		t.Skip("Skipping Git integration test - requires network access")

		comp := &VersionedComponent{
			URL:      "https://github.com/git/git.git",
			Branch:   "master",
			Version:  "master",
			Type:     ComponentTypeSourcecode,
			IsLocked: false,
		}

		err := handler.LockVersion(comp)
		if err != nil {
			t.Fatalf("Failed to lock version: %v", err)
		}

		// Verify the version was changed to a commit SHA
		if !handler.isCommitSHA(comp.Version) {
			t.Errorf("Expected version to be commit SHA after lock, got: %s", comp.Version)
		}

		// Verify it's marked as locked
		if !comp.IsLocked {
			t.Errorf("Expected component to be locked")
		}

		// Verify OldVersion was preserved
		if comp.OldVersion != "master" {
			t.Errorf("Expected OldVersion to be 'master', got: %s", comp.OldVersion)
		}

		t.Logf("✓ Locked master branch to commit: %s", comp.Version[:8])
	})

	t.Run("lock tag to commit SHA", func(t *testing.T) {
		t.Skip("Skipping Git integration test - requires network access")

		comp := &VersionedComponent{
			URL:      "https://github.com/git/git.git",
			Tag:      "v2.40.0",
			Version:  "v2.40.0",
			Type:     ComponentTypeSourcecode,
			IsLocked: false,
		}

		err := handler.LockVersion(comp)
		if err != nil {
			t.Fatalf("Failed to lock version: %v", err)
		}

		// Verify the version was changed to a commit SHA
		if !handler.isCommitSHA(comp.Version) {
			t.Errorf("Expected version to be commit SHA after lock, got: %s", comp.Version)
		}

		if !comp.IsLocked {
			t.Errorf("Expected component to be locked")
		}

		t.Logf("✓ Locked tag v2.40.0 to commit: %s", comp.Version[:8])
	})
}

// TestGitHandler_FallbackWithoutGitClient tests that handler gracefully degrades without GitClient.
func TestGitHandler_FallbackWithoutGitClient(t *testing.T) {
	handler := NewGitHandler()
	// Intentionally NOT setting GitClient

	comp := &VersionedComponent{
		URL:     "https://github.com/user/repo.git",
		Branch:  "develop",
		Version: "develop",
		Type:    ComponentTypeSourcecode,
	}

	t.Run("ResolveVersion without GitClient", func(t *testing.T) {
		resolved, err := handler.ResolveVersion(comp, nil)
		if err != nil {
			t.Errorf("Should not error without GitClient, got: %v", err)
		}

		// Should return the branch name as-is
		if resolved != "develop" {
			t.Errorf("Expected 'develop' but got: %s", resolved)
		}

		t.Logf("✓ Fallback mode returned ref: %s", resolved)
	})

	t.Run("LockVersion without GitClient", func(t *testing.T) {
		err := handler.LockVersion(comp)
		if err != nil {
			t.Errorf("Should not error without GitClient, got: %v", err)
		}

		// Should still mark as locked even without resolving
		if !comp.IsLocked {
			t.Errorf("Expected component to be locked")
		}

		// Version should still be the branch name (not resolved)
		if comp.Version != "develop" {
			t.Errorf("Expected version to remain 'develop', got: %s", comp.Version)
		}

		t.Logf("✓ Fallback mode locked without resolution: %s", comp.Version)
	})
}

// TestComponentManager_SetGitClient tests the propagation of GitClient through ComponentManager.
func TestComponentManager_SetGitClient(t *testing.T) {
	// Create a component manager
	cm := NewComponentManager()

	// Create a GitClient
	gitClient := NewGitClient()

	// Set the GitClient (should propagate to Git handler)
	cm.SetGitClient(gitClient)

	// Verify the Git handler received the GitClient
	handler, err := GetComponentHandler(ComponentTypeSourcecode)
	if err != nil {
		t.Fatalf("Failed to get Git handler: %v", err)
	}

	gitHandler, ok := handler.(*GitHandler)
	if !ok {
		t.Fatalf("Handler is not a GitHandler")
	}

	if gitHandler.gitClient == nil {
		t.Errorf("GitClient was not set on Git handler")
	}

	t.Logf("✓ GitClient successfully propagated to Git handler")
}

// TestGitHandler_AlreadyCommitSHA verifies handler recognizes existing commit SHAs.
func TestGitHandler_AlreadyCommitSHA(t *testing.T) {
	handler := NewGitHandler()
	gitClient := NewGitClient()
	handler.SetGitClient(gitClient)

	comp := &VersionedComponent{
		URL:     "https://github.com/user/repo.git",
		Version: "abc123def456abc123def456abc123def456abc1",
		Type:    ComponentTypeSourcecode,
	}

	t.Run("ResolveVersion with existing SHA", func(t *testing.T) {
		resolved, err := handler.ResolveVersion(comp, nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Should return the same SHA without trying to resolve
		if resolved != comp.Version {
			t.Errorf("Expected SHA to be returned unchanged, got: %s", resolved)
		}

		t.Logf("✓ Existing commit SHA returned unchanged: %s", resolved[:8])
	})

	t.Run("LockVersion with existing SHA", func(t *testing.T) {
		err := handler.LockVersion(comp)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Should just mark as locked without changing version
		if !comp.IsLocked {
			t.Errorf("Expected component to be locked")
		}

		if comp.Version != "abc123def456abc123def456abc123def456abc1" {
			t.Errorf("Expected version to remain unchanged, got: %s", comp.Version)
		}

		t.Logf("✓ Existing commit SHA locked without resolution")
	})
}
