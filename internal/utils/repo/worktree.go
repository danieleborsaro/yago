package repo

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// git would take it as an option, and some of those throw local changes away
	if strings.HasPrefix(ref, "-") {
		return errors.Newf(errors.ErrParam, "%q is not a branch, tag or commit", ref)
	}

	top, err := worktreeTop(path)
	if err != nil {
		return err
	}
	if top == "" {
		// the python tool warned and went on with the files as they were, so this does too
		logging.Warn("%s isn't in a git repository, so %s can't be checked out, using the files as they are", path, ref)
		return nil
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

	// resolved before the checkout moves HEAD, a branch that's only on origin won't resolve yet and gets
	// matched by its name after
	want, _ := gitOutput(top, "rev-parse", "--verify", "--quiet", ref+"^{commit}")

	// ending the options makes git take ref as a branch, tag or commit, never as a file it checks out while
	// HEAD stays put, and an ignored file in the way stops the checkout like an untracked one instead of
	// being overwritten
	if _, err := gitOutput(top, "checkout", "--quiet", "--no-overwrite-ignore", ref, "--"); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to check out %s in %s", ref, top)
	}

	head := headOf(top)
	if head.Branch != ref && (want == "" || head.Commit != want) {
		return errors.Newf(errors.ErrFail, "checking out %s in %s left %s checked out", ref, top, head)
	}
	logging.Info("Checked out %s in %s", ref, top)
	return nil
}

// Head is what a work tree has checked out, Branch is empty when HEAD is detached and Commit is empty on a
// branch with no commits yet
type Head struct {
	Branch string
	Commit string
}

func (h Head) String() string {
	commit := h.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	switch {
	case h.Branch == "" && commit == "":
		return "nothing"
	case h.Branch == "":
		return "detached HEAD at " + commit
	case commit == "":
		return h.Branch + " with no commits"
	default:
		return h.Branch + " at " + commit
	}
}

// WorktreeHead reads what the work tree holding path has checked out, outside a git repository it's empty
func WorktreeHead(path string) (Head, error) {
	top, err := worktreeTop(path)
	if err != nil || top == "" {
		return Head{}, err
	}
	return headOf(top), nil
}

// a detached HEAD has no branch and a new branch has no commit, neither is an error
func headOf(top string) Head {
	branch, _ := gitOutput(top, "symbolic-ref", "--quiet", "--short", "HEAD")
	commit, _ := gitOutput(top, "rev-parse", "--verify", "--quiet", "HEAD")
	return Head{Branch: branch, Commit: commit}
}

// empty when path isn't in a git repository
func worktreeTop(path string) (string, error) {
	dir, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	top, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil && strings.Contains(err.Error(), "not a git repository") {
		return "", nil
	}
	return top, err
}

// git's messages are matched on, so they're asked for in english
func gitOutput(dir string, args ...string) (string, error) {
	var out string
	err := withSafeDirectoryRetry(dir, logging.NewLogger(logging.INFO), func(extraArgs []string) error {
		cmd := exec.Command("git", slices.Concat(extraArgs, []string{"-C", dir}, args)...) //nolint:gosec // only ever git, and no shell is involved
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
