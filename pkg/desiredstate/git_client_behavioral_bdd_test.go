package desiredstate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/pkg/desiredstate"
)

func TestGitClientResolve_OptionLikeValues_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Resolving a component ref refuses a URL or ref that git would read as an option",
		CurrentImpl:     "GitClient.ResolveRefToCommit checks the ref, then goes through uRepo.LsRemote",
		ExpectedOutcome: "An error comes back before any ls-remote runs",
		Rationale:       "git ls-remote --upload-pack=<cmd> runs <cmd> when a dash led component URL lands where git reads options, and refs from the same yaml are refused the same way",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name string
		url  string
		ref  string
	}{
		{name: "url", url: "--upload-pack=touch %s", ref: "v1.0.0"},
		{name: "ref", url: "https://example.com/foo/bar.git", ref: "--upload-pack=touch %s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a value that would create a marker file if git ran it
			marker := filepath.Join(t.TempDir(), "pwned")
			url := strings.Replace(tt.url, "%s", marker, 1)
			ref := strings.Replace(tt.ref, "%s", marker, 1)

			// When: the ref is resolved
			_, err := desiredstate.NewGitClient().ResolveRefToCommit(url, ref)

			// Then: it's refused and the marker was never made
			if err == nil || !strings.Contains(err.Error(), "can't start with a dash") {
				t.Fatalf("ResolveRefToCommit error = %v, want a dash refusal", err)
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Errorf("%s exists, git ran the injected command", marker)
			}
		})
	}
}
