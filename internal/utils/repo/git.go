package repo

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Global cache for cloned repositories - URL to workdir mapping
var (
	cachedWorkDirs = make(map[string]string)
	cacheMutex     sync.RWMutex
)

// GetCachedRepo retrieves a cached repository path by URL
func GetCachedRepo(url string) (string, bool) {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	path, found := cachedWorkDirs[url]
	logging.Debug("Repo cache lookup: %s -> %s (found: %v)", url, path, found)
	return path, found
}

// CacheRepo adds a repository to the cache
func CacheRepo(url, workDir string) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	cachedWorkDirs[url] = workDir
	logging.Debug("Repo added to cache: %s -> %s", url, workDir)
}

// IsCached checks if a repository URL is in the cache
func IsCached(url string) bool {
	_, found := GetCachedRepo(url)
	return found
}

// ProgressHandler defines the interface for handling git operation progress
type ProgressHandler interface {
	io.Writer
}

// LogProgressHandler implements ProgressHandler using logger
// This provides simple progress feedback via log messages
type LogProgressHandler struct {
	logger *logging.Logger
}

// NewLogProgressHandler creates a new log-based progress handler
func NewLogProgressHandler(logger *logging.Logger) *LogProgressHandler {
	return &LogProgressHandler{logger: logger}
}

// Write implements io.Writer to receive progress updates
func (p *LogProgressHandler) Write(data []byte) (n int, err error) {
	// Filter out empty lines and format for logging
	msg := strings.TrimSpace(string(data))
	if msg != "" {
		p.logger.Debug("[git] %s", msg)
	}
	return len(data), nil
}

// isUnsafeRepositoryError checks if an error is related to unsafe repository
// This happens commonly in Docker/container environments when mounting volumes
func isUnsafeRepositoryError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "unsafe repository") ||
		strings.Contains(errMsg, "dubious ownership")
}

// markDirectoryAsSafe adds a directory to git's safe.directory config
// This is needed when working with repositories mounted as Docker volumes
func markDirectoryAsSafe(path string, logger *logging.Logger) error {
	logger.Debug("Git work directory %s detected as unsafe, marking as safe (common with Docker volumes)", path)

	cmd := exec.Command("git", "config", "--global", "--add", "safe.directory", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to mark directory as safe (output: %s)", string(output))
	}

	logger.Debug("Successfully marked directory as safe: %s", path)
	return nil
}

// withSafeDirectoryRetry executes a function and retries once if it fails with unsafe repository error
// Automatically marks the directory as safe before retrying
func withSafeDirectoryRetry(path string, logger *logging.Logger, fn func() error) error {
	err := fn()
	if err != nil && isUnsafeRepositoryError(err) {
		// Try to mark as safe and retry
		if markErr := markDirectoryAsSafe(path, logger); markErr != nil {
			// If we can't mark as safe, return original error
			return err
		}
		// Retry the operation
		return fn()
	}
	return err
}

// cloneWithGitCLI clones repositories using native git command.
// This delegates all auth/transport behavior to the host environment and git configuration.
func cloneWithGitCLI(url, path, branch string, bare, mirror bool, logger *logging.Logger) error {
	args := []string{"-c", "color.ui=always", "clone", "--progress"}

	if mirror {
		args = append(args, "--mirror")
	} else if bare {
		args = append(args, "--bare")
	}

	if branch != "" {
		args = append(args, "--branch", branch, "--single-branch")
	}

	args = append(args, url, path)

	logger.Info("Cloning repository with native git: git %s", strings.Join(args, " "))
	cmd := exec.Command("git", args...)

	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)

	err := cmd.Run()
	if err != nil {
		combined := strings.TrimSpace(stdoutBuf.String() + "\n" + stderrBuf.String())
		return errors.Wrapf(errors.ErrFail, err, "failed to clone with native git (output: %s)", combined)
	}

	return nil
}

