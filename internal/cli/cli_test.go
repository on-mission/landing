package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/persona"
)

func TestResultJSONIncludesPersonaDeliveryOnlyWhenApplied(t *testing.T) {
	delivery := harness.PersonaDeliveryAppendSystemPrompt
	withPersona, err := json.Marshal(result{PersonaDelivery: &delivery})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withPersona), `"personaDelivery":"append-system-prompt"`) {
		t.Fatalf("JSON result = %s, want persona delivery", withPersona)
	}
	withoutPersona, err := json.Marshal(result{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(withoutPersona), "personaDelivery") {
		t.Fatalf("JSON result = %s, want no persona delivery", withoutPersona)
	}
}

func TestManagementCommand(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		forced    bool
		want      command
		wantMatch bool
	}{
		{name: "bare config shows resolved policy", args: []string{"config"}, want: commandConfigShow, wantMatch: true},
		{name: "config init", args: []string{"config", "init"}, want: commandConfigInit, wantMatch: true},
		{name: "config show", args: []string{"config", "show"}, want: commandConfigShow, wantMatch: true},
		{name: "tier list", args: []string{"tier", "list"}, want: commandTierList, wantMatch: true},
		{name: "tier add", args: []string{"tier", "add"}, want: commandTierAdd, wantMatch: true},
		{name: "tier update", args: []string{"tier", "update"}, want: commandTierUpdate, wantMatch: true},
		{name: "tier remove", args: []string{"tier", "remove"}, want: commandTierRemove, wantMatch: true},
		{name: "harness list", args: []string{"harness", "list"}, want: commandHarnessList, wantMatch: true},
		{name: "persona list", args: []string{"persona", "list"}, want: commandPersonaList, wantMatch: true},
		{name: "persona show", args: []string{"persona", "show"}, want: commandPersonaShow, wantMatch: true},
		{name: "persona add", args: []string{"persona", "add"}, want: commandPersonaAdd, wantMatch: true},
		{name: "persona update", args: []string{"persona", "update"}, want: commandPersonaUpdate, wantMatch: true},
		{name: "persona remove", args: []string{"persona", "remove"}, want: commandPersonaRemove, wantMatch: true},
		{name: "unmatched input is a prompt", args: []string{"config", "custom"}},
		{name: "command with extra input is a prompt", args: []string{"tier", "list", "please"}},
		{name: "terminator forces prompt", args: []string{"config"}, forced: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := managementCommand(test.args, test.forced)
			if ok != test.wantMatch || got != test.want {
				t.Fatalf("managementCommand(%q, %t) = %q, %t; want %q, %t", test.args, test.forced, got, ok, test.want, test.wantMatch)
			}
		})
	}
}

func TestParseArgumentsTerminatesFlagsAndAttachesFallback(t *testing.T) {
	values, positionals, err := parseArguments([]string{"tier", "add", "--route", "codex/gpt-5.6-terra", "--fallback-below", "20", "--route", "cline", "--max-wait", "2s", "--", "literal", "--tier"})
	if err != nil {
		t.Fatalf("parseArguments() returned unexpected error: %v", err)
	}
	if !values.ForcedPrompt || !values.MaxWait.Set || values.MaxWait.Value != "2s" || len(values.Routes) != 2 || values.Routes[0].FallbackBelowPercent == nil || *values.Routes[0].FallbackBelowPercent != 20 || values.Routes[1].Model != nil {
		t.Fatalf("parseArguments() routes = %#v, max wait = %#v, forced = %t; want two routes with fallback attached to codex", values.Routes, values.MaxWait, values.ForcedPrompt)
	}
	if got := strings.Join(positionals, " "); got != "tier add literal --tier" {
		t.Fatalf("parseArguments() positionals = %q, want %q", got, "tier add literal --tier")
	}
}

func TestParseArgumentsRejectsRemovedCommsWaitOption(t *testing.T) {
	if _, _, err := parseArguments([]string{"comms", "--wait", "2s"}); err == nil {
		t.Fatal("parseArguments() accepted removed --wait option")
	}
}

func TestSelectTier(t *testing.T) {
	configuration := config.Config{DefaultTier: "default", Tiers: map[string]config.Tier{"default": {Name: "default"}, "other": {Name: "other"}}}
	tests := []struct {
		name     string
		supplied parsedOption
		reply    bool
		config   config.Config
		want     string
		wantErr  string
	}{
		{name: "uses configured default", config: configuration, want: "default"},
		{name: "named tier wins", supplied: parsedOption{Value: "other", Set: true}, config: configuration, want: "other"},
		{name: "missing default lists tiers", config: config.Config{Tiers: configuration.Tiers}, wantErr: "configured tiers are default, other"},
		{name: "reply has no tier", reply: true, config: configuration},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectTier(test.supplied, test.reply, test.config)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("selectTier() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("selectTier() = %q, %v; want %q, nil", got, err, test.want)
			}
		})
	}
}

