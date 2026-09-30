package repo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// a repo with one commit on main and feature and an empty sub folder, named by the caller so its path can hold
// awkward characters
func newNamedRepo(t *testing.T, name string) (root, mainHash string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitCmd(t, root, "init", "-q", "-b", "main")
	gitCmd(t, root, "commit", "-q", "--allow-empty", "-m", "main")
	gitCmd(t, root, "branch", "feature")
	return root, gitCmd(t, root, "rev-parse", "HEAD")
}

// git then treats every repo as owned by someone else, so set it up after any fixture is built
func assumeDifferentOwner(t *testing.T) (globalConfig string) {
	t.Helper()
	globalConfig = filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(globalConfig, nil, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	return globalConfig
}

func assertEmptyFile(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // path comes from t.TempDir
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(data) != 0 {
		t.Errorf("global git config was written:\n%s", data)
	}
}

func TestSafeDirectory_LeavesGlobalConfigAlone_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A repo git refuses for its owner is switched and read from its root or a subdirectory without touching the user's git config",
		CurrentImpl:     "withSafeDirectoryRetry retries once with -c safe.directory set to the repo git named",
		ExpectedOutcome: "The work tree moves to the ref, whatever the repo is called, and the global git config file stays empty",
		Rationale:       "yago used to run git config --global --add safe.directory on every such run, and a quote in the repo's path must not cut the trusted path short",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name string
		repo string
		dir  string
	}{
		{name: "root", repo: "repo"},
		{name: "subdirectory", repo: "repo", dir: "sub"},
		{name: "root with a quote in its name", repo: "team's-repo"},
		{name: "subdirectory with a quote in its name", repo: "team's-repo", dir: "sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a repo git treats as owned by someone else, and an empty global config
			root, mainHash := newNamedRepo(t, tt.repo)
			globalConfig := assumeDifferentOwner(t)

			// When: feature is checked out from the root or the subdirectory, the way a promotion does
			path := filepath.Join(root, tt.dir)
			err := SwitchWorktree(path, "feature")

			// Then: the work tree is on feature, and the global config is untouched
			if err != nil {
				t.Fatalf("SwitchWorktree: %v", err)
			}
			head, err := WorktreeHead(path)
			if err != nil {
				t.Fatalf("WorktreeHead: %v", err)
			}
			if head.Branch != "feature" || head.Commit != mainHash {
				t.Errorf("head = %s, want feature at %s", head, mainHash)
			}
			assertEmptyFile(t, globalConfig)
		})
	}
}

func TestSafeDirectory_RepoAboveWithAnotherOwner_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A repo above the folder yago was pointed at is only trusted when its .git has the folder's owner",
		CurrentImpl:     "trustedRepository compares the owners before withSafeDirectoryRetry sets safe.directory",
		ExpectedOutcome: "Pointing at the repo itself still works, a folder inside a repo with another owner gets git's refusal back",
		Rationale:       "Trusting whatever repo git found further up, like someone else's /tmp/.git, would run its hooks and fsmonitor as the user, which safe.directory is there to stop",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a repo git treats as owned by someone else, with its .git owned by another user than its folders
	root, mainHash := newNamedRepo(t, "repo")
	globalConfig := assumeDifferentOwner(t)
	old := fileOwner
	fileOwner = func(path string) (uint32, bool) {
		if filepath.Base(path) == ".git" {
			return 2, true
		}
		return 1, true
	}
	t.Cleanup(func() { fileOwner = old })

	t.Run("subdirectory", func(t *testing.T) {
		// When: the head is read from a folder inside the repo
		_, err := WorktreeHead(filepath.Join(root, "sub"))

		// Then: git's refusal comes back and the global config is untouched
		if err == nil || !strings.Contains(err.Error(), "dubious ownership") {
			t.Fatalf("WorktreeHead error = %v, want git's ownership refusal", err)
		}
		assertEmptyFile(t, globalConfig)
	})

	t.Run("root", func(t *testing.T) {
		// When: the head is read from the repo itself
		head, err := WorktreeHead(root)

		// Then: pointing at it is enough to trust it
		if err != nil {
			t.Fatalf("WorktreeHead: %v", err)
		}
		if head.Branch != "main" || head.Commit != mainHash {
			t.Errorf("head = %s, want main at %s", head, mainHash)
		}
		assertEmptyFile(t, globalConfig)
	})
}

func TestSafeDirectory_RepoPathFromGitsMessage_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "The repo to trust is read from the first line of git's refusal",
		CurrentImpl:     "unsafeRepositoryPath takes everything between the opening quote and the quote that ends the line",
		ExpectedOutcome: "Quotes inside the path are kept, and a message it can't read falls back to the folder yago was pointed at",
		Rationale:       "git doesn't escape the path on that line, so stopping at the first quote trusted the wrong folder",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "quote in the path",
			msg:  "git rev-parse --show-toplevel: exit status 128: fatal: detected dubious ownership in repository at '/work/team's-repo'\nTo add an exception for this directory, call:",
			want: "/work/team's-repo",
		},
		{
			name: "only line",
			msg:  "fatal: detected dubious ownership in repository at '/work/repo'",
			want: "/work/repo",
		},
		{
			name: "windows line ending",
			msg:  "fatal: detected dubious ownership in repository at '/work/repo'\r\nTo add an exception for this directory, call:",
			want: "/work/repo",
		},
		{
			name: "older git wording",
			msg:  "fatal: unsafe repository ('/work/repo' is owned by someone else)",
			want: "/pointed/at",
		},
		{
			name: "no closing quote",
			msg:  "fatal: detected dubious ownership in repository at '/work/repo",
			want: "/pointed/at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: git's refusal
			err := errors.New(tt.msg)

			// When: the repo path is read from it
			got := unsafeRepositoryPath(err, "/pointed/at")

			// Then: it's the whole path git named, or the folder yago was pointed at
			if got != tt.want {
				t.Errorf("unsafeRepositoryPath = %q, want %q", got, tt.want)
			}
		})
	}
}