// RepoConfig holds configuration for repository operations
type RepoConfig struct {
	Username        string
	Token           string
	Logger          *logging.Logger
	ProgressHandler ProgressHandler // Optional progress handler for git operations
}

// Repository represents a Git repository
type Repository struct {
	repo     *git.Repository
	config   *RepoConfig
	path     string
	isBare   bool // Whether this is a bare repository
	isMirror bool // Whether this is a mirror repository
}

// NewRepository creates a new repository instance from an existing repo
func NewRepository(path string, config *RepoConfig) (*Repository, error) {
	if config == nil {
		config = &RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}
	}

	var repo *git.Repository
	var err error

	// Use safe directory retry wrapper
	err = withSafeDirectoryRetry(path, config.Logger, func() error {
		repo, err = git.PlainOpen(path)
		return err
	})

	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to open repository at %s", path)
	}

	// Detect if this is a bare repository
	isBare := false
	if cfg, err := repo.Config(); err == nil {
		isBare = cfg.Core.IsBare
	}

	return &Repository{
		repo:     repo,
		config:   config,
		path:     path,
		isBare:   isBare,
		isMirror: false, // Mirror detection requires checking remote config
	}, nil
}

// Load tries to load a directory as a Git repository
// Returns (repo, true, nil) if successful
// Returns (nil, false, nil) if not a git repo (logs warning)
// Returns (nil, false, err) for actual errors
func Load(workDir string, config *RepoConfig) (*Repository, bool, error) {
	if config == nil {
		config = &RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}
	}

	if workDir == "" {
		return nil, false, errors.NewParamError("workdir not specified")
	}

	// Expand and normalize path
	absPath, err := filepath.Abs(os.ExpandEnv(workDir))
	if err != nil {
		absPath = workDir
	}

	config.Logger.Info("Loading repo: '%s'", absPath)

	// Try to open as a git repository with safe directory retry
	var repo *git.Repository
	err = withSafeDirectoryRetry(absPath, config.Logger, func() error {
		var openErr error
		repo, openErr = git.PlainOpen(absPath)
		return openErr
	})

	if err != nil {
		// Not a git repo - this is expected behavior, just warn
		config.Logger.Warn("Not a Git repo, skipping: %s", absPath)
		return nil, false, nil
	}

	// Successfully loaded as git repo
	// Detect if this is a bare repository
	isBare := false
	if cfg, err := repo.Config(); err == nil {
		isBare = cfg.Core.IsBare
	}

	repository := &Repository{
		repo:     repo,
		config:   config,
		path:     absPath,
		isBare:   isBare,
		isMirror: false,
	}

	// Add to cache if we loaded successfully
	if url, err := repository.GetRemoteURL(); err == nil && url != "" {
		CacheRepo(url, absPath)
	}

	return repository, true, nil
}

// Clone clones a repository to the specified path
// Checks cache first to avoid duplicate clones unless force is true
// If force is true, bypasses cache and clones even if already cached
func Clone(url, path string, force bool, config *RepoConfig) (*Repository, error) {
	if config == nil {
		config = &RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}
	}

	// Check cache first (unless force is specified)
	if !force {
		if cachedPath, found := GetCachedRepo(url); found {
			config.Logger.Info("Repository already cached: %s -> %s", url, cachedPath)
			return NewRepository(cachedPath, config)
		}
	} else {
		config.Logger.Info("Force clone enabled, bypassing cache")
	}

	config.Logger.Info("Cloning repository from %s to %s", url, path)

	// Apply namedWorkDir logic - append repo name to path if needed
	path = NamedWorkDir(path, url, false, false, true)
	config.Logger.Debug("Work dir after namedWorkDir: '%s'", path)

	// Ensure directory exists
	if path == "" {
		return nil, errors.NewParamError("path cannot be empty")
	}

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create parent directory")
	}

	if err := cloneWithGitCLI(url, path, "", false, false, config.Logger); err != nil {
		return nil, err
	}

	config.Logger.Info("Repository cloned successfully")

	// Add to cache
	CacheRepo(url, path)
	return NewRepository(path, config)
}