func TestConfigInitReportsThatItCannotYetResolve(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	code, err, stdout, _ := runCLI(t, directory, []string{"config", "init"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(config init) = %d, %v; want %d, nil", code, err, exitOK)
	}
	want := "tiers: none; configuration file names no tiers, and dispatch resolves only when at least one exists\n"
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("Run(config init) stdout = %q, want it to state %q", stdout.String(), want)
	}
	if strings.Contains(stdout.String(), "add a tier") || strings.Contains(stdout.String(), "run ") {
		t.Fatalf("Run(config init) stdout = %q, want a description, not an instruction", stdout.String())
	}
}

func TestRunManagementCommands(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	code, err, stdout, _ := runCLI(t, directory, []string{"config", "init"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(config init) = %d, %v; want %d, nil", code, err, exitOK)
	}
	if !strings.Contains(stdout.String(), "configuration:") {
		t.Fatalf("Run(config init) stdout = %q, want configuration path", stdout.String())
	}
	configurationPath := filepath.Join(directory, config.ConfigFileName)
	if _, err := os.Stat(configurationPath); err != nil {
		t.Fatalf("Stat(%q) after config init returned unexpected error: %v", configurationPath, err)
	}
	code, err, _, _ = runCLI(t, directory, []string{"config", "init"})
	if code != exitUsage || err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Run(config init) after configuration exists = %d, %v; want %d, configuration already exists error", code, err, exitUsage)
	}

	code, err, _, _ = runCLI(t, directory, []string{"tier", "add", "--name", "research", "--description", "Reads widely.", "--route", "codex/gpt-5.6-terra", "--default"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(tier add) = %d, %v; want %d, nil", code, err, exitOK)
	}

	for _, args := range [][]string{{"config"}, {"config", "show"}, {"tier", "list"}} {
		code, err, stdout, _ = runCLI(t, directory, args)
		if err != nil || code != exitOK {
			t.Fatalf("Run(%q) = %d, %v; want %d, nil", args, code, err, exitOK)
		}
		if !strings.Contains(stdout.String(), "research") {
			t.Fatalf("Run(%q) stdout = %q, want configured tiers", args, stdout.String())
		}
	}
	code, err, stdout, _ = runCLI(t, directory, []string{"tier", "update", "--name", "research", "--description", "Reads primary sources.", "--route", "codex/gpt-5.6-terra"})
	if err != nil || code != exitOK || !strings.Contains(stdout.String(), "update tier: research") {
		t.Fatalf("Run(tier update) = %d, %v, %q; want updated tier", code, err, stdout.String())
	}
	code, err, _, _ = runCLI(t, directory, []string{"tier", "add", "--name", "backup", "--route", "codex/gpt-5.6-terra"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(tier add backup) = %d, %v; want %d, nil", code, err, exitOK)
	}
	code, err, stdout, _ = runCLI(t, directory, []string{"tier", "remove", "--name", "research"})
	if err != nil || code != exitOK || !strings.Contains(stdout.String(), "removed tier: research") {
		t.Fatalf("Run(tier remove) = %d, %v, %q; want removed tier", code, err, stdout.String())
	}
}

func TestTierUpdateAppliesOnlySuppliedFields(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	writeTestConfiguration(t, directory, `{"version":1,"defaultTier":"engineer","tiers":{"engineer":{"routes":[{"harness":"codex","model":"gpt-5.6-terra"}]}}}`)
	load := func(t *testing.T) config.Config {
		t.Helper()
		configuration, err := config.Load(context.Background(), directory, config.NewHarnesses(newRegistry()))
		if err != nil {
			t.Fatalf("config.Load() returned unexpected error: %v", err)
		}

		return configuration
	}
	before := load(t)
	original := before.Tiers["engineer"]

	code, err, _, _ := runCLI(t, directory, []string{"tier", "update", "--name", "engineer", "--default"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(tier update --default) = %d, %v; want %d, nil", code, err, exitOK)
	}
	afterDefault := load(t)
	if afterDefault.DefaultTier != "engineer" || afterDefault.Tiers["engineer"].Description != original.Description || !equalRoutes(afterDefault.Tiers["engineer"].Routes, original.Routes) {
		t.Fatalf("tier update --default changed tier policy: %#v", afterDefault)
	}

	code, err, _, _ = runCLI(t, directory, []string{"tier", "update", "--name", "engineer", "--description", "Small, focused changes."})
	if err != nil || code != exitOK {
		t.Fatalf("Run(tier update --description) = %d, %v; want %d, nil", code, err, exitOK)
	}
	afterDescription := load(t)
	if afterDescription.Tiers["engineer"].Description != "Small, focused changes." || !equalRoutes(afterDescription.Tiers["engineer"].Routes, original.Routes) {
		t.Fatalf("tier update --description changed routes: %#v", afterDescription.Tiers["engineer"])
	}

	code, err, _, _ = runCLI(t, directory, []string{"tier", "update", "--name", "engineer", "--route", "codex/gpt-5.6-terra"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(tier update --route) = %d, %v; want %d, nil", code, err, exitOK)
	}
	afterRoutes := load(t)
	model := "gpt-5.6-terra"
	wantRoutes := []config.Route{{Harness: "codex", Model: &model}}
	if afterRoutes.Tiers["engineer"].Description != "Small, focused changes." || !equalRoutes(afterRoutes.Tiers["engineer"].Routes, wantRoutes) {
		t.Fatalf("tier update --route did not replace routes wholesale: %#v", afterRoutes.Tiers["engineer"])
	}

	path := filepath.Join(directory, config.ConfigFileName)
	beforeNoop, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) returned unexpected error: %v", path, readErr)
	}
	code, err, stdout, _ := runCLI(t, directory, []string{"tier", "update", "--name", "engineer"})
	if err != nil || code != exitOK || stdout.String() != "tier update: no fields supplied; configuration unchanged\n" {
		t.Fatalf("Run(tier update with no fields) = %d, %v, %q; want %d, nil, no-op message", code, err, stdout.String(), exitOK)
	}
	afterNoop, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) after no-op returned unexpected error: %v", path, readErr)
	}
	if string(afterNoop) != string(beforeNoop) {
		t.Fatalf("tier update with no fields rewrote %q", path)
	}

	code, err, _, _ = runCLI(t, directory, []string{"tier", "update", "--name", "nosuch", "--default"})
	if code != exitUsage || err == nil || !strings.Contains(err.Error(), `tier "nosuch" does not exist; configured tiers are "engineer"`) {
		t.Fatalf("Run(tier update missing --default) = %d, %v; want missing tier names", code, err)
	}
}

