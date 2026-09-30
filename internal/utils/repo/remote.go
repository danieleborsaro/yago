package repo

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/mattn/go-isatty"
)

// without a terminal an unreachable host would otherwise hang yago forever
var lsRemoteTimeout = 60 * time.Second

// swapped in tests, which may or may not run in a terminal
var hasTerminal = func() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// RejectOptionLike refuses values from yaml that git would read as an option, the upload pack one runs any command it's given
func RejectOptionLike(kind, value string) error {
	if strings.HasPrefix(value, "-") {
		return errors.Newf(errors.ErrParam, "%s %q can't start with a dash", kind, value)
	}
	return nil
}

// LsRemote runs git ls-remote with the options ended before the url and patterns, and returns stdout, stderr goes in the error
func LsRemote(flags []string, url string, patterns ...string) ([]byte, error) {
	if err := RejectOptionLike("repository URL", url); err != nil {
		return nil, err
	}
	for _, p := range patterns {
		if err := RejectOptionLike("ref", p); err != nil {
			return nil, err
		}
	}

	args := append([]string{"ls-remote"}, flags...)
	args = append(args, "--", url)
	args = append(args, patterns...)

	// in a terminal git asks for credentials like it always has, with no deadline to cut someone off mid password,
	// and ctrl c stops it with its helpers, without one nobody can answer, so it fails fast and a deadline stops a
	// host that never answers
	interactive := hasTerminal()
	ctx, cancel := context.Background(), context.CancelFunc(func() {})
	if !interactive {
		ctx, cancel = context.WithTimeout(ctx, lsRemoteTimeout)
	}
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // only ever git, no shell, and the options end before any yaml value
	if !interactive {
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		isolateProcessGroup(cmd)
	}
	// where the whole group can't be killed, a helper like ssh can outlive git and keep the pipes open
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.Newf(errors.ErrFail, "git ls-remote %s timed out after %s", url, lsRemoteTimeout)
		}
		return nil, errors.Wrapf(errors.ErrFail, err, "git ls-remote %s failed (output: %s)", url, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
