//go:build unix

package jobs

import (
	"os"
	"syscall"
)

// processAlive reports whether the process still holds a pid, including a
// terminated-but-unreaped child: kill(pid, 0) succeeds against a zombie
// until something calls wait on it.
func processAlive(process *os.Process) bool {
	return process.Signal(syscall.Signal(0)) == nil
}