// CloneBranch clones a specific branch of a repository
// Checks cache first to avoid duplicate clones unless force is true
// If force is true, bypasses cache and clones even if already cached
func CloneBranch(url, path, branch string, force bool, config *RepoConfig) (*Repository, error) {
	if config == nil {
		config = &RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}
	}

	// Check cache first (unless force is specified)
	cacheKey := fmt.Sprintf("%s#%s", url, branch)
	if !force {
		if cachedPath, found := GetCachedRepo(cacheKey); found {
			config.Logger.Info("Repository branch already cached: %s#%s -> %s", url, branch, cachedPath)
			repo, err := NewRepository(cachedPath, config)
			if err != nil {
				return nil, err
			}
			// Ensure we're on the right branch
			currentBranch, _ := repo.GetCurrentBranch()
			if currentBranch != branch {
				if err := repo.CheckoutBranch(branch); err != nil {
					return nil, errors.Wrapf(errors.ErrFail, err, "failed to checkout cached branch %s", branch)
				}
			}
			return repo, nil
		}
	} else {
		config.Logger.Info("Force clone enabled, bypassing cache")
	}

	config.Logger.Info("Cloning branch %s from %s to %s", branch, url, path)

	// Apply namedWorkDir logic - append repo name to path if needed
	path = NamedWorkDir(path, url, false, false, true)
	config.Logger.Debug("Work dir after namedWorkDir: '%s'", path)

	// Ensure directory exists
	if path == "" {
		return nil, errors.NewParamError("path cannot be empty")
	}

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create parent directory")
	}

	if err := cloneWithGitCLI(url, path, branch, false, false, config.Logger); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to clone branch %s", branch)
	}

	config.Logger.Info("Branch %s cloned successfully", branch)

	// Add to cache with branch-specific key
	CacheRepo(cacheKey, path)

	return NewRepository(path, config)
}

// CloneBare clones a repository as a bare repository
// Bare repositories don't have a working directory, only .git content
// Useful for caching and CI/CD pipelines
// If force is true, bypasses cache and clones even if already cached
func CloneBare(url, path string, force bool, config *RepoConfig) (*Repository, error) {
	if config == nil {
		config = &RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}
	}

	// Check cache first (unless force is specified)
	cacheKey := fmt.Sprintf("%s#bare", url)
	if !force {
		if cachedPath, found := GetCachedRepo(cacheKey); found {
			config.Logger.Info("Bare repository already cached: %s -> %s", url, cachedPath)
			return NewRepository(cachedPath, config)
		}
	} else {
		config.Logger.Info("Force clone enabled, bypassing cache")
	}

	config.Logger.Info("Cloning bare repository from %s to %s", url, path)

	// Apply namedWorkDir logic - append repo name with .bare suffix
	path = NamedWorkDir(path, url, true, false, true)
	config.Logger.Debug("Work dir after namedWorkDir: '%s'", path)

	// Ensure directory exists
	if path == "" {
		return nil, errors.NewParamError("path cannot be empty")
	}

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create parent directory")
	}

	if err := cloneWithGitCLI(url, path, "", true, false, config.Logger); err != nil {
		return nil, err
	}

	config.Logger.Info("Bare repository cloned successfully")

	// Add to cache
	CacheRepo(cacheKey, path)

	return NewRepository(path, config)
}

