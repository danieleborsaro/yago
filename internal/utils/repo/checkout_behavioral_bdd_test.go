package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
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

type checkoutFixture struct {
	clone    string
	mainHash string
	featHash string
}

// feature only exists as origin/feature in the clone
func newCheckoutFixture(t *testing.T) checkoutFixture {
	t.Helper()
	origin := t.TempDir()
	gitCmd(t, origin, "init", "-q", "-b", "main")
	gitCmd(t, origin, "commit", "-q", "--allow-empty", "-m", "main")
	mainHash := gitCmd(t, origin, "rev-parse", "HEAD")
	gitCmd(t, origin, "tag", "lightweight")
	gitCmd(t, origin, "switch", "-q", "-c", "feature")
	gitCmd(t, origin, "commit", "-q", "--allow-empty", "-m", "feature")
	featHash := gitCmd(t, origin, "rev-parse", "HEAD")
	gitCmd(t, origin, "tag", "-a", "v2.0.0", "-m", "annotated")
	gitCmd(t, origin, "switch", "-q", "main")

	clone := filepath.Join(t.TempDir(), "clone")
	gitCmd(t, filepath.Dir(clone), "clone", "-q", origin, clone)
	return checkoutFixture{clone: clone, mainHash: mainHash, featHash: featHash}
}

func TestRepositoryCheckoutRef_AnyRefKind_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Check out a local branch, an origin only branch, a tag or a commit",
		CurrentImpl:     "Repository.CheckoutRef tries the local branch, then origin/<ref>, then ResolveRevision",
		ExpectedOutcome: "HEAD moves to the ref, branches stay attached and tags or commits leave HEAD detached",
		Rationale:       "Desired states pin refs to tags and commits, checkout used to only understand local branches",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	fx := newCheckoutFixture(t)

	tests := []struct {
		name       string
		ref        string
		wantHash   string
		wantBranch string
	}{
		{name: "local branch", ref: "main", wantHash: fx.mainHash, wantBranch: "main"},
		{name: "branch only on origin", ref: "feature", wantHash: fx.featHash, wantBranch: "feature"},
		{name: "lightweight tag", ref: "lightweight", wantHash: fx.mainHash},
		{name: "annotated tag", ref: "v2.0.0", wantHash: fx.featHash},
		{name: "full commit hash", ref: fx.featHash, wantHash: fx.featHash},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a clone sitting on main
			gitCmd(t, fx.clone, "checkout", "-q", "main")
			r, err := NewRepository(fx.clone, &RepoConfig{Logger: logging.NewLogger(logging.ERROR)})
			if err != nil {
				t.Fatalf("NewRepository: %v", err)
			}

			// When: the ref is checked out
			err = r.CheckoutRef(tt.ref)

			// Then: HEAD is at the ref, on a branch only when the ref is one
			if err != nil {
				t.Fatalf("CheckoutRef(%q): %v", tt.ref, err)
			}
			if got := gitCmd(t, fx.clone, "rev-parse", "HEAD"); got != tt.wantHash {
				t.Errorf("HEAD = %s, want %s", got, tt.wantHash)
			}
			if got := gitCmd(t, fx.clone, "branch", "--show-current"); got != tt.wantBranch {
				t.Errorf("current branch = %q, want %q", got, tt.wantBranch)
			}
		})
	}
}

func TestRepositoryCheckoutRef_UnknownRef_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Refuse to check out a ref that doesn't exist",
		CurrentImpl:     "Repository.CheckoutRef returns an error when no branch, tag or commit matches",
		ExpectedOutcome: "An error saying the ref isn't a branch, tag or commit, with HEAD left where it was",
		Rationale:       "A typo in a pinned ref must stop the run, not deploy whatever is checked out",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a clone sitting on main
	fx := newCheckoutFixture(t)
	r, err := NewRepository(fx.clone, &RepoConfig{Logger: logging.NewLogger(logging.ERROR)})
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}

	// When: a ref that doesn't exist is checked out
	err = r.CheckoutRef("does-not-exist")

	// Then: it fails and HEAD doesn't move
	if err == nil || !strings.Contains(err.Error(), "not a branch, tag or commit") {
		t.Errorf("error = %v, want it to say the ref is not a branch, tag or commit", err)
	}
	if got := gitCmd(t, fx.clone, "rev-parse", "HEAD"); got != fx.mainHash {
		t.Errorf("HEAD moved to %s after a failed checkout, want %s", got, fx.mainHash)
	}
}

func TestSwitchWorktree_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Check out a branch in the work tree holding a file, the way promote reads each side",
		CurrentImpl:     "SwitchWorktree runs git rev-parse, symbolic-ref, status and checkout in that work tree",
		ExpectedOutcome: "It switches when it has to, leaves an already checked out branch alone and refuses uncommitted changes",
		Rationale:       "Promote reads the source and destination on their own branches without losing or stashing local work",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// SwitchWorktree runs git with the process environment, so the developer's config is kept out here too
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	tests := []struct {
		name       string
		ref        string
		prepare    func(t *testing.T, repo string)
		wantErr    string
		wantBranch string
	}{
		{name: "switches to another branch", ref: "feature", wantBranch: "feature"},
		{
			name: "leaves the checked out branch alone, local edits too",
			ref:  "main",
			prepare: func(t *testing.T, repo string) {
				writeFile(t, filepath.Join(repo, "desiredstate.yaml"), "edited\n")
			},
			wantBranch: "main",
		},
		{
			name: "refuses uncommitted changes",
			ref:  "feature",
			prepare: func(t *testing.T, repo string) {
				writeFile(t, filepath.Join(repo, "desiredstate.yaml"), "edited\n")
			},
			wantErr:    "uncommitted changes",
			wantBranch: "main",
		},
		{
			name: "untracked files don't count",
			ref:  "feature",
			prepare: func(t *testing.T, repo string) {
				writeFile(t, filepath.Join(repo, "notes.txt"), "scratch\n")
			},
			wantBranch: "feature",
		},
		{name: "unknown ref", ref: "does-not-exist", wantErr: "failed to check out", wantBranch: "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a repo on main with a feature branch, and maybe some local changes
			repo := t.TempDir()
			gitCmd(t, repo, "init", "-q", "-b", "main")
			writeFile(t, filepath.Join(repo, "desiredstate.yaml"), "main\n")
			gitCmd(t, repo, "add", "desiredstate.yaml")
			gitCmd(t, repo, "commit", "-q", "-m", "main")
			gitCmd(t, repo, "branch", "feature")
			if tt.prepare != nil {
				tt.prepare(t, repo)
			}

			// When: the file's work tree is switched to ref
			err := SwitchWorktree(filepath.Join(repo, "desiredstate.yaml"), tt.ref)

			// Then: it switched or refused as expected
			if tt.wantErr == "" && err != nil {
				t.Fatalf("SwitchWorktree: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
			if got := gitCmd(t, repo, "branch", "--show-current"); got != tt.wantBranch {
				t.Errorf("checked out %q, want %q", got, tt.wantBranch)
			}
			if stashes := gitCmd(t, repo, "stash", "list"); stashes != "" {
				t.Errorf("changes were stashed: %s", stashes)
			}
		})
	}

	t.Run("outside a git repository", func(t *testing.T) {
		// Given: a file that isn't in any repository
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "desiredstate.yaml"), "plain\n")

		// When: its work tree is switched
		err := SwitchWorktree(filepath.Join(dir, "desiredstate.yaml"), "main")

		// Then: it warns and carries on with the file as it is, like the python tool
		if err != nil {
			t.Errorf("SwitchWorktree outside a repository: %v", err)
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
