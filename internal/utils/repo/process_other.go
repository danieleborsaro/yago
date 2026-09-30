//go:build !unix

package repo

import "os/exec"

// windows has no sessions to detach into, the timeout and WaitDelay still stop yago waiting
func detachFromTerminal(*exec.Cmd) {}
