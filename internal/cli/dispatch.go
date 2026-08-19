package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/on-mission/landing/internal/adapters"
	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/dispatch"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
	"github.com/on-mission/landing/internal/journal"
	"github.com/on-mission/landing/internal/meeting"
	"github.com/on-mission/landing/internal/paths"
	"github.com/on-mission/landing/internal/router"
)

func runDispatch(ctx context.Context, inputs Inputs, values options, positionals []string, invocationDir string, registry router.Registry) (int, error) {
	if err := unexpectedOptions(values, "prompt dispatch", "tier", "reply", "persona", "cwd", "label", "timeout", "prompt-file", "json"); err != nil {
		return exitUsage, err
	}
	if len(positionals) == 0 && !values.PromptFile.Set {
		return exitUsage, &usageError{message: "no prompt was supplied"}
	}
	configuration, err := loadConfiguration(ctx, invocationDir, registry)
	if err != nil {
		return exitUsage, err
	}
	isReply := values.Reply.Set
	dispatchTier, err := selectTier(values.Tier, isReply, *configuration)
	if err != nil {
		return exitUsage, err
	}
	prompt, err := resolvePrompt(values.PromptFile, positionals, inputs.Stdin, inputs.StdinIsTerminal)
	if err != nil {
		return exitUsage, err
	}
	if prompt == "" {
		return exitUsage, &usageError{message: "the resolved prompt is empty"}
	}
	timeout, err := resolveTimeout(isReply, values.Timeout)
	if err != nil {
		return exitUsage, err
	}
	cwd, err := paths.ResolveDispatchCwd(ctx, invocationDir, values.CWD.Value)
	if err != nil {
		return exitFailed, err
	}
	resolvedValues := values
	resolvedValues.CWD.Value = cwd

	store := jobs.NewStore(ctx)
	defer store.Shutdown(context.Background())
	engine := dispatch.New(router.New(registry), store, *configuration)
	progressName := dispatchTier
	if isReply {
		progressName = "reply"
	}
	stopProgress := startProgress(progressName, inputs.Stderr)
	defer stopProgress()

	response, err := dispatchRequest(ctx, engine, store, invocationDir, resolvedValues, dispatchTier, prompt, timeout)
	if err != nil {
		return exitFailed, err
	}

	return report(response, values.JSON, inputs.Stdout, inputs.Stderr)
}

func runMeeting(ctx context.Context, inputs Inputs, values options, positionals []string, invocationDir string, registry router.Registry) (int, error) {
	if err := unexpectedOptions(values, "meeting", "tier", "persona", "arbiter", "cast", "cwd", "label", "timeout", "prompt-file", "json"); err != nil {
		return exitUsage, err
	}
	if err := meeting.Validate(values.Personas, values.Arbiter.Value); err != nil {
		return exitUsage, &usageError{message: err.Error()}
	}
	prompt, err := resolvePrompt(values.PromptFile, positionals, inputs.Stdin, inputs.StdinIsTerminal)
	if err != nil {
		return exitUsage, err
	}
	casts, err := meetingCasts(ctx, values.Casts, values.Personas, registry)
	if err != nil {
		return exitUsage, err
	}
	if prompt == "" {
		return exitUsage, &usageError{message: "the resolved meeting question is empty"}
	}
	configuration, err := loadConfiguration(ctx, invocationDir, registry)
	if err != nil {
		return exitUsage, err
	}
	tier, err := selectTier(values.Tier, false, *configuration)
	if err != nil {
		return exitUsage, err
	}
	timeout, err := resolveTimeout(false, values.Timeout)
	if err != nil {
		return exitUsage, err
	}
	cwd, err := paths.ResolveDispatchCwd(ctx, invocationDir, values.CWD.Value)
	if err != nil {
		return exitFailed, err
	}

	store := jobs.NewStore(ctx)
	defer store.Shutdown(context.Background())
	engine := dispatch.New(router.New(registry), store, *configuration)
	stopProgress := startProgress("meeting", inputs.Stderr)
	defer stopProgress()
	result, err := meeting.Run(ctx, engine, meeting.Request{
		Question: prompt,
		Personas: values.Personas,
		Arbiter:  values.Arbiter.Value,
		Casts:    casts,
		Tier:     tier,
		CWD:      cwd,
		Label:    optionPointer(values.Label),
		Timeout:  timeout,
	})
	if err != nil {
		return exitFailed, err
	}

	return reportMeeting(result, values.JSON, inputs.Stdout)
}

