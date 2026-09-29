package repo

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// SwitchWorktree checks out ref in the work tree holding path, the way the python tool read a desired state
// from another branch, it does nothing when ref is already checked out, and refuses when tracked files have
// uncommitted changes rather than carry them over to ref or stash them
func SwitchWorktree(path, ref string) error {
	if ref == "" {
		return nil
	}
	dir, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	top, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		// the python tool warned and went on with the files as they were, so this does too
		if strings.Contains(err.Error(), "not a git repository") {
			logging.Warn("%s isn't in a git repository, so %s can't be checked out, using the files as they are", path, ref)
			return nil
		}
		return err
	}

	if current, err := gitOutput(top, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil && current == ref {
		logging.Debug("%s already has %s checked out", top, ref)
		return nil
	}

	changes, err := gitOutput(top, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return err
	}
	if changes != "" {
		return errors.Newf(errors.ErrFail, "%s has uncommitted changes, commit or stash them before %s is checked out:\n%s", top, ref, changes)
	}

	if _, err := gitOutput(top, "checkout", "--quiet", ref); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to check out %s in %s", ref, top)
	}
	logging.Info("Checked out %s in %s", ref, top)
	return nil
}

// git's messages are matched on, so they're asked for in english
func gitOutput(dir string, args ...string) (string, error) {
	var out string
	err := withSafeDirectoryRetry(dir, logging.NewLogger(logging.INFO), func() error {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		out = strings.TrimRight(stdout.String(), "\r\n")
		return nil
	})
	return out, err
}
