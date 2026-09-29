package adapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
)

const (
	claudePermissionMode        = "bypassPermissions"
	claudeCapacityCacheDuration = time.Minute
	claudeCapacityProbeTimeout  = 10 * time.Second
)

type claudeAdapter struct {
	capacityMu        sync.Mutex
	cachedCapacity    harness.Capacity
	capacityExpiresAt time.Time
	capacityInFlight  chan struct{}
}

type claudeResult struct {
	IsError           *bool
	IsErrorRaw        json.RawMessage
	Result            *string
	ResultRaw         json.RawMessage
	SessionID         *string
	PermissionDenials json.RawMessage
}

var _ harness.Adapter = (*claudeAdapter)(nil)

func NewClaude() harness.Adapter {
	return &claudeAdapter{}
}

func (adapter *claudeAdapter) ID() string {
	return "claude"
}

func (adapter *claudeAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{
		Models:    []string{"claude-opus-5-5", "claude-opus-5", "claude-sonnet-5", "claude-haiku-4-5-20251001"},
		Aliases:   map[string]string{"opus-5.5": "claude-opus-5-5"},
		Latest:    "claude-opus-5-5",
		Authority: harness.ModelCatalogAdvisory,
	}
}

func (adapter *claudeAdapter) ValidateModel(ctx context.Context, model string) harness.ModelValidation {
	probe := runModelProbe(ctx, "claude", []string{
		"-p", "Say hi",
		"--output-format", "json",
		"--model", model,
		"--permission-mode", "plan",
	}, nil)
	if claudeUnknownModel(probe.output()) {
		return harness.InvalidModel(claudeRejectionEvidence(probe))
	}
	result := parseClaudeResult(probe.stdout)
	if probe.err == nil && result != nil && result.IsError != nil && !*result.IsError && result.Result != nil && *result.Result != "" {
		return harness.ValidModel("model probe returned a reply")
	}

	return harness.UnverifiedModel(probeFailureEvidence(probe))
}

func claudeRejectionEvidence(probe modelProbe) string {
	result := parseClaudeResult(probe.stdout)
	if result != nil && result.Result != nil && *result.Result != "" {
		return *result.Result
	}

	return rejectionEvidence(probe.output(), claudeUnknownModel)
}

func claudeUnknownModel(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "[claude-code:unrecognized_model]") ||
		strings.Contains(lower, "there's an issue with the selected model")
}

func (adapter *claudeAdapter) Detect(ctx context.Context) harness.Detection {
	path := executableIn(adapter.SpawnPath(), "claude")
	if path == "" {
		return absentDetection()
	}

	capacity, err := requestClaudeUsage(ctx)
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return capacityDetection(path, capacity, detail, claudeAuthenticationFailure)
}

func (adapter *claudeAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{Continuation: true, Sandbox: false}
}

func (adapter *claudeAdapter) SpawnPath() string {
	return spawnPath(nil)
}

func (adapter *claudeAdapter) Validate(params harness.StartParams) error {
	if params.Model == nil || *params.Model == "" {
		return fmt.Errorf("model is required for claude")
	}

	return nil
}

// A dispatched job runs unattended: there is no human on the other end to
// approve a tool-permission prompt. Without this mode, Claude asks for
// approval, declines the work when nobody responds, and still exits 0.
//
// The Codex adapter uses danger-full-access, so leaving Claude gated made the
// same brief succeed or fail based on routing. bypassPermissions is the only
// Claude permission mode that actually runs tools; dontAsk auto-denies them.
func (adapter *claudeAdapter) BuildStart(params harness.StartParams) (harness.Request, error) {
	model, err := claudeModel(params.Model)
	if err != nil {
		return harness.Request{}, err
	}

	args := []string{
		"-p", params.Prompt,
		"--output-format", "json",
		"--model", model,
		"--permission-mode", claudePermissionMode,
	}
	if params.Persona != nil {
		args = append(args, "--append-system-prompt", promptWithPersona("", params.Persona))
	}

	return harness.Request{Command: "claude", Args: args, PersonaDelivery: personaDelivery(params.Persona, harness.PersonaDeliveryAppendSystemPrompt)}, nil
}

