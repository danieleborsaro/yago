//go:build !unix

package repo

import (
	"context"
	"os/exec"
)

// windows has no process groups to kill, the timeout and WaitDelay still stop yago waiting
func isolateProcessGroup(*exec.Cmd) {}

// git shares the console with yago there, so ctrl c reaches it anyway
func cancelOnSignal(context.CancelFunc) (release func()) {
	return func() {}
}
