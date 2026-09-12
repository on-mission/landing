// Package dispatch composes routing, persona prompts, and blocking job execution.
package dispatch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
	"github.com/on-mission/landing/internal/journal"
	"github.com/on-mission/landing/internal/paths"
	"github.com/on-mission/landing/internal/persona"
	"github.com/on-mission/landing/internal/router"
)

const defaultAwaitTimeout = 10 * time.Minute

type Request struct {
	Tier         string
	Prompt       string
	CWD          string
	Label        *string
	Persona      string
	AwaitTimeout time.Duration
	Route        *config.Route
}

type ReplyRequest struct {
	JobID        string
	Prompt       string
	Label        *string
	Persona      string
	PersonaCWD   string
	AwaitTimeout time.Duration
}

type Response struct {
	jobs.Projection
	Role string
}

type Engine struct {
	router        *router.Router
	jobs          *jobs.Store
	configuration config.Config
}

func New(router *router.Router, store *jobs.Store, configuration config.Config) *Engine {
	return &Engine{router: router, jobs: store, configuration: configuration}
}

// TierResolutionError reports a request or continuation whose tier is absent from the resolved policy.
type TierResolutionError struct {
	Tier           string
	JobID          string
	AvailableTiers []string
}

func (tierError *TierResolutionError) Error() string {
	available := quotedTiers(tierError.AvailableTiers)
	if tierError.JobID == "" {
		return fmt.Sprintf("requested tier %q is not configured; configured tiers are %s", tierError.Tier, available)
	}
	if tierError.Tier == "" {
		return fmt.Sprintf("job %q has no stored tier; configured tiers are %s", tierError.JobID, available)
	}

	return fmt.Sprintf("job %q was created under tier %q; configured tiers are %s", tierError.JobID, tierError.Tier, available)
}

func (engine *Engine) Dispatch(ctx context.Context, request Request) (Response, error) {
	if request.Route != nil {
		role, err := engine.pinnedRole(request)
		if err != nil {
			return Response{}, err
		}

		return engine.dispatchCast(ctx, role, request)
	}
	tier, err := engine.tier(request.Tier)
	if err != nil {
		return Response{}, err
	}

	return engine.dispatch(ctx, tier, request)
}

// pinnedRole names the work a pinned dispatch is recorded under. A meeting
// cast runs inside a tier and keeps that tier's name. A caller who named a
// route instead of a tier has no tier to record, so the route itself becomes
// the role; that is what later lets a reply resume the thread it started.
func (engine *Engine) pinnedRole(request Request) (string, error) {
	if request.Tier == "" {
		return request.Route.String(), nil
	}
	tier, err := engine.tier(request.Tier)
	if err != nil {
		return "", err
	}

	return tier.Name, nil
}

func (engine *Engine) Reply(ctx context.Context, request ReplyRequest) (Response, error) {
	if request.JobID == "" {
		return Response{}, harness.NewError(harness.ErrorCodeInvalidArgs, "jobId is required.", nil)
	}
	original, err := engine.lookupOrAdopt(ctx, request.JobID)
	if err != nil {
		return Response{}, err
	}
	role, err := roleFor(*original, engine.configuration)
	if err != nil {
		return Response{}, err
	}
	adapter, err := engine.router.Adapter(original.Provider)
	if err != nil {
		return Response{}, fmt.Errorf("resolve original provider %q: %w", original.Provider, err)
	}
	if !adapter.Capabilities().Continuation {
		return Response{}, harness.NewError(
			harness.ErrorCodeContinuationUnsupported,
			fmt.Sprintf("job %q ran on %s, which does not support continuing a thread", request.JobID, original.Provider),
			nil,
		)
	}
	if original.ThreadID == nil || *original.ThreadID == "" {
		return Response{}, harness.NewError(
			harness.ErrorCodeThreadNotFound,
			fmt.Sprintf("job %q recorded no thread id, so it has no thread to continue", request.JobID),
			nil,
		)
	}
	selectedPersona, err := resolvePersona(ctx, request.Persona, request.PersonaCWD)
	if err != nil {
		return Response{}, err
	}
	label := request.Label
	if label == nil {
		label = original.Label
	}
	registration := threadRegistration{resumable: adapter.Capabilities().Continuation}
	record, err := engine.jobs.Resume(ctx, adapter, jobs.ResumeOptions{
		ThreadID:       *original.ThreadID,
		ConversationID: original.JobID,
		Prompt:         request.Prompt,
		Persona:        selectedPersona,
		Model:          original.Model,
		CWD:            original.CWD,
		Label:          label,
		Role:           stringPointer(role),
		BeforeSpawn:    registration.beforeSpawn,
	})
	if err != nil {
		return Response{}, err
	}
	registration.update(record)
	thread := registration.thread
	finished, timeoutMessage, err := engine.await(ctx, record.JobID, request.AwaitTimeout, thread)
	if err != nil {
		return Response{}, err
	}
	response := responseFor(*finished, role)
	response.JobID = original.JobID
	if timeoutMessage != "" {
		response.Status = harness.JobStatusTimeout
		response.Error = stringPointer(timeoutMessage)
	}

	return response, nil
}