func (adapter *claudeAdapter) BuildResume(params harness.ResumeParams) (harness.Request, error) {
	if params.ThreadID == "" {
		return harness.Request{}, fmt.Errorf("thread_id is required for resume")
	}
	model, err := claudeModel(params.Model)
	if err != nil {
		return harness.Request{}, err
	}

	args := []string{
		"-p", params.Prompt,
		"--output-format", "json",
		"--model", model,
		"--permission-mode", claudePermissionMode,
	}
	if params.Persona != nil {
		args = append(args, "--append-system-prompt", promptWithPersona("", params.Persona))
	}
	args = append(args, "--resume", params.ThreadID)

	return harness.Request{Command: "claude", Args: args, PersonaDelivery: personaDelivery(params.Persona, harness.PersonaDeliveryAppendSystemPrompt)}, nil
}

func claudeModel(model *string) (string, error) {
	if model == nil || *model == "" {
		return "", fmt.Errorf("model is required for claude")
	}

	return *model, nil
}

func (adapter *claudeAdapter) OnStdoutLine(_ string, _ *harness.JobRecord) {}

func (adapter *claudeAdapter) Finalize(_ context.Context, params harness.FinalizeParams) (harness.Finalized, error) {
	result := parseClaudeResult(params.Stdout)
	exhausted := params.ExitCode != 0 && claudeExhaustionMessage(params.Stdout+"\n"+params.Stderr)
	if result == nil {
		errorMessage := claudeMalformedOutputError(params.Stdout, params.Stderr)
		if claudeAuthenticationFailure(params.Stdout, params.Stderr) {
			errorMessage = authenticationError("Claude", "claude", params.Stdout, params.Stderr)
		}
		return harness.Finalized{
			Status:    harness.JobStatusFailed,
			Error:     errorMessage,
			Exhausted: exhausted,
		}, nil
	}

	denied := claudeDeniedTools(result.PermissionDenials)
	status := claudeStatus(params.ExitCode, result.IsError, denied)
	if status == harness.JobStatusDone {
		return harness.Finalized{
			Status:   status,
			Output:   result.Result,
			Error:    truncateOutput(params.Stderr),
			ThreadID: result.SessionID,
		}, nil
	}

	errorMessage := claudeFailureError(params.ExitCode, result.IsErrorRaw, denied, params.Stdout, params.Stderr)
	return harness.Finalized{
		Status:    status,
		Output:    result.Result,
		Error:     errorMessage,
		Exhausted: exhausted || claudeExhaustionMessage(claudeResultText(result.ResultRaw)),
		ThreadID:  result.SessionID,
	}, nil
}

func parseClaudeResult(stdout string) *claudeResult {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &object); err != nil || object == nil {
		return nil
	}

	result := &claudeResult{
		IsErrorRaw:        object["is_error"],
		ResultRaw:         object["result"],
		PermissionDenials: object["permission_denials"],
	}
	if raw, ok := object["is_error"]; ok {
		if err := json.Unmarshal(raw, &result.IsError); err != nil {
			result.IsError = nil
		}
	}
	if raw, ok := object["result"]; ok {
		if err := json.Unmarshal(raw, &result.Result); err != nil {
			result.Result = nil
		}
	}
	if raw, ok := object["session_id"]; ok {
		if err := json.Unmarshal(raw, &result.SessionID); err != nil {
			result.SessionID = nil
		}
	}

	return result
}

func claudeStatus(exitCode int, isError *bool, denied []string) harness.JobStatus {
	if exitCode == 0 && isError != nil && !*isError && len(denied) == 0 {
		return harness.JobStatusDone
	}

	return harness.JobStatusFailed
}

// permission_denials is the canary for bypassPermissions silently ceasing to
// work. A managed policy can block tools while Claude exits 0 with is_error
// false, which otherwise banks an unattended job as successful work.
func claudeDeniedTools(raw json.RawMessage) []string {
	var denials []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &denials) != nil || denials == nil {
		return nil
	}

	tools := make([]string, 0, len(denials))
	for _, denial := range denials {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(denial, &object); err != nil || object == nil {
			tools = append(tools, "unknown tool")
			continue
		}
		var toolName string
		if rawName, ok := object["tool_name"]; !ok || json.Unmarshal(rawName, &toolName) != nil || toolName == "" {
			tools = append(tools, "unknown tool")
			continue
		}
		tools = append(tools, toolName)
	}

	return tools
}

