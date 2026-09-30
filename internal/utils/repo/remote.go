package repo

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

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

	cmd := exec.Command("git", args...) //nolint:gosec // only ever git, no shell, and the options end before any yaml value
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(errors.ErrFail, err, "git ls-remote %s failed (output: %s)", url, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
