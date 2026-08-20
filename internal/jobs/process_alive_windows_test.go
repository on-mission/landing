//go:build windows

package jobs

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

// processAlive reports whether the process is still running. Windows has no
// signal(0)-style liveness probe and no zombie state to wait past, so this
// opens the process by pid and reads its exit code directly.
func processAlive(process *os.Process) bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	handle, _, _ := kernel32.NewProc("OpenProcess").Call(
		processQueryLimitedInformation, 0, uintptr(process.Pid),
	)
	if handle == 0 {
		return false
	}
	defer kernel32.NewProc("CloseHandle").Call(handle)

	var exitCode uint32
	queried, _, _ := kernel32.NewProc("GetExitCodeProcess").Call(handle, uintptr(unsafe.Pointer(&exitCode)))
	if queried == 0 {
		return false
	}

	return exitCode == stillActive
}
