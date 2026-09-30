//go:build unix

package repo

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func readInt(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // path comes from t.TempDir
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return n
}

// a git that records its prompt setting, its pid and its process group, then runs body
func promptRecordingGit(t *testing.T, body string) (prompt, pid, pgid string) {
	t.Helper()
	dir := t.TempDir()
	prompt, pid, pgid = filepath.Join(dir, "prompt"), filepath.Join(dir, "pid"), filepath.Join(dir, "pgid")
	fakeGit(t, "printf '%s' \"$GIT_TERMINAL_PROMPT\" > '"+prompt+"'\necho $$ > '"+pid+"'\nps -o pgid= -p $$ > '"+pgid+"'\n"+body)
	return prompt, pid, pgid
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // path comes from t.TempDir
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestLsRemote_NoTerminalNeverPrompts_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Without a terminal, ls-remote runs git with prompts off and in a session of its own",
		CurrentImpl:     "LsRemote sets GIT_TERMINAL_PROMPT=0 and isolateProcessGroup starts git with Setsid when hasTerminal is false",
		ExpectedOutcome: "git sees GIT_TERMINAL_PROMPT=0 and leads its own process group",
		Rationale:       "Nobody can answer a prompt without a terminal, and git needs a group of its own for a timeout to kill its helpers",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: no terminal, and a git that records how it was started
	setTerminal(t, false)
	prompt, pid, pgid := promptRecordingGit(t, "")

	// When: ls-remote runs
	if _, err := LsRemote(nil, "https://example.com/foo/bar.git"); err != nil {
		t.Fatalf("LsRemote: %v", err)
	}

	// Then: prompts are off and git is its own group's leader
	if got := readFile(t, prompt); got != "0" {
		t.Errorf("GIT_TERMINAL_PROMPT = %q, want 0", got)
	}
	if gotPid, gotPgid := readInt(t, pid), readInt(t, pgid); gotPid != gotPgid {
		t.Errorf("git ran as %d in process group %d, want a group of its own", gotPid, gotPgid)
	}
}

func TestLsRemote_TerminalLetsGitPrompt_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "In a terminal, ls-remote lets git prompt with no deadline, in yago's own process group",
		CurrentImpl:     "LsRemote leaves GIT_TERMINAL_PROMPT, the process group and the deadline alone when hasTerminal is true",
		ExpectedOutcome: "git runs past the deadline without being killed, with prompts left on and in yago's process group",
		Rationale:       "Someone typing a password or passphrase mustn't be cut off, and ctrl c has to reach git and its helpers",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a terminal, a git that records how it was started and takes longer than a short deadline
	setTerminal(t, true)
	t.Setenv("GIT_TERMINAL_PROMPT", "")
	prompt, _, pgid := promptRecordingGit(t, "sleep 1")
	old := lsRemoteTimeout
	lsRemoteTimeout = 100 * time.Millisecond
	t.Cleanup(func() { lsRemoteTimeout = old })

	// When: ls-remote runs
	_, err := LsRemote(nil, "https://example.com/foo/bar.git")

	// Then: it wasn't cut off, prompts were left on and git shared yago's process group
	if err != nil {
		t.Fatalf("LsRemote: %v", err)
	}
	if got := readFile(t, prompt); got == "0" {
		t.Errorf("GIT_TERMINAL_PROMPT = %q, want prompts left on", got)
	}
	if got := readInt(t, pgid); got != syscall.Getpgrp() {
		t.Errorf("git ran in process group %d, want yago's %d so ctrl c reaches it", got, syscall.Getpgrp())
	}
}

func TestLsRemote_KillsLeftoverChildren_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Without a terminal, a timed out ls-remote takes the helpers git started with it",
		CurrentImpl:     "isolateProcessGroup cancels by killing git's whole process group",
		ExpectedOutcome: "The child still holding git's output is gone soon after the timeout",
		Rationale:       "ssh or git-remote-https outliving a killed git kept running and held its output",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: no terminal, a git that hangs with a child holding its output, the way ssh outlives a killed git, and a short deadline
	setTerminal(t, false)
	child := filepath.Join(t.TempDir(), "child")
	fakeGit(t, "sleep 30 &\necho $! > '"+child+"'\nwait")
	old := lsRemoteTimeout
	lsRemoteTimeout = time.Second
	t.Cleanup(func() { lsRemoteTimeout = old })

	// When: ls-remote runs past its deadline
	_, err := LsRemote(nil, "https://example.com/foo/bar.git")

	// Then: it timed out and the child is gone
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("LsRemote error = %v, want a timeout", err)
	}
	pid := readInt(t, child)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	deadline := time.Now().Add(5 * time.Second)
	for !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d still running after the timeout", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLookupOwner_RealUID_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A path's owner is read as its real uid, and a missing path has none",
		CurrentImpl:     "lookupOwner reads Stat_t.Uid from an lstat",
		ExpectedOutcome: "A folder this process made has its uid, and a missing path gives no owner",
		Rationale:       "Trusting a repo git refused rests on this uid, a lookup that always agreed would trust someone else's repo",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a folder this process made, and a path that isn't there
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")

	// When: their owners are looked up
	owner, ok := lookupOwner(dir)
	_, missingOK := lookupOwner(missing)

	// Then: the folder has this process's uid and the missing path has no owner
	if !ok || int(owner) != os.Geteuid() {
		t.Errorf("lookupOwner(%s) = %d, %v, want %d", dir, owner, ok, os.Geteuid())
	}
	if missingOK {
		t.Errorf("lookupOwner(%s) found an owner for a missing path", missing)
	}
}
