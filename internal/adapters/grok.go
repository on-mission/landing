package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
)

const (
	grokCapacityCacheTTL = time.Minute
	grokModelsCacheTTL   = time.Minute
	grokModelsTimeout    = 10 * time.Second
	grokProtocolVersion  = "2025-06-18"
)

type grokModelsReader func(context.Context, string) ([]byte, error)

type grokAdapter struct {
	mutex              sync.Mutex
	command            string
	resolutionErr      error
	capacity           harness.Capacity
	capacityExpiresAt  time.Time
	capacityCached     bool
	capacityInProgress chan struct{}
	models             []string
	modelsExpiresAt    time.Time
	modelsCached       bool
	modelsInProgress   chan struct{}
	readModels         grokModelsReader
}

type grokResult struct {
	Text       *string `json:"text"`
	StopReason *string `json:"stopReason"`
	SessionID  *string `json:"sessionId"`
}

type grokBilling struct {
	Config struct {
		CreditUsagePercent json.RawMessage `json:"creditUsagePercent"`
		CurrentPeriod      struct {
			End *string `json:"end"`
		} `json:"currentPeriod"`
		OnDemandCap struct {
			Value json.RawMessage `json:"val"`
		} `json:"onDemandCap"`
		OnDemandUsed struct {
			Value json.RawMessage `json:"val"`
		} `json:"onDemandUsed"`
		IsUnifiedBillingUser *bool   `json:"isUnifiedBillingUser"`
		BillingPeriodEnd     *string `json:"billingPeriodEnd"`
	} `json:"config"`
}

var _ harness.Adapter = (*grokAdapter)(nil)

func NewGrok() harness.Adapter {
	return &grokAdapter{}
}

func (adapter *grokAdapter) ID() string {
	return "grok"
}

func (adapter *grokAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{
		Models:    adapter.catalogModels(),
		Authority: harness.ModelCatalogAuthoritative,
	}
}

func (adapter *grokAdapter) catalogModels() []string {
	ctx, cancel := context.WithTimeout(context.Background(), grokModelsTimeout)
	defer cancel()

	adapter.mutex.Lock()
	if adapter.modelsCached && adapter.modelsExpiresAt.After(time.Now()) {
		models := append([]string(nil), adapter.models...)
		adapter.mutex.Unlock()
		return models
	}
	if adapter.modelsInProgress != nil {
		inProgress := adapter.modelsInProgress
		adapter.mutex.Unlock()
		select {
		case <-inProgress:
			adapter.mutex.Lock()
			models := append([]string(nil), adapter.models...)
			adapter.mutex.Unlock()
			return models
		case <-ctx.Done():
			return grokFallbackModels()
		}
	}

	inProgress := make(chan struct{})
	adapter.modelsInProgress = inProgress
	adapter.mutex.Unlock()

	models, err := adapter.listGrokModels(ctx)
	if err != nil {
		models = grokFallbackModels()
	}
	adapter.mutex.Lock()
	adapter.models = append([]string(nil), models...)
	adapter.modelsCached = true
	adapter.modelsExpiresAt = time.Now().Add(grokModelsCacheTTL)
	adapter.modelsInProgress = nil
	close(inProgress)
	adapter.mutex.Unlock()

	return append([]string(nil), models...)
}

func grokFallbackModels() []string {
	return []string{"grok-4.6", "grok-4.5"}
}

func (adapter *grokAdapter) listGrokModels(ctx context.Context) ([]string, error) {
	command, err := adapter.resolveGrokCommand()
	if err != nil {
		return nil, err
	}

	reader := adapter.readModels
	if reader == nil {
		reader = runGrokModels
	}
	output, err := reader(ctx, command)
	if err != nil {
		return nil, err
	}

	return parseGrokModels(output)
}

func runGrokModels(ctx context.Context, command string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, command, "models").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("grok models: %w", err)
	}

	return output, nil
}

func parseGrokModels(output []byte) ([]string, error) {
	availableModels := false
	models := make([]string, 0)
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Available models:" {
			availableModels = true
			continue
		}
		if !availableModels {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 || (fields[0] != "*" && fields[0] != "-") {
			continue
		}
		if grokModelListed(models, fields[1]) {
			continue
		}
		models = append(models, fields[1])
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("grok models output did not contain an available model")
	}

	return models, nil
}

