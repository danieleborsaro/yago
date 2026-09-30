package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// puts a fake git first on PATH that writes its args one per line to the returned file, then runs body
func fakeGit(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argsFile + "'\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o700); err != nil { //nolint:gosec // test script has to be executable
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsFile
}

func readArgs(t *testing.T, argsFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argsFile) //nolint:gosec // path comes from t.TempDir
	if err != nil {
		t.Fatalf("read fake git args: %v", err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func TestLsRemote_OptionLikeValues_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "ls-remote refuses a URL or ref from yaml that starts with a dash",
		CurrentImpl:     "LsRemote checks every value with RejectOptionLike before git runs",
		ExpectedOutcome: "An ErrParam error comes back and git is never started",
		Rationale:       "git ls-remote --upload-pack=<cmd> runs <cmd>, so a crafted desired state could run anything",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name     string
		url      string
		patterns []string
	}{
		{name: "upload pack url", url: "--upload-pack=touch pwned"},
		{name: "short option url", url: "-u"},
		{name: "option ref", url: "https://example.com/foo/bar.git", patterns: []string{"--exec=touch pwned"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a git on PATH that records whether it ran
			argsFile := fakeGit(t, "exit 0")

			// When: ls-remote is asked for the dash led value
			_, err := LsRemote(nil, tt.url, tt.patterns...)

			// Then: it's refused and git never ran
			if err == nil || !strings.Contains(err.Error(), "can't start with a dash") {
				t.Fatalf("LsRemote error = %v, want a dash refusal", err)
			}
			if _, statErr := os.Stat(argsFile); !os.IsNotExist(statErr) {
				t.Errorf("git ran with %v, it should never have started", readArgs(t, argsFile))
			}
		})
	}
}

func TestLsRemote_EndsOptionsBeforeURL_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "ls-remote puts the URL and patterns after --",
		CurrentImpl:     "LsRemote builds ls-remote <flags> -- <url> <patterns>",
		ExpectedOutcome: "git sees the flags yago chose, then --, then the values from yaml",
		Rationale:       "Ending the options means git can never read a yaml value as a flag",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a git on PATH that records its args
	argsFile := fakeGit(t, "exit 0")

	// When: ls-remote runs with a flag, a url and a pattern
	if _, err := LsRemote([]string{"--heads"}, "https://example.com/foo/bar.git", "refs/heads/main"); err != nil {
		t.Fatalf("LsRemote: %v", err)
	}

	// Then: the url and pattern come after the end of options
	got := strings.Join(readArgs(t, argsFile), " ")
	want := "ls-remote --heads -- https://example.com/foo/bar.git refs/heads/main"
	if got != want {
		t.Errorf("git args = %q, want %q", got, want)
	}
}

func TestLsRemote_HungRemote_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "ls-remote gives up on a remote that never answers",
		CurrentImpl:     "LsRemote runs git under a context deadline with a WaitDelay for leftover children",
		ExpectedOutcome: "A timed out error comes back soon after the deadline instead of hanging",
		Rationale:       "An unreachable host or a credential prompt used to hang yago with no way out but ctrl c",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a git that hangs, and a short deadline
	fakeGit(t, "sleep 30")
	old := lsRemoteTimeout
	lsRemoteTimeout = 200 * time.Millisecond
	t.Cleanup(func() { lsRemoteTimeout = old })

	// When: ls-remote runs
	start := time.Now()
	_, err := LsRemote(nil, "https://example.com/foo/bar.git")
	elapsed := time.Since(start)

	// Then: it times out well before the fake git would have finished
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("LsRemote error = %v, want a timeout", err)
	}
	if elapsed > 10*time.Second {
		t.Errorf("LsRemote took %s, the deadline was %s", elapsed, lsRemoteTimeout)
	}
}

func TestLsRemote_LocalRemote_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "ls-remote still lists refs from a real remote",
		CurrentImpl:     "LsRemote returns stdout of git ls-remote",
		ExpectedOutcome: "The branch and its commit come back, and a missing ref gives empty output",
		Rationale:       "The -- and the timeout must not change what callers parse",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a remote with one commit on main
	fx := newCheckoutFixture(t)

	// When: its heads are listed, and a ref it doesn't have
	out, err := LsRemote([]string{"--heads"}, fx.clone, "refs/heads/main")
	if err != nil {
		t.Fatalf("LsRemote: %v", err)
	}
	missing, err := LsRemote(nil, fx.clone, "refs/heads/nope")
	if err != nil {
		t.Fatalf("LsRemote missing ref: %v", err)
	}

	// Then: main points at its commit and the missing ref lists nothing
	if !strings.HasPrefix(string(out), fx.mainHash+"\trefs/heads/main") {
		t.Errorf("ls-remote output = %q, want main at %s", out, fx.mainHash)
	}
	if strings.TrimSpace(string(missing)) != "" {
		t.Errorf("ls-remote for a missing ref = %q, want nothing", missing)
	}
}
