package repo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	uRepo "github.com/danieleborsaro/yago/internal/utils/repo"
)

const defaultFixedCloneBaseDir = "/tmp/gitops-repo"

var (
	useFixedCloneBaseDir = false
	fixedCloneBaseDir    = defaultFixedCloneBaseDir
)

// SetCloneBaseDirMode configures how default clone destinations are derived when no workdir is provided.
// - useFixed=true: clone under fixedBaseDir (e.g. /tmp/gitops-repo/<repo>)
// - useFixed=false: clone under a unique temp dir (supports parallel runs)
func SetCloneBaseDirMode(useFixed bool, fixedBaseDir string) {
	useFixedCloneBaseDir = useFixed
	if fixedBaseDir != "" {
		fixedCloneBaseDir = fixedBaseDir
	}
}

func resolveDefaultCloneBaseDir() (string, error) {
	if useFixedCloneBaseDir {
		return fixedCloneBaseDir, nil
	}

	tempDir, err := os.MkdirTemp("", "gitops-repo-")
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to create temporary clone base directory")
	}

	return tempDir, nil
}

// Repo handles Git repository operations for GitOps
// Transparently handles both git repositories and plain directories
type Repo struct {
	URL          string
	Ref          string
	Path         string   // Path within the repo (subdirectory)
	Watch        []string // Watch list for file monitoring
	WorkDir      string
	Name         string
	Organisation string
	Branch       string            // Current branch name
	Commit       string            // Current commit hash
	IsBare       bool              // Whether this is a bare repository
	IsMirror     bool              // Whether this is a mirror repository
	repository   *uRepo.Repository // Low-level git repo (nil if not a git repo)
	isGitRepo    bool              // Whether this is a git repository
	config       *uRepo.RepoConfig
}

// Note: Caching is handled by internal/utils/repo package
// We delegate to uRepo.IsCached(), uRepo.GetCachedRepo(), uRepo.CacheRepo()

// NewRepo creates a new repository instance
func NewRepo(url, ref, workDir string, config *uRepo.RepoConfig) *Repo {
	if config == nil {
		logger := logging.NewLogger(logging.INFO)
		config = &uRepo.RepoConfig{
			Logger:          logger,
			ProgressHandler: uRepo.NewLogProgressHandler(logger),
		}
	}

	// Ensure progress handler is set if not provided
	if config.ProgressHandler == nil && config.Logger != nil {
		config.ProgressHandler = uRepo.NewLogProgressHandler(config.Logger)
	}

	repo := &Repo{
		URL:     url,
		Ref:     ref,
		WorkDir: workDir,
		config:  config,
	}

	repo.parseURL()
	return repo
}

// NewRepoFromDesiredState creates a repository from desired state content
func NewRepoFromDesiredState(content map[string]interface{}, item, workDir, refOverride string, config *uRepo.RepoConfig) (*Repo, error) {
	if config == nil {
		logger := logging.NewLogger(logging.INFO)
		config = &uRepo.RepoConfig{
			Logger:          logger,
			ProgressHandler: uRepo.NewLogProgressHandler(logger),
		}
	}

	// Ensure progress handler is set if not provided
	if config.ProgressHandler == nil && config.Logger != nil {
		config.ProgressHandler = uRepo.NewLogProgressHandler(config.Logger)
	}

	repo := &Repo{
		WorkDir: workDir,
		config:  config,
	}

	// Parse repo info from desired state
	err := repo.initFromDesiredState(content, item)
	if err != nil {
		return nil, err
	}

	// Determine the ref to use
	if refOverride != "" {
		repo.Ref = refOverride
	}

	// Try to load from existing workdir first
	if workDir != "" {
		repo.Load()
	}

	// If not loaded as git repo, try to clone
	// Let Clone() handle the check - it knows if caching prevents the need to clone
	if !repo.isGitRepo {
		err = repo.Clone()
		if err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to clone repository")
		}
	}

	// Only checkout if a ref override was explicitly provided
	// Otherwise, use whatever ref the loaded/cached/cloned repo has
	// This avoids checkout errors when the cached repo has uncommitted changes
	if refOverride != "" {
		err = repo.Checkout()
		if err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to checkout ref")
		}
	}

	return repo, nil
}