func grokModelListed(models []string, candidate string) bool {
	for _, model := range models {
		if model == candidate {
			return true
		}
	}

	return false
}

func (adapter *grokAdapter) Detect(ctx context.Context) harness.Detection {
	directories, err := grokCandidateDirectories()
	if err != nil {
		return capacityDetection("", harness.UnknownCapacity(), err.Error(), grokAuthenticationFailure)
	}
	path := executableIn(strings.Join(directories, string(os.PathListSeparator)), "grok")
	if path == "" {
		return absentDetection()
	}

	capacity, detail := adapter.readGrokCapacity(ctx, path)
	return capacityDetection(path, capacity, detail, grokAuthenticationFailure)
}

func (adapter *grokAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{Continuation: true, Sandbox: false}
}

func (adapter *grokAdapter) Validate(harness.StartParams) error {
	return nil
}

// A dispatched job runs unattended. Grok's permission gate cancels with an
// empty result when nobody can answer it, and local approvals can mask that
// behavior, so permission cannot be left to machine state.
func (adapter *grokAdapter) BuildStart(params harness.StartParams) (harness.Request, error) {
	command, err := adapter.resolveGrokCommand()
	if err != nil {
		return harness.Request{}, err
	}

	args := []string{"-p", promptWithPersona(params.Prompt, params.Persona), "--output-format", "json", "--cwd", params.CWD, "--permission-mode", "bypassPermissions"}
	if params.Model != nil && *params.Model != "" {
		args = append(args, "-m", *params.Model)
	}

	return harness.Request{Command: command, Args: args, PersonaDelivery: personaDelivery(params.Persona, harness.PersonaDeliveryPromptComposition)}, nil
}

func (adapter *grokAdapter) BuildResume(params harness.ResumeParams) (harness.Request, error) {
	if params.ThreadID == "" {
		return harness.Request{}, fmt.Errorf("thread_id is required for resume.")
	}

	command, err := adapter.resolveGrokCommand()
	if err != nil {
		return harness.Request{}, err
	}

	return harness.Request{
		Command:         command,
		Args:            []string{"-r", params.ThreadID, "-p", promptWithPersona(params.Prompt, params.Persona), "--output-format", "json", "--cwd", params.CWD, "--permission-mode", "bypassPermissions"},
		PersonaDelivery: personaDelivery(params.Persona, harness.PersonaDeliveryPromptComposition),
	}, nil
}

func (adapter *grokAdapter) OnStdoutLine(string, *harness.JobRecord) {}

func (adapter *grokAdapter) Finalize(_ context.Context, params harness.FinalizeParams) (harness.Finalized, error) {
	result := parseGrokResult(params.Stdout)
	exhausted := params.ExitCode != 0 && grokExhausted(params.Stderr+"\n"+grokStopReason(params.Stdout))
	if result == nil {
		if grokAuthenticationFailure(params.Stdout, params.Stderr) {
			return harness.Finalized{
				Status:    harness.JobStatusFailed,
				Exhausted: exhausted,
				Error:     authenticationError("Grok", "grok login", params.Stdout, params.Stderr),
			}, nil
		}

		return harness.Finalized{
			Status:    harness.JobStatusFailed,
			Exhausted: exhausted,
			Error: strings.Join([]string{
				"Grok stdout did not contain the expected JSON result object.",
				fmt.Sprintf("stdout tail: %s", tail(params.Stdout)),
				fmt.Sprintf("stderr tail: %s", tail(params.Stderr)),
			}, "\n"),
		}, nil
	}

	status := harness.JobStatusFailed
	if params.ExitCode == 0 && *result.StopReason == "end_turn" {
		status = harness.JobStatusDone
	}
	if status == harness.JobStatusDone {
		return harness.Finalized{
			Status:   status,
			Output:   result.Text,
			ThreadID: result.SessionID,
			Error:    truncateAdapterOutput(params.Stderr),
		}, nil
	}

	return harness.Finalized{
		Status:    status,
		Output:    result.Text,
		Exhausted: exhausted,
		ThreadID:  result.SessionID,
		Error: strings.Join([]string{
			"Grok did not complete successfully.",
			fmt.Sprintf("exit code: %d", params.ExitCode),
			fmt.Sprintf("stopReason: %s", *result.StopReason),
			fmt.Sprintf("stderr tail: %s", tail(params.Stderr)),
		}, "\n"),
	}, nil
}

