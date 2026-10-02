package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
)

func quietConfig() *RepoConfig {
	return &RepoConfig{Logger: logging.NewLogger(logging.ERROR)}
}

// clone shells out to git, so the developer's global config (signing, hooks) has to stay out
func isolateGitConfig(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// origin is a bare repo with main and feature, its dir name doubles as the repo name clones get
func newOrigin(t *testing.T, name string) string {
	t.Helper()
	isolateGitConfig(t)
	work := t.TempDir()
	gitCmd(t, work, "init", "-q", "-b", "main")
	gitCmd(t, work, "commit", "-q", "--allow-empty", "-m", "main")
	gitCmd(t, work, "branch", "feature")
	origin := filepath.Join(t.TempDir(), name+".git")
	gitCmd(t, work, "clone", "-q", "--bare", work, origin)
	return origin
}

func TestClone_NamesAndCachesTheWorkDir_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Clone into <path>/<repo name> and remember it, so a second clone of the same URL reuses it",
		CurrentImpl:     "Clone runs git clone into NamedWorkDir(path, url), then CacheRepo(url, path)",
		ExpectedOutcome: "The clone lands in a dir named after the repo, and a repeat call returns that dir without cloning again",
		Rationale:       "One run can reference the same repo from many desired states, cloning it each time is slow and wastes disk",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: an origin nobody has cloned yet
	origin := newOrigin(t, "clone-named")
	base := t.TempDir()

	// When: it's cloned twice into different bases
	first, err := Clone(origin, base, false, quietConfig())
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	second, err := Clone(origin, t.TempDir(), false, quietConfig())
	if err != nil {
		t.Fatalf("second Clone: %v", err)
	}

	// Then: the first lands under the repo name, the second is the cached one
	want := filepath.Join(ExpandPath(base), "clone-named")
	if first.GetPath() != want {
		t.Errorf("path = %s, want %s", first.GetPath(), want)
	}
	if second.GetPath() != want {
		t.Errorf("second clone path = %s, want the cached %s", second.GetPath(), want)
	}
	if first.IsBare() {
		t.Error("a plain clone reports itself as bare")
	}
}

func TestClone_ForceBypassesCache_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A forced clone ignores the cache",
		CurrentImpl:     "Clone skips GetCachedRepo when force is true",
		ExpectedOutcome: "A fresh clone at the new path",
		Rationale:       "Callers that need a clean copy can't be handed a cached dir someone else already changed",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: an origin that's already been cloned
	origin := newOrigin(t, "clone-force")
	if _, err := Clone(origin, t.TempDir(), false, quietConfig()); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// When: it's cloned again with force
	base := t.TempDir()
	r, err := Clone(origin, base, true, quietConfig())

	// Then: it's a new clone at the new base
	if err != nil {
		t.Fatalf("forced Clone: %v", err)
	}
	if want := filepath.Join(ExpandPath(base), "clone-force"); r.GetPath() != want {
		t.Errorf("path = %s, want %s", r.GetPath(), want)
	}
}

func TestCloneBranch_ChecksOutTheBranch_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Clone a single branch, and put a cached clone back on that branch",
		CurrentImpl:     "CloneBranch clones with --branch --single-branch and caches under url#branch, a cache hit checks the branch out again",
		ExpectedOutcome: "The clone is on the branch both times",
		Rationale:       "Desired states pin a branch, reading files from whatever a cached clone was last left on would be wrong",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: an origin with a feature branch
	origin := newOrigin(t, "clone-branch")

	// When: the branch is cloned, moved off, then asked for again
	r, err := CloneBranch(origin, t.TempDir(), "feature", false, quietConfig())
	if err != nil {
		t.Fatalf("CloneBranch: %v", err)
	}
	gitCmd(t, r.GetPath(), "checkout", "-q", "--detach")
	again, err := CloneBranch(origin, t.TempDir(), "feature", false, quietConfig())

	// Then: both are on feature, in the same dir
	if err != nil {
		t.Fatalf("second CloneBranch: %v", err)
	}
	if again.GetPath() != r.GetPath() {
		t.Errorf("second clone path = %s, want the cached %s", again.GetPath(), r.GetPath())
	}
	if got := gitCmd(t, r.GetPath(), "branch", "--show-current"); got != "feature" {
		t.Errorf("branch = %q, want feature", got)
	}
}

