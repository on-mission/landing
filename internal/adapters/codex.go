package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
)

const (
	defaultCodexModel           = "gpt-5.6-terra"
	defaultCodexReasoningEffort = codexReasoningEffortHigh
	defaultCodexSandbox         = codexSandboxDangerFullAccess
	codexCapacityCacheDuration  = time.Minute
)

type codexSandbox string

const (
	codexSandboxReadOnly         codexSandbox = "read-only"
	codexSandboxWorkspaceWrite   codexSandbox = "workspace-write"
	codexSandboxDangerFullAccess codexSandbox = "danger-full-access"
)

func parseCodexSandbox(value string) (codexSandbox, error) {
	sandbox := codexSandbox(value)
	switch sandbox {
	case codexSandboxReadOnly, codexSandboxWorkspaceWrite, codexSandboxDangerFullAccess:
		return sandbox, nil
	default:
		return "", fmt.Errorf("Invalid sandbox '%s'. Allowed: read-only, workspace-write, danger-full-access", value)
	}
}

type codexReasoningEffort string

const (
	codexReasoningEffortLow    codexReasoningEffort = "low"
	codexReasoningEffortMedium codexReasoningEffort = "medium"
	codexReasoningEffortHigh   codexReasoningEffort = "high"
)

func parseCodexReasoningEffort(value string) (codexReasoningEffort, error) {
	effort := codexReasoningEffort(value)
	switch effort {
	case codexReasoningEffortLow, codexReasoningEffortMedium, codexReasoningEffortHigh:
		return effort, nil
	default:
		return "", fmt.Errorf("Invalid reasoning_effort '%s'. Allowed: low, medium, high", value)
	}
}

type codexAdapter struct {
	outputRootOnce sync.Once
	outputRoot     string
	outputRootErr  error

	capacityMu        sync.Mutex
	cachedCapacity    harness.Capacity
	capacityExpiresAt time.Time
}

var _ harness.Adapter = (*codexAdapter)(nil)

func NewCodex() harness.Adapter {
	return &codexAdapter{}
}

func (adapter *codexAdapter) ID() string {
	return "codex"
}

func (adapter *codexAdapter) Models() []string {
	// Codex has no model-listing surface yet. This literal is a module-local
	// stand-in for one, so replacing it does not change configuration or routing.
	return []string{"gpt-5.6-terra", "gpt-5.3-codex-spark", "gpt-5.6-luna"}
}

func (adapter *codexAdapter) Detect(ctx context.Context) harness.Detection {
	path := executableIn(adapter.SpawnPath(), "codex")
	if path == "" {
		return absentDetection()
	}

	capacity, detail := adapter.readCodexCapacity(ctx)
	return capacityDetection(path, capacity, detail, codexAuthenticationFailure)
}

func (adapter *codexAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{Continuation: true, Sandbox: true}
}

func (adapter *codexAdapter) Validate(params harness.StartParams) error {
	if params.Sandbox != nil && *params.Sandbox != "" {
		if _, err := parseCodexSandbox(*params.Sandbox); err != nil {
			return err
		}
	}

	if params.ReasoningEffort != nil && *params.ReasoningEffort != "" {
		if _, err := parseCodexReasoningEffort(*params.ReasoningEffort); err != nil {
			return err
		}
	}

	return nil
}

func (adapter *codexAdapter) BuildStart(params harness.StartParams) (harness.Request, error) {
	outputFile, err := adapter.outputFileFor(params.JobID)
	if err != nil {
		return harness.Request{}, fmt.Errorf("create Codex output path: %w", err)
	}

	args := []string{"exec"}
	args = appendCodexCommonFlags(args, outputFile, params.Model, params.ReasoningEffort, params.ExtraConfig)
	args = append(args, "-s", codexSandboxFor(params.Sandbox))
	if params.CWD != "" {
		args = append(args, "-C", params.CWD)
	}
	args = append(args, promptWithPersona(params.Prompt, params.Persona))

	return harness.Request{Command: "codex", Args: args, PersonaDelivery: personaDelivery(params.Persona, harness.PersonaDeliveryPromptComposition)}, nil
}

