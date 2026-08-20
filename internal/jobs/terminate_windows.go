//go:build windows

package jobs

import "os"

// terminateProcess ends the process. Windows has no equivalent of SIGTERM
// that a harness can catch and shut down cleanly on, so this calls
// TerminateProcess (via Process.Kill) for an immediate, unconditional stop.
// A harness is given no chance to flush output or exit gracefully; the
// scope matches Unix's SIGTERM in that only this process is targeted, not
// any children it spawned.
func terminateProcess(process *os.Process) error {
	return process.Kill()
}