func (engine *Engine) dispatch(ctx context.Context, tier config.Tier, request Request) (Response, error) {
	// The CLI resolves the working directory before it reaches here, so this
	// boundary asserts the contract rather than re-deriving it. Re-resolving
	// would need an invocation directory this package does not have, and the
	// last attempt to supply one silently bound the caller's cwd to it.
	cwd := request.CWD
	if !filepath.IsAbs(cwd) {
		return Response{}, harness.NewError(
			harness.ErrorCodeInvalidCWD,
			fmt.Sprintf("dispatch cwd %s is not absolute; it is resolved before dispatch", paths.Display(cwd)),
			nil,
		)
	}
	selectedPersona, err := resolvePersona(ctx, request.Persona, cwd)
	if err != nil {
		return Response{}, err
	}
	route, err := engine.router.Resolve(ctx, tier)
	if err != nil {
		return Response{}, err
	}
	if route.Provider == "" {
		return Response{}, noProviderAvailable(tier)
	}
	adapter, err := engine.router.Adapter(route.Provider)
	if err != nil {
		return Response{}, fmt.Errorf("resolve routed provider %q: %w", route.Provider, err)
	}
	registration := threadRegistration{resumable: adapter.Capabilities().Continuation}
	record, err := engine.jobs.Start(ctx, adapter, jobs.StartOptions{
		Prompt:        request.Prompt,
		Persona:       selectedPersona,
		CWD:           cwd,
		Label:         request.Label,
		Role:          stringPointer(tier.Name),
		Model:         route.Model,
		RoutedBecause: stringPointer(route.RoutedBecause),
		Capacity:      route.Capacities[adapter.ID()],
		RoutingScore:  route.Score,
		BeforeSpawn:   registration.beforeSpawn,
	})
	if err != nil {
		return Response{}, err
	}

	registration.update(record)
	return engine.awaitDispatch(ctx, tier, request, selectedPersona, record, registration.thread)
}

// dispatchCast starts the caller-selected route without tier routing or recovery.
// A cast is a meeting-scoped route choice, so re-routing it would violate that choice.
func (engine *Engine) dispatchCast(ctx context.Context, role string, request Request) (Response, error) {
	cwd := request.CWD
	if !filepath.IsAbs(cwd) {
		return Response{}, harness.NewError(
			harness.ErrorCodeInvalidCWD,
			fmt.Sprintf("dispatch cwd %s is not absolute; it is resolved before dispatch", paths.Display(cwd)),
			nil,
		)
	}
	selectedPersona, err := resolvePersona(ctx, request.Persona, cwd)
	if err != nil {
		return Response{}, err
	}
	adapter, err := engine.router.Adapter(request.Route.Harness)
	if err != nil {
		return Response{}, fmt.Errorf("resolve cast harness %q: %w", request.Route.Harness, err)
	}
	model := request.Route.Model
	reason := fmt.Sprintf("caller cast to %s", adapter.ID())
	if model != nil {
		reason += fmt.Sprintf("/%s", *model)
	}
	registration := threadRegistration{resumable: adapter.Capabilities().Continuation}
	record, err := engine.jobs.Start(ctx, adapter, jobs.StartOptions{
		Prompt:        request.Prompt,
		Persona:       selectedPersona,
		CWD:           cwd,
		Label:         request.Label,
		Role:          stringPointer(role),
		Model:         model,
		RoutedBecause: stringPointer(reason),
		Capacity:      harness.UnknownCapacity(),
		BeforeSpawn:   registration.beforeSpawn,
	})
	if err != nil {
		return Response{}, err
	}

	registration.update(record)
	return engine.awaitCast(ctx, role, request, record, registration.thread)
}