// NewRepoFromWorkDir creates a new Repo from an existing working directory
func NewRepoFromWorkDir(workDir string, refOverride string, config *uRepo.RepoConfig) (*Repo, error) {
	// Fail-fast: validate required parameters at entry point
	if workDir == "" {
		return nil, errors.NewParamError("workDir cannot be empty")
	}

	if config == nil {
		logger := logging.NewLogger(logging.INFO)
		config = &uRepo.RepoConfig{
			Logger:          logger,
			ProgressHandler: uRepo.NewLogProgressHandler(logger),
		}
	}

	// Ensure progress handler is set if not provided
	if config.ProgressHandler == nil && config.Logger != nil {
		config.ProgressHandler = uRepo.NewLogProgressHandler(config.Logger)
	}

	repo := &Repo{
		WorkDir: workDir,
		Ref:     refOverride,
		config:  config,
	}

	// Try to load as git repo (transparently handles non-git directories)
	repo.Load()

	// Only checkout if a ref was explicitly provided
	// When loading from existing workdir, we use what's already checked out
	if refOverride != "" {
		err := repo.Checkout()
		if err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to checkout ref")
		}
	}

	// Initialize from git config - method handles preconditions
	err := repo.InitFromGitConfig()
	if err != nil {
		// Don't fail on InitFromGitConfig - it's not critical
		repo.config.Logger.Debug("Could not initialize from git config: %v", err)
	}

	return repo, nil
}

// Load attempts to load a directory as a Git repository
// Transparently falls back to plain filesystem handling if not a git repo
// Returns true if loaded as git repo, false otherwise
func (r *Repo) Load() bool {
	if r.WorkDir == "" {
		return false
	}

	r.config.Logger.Debug("Loading directory: %s", r.WorkDir)

	// Try to load as git repository
	repository, isGit, err := uRepo.Load(r.WorkDir, r.config)
	if err != nil {
		r.config.Logger.Warn("Error loading directory: %v", err)
		return false
	}

	if isGit {
		// Successfully loaded as git repo
		r.repository = repository
		r.isGitRepo = true
		r.WorkDir = repository.GetPath()

		// Update all properties from git config
		r.updateProperties()

		// Cache this repo
		if r.URL != "" {
			r.setCache()
		}

		r.config.Logger.Debug("Successfully loaded git repository: %s (URL: %s)", r.WorkDir, r.URL)
		return true
	}

	// Not a git repo - transparently fall back to plain filesystem
	r.isGitRepo = false
	r.config.Logger.Debug("Using directory as plain filesystem (not a git repo): %s", r.WorkDir)
	return false
}

// Clone clones the repository if not already cached
// Uses caching system to avoid repeated clones
func (r *Repo) Clone() error {
	if r.URL == "" {
		return errors.NewParamError("repository URL not set")
	}

	r.config.Logger.Debug("Clone() called for URL: %s (ref: %s)", r.URL, r.Ref)
	r.config.Logger.Debug("Retrieving repo: '%s?ref=%s'", r.URL, r.Ref)

	// Check if already cached - avoid cloning again
	if r.isCached() {
		r.config.Logger.Debug("Using cached repository instead of cloning: %s", r.URL)
		r.WorkDir = r.getCache()

		// Load from cache
		repository, err := uRepo.NewRepository(r.WorkDir, r.config)
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to load cached repository")
		}

		r.repository = repository
		r.isGitRepo = true

		r.config.Logger.Info("Successfully loaded repository %s (ref: %s) from cache: %s", r.URL, r.Ref, r.WorkDir)
		return nil
	}

	// Not cached - need to clone
	r.config.Logger.Info("Cloning repository: %s (ref: %s)", r.URL, r.Ref)

	// Determine work directory for clone
	workDir := r.WorkDir
	if workDir == "" {
		baseDir, err := resolveDefaultCloneBaseDir()
		if err != nil {
			return err
		}
		workDir = baseDir
	}

	r.config.Logger.Info("Cloning to directory: %s", workDir)

	// Clone the repository
	var repository *uRepo.Repository
	var err error

	if r.Ref != "" {
		repository, err = uRepo.CloneBranch(r.URL, workDir, r.Ref, false, r.config)
	} else {
		repository, err = uRepo.Clone(r.URL, workDir, false, r.config)
	}

	if err != nil {
		r.config.Logger.Error("Failed to clone repository %s: %v", r.URL, err)
		return errors.Wrapf(errors.ErrFail, err, "failed to clone repository")
	}

	r.repository = repository
	r.isGitRepo = true
	r.WorkDir = repository.GetPath()

	// Update properties from git
	r.updateProperties()

	// Cache this repo
	r.setCache()

	r.config.Logger.Info("Successfully cloned repository: %s -> %s", r.URL, r.WorkDir)
	return nil
}

