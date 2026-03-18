//go:build windows

package daemon

import "os"

func isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds. Signal(nil) checks handle validity.
	return proc.Signal(os.Signal(nil)) == nil
}
