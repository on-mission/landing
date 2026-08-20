// Package paths resolves Landing's dispatch and private state locations.
package paths

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/on-mission/landing/internal/harness"
)

// Display renders path for a model-facing message without escaping its separators.
func Display(path string) string {
	return `"` + path + `"`
}

// ResolveDispatchCwd resolves where a dispatch runs. An empty suppliedCWD means
// the invocation directory; a relative one resolves against it; an absolute one
// is used as given. Nothing is inferred from an ancestor directory.
func ResolveDispatchCwd(ctx context.Context, invocationDir string, suppliedCWD string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	cwd := suppliedCWD
	if cwd == "" {
		cwd = invocationDir
	}
	target := cwd
	if !filepath.IsAbs(cwd) {
		target = filepath.Join(invocationDir, cwd)
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", harness.WrapError(
			harness.ErrorCodeInvalidCWD,
			fmt.Sprintf("cwd %s does not exist or cannot be resolved.", Display(cwd)),
			nil,
			err,
		)
	}

	return resolved, nil
}

func StateRoot() (string, error) {
	if stateRoot, ok := os.LookupEnv("LANDING_STATE_DIR"); ok {
		return stateRoot, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".landing"), nil
}

func JobsDir() (string, error) {
	stateRoot, err := StateRoot()
	if err != nil {
		return "", err
	}

	return filepath.Join(stateRoot, "jobs"), nil
}

func EnsureDirs(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	directory, err := JobsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create jobs directory %s: %w", Display(directory), err)
	}

	return directory, nil
}