func (engine *Engine) awaitCast(ctx context.Context, role string, request Request, record *harness.JobRecord, thread *threadParticipant) (Response, error) {
	finished, timeoutMessage, err := engine.await(ctx, record.JobID, request.AwaitTimeout, thread)
	if err != nil {
		return Response{}, err
	}
	response := responseFor(*finished, role)
	if timeoutMessage != "" {
		response.Status = harness.JobStatusTimeout
		response.Error = stringPointer(timeoutMessage)
	}

	return response, nil
}

func (engine *Engine) awaitDispatch(ctx context.Context, tier config.Tier, request Request, selectedPersona *harness.Persona, initial *harness.JobRecord, initialThread *threadParticipant) (Response, error) {
	current := initial
	thread := initialThread
	awaitTimeout := normalizedTimeout(request.AwaitTimeout)
	waitContext, cancel := context.WithTimeout(ctx, awaitTimeout)
	defer cancel()
	for {
		finished, err := engine.jobs.Wait(waitContext, current.JobID)
		if err != nil {
			if !errors.Is(err, context.DeadlineExceeded) {
				return Response{}, err
			}
			timedOut, timeoutMessage, timeoutErr := engine.jobs.AbandonTimedOut(context.Background(), current.JobID, awaitTimeout)
			if timeoutErr != nil {
				return Response{}, timeoutErr
			}
			thread.finish(timedOut)
			response := responseFor(*timedOut, tier.Name)
			response.Status = harness.JobStatusTimeout
			response.Error = stringPointer(timeoutMessage)
			return response, nil
		}
		thread.finish(finished)
		if !finished.Exhausted || finished.RerouteCount >= 1 {
			return responseFor(*finished, tier.Name), nil
		}

		expiresAt := engine.router.MarkProviderCold(finished.Provider, finished.Capacity, time.Now())
		failedReason := fmt.Sprintf("%s; %s exhausted and was marked cold until %s; re-routing once", stringValue(finished.RoutedBecause), finished.Provider, expiresAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"))
		if err := engine.jobs.UpdateRerouting(ctx, finished.JobID, failedReason, 1); err != nil {
			return Response{}, err
		}
		reroute, err := engine.router.Resolve(waitContext, tier)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				timedOut, timeoutMessage, timeoutErr := engine.jobs.AbandonTimedOut(context.Background(), finished.JobID, awaitTimeout)
				if timeoutErr != nil {
					return Response{}, timeoutErr
				}
				thread.finish(timedOut)
				response := responseFor(*timedOut, tier.Name)
				response.Status = harness.JobStatusTimeout
				response.Error = stringPointer(timeoutMessage)
				return response, nil
			}
			return Response{}, err
		}
		if reroute.Provider == "" {
			return responseFor(*finished, tier.Name), nil
		}
		nextAdapter, err := engine.router.Adapter(reroute.Provider)
		if err != nil {
			return Response{}, fmt.Errorf("resolve rerouted provider %q: %w", reroute.Provider, err)
		}
		nextRegistration := threadRegistration{resumable: nextAdapter.Capabilities().Continuation}
		next, err := engine.jobs.Start(waitContext, nextAdapter, jobs.StartOptions{
			Prompt:        request.Prompt,
			Persona:       selectedPersona,
			CWD:           finished.CWD,
			Label:         request.Label,
			Role:          stringPointer(tier.Name),
			Model:         reroute.Model,
			RoutedBecause: stringPointer(fmt.Sprintf("%s; re-routed to %s: %s", failedReason, nextAdapter.ID(), reroute.RoutedBecause)),
			Capacity:      reroute.Capacities[nextAdapter.ID()],
			RoutingScore:  reroute.Score,
			RerouteCount:  1,
			ReroutedFrom:  stringPointer(finished.JobID),
			BeforeSpawn:   nextRegistration.beforeSpawn,
		})
		if err != nil {
			return Response{}, err
		}
		if err := engine.jobs.LinkReroute(ctx, finished.JobID, next.JobID); err != nil {
			return Response{}, err
		}
		current = next
		nextRegistration.update(next)
		thread = nextRegistration.thread
	}
}

func resolvePersona(ctx context.Context, name string, cwd string) (*harness.Persona, error) {
	if name == "" {
		return nil, nil
	}

	selected, err := persona.Resolve(ctx, name, cwd)
	if err != nil {
		return nil, err
	}

	return &harness.Persona{
		Instructions:   selected.Instructions,
		Directory:      selected.Directory,
		ReferenceFiles: append([]string(nil), selected.ReferenceFiles...),
	}, nil
}

