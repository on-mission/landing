package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/dispatch"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
	"github.com/on-mission/landing/internal/meeting"
	"github.com/on-mission/landing/internal/modeltarget"
	"github.com/on-mission/landing/internal/router"
)

func TestMeetingSeatsAllowsSeveralSeatsForOnePersonaAndDeduplicatesRoutes(t *testing.T) {
	configuration := config.Config{Tiers: map[string]config.Tier{"review": {Name: "review"}}}
	registry := router.NewMapRegistry(map[string]harness.Adapter{
		"codex":  castAdapter{models: []string{"sol"}},
		"claude": castAdapter{models: []string{"opus"}},
		"grok":   castAdapter{models: []string{"judge"}},
	})
	values := options{
		Personas: []string{"other"},
		Arbiter:  parsedOption{Value: "chair=grok/judge", Set: true},
		Casts: []castOption{
			{Persona: "one", Route: routeOption{Target: "codex/sol,claude/opus"}},
			{Persona: "one", Route: routeOption{Target: "codex/sol"}},
		},
	}
	seats, arbiter, err := meetingSeats(context.Background(), values, configuration, registry, "review", func(context.Context, string) (config.Route, error) {
		return config.Route{Harness: "codex", Model: stringPointer("tier")}, nil
	})
	if err != nil {
		t.Fatalf("meetingSeats() returned unexpected error: %v", err)
	}
	if len(seats) != 3 || seats[0].Persona != "other" || seats[1].Persona != "one" || seats[2].Persona != "one" || seats[1].Route.String() != "codex/sol" || seats[2].Route.String() != "claude/opus" {
		t.Fatalf("meetingSeats() = %#v, want tier seat plus two deduplicated seats for cast-only persona", seats)
	}
	if arbiter.Persona != "chair" || arbiter.Route.String() != "grok/judge" {
		t.Fatalf("meetingSeats() arbiter = %#v, want pinned arbiter", arbiter)
	}
}

func TestMeetingArbiterRejectsMultiRouteTarget(t *testing.T) {
	configuration := config.Config{Latest: map[string]string{"codex": "sol", "claude": "opus"}, Tiers: map[string]config.Tier{"review": {Name: "review"}}}
	registry := router.NewMapRegistry(map[string]harness.Adapter{
		"codex":  castAdapter{models: []string{"sol"}, latest: "sol"},
		"claude": castAdapter{models: []string{"opus"}, latest: "opus"},
	})
	_, err := meetingArbiter(context.Background(), "chair=latest", modelResolver(configuration, registry), func(context.Context, string) (config.Route, error) {
		return config.Route{}, nil
	}, "review")
	if err == nil || !strings.Contains(err.Error(), "resolves to several routes") {
		t.Fatalf("meetingArbiter() error = %v, want multi-route usage error", err)
	}
}

func TestReportMeetingCarriesSeatResolutionAndFailures(t *testing.T) {
	answer := "one answer"
	reading := "arbiter reading"
	result := meeting.Result{
		Question: "question",
		Positions: []meeting.Position{
			{Seat: meeting.Seat{Persona: "one", Route: testRoute("codex", "sol"), Target: "codex/latest", LatestSource: "project override"}, Response: doneResponse(answer, "codex")},
			{Seat: meeting.Seat{Persona: "one", Route: testRoute("claude", "opus"), Target: "claude/latest", LatestSource: "landing default"}, Failure: "harness failed"},
		},
		Arbiter:  &meeting.Reading{Seat: meeting.Seat{Persona: "chair", Route: testRoute("grok", "judge")}, Response: doneResponse(reading, "grok")},
		Failures: []meeting.Failure{{Stage: "participant", Seat: meeting.Seat{Persona: "one", Route: testRoute("claude", "opus")}, Error: "harness failed"}},
	}
	output := &bytes.Buffer{}
	code, err := reportMeeting(result, false, output)
	if err != nil || code != exitFailed {
		t.Fatalf("reportMeeting() = %d, %v, want %d, nil", code, err, exitFailed)
	}
	for _, want := range []string{"seat: one  codex/sol (latest; project override)", "## one · codex/sol\none answer", "## arbiter · chair · grok/judge\narbiter reading", "participant one · claude/opus: harness failed"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("reportMeeting() output = %q, want %q", output.String(), want)
		}
	}
	jsonOutput := &bytes.Buffer{}
	code, err = reportMeeting(result, true, jsonOutput)
	if err != nil || code != exitFailed {
		t.Fatalf("reportMeeting(JSON) = %d, %v, want %d, nil", code, err, exitFailed)
	}
	var reported meetingResult
	if err := json.Unmarshal(jsonOutput.Bytes(), &reported); err != nil {
		t.Fatalf("reportMeeting(JSON) could not decode: %v", err)
	}
	if len(reported.Positions) != 2 || reported.Positions[0].Target != "codex/latest" || reported.Positions[0].LatestSource != "project override" || reported.Positions[1].Error != "harness failed" || reported.Arbiter == nil || reported.Arbiter.Persona != "chair" {
		t.Fatalf("reportMeeting(JSON) = %#v, want per-seat route, target, latest source, and status", reported)
	}
}

