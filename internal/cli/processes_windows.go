//go:build windows

package cli

import "context"

// processes returns no parent processes. Windows has no Unix ps, and a chat
// with no harness parent is valid: the session is the name passed to --as.
func processes(_ context.Context) (map[int]process, error) {
	return map[int]process{}, nil
}
