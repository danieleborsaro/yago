package repo

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// an unreachable host or a credential prompt would otherwise hang yago forever
var lsRemoteTimeout = 60 * time.Second

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

	ctx, cancel := context.WithTimeout(context.Background(), lsRemoteTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // only ever git, no shell, and the options end before any yaml value
	// ssh can outlive a killed git and keep the pipes open
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
