package meeting

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/dispatch"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
)

func TestRunKeepsSeatContextsIsolatedAndPassesSeatLabelsVerbatimToArbiter(t *testing.T) {
	dispatcher := &fakeDispatcher{dispatch: func(_ context.Context, request dispatch.Request) (dispatch.Response, error) {
		if request.Persona == "chair" && strings.Contains(request.Prompt, "The participant positions:") {
			return response("arbiter reading"), nil
		}
		return response(request.Persona + " position"), nil
	}}
	result, err := Run(context.Background(), dispatcher, testRequest())
	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if result.Arbiter == nil || result.Arbiter.Response == nil || len(result.Positions) != 3 {
		t.Fatalf("Run() = %#v, want positions and arbiter reading", result)
	}
	requests := dispatcher.Requests()
	for _, request := range requests[:3] {
		for _, answer := range []string{"optimist position", "skeptic position", "observer position"} {
			if strings.Contains(request.Prompt, answer) {
				t.Fatalf("participant prompt for %q contains %q: %q", request.Persona, answer, request.Prompt)
			}
		}
	}
	arbiter := requestFor(requests, "chair", "The participant positions:")
	for _, want := range []string{"## optimist · codex/sol", "## skeptic · claude/opus", "## observer · grok/fast", "optimist position", "skeptic position", "observer position"} {
		if !strings.Contains(arbiter.Prompt, want) {
			t.Fatalf("arbiter prompt = %q, missing %q", arbiter.Prompt, want)
		}
	}
}

func TestRunDispatchesEverySeatBeforeAnyCanFinish(t *testing.T) {
	started := make(chan string, 3)
	release := make(chan struct{})
	dispatcher := &fakeDispatcher{dispatch: func(ctx context.Context, request dispatch.Request) (dispatch.Response, error) {
		started <- request.Persona
		select {
		case <-release:
			return response(request.Persona + " position"), nil
		case <-ctx.Done():
			return dispatch.Response{}, ctx.Err()
		}
	}}
	result := make(chan Result, 1)
	errs := make(chan error, 1)
	go func() {
		meeting, err := Run(context.Background(), dispatcher, testRequest())
		result <- meeting
		errs <- err
	}()
	for range 3 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("Run() did not start every seat concurrently")
		}
	}
	close(release)
	if err := <-errs; err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if meeting := <-result; meeting.Arbiter == nil {
		t.Fatalf("Run() = %#v, want arbiter reading", meeting)
	}
}

func TestRunKeepsArbiterAndParticipantInstancesSeparate(t *testing.T) {
	dispatcher := &fakeDispatcher{dispatch: func(_ context.Context, request dispatch.Request) (dispatch.Response, error) {
		if request.Persona == "chair" && strings.Contains(request.Prompt, "The participant positions:") {
			return response("arbiter reading"), nil
		}
		return response(request.Persona + " position"), nil
	}}
	request := Request{
		Question: "question",
		Seats: []Seat{
			{Persona: "chair", Route: route("codex", "outside-tier")},
			{Persona: "other", Route: route("claude", "opus")},
		},
		Arbiter: Seat{Persona: "chair", Route: route("grok", "judge")},
		Tier:    "review",
		CWD:     "/tmp",
		Timeout: time.Second,
	}
	_, err := Run(context.Background(), dispatcher, request)
	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	requests := dispatcher.Requests()
	participant := requestFor(requests, "chair", "question")
	if participant.Route == nil || participant.Route.Harness != "codex" || strings.Contains(participant.Prompt, "other position") {
		t.Fatalf("chair participant request = %#v, want a separate seat context", participant)
	}
	arbiter := requestFor(requests, "chair", "The participant positions:")
	if arbiter.Route == nil || arbiter.Route.Harness != "grok" || !strings.Contains(arbiter.Prompt, "chair position") || !strings.Contains(arbiter.Prompt, "other position") {
		t.Fatalf("chair arbiter request = %#v, want a separate arbiter context", arbiter)
	}
}