// Checkout checks out the specified ref
// Handles all preconditions internally
func (r *Repo) Checkout() error {
	// Skip if no ref specified
	if r.Ref == "" {
		r.config.Logger.Debug("No ref specified, skipping checkout")
		return nil
	}

	// Warn if not a git repo but don't fail
	if !r.isGitRepo {
		r.config.Logger.Warn("Not a git repository, cannot checkout ref: %s", r.Ref)
		return nil
	}

	r.config.Logger.Info("Checking out ref: '%s?ref=%s'", r.URL, r.Ref)

	// If cached, reload from cache
	if r.isCached() {
		r.WorkDir = r.getCache()
		r.config.Logger.Debug("Checkout dir: '%s'", r.WorkDir)

		// Reload git object from cache
		repository, err := uRepo.NewRepository(r.WorkDir, r.config)
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to load repository from cache")
		}
		r.repository = repository
	}

	// Verify we have a repository loaded
	if r.repository == nil {
		return errors.New(errors.ErrFail, "repository not loaded, unable to checkout")
	}

	// Perform checkout
	err := r.repository.CheckoutBranch(r.Ref)
	if err != nil {
		r.config.Logger.Error("Failed to checkout ref %s: %v", r.Ref, err)
		return err
	}

	// Update properties after checkout
	r.updateProperties()

	r.config.Logger.Info("Successfully checked out ref: %s", r.Ref)
	return nil
}

// GetLocalPath returns the local path to the repository
func (r *Repo) GetLocalPath() string {
	return r.WorkDir
}

// GetFilePath returns the full path to a file within the repository
func (r *Repo) GetFilePath() string {
	if r.WorkDir == "" || r.Path == "" {
		return r.WorkDir
	}
	return filepath.Join(r.WorkDir, r.Path)
}

// GetWatchList returns the watch list for file monitoring
// Returns an empty slice if no watch list is configured
func (r *Repo) GetWatchList() []string {
	if r.Watch == nil {
		return []string{}
	}
	return r.Watch
}

// ValidateRemote checks if the remote repository is reachable
// Returns an error with details if the remote cannot be reached
func (r *Repo) ValidateRemote() error {
	if r.URL == "" {
		return errors.New(errors.ErrParam, "VALIDATION ERROR: repository URL is empty")
	}

	r.config.Logger.Debug("Validating remote accessibility for: %s", r.URL)

	// Use git ls-remote to check if remote is reachable
	// This is lightweight and doesn't clone the repository
	cmd := exec.Command("git", "ls-remote", "--heads", r.URL)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Parse the error to provide more helpful messages
		errorMsg := string(output)

		if strings.Contains(errorMsg, "not found") || strings.Contains(errorMsg, "does not exist") ||
			strings.Contains(errorMsg, "could not read Username") {
			// GitHub returns an ambiguous credential prompt over HTTPS for both private and non-existent repos.
			return errors.Newf(errors.ErrFail, "VALIDATION ERROR: repository not found: %s\nPlease verify the URL is correct and you have access", r.URL)
		}
		if strings.Contains(errorMsg, "Permission denied") || strings.Contains(errorMsg, "Authentication failed") {
			return errors.Newf(errors.ErrFail, "VALIDATION ERROR: authentication failed for: %s\nPlease check your SSH keys or credentials", r.URL)
		}
		if strings.Contains(errorMsg, "Could not resolve host") {
			return errors.Newf(errors.ErrFail, "VALIDATION ERROR: cannot resolve host for: %s\nPlease check your network connection and URL", r.URL)
		}

		// Generic error
		return errors.Newf(errors.ErrFail, "VALIDATION ERROR: remote repository not accessible: %s\nError: %v\nOutput: %s", r.URL, err, errorMsg)
	}

	r.config.Logger.Debug("Remote repository validated successfully: %s", r.URL)
	return nil
}

