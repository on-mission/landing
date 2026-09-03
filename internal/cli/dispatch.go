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
	if err := unexpectedOptions(values, "prompt dispatch", "tier", "model", "reply", "persona", "cwd", "label", "timeout", "prompt-file", "json"); err != nil {
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
	target, err := selectTarget(ctx, values, isReply, *configuration, registry)
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
	progressName := target.name()
	if isReply {
		progressName = "reply"
	}
	stopProgress := startProgress(progressName, inputs.Stderr)
	defer stopProgress()

	response, err := dispatchRequest(ctx, engine, store, invocationDir, resolvedValues, target, prompt, timeout)
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
		route, err := pinnedRoute(ctx, cast.Route, registry, fmt.Sprintf("--cast for %q", cast.Persona))
		if err != nil {
			return nil, err
		}
		resolved[cast.Persona] = route
	}

	return resolved, nil
}

// pinnedRoute validates a route the caller named outright, for a dispatch's
// --model and for a meeting's --cast alike. A pinned route skips tier routing,
// and with it the configuration checks that would have caught an unsupported
// harness or an unreachable model, so this is where those are caught instead.
func pinnedRoute(ctx context.Context, option routeOption, registry router.Registry, subject string) (config.Route, error) {
	adapter, err := registry.Resolve(option.Harness)
	if err != nil {
		return config.Route{}, &usageError{message: fmt.Sprintf("%s names unsupported harness %q; supported harnesses are %s", subject, option.Harness, strings.Join(sortedHarnessIDs(registry), ", "))}
	}
	// A pinned route never consults capacity, so a failed or absent capacity
	// gauge is no reason to refuse one: an unmeasured route ranks below a
	// measured route rather than being disabled. What does refuse a pin is a
	// harness that cannot run the work at all — one that is not installed, or
	// that is installed and has said it is not authenticated.
	detection := adapter.Detect(ctx)
	if detection.Status == harness.DetectionAbsent || detection.Status == harness.DetectionUnauthenticated {
		return config.Route{}, &usageError{message: fmt.Sprintf("%s names harness %q with observed state %q", subject, option.Harness, detection.Status)}
	}
	catalog := adapter.ModelCatalog()
	if option.Model != nil && !catalog.Supports(*option.Model) {
		return config.Route{}, &usageError{message: fmt.Sprintf("%s names model %q, which harness %q cannot reach; reachable models are %s", subject, *option.Model, option.Harness, quotedModels(catalog.Models))}
	}

	return config.Route{Harness: option.Harness, Model: option.Model}, nil
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

// dispatchTarget is what a dispatch runs on: a tier Landing routes within, or
// a route the caller named outright. Exactly one of the two is set.
type dispatchTarget struct {
	Tier  string
	Route *config.Route
}

// name is what progress output and diagnostics call this target.
func (target dispatchTarget) name() string {
	if target.Route != nil {
		return target.Route.String()
	}

	return target.Tier
}

// selectTarget resolves a dispatch's execution path. --tier names a policy
// Landing routes within; --model names one route and skips routing entirely.
// Both answer the same question, so a caller supplies at most one.
func selectTarget(ctx context.Context, values options, reply bool, configuration config.Config, registry router.Registry) (dispatchTarget, error) {
	if values.Model == nil {
		tier, err := selectTier(values.Tier, reply, configuration)
		if err != nil {
			return dispatchTarget{}, err
		}

		return dispatchTarget{Tier: tier}, nil
	}
	if values.Tier.Set {
		return dispatchTarget{}, &usageError{message: "--model is present with --tier; a dispatch names one or the other"}
	}
	if reply {
		return dispatchTarget{}, &usageError{message: "--model is present with --reply; a reply continues on the route its thread already runs on"}
	}
	route, err := pinnedRoute(ctx, *values.Model, registry, "--model")
	if err != nil {
		return dispatchTarget{}, err
	}

	return dispatchTarget{Route: &route}, nil
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

func dispatchRequest(ctx context.Context, engine *dispatch.Engine, store *jobs.Store, invocationDir string, values options, target dispatchTarget, prompt string, timeout time.Duration) (dispatch.Response, error) {
	label := optionPointer(values.Label)
	if !values.Reply.Set {
		return engine.Dispatch(ctx, dispatch.Request{Tier: target.Tier, Route: target.Route, Prompt: prompt, CWD: values.CWD.Value, Label: label, Persona: values.Persona.Value, AwaitTimeout: timeout})
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