func TestRunReportsFailedSeatAndProceedsWithTwoAnswers(t *testing.T) {
	dispatcher := &fakeDispatcher{dispatch: func(_ context.Context, request dispatch.Request) (dispatch.Response, error) {
		if request.Persona == "observer" {
			return dispatch.Response{}, errors.New("participant harness failed")
		}
		if strings.Contains(request.Prompt, "The participant positions:") {
			return response("arbiter reading"), nil
		}
		return response(request.Persona + " position"), nil
	}}
	result, err := Run(context.Background(), dispatcher, testRequest())
	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if result.Arbiter == nil || result.Arbiter.Response == nil || len(result.Failures) != 1 || result.Failures[0].Seat.Persona != "observer" {
		t.Fatalf("Run() = %#v, want a visible failed seat and arbiter reading", result)
	}
}

func TestRunRefusesBeforeDispatchingAnySeatWhenPreflightFails(t *testing.T) {
	dispatcher := &fakeDispatcher{
		dispatch: func(_ context.Context, request dispatch.Request) (dispatch.Response, error) {
			return response(request.Persona + " position"), nil
		},
		validate: func(_ context.Context, candidate config.Route) error {
			if candidate.String() == "codex/invalid" {
				return errors.New("provider rejected the model")
			}
			return nil
		},
	}
	request := testRequest()
	request.Seats[0].Route = route("codex", "invalid")
	_, err := Run(context.Background(), dispatcher, request)
	if err == nil || !strings.Contains(err.Error(), "codex/invalid") {
		t.Fatalf("Run() error = %v, want invalid route evidence", err)
	}
	if got := len(dispatcher.Requests()); got != 0 {
		t.Fatalf("Run() dispatched %d requests after preflight failed, want 0", got)
	}
}

func TestValidateCountsSeatsRatherThanDistinctPersonas(t *testing.T) {
	seats := []Seat{
		{Persona: "one", Route: route("codex", "sol")},
		{Persona: "one", Route: route("claude", "opus")},
	}
	if err := Validate(seats, Seat{Persona: "chair", Route: route("grok", "judge")}); err != nil {
		t.Fatalf("Validate() returned unexpected error: %v", err)
	}
}

func testRequest() Request {
	return Request{
		Question: "question",
		Seats: []Seat{
			{Persona: "optimist", Route: route("codex", "sol")},
			{Persona: "skeptic", Route: route("claude", "opus")},
			{Persona: "observer", Route: route("grok", "fast")},
		},
		Arbiter: Seat{Persona: "chair", Route: route("claude", "judge")},
		Tier:    "review",
		CWD:     "/tmp",
	}
}

func route(harnessID string, model string) config.Route {
	return config.Route{Harness: harnessID, Model: &model}
}

type fakeDispatcher struct {
	mu       sync.Mutex
	requests []dispatch.Request
	dispatch func(context.Context, dispatch.Request) (dispatch.Response, error)
	validate func(context.Context, config.Route) error
}

func (fake *fakeDispatcher) Dispatch(ctx context.Context, request dispatch.Request) (dispatch.Response, error) {
	fake.mu.Lock()
	fake.requests = append(fake.requests, request)
	fake.mu.Unlock()

	return fake.dispatch(ctx, request)
}

func (fake *fakeDispatcher) ValidateRoute(ctx context.Context, candidate config.Route) error {
	if fake.validate == nil {
		return nil
	}

	return fake.validate(ctx, candidate)
}

func (fake *fakeDispatcher) Requests() []dispatch.Request {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return append([]dispatch.Request(nil), fake.requests...)
}

func response(answer string) dispatch.Response {
	return dispatch.Response{Projection: jobs.Projection{Status: harness.JobStatusDone, Output: &answer}}
}

func requestFor(requests []dispatch.Request, persona string, promptPart string) dispatch.Request {
	for _, request := range requests {
		if request.Persona == persona && strings.Contains(request.Prompt, promptPart) {
			return request
		}
	}

	return dispatch.Request{}
}