func TestCloneBranch_UnknownBranch_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Fail when the branch isn't on the remote",
		CurrentImpl:     "CloneBranch wraps the git clone error with the branch name",
		ExpectedOutcome: "An error naming the branch and nothing cached",
		Rationale:       "A typo in a desired state has to stop the run, not deploy the default branch",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: an origin without the branch
	origin := newOrigin(t, "clone-missing")

	// When: the missing branch is cloned
	_, err := CloneBranch(origin, t.TempDir(), "nope", false, quietConfig())

	// Then: it fails naming the branch and isn't cached
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("error = %v, want one naming the branch", err)
	}
	if IsCached(origin + "#nope") {
		t.Error("a failed clone was cached")
	}
}

func TestCloneBareAndMirror_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Bare and mirror clones land in <repo>.bare and report themselves as bare",
		CurrentImpl:     "CloneBare and CloneMirror use NamedWorkDir with the bare flag and cache under url#bare and url#mirror",
		ExpectedOutcome: "A bare repo in <base>/<repo>.bare, cached separately from a normal clone",
		Rationale:       "Bundles are made from bare clones, and a bare and a normal clone of one URL must not share a cache entry",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name  string
		clone func(url, path string, force bool, config *RepoConfig) (*Repository, error)
		key   string
	}{
		{name: "bare", clone: CloneBare, key: "#bare"},
		{name: "mirror", clone: CloneMirror, key: "#mirror"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: an origin
			origin := newOrigin(t, "clone-"+tt.name)
			base := t.TempDir()

			// When: it's cloned bare
			r, err := tt.clone(origin, base, false, quietConfig())

			// Then: it's bare, under <repo>.bare and cached under its own key
			if err != nil {
				t.Fatalf("clone: %v", err)
			}
			if want := filepath.Join(ExpandPath(base), "clone-"+tt.name+".bare"); r.GetPath() != want {
				t.Errorf("path = %s, want %s", r.GetPath(), want)
			}
			if !r.IsBare() {
				t.Error("IsBare() = false for a bare clone")
			}
			if r.IsMirror() != (tt.name == "mirror") {
				t.Errorf("IsMirror() = %v for a %s clone", r.IsMirror(), tt.name)
			}
			if !IsCached(origin + tt.key) {
				t.Errorf("not cached under %s%s", origin, tt.key)
			}
			if IsCached(origin) {
				t.Error("a bare clone took the plain clone's cache entry")
			}
		})
	}
}

func TestBundle_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Bundle a bare repo into <repo>.bundle, and refuse a non bare one",
		CurrentImpl:     "Repository.Bundle runs git bundle create --all, named after the repo without .bare",
		ExpectedOutcome: "A bundle git can verify, or an error for a working clone",
		Rationale:       "Bundles carry repos into air gapped environments, a half made one fails far from here",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	t.Run("bare repo", func(t *testing.T) {
		// Given: a bare clone
		origin := newOrigin(t, "bundle-bare")
		r, err := CloneBare(origin, t.TempDir(), false, quietConfig())
		if err != nil {
			t.Fatalf("CloneBare: %v", err)
		}

		// When: it's bundled into a new dir
		dest := filepath.Join(t.TempDir(), "out")
		file, err := r.Bundle(dest)

		// Then: the bundle is there and valid
		if err != nil {
			t.Fatalf("Bundle: %v", err)
		}
		if want := filepath.Join(dest, "bundle-bare.bundle"); file != want {
			t.Errorf("bundle = %s, want %s", file, want)
		}
		gitCmd(t, r.GetPath(), "bundle", "verify", "-q", file)
	})

	t.Run("working clone", func(t *testing.T) {
		// Given: a normal clone
		origin := newOrigin(t, "bundle-work")
		r, err := Clone(origin, t.TempDir(), false, quietConfig())
		if err != nil {
			t.Fatalf("Clone: %v", err)
		}

		// When: it's bundled
		_, err = r.Bundle(t.TempDir())

		// Then: it's refused
		if err == nil || !strings.Contains(err.Error(), "not bare") {
			t.Errorf("error = %v, want a not bare error", err)
		}
	})
}