func claudeMalformedOutputError(stdout, stderr string) string {
	return strings.Join([]string{
		"Claude stdout did not contain the expected JSON result object.",
		"stdout tail: " + tail(stdout),
		"stderr tail: " + tail(stderr),
	}, "\n")
}

func claudeFailureError(exitCode int, isErrorRaw json.RawMessage, denied []string, stdout, stderr string) string {
	if len(denied) > 0 {
		return strings.Join([]string{
			"Claude was blocked from doing the work: tool use was denied by permission policy.",
			"The harness did not crash; it ended its turn without doing the work because a tool it invoked was not permitted.",
			"requested permission mode: --permission-mode " + claudePermissionMode,
			"denied tools: " + strings.Join(denied, ", "),
			fmt.Sprintf("exit code: %d", exitCode),
		}, "\n")
	}
	if claudeAuthenticationFailure(stdout, stderr) {
		return authenticationError("Claude", "claude", stdout, stderr)
	}

	return strings.Join([]string{
		"Claude did not complete successfully.",
		fmt.Sprintf("exit code: %d", exitCode),
		"is_error: " + claudeIsErrorText(isErrorRaw),
		"stderr tail: " + tail(stderr),
	}, "\n")
}

func claudeIsErrorText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "undefined"
	}

	return string(raw)
}

func claudeResultText(raw json.RawMessage) string {
	var result string
	if len(raw) == 0 || json.Unmarshal(raw, &result) != nil {
		return string(raw)
	}

	return result
}

func claudeExhaustionMessage(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "usage limit") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "out of credits") ||
		strings.Contains(lower, "spend cap")
}

func claudeAuthenticationFailure(stdout, stderr string) bool {
	output := strings.ToLower(stdout + "\n" + stderr)
	return (strings.Contains(output, "not logged in") ||
		strings.Contains(output, "not authenticated") ||
		strings.Contains(output, "not signed in")) &&
		(strings.Contains(output, "claude login") || strings.Contains(output, "/login"))
}

func (adapter *claudeAdapter) ProbeCapacity(ctx context.Context) harness.Capacity {
	capacity, inFlight := adapter.beginCapacityProbe(time.Now())
	if capacity != nil {
		return *capacity
	}
	if inFlight != nil {
		select {
		case <-inFlight:
			cached, ok := adapter.cachedCapacityFor(time.Now())
			if ok {
				return cached
			}
			return harness.UnknownCapacity()
		case <-ctx.Done():
			return harness.UnknownCapacity()
		}
	}

	value, err := requestClaudeUsage(ctx)
	if err != nil {
		value = harness.UnknownCapacity()
	}
	return adapter.completeCapacityProbe(value, time.Now())
}

func (adapter *claudeAdapter) beginCapacityProbe(now time.Time) (*harness.Capacity, chan struct{}) {
	adapter.capacityMu.Lock()
	defer adapter.capacityMu.Unlock()

	if adapter.capacityExpiresAt.After(now) {
		capacity := adapter.cachedCapacity
		return &capacity, nil
	}
	if adapter.capacityInFlight != nil {
		return nil, adapter.capacityInFlight
	}

	adapter.capacityInFlight = make(chan struct{})
	return nil, nil
}

func (adapter *claudeAdapter) cachedCapacityFor(now time.Time) (harness.Capacity, bool) {
	adapter.capacityMu.Lock()
	defer adapter.capacityMu.Unlock()

	if adapter.capacityExpiresAt.After(now) {
		return adapter.cachedCapacity, true
	}
	return harness.UnknownCapacity(), false
}

func (adapter *claudeAdapter) completeCapacityProbe(capacity harness.Capacity, now time.Time) harness.Capacity {
	adapter.capacityMu.Lock()
	defer adapter.capacityMu.Unlock()

	adapter.cachedCapacity = capacity
	adapter.capacityExpiresAt = now.Add(claudeCapacityCacheDuration)
	close(adapter.capacityInFlight)
	adapter.capacityInFlight = nil
	return capacity
}

