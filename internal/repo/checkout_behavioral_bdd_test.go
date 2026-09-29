package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	uRepo "github.com/danieleborsaro/yago/internal/utils/repo"
)

// keeps the developer's own git config out, commit signing breaks these
func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=yago", "GIT_AUTHOR_EMAIL=yago@example.com",
		"GIT_COMMITTER_NAME=yago", "GIT_COMMITTER_EMAIL=yago@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestRepoCheckout_TagRef_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Check out a repo pinned to a tag after cloning it",
		CurrentImpl:     "Repo.Checkout calls Repository.CheckoutRef, which resolves branches, tags and commits",
		ExpectedOutcome: "The work tree and Repo.Commit are at the tagged commit",
		Rationale:       "Desired states pin repos to release tags, checkout used to fail with reference not found",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: an origin with an annotated release tag and a later commit on main
	origin := t.TempDir()
	gitCmd(t, origin, "init", "-q", "-b", "main")
	gitCmd(t, origin, "commit", "-q", "--allow-empty", "-m", "release")
	tagged := gitCmd(t, origin, "rev-parse", "HEAD")
	gitCmd(t, origin, "tag", "-a", "v1.0.0", "-m", "release")
	gitCmd(t, origin, "commit", "-q", "--allow-empty", "-m", "after release")

	// When: the repo is cloned and checked out at the tag
	cfg := &uRepo.RepoConfig{Logger: logging.NewLogger(logging.ERROR)}
	r := NewRepo("file://"+origin, "v1.0.0", filepath.Join(t.TempDir(), "work"), cfg)
	if err := r.Clone(); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	err := r.Checkout()

	// Then: HEAD is the tagged commit
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if r.Commit != tagged {
		t.Errorf("Commit = %s, want the tagged commit %s", r.Commit, tagged)
	}
	if got := gitCmd(t, r.WorkDir, "rev-parse", "HEAD"); got != tagged {
		t.Errorf("work tree HEAD = %s, want %s", got, tagged)
	}
}