func meetingCasts(ctx context.Context, casts []castOption, personas []string, registry router.Registry) (map[string]config.Route, error) {
	participants := make(map[string]struct{}, len(personas))
	for _, persona := range personas {
		participants[persona] = struct{}{}
	}
	resolved := make(map[string]config.Route, len(casts))
	for _, cast := range casts {
		if _, ok := participants[cast.Persona]; !ok {
			return nil, &usageError{message: fmt.Sprintf("--cast names %q, which is not a meeting participant", cast.Persona)}
		}
		if _, ok := resolved[cast.Persona]; ok {
			return nil, &usageError{message: fmt.Sprintf("--cast names participant %q more than once", cast.Persona)}
		}
		adapter, err := registry.Resolve(cast.Route.Harness)
		if err != nil {
			return nil, &usageError{message: fmt.Sprintf("--cast for %q names unsupported harness %q", cast.Persona, cast.Route.Harness)}
		}
		detection := adapter.Detect(ctx)
		if detection.Status != harness.DetectionReady {
			return nil, &usageError{message: fmt.Sprintf("--cast for %q names harness %q with observed state %q", cast.Persona, cast.Route.Harness, detection.Status)}
		}
		if cast.Route.Model != nil && !meetingModelSupported(*cast.Route.Model, adapter.Models()) {
			return nil, &usageError{message: fmt.Sprintf("--cast for %q names model %q, which harness %q cannot reach; reachable models are %s", cast.Persona, *cast.Route.Model, cast.Route.Harness, quotedModels(adapter.Models()))}
		}
		route := config.Route{Harness: cast.Route.Harness, Model: cast.Route.Model}
		resolved[cast.Persona] = route
	}

	return resolved, nil
}

func meetingModelSupported(model string, models []string) bool {
	for _, candidate := range models {
		if model == candidate {
			return true
		}
	}

	return false
}

func quotedModels(models []string) string {
	if len(models) == 0 {
		return "none"
	}
	quoted := make([]string, 0, len(models))
	for _, model := range models {
		quoted = append(quoted, fmt.Sprintf("%q", model))
	}

	return strings.Join(quoted, ", ")
}

func loadConfiguration(ctx context.Context, invocationDir string, registry router.Registry) (*config.Config, error) {
	configuration, err := config.Load(ctx, invocationDir, config.NewHarnesses(registry))
	if err != nil {
		return nil, err
	}

	return &configuration, nil
}

func selectTier(supplied parsedOption, reply bool, configuration config.Config) (string, error) {
	if reply {
		if supplied.Set {
			return "", &usageError{message: "--tier is present with --reply"}
		}
		return "", nil
	}
	if supplied.Set {
		if _, ok := configuration.Tier(supplied.Value); !ok {
			return "", &usageError{message: fmt.Sprintf("tier %q is not configured; configured tiers are %s", supplied.Value, strings.Join(configuration.TierNames(), ", "))}
		}
		return supplied.Value, nil
	}
	if configuration.DefaultTier != "" {
		return configuration.DefaultTier, nil
	}

	return "", &usageError{message: fmt.Sprintf("no default tier is configured; configured tiers are %s", strings.Join(configuration.TierNames(), ", "))}
}

func dispatchRequest(ctx context.Context, engine *dispatch.Engine, store *jobs.Store, invocationDir string, values options, tier string, prompt string, timeout time.Duration) (dispatch.Response, error) {
	label := optionPointer(values.Label)
	if !values.Reply.Set {
		return engine.Dispatch(ctx, dispatch.Request{Tier: tier, Prompt: prompt, CWD: values.CWD.Value, Label: label, Persona: values.Persona.Value, AwaitTimeout: timeout})
	}
	if values.Reply.Value == "" {
		return dispatch.Response{}, &usageError{message: "--reply has an empty id"}
	}
	if err := adoptThread(ctx, store, values.Reply.Value, invocationDir, values.CWD); err != nil {
		return dispatch.Response{}, err
	}
	return engine.Reply(ctx, dispatch.ReplyRequest{JobID: values.Reply.Value, Prompt: prompt, Label: label, Persona: values.Persona.Value, PersonaCWD: values.CWD.Value, AwaitTimeout: timeout})
}

func adoptThread(ctx context.Context, store *jobs.Store, jobID string, invocationDir string, suppliedCWD parsedOption) error {
	if _, exists := store.Get(jobID); exists {
		return nil
	}
	recorded, err := journal.ReadJob(ctx, jobID)
	if err != nil {
		return err
	}
	if recorded == nil {
		return harness.NewError(harness.ErrorCodeJobNotFound, fmt.Sprintf("thread %q has no retained job record", jobID), nil)
	}
	if recorded.ThreadID == nil || *recorded.ThreadID == "" {
		return harness.NewError(harness.ErrorCodeThreadNotFound, fmt.Sprintf("job %q has no thread id", jobID), nil)
	}
	overrideCWD := ""
	if suppliedCWD.Set {
		overrideCWD, err = paths.ResolveDispatchCwd(ctx, invocationDir, suppliedCWD.Value)
		if err != nil {
			return err
		}
	}
	_, err = store.Adopt(ctx, *recorded, overrideCWD)

	return err
}