func (adapter *grokAdapter) ProbeCapacity(ctx context.Context) harness.Capacity {
	adapter.mutex.Lock()
	if adapter.capacityCached && adapter.capacityExpiresAt.After(time.Now()) {
		value := adapter.capacity
		adapter.mutex.Unlock()
		return value
	}
	if adapter.capacityInProgress != nil {
		inProgress := adapter.capacityInProgress
		adapter.mutex.Unlock()
		select {
		case <-inProgress:
			adapter.mutex.Lock()
			value := adapter.capacity
			adapter.mutex.Unlock()
			return value
		case <-ctx.Done():
			return harness.UnknownCapacity()
		}
	}

	inProgress := make(chan struct{})
	adapter.capacityInProgress = inProgress
	adapter.mutex.Unlock()

	value, _ := adapter.readGrokCapacity(ctx, "")
	adapter.mutex.Lock()
	adapter.capacity = value
	adapter.capacityCached = true
	adapter.capacityExpiresAt = time.Now().Add(grokCapacityCacheTTL)
	adapter.capacityInProgress = nil
	close(inProgress)
	adapter.mutex.Unlock()

	return value
}

func (adapter *grokAdapter) SpawnPath() string {
	return spawnPath(nil)
}

func (adapter *grokAdapter) resolveGrokCommand() (string, error) {
	adapter.mutex.Lock()
	defer adapter.mutex.Unlock()

	if adapter.command != "" {
		return adapter.command, nil
	}
	if adapter.resolutionErr != nil {
		return "", adapter.resolutionErr
	}

	directories, err := grokCandidateDirectories()
	if err != nil {
		adapter.resolutionErr = err
		return "", err
	}
	for _, directory := range directories {
		candidate, err := executablePath(directory, "grok")
		if err != nil || !isExecutable(candidate) {
			continue
		}
		adapter.command = candidate
		return candidate, nil
	}

	adapter.resolutionErr = fmt.Errorf("No grok executable found. Searched directories: %s.", searchedDirectories(directories))
	return "", adapter.resolutionErr
}

func (adapter *grokAdapter) readGrokCapacity(ctx context.Context, command string) (harness.Capacity, string) {
	if command == "" {
		resolved, err := adapter.resolveGrokCommand()
		if err != nil {
			return harness.UnknownCapacity(), err.Error()
		}
		command = resolved
	}

	result, err := jsonRPC(ctx, command, []string{"agent", "stdio"}, jobs.WithCommsProbeEnvironment(os.Environ()), []rpcStep{
		grokInitializeRequest(),
		request(2, "_x.ai/billing", struct{}{}),
	})
	if err != nil {
		return harness.UnknownCapacity(), err.Error()
	}

	return grokCapacityFromBilling(result)
}

func grokCapacityFromBilling(payload []byte) (harness.Capacity, string) {
	var billing grokBilling
	if err := json.Unmarshal(payload, &billing); err != nil {
		return harness.UnknownCapacity(), fmt.Sprintf("Grok billing response was not valid JSON: %v", err)
	}

	capValue, capDetail := grokBillingNumber(billing.Config.OnDemandCap.Value, "onDemandCap.val")
	if capDetail != "" {
		return harness.UnknownCapacity(), capDetail
	}
	if capValue != nil && *capValue < 0 {
		return harness.UnknownCapacity(), "Grok billing field onDemandCap.val was negative."
	}
	if capValue != nil && *capValue > 0 {
		usedValue, usedDetail := grokBillingNumber(billing.Config.OnDemandUsed.Value, "onDemandUsed.val")
		if usedDetail != "" {
			return harness.UnknownCapacity(), usedDetail
		}
		if usedValue == nil {
			return harness.UnknownCapacity(), "Grok billing response was missing onDemandUsed.val for its positive on-demand cap."
		}
		if *usedValue < 0 {
			return harness.UnknownCapacity(), "Grok billing field onDemandUsed.val was negative."
		}

		return grokMeasuredCapacity((*usedValue / *capValue)*100, billing)
	}

	unified := billing.Config.IsUnifiedBillingUser != nil && *billing.Config.IsUnifiedBillingUser
	if capValue != nil && unified {
		return harness.NoCapacityGauge(), "Grok unified billing has no on-demand cap, so subscription capacity has no gauge."
	}

	legacyValue, legacyDetail := grokBillingNumber(billing.Config.CreditUsagePercent, "creditUsagePercent")
	if legacyDetail != "" {
		return harness.UnknownCapacity(), legacyDetail
	}
	if legacyValue != nil {
		return grokMeasuredCapacity(*legacyValue, billing)
	}
	if capValue != nil {
		return harness.NoCapacityGauge(), "Grok billing has no on-demand cap, so subscription capacity has no gauge."
	}

	return harness.UnknownCapacity(), "Grok billing response was missing both creditUsagePercent and onDemandCap.val."
}

