package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	uRepo "github.com/danieleborsaro/yago/internal/utils/repo"
)

func TestRepoValidation_OptionLikeValues_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior:        "Repo validation refuses a URL or ref from yaml that git would read as an option",
		CurrentImpl:     "ValidateRemote and ValidateRef go through uRepo.LsRemote",
		ExpectedOutcome: "A validation error comes back and nothing the value names gets run",
		Rationale:       "git ls-remote --upload-pack=<cmd> runs <cmd>, the URL and ref come from desired state yaml",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	config := &uRepo.RepoConfig{Logger: logging.NewLogger(logging.ERROR)}

	tests := []struct {
		name     string
		url      string
		ref      string
		validate func(*Repo) error
	}{
		{name: "remote url", url: "--upload-pack=touch %s", validate: (*Repo).ValidateRemote},
		{name: "ref url", url: "--upload-pack=touch %s", ref: "main", validate: (*Repo).ValidateRef},
		{name: "ref", url: "https://example.com/foo/bar.git", ref: "--upload-pack=touch %s", validate: (*Repo).ValidateRef},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a value that would create a marker file if git ran it
			marker := filepath.Join(t.TempDir(), "pwned")
			r := &Repo{URL: strings.Replace(tt.url, "%s", marker, 1), Ref: strings.Replace(tt.ref, "%s", marker, 1), config: config}

			// When: the repo is validated
			err := tt.validate(r)

			// Then: validation fails and the marker was never made
			if err == nil || !strings.Contains(err.Error(), "can't start with a dash") {
				t.Fatalf("validation error = %v, want a dash refusal", err)
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Errorf("%s exists, git ran the injected command", marker)
			}
		})
	}
}
