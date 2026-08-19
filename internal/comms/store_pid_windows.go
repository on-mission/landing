//go:build windows

package comms

import (
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	synchronize                    = 0x00100000
	stillActive                    = 259
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	handle, _, openErr := kernel32.NewProc("OpenProcess").Call(
		processQueryLimitedInformation|synchronize, 0, uintptr(pid),
	)
	if handle == 0 {
		return openErr != syscall.Errno(87)
	}
	var exitCode uint32
	queried, _, queryErr := kernel32.NewProc("GetExitCodeProcess").Call(handle, uintptr(unsafe.Pointer(&exitCode)))
	closed, _, closeErr := kernel32.NewProc("CloseHandle").Call(handle)
	if queried == 0 || queryErr != nil || closed == 0 || closeErr != nil {
		return true
	}

	return exitCode == stillActive
}
