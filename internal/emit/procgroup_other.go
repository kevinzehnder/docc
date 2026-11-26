//go:build !unix

package emit

import "os/exec"

// killGroupOnCancel is a no-op where process groups are not available;
// cmd.WaitDelay still bounds the wait.
func killGroupOnCancel(*exec.Cmd) {}
