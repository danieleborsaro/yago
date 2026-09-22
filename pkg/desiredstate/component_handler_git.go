package desiredstate

import (
	"regexp"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// GitHandler implements ComponentTypeHandler for Git repositories.
// It handles Git references (branches, tags, commits), repository cloning, and version resolution.
//
// Git components have the following structure in YAML:
//
//	sourcecode:
//	  my-repo:
//	    git:
//	      url: https://github.com/user/repo.git
//	      branch: develop
//	      tag: v1.2.3
//	      path: /config
//
// The handler supports:
//   - Branch references
//   - Tag references
//   - Commit SHA references
//   - Path within repository
type GitHandler struct {
	BaseComponentHandler
	gitClient *GitClient    // Optional GitClient for resolving refs to commits
	cache     *VersionCache // Optional cache for resolved versions
}

// NewGitHandler creates a new Git handler instance.
func NewGitHandler() *GitHandler {
	return &GitHandler{}
}

// Type returns the component type this handler manages.
func (h *GitHandler) Type() ComponentType {
	return ComponentTypeSourcecode
}

// SetGitClient injects a GitClient for resolving Git references.
// This allows the handler to resolve branch/tag names to commit SHAs.
func (h *GitHandler) SetGitClient(client *GitClient) {
	h.gitClient = client
	logging.Debug("Set GitClient on Git handler")
}

// SetCache injects a VersionCache for caching resolved versions.
func (h *GitHandler) SetCache(cache *VersionCache) {
	h.cache = cache
	logging.Debug("Set VersionCache on Git handler")
}

// ParseComponent parses a Git component from YAML data.
// Extracts URL, branch, tag, path fields.
func (h *GitHandler) ParseComponent(data map[string]interface{}, partID, partFile string) (*VersionedComponent, error) {
	comp := &VersionedComponent{
		PartFile:         partFile,
		PartID:           partID,
		Type:             ComponentTypeSourcecode,
		VersionSeparator: "@",
		LatestIdentifier: "latest",
		IsVersioned:      true,
		Metadata:         make(map[string]interface{}),
	}

	// Extract URL (required)
	url, err := ParseYAMLString(data, "url", true)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "Git component must have 'url' field")
	}
	comp.URL = url

	// Extract branch (optional)
	branch, _ := ParseYAMLString(data, "branch", false)
	if branch != "" {
		comp.Branch = branch
		comp.Metadata["branch"] = branch
	}

	// Extract tag (optional)
	tag, _ := ParseYAMLString(data, "tag", false)
	if tag != "" {
		comp.Tag = tag
		comp.Metadata["tag"] = tag
	}

	// Extract path (optional)
	path, _ := ParseYAMLString(data, "path", false)
	if path != "" {
		comp.Path = path
		comp.Metadata["path"] = path
	}

	// Determine version and lock state
	// Priority: tag > branch > default to "main"
	if tag != "" {
		comp.Version = tag
		// Check if tag looks like a commit SHA
		comp.IsLocked = h.isCommitSHA(tag)
	} else if branch != "" {
		comp.Version = branch
		comp.IsLocked = false
	} else {
		comp.Version = "main"
		comp.Branch = "main"
		comp.IsLocked = false
	}

	logging.Debug("Parsed Git component: %s (url: %s, branch: %s, tag: %s, version: %s, locked: %t)",
		partID, url, branch, tag, comp.Version, comp.IsLocked)

	return comp, nil
}

