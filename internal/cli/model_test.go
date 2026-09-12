package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/on-mission/landing/internal/adapters"
	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/router"
)

func TestRunDispatchPinsNamedModelInPlaceOfATier(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	// The pinned model is deliberately absent from every configured tier: a
	// caller who names a route is not choosing among a tier's routes.
	writeTestConfiguration(t, directory, `{"version":1,"defaultTier":"review","tiers":{"review":{"routes":[{"harness":"recording","model":"quick"}]}}}`)
	adapter := &pinnedRouteAdapter{status: harness.DetectionReady, models: []string{"quick", "careful"}}
	code, err := dispatchWithPin(t, directory, adapter, routeOption{Harness: "recording", Model: stringPointer("careful")})
	if code != exitFailed || err == nil || !strings.Contains(err.Error(), "stop before spawn") {
		t.Fatalf("runDispatch(--model) = %d, %v; want the pinned harness reached before spawn", code, err)
	}
	if adapter.params.Model == nil || *adapter.params.Model != "careful" {
		t.Fatalf("runDispatch(--model) model = %v, want the model the caller named", adapter.params.Model)
	}
}

func TestRunDispatchPinsRouteOnAHarnessWithUnreadableCapacity(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	writeTestConfiguration(t, directory, `{"version":1,"defaultTier":"review","tiers":{"review":{"routes":[{"harness":"recording"}]}}}`)
	// A pin never consults capacity, so an unreadable gauge must not disable
	// the route the caller named.
	adapter := &pinnedRouteAdapter{status: harness.DetectionUnreadable, models: []string{"careful"}}
	code, err := dispatchWithPin(t, directory, adapter, routeOption{Harness: "recording", Model: stringPointer("careful")})
	if code != exitFailed || err == nil || !strings.Contains(err.Error(), "stop before spawn") {
		t.Fatalf("runDispatch(--model on unreadable harness) = %d, %v; want the route to run anyway", code, err)
	}
}

// This catches a newly released model being rejected by Landing before its
// advisory harness has a chance to accept it.
func TestRunDispatchDefersUnlistedAdvisoryModelToTheHarness(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	writeTestConfiguration(t, directory, `{"version":1,"defaultTier":"review","tiers":{"review":{"routes":[{"harness":"recording"}]}}}`)
	adapter := &pinnedRouteAdapter{status: harness.DetectionReady, models: []string{"known"}, authority: harness.ModelCatalogAdvisory}
	code, err := dispatchWithPin(t, directory, adapter, routeOption{Harness: "recording", Model: stringPointer("newly-released")})
	if code != exitFailed || err == nil || !strings.Contains(err.Error(), "stop before spawn") {
		t.Fatalf("runDispatch(--model unlisted advisory model) = %d, %v; want the harness reached before spawn", code, err)
	}
	if adapter.params.Model == nil || *adapter.params.Model != "newly-released" {
		t.Fatalf("runDispatch(--model unlisted advisory model) model = %v, want the caller's model", adapter.params.Model)
	}
}