func TestTierAddNamesSupportedModels(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	if code, err, _, _ := runCLI(t, directory, []string{"config", "init"}); err != nil || code != exitOK {
		t.Fatalf("Run(config init) = %d, %v; want %d, nil", code, err, exitOK)
	}
	code, err, _, _ := runCLI(t, directory, []string{"tier", "add", "--name", "bad", "--route", "codex/gpt-9"})
	if code != exitUsage {
		t.Fatalf("Run(tier add unsupported model) code = %d, want %d", code, exitUsage)
	}
	if err == nil || !strings.Contains(err.Error(), "supported models are") || !strings.Contains(err.Error(), "gpt-5.6-terra") {
		t.Fatalf("Run(tier add unsupported model) error = %v, want supported codex models", err)
	}
}

func TestHarnessReportDistinguishesDetectionStates(t *testing.T) {
	reports := []harnessReport{
		{Name: "absent", Status: harness.DetectionAbsent, Capacity: capacityReport{}},
		{Name: "unauthenticated", Status: harness.DetectionUnauthenticated, Capacity: capacityReport{}, Detail: "authentication failed"},
		{Name: "unreadable", Status: harness.DetectionUnreadable, Capacity: capacityReport{}, Detail: "capacity response was unreadable"},
		{Name: "ready", Status: harness.DetectionReady, Capacity: capacityReport{Known: true}},
	}
	output := &bytes.Buffer{}
	if code, err := writeHarnessReports(reports, output); err != nil || code != exitOK {
		t.Fatalf("writeHarnessReports() = %d, %v; want %d, nil", code, err, exitOK)
	}
	for _, status := range []string{"absent", "unauthenticated", "unreadable", "ready"} {
		if !strings.Contains(output.String(), status) {
			t.Fatalf("writeHarnessReports() output = %q, want status %q", output.String(), status)
		}
	}
	if strings.Contains(output.String(), "unreadable: unauthenticated") {
		t.Fatalf("writeHarnessReports() output = %q, unreadable was rendered as unauthenticated", output.String())
	}
}

func TestHelpVersionAndErrorCodes(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		code, err, _, _ := runCLI(t, directory, args)
		if err != nil || code != exitOK {
			t.Fatalf("Run(%q) = %d, %v; want %d, nil", args, code, err, exitOK)
		}
	}
	for _, test := range []struct {
		err  error
		want int
	}{
		{err: &usageError{message: "invalid input"}, want: exitUsage},
		{err: harness.NewError(harness.ErrorCodeConfigNotFound, "no configuration", nil), want: exitUsage},
		{err: harness.NewError(harness.ErrorCodeNoProviderAvailable, "no route", nil), want: exitFailed},
	} {
		if got := ReportError(&bytes.Buffer{}, test.err); got != test.want {
			t.Fatalf("ReportError(%v) = %d, want %d", test.err, got, test.want)
		}
	}
}