func TestParseArgumentsKeepsMeetingTargets(t *testing.T) {
	values, positionals, err := parseArguments([]string{"meeting", "--arbiter", "chair=claude/latest", "--cast", "one=latest,codex/sol", "question"})
	if err != nil {
		t.Fatalf("parseArguments() returned unexpected error: %v", err)
	}
	if values.Arbiter.Value != "chair=claude/latest" || len(values.Casts) != 1 || values.Casts[0].Route.Target != "latest,codex/sol" || strings.Join(positionals, ",") != "meeting,question" {
		t.Fatalf("parseArguments() = %#v, %q, want meeting target values", values, positionals)
	}
}

func TestMeetingHelpCarriesSeatSyntax(t *testing.T) {
	code, err, stdout, _ := runCLI(t, t.TempDir(), []string{"meeting", "--help"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(meeting --help) = %d, %v, want %d, nil", code, err, exitOK)
	}
	for _, want := range []string{"persona>[=<target>]", "participant seats", "may not\nresolve to several routes", "validates every seat"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("meeting help = %q, want %q", stdout.String(), want)
		}
	}
}

func TestMeetingCommandIsExplicit(t *testing.T) {
	if !meetingCommand([]string{"meeting", "question"}, false) {
		t.Fatal("meetingCommand() = false, want explicit meeting command")
	}
	if meetingCommand([]string{"meeting", "question"}, true) {
		t.Fatal("meetingCommand() = true for forced prompt, want false")
	}
}

func doneResponse(answer string, provider string) *dispatch.Response {
	return &dispatch.Response{Projection: jobs.Projection{Status: harness.JobStatusDone, Output: &answer, Provider: provider}}
}

func testRoute(harnessID string, model string) config.Route {
	return config.Route{Harness: harnessID, Model: stringPointer(model)}
}

func modelResolver(configuration config.Config, registry router.Registry) modeltarget.Resolver {
	return modeltarget.New(configuration, registry)
}

type castAdapter struct {
	models []string
	latest string
}

func (adapter castAdapter) ID() string { return "remote" }

func (adapter castAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{Models: append([]string(nil), adapter.models...), Latest: adapter.latest, Authority: harness.ModelCatalogAuthoritative}
}

func (adapter castAdapter) ValidateModel(context.Context, string) harness.ModelValidation {
	return harness.ValidModel("test validation")
}

func (adapter castAdapter) Detect(context.Context) harness.Detection {
	return harness.Detection{Status: harness.DetectionReady, Capacity: harness.UnknownCapacity()}
}

func (adapter castAdapter) Capabilities() harness.Capabilities { return harness.Capabilities{} }

func (adapter castAdapter) Validate(harness.StartParams) error { return nil }

func (adapter castAdapter) BuildStart(harness.StartParams) (harness.Request, error) {
	return harness.Request{}, nil
}

func (adapter castAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return harness.Request{}, nil
}

func (adapter castAdapter) OnStdoutLine(string, *harness.JobRecord) {}

func (adapter castAdapter) Finalize(context.Context, harness.FinalizeParams) (harness.Finalized, error) {
	return harness.Finalized{}, nil
}

func (adapter castAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return harness.UnknownCapacity()
}

func (adapter castAdapter) SpawnPath() string { return "" }
