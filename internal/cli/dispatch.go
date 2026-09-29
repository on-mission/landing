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
	"github.com/on-mission/landing/internal/modeltarget"
	"github.com/on-mission/landing/internal/modelvalidation"
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
	if target.Tier != "" {
		tier := configuration.Tiers[target.Tier]
		executionTier, tierResolutions, err := modeltarget.New(*configuration, registry).ResolveTier(tier)
		if err != nil {
			return exitUsage, err
		}
		executionConfiguration := *configuration
		executionConfiguration.Tiers = make(map[string]config.Tier, len(configuration.Tiers))
		for name, configuredTier := range configuration.Tiers {
			executionConfiguration.Tiers[name] = configuredTier
		}
		executionConfiguration.Tiers[target.Tier] = executionTier
		configuration = &executionConfiguration
		target.resolutions = append(target.resolutions, tierResolutions...)
	}
	prompt, err := resolvePrompt(values.PromptFile, positionals, inputs.Stdin, inputs.StdinIsTerminal)
	if err != nil {
		return exitUsage, err
	}
	if prompt == "" {
		return exitUsage, &usageError{message: "the resolved prompt is empty"}
	}
	timeout, err := resolveTimeout(values.Timeout)
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

	return report(response, values.JSON, inputs.Stdout, inputs.Stderr, resolutionFor(response, target.resolutions))
}

