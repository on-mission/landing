//go:build windows

package comms

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	lockViolation           = syscall.Errno(33)
)

type stateLock struct {
	file       *os.File
	overlapped syscall.Overlapped
}

func acquireStateLock(ctx context.Context, path string, exclusive bool) (*stateLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open comms lock %q: %w", path, err)
	}
	flags := uintptr(lockfileFailImmediately)
	if exclusive {
		flags |= lockfileExclusiveLock
	}
	lock := &stateLock{file: file}
	for {
		if err := ctx.Err(); err != nil {
			closeErr := file.Close()
			return nil, errors.Join(err, closeErr)
		}
		locked, _, lockErr := syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx").Call(
			file.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&lock.overlapped)),
		)
		if locked != 0 {
			return lock, nil
		}
		if errno, ok := lockErr.(syscall.Errno); !ok || errno != lockViolation {
			closeErr := file.Close()
			return nil, errors.Join(fmt.Errorf("acquire comms lock %q: %w", path, lockErr), closeErr)
		}
		if err := waitForLock(ctx); err != nil {
			closeErr := file.Close()
			return nil, errors.Join(err, closeErr)
		}
	}
}

func (lock *stateLock) Close() error {
	unlocked, _, unlockErr := syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx").Call(
		lock.file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&lock.overlapped)),
	)
	if unlocked != 0 {
		unlockErr = nil
	}
	closeErr := lock.file.Close()
	return errors.Join(unlockErr, closeErr)
}

func waitForLock(ctx context.Context) error {
	timer := time.NewTimer(pollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
