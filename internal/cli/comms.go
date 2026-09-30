package cli

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// process is one row from the process table. command is the ps command
// column, which is how a caller recognizes an ancestor executable.
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
// entry is the closest ancestor. The walk is the same process table read the
// comms commands used; messaging uses it to find a harness.
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

func processes(ctx context.Context) (map[int]process, error) {
	command := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,command=")
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	entries := make(map[int]process)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		parent, parentErr := strconv.Atoi(fields[1])
		if pidErr != nil || parentErr != nil {
			continue
		}
		entries[pid] = process{pid: pid, parent: parent, command: strings.Join(fields[2:], " ")}
	}

	return entries, nil
}
