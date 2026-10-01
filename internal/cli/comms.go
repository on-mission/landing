package cli

import (
	"context"
	"os"
)

// process is one row from the process table. command is the executable
// string a caller uses to recognize an ancestor.
type process struct {
	pid     int
	parent  int
	command string
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

// ancestorProcesses walks from this process's parent toward init. The first
// entry is the closest ancestor. Messaging uses the walk to find a harness.
func ancestorProcesses(ctx context.Context) ([]process, error) {
	entries, err := processes(ctx)
	if err != nil {
		return nil, err
	}
	chain := make([]process, 0)
	seen := map[int]struct{}{}
	current := os.Getpid()
	for {
		if _, ok := seen[current]; ok {
			break
		}
		seen[current] = struct{}{}
		entry, ok := entries[current]
		if !ok || entry.parent <= 0 || entry.parent == current {
			break
		}
		parent, ok := entries[entry.parent]
		if !ok {
			break
		}
		chain = append(chain, parent)
		current = parent.pid
	}

	return chain, nil
}