// CloneMirror clones a repository as a mirror
// Mirror repositories include all refs and are suitable for backup/mirroring
// If force is true, bypasses cache and clones even if already cached
func CloneMirror(url, path string, force bool, config *RepoConfig) (*Repository, error) {
	if config == nil {
		config = &RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}
	}

	// Check cache first (unless force is specified)
	cacheKey := fmt.Sprintf("%s#mirror", url)
	if !force {
		if cachedPath, found := GetCachedRepo(cacheKey); found {
			config.Logger.Info("Mirror repository already cached: %s -> %s", url, cachedPath)
			return NewRepository(cachedPath, config)
		}
	} else {
		config.Logger.Info("Force clone enabled, bypassing cache")
	}

	config.Logger.Info("Cloning mirror repository from %s to %s", url, path)

	// Apply namedWorkDir logic - append repo name with .bare suffix (mirrors are bare)
	path = NamedWorkDir(path, url, true, true, true)
	config.Logger.Debug("Work dir after namedWorkDir: '%s'", path)

	// Ensure directory exists
	if path == "" {
		return nil, errors.NewParamError("path cannot be empty")
	}

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to create parent directory")
	}

	if err := cloneWithGitCLI(url, path, "", false, true, config.Logger); err != nil {
		return nil, err
	}

	config.Logger.Info("Mirror repository cloned successfully")

	// Add to cache
	CacheRepo(cacheKey, path)

	return NewRepository(path, config)
}

// GetCurrentBranch returns the current branch name
func (r *Repository) GetCurrentBranch() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get HEAD")
	}

	if head.Name().IsBranch() {
		return head.Name().Short(), nil
	}

	return head.Hash().String()[:8], nil // Return short hash if detached HEAD
}

// GetCurrentCommit returns the current commit hash
func (r *Repository) GetCurrentCommit() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get HEAD")
	}

	return head.Hash().String(), nil
}

// GetCurrentCommitShort returns the short current commit hash
func (r *Repository) GetCurrentCommitShort() (string, error) {
	hash, err := r.GetCurrentCommit()
	if err != nil {
		return "", err
	}

	if len(hash) >= 8 {
		return hash[:8], nil
	}
	return hash, nil
}

// CheckoutBranch checks out a branch
func (r *Repository) CheckoutBranch(branch string) error {
	r.config.Logger.Info("Checking out branch: %s", branch)

	var checkoutErr error
	err := withSafeDirectoryRetry(r.path, r.config.Logger, func() error {
		workTree, err := r.repo.Worktree()
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to get worktree")
		}

		checkoutErr = workTree.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName(branch),
		})
		return checkoutErr
	})

	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to checkout branch %s", branch)
	}

	r.config.Logger.Info("Successfully checked out branch: %s", branch)
	return nil
}

// CreateBranch creates a new branch
func (r *Repository) CreateBranch(branchName string) error {
	r.config.Logger.Info("Creating branch: %s", branchName)

	workTree, err := r.repo.Worktree()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to get worktree")
	}

	err = workTree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	})
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to create branch %s", branchName)
	}

	r.config.Logger.Info("Successfully created and checked out branch: %s", branchName)
	return nil
}

// Pull pulls the latest changes
// If fetchAll is true, fetches all branches (git pull --all / git fetch --all)
// If branch is specified, pulls only that branch
func (r *Repository) Pull(branch string, fetchAll bool) error {
	if fetchAll {
		r.config.Logger.Info("Pulling all branches")
	} else if branch != "" {
		r.config.Logger.Info("Pulling branch: %s", branch)
	} else {
		r.config.Logger.Info("Pulling latest changes")
	}

	// Handle bare repositories differently - use fetch instead of pull
	if r.isBare {
		r.config.Logger.Info("Bare repository detected, using fetch instead of pull")
		return r.Fetch(fetchAll)
	}

	// Handle fetch-all case using git command directly (go-git doesn't support pull --all)
	if fetchAll {
		return r.pullAll()
	}

	var pullErr error
	err := withSafeDirectoryRetry(r.path, r.config.Logger, func() error {
		workTree, err := r.repo.Worktree()
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to get worktree")
		}

		pullOptions := &git.PullOptions{}

		// Add specific branch if provided
		if branch != "" {
			pullOptions.RemoteName = "origin"
			pullOptions.ReferenceName = plumbing.NewBranchReferenceName(branch)
		}

		// Add authentication if provided
		if r.config.Username != "" && r.config.Token != "" {
			pullOptions.Auth = &http.BasicAuth{
				Username: r.config.Username,
				Password: r.config.Token,
			}
		}

		// Add progress handler if provided
		if r.config.ProgressHandler != nil {
			pullOptions.Progress = r.config.ProgressHandler
			r.config.Logger.Debug("Progress reporting enabled for pull operation")
		}

		pullErr = workTree.Pull(pullOptions)
		return pullErr
	})

	if err != nil && err != git.NoErrAlreadyUpToDate {
		return errors.Wrapf(errors.ErrFail, err, "failed to pull")
	}

	if err == git.NoErrAlreadyUpToDate {
		r.config.Logger.Info("Repository is already up to date")
	} else {
		r.config.Logger.Info("Successfully pulled latest changes")
	}

	return nil
}

