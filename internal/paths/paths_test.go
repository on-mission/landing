package paths

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/on-mission/landing/internal/harness"
)

func TestResolveDispatchCwd(t *testing.T) {
	invocationDir := filepath.Join(t.TempDir(), "project", "invocation")
	relativeDir := filepath.Join(invocationDir, "relative")
	absoluteDir := t.TempDir()
	if err := os.MkdirAll(relativeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		suppliedCWD   string
		want          string
		wantErrorCode harness.ErrorCode
		wantErrorPath string
	}{
		{name: "defaults to the invocation directory", want: invocationDir},
		{name: "resolves a relative path from the invocation directory", suppliedCWD: "relative", want: relativeDir},
		{name: "uses an absolute path as given", suppliedCWD: absoluteDir, want: absoluteDir},
		{name: "reports a missing supplied path", suppliedCWD: "missing", wantErrorCode: harness.ErrorCodeInvalidCWD, wantErrorPath: "missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveDispatchCwd(context.Background(), invocationDir, test.suppliedCWD)
			if test.wantErrorCode != "" {
				var typed *harness.Error
				if !errors.As(err, &typed) || typed.Code != test.wantErrorCode {
					t.Fatalf("ResolveDispatchCwd() error = %T %v, want %s", err, err, test.wantErrorCode)
				}
				if !strings.Contains(err.Error(), test.wantErrorPath) {
					t.Fatalf("ResolveDispatchCwd() error = %q, want path %q", err, test.wantErrorPath)
				}
				return
			}
			want, resolveErr := filepath.EvalSymlinks(test.want)
			if resolveErr != nil {
				t.Fatal(resolveErr)
			}
			if err != nil || got != want {
				t.Fatalf("ResolveDispatchCwd() = %q, %v; want %q, nil", got, err, want)
			}
		})
	}
}

func TestStateRootHonorsLandingStateDir(t *testing.T) {
	want := filepath.Join(t.TempDir(), "state")
	t.Setenv("LANDING_STATE_DIR", want)
	got, err := StateRoot()
	if err != nil || got != want {
		t.Fatalf("StateRoot() = %q, %v; want %q, nil", got, err, want)
	}
}

func TestDisplayPreservesWindowsSeparators(t *testing.T) {
	path := `C:\Users\Landing Model\PERSONA.md`
	if got, want := Display(path), `"C:\Users\Landing Model\PERSONA.md"`; got != want {
		t.Fatalf("Display(%q) = %q, want %q", path, got, want)
	}
}