// ValidateRef checks if the specified ref/branch/tag exists on the remote
// Returns an error if the ref doesn't exist or cannot be validated
func (r *Repo) ValidateRef() error {
	if r.URL == "" {
		return errors.New(errors.ErrParam, "VALIDATION ERROR: repository URL is empty, cannot validate ref")
	}

	if r.Ref == "" {
		// No ref specified, will use default branch - this is valid
		r.config.Logger.Debug("No ref specified, will use repository default branch")
		return nil
	}

	r.config.Logger.Debug("Validating ref '%s' exists on remote: %s", r.Ref, r.URL)

	// Use git ls-remote to check if ref exists
	// Try multiple formats: refs/heads/{ref}, refs/tags/{ref}, and direct ref
	cmd := exec.Command("git", "ls-remote", r.URL, r.Ref)
	output, err := cmd.CombinedOutput()

	if err != nil {
		errorMsg := string(output)
		return errors.Newf(errors.ErrFail, "VALIDATION ERROR: failed to validate ref '%s' on %s\nError: %v\nOutput: %s", r.Ref, r.URL, err, errorMsg)
	}

	// Check if we got any output - empty output means ref doesn't exist
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		// Try checking as a branch
		cmdBranch := exec.Command("git", "ls-remote", "--heads", r.URL, fmt.Sprintf("refs/heads/%s", r.Ref))
		outputBranch, errBranch := cmdBranch.CombinedOutput()

		if errBranch == nil && strings.TrimSpace(string(outputBranch)) != "" {
			r.config.Logger.Debug("Ref '%s' validated as branch on remote", r.Ref)
			return nil
		}

		// Try checking as a tag
		cmdTag := exec.Command("git", "ls-remote", "--tags", r.URL, fmt.Sprintf("refs/tags/%s", r.Ref))
		outputTag, errTag := cmdTag.CombinedOutput()

		if errTag == nil && strings.TrimSpace(string(outputTag)) != "" {
			r.config.Logger.Debug("Ref '%s' validated as tag on remote", r.Ref)
			return nil
		}

		return errors.Newf(errors.ErrFail, "VALIDATION ERROR: ref '%s' not found on remote: %s\nPlease verify the branch or tag name is correct\nAvailable refs can be listed with: git ls-remote %s", r.Ref, r.URL, r.URL)
	}

	r.config.Logger.Debug("Ref '%s' validated successfully on remote", r.Ref)
	return nil
}

// Validate performs comprehensive validation before clone/checkout operations
// Checks remote accessibility and ref existence
// Returns the first error encountered, or nil if all validations pass
func (r *Repo) Validate() error {
	r.config.Logger.Debug("Starting repository validation for: %s", r.URL)

	// First validate remote is accessible
	if err := r.ValidateRemote(); err != nil {
		return err
	}

	// Then validate ref exists (if specified)
	if err := r.ValidateRef(); err != nil {
		return err
	}

	r.config.Logger.Info("Repository validation passed: url=%s, ref=%s", r.URL, r.Ref)
	return nil
}

// IsGitRepo returns whether this is a git repository
func (r *Repo) IsGitRepo() bool {
	return r.isGitRepo
}

// CloneBare clones the repository as a bare repository
// Bare repositories don't have a working directory, only .git content
func (r *Repo) CloneBare() error {
	if r.URL == "" {
		return errors.NewParamError("repository URL not set")
	}

	r.config.Logger.Info("Cloning bare repository: %s", r.URL)

	// Determine work directory for clone
	workDir := r.WorkDir
	if workDir == "" {
		// Use temp directory with .bare suffix
		workDir = filepath.Join("/tmp", "gitops-repo", r.Name+".bare")
	}

	// Clone as bare
	repository, err := uRepo.CloneBare(r.URL, workDir, false, r.config)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to clone bare repository")
	}

	r.repository = repository
	r.isGitRepo = true
	r.IsBare = true
	r.WorkDir = repository.GetPath()

	// Update properties from git
	r.updateProperties()

	// Cache this repo
	r.setCache()

	r.config.Logger.Info("Successfully cloned bare repository: %s -> %s", r.URL, r.WorkDir)
	return nil
}