func grokBillingNumber(raw json.RawMessage, name string) (*float64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}

	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		var encoded string
		if stringErr := json.Unmarshal(raw, &encoded); stringErr != nil {
			return nil, fmt.Sprintf("Grok billing field %s was not numeric.", name)
		}
		parsed, parseErr := strconv.ParseFloat(encoded, 64)
		if parseErr != nil {
			return nil, fmt.Sprintf("Grok billing field %s was not numeric.", name)
		}
		value = parsed
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Sprintf("Grok billing field %s was non-finite.", name)
	}

	return &value, ""
}

func grokMeasuredCapacity(usedPercent float64, billing grokBilling) (harness.Capacity, string) {
	if math.IsNaN(usedPercent) || math.IsInf(usedPercent, 0) {
		return harness.UnknownCapacity(), "Grok billing usage percentage was non-finite."
	}
	if usedPercent < 0 {
		return harness.UnknownCapacity(), "Grok billing usage percentage was negative."
	}

	resetValues := make([]string, 0, 2)
	if billing.Config.CurrentPeriod.End != nil {
		resetValues = append(resetValues, *billing.Config.CurrentPeriod.End)
	}
	if billing.Config.BillingPeriodEnd != nil {
		resetValues = append(resetValues, *billing.Config.BillingPeriodEnd)
	}
	if len(resetValues) == 0 {
		return harness.UnknownCapacity(), "Grok billing response was missing currentPeriod.end and billingPeriodEnd."
	}
	for _, resetValue := range resetValues {
		resetsAt, err := time.Parse(time.RFC3339, resetValue)
		if err == nil {
			return harness.KnownCapacity([]harness.Bucket{{
				ID:          "credits",
				UsedPercent: usedPercent,
				ResetsAt:    resetsAt,
			}}), ""
		}
	}

	return harness.UnknownCapacity(), fmt.Sprintf("Grok billing reset timestamp %q was not RFC3339.", strings.Join(resetValues, ", "))
}

func grokInitializeRequest() rpcStep {
	return request(1, "initialize", struct {
		ProtocolVersion    string   `json:"protocolVersion"`
		ClientCapabilities struct{} `json:"clientCapabilities"`
	}{ProtocolVersion: grokProtocolVersion})
}

func grokCandidateDirectories() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	return uniqueDirectories(append([]string{
		filepath.Join(home, ".grok", "bin"),
		filepath.Join(home, ".local", "bin"),
		"/usr/local/bin",
		"/opt/homebrew/bin",
	}, pathDirectories()...)), nil
}

func parseGrokResult(stdout string) *grokResult {
	var result grokResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
		return nil
	}
	if result.Text == nil || result.StopReason == nil || result.SessionID == nil {
		return nil
	}

	return &result
}

func grokStopReason(stdout string) string {
	var result struct {
		StopReason string `json:"stopReason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
		return ""
	}

	return result.StopReason
}

func grokExhausted(value string) bool {
	lower := strings.ToLower(value)
	for _, message := range []string{"hit your weekly limit", "out of credits", "credit limit for your plan", "spending cap"} {
		if strings.Contains(lower, message) {
			return true
		}
	}

	return false
}

func grokAuthenticationFailure(stdout, stderr string) bool {
	output := stdout + "\n" + stderr
	return regexMatches(`(?i)\bnot signed in\b`, output) && regexMatches(`(?i)\bgrok login\b`, output)
}

func regexMatches(pattern, value string) bool {
	matches, err := regexp.MatchString(pattern, value)
	return err == nil && matches
}