func (adapter *codexAdapter) BuildResume(params harness.ResumeParams) (harness.Request, error) {
	if params.ThreadID == "" {
		return harness.Request{}, fmt.Errorf("thread_id is required for resume")
	}

	outputFile, err := adapter.outputFileFor(params.JobID)
	if err != nil {
		return harness.Request{}, fmt.Errorf("create Codex output path: %w", err)
	}

	args := []string{"exec", "resume"}
	args = appendCodexCommonFlags(args, outputFile, params.Model, params.ReasoningEffort, params.ExtraConfig)
	args = append(args, params.ThreadID, promptWithPersona(params.Prompt, params.Persona))

	return harness.Request{Command: "codex", Args: args, PersonaDelivery: personaDelivery(params.Persona, harness.PersonaDeliveryPromptComposition)}, nil
}

func appendCodexCommonFlags(args []string, outputFile string, model *string, reasoningEffort *string, extraConfig []string) []string {
	flags := append(
		args,
		"--model", codexModelFor(model),
		"-c", "reasoning_effort="+codexReasoningEffortFor(reasoningEffort),
		"--skip-git-repo-check",
		"--output-last-message", outputFile,
		"--json",
	)
	for _, value := range extraConfig {
		flags = append(flags, "-c", value)
	}
	return flags
}

func codexModelFor(model *string) string {
	if model == nil {
		return defaultCodexModel
	}

	return *model
}

func codexReasoningEffortFor(reasoningEffort *string) string {
	if reasoningEffort == nil {
		return string(defaultCodexReasoningEffort)
	}

	return *reasoningEffort
}

func codexSandboxFor(sandbox *string) string {
	if sandbox == nil {
		return string(defaultCodexSandbox)
	}

	return *sandbox
}