func (engine *Engine) await(ctx context.Context, jobID string, timeout time.Duration, thread *threadParticipant) (*harness.JobRecord, string, error) {
	awaitTimeout := normalizedTimeout(timeout)
	waitContext, cancel := context.WithTimeout(ctx, awaitTimeout)
	defer cancel()
	record, err := engine.jobs.Wait(waitContext, jobID)
	if err == nil {
		thread.finish(record)
		return record, "", nil
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		return nil, "", err
	}

	record, message, err := engine.jobs.AbandonTimedOut(context.Background(), jobID, awaitTimeout)
	if err == nil {
		thread.finish(record)
	}

	return record, message, err
}

func (engine *Engine) lookupOrAdopt(ctx context.Context, jobID string) (*harness.JobRecord, error) {
	if record, ok := engine.jobs.Get(jobID); ok {
		return record, nil
	}
	recorded, err := journal.ReadJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if recorded == nil {
		return nil, harness.NewError(
			harness.ErrorCodeJobNotFound,
			fmt.Sprintf("thread %q has no retained job record", jobID),
			nil,
		)
	}
	adopted, err := engine.jobs.Adopt(ctx, *recorded, "")
	if err != nil {
		return nil, err
	}
	if adopted == nil {
		return nil, harness.NewError(harness.ErrorCodeJobNotFound, fmt.Sprintf("Job '%s' was not found.", jobID), nil)
	}

	return adopted, nil
}

func responseFor(record harness.JobRecord, role string) Response {
	projection := jobs.Project(record)
	projection.Role = stringPointer(role)
	return Response{Projection: projection, Role: role}
}

func noProviderAvailable(tier config.Tier) *harness.Error {
	return harness.NewError(
		harness.ErrorCodeNoProviderAvailable,
		fmt.Sprintf("no route eligible for tier %q resolved: every route is cold after a recent confirmed failure or measured at no remaining availability", tier.Name),
		nil,
	)
}

func (engine *Engine) tier(name string) (config.Tier, error) {
	tier, ok := engine.configuration.Tier(name)
	if ok {
		return tier, nil
	}

	return config.Tier{}, &TierResolutionError{Tier: name, AvailableTiers: engine.configuration.TierNames()}
}

// roleFor names the work a recorded job ran under. A job routed through a tier
// records that tier's name; a job the caller pinned with --model records the
// route. A configured tier is read first, so a project may name a tier whatever
// it likes without changing how its own jobs resolve.
func roleFor(record harness.JobRecord, configuration config.Config) (string, error) {
	if record.Role == nil || *record.Role == "" {
		return "", &TierResolutionError{JobID: record.JobID, AvailableTiers: configuration.TierNames()}
	}
	tier, ok := configuration.Tier(*record.Role)
	if ok {
		return tier.Name, nil
	}
	if isRouteRole(record, *record.Role) {
		return *record.Role, nil
	}

	return "", &TierResolutionError{
		Tier:           *record.Role,
		JobID:          record.JobID,
		AvailableTiers: configuration.TierNames(),
	}
}

// isRouteRole reports whether a role names the route that actually ran this
// job. Requiring the record's own provider keeps a tier name that has since
// been removed from configuration from being read as a route.
//
// A harness with no pinnable model records just its own name, because that is
// what Route.String() renders for it. Demanding a model here meant such a job
// could be started and never continued: the reply read the bare harness name as
// a tier, found none configured, and refused. cline is that harness today, and
// every --model dispatch to it was a one-shot.
//
// If a tier were named after the harness a job ran on and then removed, that
// job now resumes as a route instead of erroring. That is the right answer —
// the job did run there — and a configured tier is still read first, so a live
// tier of that name keeps its meaning.
func isRouteRole(record harness.JobRecord, role string) bool {
	harnessName, model, hasModel := strings.Cut(role, "/")
	if harnessName != record.Provider {
		return false
	}
	if !hasModel {
		return true
	}

	return model != "" && !strings.Contains(model, "/")
}

func quotedTiers(tiers []string) string {
	if len(tiers) == 0 {
		return "none"
	}
	quoted := make([]string, 0, len(tiers))
	for _, tier := range tiers {
		quoted = append(quoted, fmt.Sprintf("%q", tier))
	}

	return strings.Join(quoted, ", ")
}

func normalizedTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultAwaitTimeout
	}

	return timeout
}

func stringPointer(value string) *string {
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
