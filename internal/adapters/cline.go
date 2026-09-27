package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/on-mission/landing/internal/harness"
)

const clineMinimumMajorVersion = 3

type clineAdapter struct {
	mutex         sync.Mutex
	command       string
	resolutionErr error
}

type clineVersionResult struct {
	command string
	version string
	major   int
}

type clineEvent struct {
	Type          *string         `json:"type"`
	HookEventName *string         `json:"hookEventName"`
	TaskID        *string         `json:"taskId"`
	FinishReason  json.RawMessage `json:"finishReason"`
	Text          json.RawMessage `json:"text"`
}

var _ harness.Adapter = (*clineAdapter)(nil)
var _ harness.ConfigValidator = (*clineAdapter)(nil)

func NewCline() harness.Adapter {
	return &clineAdapter{}
}

func (adapter *clineAdapter) ID() string {
	return "cline"
}

func (adapter *clineAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{Authority: harness.ModelCatalogAdvisory}
}

func (adapter *clineAdapter) ValidateConfiguration(routes []harness.ConfiguredRoute) error {
	var configured *harness.ConfiguredRoute
	for index := range routes {
		route := &routes[index]
		if route.Model == nil || *route.Model == "" {
			continue
		}
		modelType, modelID, hasModelType := strings.Cut(*route.Model, "/")
		if !hasModelType || modelType == "" || modelID == "" || strings.Contains(modelID, "/") {
			return fmt.Errorf("cline model %q at %s must use provider/model format", *route.Model, route.Location)
		}
		if configured == nil {
			configured = route
			continue
		}
		if *configured.Model == *route.Model {
			continue
		}

		return fmt.Errorf("cline model %q at %s conflicts with cline model %q at %s; configure at most one cline model", *configured.Model, configured.Location, *route.Model, route.Location)
	}

	return nil
}

func (adapter *clineAdapter) Detect(ctx context.Context) harness.Detection {
	directories := clineCandidateDirectories()
	path := executableIn(strings.Join(directories, string(os.PathListSeparator)), "cline")
	if path == "" {
		return absentDetection()
	}

	// This intentionally remains ready: `cline config` requires both streams to
	// be terminals, and `cline doctor` reports versions and hub-daemon diagnostics, not auth.
	// When Cline exposes a non-interactive auth signal, apply Grok's treatment.
	return capacityDetection(path, adapter.ProbeCapacity(ctx), "Cline does not expose a capacity gauge.", clineAuthenticationFailure)
}

func (adapter *clineAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{Continuation: false, Sandbox: false}
}

func (adapter *clineAdapter) Validate(params harness.StartParams) error {
	if params.Timeout != nil && *params.Timeout < 0 {
		return fmt.Errorf("timeout must be a non-negative number of seconds.")
	}

	return nil
}

func (adapter *clineAdapter) BuildStart(params harness.StartParams) (harness.Request, error) {
	command, err := adapter.resolveClineCommand()
	if err != nil {
		return harness.Request{}, err
	}

	args := []string{
		"--json",
		"--auto-approve",
		"true",
		"-c",
		params.CWD,
	}
	if params.Timeout != nil {
		args = append(args, "-t", clineTimeoutArgument(*params.Timeout))
	}
	if params.Model != nil && *params.Model != "" {
		args = append(args, "-m", *params.Model)
	}
	if params.Provider != nil && *params.Provider != "" {
		args = append(args, "-P", *params.Provider)
	}
	args = append(args, promptWithPersona(params.Prompt, params.Persona))

	return harness.Request{Command: command, Args: args, PersonaDelivery: personaDelivery(params.Persona, harness.PersonaDeliveryPromptComposition)}, nil
}

func (adapter *clineAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return harness.Request{}, fmt.Errorf("The cline provider cannot continue a thread headlessly through its CLI.")
}

func (adapter *clineAdapter) OnStdoutLine(line string, record *harness.JobRecord) {
	if record == nil || record.ThreadID != nil {
		return
	}

	var event clineEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &event); err != nil {
		return
	}
	if event.Type == nil || *event.Type != "hook_event" || event.HookEventName == nil || *event.HookEventName != "agent_start" || event.TaskID == nil {
		return
	}

	record.ThreadID = event.TaskID
}

func (adapter *clineAdapter) Finalize(_ context.Context, params harness.FinalizeParams) (harness.Finalized, error) {
	result := lastClineRunResult(params.Stdout)
	authentication := ""
	if params.ExitCode != 0 && clineAuthenticationFailure(params.Stdout, params.Stderr) {
		authentication = authenticationError("Cline", "cline auth", params.Stdout, params.Stderr)
	}
	if result == nil {
		errorMessage := authentication
		if errorMessage == "" {
			errorMessage = clineFailureError(params.Stderr, nil, "No run_result line found in cline stdout.")
		}
		return harness.Finalized{
			Status: harness.JobStatusFailed,
			Error:  errorMessage,
		}, nil
	}

	finishReason := jsonString(result.FinishReason)
	status := harness.JobStatusFailed
	if params.ExitCode == 0 && finishReason != nil && *finishReason == "completed" {
		status = harness.JobStatusDone
	}
	if status == harness.JobStatusDone {
		return harness.Finalized{
			Status: status,
			Output: jsonString(result.Text),
			Error:  truncateAdapterOutput(params.Stderr),
		}, nil
	}

	errorMessage := authentication
	if errorMessage == "" {
		errorMessage = clineFailureError(params.Stderr, finishReason, "Cline did not complete successfully.")
	}
	return harness.Finalized{
		Status: status,
		Output: jsonString(result.Text),
		Error:  errorMessage,
	}, nil
}