func TestRunDispatchRejectsUnusableOrUnreachablePins(t *testing.T) {
	tests := []struct {
		name    string
		status  harness.DetectionStatus
		models  []string
		route   routeOption
		wantMsg string
	}{
		{name: "harness absent", status: harness.DetectionAbsent, models: []string{"careful"}, route: routeOption{Harness: "recording", Model: stringPointer("careful")}, wantMsg: `observed state "absent"`},
		{name: "harness unauthenticated", status: harness.DetectionUnauthenticated, models: []string{"careful"}, route: routeOption{Harness: "recording", Model: stringPointer("careful")}, wantMsg: `observed state "unauthenticated"`},
		{name: "model unreachable", status: harness.DetectionReady, models: []string{"careful"}, route: routeOption{Harness: "recording", Model: stringPointer("absent")}, wantMsg: `cannot reach; reachable models are "careful"`},
		{name: "harness unsupported", status: harness.DetectionReady, models: []string{"careful"}, route: routeOption{Harness: "elsewhere", Model: stringPointer("careful")}, wantMsg: `unsupported harness "elsewhere"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			writeTestConfiguration(t, directory, `{"version":1,"defaultTier":"review","tiers":{"review":{"routes":[{"harness":"recording"}]}}}`)
			adapter := &pinnedRouteAdapter{status: test.status, models: test.models}
			code, err := dispatchWithPin(t, directory, adapter, test.route)
			var usage *usageError
			if code != exitUsage || !errors.As(err, &usage) || !strings.Contains(usage.message, test.wantMsg) {
				t.Fatalf("runDispatch(--model) = %d, %v; want usage error containing %q", code, err, test.wantMsg)
			}
		})
	}
}

func TestParseModelRouteAcceptsAHarnessWithAnOptionalModel(t *testing.T) {
	tests := []struct {
		value       string
		wantHarness string
		wantModel   *string
		wantValid   bool
	}{
		{value: "cline", wantHarness: "cline", wantValid: true},
		{value: "grok/grok-4.5", wantHarness: "grok", wantModel: stringPointer("grok-4.5"), wantValid: true},
		{value: "cline/", wantValid: false},
		{value: "/model", wantValid: false},
		{value: "a/b/c", wantValid: false},
		{value: "", wantValid: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			route, err := parseModelRoute(test.value)
			if test.wantValid != (err == nil) {
				t.Fatalf("parseModelRoute(%q) error = %v; want valid %t", test.value, err, test.wantValid)
			}
			if test.wantValid && (route.Harness != test.wantHarness || !equalStrings(route.Model, test.wantModel)) {
				t.Fatalf("parseModelRoute(%q) = %#v, want harness %q and model %v", test.value, route, test.wantHarness, test.wantModel)
			}
		})
	}
}

func TestParseModelRouteDirectsQualifiedClineModelsToConfiguration(t *testing.T) {
	_, err := parseModelRoute("cline/cline-pass/deepseek-v4-pro")
	var usage *usageError
	if !errors.As(err, &usage) || !strings.Contains(usage.message, "configure the Cline model in the project policy") {
		t.Fatalf("parseModelRoute() error = %v, want a configuration direction", err)
	}
}

func TestRunDispatchLetsClaudeRejectAMissingPinnedModel(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	writeTestConfiguration(t, directory, `{"version":1,"defaultTier":"review","tiers":{"review":{"routes":[{"harness":"claude","model":"claude-sonnet-5"}]}}}`)
	route, err := parseModelRoute("claude")
	if err != nil {
		t.Fatalf("parseModelRoute(claude) = %v", err)
	}
	adapter := readyAdapter{Adapter: adapters.NewClaude()}
	code, err := runDispatch(
		context.Background(),
		Inputs{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}},
		options{Model: &route},
		[]string{"Review this decision."},
		directory,
		router.NewMapRegistry(map[string]harness.Adapter{"claude": adapter}),
	)
	var usage *usageError
	if code != exitFailed || err == nil || !strings.Contains(err.Error(), "model is required for claude") || errors.As(err, &usage) {
		t.Fatalf("runDispatch(--model claude) = %d, %v; want Claude's missing-model error, not a usage error", code, err)
	}
}

func TestSelectTargetRejectsATierAndAModelTogether(t *testing.T) {
	configuration := config.Config{DefaultTier: "review", Tiers: map[string]config.Tier{"review": {Name: "review"}}}
	registry := router.NewMapRegistry(map[string]harness.Adapter{"recording": &pinnedRouteAdapter{status: harness.DetectionReady, models: []string{"careful"}}})
	pin := routeOption{Harness: "recording", Model: stringPointer("careful")}

	tests := []struct {
		name    string
		values  options
		reply   bool
		wantMsg string
	}{
		{name: "with tier", values: options{Model: &pin, Tier: parsedOption{Value: "review", Set: true}}, wantMsg: "--model is present with --tier"},
		{name: "with reply", values: options{Model: &pin, Reply: parsedOption{Value: "job", Set: true}}, reply: true, wantMsg: "--model is present with --reply"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := selectTarget(context.Background(), test.values, test.reply, configuration, registry)
			var usage *usageError
			if !errors.As(err, &usage) || !strings.Contains(usage.message, test.wantMsg) {
				t.Fatalf("selectTarget() error = %v; want usage error containing %q", err, test.wantMsg)
			}
		})
	}
}

func TestSelectTargetFallsBackToTheDefaultTier(t *testing.T) {
	configuration := config.Config{DefaultTier: "review", Tiers: map[string]config.Tier{"review": {Name: "review"}}}
	registry := router.NewMapRegistry(map[string]harness.Adapter{})
	target, err := selectTarget(context.Background(), options{}, false, configuration, registry)
	if err != nil || target.Route != nil || target.Tier != "review" || target.name() != "review" {
		t.Fatalf("selectTarget(no options) = %#v, %v; want the default tier", target, err)
	}
}

func TestListModelsNamesSupportedAndConfiguredRoutes(t *testing.T) {
	directory := t.TempDir()
	writeTestConfiguration(t, directory, `{"version":1,"defaultTier":"review","tiers":{"review":{"routes":[{"harness":"recording","model":"quick"},{"harness":"silent"}]},"deep":{"routes":[{"harness":"recording","model":"quick"}]}}}`)
	registry := router.NewMapRegistry(map[string]harness.Adapter{
		"recording": &pinnedRouteAdapter{status: harness.DetectionReady, models: []string{"careful", "quick"}},
		"silent":    &pinnedRouteAdapter{status: harness.DetectionReady, capacity: harness.NoCapacityGauge()},
	})
	stdout := &bytes.Buffer{}
	if code, err := listModels(context.Background(), directory, registry, false, stdout); code != exitOK || err != nil {
		t.Fatalf("listModels() = %d, %v; want %d, nil", code, err, exitOK)
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("listModels() wrote %d lines:\n%s\nwant one per route", len(lines), stdout.String())
	}
	// A model no tier configures is still a route a caller may name.
	if !strings.HasPrefix(lines[0], "recording/careful") || !strings.Contains(lines[0], "tiers: none") {
		t.Fatalf("listModels() line 0 = %q, want an unconfigured supported route", lines[0])
	}
	// Tiers are named in lexical order, not configuration order.
	if !strings.HasPrefix(lines[1], "recording/quick") || !strings.Contains(lines[1], "tiers: deep, review") {
		t.Fatalf("listModels() line 1 = %q, want every tier that configures the route", lines[1])
	}
	// A harness with no model is reported as a route a caller can name directly.
	if !strings.HasPrefix(lines[2], "silent") || !strings.Contains(lines[2], "ready") {
		t.Fatalf("listModels() line 2 = %q, want a selectable modelless harness route", lines[2])
	}
}

func TestListModelsReportsRoutesWithoutAResolvableConfiguration(t *testing.T) {
	registry := router.NewMapRegistry(map[string]harness.Adapter{
		"recording": &pinnedRouteAdapter{status: harness.DetectionReady, models: []string{"careful"}},
	})
	stdout := &bytes.Buffer{}
	// Which routes exist is a property of the installed harnesses, so this
	// lookup answers before a project has configured any policy at all.
	if code, err := listModels(context.Background(), t.TempDir(), registry, false, stdout); code != exitOK || err != nil {
		t.Fatalf("listModels(no configuration) = %d, %v; want %d, nil", code, err, exitOK)
	}
	if !strings.Contains(stdout.String(), "recording/careful") || !strings.Contains(stdout.String(), "tiers: none") {
		t.Fatalf("listModels(no configuration) wrote %q, want the supported route with no tier", stdout.String())
	}
}

// This catches catalog output implying that a fixed model list is exhaustive
// when the harness can accept models Landing cannot enumerate.
func TestCatalogOutputMarksAdvisoryModelsAsIncomplete(t *testing.T) {
	reports := []harnessReport{{
		Name:         "recording",
		Status:       harness.DetectionReady,
		Capacity:     capacityReport{Known: true},
		Models:       []string{"known"},
		ModelCatalog: harness.ModelCatalogAdvisory,
	}}
	harnessOutput := &bytes.Buffer{}
	if code, err := writeHarnessReports(reports, harnessOutput); code != exitOK || err != nil {
		t.Fatalf("writeHarnessReports() = %d, %v; want %d, nil", code, err, exitOK)
	}
	if !strings.Contains(harnessOutput.String(), "advisory; may be incomplete") {
		t.Fatalf("writeHarnessReports() output = %q, want advisory catalog warning", harnessOutput.String())
	}

	directory := t.TempDir()
	writeTestConfiguration(t, directory, `{"version":1,"tiers":{"review":{"routes":[{"harness":"recording","model":"newly-released"}]}}}`)
	registry := router.NewMapRegistry(map[string]harness.Adapter{
		"recording": &pinnedRouteAdapter{status: harness.DetectionReady, models: []string{"known"}, authority: harness.ModelCatalogAdvisory},
	})
	modelOutput := &bytes.Buffer{}
	if code, err := listModels(context.Background(), directory, registry, false, modelOutput); code != exitOK || err != nil {
		t.Fatalf("listModels() = %d, %v; want %d, nil", code, err, exitOK)
	}
	if !strings.Contains(modelOutput.String(), "advisory catalog; other models may be available") {
		t.Fatalf("listModels() output = %q, want advisory catalog warning", modelOutput.String())
	}
	if !strings.Contains(modelOutput.String(), "recording/newly-released") {
		t.Fatalf("listModels() output = %q, want configured advisory model", modelOutput.String())
	}
}

func TestListModelsReportsConfiguredClineModel(t *testing.T) {
	directory := t.TempDir()
	writeTestConfiguration(t, directory, `{"version":1,"tiers":{"review":{"routes":[{"harness":"cline","model":"cline-pass/deepseek-v4-pro"}]}}}`)
	registry := router.NewMapRegistry(map[string]harness.Adapter{
		"cline": readyAdapter{Adapter: adapters.NewCline()},
	})
	stdout := &bytes.Buffer{}
	if code, err := listModels(context.Background(), directory, registry, false, stdout); code != exitOK || err != nil {
		t.Fatalf("listModels() = %d, %v; want %d, nil", code, err, exitOK)
	}
	if !strings.Contains(stdout.String(), "cline/cline-pass/deepseek-v4-pro") || !strings.Contains(stdout.String(), "tiers: review") {
		t.Fatalf("listModels() output = %q, want the configured cline model", stdout.String())
	}
}

func dispatchWithPin(t *testing.T, directory string, adapter harness.Adapter, pin routeOption) (int, error) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	return runDispatch(
		context.Background(),
		Inputs{Stdin: strings.NewReader(""), Stdout: stdout, Stderr: stderr},
		options{Model: &pin},
		[]string{"Review this decision."},
		directory,
		router.NewMapRegistry(map[string]harness.Adapter{"recording": adapter}),
	)
}

// pinnedRouteAdapter reports a fixed detection state and records what reached
// the harness, stopping before anything spawns.
type pinnedRouteAdapter struct {
	status    harness.DetectionStatus
	models    []string
	authority harness.ModelCatalogAuthority
	capacity  harness.Capacity
	params    harness.StartParams
}

type readyAdapter struct {
	harness.Adapter
}

func (adapter readyAdapter) Detect(context.Context) harness.Detection {
	return harness.Detection{Status: harness.DetectionReady}
}

func (adapter *pinnedRouteAdapter) ID() string {
	return "recording"
}

func (adapter *pinnedRouteAdapter) ModelCatalog() harness.ModelCatalog {
	authority := adapter.authority
	if authority == "" {
		authority = harness.ModelCatalogAuthoritative
	}

	return harness.ModelCatalog{Models: adapter.models, Authority: authority}
}

func (adapter *pinnedRouteAdapter) Detect(context.Context) harness.Detection {
	return harness.Detection{Status: adapter.status, Capacity: adapter.capacity}
}

func (adapter *pinnedRouteAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{}
}

func (adapter *pinnedRouteAdapter) Validate(params harness.StartParams) error {
	adapter.params = params

	return nil
}

func (adapter *pinnedRouteAdapter) BuildStart(harness.StartParams) (harness.Request, error) {
	return harness.Request{}, errors.New("stop before spawn")
}

func (adapter *pinnedRouteAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return harness.Request{}, errors.New("stop before spawn")
}

func (adapter *pinnedRouteAdapter) OnStdoutLine(string, *harness.JobRecord) {}

func (adapter *pinnedRouteAdapter) Finalize(context.Context, harness.FinalizeParams) (harness.Finalized, error) {
	return harness.Finalized{}, nil
}

func (adapter *pinnedRouteAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return adapter.capacity
}

func (adapter *pinnedRouteAdapter) SpawnPath() string {
	return os.Getenv("PATH")
}