// pullAll pulls all branches using git command (go-git doesn't support pull --all)
func (r *Repository) pullAll() error {
	r.config.Logger.Debug("Executing git pull --all and git fetch --all")

	// Execute git pull --all
	cmd := exec.Command("git", "-C", r.path, "pull", "--all")
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.config.Logger.Warn("git pull --all failed (this may be normal if there are no tracking branches): %v", err)
		r.config.Logger.Debug("Output: %s", string(output))
	} else {
		r.config.Logger.Debug("git pull --all output: %s", string(output))
	}

	// Execute git fetch --all
	cmd = exec.Command("git", "-C", r.path, "fetch", "--all")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "git fetch --all failed (output: %s)", string(output))
	}

	r.config.Logger.Debug("git fetch --all output: %s", string(output))
	r.config.Logger.Info("Successfully fetched all branches")
	return nil
}

// fetchAll fetches all branches using git command
func (r *Repository) fetchAll() error {
	r.config.Logger.Debug("Executing git fetch --all")

	cmd := exec.Command("git", "-C", r.path, "fetch", "--all")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "git fetch --all failed (output: %s)", string(output))
	}

	r.config.Logger.Debug("git fetch --all output: %s", string(output))
	r.config.Logger.Info("Successfully fetched all branches")
	return nil
}

// Fetch fetches the latest changes from remote without merging
// If fetchAll is true, fetches all branches (git fetch --all)
// This is used for bare repositories and when you want to update refs without merging
func (r *Repository) Fetch(fetchAll bool) error {
	if fetchAll {
		r.config.Logger.Info("Fetching all branches from remote")
		return r.fetchAll()
	}

	r.config.Logger.Info("Fetching latest changes from remote")

	var fetchErr error
	err := withSafeDirectoryRetry(r.path, r.config.Logger, func() error {
		fetchOptions := &git.FetchOptions{}

		// Add authentication if provided
		if r.config.Username != "" && r.config.Token != "" {
			fetchOptions.Auth = &http.BasicAuth{
				Username: r.config.Username,
				Password: r.config.Token,
			}
		}

		// Add progress handler if provided
		if r.config.ProgressHandler != nil {
			fetchOptions.Progress = r.config.ProgressHandler
			r.config.Logger.Debug("Progress reporting enabled for fetch operation")
		}

		fetchErr = r.repo.Fetch(fetchOptions)
		return fetchErr
	})

	if err != nil && err != git.NoErrAlreadyUpToDate {
		return errors.Wrapf(errors.ErrFail, err, "failed to fetch")
	}

	if err == git.NoErrAlreadyUpToDate {
		r.config.Logger.Info("Repository is already up to date")
	} else {
		r.config.Logger.Info("Successfully fetched latest changes")
	}

	return nil
}

