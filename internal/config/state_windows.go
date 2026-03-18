//go:build windows

package config

import "os"

// isProcessAlive checks if a process is still running on Windows.
// os.FindProcess always succeeds on Windows, so we attempt Signal(nil)
// which returns nil only if the process handle is valid and alive.
func isProcessAlive(proc *os.Process) bool {
	err := proc.Signal(os.Signal(nil))
	return err == nil
}
