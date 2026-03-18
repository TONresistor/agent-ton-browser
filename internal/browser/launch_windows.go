//go:build windows

package browser

import (
	"os"
	"os/exec"
)

func setSysProcAttr(_ *exec.Cmd) {
	// No-op on Windows: process detachment is not required.
}

func killProcess(proc *os.Process) error {
	return proc.Kill()
}
