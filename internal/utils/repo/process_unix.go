//go:build unix

package repo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
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

// swapped in tests, which can't have the signal reach them
var raise = func(sig syscall.Signal) {
	_ = syscall.Kill(os.Getpid(), sig)
}

// git in a session of its own misses the ctrl c or kill meant for yago, so until release a signal cancels git,
// which kills its whole group, and release then hands the signal on to yago as it would have been
func cancelOnSignal(cancel context.CancelFunc) (release func()) {
	var watched []os.Signal
	for _, sig := range []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		if !signal.Ignored(sig) {
			watched = append(watched, sig)
		}
	}
	if len(watched) == 0 {
		return func() {}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, watched...)
	done, finished := make(chan struct{}), make(chan struct{})
	var caught os.Signal
	go func() {
		defer close(finished)
		select {
		case caught = <-signals:
			cancel()
		case <-done:
		}
	}()
	return func() {
		close(done)
		<-finished
		signal.Stop(signals)
		if sig, ok := caught.(syscall.Signal); ok {
			raise(sig)
		}
	}
}