// ResolveVersion resolves "latest" or branch reference to a specific commit SHA.
// For Git, this queries the repository to get the HEAD commit of a branch.
func (h *GitHandler) ResolveVersion(comp *VersionedComponent, ctx *ResolveContext) (string, error) {
	// If already a commit SHA, return it
	if h.isCommitSHA(comp.Version) {
		logging.Debug("Git component already has commit SHA: %s", comp.Version)
		return comp.Version, nil
	}

	// Determine the ref to resolve
	ref := comp.Version
	if ref == "" || ref == "latest" {
		if comp.Tag != "" {
			ref = comp.Tag
		} else if comp.Branch != "" {
			ref = comp.Branch
		} else {
			ref = "main"
		}
	}

	// Check cache first
	cacheKey := MakeCacheKey(ComponentTypeSourcecode, comp.URL, ref)
	if cached, found := h.cache.Get(cacheKey); found {
		logging.Debug("Cache hit for Git ref: %s@%s -> %s", comp.URL, ref, cached[:8])
		return cached, nil
	}

	// If no GitClient is available, return the ref as-is (fallback)
	if h.gitClient == nil {
		logging.Warn("GitClient not available - returning ref without resolution: %s", ref)
		return ref, nil
	}

	// Use GitClient to resolve the ref to a commit SHA
	var commitSHA string
	var err error

	// Try tag resolution first if we have a tag
	if comp.Tag != "" && comp.Tag == ref {
		commitSHA, err = h.gitClient.ResolveTagToCommit(comp.URL, ref)
		if err != nil {
			logging.Warn("Failed to resolve tag %s for %s: %v", ref, comp.URL, err)
		} else {
			logging.Info("Resolved Git tag %s to commit %s for %s", ref, commitSHA[:8], comp.URL)
			// Store in cache
			h.cache.Set(cacheKey, commitSHA, ComponentTypeSourcecode)
			return commitSHA, nil
		}
	}

	// Try branch resolution if we have a branch or tag resolution failed
	if comp.Branch != "" && comp.Branch == ref {
		commitSHA, err = h.gitClient.ResolveBranchToCommit(comp.URL, ref)
		if err != nil {
			logging.Warn("Failed to resolve branch %s for %s: %v", ref, comp.URL, err)
		} else {
			logging.Info("Resolved Git branch %s to commit %s for %s", ref, commitSHA[:8], comp.URL)
			// Store in cache
			h.cache.Set(cacheKey, commitSHA, ComponentTypeSourcecode)
			return commitSHA, nil
		}
	}

	// Generic ref resolution as fallback
	commitSHA, err = h.gitClient.ResolveRefToCommit(comp.URL, ref)
	if err != nil {
		logging.Error("Failed to resolve Git ref %s for %s: %v", ref, comp.URL, err)
		return "", errors.Wrapf(errors.ErrFail, err, "failed to resolve Git ref %s", ref)
	}

	logging.Info("Resolved Git ref %s to commit %s for %s", ref, commitSHA[:8], comp.URL)
	// Store in cache
	h.cache.Set(cacheKey, commitSHA, ComponentTypeSourcecode)
	return commitSHA, nil
}

// CompareVersions compares two Git version strings.
// If versions are commit SHAs, compare lexicographically.
// If versions look like semantic versions (tags), use semantic comparison.
func (h *GitHandler) CompareVersions(v1, v2 string) (int, error) {
	// If both are commit SHAs, they're only equal if identical
	if h.isCommitSHA(v1) && h.isCommitSHA(v2) {
		if v1 == v2 {
			return 0, nil
		}
		// Different commits - compare lexicographically
		if v1 < v2 {
			return -1, nil
		}
		return 1, nil
	}

	// Try semantic version comparison (for tags like v1.2.3)
	result, err := CompareSemanticVersions(v1, v2)
	if err == nil {
		return result, nil
	}

	// Fall back to timestamp comparison
	result, err = CompareTimestampVersions(v1, v2)
	if err == nil {
		return result, nil
	}

	// Fall back to lexicographic comparison
	if v1 < v2 {
		return -1, nil
	} else if v1 > v2 {
		return 1, nil
	}
	return 0, nil
}

