//go:build !unix

package browser

import "os/exec"

func IsolateProcess(cmd *exec.Cmd) {}
