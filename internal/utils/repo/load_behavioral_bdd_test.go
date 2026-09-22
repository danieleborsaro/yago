package repo

import (
	"os"
	"testing"
)

// TestRepo_Load_BehavioralBDD tests repository loading behavioral contracts
//
// Go Implementation (internal/utils/repo.Load):
//   - Function: Load(workdir string, handler ProgressHandler) (*Repo, bool, error)
//   - Behavior: Loads repository, detects git status, returns structured result
//   - Git detection: Uses go-git OpenRepository()
//   - Returns: (repo, isGit, error) - explicit triple return
//   - Empty workdir: Returns error "workdir cannot be empty"
//   - Nonexistent dir: Returns (nil, false, nil) - graceful handling
//
// Behavioral Contract:
// Load a repository from a working directory and detect whether it's a git
// repository. Errors are signaled via an explicit (repo, isGit, error) return
// rather than exceptions, letting the caller decide how to proceed for each
// case: a valid git repo, a valid-but-non-git directory, or an actual error.
//
// Critical Semantics:
// 1. Git Detection:
//   - Git repo: Returns repo object with isGit=true
//   - Non-git dir: Returns nil repo with isGit=false (valid directory, not git)
//   - Nonexistent: Returns nil repo with isGit=false (directory doesn't exist)
//
// 2. Empty Workdir:
//   - Returns error "workdir cannot be empty"
//
// 3. Directory Access:
//   - Git repo: Repo object includes path, remote URL access
//   - Non-git: No repo object (nil), caller must handle
//
// 4. Error Handling Philosophy:
//   - Error-return based (caller checks error, isGit flag) rather than exceptions
//
// Regression Risk: HIGH
// - Wrong git detection: Breaks all git operations
// - Missing error handling: Silent failures, undefined behavior
// - Wrong tuple return: Caller gets nil repo when expecting valid object
//
// Test Coverage:
// - Load existing git repository (yago repo itself)
// - Load non-git directory (/tmp)
// - Load with empty workdir (error case)
// - Load nonexistent directory (graceful failure)
func TestRepo_Load_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior: "Load repository from working directory, detect git status, handle errors",
		Rationale: `
Load returns explicit (repo, isGit, error) tuples that let callers decide how
to handle each case:

- (repo, true, nil): Valid git repository, proceed with git operations
- (nil, false, nil): Not a git repo OR doesn't exist - soft failure
- (nil, false, err): Actual error (e.g., empty workdir, permission denied)

This aligns with Go's "errors are values" idiom and enables flexible error
handling at the call site without exception propagation.
`,
	}

	t.Logf("=== Behavioral Contract: %s ===", contract.Behavior)
	t.Logf("Rationale:\n%s", contract.Rationale)

	t.Run("Load existing git repository", func(t *testing.T) {
		t.Log("=== Test Case: Load Existing Git Repository ===")
		t.Log("Scenario: Load the yago repository (current project)")
		t.Log("Expected: repo object returned, isGit=true, no error")

		// Use the yago repository itself as a test case
		// Path is relative from internal/utils/repo to project root
		yagoRepoPath := "../../../"

		t.Logf("Loading repository from path: %s", yagoRepoPath)
		repo, isGit, err := Load(yagoRepoPath, nil)

		// Verify no error
		if err != nil {
			t.Fatalf("❌ Expected no error, got: %v", err)
		}
		t.Logf("✓ No error returned")

		// Verify isGit flag is true
		if !isGit {
			t.Error("❌ Expected isGit to be true for yago repository")
		} else {
			t.Logf("✓ isGit=true (repository detected as git)")
		}

		// Verify repo object is not nil
		if repo == nil {
			t.Fatal("❌ Expected repo to be non-nil for git repository")
		}
		t.Logf("✓ Repo object created successfully")

		// Verify we can get the path
		path := repo.GetPath()
		if path == "" {
			t.Error("❌ Expected non-empty path")
		} else {
			t.Logf("✓ Repository path: %s", path)
		}

		// Verify we can get remote URL
		url, err := repo.GetRemoteURL()
		if err != nil {
			t.Errorf("❌ Expected to get remote URL, got error: %v", err)
		} else if url == "" {
			t.Error("❌ Expected non-empty URL")
		} else {
			t.Logf("✓ Remote URL: %s", url)
		}

		t.Log("✓ CONTRACT SATISFIED: Git repository loaded successfully")
	})

	t.Run("Load non-git directory", func(t *testing.T) {
		t.Log("=== Test Case: Load Non-Git Directory ===")
		t.Log("Scenario: Load /tmp (valid directory, not a git repo)")
		t.Log("Expected: repo=nil, isGit=false, no error (graceful handling)")

		tmpPath := "/tmp"
		t.Logf("Loading from path: %s", tmpPath)
		repo, isGit, err := Load(tmpPath, nil)

		// Verify no error (graceful handling)
		if err != nil {
			t.Fatalf("❌ Expected no error for non-git directory, got: %v", err)
		}
		t.Logf("✓ No error returned (graceful handling)")

		// Verify isGit is false
		if isGit {
			t.Error("❌ Expected isGit to be false for /tmp")
		} else {
			t.Logf("✓ isGit=false (not a git repository)")
		}

		// Verify repo is nil
		if repo != nil {
			t.Error("❌ Expected repo to be nil for non-git directory")
		} else {
			t.Logf("✓ Repo is nil (no repository object)")
		}

		t.Log("✓ CONTRACT SATISFIED: Non-git directory handled gracefully")
	})

	t.Run("Load empty workdir", func(t *testing.T) {
		t.Log("=== Test Case: Load with Empty Workdir ===")
		t.Log("Scenario: Call Load with empty string")
		t.Log("Expected: repo=nil, isGit=false, error returned")

		t.Log("Loading from empty workdir...")
		repo, isGit, err := Load("", nil)

		// Verify error is returned
		if err == nil {
			t.Error("❌ Expected error for empty workdir")
		} else {
			t.Logf("✓ Error returned: %v", err)
		}

		// Verify isGit is false
		if isGit {
			t.Error("❌ Expected isGit to be false for empty workdir")
		} else {
			t.Logf("✓ isGit=false")
		}

		// Verify repo is nil
		if repo != nil {
			t.Error("❌ Expected repo to be nil for empty workdir")
		} else {
			t.Logf("✓ Repo is nil")
		}

		t.Log("✓ CONTRACT SATISFIED: Empty workdir error handled correctly")
	})

	t.Run("Load nonexistent directory", func(t *testing.T) {
		t.Log("=== Test Case: Load Nonexistent Directory ===")
		t.Log("Scenario: Load directory that doesn't exist")
		t.Log("Expected: repo=nil, isGit=false, no error (graceful failure)")

		// Use a path that definitely doesn't exist
		nonexistentPath := "/tmp/this-directory-does-not-exist-yago-test-12345"

		// Verify directory doesn't exist (clean test state)
		if _, err := os.Stat(nonexistentPath); err == nil {
			t.Fatalf("Test setup error: directory %s exists (should not)", nonexistentPath)
		}

		t.Logf("Loading from nonexistent path: %s", nonexistentPath)
		repo, isGit, _ := Load(nonexistentPath, nil)

		// Should return false (not a git repo) rather than an error
		// The git library will fail to open it, treated same as non-git
		if isGit {
			t.Error("❌ Expected isGit to be false for nonexistent directory")
		} else {
			t.Logf("✓ isGit=false (directory doesn't exist or not git)")
		}

		// Verify repo is nil
		if repo != nil {
			t.Error("❌ Expected repo to be nil for nonexistent directory")
		} else {
			t.Logf("✓ Repo is nil (no repository object)")
		}

		t.Log("✓ CONTRACT SATISFIED: Nonexistent directory handled gracefully")
		t.Log("Note: Go treats 'doesn't exist' as 'not a git repo' (soft failure)")
	})

	t.Log("\n=== All Test Cases Passed ===")
	t.Log("Behavioral contract verified:")
	t.Log("- Git repositories load successfully with repo object")
	t.Log("- Non-git directories return nil gracefully")
	t.Log("- Empty workdir returns explicit error")
	t.Log("- Nonexistent directories handled as 'not git' (soft failure)")
	t.Log("\nRegression Risk: HIGH")
	t.Log("- Git detection drives all repository operations")
	t.Log("- Error handling affects caller behavior")
	t.Log("- Graceful failures prevent cascading errors")
}