func optionPointer(option parsedOption) *string {
	if !option.Set {
		return nil
	}

	return &option.Value
}

func newRegistry() router.Registry {
	return router.NewMapRegistry(map[string]harness.Adapter{
		"codex":  adapters.NewCodex(),
		"claude": adapters.NewClaude(),
		"grok":   adapters.NewGrok(),
		"cline":  adapters.NewCline(),
	})
}

func startProgress(name string, stderr io.Writer) func() {
	startedAt := time.Now()
	ticker := time.NewTicker(progressInterval)
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-ticker.C:
				elapsed := int(math.Round(now.Sub(startedAt).Seconds()))
				if _, err := fmt.Fprintf(stderr, "%s: still running, %ds elapsed\n", name, elapsed); err != nil {
					return
				}
			}
		}
	}()

	return func() {
		close(done)
		<-stopped
	}
}

func report(response dispatch.Response, asJSON bool, stdout io.Writer, stderr io.Writer) (int, error) {
	projected := projectResult(response, response.RoutedBecause)
	if asJSON {
		encoded, err := json.MarshalIndent(projected, "", "  ")
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
			return exitFailed, err
		}
	} else {
		if projected.Output != nil && *projected.Output != "" {
			if _, err := fmt.Fprint(stdout, strings.TrimRight(*projected.Output, "\n")+"\n"); err != nil {
				return exitFailed, err
			}
		}
		if projected.Error != nil && *projected.Error != "" {
			if _, err := fmt.Fprintln(stderr, *projected.Error); err != nil {
				return exitFailed, err
			}
		}
		if projected.JobID != "" {
			if _, err := fmt.Fprintf(stdout, "thread: %s\n", projected.JobID); err != nil {
				return exitFailed, err
			}
		}
		if projected.Provider != nil {
			model := "unpinned"
			if projected.Model != nil {
				model = *projected.Model
			}
			if _, err := fmt.Fprintf(stdout, "harness: %s (model: %s)\n", *projected.Provider, model); err != nil {
				return exitFailed, err
			}
		}
	}
	if projected.Status == harness.JobStatusDone {
		return exitOK, nil
	}

	return exitFailed, nil
}

func projectResult(response dispatch.Response, routedBecause *string) result {
	provider := response.Provider
	var providerPointer *string
	if provider != "" {
		providerPointer = &provider
	}
	role := string(response.Role)

	return result{JobID: response.JobID, Role: &role, Provider: providerPointer, Model: response.Model, Label: response.Label, Status: response.Status, StartedAt: response.StartedAt, FinishedAt: response.FinishedAt, DurationMS: response.Duration.Milliseconds(), ExitCode: response.ExitCode, Output: response.Output, Error: response.Error, RoutedBecause: routedBecause, PersonaDelivery: response.PersonaDelivery}
}

func runSelftest(stdout io.Writer, stderr io.Writer) (int, bool, error) {
	stdoutBytes := selftestBytes("LANDING_SELFTEST_BYTES")
	stderrBytes := selftestBytes("LANDING_SELFTEST_STDERR_BYTES")
	if stdoutBytes == 0 && stderrBytes == 0 {
		return exitOK, false, nil
	}
	if stdoutBytes > 0 {
		if _, err := fmt.Fprint(stdout, strings.Repeat("x", stdoutBytes)+"\nEND_MARKER\n"); err != nil {
			return exitFailed, true, err
		}
	}
	if stderrBytes > 0 {
		if _, err := fmt.Fprint(stderr, strings.Repeat("y", stderrBytes)+"\nEND_MARKER_ERR\n"); err != nil {
			return exitFailed, true, err
		}
	}

	return exitOK, true, nil
}

func selftestBytes(variable string) int {
	raw := os.Getenv(variable)
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > float64(int(^uint(0)>>1)) {
		return 0
	}

	return int(value)
}

func isUsageCode(code harness.ErrorCode) bool {
	switch code {
	case harness.ErrorCodeInvalidArgs, harness.ErrorCodeInvalidCWD, harness.ErrorCodeConfigNotFound, harness.ErrorCodeConfigInvalid, harness.ErrorCodeInvalidPersona, harness.ErrorCodePersonaNotFound, harness.ErrorCodePersonaDelegationUnclosed, harness.ErrorCodeJobNotFound, harness.ErrorCodeThreadNotFound, harness.ErrorCodeContinuationUnsupported:
		return true
	default:
		return false
	}
}
