package adapters

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

const modelProbeTimeout = 120 * time.Second

type modelProbe struct {
	stdout   string
	stderr   string
	err      error
	timedOut bool
}

func runModelProbe(ctx context.Context, command string, args []string, environment []string) modelProbe {
	probeContext, cancel := context.WithTimeout(ctx, modelProbeTimeout)
	defer cancel()

	process := exec.CommandContext(probeContext, command, args...)
	if environment != nil {
		process.Env = environment
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	err := process.Run()

	return modelProbe{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		err:      err,
		timedOut: errors.Is(probeContext.Err(), context.DeadlineExceeded),
	}
}

func (probe modelProbe) output() string {
	return probe.stdout + "\n" + probe.stderr
}

func probeFailureEvidence(probe modelProbe) string {
	if probe.timedOut {
		return "model probe timed out"
	}
	if probe.err == nil {
		return "model probe completed without a reply"
	}
	if line := salientProbeLine(probe.output()); line != "" {
		return line
	}

	return "model probe failed without a definitive unknown-model rejection"
}

func rejectionEvidence(output string, recognizes func(string) bool) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && recognizes(trimmed) {
			return trimmed
		}
	}

	return "provider rejected the model"
}

func salientProbeLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if trimmed == "" {
			continue
		}
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "auth") || strings.Contains(lower, "login") || strings.Contains(lower, "network") || strings.Contains(lower, "connection") || strings.Contains(lower, "timeout") || strings.Contains(lower, "error") {
			return truncateProbeEvidence(trimmed)
		}
	}

	return ""
}

func truncateProbeEvidence(line string) string {
	const maxProbeEvidence = 400
	if len(line) <= maxProbeEvidence {
		return line
	}

	return line[:maxProbeEvidence] + "…"
}
