//go:build unix

package cli

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

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
