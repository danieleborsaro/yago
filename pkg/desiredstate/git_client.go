// Package desiredstate provides Git operations for version resolution.
package desiredstate

import (
	"fmt"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	uRepo "github.com/danieleborsaro/yago/internal/utils/repo"
)

// GitClient handles Git operations for component version resolution
type GitClient struct{}

// NewGitClient creates a new Git client instance
func NewGitClient() *GitClient {
	return &GitClient{}
}

// ResolveRefToCommit resolves a Git tag or branch to a commit SHA
// Returns the commit SHA and any error encountered
func (g *GitClient) ResolveRefToCommit(repoURL, ref string) (string, error) {
	// Try to resolve using ls-remote (doesn't require cloning)
	if err := uRepo.RejectOptionLike("ref", ref); err != nil {
		return "", err
	}
	output, err := uRepo.LsRemote(nil, repoURL, ref)
	if err != nil {
		// Try with refs/heads/ prefix for branches
		output, err = uRepo.LsRemote(nil, repoURL, "refs/heads/"+ref)
		if err != nil {
			// Try with refs/tags/ prefix for tags
			output, err = uRepo.LsRemote(nil, repoURL, "refs/tags/"+ref)
			if err != nil {
				return "", fmt.Errorf("failed to resolve ref %s in repo %s: %w", ref, repoURL, err)
			}
		}
	}

	// Parse output: "commit_sha\trefs/..."
	parts := strings.Fields(string(output))
	if len(parts) == 0 {
		return "", fmt.Errorf("ref %s not found in repo %s", ref, repoURL)
	}

	commitSHA := parts[0]
	logging.Debug("Resolved %s@%s to commit %s", repoURL, ref, commitSHA)
	return commitSHA, nil
}

// ResolveTagToCommit specifically resolves a Git tag to a commit SHA
func (g *GitClient) ResolveTagToCommit(repoURL, tag string) (string, error) {
	return g.ResolveRefToCommit(repoURL, "refs/tags/"+tag)
}

// ResolveBranchToCommit specifically resolves a Git branch to a commit SHA
func (g *GitClient) ResolveBranchToCommit(repoURL, branch string) (string, error) {
	return g.ResolveRefToCommit(repoURL, "refs/heads/"+branch)
}

// IsCommitSHA checks if a string looks like a commit SHA
func (g *GitClient) IsCommitSHA(ref string) bool {
	// SHA-1 hashes are 40 hex characters, SHA-256 are 64
	if len(ref) != 40 && len(ref) != 64 {
		return false
	}
	for _, c := range ref {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
