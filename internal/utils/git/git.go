package git

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// GetCommitMessage retrieves the commit message for a specific commit SHA or ref.
// This executes git commands to extract the commit message.
func GetCommitMessage(repoPath, ref string) (string, error) {
	if repoPath == "" {
		return "", errors.NewParamError("repository path must be specified")
	}
	if ref == "" {
		return "", errors.NewParamError("ref must be specified")
	}

	logging.Debug("Retrieving commit message for ref: %s in repo: %s", ref, repoPath)

	// Execute: git log --format=%B -n 1 <ref>
	// %B = raw body (commit message)
	// -n 1 = only the most recent commit
	cmd := exec.Command("git", "-C", repoPath, "log", "--format=%B", "-n", "1", ref)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get commit message: %s", string(output))
	}

	message := strings.TrimSpace(string(output))
	logging.Debug("Commit message: %s", message)

	return message, nil
}

// GetCommitMessageFromRemote retrieves the commit message for a specific ref from a remote repository.
// This is more complex as it requires either:
// 1. Cloning the repo temporarily
// 2. Using git ls-remote + git show (requires shallow clone)
//
// For now, this is a placeholder that would need implementation based on use case.
func GetCommitMessageFromRemote(remoteURL, ref string) (string, error) {
	logging.Warn("GetCommitMessageFromRemote not fully implemented - requires temporary clone")

	// TODO: Implement remote commit message retrieval
	// Options:
	// 1. Create temporary directory
	// 2. git clone --depth 1 --single-branch --branch <ref> <url> <tmpdir>
	// 3. git log --format=%B -n 1
	// 4. Cleanup tmpdir

	return "", errors.New(errors.ErrFail, "remote commit message retrieval not yet implemented")
}

// GetLatestCommitSHA gets the latest commit SHA for a branch or tag.
func GetLatestCommitSHA(repoPath, ref string) (string, error) {
	if repoPath == "" {
		return "", errors.NewParamError("repository path must be specified")
	}
	if ref == "" {
		ref = "HEAD"
	}

	logging.Debug("Getting commit SHA for ref: %s in repo: %s", ref, repoPath)

	// Execute: git rev-parse <ref>
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", ref)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get commit SHA: %s", string(output))
	}

	sha := strings.TrimSpace(string(output))
	logging.Debug("Commit SHA: %s", sha)

	return sha, nil
}

// ParseCommitMessageForJiraIssue is a helper that combines getting commit message
// and parsing for Jira issue. This is commonly used together.
func ParseCommitMessageForJiraIssue(repoPath, ref string, parseFunc func(string) string) (string, error) {
	message, err := GetCommitMessage(repoPath, ref)
	if err != nil {
		return "", err
	}

	if parseFunc == nil {
		return "", errors.NewParamError("parse function must be provided")
	}

	issue := parseFunc(message)
	return issue, nil
}

// ResolveBranchToCommit resolves a branch name to its current commit SHA.
func ResolveBranchToCommit(repoPath, branch string) (string, error) {
	if branch == "" {
		return "", errors.NewParamError("branch must be specified")
	}

	// Ensure we're using the full ref path for branches
	ref := branch
	if !strings.HasPrefix(branch, "refs/") {
		ref = fmt.Sprintf("refs/heads/%s", branch)
	}

	return GetLatestCommitSHA(repoPath, ref)
}

// ResolveTagToCommit resolves a tag name to its commit SHA.
func ResolveTagToCommit(repoPath, tag string) (string, error) {
	if tag == "" {
		return "", errors.NewParamError("tag must be specified")
	}

	// Ensure we're using the full ref path for tags
	ref := tag
	if !strings.HasPrefix(tag, "refs/") {
		ref = fmt.Sprintf("refs/tags/%s", tag)
	}

	return GetLatestCommitSHA(repoPath, ref)
}