func runMeeting(ctx context.Context, inputs Inputs, values options, positionals []string, invocationDir string, registry router.Registry) (int, error) {
	if err := unexpectedOptions(values, "meeting", "tier", "persona", "arbiter", "cast", "cwd", "label", "timeout", "prompt-file", "json"); err != nil {
		return exitUsage, err
	}
	if _, _, _, err := parseMeetingArbiter(values.Arbiter.Value); err != nil {
		return exitUsage, err
	}
	prompt, err := resolvePrompt(values.PromptFile, positionals, inputs.Stdin, inputs.StdinIsTerminal)
	if err != nil {
		return exitUsage, err
	}
	configuration, err := loadConfiguration(ctx, invocationDir, registry)
	if err != nil {
		return exitUsage, err
	}
	if prompt == "" {
		return exitUsage, &usageError{message: "the resolved meeting question is empty"}
	}
	tier, err := selectTier(values.Tier, false, *configuration)
	if err != nil {
		return exitUsage, err
	}
	executionTier, _, err := modeltarget.New(*configuration, registry).ResolveTier(configuration.Tiers[tier])
	if err != nil {
		return exitUsage, err
	}
	executionConfiguration := *configuration
	executionConfiguration.Tiers = make(map[string]config.Tier, len(configuration.Tiers))
	for name, configuredTier := range configuration.Tiers {
		executionConfiguration.Tiers[name] = configuredTier
	}
	executionConfiguration.Tiers[tier] = executionTier
	configuration = &executionConfiguration
	timeout, err := resolveTimeout(values.Timeout)
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
	seats, arbiter, err := meetingSeats(ctx, values, *configuration, registry, tier, engine.MeetingRoute)
	if err != nil {
		return exitUsage, err
	}
	stopProgress := startProgress("meeting", inputs.Stderr)
	defer stopProgress()
	result, err := meeting.Run(ctx, engine, meeting.Request{
		Question: prompt,
		Seats:    seats,
		Arbiter:  arbiter,
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

type meetingRouteResolver func(context.Context, string) (config.Route, error)

func meetingSeats(ctx context.Context, values options, configuration config.Config, registry router.Registry, tier string, routeForTier meetingRouteResolver) ([]meeting.Seat, meeting.Seat, error) {
	resolver := modeltarget.New(configuration, registry)
	castResolutions := make(map[string][]modeltarget.Resolution, len(values.Casts))
	castPersonas := make([]string, 0, len(values.Casts))
	for _, cast := range values.Casts {
		resolutions, err := resolver.Resolve(cast.Route.Target)
		if err != nil {
			return nil, meeting.Seat{}, &usageError{message: fmt.Sprintf("--cast for %q: %v", cast.Persona, err)}
		}
		if _, exists := castResolutions[cast.Persona]; !exists {
			castPersonas = append(castPersonas, cast.Persona)
		}
		castResolutions[cast.Persona] = append(castResolutions[cast.Persona], resolutions...)
	}
	participants := uniquePersonas(values.Personas, castPersonas)
	seats := make([]meeting.Seat, 0, len(participants))
	for _, persona := range participants {
		resolutions := deduplicateMeetingResolutions(castResolutions[persona])
		if len(resolutions) == 0 {
			route, err := routeForTier(ctx, tier)
			if err != nil {
				return nil, meeting.Seat{}, err
			}
			seats = append(seats, meeting.Seat{Persona: persona, Route: route})
			continue
		}
		for _, resolution := range resolutions {
			seats = append(seats, meeting.Seat{Persona: persona, Route: resolution.Route, Target: resolution.Written, LatestSource: string(resolution.LatestSource)})
		}
	}
	arbiter, err := meetingArbiter(ctx, values.Arbiter.Value, resolver, routeForTier, tier)
	if err != nil {
		return nil, meeting.Seat{}, err
	}

	return seats, arbiter, nil
}

func meetingArbiter(ctx context.Context, value string, resolver modeltarget.Resolver, routeForTier meetingRouteResolver, tier string) (meeting.Seat, error) {
	persona, target, pinned, err := parseMeetingArbiter(value)
	if err != nil {
		return meeting.Seat{}, err
	}
	if !pinned {
		route, err := routeForTier(ctx, tier)
		if err != nil {
			return meeting.Seat{}, err
		}
		return meeting.Seat{Persona: persona, Route: route}, nil
	}
	resolutions, err := resolver.Resolve(target)
	if err != nil {
		return meeting.Seat{}, &usageError{message: fmt.Sprintf("--arbiter for %q: %v", persona, err)}
	}
	if len(resolutions) != 1 {
		routes := make([]string, 0, len(resolutions))
		for _, resolution := range resolutions {
			routes = append(routes, resolution.Route.String())
		}
		return meeting.Seat{}, &usageError{message: fmt.Sprintf("--arbiter target %q resolves to several routes: %s", target, strings.Join(routes, ", "))}
	}

	resolution := resolutions[0]
	return meeting.Seat{Persona: persona, Route: resolution.Route, Target: resolution.Written, LatestSource: string(resolution.LatestSource)}, nil
}

func parseMeetingArbiter(value string) (string, string, bool, error) {
	persona, target, pinned := strings.Cut(value, "=")
	if persona == "" {
		return "", "", false, &usageError{message: "meeting has no arbiter"}
	}
	if pinned && target == "" {
		return "", "", false, &usageError{message: fmt.Sprintf("--arbiter has invalid value %q; expected persona or persona=target", value)}
	}

	return persona, target, pinned, nil
}

func uniquePersonas(groups ...[]string) []string {
	seen := make(map[string]struct{})
	personas := make([]string, 0)
	for _, group := range groups {
		for _, persona := range group {
			if _, exists := seen[persona]; exists {
				continue
			}
			seen[persona] = struct{}{}
			personas = append(personas, persona)
		}
	}

	return personas
}

func deduplicateMeetingResolutions(resolutions []modeltarget.Resolution) []modeltarget.Resolution {
	seen := make(map[string]struct{}, len(resolutions))
	unique := make([]modeltarget.Resolution, 0, len(resolutions))
	for _, resolution := range resolutions {
		key := resolution.Route.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, resolution)
	}

	return unique
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
	if option.Model != nil {
		validation := modelvalidation.Validate(ctx, adapter, *option.Model)
		if validation.Status != harness.ModelValid {
			return config.Route{}, &usageError{message: fmt.Sprintf("model %s/%s is %s: %s", option.Harness, *option.Model, validation.Status, validation.Evidence)}
		}
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
	Tier        string
	Route       *config.Route
	resolutions []modeltarget.Resolution
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
	if values.Model.Target == "" {
		route, err := pinnedRoute(ctx, *values.Model, registry, "--model")
		if err != nil {
			return dispatchTarget{}, err
		}

		return dispatchTarget{Route: &route}, nil
	}
	resolutions, err := modeltarget.New(configuration, registry).Resolve(values.Model.Target)
	if err != nil {
		return dispatchTarget{}, &usageError{message: err.Error()}
	}
	if len(resolutions) != 1 {
		routes := make([]string, 0, len(resolutions))
		for _, resolution := range resolutions {
			routes = append(routes, resolution.Route.String())
		}
		return dispatchTarget{}, &usageError{message: fmt.Sprintf("--model target %q resolves to several routes: %s", values.Model.Target, strings.Join(routes, ", "))}
	}
	route, err := pinnedRoute(ctx, routeOption{Harness: resolutions[0].Route.Harness, Model: resolutions[0].Route.Model}, registry, "--model")
	if err != nil {
		return dispatchTarget{}, err
	}

	return dispatchTarget{Route: &route, resolutions: resolutions}, nil
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

func report(response dispatch.Response, asJSON bool, stdout io.Writer, stderr io.Writer, resolution *modeltarget.Resolution) (int, error) {
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
		if resolution != nil {
			if _, err := fmt.Fprintf(stdout, "route: %s%s\n", resolution.Route.String(), resolutionNote(*resolution)); err != nil {
				return exitFailed, err
			}
		}
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

func resolutionFor(response dispatch.Response, resolutions []modeltarget.Resolution) *modeltarget.Resolution {
	route := config.Route{Harness: response.Provider, Model: response.Model}.String()
	for index := range resolutions {
		if resolutions[index].Route.String() == route {
			return &resolutions[index]
		}
	}

	return nil
}

func resolutionNote(resolution modeltarget.Resolution) string {
	if resolution.LatestSource == "" {
		return ""
	}

	return fmt.Sprintf(" (latest; %s)", resolution.LatestSource)
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
