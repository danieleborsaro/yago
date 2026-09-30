//go:build unix

package repo

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// a session of its own so a timeout kills the helpers git started too, a killed git used to leave ssh or
// git-remote-https running and holding its output
func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
