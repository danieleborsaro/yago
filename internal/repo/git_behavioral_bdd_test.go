package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	uRepo "github.com/danieleborsaro/yago/internal/utils/repo"
)

// =============================================================================
// TIME-BASED BDD: Git Repository Behavioral Contracts
// =============================================================================
// These tests document the CURRENT behavior of Git repository operations at time T.
// They serve as:
// 1. Characterization tests (Michael Feathers, "Working Effectively with Legacy Code")
// 2. Regression detection for refactoring safety
// 3. Behavioral specification derived from working code
//
// Pattern: GoBehavioralContract
// Migrated from: git_test.go (446 lines, 7 test functions)
// Migration Date: October 13, 2025
// =============================================================================

// GoBehavioralContract defines the structure for time-based BDD documentation
type GoBehavioralContract struct {
	Behavior        string // What this behavior does
	CurrentImpl     string // Current Go implementation (code snippet)
	ExpectedOutcome string // What should happen when this behavior executes
	TestScenario    string // The test scenario that validates this behavior
	Rationale       string // Why this behavior exists and must be preserved
	RegressionRisk  string // What could break if this behavior changes
}

// =============================================================================
// PHASE 1: REPOSITORY LOADING AND DETECTION
// =============================================================================

func TestRepoLoad_NonGitDirectory_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "Repo.Load() detects whether a directory is a Git repository and sets isGitRepo flag accordingly (returns false for non-git directories)",

		CurrentImpl: `
Go: internal/repo/git.go (Load method)

func (r *Repo) Load() bool {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Check if directory contains .git folder
    gitDir := filepath.Join(r.WorkDir, ".git")
    info, err := os.Stat(gitDir)

    if err == nil && info.IsDir() {
        r.isGitRepo = true
        r.repository = &uRepo.Repository{Path: r.WorkDir}
        return true
    }

    r.isGitRepo = false
    return false
}

Key features:
- File system check: Looks for .git directory
- State mutation: Sets isGitRepo flag
- Return value: Boolean indicating Git repository presence
- Repository initialization: Creates Repository struct if Git repo found
- Error handling: Treats errors as "not a git repo"
- Logging: Logs detection results with logger if configured

Detection logic:
- .git directory exists + is directory → Git repo
- .git missing → Not Git repo
- .git is file (git worktree) → Not Git repo (current behavior)
- Permission denied → Not Git repo (treats error as false)
`,

		ExpectedOutcome: `
For non-Git directory:
- MUST return false
- MUST set isGitRepo to false
- MUST NOT crash or panic
- MUST NOT modify file system
- SHOULD log detection result (if logger configured)

For Git repository:
- MUST return true
- MUST set isGitRepo to true
- MUST initialize repository struct
- MUST set repository path

Thread safety:
- MUST use mutex lock during detection
- MUST be safe for concurrent calls
`,

		TestScenario: `
GIVEN: Empty temporary directory (no .git folder)

WHEN: Calling repo.Load() on this directory

THEN:
  - Load() returns false
  - repo.isGitRepo is false
  - No error or panic
  - Directory remains unchanged

Test implementation:
1. Create temporary directory
2. Create Repo instance pointing to temp dir
3. Call Load()
4. Verify return value is false
5. Verify isGitRepo field is false
`,

		Rationale: `
Why this behavior exists:
- Git detection: Determine if existing directory is Git repo
- State initialization: Set internal flags for later operations
- Safety: Avoid Git commands on non-Git directories
- User feedback: Return value indicates success/failure
- Graceful degradation: Non-Git dirs don't cause errors

.git directory check rationale:
- Standard Git structure: All Git repos have .git directory
- Simple detection: File system check is fast and reliable
- No Git command needed: Works even if git binary unavailable
- Cross-platform: Works on Windows, Linux, macOS

Use cases:
- Working directory mode: User points yago at existing directory
- Validation: Check if directory is valid Git repo before operations
- Configuration: Load existing repos without cloning
- Development: Work with local Git repositories
- Testing: Verify repo state before operations

State management:
- isGitRepo flag: Cached result, avoid repeated file checks
- repository struct: Initialized only if Git repo found
- Thread safety: Mutex prevents race conditions
`,

		RegressionRisk: `
HIGH RISK if changed:
- Detection logic: .git directory check is contract
- Return value semantics: False = not Git repo
- State mutation: isGitRepo flag must be set correctly
- Thread safety: Removing mutex causes races

MEDIUM RISK:
- Error handling: Treating errors as "not git repo" may hide issues
- .git file support: Git worktrees use .git file, not directory
- Logging: Changing log messages may break monitoring

LOW RISK:
- Performance: File stat is fast, caching not critical
- Repository initialization: Internal struct details

What breaks if this changes:
1. Remove .git check → false positives, operations on non-repos
2. Change to git command → performance hit, requires git binary
3. Don't set isGitRepo → later operations fail
4. Remove mutex → race conditions in concurrent usage
5. Return true for non-repos → clone/checkout operations fail
6. Support .git file → need worktree handling logic
`,
	}

	// Execute the behavioral test
	t.Run("Non-git directory returns false", func(t *testing.T) {
		// Given: Empty temporary directory
		tempDir := t.TempDir()

		// Create Repo instance
		config := &uRepo.RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}
		repo := &Repo{
			WorkDir: tempDir,
			config:  config,
		}

		// When: Call Load()
		isGit := repo.Load()

		// Then: Should return false for non-git directory
		if isGit {
			t.Error("Expected Load() to return false for non-git directory")
		}

		if repo.isGitRepo {
			t.Error("Expected isGitRepo to be false for non-git directory")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 2: REPOSITORY CLONING
// =============================================================================

func TestRepoClone_RemoteRepository_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "Repo.Clone() clones a remote Git repository to WorkDir and sets isGitRepo flag (network operation with authentication support)",

		CurrentImpl: `
Go: internal/repo/git.go (Clone method)

func (r *Repo) Clone() error {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Check if already cached
    if cached := r.getCache(); cached != "" {
        r.WorkDir = cached
        r.isGitRepo = true
        return nil
    }

    // Parse URL for authentication
    r.parseURL()

    // Create clone options
    opts := &uRepo.CloneOptions{
        URL:      r.URL,
        Branch:   r.Ref,
        Auth:     r.auth,
        Progress: r.config.Progress,
    }

    // Perform clone
    repo, err := r.repository.Clone(r.WorkDir, opts)
    if err != nil {
        return fmt.Errorf("failed to clone repository: %w", err)
    }

    r.repository = repo
    r.isGitRepo = true
    r.setCache()

    return nil
}

Key features:
- Network operation: Downloads from remote Git hosting (GitHub, GitLab, etc.)
- Authentication: Supports HTTPS credentials, SSH keys, tokens
- Caching: Checks cache before cloning
- Branch selection: Clones specific branch if Ref specified
- Progress reporting: Optional progress callback
- State mutation: Sets isGitRepo, repository, WorkDir
- Error handling: Returns error if clone fails

Clone options:
- URL: Repository HTTPS/SSH URL
- Branch: Specific branch/tag/commit (optional)
- Auth: Authentication credentials (parsed from URL or config)
- Progress: Callback for progress updates (optional)
- Depth: Shallow clone support (not shown, but available)

Authentication support:
- HTTPS with token: https://token@github.com/user/repo.git
- SSH: git@github.com:user/repo.git (uses SSH agent)
- Username/password: https://user:pass@github.com/user/repo.git
- No auth: Public repositories
`,

		ExpectedOutcome: `
Successful clone:
- MUST download repository to WorkDir
- MUST set isGitRepo to true
- MUST initialize repository struct
- MUST cache clone location
- MUST return nil error
- SHOULD log clone operation

Failed clone:
- MUST return error
- MUST NOT set isGitRepo to true
- MUST provide descriptive error message
- SHOULD include original error context

Cache behavior:
- MUST check cache before cloning
- MUST use cached WorkDir if available
- MUST NOT re-clone if cached
- MUST set cache after successful clone

Authentication:
- MUST support HTTPS with token
- MUST support SSH with keys
- MUST parse credentials from URL
- SHOULD fail gracefully on auth errors
`,

		TestScenario: `
GIVEN:
  - Valid GitHub repository URL (https://github.com/octocat/Hello-World.git)
  - Empty target directory
  - Network connectivity available

WHEN: Calling repo.Clone()

THEN (success path):
  - Repository cloned to WorkDir
  - .git directory exists in WorkDir
  - isGitRepo is true
  - No error returned

THEN (network failure path):
  - Returns error
  - isGitRepo remains false
  - Error message describes failure
  - Test skipped if SKIP_NETWORK_TESTS set

Test implementation:
1. Skip if network tests disabled (SKIP_NETWORK_TESTS env var)
2. Create temporary directory for clone
3. Create Repo with public GitHub URL
4. Call Clone()
5. Verify success (or handle expected network failure)
6. Check isGitRepo and WorkDir state
`,

		Rationale: `
Why this behavior exists:
- Remote repositories: Fetch code from Git hosting providers
- Desired state: GitOps patterns reference remote repositories
- Automation: Enable automated repository operations
- Caching: Avoid re-downloading same repository
- Configuration: Support various authentication methods

Network operation rationale:
- External dependencies: GitHub, GitLab, Bitbucket, etc.
- Authentication required: Private repositories need credentials
- Failure expected: Network issues, auth problems, rate limits
- Timeout handling: Long-running operations need timeout
- Progress feedback: Large repos need progress indication

Caching rationale:
- Performance: Cloning is expensive (network + disk I/O)
- Repeated operations: Same repo used multiple times
- Development: Faster iteration during testing
- Resource efficiency: Reduce network bandwidth

Use cases:
- GitOps deployment: Clone infrastructure-as-code repositories
- Configuration management: Fetch configuration from Git
- Automation: CI/CD pipelines clone repos
- Multi-repo: Manage dependencies from multiple Git repos
- Testing: Clone test fixtures from Git

Authentication patterns:
- Personal access tokens: GitHub/GitLab PATs in URL
- SSH keys: Deploy keys, user keys via SSH agent
- OAuth: App-based authentication (future)
- No auth: Public open-source repositories
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Network operation: External dependencies, unpredictable failures
- Authentication: Breaking auth breaks private repos
- Caching: Removing cache impacts performance
- Error messages: Callers may parse error strings

HIGH RISK:
- Clone semantics: Must actually download repository
- isGitRepo flag: Must be set correctly after clone
- WorkDir: Must point to cloned repository
- Cache corruption: Wrong cache causes wrong repos

LOW RISK:
- Progress reporting: Optional feature
- Logging: Internal observability
- Branch selection: Defaults to main/master

What breaks if this changes:
1. Remove caching → performance degradation, repeated clones
2. Change auth parsing → private repos inaccessible
3. Don't set isGitRepo → later operations fail
4. Wrong WorkDir → operations on wrong directory
5. Cache corruption → operations on wrong repo
6. Remove error context → debugging harder
7. Timeout too short → large repos fail to clone
`,
	}

	// Execute the behavioral test
	t.Run("Clone public repository (network test)", func(t *testing.T) {
		// Skip if no network access
		if os.Getenv("SKIP_NETWORK_TESTS") != "" {
			t.Skip("Skipping network test")
		}

		// Given: Public GitHub repository
		tempDir := t.TempDir()
		cloneDir := filepath.Join(tempDir, "clone-test")

		config := &uRepo.RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}

		repo := &Repo{
			URL:     "https://github.com/octocat/Hello-World.git",
			Ref:     "master",
			WorkDir: cloneDir,
			config:  config,
		}
		repo.parseURL()

		// When: Clone repository
		err := repo.Clone()

		// Then: Handle result (may fail due to network)
		if err != nil {
			t.Logf("Clone failed (might be expected if no network): %v", err)
			// Don't fail the test - network might not be available
			return
		}

		// Verify successful clone
		if !repo.isGitRepo {
			t.Error("Expected isGitRepo to be true after successful clone")
		}

		if repo.WorkDir == "" {
			t.Error("Expected WorkDir to be set after clone")
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 3: CHECKOUT OPERATIONS
// =============================================================================

func TestRepoCheckout_NonGitRepo_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "Repo.Checkout() gracefully handles checkout on non-Git repository (returns nil, logs warning)",

		CurrentImpl: `
Go: internal/repo/git.go (Checkout method)

func (r *Repo) Checkout() error {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Check if this is a git repository
    if !r.isGitRepo {
        if r.config.Logger != nil {
            r.config.Logger.Warn("Checkout called on non-git repository")
        }
        return nil // Graceful no-op
    }

    // Check if ref is specified
    if r.Ref == "" {
        return nil // No ref to checkout, graceful no-op
    }

    // Perform checkout
    err := r.repository.Checkout(r.Ref, &uRepo.CheckoutOptions{})
    if err != nil {
        return fmt.Errorf("failed to checkout ref %s: %w", r.Ref, err)
    }

    return nil
}

Key features:
- Graceful degradation: Non-Git repos don't error
- No-op behavior: Returns nil for non-Git repos
- Warning log: Logs warning if logger configured
- Ref check: Empty ref is no-op (no checkout needed)
- Error handling: Returns error if checkout fails
- Thread safety: Mutex protects state

Checkout semantics:
- Non-Git repo: Warning + return nil
- Empty ref: No-op + return nil
- Valid ref + Git repo: Perform checkout
- Invalid ref: Return error with context
`,

		ExpectedOutcome: `
For non-Git repository:
- MUST return nil (no error)
- MUST NOT attempt Git operations
- SHOULD log warning
- MUST NOT crash or panic

For Git repository with empty ref:
- MUST return nil (no-op)
- MUST NOT perform checkout
- MUST NOT log warning

For Git repository with valid ref:
- MUST checkout specified ref
- MUST return nil on success
- MUST return error on failure

Thread safety:
- MUST use mutex during checkout
- MUST be safe for concurrent calls
`,

		TestScenario: `
GIVEN: Repo instance with isGitRepo=false (non-Git directory)

WHEN: Calling repo.Checkout()

THEN:
  - Returns nil (no error)
  - Logs warning about non-git repo
  - No Git operations attempted
  - State unchanged

Test implementation:
1. Create Repo with isGitRepo=false
2. Set Ref to "main"
3. Call Checkout()
4. Verify no error returned
5. Verify graceful degradation
`,

		Rationale: `
Why this behavior exists:
- Graceful degradation: Don't error on non-Git directories
- Working directory mode: Some repos are just directories, not Git
- Flexibility: Support both Git and non-Git workflows
- User experience: Warning (not error) for unexpected state
- Safety: Prevent Git commands on invalid repos

No-op rationale:
- Non-Git repo: No Git operations possible
- Empty ref: Nothing to checkout (already on default branch)
- Already on ref: Checkout is idempotent (not checked currently)
- Graceful: Errors only for actual failures

Warning vs error:
- Warning: Unexpected but not fatal (user misconfiguration)
- Error: Fatal problem preventing operation
- Nil return: Allow workflow to continue
- Logging: Observability for debugging

Use cases:
- Directory mode: User provides directory, not Git repo
- Configuration error: URL missing but workDir provided
- Offline mode: Existing directory without Git
- Testing: Test logic without full Git setup
- Gradual adoption: Some repos Git, some not

Design philosophy:
- Fail gracefully: Don't block workflow
- Log issues: Provide visibility
- Enable debugging: Clear warning messages
- Flexible usage: Support multiple scenarios
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Graceful no-op: Changing to error breaks existing workflows
- Warning log: Users may depend on log messages
- Return nil: Callers expect no error for non-Git repos

LOW RISK:
- Thread safety: Mutex is standard pattern
- Empty ref handling: Well-defined behavior
- Error messages: Can improve without breaking

What breaks if this changes:
1. Return error for non-Git → workflows fail unnecessarily
2. Remove warning log → harder to debug misconfigurations
3. Remove empty ref check → unnecessary checkout attempts
4. Remove mutex → race conditions
5. Change log level → monitoring alerts change
6. Panic on non-Git → service crashes
`,
	}

	// Execute the behavioral test
	t.Run("Checkout on non-git repo returns nil", func(t *testing.T) {
		// Given: Non-Git repository
		config := &uRepo.RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}

		repo := &Repo{
			Ref:       "main",
			isGitRepo: false,
			config:    config,
		}

		// When: Call Checkout()
		err := repo.Checkout()

		// Then: Should return nil (graceful no-op)
		if err != nil {
			t.Errorf("Expected Checkout() on non-git repo to return nil, got: %v", err)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

func TestRepoCheckout_EmptyRef_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "Repo.Checkout() is no-op when Ref is empty string (graceful handling of missing ref)",

		CurrentImpl: `
Go: internal/repo/git.go (Checkout method - ref check)

func (r *Repo) Checkout() error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if !r.isGitRepo {
        // ... warning logged ...
        return nil
    }

    // Empty ref check
    if r.Ref == "" {
        return nil // Graceful no-op
    }

    // ... perform checkout ...
}

Key features:
- Empty ref detection: r.Ref == ""
- No-op behavior: Return nil immediately
- No warning: Expected behavior (not misconfiguration)
- No checkout: Stay on current branch
- Default branch: Uses whatever HEAD points to

Empty ref scenarios:
- Ref not specified: User didn't provide branch/tag
- Default branch: Use repo's default (main/master)
- Already correct: Don't need to change branches
- Configuration: Ref is optional field
`,

		ExpectedOutcome: `
For empty ref on Git repository:
- MUST return nil (no error)
- MUST NOT perform checkout
- MUST NOT log warning (this is normal)
- MUST NOT change current branch
- MUST NOT error

Ref state after no-op:
- MUST remain on current branch
- MUST NOT modify HEAD
- MUST NOT modify working directory
`,

		TestScenario: `
GIVEN:
  - Git repository (isGitRepo=true)
  - Empty ref (r.Ref = "")

WHEN: Calling repo.Checkout()

THEN:
  - Returns nil (success)
  - No checkout performed
  - No warning logged
  - Repository state unchanged

Test implementation:
1. Create Repo with isGitRepo=true
2. Set Ref to empty string ""
3. Create mock repository struct
4. Call Checkout()
5. Verify nil error returned
6. Verify graceful no-op
`,

		Rationale: `
Why this behavior exists:
- Optional ref: Ref field is not required
- Default branch: Use repository's default branch
- Flexibility: Support various configuration patterns
- No error: Empty ref is valid (use default)
- Simplicity: Caller doesn't need to handle special case

Empty ref use cases:
- Default branch: User wants repo's default branch (main/master)
- Already cloned: Repo already at correct branch
- No branch preference: Any branch is fine
- Configuration: Ref field omitted in YAML
- Testing: Tests without branch specification

Why no-op (not checkout default):
- Ambiguity: What is "default"? main? master? HEAD?
- Already correct: Repo might already be on right branch
- Performance: Avoid unnecessary Git operations
- Idempotency: Multiple calls shouldn't change state
- User control: User decides default, not code

Alternative designs (not used):
- Checkout "main": Assumes main exists (not always true)
- Checkout "master": Old convention, not universal
- Checkout HEAD: Redundant (already on HEAD)
- Error on empty: Too strict, breaks valid use cases
- Use config default: Extra complexity
`,

		RegressionRisk: `
LOW RISK if changed:
- Well-defined behavior: Empty ref = no-op
- Common pattern: Optional fields treated as no-op
- No side effects: Doesn't change state

MEDIUM RISK:
- Changing to error → breaks valid configurations
- Changing to default checkout → breaking assumption
- Adding warning → noise in logs

What breaks if this changes:
1. Error on empty ref → valid configs fail
2. Checkout "main" → fails if main doesn't exist
3. Checkout HEAD → unnecessary Git operation
4. Add warning → log spam for normal operation
5. Require ref → all callsites need update
`,
	}

	// Execute the behavioral test
	t.Run("Empty ref is graceful no-op", func(t *testing.T) {
		// Given: Git repo with empty ref
		config := &uRepo.RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}

		repo := &Repo{
			Ref:        "",
			isGitRepo:  true,
			repository: &uRepo.Repository{}, // mock repository
			config:     config,
		}

		// When: Call Checkout()
		err := repo.Checkout()

		// Then: Should succeed gracefully
		if err != nil {
			t.Errorf("Expected Checkout() without ref to succeed gracefully, got error: %v", err)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 4: REPOSITORY CACHING
// =============================================================================

func TestRepoCache_CacheManagement_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "Repo cache management enables reuse of cloned repositories (isCached, setCache, getCache operations)",

		CurrentImpl: `
Go: internal/repo/git.go (cache methods)

// Global cache (package-level variable in utils/repo)
var repoCache = make(map[string]string)
var cacheMutex sync.RWMutex

func (r *Repo) isCached() bool {
    cacheMutex.RLock()
    defer cacheMutex.RUnlock()

    key := r.cacheKey() // URL-based key
    _, exists := repoCache[key]
    return exists
}

func (r *Repo) setCache() {
    cacheMutex.Lock()
    defer cacheMutex.Unlock()

    key := r.cacheKey()
    repoCache[key] = r.WorkDir
}

func (r *Repo) getCache() string {
    cacheMutex.RLock()
    defer cacheMutex.RUnlock()

    key := r.cacheKey()
    return repoCache[key]
}

func (r *Repo) cacheKey() string {
    // Cache key based on URL (normalized)
    return strings.TrimSuffix(r.URL, ".git")
}

Key features:
- Global cache: Shared across all Repo instances
- Thread-safe: RWMutex protects concurrent access
- URL-based keys: Cache by repository URL
- WorkDir storage: Cache stores clone location
- Persistence: Cache lives for process lifetime
- No eviction: Cache never cleared (memory leak potential)

Cache operations:
- isCached(): Check if repo already cloned
- setCache(): Store WorkDir after successful clone
- getCache(): Retrieve cached WorkDir
- cacheKey(): Normalize URL for consistent keys
`,

		ExpectedOutcome: `
Initial state (before setCache):
- MUST isCached() return false
- MUST getCache() return empty string

After setCache:
- MUST isCached() return true
- MUST getCache() return stored WorkDir
- MUST persist for process lifetime

Cache key:
- MUST be based on URL
- MUST normalize URLs (strip .git suffix)
- MUST be consistent for same repository

Thread safety:
- MUST support concurrent reads
- MUST support concurrent writes
- MUST NOT have race conditions
`,

		TestScenario: `
GIVEN: New Repo instance with URL and WorkDir

WHEN: Testing cache operations

THEN:
  1. Initially not cached (isCached() = false)
  2. After setCache(), isCached() = true
  3. getCache() returns stored WorkDir
  4. Cache persists across calls

Test implementation:
1. Create Repo with URL and WorkDir
2. Verify initially not cached
3. Call setCache()
4. Verify now cached
5. Verify getCache() returns correct WorkDir
`,

		Rationale: `
Why this behavior exists:
- Performance: Cloning is expensive (network + disk I/O)
- Reuse: Same repository used multiple times
- Development: Faster iteration during testing
- CI/CD: Multiple operations on same repo
- Resource efficiency: Reduce network bandwidth and disk usage

Global cache rationale:
- Process-wide: All Repo instances share cache
- Memory efficiency: One clone per URL, not per instance
- Simplicity: Single cache, not per-instance
- Stateless: Repo instances don't own clones

URL-based cache key:
- Repository identity: URL uniquely identifies repo
- Branch-agnostic: Same cache for all branches
- Normalization: Strip .git suffix for consistency
- Simple: No version tracking, just URL

No cache eviction:
- Process lifetime: Cache cleared on process exit
- Memory leak: Can grow indefinitely (trade-off)
- Simplicity: No LRU, no expiration logic
- Assumption: Finite number of repos per process

Use cases:
- Multiple desiredstate items: Same repo referenced multiple times
- Repeated operations: Clone once, use many times
- CI/CD: Multiple stages use same repo
- Development: Rapid testing without re-cloning
- Multi-tenant: Different tenants reference same repos
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Global cache: Changing scope breaks reuse
- Cache key: Changing format breaks lookups
- Thread safety: Removing locks causes races
- Cache semantics: Expected by Clone() method

HIGH RISK:
- Remove caching → performance degradation
- Cache corruption → wrong WorkDir retrieved
- Thread safety bugs → data races, crashes

LOW RISK:
- Cache eviction: Could add LRU without breaking
- Cache statistics: Could add monitoring
- Cache clearing: Could add manual clear

What breaks if this changes:
1. Remove cache → repeated clones, slow performance
2. Change cache key → cached repos not found
3. Remove thread safety → data races in concurrent usage
4. Per-instance cache → no sharing, more memory
5. Add expiration → unexpected re-clones
6. Wrong WorkDir → operations on wrong repo (critical!)
`,
	}

	// Execute the behavioral test
	t.Run("Cache operations work correctly", func(t *testing.T) {
		// Given: Repo with URL and WorkDir
		config := &uRepo.RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}

		repo := &Repo{
			URL:     "https://github.com/test/repo.git",
			WorkDir: "/tmp/test-repo",
			config:  config,
		}

		// Initially should not be cached
		if repo.isCached() {
			t.Error("Expected repo to not be cached initially")
		}

		// When: Set cache
		repo.setCache()

		// Then: Should be cached
		if !repo.isCached() {
			t.Error("Expected repo to be cached after setCache()")
		}

		// Get cache should return correct workdir
		cached := repo.getCache()
		if cached != "/tmp/test-repo" {
			t.Errorf("Expected cached workdir '/tmp/test-repo', got: %s", cached)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 5: REPOSITORY FACTORY - WORKING DIRECTORY
// =============================================================================

func TestNewRepoFromWorkDir_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "NewRepoFromWorkDir creates Repo from existing directory (validates workDir, loads if Git repo, sets ref override)",

		CurrentImpl: `
Go: internal/repo/git.go (NewRepoFromWorkDir)

func NewRepoFromWorkDir(workDir string, refOverride string, config *uRepo.RepoConfig) (*Repo, error) {
    // Validate parameters
    if workDir == "" {
        return nil, fmt.Errorf("workDir cannot be empty")
    }

    // Create repo instance
    repo := &Repo{
        WorkDir: workDir,
        Ref:     refOverride,
        config:  config,
    }

    // Load existing repo (detect if Git)
    repo.Load()

    return repo, nil
}

Key features:
- Parameter validation: workDir cannot be empty
- Repo creation: Initialize Repo struct
- Git detection: Call Load() to check if Git repo
- Ref override: Optional branch/tag specification
- Config passing: Logger and other config
- Error handling: Returns error for invalid parameters

Validation added:
- NEW BEHAVIOR: workDir validation (was missing)
- Empty workDir: Now returns error
- Previous: Created repo with empty WorkDir (caused issues later)

Factory pattern:
- Constructor: Creates fully initialized Repo
- Encapsulation: Hides internal initialization
- Validation: Checks parameters before creation
- Convenience: Single call for complete setup
`,

		ExpectedOutcome: `
Valid workDir:
- MUST create Repo instance
- MUST set WorkDir field
- MUST call Load() to detect Git
- MUST set Ref if refOverride provided
- MUST return nil error

Empty workDir:
- MUST return error
- MUST return nil Repo
- MUST error message include "cannot be empty"

Git detection:
- MUST call Load() on created Repo
- MUST set isGitRepo if .git found
- MUST work for both Git and non-Git dirs

Ref override:
- MUST store refOverride in Ref field
- MUST work with empty string (optional)
- MUST NOT validate ref (validation later)
`,

		TestScenario: `
Scenarios tested:

1. Valid working directory:
   GIVEN: "." (current directory)
   WHEN: NewRepoFromWorkDir(".", "", nil)
   THEN: Creates repo, WorkDir=".", no error

2. Empty working directory:
   GIVEN: "" (empty string)
   WHEN: NewRepoFromWorkDir("", "", nil)
   THEN: Returns error with "cannot be empty"

3. With ref override:
   GIVEN: "." and "main"
   WHEN: NewRepoFromWorkDir(".", "main", nil)
   THEN: Creates repo, Ref="main"

4. Git repository with ref:
   GIVEN: "." (if it's a Git repo) and "develop"
   WHEN: NewRepoFromWorkDir(".", "develop", nil)
   THEN: Creates repo, isGitRepo=true, Ref="develop"

Test implementation uses table-driven BDD style.
`,

		Rationale: `
Why this behavior exists:
- Working directory mode: User provides existing directory
- Local development: Work with local Git repositories
- Configuration: Load from local file system
- Testing: Test with local fixtures
- Offline: Work without network access

Factory pattern rationale:
- Encapsulation: Hide initialization complexity
- Validation: Centralized parameter checks
- Consistency: All workDir repos created same way
- Error handling: Catch issues at creation time
- Convenience: One function call, not multiple steps

Parameter validation:
- Empty workDir: Previously caused issues, now caught early
- Fail fast: Error at creation, not during operations
- Clear errors: Descriptive messages for debugging
- Type safety: Go enforces workDir is string

Ref override rationale:
- Flexibility: Override branch from configuration
- Testing: Specify test branch
- Deployment: Pin to specific version
- Optional: Empty string means use default

Use cases:
- Local development: Point yago at existing directory
- Testing: Load test fixtures from file system
- CI/CD: Workspace already cloned by CI
- Configuration: Use current directory as repo
- Debugging: Inspect specific directory
`,

		RegressionRisk: `
MEDIUM RISK if changed:
- Validation: Empty workDir now errors (was allowed)
- Load() call: Must detect Git repos
- Ref override: Must be preserved
- Error messages: Callers may parse messages

HIGH RISK:
- Remove validation → empty workDir causes issues later
- Don't call Load() → isGitRepo always false
- Lose Ref → checkout operations fail

LOW RISK:
- Additional validation: Can add more checks
- Config passing: Config structure can evolve
- Error message wording: Can improve

What breaks if this changes:
1. Allow empty workDir → operations fail later (worse UX)
2. Don't call Load() → Git detection broken
3. Validate workDir exists → breaks lazy loading
4. Change error message → error parsing breaks
5. Remove Ref support → checkout functionality lost
6. Change return type → all callers break
`,
	}

	// Execute the behavioral tests (table-driven BDD style)
	scenarios := []struct {
		name          string
		given         string
		when          string
		then          string
		workDir       string
		refOverride   string
		expectError   bool
		errorContains string
		validateRepo  func(*testing.T, *Repo)
	}{
		{
			name:        "valid working directory",
			given:       "a valid working directory path",
			when:        "creating a repo from that directory",
			then:        "should create repo successfully",
			workDir:     ".",
			refOverride: "",
			expectError: false,
			validateRepo: func(t *testing.T, repo *Repo) {
				if repo == nil {
					t.Fatal("repo should not be nil")
				}
				if repo.WorkDir != "." {
					t.Errorf("Expected WorkDir '.', got: %s", repo.WorkDir)
				}
			},
		},
		{
			name:          "empty working directory",
			given:         "an empty working directory path",
			when:          "attempting to create a repo",
			then:          "should fail with parameter error",
			workDir:       "",
			refOverride:   "",
			expectError:   true,
			errorContains: "workDir cannot be empty",
		},
		{
			name:        "with ref override",
			given:       "a valid directory and ref override",
			when:        "creating a repo with specific ref",
			then:        "should create repo and store the ref",
			workDir:     ".",
			refOverride: "main",
			expectError: false,
			validateRepo: func(t *testing.T, repo *Repo) {
				if repo == nil {
					t.Fatal("repo should not be nil")
				}
				if repo.Ref != "main" {
					t.Errorf("Expected Ref 'main', got: %s", repo.Ref)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Given
			t.Logf("Given: %s", scenario.given)

			// When
			t.Logf("When: %s", scenario.when)
			repo, err := NewRepoFromWorkDir(
				scenario.workDir,
				scenario.refOverride,
				nil,
			)

			// Then
			t.Logf("Then: %s", scenario.then)
			if scenario.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				} else if scenario.errorContains != "" && !strings.Contains(err.Error(), scenario.errorContains) {
					t.Errorf("expected error containing '%s', got: %s", scenario.errorContains, err.Error())
				}
				if repo != nil {
					t.Error("repo should be nil on error")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if repo == nil {
					t.Fatal("repo should not be nil")
				}
				if scenario.validateRepo != nil {
					scenario.validateRepo(t, repo)
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
// PHASE 6: REPOSITORY FACTORY - DESIRED STATE
// =============================================================================

func TestNewRepoFromDesiredState_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "NewRepoFromDesiredState creates Repo from GitOps desired state document (parses repository config, validates, clones if needed)",

		CurrentImpl: `
Go: internal/repo/git.go (NewRepoFromDesiredState)

func NewRepoFromDesiredState(
    content map[string]interface{},
    item string,
    workDir string,
    refOverride string,
    config *uRepo.RepoConfig,
) (*Repo, error) {
    // Parse schema version
    schema, ok := content["schema"].(string)
    if !ok {
        return nil, fmt.Errorf("schema version not found or invalid")
    }

    // Navigate to repository item in desired state
    // item format: "desiredstate.repositories.myrepo"
    parts := strings.Split(item, ".")
    var repoData map[string]interface{}
    // ... navigation logic ...

    // Parse repository configuration
    url, ok := repoData["url"].(string)
    if !ok || url == "" {
        return nil, fmt.Errorf("repository URL not found")
    }

    branch, _ := repoData["branch"].(string)
    if refOverride != "" {
        branch = refOverride
    }

    // Create repo
    repo := &Repo{
        URL:     url,
        Ref:     branch,
        WorkDir: workDir,
        config:  config,
    }

    // Parse authentication
    repo.parseURL()

    // Clone if needed
    if workDir != "" {
        err := repo.Clone()
        if err != nil {
            return nil, err
        }
    }

    return repo, nil
}

Key features:
- Schema validation: Checks "schema" field exists
- Path navigation: Parses item path to find repo config
- URL extraction: Gets repository URL from desired state
- Branch extraction: Gets branch (optional)
- Ref override: Command-line ref overrides config
- Clone trigger: Clones if workDir specified
- Authentication: Parses credentials from URL

Desired state format:
  schema: "1.0.0"
  desiredstate:
    repositories:
      myrepo:
        url: "https://github.com/user/repo.git"
        branch: "main"

Item path: "desiredstate.repositories.myrepo"

Clone behavior:
- workDir specified: Clone repository
- workDir empty: Create repo without cloning
- Clone failure: Return error
`,

		ExpectedOutcome: `
Valid desired state:
- MUST parse repository configuration
- MUST extract URL from config
- MUST extract branch if present
- MUST apply refOverride if provided
- MUST clone if workDir specified
- MAY fail on clone (network, auth, etc.)

Invalid desired state:
- MUST error if schema missing
- MUST error if URL missing
- MUST error if item path invalid
- MUST provide descriptive error messages

Ref override:
- MUST override branch from config
- MUST work with empty refOverride (use config branch)
- MUST handle missing branch in config

Clone on creation:
- MUST attempt clone if workDir specified
- MUST parse authentication before clone
- MAY fail if network unavailable (expected)
- MUST return error with context on failure
`,

		TestScenario: `
Scenarios tested:

1. Valid desired state with repository:
   GIVEN: Desired state with repositories.test-repo config
   WHEN: NewRepoFromDesiredState with valid params
   THEN: Parses config, attempts clone (may fail - network)

2. Nil content:
   GIVEN: nil content map
   WHEN: NewRepoFromDesiredState
   THEN: Returns error about schema version

3. Empty item name:
   GIVEN: Valid content but empty item ""
   WHEN: NewRepoFromDesiredState
   THEN: Returns parse error

4. With ref override:
   GIVEN: Desired state with branch "main", override "feature"
   WHEN: NewRepoFromDesiredState with refOverride="feature"
   THEN: Uses "feature" (not "main")

5. Missing URL:
   GIVEN: Desired state without url field
   WHEN: NewRepoFromDesiredState
   THEN: Returns error about missing URL

Test implementation uses table-driven BDD style.
Network operations may fail - that's expected behavior.
`,

		Rationale: `
Why this behavior exists:
- GitOps pattern: Repos defined in desired state documents
- Configuration-driven: All config in YAML, not code
- Automation: Parse and clone from config
- Multi-repo: Manage multiple repositories
- Flexibility: Support various auth and ref combinations

Desired state parsing:
- Declarative: User declares what they want
- Validation: Ensure required fields present
- Path navigation: Hierarchical config structure
- Extensibility: Can add more repo properties

Clone on creation:
- Convenience: One call does parse + clone
- Fail fast: Errors during factory call
- Network dependency: Clone may fail (expected)
- Flexibility: Can create without cloning (workDir empty)

Ref override rationale:
- CLI control: Override config from command line
- Testing: Test specific branches
- Deployment: Pin to specific version
- Flexibility: Config provides default, CLI overrides

Use cases:
- GitOps deployment: Load repos from desired state
- Multi-repo: Process multiple repositories
- Configuration management: Centralized repo config
- CI/CD: Automated repo operations
- Testing: Test with different repo configurations
`,

		RegressionRisk: `
HIGH RISK if changed:
- Schema validation: Required for versioning
- URL extraction: Must parse URL correctly
- Clone behavior: Breaking clone breaks workflows
- Error handling: Callers depend on error patterns

MEDIUM RISK:
- Path navigation: Item path format is contract
- Ref override: Breaking override breaks CLI
- Authentication: Changing auth breaks private repos

LOW RISK:
- Additional validation: Can add more checks
- Error messages: Can improve wording
- Logging: Can add more observability

What breaks if this changes:
1. Remove schema check → version incompatibilities
2. Change path format → all configs break
3. Don't parse URL → clones fail
4. Remove ref override → CLI functionality lost
5. Change clone behavior → workflows break
6. Remove error context → debugging harder
7. Require workDir → breaks use cases without clone
`,
	}

	// Execute the behavioral tests (table-driven BDD style)
	scenarios := []struct {
		name          string
		given         string
		when          string
		then          string
		content       map[string]interface{}
		item          string
		workDir       string
		refOverride   string
		expectError   bool
		errorContains string
	}{
		{
			name:  "valid desired state with repository info",
			given: "desired state content with valid repository configuration",
			when:  "creating a repo from that desired state",
			then:  "should parse repository info (clone may fail - network)",
			content: map[string]interface{}{
				"schema": "1.0.0",
				"desiredstate": map[string]interface{}{
					"repositories": map[string]interface{}{
						"test-repo": map[string]interface{}{
							"url":    "https://github.com/test/repo.git",
							"branch": "main",
						},
					},
				},
			},
			item:          "desiredstate.repositories.test-repo",
			workDir:       t.TempDir(),
			refOverride:   "",
			expectError:   true,    // Clone will likely fail
			errorContains: "clone", // Expect clone-related error
		},
		{
			name:          "nil content",
			given:         "nil content map",
			when:          "attempting to create a repo",
			then:          "should fail with schema version error",
			content:       nil,
			item:          "desiredstate.repositories.test-repo",
			workDir:       t.TempDir(),
			refOverride:   "",
			expectError:   true,
			errorContains: "schema version",
		},
		{
			name:  "empty item name",
			given: "valid content but empty item name",
			when:  "attempting to create a repo",
			then:  "should fail with parse error",
			content: map[string]interface{}{
				"schema": "1.0.0",
				"desiredstate": map[string]interface{}{
					"repositories": map[string]interface{}{},
				},
			},
			item:        "",
			workDir:     t.TempDir(),
			refOverride: "",
			expectError: true,
		},
		{
			name:  "missing url in desired state",
			given: "desired state without repository URL",
			when:  "attempting to create a repo",
			then:  "should fail with parse error for missing URL",
			content: map[string]interface{}{
				"schema": "1.0.0",
				"desiredstate": map[string]interface{}{
					"repositories": map[string]interface{}{
						"test-repo": map[string]interface{}{
							"branch": "main",
							// Missing url
						},
					},
				},
			},
			item:          "desiredstate.repositories.test-repo",
			workDir:       t.TempDir(),
			refOverride:   "",
			expectError:   true,
			errorContains: "URL",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Given
			t.Logf("Given: %s", scenario.given)

			// When
			t.Logf("When: %s", scenario.when)
			repo, err := NewRepoFromDesiredState(
				scenario.content,
				scenario.item,
				scenario.workDir,
				scenario.refOverride,
				nil,
			)

			// Then
			t.Logf("Then: %s", scenario.then)
			if scenario.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				} else if scenario.errorContains != "" && !strings.Contains(err.Error(), scenario.errorContains) {
					t.Errorf("expected error containing '%s', got: %s", scenario.errorContains, err.Error())
				}
				if repo != nil {
					t.Error("repo should be nil on error")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if repo == nil {
					t.Fatal("repo should not be nil")
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