// Push pushes changes to remote
func (r *Repository) Push() error {
	r.config.Logger.Info("Pushing changes to remote")

	pushOptions := &git.PushOptions{}

	// Add authentication if provided
	if r.config.Username != "" && r.config.Token != "" {
		pushOptions.Auth = &http.BasicAuth{
			Username: r.config.Username,
			Password: r.config.Token,
		}
	}

	err := r.repo.Push(pushOptions)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return errors.Wrapf(errors.ErrFail, err, "failed to push")
	}

	if err == git.NoErrAlreadyUpToDate {
		r.config.Logger.Info("Remote is already up to date")
	} else {
		r.config.Logger.Info("Successfully pushed changes")
	}

	return nil
}

// Add adds files to the staging area
func (r *Repository) Add(files ...string) error {
	r.config.Logger.Info("Adding files to staging: %s", strings.Join(files, ", "))

	workTree, err := r.repo.Worktree()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to get worktree")
	}

	for _, file := range files {
		_, err = workTree.Add(file)
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to add file %s", file)
		}
	}

	r.config.Logger.Info("Successfully added files to staging")
	return nil
}

// AddAll adds all files to the staging area
func (r *Repository) AddAll() error {
	r.config.Logger.Info("Adding all files to staging")

	workTree, err := r.repo.Worktree()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to get worktree")
	}

	_, err = workTree.Add(".")
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to add all files")
	}

	r.config.Logger.Info("Successfully added all files to staging")
	return nil
}

// Commit commits staged changes
func (r *Repository) Commit(message, author, email string) (string, error) {
	r.config.Logger.Info("Committing changes: %s", message)

	workTree, err := r.repo.Worktree()
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get worktree")
	}

	commit, err := workTree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  author,
			Email: email,
		},
	})
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to commit")
	}

	commitHash := commit.String()
	r.config.Logger.Info("Successfully committed changes: %s", commitHash[:8])

	return commitHash, nil
}

// Bundle creates a git bundle from a bare repository
// Bundles are useful for offline transport of repository data
func (r *Repository) Bundle(destDir string) (string, error) {
	if !r.isBare {
		return "", errors.New(errors.ErrFail, "repository is not bare, cannot create bundle")
	}

	r.config.Logger.Info("Creating bundle from bare repository: %s", r.path)

	// Determine destination directory
	bundleDir := r.path
	if destDir != "" {
		bundleDir = destDir
	}

	// Ensure destination directory exists
	bundleDir, err := filepath.Abs(os.ExpandEnv(bundleDir))
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to expand bundle directory path")
	}

	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to create bundle directory")
	}

	// Generate bundle filename: remove .bare suffix and add .bundle
	repoName := filepath.Base(r.path)
	repoName = strings.TrimSuffix(repoName, ".bare")
	bundleFile := filepath.Join(bundleDir, repoName+".bundle")

	r.config.Logger.Info("Creating bundle: '%s' --> '%s'", r.path, bundleFile)

	// Execute git bundle create command
	// go-git doesn't support bundle creation, so we use git command directly
	cmd := exec.Command("git", "-C", r.path, "bundle", "create", bundleFile, "--all")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to create bundle (output: %s)", string(output))
	}

	r.config.Logger.Info("Successfully created bundle: %s", bundleFile)
	return bundleFile, nil
}

// GetStatus returns the repository status
func (r *Repository) GetStatus() (git.Status, error) {
	workTree, err := r.repo.Worktree()
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to get worktree")
	}

	return workTree.Status()
}

// IsClean checks if the repository has no uncommitted changes
func (r *Repository) IsClean() (bool, error) {
	status, err := r.GetStatus()
	if err != nil {
		return false, err
	}

	return status.IsClean(), nil
}

// GetRemoteURL returns the URL of the origin remote
func (r *Repository) GetRemoteURL() (string, error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get origin remote")
	}

	config := remote.Config()
	if len(config.URLs) > 0 {
		return config.URLs[0], nil
	}

	return "", errors.New(errors.ErrFail, "no URLs found for origin remote")
}

// GetPath returns the repository path
func (r *Repository) GetPath() string {
	return r.path
}

// IsBare returns whether this is a bare repository
func (r *Repository) IsBare() bool {
	return r.isBare
}