func (adapter *codexAdapter) OnStdoutLine(line string, record *harness.JobRecord) {
	if record == nil || record.ThreadID != nil || strings.TrimSpace(line) == "" {
		return
	}

	var event struct {
		Type     string  `json:"type"`
		ThreadID *string `json:"thread_id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &event); err != nil {
		return
	}
	if event.Type == "thread.started" && event.ThreadID != nil {
		record.ThreadID = event.ThreadID
	}
}

func (adapter *codexAdapter) Finalize(_ context.Context, params harness.FinalizeParams) (harness.Finalized, error) {
	output := (*string)(nil)
	if params.Record != nil {
		output = adapter.readOutputFile(params.Record.JobID)
	}
	failed := params.ExitCode != 0
	errorMessage := truncateOutput(params.Stderr)
	if failed && codexAuthenticationFailure(params.Stdout, params.Stderr) {
		errorMessage = authenticationError("Codex", "codex --login", params.Stdout, params.Stderr)
	}

	return harness.Finalized{
		Status:    codexFinalStatus(failed),
		Output:    output,
		Error:     errorMessage,
		Exhausted: failed && codexExhaustionMessage(params.Stderr),
	}, nil
}

func codexFinalStatus(failed bool) harness.JobStatus {
	if failed {
		return harness.JobStatusFailed
	}

	return harness.JobStatusDone
}

func truncateOutput(value string) string {
	if len(value) <= outputTailBytes {
		return value
	}

	return value[len(value)-outputTailBytes:]
}

func codexExhaustionMessage(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "you've hit your usage limit") ||
		strings.Contains(lower, "your workspace is out of credits") ||
		strings.Contains(lower, "you hit your spend cap")
}

func codexAuthenticationFailure(stdout, stderr string) bool {
	output := strings.ToLower(stdout + "\n" + stderr)
	return (strings.Contains(output, "not logged in") ||
		strings.Contains(output, "not authenticated") ||
		strings.Contains(output, "not signed in")) &&
		strings.Contains(output, "codex --login")
}

func (adapter *codexAdapter) ProbeCapacity(ctx context.Context) harness.Capacity {
	if capacity, ok := adapter.cachedCapacityFor(time.Now()); ok {
		return capacity
	}

	capacity, _ := adapter.readCodexCapacity(ctx)
	return adapter.cacheCapacity(capacity, time.Now())
}

func (adapter *codexAdapter) readCodexCapacity(ctx context.Context) (harness.Capacity, string) {
	result, err := jsonRPC(ctx, "codex", []string{"app-server"}, codexProbeEnvironment(adapter.SpawnPath()), []rpcStep{
		request(1, "initialize", map[string]any{
			"clientInfo": map[string]string{"name": "landing", "version": "1"},
		}),
		notification("initialized", map[string]string{}),
		request(2, "account/rateLimits/read", map[string]string{}),
	})
	if err != nil {
		return harness.UnknownCapacity(), err.Error()
	}

	return codexCapacityFromResult(result), ""
}

func codexProbeEnvironment(path string) []string {
	environment := os.Environ()
	updated := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, "PATH=") {
			updated = append(updated, entry)
		}
	}

	return jobs.WithCommsProbeEnvironment(append(updated, "PATH="+path))
}

func (adapter *codexAdapter) cachedCapacityFor(now time.Time) (harness.Capacity, bool) {
	adapter.capacityMu.Lock()
	defer adapter.capacityMu.Unlock()

	if adapter.capacityExpiresAt.After(now) {
		return adapter.cachedCapacity, true
	}

	return harness.UnknownCapacity(), false
}

func (adapter *codexAdapter) cacheCapacity(capacity harness.Capacity, now time.Time) harness.Capacity {
	adapter.capacityMu.Lock()
	defer adapter.capacityMu.Unlock()

	adapter.cachedCapacity = capacity
	adapter.capacityExpiresAt = now.Add(codexCapacityCacheDuration)
	return capacity
}

func codexCapacityFromResult(result json.RawMessage) harness.Capacity {
	var response struct {
		RateLimitsByLimitID map[string]json.RawMessage `json:"rateLimitsByLimitId"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return harness.UnknownCapacity()
	}

	buckets := make([]harness.Bucket, 0, len(response.RateLimitsByLimitID))
	for id, rawLimit := range response.RateLimitsByLimitID {
		var limit struct {
			Primary *struct {
				UsedPercent *float64 `json:"usedPercent"`
				ResetsAt    *float64 `json:"resetsAt"`
			} `json:"primary"`
		}
		if err := json.Unmarshal(rawLimit, &limit); err != nil || limit.Primary == nil ||
			limit.Primary.UsedPercent == nil || limit.Primary.ResetsAt == nil ||
			!isFinite(*limit.Primary.UsedPercent) || !isFinite(*limit.Primary.ResetsAt) {
			continue
		}

		buckets = append(buckets, harness.Bucket{
			ID:          id,
			UsedPercent: *limit.Primary.UsedPercent,
			ResetsAt:    time.UnixMilli(int64(*limit.Primary.ResetsAt * 1000)),
		})
	}

	if len(buckets) == 0 {
		return harness.UnknownCapacity()
	}

	return harness.KnownCapacity(buckets)
}

func isFinite(value float64) bool {
	return !math.IsInf(value, 0) && !math.IsNaN(value)
}

func (adapter *codexAdapter) SpawnPath() string {
	candidates := []string{"/usr/local/bin", "/opt/homebrew/bin"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append([]string{filepath.Join(home, ".local", "bin")}, candidates...)
	}

	return spawnPath(candidates)
}

func (adapter *codexAdapter) outputFileFor(jobID string) (string, error) {
	adapter.outputRootOnce.Do(func() {
		adapter.outputRoot, adapter.outputRootErr = os.MkdirTemp("", "codex-")
	})
	if adapter.outputRootErr != nil {
		return "", adapter.outputRootErr
	}

	return filepath.Join(adapter.outputRoot, jobID+".out"), nil
}

func (adapter *codexAdapter) readOutputFile(jobID string) *string {
	path, err := adapter.outputFileFor(jobID)
	if err != nil {
		return nil
	}

	output, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	value := string(output)
	return &value
}