// CloneMirror clones the repository as a mirror
// Mirror repositories include all refs and are suitable for backup/mirroring
func (r *Repo) CloneMirror() error {
	if r.URL == "" {
		return errors.NewParamError("repository URL not set")
	}

	r.config.Logger.Info("Cloning mirror repository: %s", r.URL)

	// Determine work directory for clone
	workDir := r.WorkDir
	if workDir == "" {
		// Use temp directory with .bare suffix (mirrors are also bare)
		workDir = filepath.Join("/tmp", "gitops-repo", r.Name+".bare")
	}

	// Clone as mirror
	repository, err := uRepo.CloneMirror(r.URL, workDir, false, r.config)
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to clone mirror repository")
	}

	r.repository = repository
	r.isGitRepo = true
	r.IsBare = true
	r.IsMirror = true
	r.WorkDir = repository.GetPath()

	// Update properties from git
	r.updateProperties()

	// Cache this repo
	r.setCache()

	r.config.Logger.Info("Successfully cloned mirror repository: %s -> %s", r.URL, r.WorkDir)
	return nil
}

// Bundle creates a git bundle from a bare repository
// Returns the path to the created bundle file
func (r *Repo) Bundle(destDir string) (string, error) {
	if !r.IsBare {
		return "", errors.New(errors.ErrFail, "repository is not bare, cannot create bundle")
	}

	if !r.isGitRepo || r.repository == nil {
		return "", errors.New(errors.ErrFail, "repository not loaded")
	}

	return r.repository.Bundle(destDir)
}

// isCached checks whether this repo URL was already cloned
// Delegates to low-level uRepo.IsCached()
func (r *Repo) isCached() bool {
	exists := uRepo.IsCached(r.URL)
	r.config.Logger.Debug("Repo is %scached: %s", map[bool]string{true: "", false: "not "}[exists], r.URL)
	return exists
}

// getCache retrieves the cached workdir for this repo URL
// Delegates to low-level uRepo.GetCachedRepo()
func (r *Repo) getCache() string {
	workdir, _ := uRepo.GetCachedRepo(r.URL)
	r.config.Logger.Debug("Repo retrieved from cache: %s --> %s", r.URL, workdir)
	return workdir
}

// setCache stores the workdir for this repo URL in the cache
// Delegates to low-level uRepo.CacheRepo()
func (r *Repo) setCache() {
	uRepo.CacheRepo(r.URL, r.WorkDir)
	r.config.Logger.Debug("Repo added to cache: %s --> %s", r.URL, r.WorkDir)
}

// updateProperties updates git properties for current checked out workdir
// This method is defensive - it logs warnings for individual failures rather than aborting
func (r *Repo) updateProperties() error {
	if !r.isGitRepo || r.repository == nil {
		r.config.Logger.Debug("Not a git repository, skipping property update")
		return nil
	}

	// Get URL - log warning if it fails but continue
	url, err := r.repository.GetRemoteURL()
	if err != nil {
		r.config.Logger.Warn("Failed to get remote URL: %v", err)
	} else {
		r.URL = url
	}

	// Sync bare/mirror status from low-level repository
	r.IsBare = r.repository.IsBare()
	r.IsMirror = r.repository.IsMirror()

	// Skip branch/commit detection for bare repos
	if r.IsBare {
		r.config.Logger.Debug("Bare repo, not loading branch/commit properties")
		r.config.Logger.Debug("Repo properties: url=%s, bare=%v, mirror=%v", r.URL, r.IsBare, r.IsMirror)
		return nil
	}

	// Get current branch - handle detached HEAD state gracefully
	branch, err := r.repository.GetCurrentBranch()
	if err == nil {
		r.Branch = branch
		// Only set Ref from branch if Ref wasn't explicitly provided
		if r.Ref == "" {
			r.Ref = branch
		}
	} else {
		r.config.Logger.Debug("Unable to detect repo branch (may be in detached HEAD state): %v", err)
	}

	// Get current commit with fallback to subprocess if go-git fails
	commit, err := r.repository.GetCurrentCommit()
	if err != nil {
		r.config.Logger.Warn("Failed to get commit via go-git, attempting subprocess fallback: %v", err)

		// Try subprocess as fallback
		cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
		cmd.Dir = r.WorkDir
		output, cmdErr := cmd.CombinedOutput()

		if cmdErr != nil {
			r.config.Logger.Warn("Failed to get commit via subprocess: %v", cmdErr)
		} else {
			commit = strings.TrimSpace(string(output))
			r.Commit = commit
		}
	} else {
		r.Commit = commit
	}

	r.config.Logger.Debug("Repo properties: url=%s, ref=%s, branch=%s, commit=%s, bare=%v, mirror=%v",
		r.URL, r.Ref, r.Branch, r.Commit, r.IsBare, r.IsMirror)
	return nil
}

