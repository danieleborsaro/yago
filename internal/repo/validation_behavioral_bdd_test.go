package repo

import (
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	uRepo "github.com/danieleborsaro/yago/internal/utils/repo"
)

// =============================================================================
// TIME-BASED BDD: Repository Validation Behavioral Contracts
// =============================================================================
// These tests document the CURRENT behavior of repository validation at time T.
// They serve as:
// 1. Characterization tests (Michael Feathers, "Working Effectively with Legacy Code")
// 2. Regression detection for refactoring safety
// 3. Behavioral specification derived from working code
//
// Pattern: GoBehavioralContract
// Migrated from: validation_test.go (239 lines, 3 test functions)
// Migration Date: October 13, 2025
// =============================================================================

// =============================================================================
// PHASE 1: REMOTE URL VALIDATION
// =============================================================================

func TestValidateRemote_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "Repo.ValidateRemote() verifies remote repository accessibility (network operation, checks if remote URL exists and is reachable)",

		CurrentImpl: `
Go: internal/repo/git.go (ValidateRemote method)

func (r *Repo) ValidateRemote() error {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // Check URL not empty
    if r.URL == "" {
        return fmt.Errorf("URL is empty")
    }

    // Parse URL for authentication
    r.parseURL()

    // Perform remote validation via ls-remote
    // This checks if the repository exists and is accessible
    err := r.repository.ValidateRemote(r.URL, r.auth)
    if err != nil {
        return fmt.Errorf("remote validation failed: %w", err)
    }

    return nil
}

Key features:
- Network operation: Contacts remote Git server
- URL validation: Empty URL rejected
- Authentication: Uses parsed credentials
- ls-remote: Git command to check remote without cloning
- Accessibility check: Verifies repo exists and is reachable
- Error context: Wraps errors with context

Validation checks:
- URL not empty
- URL format valid (http/https/git/ssh)
- Remote repository exists
- Authentication succeeds (if required)
- Network reachable

ls-remote operation:
- Lightweight: Doesn't clone, just queries remote
- Auth-aware: Uses credentials if private repo
- Fast: Only fetches ref information
- Standard: Works with GitHub, GitLab, Bitbucket, etc.
`,

		ExpectedOutcome: `
Valid public repository:
- MUST return nil (no error)
- MUST succeed for accessible repos
- SHOULD complete quickly (network dependent)

Empty URL:
- MUST return error
- MUST error message contain "URL is empty"
- MUST NOT attempt network operation

Invalid/non-existent repository:
- MUST return error
- MUST error message contain "not found" or similar
- MUST indicate remote validation failure

Authentication errors:
- MUST return error for private repos without auth
- MUST error indicate authentication failure
- SHOULD preserve error context

Network considerations:
- MAY timeout on network issues
- MAY fail in offline environments
- SHOULD be skipped in short tests (testing.Short())
`,

		TestScenario: `
Scenarios tested:

1. Valid public GitHub repository:
   GIVEN: https://github.com/octocat/Hello-World.git
   WHEN: ValidateRemote()
   THEN: Returns nil (success)

2. Empty URL:
   GIVEN: URL = ""
   WHEN: ValidateRemote()
   THEN: Returns error "URL is empty"

3. Non-existent repository:
   GIVEN: https://github.com/invalid-org-99999/non-existent-repo-99999.git
   WHEN: ValidateRemote()
   THEN: Returns error containing "not found"

Test implementation:
- Table-driven tests
- Network test (skipped in short mode)
- Tests actual remote repositories
- Error message validation
`,

		Rationale: `
Why this behavior exists:
- Pre-flight check: Validate before cloning (save time/bandwidth)
- User feedback: Early error messages for invalid configs
- Configuration validation: Catch typos in URLs
- Authentication check: Verify credentials before clone
- Resource efficiency: Don't clone if repo doesn't exist

Network operation rationale:
- Accuracy: Only way to verify repo truly exists
- ls-remote: Lightweight compared to clone
- Real-world check: Confirms URL, auth, network all working
- Fail fast: Catch issues before expensive clone

Empty URL check:
- Common mistake: Missing URL in configuration
- Clear error: "URL is empty" is obvious
- Fail fast: No network call needed
- Prevents: ls-remote from failing with cryptic error

Use cases:
- CI/CD validation: Validate configs before deployment
- User input: Check user-provided repository URLs
- GitOps: Validate desired state repo references
- Pre-clone: Ensure clone will succeed
- Configuration: Validate YAML/JSON config files

Why ls-remote:
- No clone: Don't download entire repository
- Fast: Only fetches ref information (KB, not MB/GB)
- Standard: Works with all Git hosting providers
- Auth-aware: Uses credentials for private repos
- Reliable: Standard Git operation
`,

		RegressionRisk: `
HIGH RISK if changed:
- Network dependency: Real operation, not mock
- Error messages: Callers may parse "URL is empty", "not found"
- ls-remote: Removing breaks pre-flight validation
- Authentication: Must use credentials for private repos

MEDIUM RISK:
- Timeout handling: Long timeout breaks UX
- Empty URL check: Removing makes error less clear
- Error wrapping: Context helps debugging

LOW RISK:
- Logging: Can add more observability
- URL parsing: Internal detail
- Progress reporting: Optional feature

What breaks if this changes:
1. Remove network check → can't verify repo exists
2. Change error messages → error parsing breaks
3. Remove empty URL check → worse error message
4. Don't use auth → private repos fail
5. Remove error context → debugging harder
6. Long timeout → UX degradation
7. Skip validation → clone fails later (worse UX)
`,
	}

	// Skip network tests in CI/CD environments
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Non-existent/private-looking repos make GitHub prompt for https credentials; disable that here.
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	// Execute the behavioral tests
	logger := logging.NewLogger(logging.DEBUG)
	config := &uRepo.RepoConfig{
		Logger:          logger,
		ProgressHandler: uRepo.NewLogProgressHandler(logger),
	}

	tests := []struct {
		name        string
		url         string
		shouldError bool
		errorSubstr string
	}{
		{
			name:        "valid public github repo",
			url:         "https://github.com/octocat/Hello-World.git",
			shouldError: false,
		},
		{
			name:        "empty URL",
			url:         "",
			shouldError: true,
			errorSubstr: "URL is empty",
		},
		{
			name:        "invalid URL - non-existent repo",
			url:         "https://github.com/invalid-org-99999/non-existent-repo-99999.git",
			shouldError: true,
			errorSubstr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repo{
				URL:    tt.url,
				config: config,
			}

			err := repo.ValidateRemote()

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorSubstr, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 2: REF VALIDATION
// =============================================================================

func TestValidateRef_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "Repo.ValidateRef() verifies branch/tag/commit reference exists in remote repository (network operation, empty ref is valid)",

		CurrentImpl: `
Go: internal/repo/git.go (ValidateRef method)

func (r *Repo) ValidateRef() error {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // Empty ref is valid (use default branch)
    if r.Ref == "" {
        return nil
    }

    // Check URL first
    if r.URL == "" {
        return fmt.Errorf("URL is empty")
    }

    // Parse URL for authentication
    r.parseURL()

    // Validate ref exists in remote
    err := r.repository.ValidateRef(r.URL, r.Ref, r.auth)
    if err != nil {
        return fmt.Errorf("ref validation failed for '%s': %w", r.Ref, err)
    }

    return nil
}

Key features:
- Network operation: Queries remote for ref
- Empty ref allowed: Returns nil (use default)
- URL required: Validates URL before ref
- ls-remote: Checks if ref exists remotely
- Branch/tag/commit: Validates any ref type
- Error context: Includes ref name in error

Ref validation:
- Branches: refs/heads/main, refs/heads/develop
- Tags: refs/tags/v1.0.0, refs/tags/release
- Commits: Full SHA or short SHA
- Empty: Valid (use repo default)

Empty ref semantics:
- Not an error: Empty means "use default"
- Common case: Users often omit branch
- Default branch: main, master, or repo default
- Flexibility: Don't force users to specify
`,

		ExpectedOutcome: `
Valid branch (master):
- MUST return nil (no error)
- MUST verify branch exists on remote
- MUST work for any existing branch

Empty ref:
- MUST return nil (no error)
- MUST NOT perform network check
- MUST treat as valid (use default branch)

Invalid branch:
- MUST return error
- MUST error message contain ref name
- MUST error indicate "not found" or similar

Empty URL with ref:
- MUST return error
- MUST error message contain "URL is empty"
- MUST NOT attempt network operation

Valid URL, invalid ref:
- MUST return error
- MUST error message contain ref name
- MUST indicate ref validation failure
`,

		TestScenario: `
Scenarios tested:

1. Valid branch - master:
   GIVEN: URL=octocat/Hello-World, Ref=master
   WHEN: ValidateRef()
   THEN: Returns nil (branch exists)

2. Empty ref - should be valid:
   GIVEN: URL=octocat/Hello-World, Ref=""
   WHEN: ValidateRef()
   THEN: Returns nil (use default)

3. Invalid branch:
   GIVEN: URL=octocat/Hello-World, Ref=non-existent-branch-99999
   WHEN: ValidateRef()
   THEN: Returns error with "not found"

4. Empty URL with ref:
   GIVEN: URL="", Ref=main
   WHEN: ValidateRef()
   THEN: Returns error "URL is empty"

Test implementation:
- Table-driven tests
- Network test (skipped in short mode)
- Multiple ref scenarios
- Error message validation
`,

		Rationale: `
Why this behavior exists:
- Pre-clone validation: Verify ref before clone
- User feedback: Clear error for typos in branch names
- CI/CD: Validate deployment branches
- GitOps: Validate desired state references
- Fail fast: Catch ref errors before clone

Empty ref rationale:
- Optional field: Branch is not always required
- Default branch: Use whatever repo defines
- Flexibility: Support various use cases
- Common pattern: Most tools allow omitting branch
- No error: Empty is valid, not an error

URL check first:
- Dependency: Can't validate ref without URL
- Clear error: "URL is empty" more useful than ls-remote error
- Fail fast: Don't attempt network with bad config
- Logical order: URL must exist to check ref

Network operation:
- Accuracy: Only way to verify ref exists
- ls-remote: Lists all refs, check if ours exists
- Real-world: Confirms ref actually available
- Auth-aware: Works with private repos
- Standard: Git standard operation

Use cases:
- Branch validation: Ensure deployment branch exists
- Tag validation: Verify release tags before deploy
- Commit validation: Check if SHA exists (rare)
- Configuration: Validate YAML/JSON ref fields
- User input: Validate user-provided branches
`,

		RegressionRisk: `
HIGH RISK if changed:
- Empty ref semantics: Must remain valid (no error)
- Network dependency: Real validation, not mock
- Error messages: Callers parse "not found", "URL is empty"
- ls-remote: Standard validation mechanism

MEDIUM RISK:
- URL check order: Must check URL before ref
- Error context: Ref name in error helps debugging
- Authentication: Must work with private repos

LOW RISK:
- Logging: Can add more observability
- Ref format: Accepts branches, tags, commits
- Timeout handling: Can improve

What breaks if this changes:
1. Error on empty ref → breaks valid use cases
2. Remove network check → can't verify ref exists
3. Change error messages → error parsing breaks
4. Skip URL check → worse error messages
5. Remove error context → harder to debug
6. Don't use auth → private repos fail
7. Require ref → breaks optional ref configs
`,
	}

	// Skip network tests in CI/CD environments
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Execute the behavioral tests
	logger := logging.NewLogger(logging.DEBUG)
	config := &uRepo.RepoConfig{
		Logger:          logger,
		ProgressHandler: uRepo.NewLogProgressHandler(logger),
	}

	tests := []struct {
		name        string
		url         string
		ref         string
		shouldError bool
		errorSubstr string
	}{
		{
			name:        "valid branch - master",
			url:         "https://github.com/octocat/Hello-World.git",
			ref:         "master",
			shouldError: false,
		},
		{
			name:        "empty ref - should be valid (uses default)",
			url:         "https://github.com/octocat/Hello-World.git",
			ref:         "",
			shouldError: false,
		},
		{
			name:        "invalid branch",
			url:         "https://github.com/octocat/Hello-World.git",
			ref:         "non-existent-branch-99999",
			shouldError: true,
			errorSubstr: "not found",
		},
		{
			name:        "empty URL with ref",
			url:         "",
			ref:         "main",
			shouldError: true,
			errorSubstr: "URL is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repo{
				URL:    tt.url,
				Ref:    tt.ref,
				config: config,
			}

			err := repo.ValidateRef()

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorSubstr, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 3: COMPLETE VALIDATION
// =============================================================================

func TestValidate_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "Repo.Validate() performs complete repository validation (combines ValidateRemote + ValidateRef for comprehensive pre-clone check)",

		CurrentImpl: `
Go: internal/repo/git.go (Validate method)

func (r *Repo) Validate() error {
    // Validate remote first
    if err := r.ValidateRemote(); err != nil {
        return fmt.Errorf("remote validation: %w", err)
    }

    // Then validate ref
    if err := r.ValidateRef(); err != nil {
        return fmt.Errorf("ref validation: %w", err)
    }

    return nil
}

Key features:
- Composite validation: Combines two validation steps
- Sequential: Remote first, then ref
- Fail fast: Returns on first error
- Error context: Prefixes errors with validation type
- Comprehensive: Full pre-clone validation

Validation order:
1. Remote: Check URL and repository accessibility
2. Ref: Check branch/tag/commit exists

Why this order:
- Logical: Can't check ref if repo doesn't exist
- Efficiency: Skip ref check if remote fails
- Clear errors: Remote error first helps debugging
- Network: One failure stops both network calls

Complete validation:
- URL format and accessibility
- Repository exists and is reachable
- Authentication works (if required)
- Branch/tag/commit exists (if specified)
- All prerequisites for successful clone
`,

		ExpectedOutcome: `
Valid repo and ref:
- MUST return nil (no error)
- MUST validate remote first
- MUST validate ref second
- MUST succeed for valid configurations

Valid repo, no ref:
- MUST return nil (no error)
- MUST validate remote only
- MUST treat empty ref as valid

Invalid repo:
- MUST return error
- MUST error from ValidateRemote
- MUST NOT check ref (fail fast)
- MUST error message indicate remote validation

Valid repo, invalid ref:
- MUST return error
- MUST error from ValidateRef
- MUST indicate ref validation failure
- MUST include ref name in error

Error propagation:
- MUST preserve error context
- MUST indicate which validation failed
- MUST wrap errors for clarity
`,

		TestScenario: `
Scenarios tested:

1. Valid repo and ref:
   GIVEN: URL=octocat/Hello-World, Ref=master
   WHEN: Validate()
   THEN: Returns nil (both valid)

2. Valid repo, no ref:
   GIVEN: URL=octocat/Hello-World, Ref=""
   WHEN: Validate()
   THEN: Returns nil (empty ref valid)

3. Invalid repo:
   GIVEN: URL=invalid-org/non-existent-repo, Ref=main
   WHEN: Validate()
   THEN: Returns error from remote validation

4. Valid repo, invalid ref:
   GIVEN: URL=octocat/Hello-World, Ref=non-existent-branch
   WHEN: Validate()
   THEN: Returns error from ref validation

Test implementation:
- Table-driven tests
- Network test (skipped in short mode)
- Complete validation scenarios
- Error message validation
`,

		Rationale: `
Why this behavior exists:
- Comprehensive check: All-in-one validation
- Pre-clone: Catch all issues before expensive clone
- User convenience: Single call validates everything
- CI/CD: Validate configs before deployment
- GitOps: Validate desired state completeness

Composite validation rationale:
- DRY: Don't duplicate validation logic
- Separation: Each method handles one concern
- Reusability: Can call ValidateRemote or ValidateRef separately
- Maintainability: One place to update each validation
- Flexibility: Callers can choose granular or complete

Sequential validation:
- Logical order: Remote must exist to check ref
- Efficiency: Don't waste network call if remote fails
- Clear errors: First failure is most important
- Fail fast: Stop on first problem
- User feedback: Error indicates which part failed

Use cases:
- Complete validation: Full pre-clone check
- Configuration validation: Validate YAML/JSON files
- User input: Validate user-provided configs
- CI/CD: Pre-deployment validation
- GitOps: Desired state validation
- API validation: REST/GraphQL input validation

When to use Validate vs separate calls:
- Validate(): Want complete validation, both must pass
- ValidateRemote(): Only care about URL/repo
- ValidateRef(): Assume repo valid, check ref only
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Validation order: Remote first, then ref (logical)
- Error wrapping: Context helps debugging
- Composite pattern: Combines two validations
- Fail fast: Returns on first error

HIGH RISK:
- Remove ValidateRemote → incomplete validation
- Remove ValidateRef → incomplete validation
- Change order → illogical (ref before remote)
- Remove error context → harder to debug

LOW RISK:
- Additional validation: Can add more checks
- Logging: Can add observability
- Validation options: Can add flags

What breaks if this changes:
1. Skip ValidateRemote → clone fails with worse error
2. Skip ValidateRef → clone fails on bad ref
3. Reverse order → inefficient, confusing errors
4. Remove error context → can't tell which failed
5. Don't fail fast → unnecessary network calls
6. Parallel validation → harder to understand errors
`,
	}

	// Skip network tests in CI/CD environments
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Non-existent/private-looking repos make GitHub prompt for https credentials; disable that here.
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	// Execute the behavioral tests
	logger := logging.NewLogger(logging.DEBUG)
	config := &uRepo.RepoConfig{
		Logger:          logger,
		ProgressHandler: uRepo.NewLogProgressHandler(logger),
	}

	tests := []struct {
		name        string
		url         string
		ref         string
		shouldError bool
		errorSubstr string
	}{
		{
			name:        "valid repo and ref",
			url:         "https://github.com/octocat/Hello-World.git",
			ref:         "master",
			shouldError: false,
		},
		{
			name:        "valid repo, no ref",
			url:         "https://github.com/octocat/Hello-World.git",
			ref:         "",
			shouldError: false,
		},
		{
			name:        "invalid repo",
			url:         "https://github.com/invalid-org-99999/non-existent-repo-99999.git",
			ref:         "main",
			shouldError: true,
			errorSubstr: "not found",
		},
		{
			name:        "valid repo, invalid ref",
			url:         "https://github.com/octocat/Hello-World.git",
			ref:         "non-existent-branch-99999",
			shouldError: true,
			errorSubstr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repo{
				URL:    tt.url,
				Ref:    tt.ref,
				config: config,
			}

			err := repo.Validate()

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorSubstr, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}
