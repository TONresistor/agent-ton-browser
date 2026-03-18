//go:build windows

package config

import (
	"os"
	"syscall"
)

func isProcessAlive(proc *os.Process) bool {
	handle := proc.Handle
	var code uint32
	err := syscall.GetExitCodeProcess(syscall.Handle(handle), &code)
	if err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}