// InitFromGitConfig initializes repository info from git config
func (r *Repo) InitFromGitConfig() error {
	if !r.isGitRepo || r.repository == nil {
		return errors.New(errors.ErrFail, "not a git repository")
	}

	// Get remote URL
	url, err := r.repository.GetRemoteURL()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to get remote URL")
	}

	r.URL = url
	r.parseURL()

	// Get current branch (for informational purposes)
	// Don't set r.Ref - that would trigger checkout attempts
	branch, err := r.repository.GetCurrentBranch()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to get current branch")
	}

	// Only set Ref if it wasn't already set
	// This preserves explicitly provided refs and avoids setting refs when loading from workdir
	if r.Ref == "" {
		r.Ref = branch
	}

	return nil
}

// parseURL extracts organization and name from Git URL
// Uses low-level parsing functions
func (r *Repo) parseURL() {
	if r.URL == "" {
		return
	}

	r.Name = uRepo.ParseRepoName(r.URL)
	r.Organisation = uRepo.ParseRepoOrganisation(r.URL)
}

// initFromDesiredState initializes repository from desired state content
func (r *Repo) initFromDesiredState(content map[string]interface{}, item string) error {
	r.config.Logger.Debug("Initializing repo from desired state, item path: '%s'", item)

	// Extract schema version once for all parse operations
	versionStr, err := extractSchemaVersion(content)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "CONFIGURATION ERROR: cannot determine schema version")
	}
	version := schema.SchemaVersion(versionStr)

	// Extract namespace from content (defaults to "yago" if not specified)
	namespace := extractNamespace(content)

	// Get PropertyPaths once and reuse for all field parsing (avoids duplicate log messages)
	handler := parser.NewYAMLHandler("")
	propertyPaths, err := handler.GetPropertyPathsWithNamespace(namespace, version, true)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "CONFIGURATION ERROR: failed to get property paths for schema %s", version)
	}

	// Parse URL - FAIL if not found (required field)
	url, err := parseRepoURL(content, item, propertyPaths, r.config)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "CONFIGURATION ERROR: failed to parse repository URL")
	}
	r.URL = url
	r.config.Logger.Debug("Parsed repo URL: %s", r.URL)

	// Parse ref/tag/branch with precedence - FAIL if nothing found
	ref, err := parseRepoRef(content, item, propertyPaths, r.config)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "CONFIGURATION ERROR: failed to parse repository ref/tag/branch")
	}
	r.Ref = ref
	r.config.Logger.Debug("Parsed repo ref: %s", r.Ref)

	// Parse path (optional - can be empty)
	path, err := parseRepoPath(content, item, propertyPaths, r.config)
	if err != nil {
		// Path errors are not fatal, just log
		r.config.Logger.Debug("Could not parse repo path (optional field): %v", err)
		path = ""
	}
	r.Path = path
	if path != "" {
		r.config.Logger.Debug("Parsed repo path: %s", r.Path)
	}

	// Parse watch list (optional - can be empty)
	watch, err := parseRepoWatch(content, item, propertyPaths, r.config)
	if err != nil {
		// Watch list errors are not fatal, just log
		r.config.Logger.Debug("Could not parse repo watch list (optional field): %v", err)
		watch = []string{}
	}
	r.Watch = watch
	if len(watch) > 0 {
		r.config.Logger.Debug("Parsed repo watch list with %d entries", len(watch))
	}

	r.parseURL()
	r.config.Logger.Info("Initialized repo from desired state: url=%s, name=%s, org=%s, ref=%s, path=%s, watch=%d entries",
		r.URL, r.Name, r.Organisation, r.Ref, r.Path, len(r.Watch))

	return nil
}

// ParseDesiredStateRef parses a desired state reference and returns absolute path and relative path
func ParseDesiredStateRef(dictionary map[string]interface{}, config *uRepo.RepoConfig) (string, string, error) {
	// Parse the repository configuration
	repo, err := NewRepoFromDesiredState(dictionary, "", "", "", config)
	if err != nil {
		return "", "", err
	}

	return repo.GetLocalPath(), repo.Path, nil
}

// Helper functions for parsing repository configuration with schema integration and fail-fast