func requestClaudeUsage(ctx context.Context) (harness.Capacity, error) {
	ctx, cancel := context.WithTimeout(ctx, claudeCapacityProbeTimeout)
	defer cancel()

	process := exec.CommandContext(ctx, "claude", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "-p")
	process.Env = jobs.WithCommsProbeEnvironment(os.Environ())
	stdin, err := process.StdinPipe()
	if err != nil {
		return harness.UnknownCapacity(), fmt.Errorf("open Claude capacity stdin: %w", err)
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return harness.UnknownCapacity(), fmt.Errorf("open Claude capacity stdout: %w", err)
	}
	var stderr bytes.Buffer
	process.Stderr = &stderr
	if err := process.Start(); err != nil {
		return harness.UnknownCapacity(), fmt.Errorf("start Claude capacity probe: %w", err)
	}
	defer func() {
		// The probe response is sufficient; these cleanup failures cannot alter it.
		_ = stdin.Close()
		cancel()
		_ = process.Wait()
	}()

	request, err := json.Marshal(struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Request   struct {
			Subtype string `json:"subtype"`
		} `json:"request"`
	}{
		Type:      "control_request",
		RequestID: "r1",
		Request: struct {
			Subtype string `json:"subtype"`
		}{Subtype: "get_usage"},
	})
	if err != nil {
		return harness.UnknownCapacity(), fmt.Errorf("encode Claude capacity request: %w", err)
	}
	if _, err := stdin.Write(append(request, '\n')); err != nil {
		return harness.UnknownCapacity(), fmt.Errorf("write Claude capacity request: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var observed strings.Builder
	for scanner.Scan() {
		line := scanner.Bytes()
		if observed.Len() > 0 {
			observed.WriteByte('\n')
		}
		observed.Write(line)
		capacity, found := claudeCapacityFromLine(line)
		if found {
			return capacity, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return harness.UnknownCapacity(), fmt.Errorf("read Claude capacity response: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return harness.UnknownCapacity(), err
	}

	detail := strings.TrimSpace(observed.String() + "\n" + stderr.String())
	if detail == "" {
		return harness.UnknownCapacity(), fmt.Errorf("Claude exited before its capacity response")
	}

	return harness.UnknownCapacity(), fmt.Errorf("Claude capacity response: %s", detail)
}

func claudeCapacityFromLine(line []byte) (harness.Capacity, bool) {
	var message struct {
		Type     string          `json:"type"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(line, &message); err != nil || message.Type != "control_response" || len(message.Response) == 0 {
		return harness.UnknownCapacity(), false
	}

	var controlResponse struct {
		RequestID string          `json:"request_id"`
		Response  json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(message.Response, &controlResponse); err != nil || controlResponse.RequestID != "r1" {
		return harness.UnknownCapacity(), false
	}

	response := controlResponse.Response
	if len(response) == 0 || string(response) == "null" {
		response = message.Response
	}
	return claudeCapacityFromResponse(response), true
}

func claudeCapacityFromResponse(response json.RawMessage) harness.Capacity {
	var payload struct {
		RateLimits map[string]json.RawMessage `json:"rate_limits"`
	}
	if err := json.Unmarshal(response, &payload); err != nil {
		return harness.UnknownCapacity()
	}

	buckets := make([]harness.Bucket, 0, len(payload.RateLimits))
	for id, rawLimit := range payload.RateLimits {
		var limit struct {
			Utilization *float64 `json:"utilization"`
			ResetsAt    *string  `json:"resets_at"`
		}
		if err := json.Unmarshal(rawLimit, &limit); err != nil || limit.Utilization == nil || limit.ResetsAt == nil ||
			!isFinite(*limit.Utilization) {
			continue
		}
		resetsAt, err := time.Parse(time.RFC3339, *limit.ResetsAt)
		if err != nil {
			continue
		}

		buckets = append(buckets, harness.Bucket{ID: id, UsedPercent: *limit.Utilization, ResetsAt: resetsAt})
	}

	if len(buckets) == 0 {
		return harness.UnknownCapacity()
	}
	return harness.KnownCapacity(buckets)
}
