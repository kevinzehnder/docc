//go:build unix

package emit

import (
	"os/exec"
	"syscall"
)

// killGroupOnCancel runs cmd in its own process group and, when its context
// ends, kills the whole group. `soffice` is a wrapper script that starts
// soffice.bin as a child; killing only the wrapper leaves the child running
// and holding the output pipe open.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
