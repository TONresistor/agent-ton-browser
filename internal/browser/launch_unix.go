//go:build !windows

package browser

import (
	"os"
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
