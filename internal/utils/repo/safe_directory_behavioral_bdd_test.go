package repo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// named by the caller so its path can hold awkward characters, like a quote
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
func assumeDifferentOwner(t *testing.T, root string) (globalConfig string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("yago can't compare owners on windows, so it never trusts a repo git refused there")
	}
	globalConfig = filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(globalConfig, nil, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	// GitHub's runners trust every repo in their system config, and config passed in the environment counts too
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")

	// a git that never refuses would let these tests pass without the retry ever running
	out, err := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").CombinedOutput()
	if err == nil || !isUnsafeRepositoryError(errors.New(string(out))) {
		t.Fatalf("git didn't refuse %s for its owner, so nothing here would be tested: %v %s", root, err, out)
	}
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
			// Given: a repo git refuses for its owner, and an empty global config
			root, mainHash := newNamedRepo(t, tt.repo)
			globalConfig := assumeDifferentOwner(t, root)

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

func TestSafeDirectory_OwnersDecideTrust_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A repo git refuses is only trusted when it and its .git have the owner of the folder yago works in",
		CurrentImpl:     "trustedRepository compares the owner of the folder holding what yago was pointed at with the repo top and its .git before withSafeDirectoryRetry sets safe.directory",
		ExpectedOutcome: "One owner throughout is trusted whoever owns the file itself, any other owner on the repo or its .git gets git's refusal back",
		Rationale:       "Trusting whatever repo git found, like someone else's /tmp/.git above a file sitting in /tmp, would run its hooks and fsmonitor as the user, and yago running as root in a container rewrites the files it checks out",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name    string
		point   string
		owners  map[string]uint32
		trusted bool
	}{
		{name: "the repo itself with one owner", owners: map[string]uint32{"repo": 1, ".git": 1}, trusted: true},
		{name: "a folder inside with one owner", point: "sub", owners: map[string]uint32{"repo": 1, "sub": 1, ".git": 1}, trusted: true},
		{name: "a file inside with one owner", point: "ds.yaml", owners: map[string]uint32{"repo": 1, "ds.yaml": 1, ".git": 1}, trusted: true},
		{name: "a file root rewrote in a container", point: "ds.yaml", owners: map[string]uint32{"repo": 1, "ds.yaml": 0, ".git": 1}, trusted: true},
		{name: "a file that isn't there yet", point: "missing.yaml", owners: map[string]uint32{"repo": 1, ".git": 1}, trusted: true},
		{name: "the repo itself when its .git has another owner", owners: map[string]uint32{"repo": 1, ".git": 2}},
		{name: "a folder inside when the .git has another owner", point: "sub", owners: map[string]uint32{"repo": 1, "sub": 1, ".git": 2}},
		{name: "a folder inside when the repo top has another owner", point: "sub", owners: map[string]uint32{"repo": 2, "sub": 1, ".git": 1}},
		{name: "a file sitting in a folder someone else owns, like /tmp", point: "ds.yaml", owners: map[string]uint32{"repo": 0, "ds.yaml": 1, ".git": 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a repo git refuses for its owner, with the owners faked by name
			root, mainHash := newNamedRepo(t, "repo")
			if err := os.WriteFile(filepath.Join(root, "ds.yaml"), nil, 0o600); err != nil {
				t.Fatalf("write ds.yaml: %v", err)
			}
			globalConfig := assumeDifferentOwner(t, root)
			old := fileOwner
			fileOwner = func(path string) (uint32, bool) {
				owner, ok := tt.owners[filepath.Base(path)]
				return owner, ok
			}
			t.Cleanup(func() { fileOwner = old })

			// When: the head is read through what yago was pointed at
			head, err := WorktreeHead(filepath.Join(root, tt.point))

			// Then: the repo is read when trusted, otherwise git's refusal comes back, and the global config is untouched
			if tt.trusted {
				if err != nil {
					t.Fatalf("WorktreeHead: %v", err)
				}
				if head.Branch != "main" || head.Commit != mainHash {
					t.Errorf("head = %s, want main at %s", head, mainHash)
				}
			} else if err == nil || !strings.Contains(err.Error(), "dubious ownership") {
				t.Fatalf("WorktreeHead error = %v, want git's ownership refusal", err)
			}
			assertEmptyFile(t, globalConfig)
		})
	}
}

func TestSafeDirectory_LinkedWorktreeGitDir_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A linked worktree is only trusted when the gitdir its .git file names has the same owner too",
		CurrentImpl:     "trustedRepository reads the gitdir from a .git file and checks its owner alongside the worktree and the file",
		ExpectedOutcome: "One owner throughout is trusted, a gitdir with another owner gets git's refusal back",
		Rationale:       "git reads config and hooks from that gitdir and checks its owner itself, trusting the worktree alone would run someone else's hooks",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name        string
		gitDirOwner uint32
		trusted     bool
	}{
		{name: "gitdir with the worktree's owner", gitDirOwner: 1, trusted: true},
		{name: "gitdir with another owner", gitDirOwner: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a linked worktree of a repo git refuses for its owner, with the owner of the gitdir its .git file names faked
			root, mainHash := newNamedRepo(t, "repo")
			linked := filepath.Join(filepath.Dir(root), "linked")
			gitCmd(t, root, "worktree", "add", "-q", linked, "feature")
			globalConfig := assumeDifferentOwner(t, linked)
			old := fileOwner
			fileOwner = func(path string) (uint32, bool) {
				if strings.Contains(filepath.ToSlash(path), "/.git/worktrees/") {
					return tt.gitDirOwner, true
				}
				return 1, true
			}
			t.Cleanup(func() { fileOwner = old })

			// When: the head is read from the linked worktree
			head, err := WorktreeHead(linked)

			// Then: it's read when the gitdir has the same owner, otherwise git's refusal comes back, and the global config is untouched
			if tt.trusted {
				if err != nil {
					t.Fatalf("WorktreeHead: %v", err)
				}
				if head.Branch != "feature" || head.Commit != mainHash {
					t.Errorf("head = %s, want feature at %s", head, mainHash)
				}
			} else if err == nil || !strings.Contains(err.Error(), "dubious ownership") {
				t.Fatalf("WorktreeHead error = %v, want git's ownership refusal", err)
			}
			assertEmptyFile(t, globalConfig)
		})
	}
}

