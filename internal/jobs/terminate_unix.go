//go:build unix

package jobs

import (
	"os"
	"syscall"
)

// terminateProcess asks the process to shut down. Unix delivers SIGTERM,
// which a well-behaved harness can catch to flush state before exiting.
func terminateProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