// IsMirror returns whether this is a mirror repository
func (r *Repository) IsMirror() bool {
	return r.isMirror
}

// IsRepository checks if a path contains a Git repository
func IsRepository(path string) bool {
	gitPath := filepath.Join(path, ".git")
	if info, err := os.Stat(gitPath); err == nil {
		return info.IsDir()
	}
	return false
}

// InitRepository initializes a new Git repository
func InitRepository(path string, config *RepoConfig) (*Repository, error) {
	if config == nil {
		config = &RepoConfig{
			Logger: logging.NewLogger(logging.INFO),
		}
	}

	config.Logger.Info("Initializing repository at: %s", path)

	repo, err := git.PlainInit(path, false)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "failed to initialize repository")
	}

	config.Logger.Info("Repository initialized successfully")

	return &Repository{
		repo:   repo,
		config: config,
		path:   path,
	}, nil
}

// ParseRepoName extracts the repository name from a Git URL
// Example: "tf-desiredstates" from "git@github.com:RedCloudTechnology/tf-desiredstates.git"
func ParseRepoName(url string) string {
	logging.Debug("Getting repo name from %s", url)

	// Remove .git suffix if present
	url = strings.TrimSuffix(url, ".git")

	// Get the last part of the path
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return ""
}

// ExpandPath expands environment variables and ~ in a path
func ExpandPath(path string) string {
	if path == "" {
		return path
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = home
		}
	}

	// Make absolute path
	absPath, err := filepath.Abs(path)
	if err == nil {
		return absPath
	}

	return path
}

// NamedWorkDir updates a working directory path to include the repository name
// - For regular repos: appends repo name if not already present
// - For bare repos: appends repo name with .bare suffix
// - Expands ~ and environment variables
// - Returns absolute path
func NamedWorkDir(workDir, url string, isBare, isMirror, isCloning bool) string {
	// First expand and normalize the path
	if workDir != "" {
		workDir = ExpandPath(workDir)
	}

	// Only append repo name when cloning and when both workdir and url are set
	if isCloning && workDir != "" && url != "" {
		repoName := strings.TrimSuffix(ParseRepoName(url), ".git")
		dirName := filepath.Base(workDir)

		// For bare/mirror repos, add .bare suffix if not already present
		if isBare || isMirror {
			expectedName := repoName + ".bare"
			if dirName != expectedName {
				workDir = filepath.Join(workDir, expectedName)
			}
		} else {
			// For regular repos, just append repo name if different
			if repoName != "" && dirName != repoName {
				workDir = filepath.Join(workDir, repoName)
			}
		}
	}

	// Final path expansion
	if workDir != "" {
		workDir = ExpandPath(workDir)
	}

	return workDir
}

// ParseRepoOrganisation extracts the organization/owner from a Git URL
// Example: "RedCloudTechnology" from "git@github.com:RedCloudTechnology/tf-desiredstates.git"
func ParseRepoOrganisation(url string) string {
	logging.Debug("Getting repo organisation from %s", url)

	// Handle SSH URLs: git@github.com:RedCloudTechnology/tf-desiredstates.git
	if strings.Contains(url, "@") && strings.Contains(url, ":") {
		// Split by : to get the part after the host
		parts := strings.Split(url, ":")
		if len(parts) >= 2 {
			// Get organization from path like RedCloudTechnology/tf-desiredstates.git
			pathParts := strings.Split(parts[1], "/")
			if len(pathParts) >= 2 {
				return pathParts[0]
			}
		}
	}

	// Handle HTTPS URLs: https://github.com/RedCloudTechnology/tf-desiredstates.git
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		// Remove protocol
		url = strings.TrimPrefix(url, "https://")
		url = strings.TrimPrefix(url, "http://")

		// Split by /
		parts := strings.Split(url, "/")
		// Format: github.com/RedCloudTechnology/tf-desiredstates.git
		if len(parts) >= 3 {
			return parts[1]
		}
	}

	return ""
}