// parseRepoURL extracts repository URL from configuration using schema-based paths
// Returns error if URL not found - NO DEFAULTS (fail-fast)
func parseRepoURL(content map[string]interface{}, item string, propertyPaths *schema.PropertyPaths, config *uRepo.RepoConfig) (string, error) {
	// Get the schema-based path to the URL field
	// The urlPath returned is just the field name (e.g., "url") since we pass empty repoPath
	urlPath := propertyPaths.GetRepoUrlPath("")

	config.Logger.Debug("Using schema-based URL field: '%s' for item '%s'", urlPath, item)

	// Navigate to repo config using item path if provided
	repoConfig := content
	if item != "" {
		yamlHandler := parser.NewYAMLHandler("")
		value, err := yamlHandler.GetValue(content, item)
		if err != nil {
			return "", errors.Wrapf(errors.ErrParse, err, "failed to navigate to repo config at path '%s'", item)
		}

		var ok bool
		repoConfig, ok = value.(map[string]interface{})
		if !ok {
			return "", errors.Newf(errors.ErrParse, "repo config at path '%s' is not a valid object", item)
		}
	}

	// Extract the URL field name from the schema path (it's just the field name)
	urlField := urlPath

	if url, exists := repoConfig[urlField]; exists {
		if urlStr, ok := url.(string); ok && urlStr != "" {
			return urlStr, nil
		}
		return "", errors.Newf(errors.ErrParse, "repo URL at field '%s' is not a valid string", urlField)
	}

	// FAIL-FAST: URL is required, no defaults
	return "", errors.Newf(errors.ErrParse, "repo URL field '%s' not found at item '%s' (required by schema)",
		urlField, item)
}

// parseRepoRef extracts repository ref/tag/branch from configuration using schema-based paths
// Precedence: ref → tag → branch
// Returns error if NONE found - NO DEFAULTS (fail-fast)
func parseRepoRef(content map[string]interface{}, item string, propertyPaths *schema.PropertyPaths, config *uRepo.RepoConfig) (string, error) {
	// Navigate to repo config using item path if provided
	repoConfig := content
	if item != "" {
		yamlHandler := parser.NewYAMLHandler("")
		value, err := yamlHandler.GetValue(content, item)
		if err != nil {
			return "", errors.Wrapf(errors.ErrParse, err, "failed to navigate to repo config at path '%s'", item)
		}

		var ok bool
		repoConfig, ok = value.(map[string]interface{})
		if !ok {
			return "", errors.Newf(errors.ErrParse, "repo config at path '%s' is not a valid object", item)
		}
	}

	// Try ref field first (highest precedence)
	refField := propertyPaths.GetRepoRefPath("")
	if ref, exists := repoConfig[refField]; exists {
		if refStr, ok := ref.(string); ok && refStr != "" {
			config.Logger.Debug("Using ref field '%s' = '%s' for item '%s'", refField, refStr, item)
			return refStr, nil
		}
	}
	config.Logger.Debug("Ref field '%s' not found or empty, trying tag...", refField)

	// Try tag field (medium precedence)
	tagField := propertyPaths.GetRepoTagPath("")
	if tag, exists := repoConfig[tagField]; exists {
		if tagStr, ok := tag.(string); ok && tagStr != "" {
			config.Logger.Debug("Using tag field '%s' = '%s' for item '%s'", tagField, tagStr, item)
			return tagStr, nil
		}
	}
	config.Logger.Debug("Tag field '%s' not found or empty, trying branch...", tagField)

	// Try branch field (lowest precedence)
	branchField := propertyPaths.GetRepoBranchPath("")
	if branch, exists := repoConfig[branchField]; exists {
		if branchStr, ok := branch.(string); ok && branchStr != "" {
			config.Logger.Debug("Using branch field '%s' = '%s' for item '%s'", branchField, branchStr, item)
			return branchStr, nil
		}
	}
	config.Logger.Debug("Branch field '%s' not found or empty", branchField)

	// FAIL-FAST: No ref/tag/branch found - this is a configuration error!
	// NO DEFAULT to "main" - fail loudly instead
	return "", errors.Newf(errors.ErrParse, "CONFIGURATION INCOMPLETE: no ref, tag, or branch specified for repository at item '%s'", item)
}