// LockVersion locks a Git component to a specific commit SHA.
// This converts branch/tag references to commit SHAs.
func (h *GitHandler) LockVersion(comp *VersionedComponent) error {
	// If already a commit SHA, just mark as locked
	if h.isCommitSHA(comp.Version) {
		comp.IsLocked = true
		logging.Debug("Git component already locked with commit SHA: %s", comp.Version)
		return nil
	}

	// If no GitClient is available, mark as locked but keep the ref (fallback)
	if h.gitClient == nil {
		logging.Warn("GitClient not available - locking with ref instead of commit SHA: %s", comp.Version)
		comp.OldVersion = comp.Version
		comp.IsLocked = true
		return nil
	}

	// Determine the ref to resolve
	ref := comp.Version
	if ref == "" {
		if comp.Tag != "" {
			ref = comp.Tag
		} else if comp.Branch != "" {
			ref = comp.Branch
		} else {
			ref = "main"
		}
	}

	// Use GitClient to resolve the ref to a commit SHA
	var commitSHA string
	var err error

	// Try tag resolution first if we have a tag
	if comp.Tag != "" && comp.Tag == ref {
		commitSHA, err = h.gitClient.ResolveTagToCommit(comp.URL, ref)
		if err == nil {
			logging.Info("Locked Git component to tag %s (commit %s)", ref, commitSHA[:8])
			comp.OldVersion = comp.Version
			comp.Version = commitSHA
			comp.IsLocked = true
			return nil
		}
		logging.Warn("Failed to resolve tag %s: %v", ref, err)
	}

	// Try branch resolution if we have a branch or tag resolution failed
	if comp.Branch != "" && comp.Branch == ref {
		commitSHA, err = h.gitClient.ResolveBranchToCommit(comp.URL, ref)
		if err == nil {
			logging.Info("Locked Git component to branch %s (commit %s)", ref, commitSHA[:8])
			comp.OldVersion = comp.Version
			comp.Version = commitSHA
			comp.IsLocked = true
			return nil
		}
		logging.Warn("Failed to resolve branch %s: %v", ref, err)
	}

	// Generic ref resolution as fallback
	commitSHA, err = h.gitClient.ResolveRefToCommit(comp.URL, ref)
	if err != nil {
		logging.Error("Failed to resolve Git ref %s for locking: %v", ref, err)
		return errors.Wrapf(errors.ErrFail, err, "failed to lock Git component to ref %s", ref)
	}

	logging.Info("Locked Git component to ref %s (commit %s)", ref, commitSHA[:8])
	comp.OldVersion = comp.Version
	comp.Version = commitSHA
	comp.IsLocked = true
	return nil
}

// UnlockVersion unlocks a Git component to track a branch.
func (h *GitHandler) UnlockVersion(comp *VersionedComponent) error {
	comp.OldVersion = comp.Version

	// Reset to branch if available, otherwise default to main
	if comp.Branch != "" {
		comp.Version = comp.Branch
	} else {
		comp.Version = "main"
		comp.Branch = "main"
	}

	comp.IsLocked = false
	comp.Tag = "" // Clear tag when unlocking

	logging.Debug("Unlocked Git component: %s -> %s", comp.OldVersion, comp.Version)
	return nil
}

// ValidateComponent performs Git-specific validation.
func (h *GitHandler) ValidateComponent(comp *VersionedComponent) error {
	if comp.URL == "" {
		return errors.New(errors.ErrParam, "Git component must have a URL")
	}

	// Validate URL format (basic check)
	if !h.isValidGitURL(comp.URL) {
		return errors.New(errors.ErrParam, "Git component has invalid URL format")
	}

	// Validate commit SHA format if present
	if comp.Version != "" && h.isCommitSHA(comp.Version) {
		if !h.isValidCommitSHA(comp.Version) {
			return errors.New(errors.ErrParam, "Git component has invalid commit SHA format")
		}
	}

	return nil
}

// isCommitSHA checks if a string looks like a Git commit SHA.
// A commit SHA is a 40-character hexadecimal string (or 7+ chars for short form).
func (h *GitHandler) isCommitSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}

	// Check if all characters are hexadecimal
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}

	return true
}

// isValidCommitSHA validates a commit SHA format more strictly.
func (h *GitHandler) isValidCommitSHA(sha string) bool {
	// Full SHA: exactly 40 hex chars
	// Short SHA: 7-40 hex chars
	if len(sha) < 7 || len(sha) > 40 {
		return false
	}

	matched, _ := regexp.MatchString("^[0-9a-fA-F]{7,40}$", sha)
	return matched
}

// isValidGitURL checks if a string looks like a valid Git URL.
func (h *GitHandler) isValidGitURL(url string) bool {
	// Simple validation - check for common Git URL patterns
	if strings.HasPrefix(url, "https://") ||
		strings.HasPrefix(url, "http://") ||
		strings.HasPrefix(url, "git@") ||
		strings.HasPrefix(url, "ssh://") ||
		strings.HasPrefix(url, "git://") {
		return true
	}

	// Check for GitHub shorthand (user/repo)
	if strings.Contains(url, "/") && !strings.Contains(url, " ") {
		return true
	}

	return false
}

// init registers the Git handler with the global component registry.
// This is called automatically when the package is imported.
func init() {
	RegisterComponentType(NewGitHandler())
	logging.Debug("Registered Git component handler")
}