func (adapter *clineAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return harness.NoCapacityGauge()
}

func (adapter *clineAdapter) SpawnPath() string {
	adapter.mutex.Lock()
	command := adapter.command
	adapter.mutex.Unlock()
	if command == "" {
		return spawnPath(nil)
	}

	return spawnPath([]string{filepath.Dir(command)})
}

func (adapter *clineAdapter) resolveClineCommand() (string, error) {
	adapter.mutex.Lock()
	defer adapter.mutex.Unlock()

	if adapter.command != "" {
		return adapter.command, nil
	}
	if adapter.resolutionErr != nil {
		return "", adapter.resolutionErr
	}

	directories := clineCandidateDirectories()
	versions := clineVersions(directories)
	for _, version := range versions {
		if version.major < clineMinimumMajorVersion {
			continue
		}
		adapter.command = version.command
		return version.command, nil
	}

	adapter.resolutionErr = fmt.Errorf("%s", noCompatibleClineError(directories, versions))
	return "", adapter.resolutionErr
}

func clineCandidateDirectories() []string {
	candidates := make([]string, 0, 5)
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(executable))
	}
	if nvmBin := os.Getenv("NVM_BIN"); nvmBin != "" {
		candidates = append(candidates, nvmBin)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "bin"))
	}
	candidates = append(candidates, "/usr/local/bin", "/opt/homebrew/bin")

	return uniqueDirectories(append(candidates, pathDirectories()...))
}

func clineVersions(directories []string) []clineVersionResult {
	versions := make([]clineVersionResult, 0, len(directories))
	seenCommands := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		command, err := executablePath(directory, "cline")
		if err != nil || !isExecutable(command) {
			continue
		}
		if _, exists := seenCommands[command]; exists {
			continue
		}
		seenCommands[command] = struct{}{}
		version, ok := clineVersion(command)
		if !ok {
			continue
		}
		versions = append(versions, version)
	}

	return versions
}

func clineVersion(command string) (clineVersionResult, bool) {
	output, err := exec.Command(command, "--version").Output()
	if err != nil {
		return clineVersionResult{}, false
	}

	match := regexp.MustCompile(`\b(\d+)\.(\d+)\.(\d+)\b`).FindString(string(output))
	if match == "" {
		return clineVersionResult{}, false
	}
	major, err := strconv.Atoi(strings.SplitN(match, ".", 2)[0])
	if err != nil {
		return clineVersionResult{}, false
	}

	return clineVersionResult{command: command, version: match, major: major}, true
}

func noCompatibleClineError(directories []string, versions []clineVersionResult) string {
	found := "- no executable cline binaries were found"
	if len(versions) > 0 {
		entries := make([]string, 0, len(versions))
		for _, version := range versions {
			entries = append(entries, fmt.Sprintf("- found cline %s at %s", version.version, version.command))
		}
		found = strings.Join(entries, "\n")
	}

	return strings.Join([]string{
		fmt.Sprintf("No compatible cline executable found; need cline >= %d.0.", clineMinimumMajorVersion),
		fmt.Sprintf("Searched directories: %s.", searchedDirectories(directories)),
		"Found:",
		found,
	}, "\n")
}

func clineTimeoutArgument(timeout time.Duration) string {
	return strconv.FormatFloat(timeout.Seconds(), 'f', -1, 64)
}

func lastClineRunResult(stdout string) *clineEvent {
	var result *clineEvent
	for _, line := range strings.Split(stdout, "\n") {
		var event clineEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type == nil || *event.Type != "run_result" {
			continue
		}
		result = &event
	}

	return result
}

func clineFailureError(stderr string, finishReason *string, message string) string {
	reason := "missing"
	if finishReason != nil {
		reason = *finishReason
	}

	return strings.Join([]string{
		message,
		fmt.Sprintf("finishReason: %s", reason),
		fmt.Sprintf("stderr tail: %s", tail(stderr)),
	}, "\n")
}

func clineAuthenticationFailure(stdout, stderr string) bool {
	output := stdout + "\n" + stderr
	return regexMatches(`(?i)\b(?:not authenticated|unauthorized|authentication required|no active configuration)\b`, output) &&
		regexMatches(`(?i)\bcline auth\b`, output)
}

func jsonString(value json.RawMessage) *string {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}

	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil
	}

	return &decoded
}

func uniqueDirectories(candidates []string) []string {
	directories := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, directory := range candidates {
		if directory == "" {
			continue
		}
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		directories = append(directories, directory)
	}

	return directories
}

func pathDirectories() []string {
	return strings.FieldsFunc(os.Getenv("PATH"), func(value rune) bool {
		return value == os.PathListSeparator
	})
}

func executablePath(directory, name string) (string, error) {
	return filepath.Abs(filepath.Join(directory, name))
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	return info.Mode().Perm()&0o111 != 0
}

func searchedDirectories(directories []string) string {
	if len(directories) == 0 {
		return "(none)"
	}

	return strings.Join(directories, ", ")
}

func truncateAdapterOutput(value string) string {
	if len(value) <= outputTailBytes {
		return value
	}

	return value[len(value)-outputTailBytes:]
}
