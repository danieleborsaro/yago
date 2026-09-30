//go:build !unix

package repo

import "os/exec"

// windows has no process groups to kill, the timeout and WaitDelay still stop yago waiting
func isolateProcessGroup(*exec.Cmd) {}