// parseRepoPath extracts repository path from configuration using schema-based paths
// Path is optional - returns empty string if not found (no error)
func parseRepoPath(content map[string]interface{}, item string, propertyPaths *schema.PropertyPaths, config *uRepo.RepoConfig) (string, error) {
	// Get the schema-based path to the path field
	pathField := propertyPaths.GetRepoPathPath("")

	// Navigate to repo config using item path if provided
	repoConfig := content
	if item != "" {
		yamlHandler := parser.NewYAMLHandler("")
		value, err := yamlHandler.GetValue(content, item)
		if err != nil {
			return "", errors.Wrapf(errors.ErrParse, err, "failed to navigate to repo config at path '%s'", item)
		}

		var ok bool
		repoConfig, ok = value.(map[string]interface{})
		if !ok {
			return "", errors.Newf(errors.ErrParse, "repo config at path '%s' is not a valid object", item)
		}
	}

	if path, exists := repoConfig[pathField]; exists {
		if pathStr, ok := path.(string); ok {
			config.Logger.Debug("Using path field '%s' = '%s' for item '%s'", pathField, pathStr, item)
			return pathStr, nil
		}
	}

	// Path not found or invalid - this is OK, it's optional
	config.Logger.Debug("Path field '%s' not found or invalid, treating as empty", pathField)
	return "", nil
}

// parseRepoWatch extracts repository watch list from configuration using schema-based paths
// Watch list is optional - returns empty list if not found (no error)
func parseRepoWatch(content map[string]interface{}, item string, propertyPaths *schema.PropertyPaths, config *uRepo.RepoConfig) ([]string, error) {
	// Get the schema-based path to the watch field
	watchField := propertyPaths.GetRepoWatchPath("")

	// Navigate to repo config using item path if provided
	repoConfig := content
	if item != "" {
		yamlHandler := parser.NewYAMLHandler("")
		value, err := yamlHandler.GetValue(content, item)
		if err != nil {
			return []string{}, errors.Wrapf(errors.ErrParse, err, "failed to navigate to repo config at path '%s'", item)
		}

		var ok bool
		repoConfig, ok = value.(map[string]interface{})
		if !ok {
			return []string{}, errors.Newf(errors.ErrParse, "repo config at path '%s' is not a valid object", item)
		}
	}

	if watchVal, exists := repoConfig[watchField]; exists {
		// Watch list should be an array
		if watchList, ok := watchVal.([]interface{}); ok {
			result := make([]string, 0, len(watchList))
			for i, item := range watchList {
				if watchItem, ok := item.(string); ok {
					result = append(result, watchItem)
				} else {
					config.Logger.Debug("Watch list item %d is not a string, skipping", i)
				}
			}
			config.Logger.Debug("Using watch field '%s' with %d entries for item '%s'", watchField, len(result), item)
			return result, nil
		}
		config.Logger.Debug("Watch field '%s' is not an array, treating as empty", watchField)
	}

	// Watch list not found or invalid - this is OK, it's optional
	config.Logger.Debug("Watch field '%s' not found, treating as empty list", watchField)
	return []string{}, nil
}

// extractSchemaVersion extracts the schema version from content
func extractSchemaVersion(content map[string]interface{}) (string, error) {
	// Look for schema field (GitOps standard location)
	if schemaVal, exists := content["schema"]; exists {
		if versionStr, ok := schemaVal.(string); ok && versionStr != "" {
			return versionStr, nil
		}
	}

	// Fallback: check for schemaVersion field (alternative location)
	if schemaVersion, exists := content["schemaVersion"]; exists {
		if versionStr, ok := schemaVersion.(string); ok && versionStr != "" {
			return versionStr, nil
		}
	}

	// Fallback: check for version field
	if version, exists := content["version"]; exists {
		if versionStr, ok := version.(string); ok && versionStr != "" {
			return versionStr, nil
		}
	}

	return "", errors.New(errors.ErrParse, "no schema, schemaVersion, or version field found in content - cannot determine schema")
}

// extractNamespace extracts the namespace from content (defaults to "yago" if not found)
func extractNamespace(content map[string]interface{}) string {
	// Look for namespace field (GitOps standard location)
	if nsVal, exists := content["namespace"]; exists {
		if nsStr, ok := nsVal.(string); ok && nsStr != "" {
			return nsStr
		}
	}

	// Default to "yago" namespace if not specified
	return "yago"
}
