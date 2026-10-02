package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	uRepo "github.com/danieleborsaro/yago/internal/utils/repo"
)

func TestRepoCache_BareCloneStaysOutOfWorkingClones_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "A bare or mirror clone of a url is never handed out as the working clone of that url",
		CurrentImpl:     "Repo caches under uRepo.CacheKey, which suffixes #bare or #mirror",
		ExpectedOutcome: "Cloning the url after a bare clone gives a new working clone with files in it",
		Rationale:       "Repo.CloneBare used to cache under the plain url, so the next Clone got a bare dir and no files",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name  string
		clone func(r *Repo) error
	}{
		{name: "bare", clone: (*Repo).CloneBare},
		{name: "mirror", clone: (*Repo).CloneMirror},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: an origin with a file, already cloned bare
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			origin := t.TempDir()
			gitCmd(t, origin, "init", "-q", "-b", "main")
			if err := os.WriteFile(filepath.Join(origin, "state.yaml"), []byte("x: 1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			gitCmd(t, origin, "add", ".")
			gitCmd(t, origin, "commit", "-q", "-m", "state")
			url := "file://" + origin
			cfg := &uRepo.RepoConfig{Logger: logging.NewLogger(logging.ERROR)}
			bare := NewRepo(url, "", t.TempDir(), cfg)
			if err := tt.clone(bare); err != nil {
				t.Fatalf("bare clone: %v", err)
			}

			// When: the same url is cloned for its files
			work := NewRepo(url, "", t.TempDir(), cfg)
			err := work.Clone()

			// Then: it's a separate working clone with the file
			if err != nil {
				t.Fatalf("Clone: %v", err)
			}
			if work.WorkDir == bare.WorkDir {
				t.Errorf("working clone reused the %s dir %s", tt.name, bare.WorkDir)
			}
			if _, err := os.Stat(filepath.Join(work.WorkDir, "state.yaml")); err != nil {
				t.Errorf("working clone has no state.yaml: %v", err)
			}
			if !bare.IsBare || bare.IsMirror != (tt.name == "mirror") {
				t.Errorf("bare clone IsBare = %v, IsMirror = %v", bare.IsBare, bare.IsMirror)
			}
		})
	}
}
