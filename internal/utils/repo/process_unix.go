//go:build unix

package repo

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// with no terminal a prompt from git or ssh fails straight away, and a timeout kills the helpers git started too,
// otherwise a killed git leaves git-remote-https or ssh reading the terminal with echo off
func detachFromTerminal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
