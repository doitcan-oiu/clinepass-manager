//go:build unix

package browser

import (
	"os/exec"
	"syscall"
)

func IsolateProcess(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