func TestRepository_CommitAndPush_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Stage, commit and push through go-git",
		CurrentImpl:     "Add, AddAll, Commit and Push wrap the go-git worktree and remote",
		ExpectedOutcome: "The commit is on origin with the given author, and the clone is clean again",
		Rationale:       "Promote writes desired states back, a commit that never reaches origin is a silent no op",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a clean clone
	origin := newOrigin(t, "commit-push")
	r, err := Clone(origin, t.TempDir(), true, quietConfig())
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if clean, err := r.IsClean(); err != nil || !clean {
		t.Fatalf("fresh clone clean = %v, err %v", clean, err)
	}

	// When: two files are added one way each, committed and pushed
	for _, name := range []string{"a.yaml", "b.yaml"} {
		if err := os.WriteFile(filepath.Join(r.GetPath(), name), []byte("x: 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if clean, _ := r.IsClean(); clean {
		t.Error("IsClean() = true with untracked files")
	}
	if err := r.Add("a.yaml"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := gitCmd(t, r.GetPath(), "diff", "--cached", "--name-only"); got != "a.yaml" {
		t.Errorf("staged after Add = %q, want only a.yaml", got)
	}
	if err := r.AddAll(); err != nil {
		t.Fatalf("AddAll: %v", err)
	}
	hash, err := r.Commit("add files", "Foo Bot", "team@example.com")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := r.Push(); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Then: origin has the commit with that author, and the clone is clean
	if got := gitCmd(t, origin, "rev-parse", "main"); got != hash {
		t.Errorf("origin main = %s, want %s", got, hash)
	}
	if got := gitCmd(t, origin, "log", "-1", "--format=%an <%ae>", "main"); got != "Foo Bot <team@example.com>" {
		t.Errorf("author = %q", got)
	}
	if got := gitCmd(t, origin, "show", "--name-only", "--format=", "main"); got != "a.yaml\nb.yaml" {
		t.Errorf("committed files = %q, want a.yaml and b.yaml", got)
	}
	if clean, _ := r.IsClean(); !clean {
		t.Error("IsClean() = false after committing everything")
	}
	if err := r.Push(); err != nil {
		t.Errorf("pushing again with nothing new: %v", err)
	}
}

func TestRepository_PullAndFetch_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Pull and fetch bring in new origin commits, and an up to date repo isn't an error",
		CurrentImpl:     "Pull and Fetch use go-git, the all variants shell out to git, and a bare repo pull becomes a fetch",
		ExpectedOutcome: "The clone reaches origin's commit and a repeat call succeeds",
		Rationale:       "Cached clones get reused across a run, they have to be refreshable without tripping on NoErrAlreadyUpToDate",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// origin moves on after the clone, pusher is a second clone that pushes
	advance := func(t *testing.T, origin string) string {
		t.Helper()
		pusher := filepath.Join(t.TempDir(), "pusher")
		gitCmd(t, filepath.Dir(pusher), "clone", "-q", origin, pusher)
		gitCmd(t, pusher, "commit", "-q", "--allow-empty", "-m", "next")
		gitCmd(t, pusher, "push", "-q", "origin", "main")
		return gitCmd(t, pusher, "rev-parse", "HEAD")
	}

	tests := []struct {
		name string
		bare bool
		sync func(r *Repository) error
		ref  string
	}{
		{name: "pull", sync: func(r *Repository) error { return r.Pull("", false) }, ref: "HEAD"},
		{name: "pull branch", sync: func(r *Repository) error { return r.Pull("main", false) }, ref: "HEAD"},
		{name: "pull all", sync: func(r *Repository) error { return r.Pull("", true) }, ref: "HEAD"},
		{name: "fetch", sync: func(r *Repository) error { return r.Fetch(false) }, ref: "origin/main"},
		{name: "fetch all", sync: func(r *Repository) error { return r.Fetch(true) }, ref: "origin/main"},
		{name: "pull on bare", bare: true, sync: func(r *Repository) error { return r.Pull("", false) }, ref: "main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a clone that's fallen behind origin
			origin := newOrigin(t, "sync")
			clone := Clone
			if tt.bare {
				clone = CloneMirror
			}
			r, err := clone(origin, t.TempDir(), true, quietConfig())
			if err != nil {
				t.Fatalf("clone: %v", err)
			}
			want := advance(t, origin)

			// When: it syncs twice
			if err := tt.sync(r); err != nil {
				t.Fatalf("sync: %v", err)
			}
			err = tt.sync(r)

			// Then: it has origin's commit and the repeat is fine
			if err != nil {
				t.Errorf("syncing when already up to date: %v", err)
			}
			if got := gitCmd(t, r.GetPath(), "rev-parse", tt.ref); got != want {
				t.Errorf("%s = %s, want %s", tt.ref, got, want)
			}
		})
	}
}

