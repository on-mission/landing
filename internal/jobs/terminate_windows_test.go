//go:build windows

package jobs

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
)

func TestNormalizeTerminationErrorTreatsAccessDeniedAsCompleted(t *testing.T) {
	err := fmt.Errorf("TerminateProcess: %w", syscall.ERROR_ACCESS_DENIED)
	if got := normalizeTerminationError(err); got != nil {
		t.Fatalf("normalizeTerminationError(%v) = %v, want nil", err, got)
	}

	other := errors.New("access check failed")
	if got := normalizeTerminationError(other); !errors.Is(got, other) {
		t.Fatalf("normalizeTerminationError(%v) = %v, want original error", other, got)
	}
}
