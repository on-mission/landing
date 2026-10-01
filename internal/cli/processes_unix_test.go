//go:build unix

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessesListsThisProcess(t *testing.T) {
	entries, err := processes(context.Background())
	if err != nil {
		t.Fatalf("processes() returned unexpected error: %v", err)
	}
	pid := os.Getpid()
	if _, ok := entries[pid]; !ok {
		t.Fatalf("processes() omitted pid %d", pid)
	}
}

func TestProcessesReportsPSFailure(t *testing.T) {
	// Bug class: a unix ps failure is treated as no parent, and messages continue.
	dir := t.TempDir()
	script := filepath.Join(dir, "ps")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(ps) returned unexpected error: %v", err)
	}
	t.Setenv("PATH", dir)
	_, err := processes(context.Background())
	if err == nil {
		t.Fatal("processes() succeeded; want the ps failure")
	}
	_, found, ancestorErr := closestHarnessAncestor(context.Background())
	if found || ancestorErr == nil {
		t.Fatalf("closestHarnessAncestor() found %t, err %v; want a wrapped ps failure", found, ancestorErr)
	}
	if !strings.Contains(ancestorErr.Error(), "find harness parent") {
		t.Fatalf("closestHarnessAncestor() error = %v, want find harness parent wrap", ancestorErr)
	}
}