func TestRepository_HeadInfo_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Report the current branch and commit, falling back to a short hash when detached",
		CurrentImpl:     "GetCurrentBranch returns the branch or the first 8 chars of HEAD, GetCurrentCommitShort trims to 8",
		ExpectedOutcome: "Branch name on a branch, the 8 char hash when detached, and the full hash from GetCurrentCommit",
		Rationale:       "Log lines and promote summaries show these, a detached checkout of a tag must not error out",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a clone on main
	fx := newCheckoutFixture(t)
	r, err := NewRepository(fx.clone, quietConfig())
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}

	// When: HEAD is read on main and then detached
	branch, _ := r.GetCurrentBranch()
	commit, _ := r.GetCurrentCommit()
	short, _ := r.GetCurrentCommitShort()
	gitCmd(t, fx.clone, "checkout", "-q", "--detach", fx.featHash)
	detached, err := r.GetCurrentBranch()

	// Then: the branch, full and short hash, and the detached short hash
	if branch != "main" {
		t.Errorf("branch = %q, want main", branch)
	}
	if commit != fx.mainHash {
		t.Errorf("commit = %s, want %s", commit, fx.mainHash)
	}
	if short != fx.mainHash[:8] {
		t.Errorf("short commit = %s, want %s", short, fx.mainHash[:8])
	}
	if err != nil || detached != fx.featHash[:8] {
		t.Errorf("detached branch = %q, err %v, want %s", detached, err, fx.featHash[:8])
	}
}

func TestRepository_CreateBranch_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Create a branch at HEAD and switch to it",
		CurrentImpl:     "CreateBranch checks out a new branch with Create set",
		ExpectedOutcome: "The clone is on the new branch at the same commit, and creating it twice fails",
		Rationale:       "Promote writes on its own branch, silently reusing an existing one could mix changes",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a clone on main
	fx := newCheckoutFixture(t)
	r, err := NewRepository(fx.clone, quietConfig())
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}

	// When: a branch is created, then created again
	err = r.CreateBranch("promote/example")
	switched := gitCmd(t, fx.clone, "branch", "--show-current")
	gitCmd(t, fx.clone, "checkout", "-q", "main")
	again := r.CreateBranch("promote/example")

	// Then: the first works at main's commit and the repeat fails
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if switched != "promote/example" {
		t.Errorf("branch after CreateBranch = %q, want promote/example", switched)
	}
	if got := gitCmd(t, fx.clone, "rev-parse", "promote/example"); got != fx.mainHash {
		t.Errorf("branch at %s, want %s", got, fx.mainHash)
	}
	if again == nil {
		t.Error("creating an existing branch succeeded")
	}
}

