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
	"github.com/on-mission/landing/internal/router"
)

func TestRunMeetingUsageErrorsHaveUsageExitCode(t *testing.T) {
	directory := t.TempDir()
	tests := map[string]struct {
		args []string
		want string
	}{
		"missing arbiter":       {args: []string{"meeting", "--persona", "optimist", "--persona", "skeptic", "question"}, want: "has no arbiter"},
		"one participant":       {args: []string{"meeting", "--persona", "optimist", "--arbiter", "chair", "question"}, want: "at least two participants"},
		"duplicate participant": {args: []string{"meeting", "--persona", "optimist", "--persona", "optimist", "--arbiter", "chair", "question"}, want: `persona "optimist" more than once`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			code, err, _, _ := runCLI(t, directory, test.args)
			if code != exitUsage || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run(%q) = %d, %v, want %d and text %q", test.args, code, err, exitUsage, test.want)
			}
		})
	}
}

func TestReportMeetingReturnsEveryPositionWhenArbiterFails(t *testing.T) {
	one := "one answer"
	two := "two answer"
	result := meeting.Result{
		Question:     "question",
		Participants: []string{"one", "two"},
		Positions: []meeting.Position{
			{Persona: "one", Response: doneResponse(one, "codex")},
			{Persona: "two", Response: doneResponse(two, "claude")},
		},
		Arbiter:  &meeting.Reading{Failure: "harness failed"},
		Failures: []meeting.Failure{{Stage: "arbiter", Persona: "chair", Error: "harness failed"}},
	}
	output := &bytes.Buffer{}
	code, err := reportMeeting(result, false, output)
	if err != nil || code != exitFailed {
		t.Fatalf("reportMeeting() = %d, %v, want %d, nil", code, err, exitFailed)
	}
	for _, want := range []string{"## one\none answer", "## two\ntwo answer", "participant: one (harness: codex, model: unpinned, not cast)", "arbiter chair: harness failed"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("reportMeeting() output = %q, want %q", output.String(), want)
		}
	}
}

func TestReportMeetingJSONReconstructsRound(t *testing.T) {
	answer := "optimist position"
	reading := "arbiter reading"
	result := meeting.Result{
		Question:     "question",
		Participants: []string{"optimist", "skeptic"},
		Casts:        map[string]config.Route{"optimist": {Harness: "codex"}},
		Positions: []meeting.Position{
			{Persona: "optimist", Response: doneResponse(answer, "codex")},
			{Persona: "skeptic", Failure: "harness failed"},
		},
		Arbiter:  &meeting.Reading{Response: doneResponse(reading, "claude")},
		Failures: []meeting.Failure{{Stage: "participant", Persona: "skeptic", Error: "harness failed"}},
	}
	output := &bytes.Buffer{}
	code, err := reportMeeting(result, true, output)
	if err != nil || code != exitFailed {
		t.Fatalf("reportMeeting() = %d, %v, want %d, nil", code, err, exitFailed)
	}
	var reported meetingResult
	if err := json.Unmarshal(output.Bytes(), &reported); err != nil {
		t.Fatalf("reportMeeting() JSON could not be decoded: %v", err)
	}
	if len(reported.Positions) != 2 || reported.Positions[0].Answer == nil || *reported.Positions[0].Answer != answer || !reported.Positions[0].Cast || reported.Arbiter == nil || reported.Arbiter.Answer == nil || *reported.Arbiter.Answer != reading || len(reported.Failures) != 1 {
		t.Fatalf("reportMeeting() JSON = %#v, want reconstructible positions, arbiter reading, and failures", reported)
	}
}

func TestParseArgumentsKeepsOneRoundMeetingSurface(t *testing.T) {
	values, positionals, err := parseArguments([]string{"meeting", "--persona", "one", "--persona", "two", "--arbiter", "chair", "--cast", "one=codex/gpt-5.6-terra", "question"})
	if err != nil {
		t.Fatalf("parseArguments() returned unexpected error: %v", err)
	}
	if strings.Join(values.Personas, ",") != "one,two" || values.Arbiter.Value != "chair" || len(values.Casts) != 1 || values.Casts[0].Route.Model == nil || *values.Casts[0].Route.Model != "gpt-5.6-terra" || strings.Join(positionals, ",") != "meeting,question" {
		t.Fatalf("parseArguments() = %#v, %q, want one-round meeting options", values, positionals)
	}
	removedOption := "--min" + "-rounds"
	if _, _, err := parseArguments([]string{"meeting", removedOption, "2", "question"}); err == nil {
		t.Fatal("parseArguments() accepted a removed meeting option")
	}
}

func TestMeetingHelpCarriesTheProtocol(t *testing.T) {
	code, err, stdout, _ := runCLI(t, t.TempDir(), []string{"meeting", "--help"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(meeting --help) = %d, %v, want %d, nil", code, err, exitOK)
	}
	for _, want := range []string{"one deliberation round", "in parallel", "decides whether to convene another", "verbatim", "synthesis from the arbiter as ordinary work"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("meeting help = %q, want %q", stdout.String(), want)
		}
	}
}

func TestMeetingCastsAcceptOutsideTierAndValidateModel(t *testing.T) {
	registry := router.NewMapRegistry(map[string]harness.Adapter{"remote": castAdapter{models: []string{"outside-tier"}}})
	casts, err := meetingCasts(context.Background(), []castOption{{Persona: "one", Route: routeOption{Harness: "remote", Model: modelPointer("outside-tier")}}}, []string{"one", "two"}, registry)
	if err != nil || casts["one"].Harness != "remote" {
		t.Fatalf("meetingCasts() = %#v, %v, want cast outside tier", casts, err)
	}
	_, err = meetingCasts(context.Background(), []castOption{{Persona: "one", Route: routeOption{Harness: "remote", Model: modelPointer("missing")}}}, []string{"one", "two"}, registry)
	if err != nil {
		t.Fatalf("meetingCasts() error = %v, want adapter validation", err)
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

func modelPointer(value string) *string { return &value }

type castAdapter struct {
	models []string
}

func (adapter castAdapter) ID() string { return "remote" }

func (adapter castAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{Models: append([]string(nil), adapter.models...), Authority: harness.ModelCatalogAuthoritative}
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
