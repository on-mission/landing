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

func TestRunKeepsParticipantContextsIsolatedAndPassesPositionsVerbatimToArbiter(t *testing.T) {
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
	for _, answer := range []string{"optimist position", "skeptic position", "observer position"} {
		if !strings.Contains(arbiter.Prompt, answer) {
			t.Fatalf("arbiter prompt = %q, missing participant position %q", arbiter.Prompt, answer)
		}
	}
}

func TestRunDispatchesEveryParticipantBeforeAnyCanFinish(t *testing.T) {
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
			t.Fatal("Run() did not start every participant concurrently")
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

func TestRunKeepsArbiterParticipantInstancesSeparate(t *testing.T) {
	dispatcher := &fakeDispatcher{dispatch: func(_ context.Context, request dispatch.Request) (dispatch.Response, error) {
		if request.Persona == "chair" && strings.Contains(request.Prompt, "The participant positions:") {
			return response("arbiter reading"), nil
		}
		return response(request.Persona + " position"), nil
	}}
	model := "outside-tier"
	request := Request{Question: "question", Personas: []string{"chair", "other"}, Arbiter: "chair", Casts: map[string]config.Route{"chair": {Harness: "codex", Model: &model}}, Tier: "review", CWD: "/tmp", Timeout: time.Second}
	_, err := Run(context.Background(), dispatcher, request)
	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	requests := dispatcher.Requests()
	participant := requestFor(requests, "chair", "question")
	if participant.Route == nil || participant.Route.Harness != "codex" || strings.Contains(participant.Prompt, "other position") {
		t.Fatalf("chair participant request = %#v, want a separate cast participant context", participant)
	}
	arbiter := requestFor(requests, "chair", "The participant positions:")
	if arbiter.Route != nil || !strings.Contains(arbiter.Prompt, "chair position") || !strings.Contains(arbiter.Prompt, "other position") {
		t.Fatalf("chair arbiter request = %#v, want an uncast reading of every position", arbiter)
	}
}

func TestRunDoesNotAskForMachineReadableOutput(t *testing.T) {
	dispatcher := &fakeDispatcher{dispatch: func(_ context.Context, request dispatch.Request) (dispatch.Response, error) {
		return response(request.Persona + " response"), nil
	}}
	_, err := Run(context.Background(), dispatcher, testRequest())
	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	for _, request := range dispatcher.Requests() {
		if strings.Contains(strings.ToLower(request.Prompt), "json") || strings.Contains(strings.ToLower(request.Prompt), "machine-readable") {
			t.Fatalf("prompt for %q asks for a structured format: %q", request.Persona, request.Prompt)
		}
	}
}

func TestRunReportsParticipantAndArbiterFailures(t *testing.T) {
	dispatcher := &fakeDispatcher{dispatch: func(_ context.Context, request dispatch.Request) (dispatch.Response, error) {
		if request.Persona == "observer" {
			return dispatch.Response{}, errors.New("participant harness failed")
		}
		if strings.Contains(request.Prompt, "The participant positions:") {
			return dispatch.Response{}, errors.New("arbiter harness failed")
		}
		return response(request.Persona + " position"), nil
	}}
	result, err := Run(context.Background(), dispatcher, testRequest())
	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if len(result.Failures) != 2 || result.Arbiter == nil || result.Arbiter.Failure != "arbiter harness failed" {
		t.Fatalf("Run() = %#v, want participant and arbiter failures", result)
	}
}

func TestRunReportsWhenFewerThanTwoParticipantsAnswer(t *testing.T) {
	dispatcher := &fakeDispatcher{dispatch: func(_ context.Context, request dispatch.Request) (dispatch.Response, error) {
		if request.Persona != "optimist" {
			return dispatch.Response{}, errors.New("participant harness failed")
		}
		return response("only answer"), nil
	}}
	result, err := Run(context.Background(), dispatcher, testRequest())
	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if result.Arbiter != nil || len(result.Failures) != 3 || result.Failures[2].Stage != "meeting" {
		t.Fatalf("Run() = %#v, want no arbiter and a nothing-to-deliberate failure", result)
	}
}

func TestValidateRejectsUsageStates(t *testing.T) {
	tests := map[string]struct {
		personas []string
		arbiter  string
		want     string
	}{
		"missing arbiter":       {personas: []string{"one", "two"}, want: "has no arbiter"},
		"one participant":       {personas: []string{"one"}, arbiter: "chair", want: "at least two participants"},
		"duplicate participant": {personas: []string{"one", "one"}, arbiter: "chair", want: `persona "one" more than once`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := Validate(test.personas, test.arbiter)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate(%q, %q) error = %v, want %q", test.personas, test.arbiter, err, test.want)
			}
		})
	}
}

func testRequest() Request {
	return Request{Question: "question", Personas: []string{"optimist", "skeptic", "observer"}, Arbiter: "chair", Tier: "review", CWD: "/tmp", Timeout: time.Second}
}

type fakeDispatcher struct {
	mu       sync.Mutex
	requests []dispatch.Request
	dispatch func(context.Context, dispatch.Request) (dispatch.Response, error)
}

func (fake *fakeDispatcher) Dispatch(ctx context.Context, request dispatch.Request) (dispatch.Response, error) {
	fake.mu.Lock()
	fake.requests = append(fake.requests, request)
	fake.mu.Unlock()

	return fake.dispatch(ctx, request)
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