func TestLoad_LinkedWorktree_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A linked git worktree loads with the main repo's origin and as a working repo, whatever the main repo is",
		CurrentImpl:     "openRepository reads config from the common dir for a .git file, and calls a repo bare only when go-git finds no worktree",
		ExpectedOutcome: "Load and NewRepository report origin, not bare, not mirror, and Load caches it under the plain url",
		Rationale:       "go-git used to miss a worktree's remotes, and a worktree of a bare repo inherits core.bare=true so it looked bare and Pull only fetched",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name  string
		flags []string
	}{
		{name: "worktree of a clone"},
		{name: "worktree of a bare repo", flags: []string{"--bare"}},
		{name: "worktree of a mirror", flags: []string{"--mirror"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a linked worktree of a main repo made with the flags
			fx := newCheckoutFixture(t)
			url := gitCmd(t, fx.clone, "remote", "get-url", "origin")
			hub := fx.clone
			if len(tt.flags) > 0 {
				hub = filepath.Join(t.TempDir(), "hub.git")
				gitCmd(t, fx.clone, append(append([]string{"clone", "-q"}, tt.flags...), url, hub)...)
			}
			linked := filepath.Join(t.TempDir(), "linked")
			gitCmd(t, hub, "worktree", "add", "-q", "--detach", linked, "main")

			// When: it's loaded both ways
			loaded, isGit, err := Load(linked, quietConfig())
			if err != nil || !isGit {
				t.Fatalf("Load = %v, %v", isGit, err)
			}
			opened, err := NewRepository(linked, quietConfig())
			if err != nil {
				t.Fatalf("NewRepository: %v", err)
			}

			// Then: both see origin as a working repo, and it's cached as one
			for name, r := range map[string]*Repository{"Load": loaded, "NewRepository": opened} {
				if got, err := r.GetRemoteURL(); err != nil || got != url {
					t.Errorf("%s remote = %q, err %v, want %s", name, got, err, url)
				}
				if r.IsBare() || r.IsMirror() {
					t.Errorf("%s IsBare = %v, IsMirror = %v for a worktree", name, r.IsBare(), r.IsMirror())
				}
			}
			if cached, _ := GetCachedRepo(url); cached != loaded.GetPath() {
				t.Errorf("cache for %s = %q, want the worktree %s", url, cached, loaded.GetPath())
			}
		})
	}
}

func TestLoad_BareRepoCacheKey_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Loading a bare or mirror repo caches it under url#bare or url#mirror, never the plain url",
		CurrentImpl:     "Load caches under CacheKey(url, isBare, isMirror)",
		ExpectedOutcome: "A later Clone of the url makes a working clone instead of reusing the bare dir",
		Rationale:       "Load used to cache every repo under its plain url, so a loaded bare repo was handed out to callers that need files",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		flag string
		key  string
	}{
		{flag: "--bare", key: "#bare"},
		{flag: "--mirror", key: "#mirror"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			// Given: a bare repo made outside yago
			isolateGitConfig(t)
			fx := newCheckoutFixture(t)
			url := gitCmd(t, fx.clone, "remote", "get-url", "origin")
			dir := filepath.Join(t.TempDir(), "loaded.git")
			gitCmd(t, fx.clone, "clone", "-q", tt.flag, url, dir)

			// When: it's loaded, then the url is cloned
			if _, isGit, err := Load(dir, quietConfig()); err != nil || !isGit {
				t.Fatalf("Load = %v, %v", isGit, err)
			}
			r, err := Clone(url, t.TempDir(), false, quietConfig())

			// Then: the load is cached under its own key and the clone is a working one
			if cached, _ := GetCachedRepo(url + tt.key); cached != dir {
				t.Errorf("cache for %s%s = %q, want %s", url, tt.key, cached, dir)
			}
			if err != nil {
				t.Fatalf("Clone: %v", err)
			}
			if r.IsBare() || r.GetPath() == dir {
				t.Errorf("Clone reused the loaded bare repo %s", r.GetPath())
			}
		})
	}
}

func TestInitRepository_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Init a new repo, and tell repos from plain dirs",
		CurrentImpl:     "InitRepository wraps git.PlainInit, IsRepository looks for a .git dir",
		ExpectedOutcome: "A new repo with no origin, IsRepository true for it and false for a plain dir",
		Rationale:       "Local desired state dirs don't have to be repos, the code has to tell which it got",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: an empty dir and a plain dir
	dir := t.TempDir()
	plain := t.TempDir()

	// When: the first is initialised
	r, err := InitRepository(dir, quietConfig())

	// Then: it's a repo with no origin and the plain dir isn't
	if err != nil {
		t.Fatalf("InitRepository: %v", err)
	}
	if !IsRepository(dir) {
		t.Error("IsRepository = false for a new repo")
	}
	if IsRepository(plain) {
		t.Error("IsRepository = true for a plain dir")
	}
	if _, err := r.GetRemoteURL(); err == nil {
		t.Error("GetRemoteURL worked on a repo without origin")
	}
	if _, err := InitRepository(dir, quietConfig()); err == nil {
		t.Error("initialising an existing repo succeeded")
	}
}
