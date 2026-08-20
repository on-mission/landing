//go:build unix

package comms

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/on-mission/landing/internal/paths"
)

type stateLock struct {
	file *os.File
}

func acquireStateLock(ctx context.Context, path string, exclusive bool) (*stateLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open comms lock %s: %w", paths.Display(path), err)
	}
	operation := syscall.LOCK_SH
	if exclusive {
		operation = syscall.LOCK_EX
	}
	for {
		if err := ctx.Err(); err != nil {
			closeErr := file.Close()
			return nil, errors.Join(err, closeErr)
		}
		err := syscall.Flock(int(file.Fd()), operation|syscall.LOCK_NB)
		if err == nil {
			return &stateLock{file: file}, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			closeErr := file.Close()
			return nil, errors.Join(fmt.Errorf("acquire comms lock %s: %w", paths.Display(path), err), closeErr)
		}
		if err := waitForLock(ctx); err != nil {
			closeErr := file.Close()
			return nil, errors.Join(err, closeErr)
		}
	}
}

func (lock *stateLock) Close() error {
	unlockErr := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
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
