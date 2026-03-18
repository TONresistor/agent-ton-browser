//go:build windows

package daemon

import "os/exec"

func setDaemonSysProcAttr(_ *exec.Cmd) {
	// No-op on Windows: process detachment is not required.
}