func TestSafeDirectory_GitDirFromFile_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "The gitdir a .git file names is read the way git reads it",
		CurrentImpl:     "gitDirFromFile takes the path after gitdir: and resolves a relative one from the folder holding the file",
		ExpectedOutcome: "Absolute and relative gitdirs resolve, anything else gives no gitdir so the repo isn't trusted",
		Rationale:       "The owner check has to look at the same gitdir git will read config and hooks from",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	base := t.TempDir()
	tests := []struct {
		name    string
		content string
		want    string
		found   bool
	}{
		{name: "absolute", content: "gitdir: /work/repo/.git/worktrees/linked\n", want: "/work/repo/.git/worktrees/linked", found: true},
		{name: "relative", content: "gitdir: ../repo/.git/worktrees/linked\n", want: filepath.Join(base, "repo", ".git", "worktrees", "linked"), found: true},
		{name: "not a gitdir file", content: "ref: refs/heads/main\n"},
		{name: "empty gitdir", content: "gitdir: \n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a .git file in a linked worktree
			dotGit := filepath.Join(base, "linked", ".git")
			if err := os.MkdirAll(filepath.Dir(dotGit), 0o750); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(dotGit, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write .git: %v", err)
			}

			// When: the gitdir is read from it
			got, found := gitDirFromFile(dotGit)

			// Then: it's the gitdir git would use, or nothing
			if got != tt.want || found != tt.found {
				t.Errorf("gitDirFromFile = %q, %v, want %q, %v", got, found, tt.want, tt.found)
			}
		})
	}
}

func TestSafeDirectory_OldGitIgnoresTheRetry_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "When git still refuses a trusted repo after the one command retry, yago says which git it needs",
		CurrentImpl:     "withSafeDirectoryRetry wraps a second refusal with the git version that honours -c safe.directory",
		ExpectedOutcome: "The command ran twice, the second time with safe.directory set, and the error names git 2.38",
		Rationale:       "git before 2.38 ignores safe.directory given with -c, and yago no longer writes it to the global config",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)
	if runtime.GOOS == "windows" {
		t.Skip("yago can't compare owners on windows, so it never retries there")
	}

	// Given: a repo folder the current user owns, and a git that refuses it whatever it's told
	dir := t.TempDir()
	refusal := fmt.Errorf("fatal: detected dubious ownership in repository at '%s'", dir)
	var calls [][]string

	// When: a command runs through the retry
	err := withSafeDirectoryRetry(dir, logging.NewLogger(logging.ERROR), func(extraArgs []string) error {
		calls = append(calls, extraArgs)
		return refusal
	})

	// Then: it was retried once with safe.directory, and the error says git 2.38 is needed
	if len(calls) != 2 || strings.Join(calls[1], " ") != "-c safe.directory="+dir {
		t.Errorf("calls = %q, want a plain run then one with -c safe.directory=%s", calls, dir)
	}
	if err == nil || !strings.Contains(err.Error(), "git 2.38") {
		t.Errorf("error = %v, want it to name git 2.38", err)
	}
}

func TestSafeDirectory_RepoPathFromGitsMessage_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "The repo to trust is read from the first line of git's refusal, in either wording",
		CurrentImpl:     "unsafeRepositoryPath takes everything between the opening quote and where the quote closes on that line",
		ExpectedOutcome: "Quotes inside the path are kept, and a message it can't read gives no path so nothing is retried",
		Rationale:       "git doesn't escape the path on that line, so stopping at the first quote trusted the wrong folder",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name  string
		msg   string
		want  string
		found bool
	}{
		{
			name:  "quote in the path",
			msg:   "git rev-parse --show-toplevel: exit status 128: fatal: detected dubious ownership in repository at '/work/team's-repo'\nTo add an exception for this directory, call:",
			want:  "/work/team's-repo",
			found: true,
		},
		{
			name:  "only line",
			msg:   "fatal: detected dubious ownership in repository at '/work/repo'",
			want:  "/work/repo",
			found: true,
		},
		{
			name:  "windows line ending",
			msg:   "fatal: detected dubious ownership in repository at '/work/repo'\r\nTo add an exception for this directory, call:",
			want:  "/work/repo",
			found: true,
		},
		{
			name:  "older git wording with a quote in the path",
			msg:   "fatal: unsafe repository ('/work/team's-repo' is owned by someone else)\nTo add an exception for this directory, call:",
			want:  "/work/team's-repo",
			found: true,
		},
		{
			name: "no closing quote",
			msg:  "fatal: detected dubious ownership in repository at '/work/repo",
		},
		{
			name: "no path at all",
			msg:  "fatal: detected dubious ownership",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: git's refusal
			err := errors.New(tt.msg)

			// When: the repo path is read from it
			got, found := unsafeRepositoryPath(err)

			// Then: it's the whole path git named, or nothing
			if got != tt.want || found != tt.found {
				t.Errorf("unsafeRepositoryPath = %q, %v, want %q, %v", got, found, tt.want, tt.found)
			}
		})
	}
}