func TestSubcommandHelpIsSpecificToTheCommand(t *testing.T) {
	tests := map[string]struct {
		args []string
		want string
	}{
		"config init":   {args: []string{"config", "init", "--help"}, want: "landing config init — create a project configuration"},
		"tier add":      {args: []string{"tier", "add", "--help"}, want: "landing tier add — add a configured execution tier"},
		"persona add":   {args: []string{"persona", "add", "--help"}, want: "--instructions-file <path>"},
		"comms install": {args: []string{"comms", "--install", "--help"}, want: "landing comms — exchange messages"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			code, err, stdout, _ := runCLI(t, t.TempDir(), test.args)
			if err != nil || code != exitOK || !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("Run(%q) = %d, %v, %q; want %d, nil, text %q", test.args, code, err, stdout.String(), exitOK, test.want)
			}
			if strings.Contains(stdout.String(), "INPUT\n  Landing first matches") {
				t.Fatalf("Run(%q) returned top-level help: %q", test.args, stdout.String())
			}
		})
	}
}

func TestPromptPrecedenceAndTimeoutDefaults(t *testing.T) {
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(promptFile, []byte("from file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt, err := resolvePrompt(parsedOption{Value: promptFile, Set: true}, []string{"-"}, strings.NewReader("from stdin"), false)
	if err != nil || prompt != "from file" {
		t.Fatalf("resolvePrompt() = %q, %v; want file prompt", prompt, err)
	}
	if timeout, err := resolveTimeout(false, parsedOption{}); err != nil || timeout != defaultDispatchTimeout {
		t.Fatalf("resolveTimeout(dispatch) = %v, %v; want %v", timeout, err, defaultDispatchTimeout)
	}
	if timeout, err := resolveTimeout(true, parsedOption{}); err != nil || timeout != defaultReplyTimeout {
		t.Fatalf("resolveTimeout(reply) = %v, %v; want %v", timeout, err, defaultReplyTimeout)
	}
}

func equalRoutes(got []config.Route, want []config.Route) bool {
	if len(got) != len(want) {
		return false
	}
	for index, route := range got {
		if route.Harness != want[index].Harness || !equalStrings(route.Model, want[index].Model) || !equalFloats(route.FallbackBelowPercent, want[index].FallbackBelowPercent) {
			return false
		}
	}

	return true
}

func equalStrings(got *string, want *string) bool {
	if got == nil || want == nil {
		return got == want
	}

	return *got == *want
}

func equalFloats(got *float64, want *float64) bool {
	if got == nil || want == nil {
		return got == want
	}

	return *got == *want
}

func runCLI(t *testing.T, invocationDir string, args []string) (int, error, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code, err := Run(context.Background(), Inputs{Args: args, InvocationDir: invocationDir, Stdin: strings.NewReader(""), Stdout: stdout, Stderr: stderr})

	return code, err, stdout, stderr
}

func TestRunPromptRoutesToDefaultBeforeDispatch(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	writeTestConfiguration(t, directory, `{"version":1,"defaultTier":"review","tiers":{"review":{"routes":[{"harness":"codex","model":"gpt-5.6-terra"}]}}}`)
	missingCWD := filepath.Join(directory, "missing")
	code, err, _, _ := runCLI(t, directory, []string{"ordinary prompt", "--cwd", missingCWD})
	if code != exitFailed {
		t.Fatalf("Run(default prompt) code = %d, want %d", code, exitFailed)
	}
	var harnessError *harness.Error
	if !errors.As(err, &harnessError) || harnessError.Code != harness.ErrorCodeInvalidCWD {
		t.Fatalf("Run(default prompt) error = %T %v, want INVALID_CWD before dispatch", err, err)
	}
}

func TestRunPersonaListUsesInvocationDirectory(t *testing.T) {
	root := t.TempDir()
	personas := filepath.Join(root, ".landing", "personas")
	invocationDir := filepath.Join(root, "work", "task")
	if err := os.MkdirAll(personas, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(invocationDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"backend", "frontend"} {
		directory := filepath.Join(personas, name)
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "PERSONA.md"), []byte("---\n---\ninstructions"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	code, err, stdout, _ := runCLI(t, invocationDir, []string{"persona", "list"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(persona list) = %d, %v; want %d, nil", code, err, exitOK)
	}
	names := strings.Fields(stdout.String())
	want := []string{"backend", "frontend"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("Run(persona list) names = %q, want %q", names, want)
	}
	for _, name := range names {
		if _, err := persona.Resolve(context.Background(), name, invocationDir); err != nil {
			t.Fatalf("Resolve(%q) returned unexpected error: %v", name, err)
		}
	}
}

func writeTestConfiguration(t *testing.T, directory string, contents string) {
	t.Helper()
	path := filepath.Join(directory, config.ConfigFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned unexpected error: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile() returned unexpected error: %v", err)
	}
}
