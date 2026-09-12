package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/on-mission/landing/internal/harness"
)

func TestClaudeBuildRequestsCarryPermissionMode(t *testing.T) {
	adapter := &claudeAdapter{}
	model := "claude-sonnet-5"
	tests := []struct {
		name  string
		build func() (harness.Request, error)
		want  []string
	}{
		{
			name: "start", build: func() (harness.Request, error) {
				return adapter.BuildStart(harness.StartParams{Prompt: "work", Model: &model})
			},
			want: []string{"-p", "work", "--output-format", "json", "--model", model, "--permission-mode", claudePermissionMode},
		},
		{
			name: "resume", build: func() (harness.Request, error) {
				return adapter.BuildResume(harness.ResumeParams{ThreadID: "thread", Prompt: "work", Model: &model})
			},
			want: []string{"-p", "work", "--output-format", "json", "--model", model, "--permission-mode", claudePermissionMode, "--resume", "thread"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := test.build()
			if err != nil || request.Command != "claude" || !reflect.DeepEqual(request.Args, test.want) {
				t.Fatalf("request = %#v, %v; want claude %#v", request, err, test.want)
			}
		})
	}
}

func TestGrokBuildRequestsCarryPermissionMode(t *testing.T) {
	adapter := &grokAdapter{command: "grok"}
	model := "grok-4.6"
	tests := []struct {
		name  string
		build func() (harness.Request, error)
		want  []string
	}{
		{
			name: "start", build: func() (harness.Request, error) {
				return adapter.BuildStart(harness.StartParams{Prompt: "work", Model: &model, CWD: "/repo"})
			},
			want: []string{"-p", "work", "--output-format", "json", "--cwd", "/repo", "--permission-mode", "bypassPermissions", "-m", model},
		},
		{
			name: "resume", build: func() (harness.Request, error) {
				return adapter.BuildResume(harness.ResumeParams{ThreadID: "thread", Prompt: "work", CWD: "/repo"})
			},
			want: []string{"-r", "thread", "-p", "work", "--output-format", "json", "--cwd", "/repo", "--permission-mode", "bypassPermissions"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := test.build()
			if err != nil || request.Command != "grok" || !reflect.DeepEqual(request.Args, test.want) {
				t.Fatalf("request = %#v, %v; want grok %#v", request, err, test.want)
			}
		})
	}
}

func TestClaudeCapacityProbeCarriesCommsProvenance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a POSIX shell script to fake the claude executable")
	}
	directory := t.TempDir()
	observed := filepath.Join(directory, "probe-environment")
	command := filepath.Join(directory, "claude")
	script := "#!/bin/sh\n" +
		"printf '%s' \"$LANDING_COMMS_PROBE\" > \"" + observed + "\"\n" +
		"read ignored\n" +
		"printf '%s\\n' '{\"type\":\"control_response\",\"response\":{\"request_id\":\"r1\",\"response\":{\"rate_limits\":{\"five_hour\":{\"utilization\":12,\"resets_at\":\"2026-08-18T20:00:00Z\"}}}}}'\n"
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))

	capacity, err := requestClaudeUsage(context.Background())
	if err != nil || !capacity.IsKnown() {
		t.Fatalf("requestClaudeUsage() = %#v, %v; want known capacity", capacity, err)
	}
	value, err := os.ReadFile(observed)
	if err != nil || string(value) != "1" {
		t.Fatalf("Claude probe %s = %q, %v; want %q", "LANDING_COMMS_PROBE", value, err, "1")
	}
}

func TestPersonaDeliveryUsesAdapterOwnedMechanisms(t *testing.T) {
	persona := &harness.Persona{
		Instructions:   "You are a pirate. Always begin with Arr.",
		Directory:      "/project/.landing/personas/pirate",
		ReferenceFiles: []string{"seamap.md"},
	}
	model := "claude-haiku-4-5-20251001"
	tests := []struct {
		name     string
		build    func() (harness.Request, error)
		delivery harness.PersonaDelivery
		prompt   string
	}{
		{
			name: "claude start uses appended system prompt",
			build: func() (harness.Request, error) {
				return (&claudeAdapter{}).BuildStart(harness.StartParams{Prompt: "What color is the sky?", Model: &model, Persona: persona})
			},
			delivery: harness.PersonaDeliveryAppendSystemPrompt,
			prompt:   "What color is the sky?",
		},
		{
			name: "claude resume uses appended system prompt",
			build: func() (harness.Request, error) {
				return (&claudeAdapter{}).BuildResume(harness.ResumeParams{ThreadID: "thread", Prompt: "What color is the sky?", Model: &model, Persona: persona})
			},
			delivery: harness.PersonaDeliveryAppendSystemPrompt,
			prompt:   "What color is the sky?",
		},
		{
			name: "codex start composes prompt",
			build: func() (harness.Request, error) {
				return (&codexAdapter{}).BuildStart(harness.StartParams{JobID: "job", Prompt: "What color is the sky?", Persona: persona})
			},
			delivery: harness.PersonaDeliveryPromptComposition,
			prompt:   "What color is the sky?",
		},
		{
			name: "codex resume composes prompt",
			build: func() (harness.Request, error) {
				return (&codexAdapter{}).BuildResume(harness.ResumeParams{JobID: "job", ThreadID: "thread", Prompt: "What color is the sky?", Persona: persona})
			},
			delivery: harness.PersonaDeliveryPromptComposition,
			prompt:   "What color is the sky?",
		},
		{
			name: "grok start composes prompt",
			build: func() (harness.Request, error) {
				return (&grokAdapter{command: "grok"}).BuildStart(harness.StartParams{Prompt: "What color is the sky?", Persona: persona})
			},
			delivery: harness.PersonaDeliveryPromptComposition,
			prompt:   "What color is the sky?",
		},
		{
			name: "grok resume composes prompt",
			build: func() (harness.Request, error) {
				return (&grokAdapter{command: "grok"}).BuildResume(harness.ResumeParams{ThreadID: "thread", Prompt: "What color is the sky?", Persona: persona})
			},
			delivery: harness.PersonaDeliveryPromptComposition,
			prompt:   "What color is the sky?",
		},
		{
			name: "cline start composes prompt",
			build: func() (harness.Request, error) {
				return (&clineAdapter{command: "cline"}).BuildStart(harness.StartParams{Prompt: "What color is the sky?", Persona: persona})
			},
			delivery: harness.PersonaDeliveryPromptComposition,
			prompt:   "What color is the sky?",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := test.build()
			if err != nil {
				t.Fatal(err)
			}
			if request.PersonaDelivery == nil || *request.PersonaDelivery != test.delivery {
				t.Fatalf("persona delivery = %v, want %q", request.PersonaDelivery, test.delivery)
			}
			if test.delivery == harness.PersonaDeliveryAppendSystemPrompt {
				assertPersonaText(t, argumentAfter(request.Args, "--append-system-prompt"), "")
				return
			}
			prompt := request.Args[len(request.Args)-1]
			if request.Command == "grok" {
				prompt = argumentAfter(request.Args, "-p")
			}
			assertPersonaText(t, prompt, test.prompt)
		})
	}
}

func argumentAfter(args []string, flag string) string {
	for index, arg := range args {
		if arg == flag && index+1 < len(args) {
			return args[index+1]
		}
	}

	return ""
}

func assertPersonaText(t *testing.T, text string, work string) {
	t.Helper()
	for _, want := range []string{"You are a pirate. Always begin with Arr.", "/project/.landing/personas/pirate", "seamap.md"} {
		if !strings.Contains(text, want) {
			t.Fatalf("persona text = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "Persona reference") {
		t.Fatalf("persona text = %q, must not frame the perspective as labelled metadata", text)
	}
	if work != "" && !strings.HasSuffix(text, work) {
		t.Fatalf("persona text = %q, want it to end with work %q", text, work)
	}
}

func TestCodexBuildStartAndThreadCapture(t *testing.T) {
	adapter := &codexAdapter{}
	model := "gpt-5.3-codex-spark"
	effort := "low"
	sandbox := "read-only"
	request, err := adapter.BuildStart(harness.StartParams{JobID: "job", Prompt: "work", Model: &model, ReasoningEffort: &effort, Sandbox: &sandbox, CWD: "/repo", ExtraConfig: []string{"foo=bar"}})
	if err != nil {
		t.Fatal(err)
	}
	if request.Command != "codex" || !reflect.DeepEqual(request.Args[:7], []string{"exec", "--model", model, "-c", "reasoning_effort=low", "--skip-git-repo-check", "--output-last-message"}) || !strings.Contains(strings.Join(request.Args, " "), "-s read-only -C /repo work") {
		t.Fatalf("codex request args = %#v", request.Args)
	}
	record := &harness.JobRecord{}
	adapter.OnStdoutLine(`{"type":"thread.started","thread_id":"thread-1"}`, record)
	if record.ThreadID == nil || *record.ThreadID != "thread-1" {
		t.Fatalf("thread capture = %#v, want thread-1", record.ThreadID)
	}
}

func TestClineThreadCaptureIgnoresNonStartEvents(t *testing.T) {
	adapter := &clineAdapter{}
	record := &harness.JobRecord{}
	adapter.OnStdoutLine(`{"type":"run_result","taskId":"wrong"}`, record)
	if record.ThreadID != nil {
		t.Fatalf("non-start event captured %q", *record.ThreadID)
	}
	adapter.OnStdoutLine(`{"type":"hook_event","hookEventName":"agent_start","taskId":"cline-1"}`, record)
	if record.ThreadID == nil || *record.ThreadID != "cline-1" {
		t.Fatalf("start event capture = %#v, want cline-1", record.ThreadID)
	}
}

func TestAdapterSpawnPathsContainExistingDirectories(t *testing.T) {
	clineCommand := filepath.Join(t.TempDir(), "cline")
	tests := []struct {
		name    string
		adapter harness.Adapter
		wantDir string
	}{
		{name: "claude", adapter: NewClaude()},
		{name: "codex", adapter: NewCodex()},
		{name: "grok", adapter: NewGrok()},
		{name: "cline", adapter: &clineAdapter{command: clineCommand}, wantDir: filepath.Dir(clineCommand)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.adapter.SpawnPath()
			if path == test.adapter.ID() {
				t.Fatalf("%s SpawnPath() = %q, want a PATH value", test.adapter.ID(), path)
			}
			if test.wantDir != "" && !slices.Contains(strings.Split(path, string(os.PathListSeparator)), test.wantDir) {
				t.Fatalf("%s SpawnPath() = %q, want resolved executable directory %q", test.adapter.ID(), path, test.wantDir)
			}
			for _, directory := range strings.Split(path, string(os.PathListSeparator)) {
				info, err := os.Stat(directory)
				if err == nil && info.IsDir() {
					return
				}
			}
			t.Fatalf("%s SpawnPath() = %q, want at least one existing directory", test.adapter.ID(), path)
		})
	}
}

func TestGrokInitializeRequestCarriesProtocolVersion(t *testing.T) {
	encoded, err := json.Marshal(grokInitializeRequest())
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Method string `json:"method"`
		Params struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"params"`
	}
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	if request.Method != "initialize" || request.Params.ProtocolVersion != grokProtocolVersion {
		t.Fatalf("grok initialize request = %s, want protocolVersion %q", encoded, grokProtocolVersion)
	}
}

func TestFailureDetectorsClassifyKnownOutput(t *testing.T) {
	tests := []struct {
		name string
		got  bool
	}{
		{name: "codex exhaustion", got: codexExhaustionMessage("You've hit your usage limit")},
		{name: "claude exhaustion", got: claudeExhaustionMessage("rate limit reached")},
		{name: "grok exhaustion", got: grokExhausted("out of credits")},
		{name: "codex authentication", got: codexAuthenticationFailure("not signed in; run codex --login", "")},
		{name: "claude authentication", got: claudeAuthenticationFailure("not authenticated; claude login", "")},
		{name: "grok authentication", got: grokAuthenticationFailure("Not signed in. Run grok login", "")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.got {
				t.Fatal("detector did not classify its documented failure signal")
			}
		})
	}
}

func TestCapacityBucketParsers(t *testing.T) {
	reset := time.Date(2026, time.August, 8, 13, 0, 0, 0, time.UTC)
	codexPayload, err := os.ReadFile(filepath.Join("testdata", "codex-rate-limits.json"))
	if err != nil {
		t.Fatal(err)
	}
	codex := codexCapacityFromResult(codexPayload)
	if !codex.IsKnown() || len(codex.Buckets()) != 2 {
		t.Fatalf("codex capacity = %#v, want two buckets", codex)
	}
	claudePayload, err := os.ReadFile(filepath.Join("testdata", "claude-rate-limits.json"))
	if err != nil {
		t.Fatal(err)
	}
	claude := claudeCapacityFromResponse(claudePayload)
	claudeBuckets := claude.Buckets()
	if !claude.IsKnown() || len(claudeBuckets) != 2 || !hasBucketResetAt(claudeBuckets, reset) {
		t.Fatalf("claude capacity = %#v, want parsed buckets resetting at %s", claude, reset)
	}
}

// This catches a provider schema migration turning an uncapped subscription into fabricated 0% usage or an unreadable harness.
func TestGrokUnifiedBillingWithoutOnDemandCapHasNoGauge(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("testdata", "grok-billing-unified.json"))
	if err != nil {
		t.Fatal(err)
	}

	capacity, detail := grokCapacityFromBilling(payload)
	if capacity.IsKnown() || capacity.HasGauge() {
		t.Fatalf("grokCapacityFromBilling() = %#v, %q; want a healthy no-gauge capacity", capacity, detail)
	}
	if !strings.Contains(detail, "subscription capacity has no gauge") {
		t.Fatalf("grokCapacityFromBilling() detail = %q, want the absent gauge explained", detail)
	}
}

// This catches ratio, reset, and legacy compatibility regressions at the Grok billing boundary.
func TestGrokBillingComputesOnlyMeasuredUsage(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		usedPercent float64
	}{
		{
			name:        "positive on-demand cap",
			payload:     `{"config":{"currentPeriod":{"end":"2026-09-04T00:23:26.632098+00:00"},"onDemandCap":{"val":200},"onDemandUsed":{"val":50},"isUnifiedBillingUser":true}}`,
			usedPercent: 25,
		},
		{
			name:        "legacy credit percentage",
			payload:     `{"config":{"currentPeriod":{"end":"2026-09-04T00:23:26.632098+00:00"},"creditUsagePercent":37.5}}`,
			usedPercent: 37.5,
		},
		{
			name:        "billing period reset fallback",
			payload:     `{"config":{"currentPeriod":{"end":"weekly"},"billingPeriodEnd":"2026-09-04T00:23:26.632098+00:00","creditUsagePercent":"12"}}`,
			usedPercent: 12,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capacity, detail := grokCapacityFromBilling([]byte(test.payload))
			buckets := capacity.Buckets()
			if detail != "" || !capacity.IsKnown() || len(buckets) != 1 || buckets[0].UsedPercent != test.usedPercent {
				t.Fatalf("grokCapacityFromBilling() = %#v, %q; want %.2f%% measured usage", capacity, detail, test.usedPercent)
			}
		})
	}
}

// This catches silent diagnostic loss when a Grok gauge exists but its payload cannot be trusted.
func TestGrokBillingFailuresExplainWhyCapacityIsUnreadable(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		detail  string
	}{
		{
			name:    "missing measurement fields",
			payload: `{"config":{"currentPeriod":{"end":"2026-09-04T00:23:26Z"}}}`,
			detail:  "missing both creditUsagePercent and onDemandCap.val",
		},
		{
			name:    "non-finite measurement",
			payload: `{"config":{"creditUsagePercent":"NaN","currentPeriod":{"end":"2026-09-04T00:23:26Z"}}}`,
			detail:  "creditUsagePercent was non-finite",
		},
		{
			name:    "unparseable reset",
			payload: `{"config":{"creditUsagePercent":12,"currentPeriod":{"end":"next Thursday"}}}`,
			detail:  "was not RFC3339",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capacity, detail := grokCapacityFromBilling([]byte(test.payload))
			if capacity.IsKnown() || !capacity.HasGauge() {
				t.Fatalf("grokCapacityFromBilling() capacity = %#v, want unreadable gauge", capacity)
			}
			if !strings.Contains(detail, test.detail) {
				t.Fatalf("grokCapacityFromBilling() detail = %q, want %q", detail, test.detail)
			}
		})
	}
}

func TestDetectionClassifiesObservedCapacity(t *testing.T) {
	codexPayload, err := os.ReadFile(filepath.Join("testdata", "codex-rate-limits.json"))
	if err != nil {
		t.Fatal(err)
	}
	grokAuthentication, err := os.ReadFile(filepath.Join("testdata", "grok-auth-error.json"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		detect    harness.Detection
		status    harness.DetectionStatus
		wantPath  string
		wantKnown bool
		wantGauge bool
	}{
		{
			name:      "reports absent when no executable resolved",
			detect:    absentDetection(),
			status:    harness.DetectionAbsent,
			wantPath:  "",
			wantGauge: true,
		},
		{
			name:      "reports ready for recorded capacity",
			detect:    capacityDetection("/recorded/bin/codex", codexCapacityFromResult(codexPayload), "", codexAuthenticationFailure),
			status:    harness.DetectionReady,
			wantPath:  "/recorded/bin/codex",
			wantKnown: true,
			wantGauge: true,
		},
		{
			name: "reports unauthenticated when a no gauge harness says so",
			detect: capacityDetection("/recorded/bin/grok", harness.NoCapacityGauge(), "You are not authenticated.", func(output, _ string) bool {
				return grokAuthenticationFromProbe([]byte(output), nil) == grokAuthenticationUnauthenticated
			}),
			status:    harness.DetectionUnauthenticated,
			wantPath:  "/recorded/bin/grok",
			wantGauge: true,
		},
		{
			name: "reports ready when a no gauge harness is authenticated",
			detect: capacityDetection("/recorded/bin/grok", harness.NoCapacityGauge(), "You are logged in with grok.com.", func(output, _ string) bool {
				return grokAuthenticationFromProbe([]byte(output), nil) == grokAuthenticationUnauthenticated
			}),
			status:    harness.DetectionReady,
			wantPath:  "/recorded/bin/grok",
			wantGauge: false,
		},
		{
			name: "reports ready when a no gauge harness is ambiguous",
			detect: capacityDetection("/recorded/bin/grok", harness.NoCapacityGauge(), "Please sign in to continue.", func(output, _ string) bool {
				return grokAuthenticationFromProbe([]byte(output), nil) == grokAuthenticationUnauthenticated
			}),
			status:    harness.DetectionReady,
			wantPath:  "/recorded/bin/grok",
			wantGauge: false,
		},
		{
			name:      "reports an unreadable gauge authentication failure",
			detect:    capacityDetection("/recorded/bin/grok", harness.UnknownCapacity(), string(grokAuthentication), grokAuthenticationFailure),
			status:    harness.DetectionUnauthenticated,
			wantPath:  "/recorded/bin/grok",
			wantGauge: true,
		},
		{
			name:      "reports ready when a readable gauge conflicts with auth output",
			detect:    capacityDetection("/recorded/bin/grok", codexCapacityFromResult(codexPayload), "You are not authenticated.", func(string, string) bool { return true }),
			status:    harness.DetectionReady,
			wantPath:  "/recorded/bin/grok",
			wantKnown: true,
			wantGauge: true,
		},
		{
			name:      "reports unreadable for an empty capacity response",
			detect:    capacityDetection("/recorded/bin/claude", harness.UnknownCapacity(), "", claudeAuthenticationFailure),
			status:    harness.DetectionUnreadable,
			wantPath:  "/recorded/bin/claude",
			wantGauge: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.detect.Status != test.status || test.detect.Path != test.wantPath {
				t.Fatalf("capacityDetection() = %#v, want status %q and path %q", test.detect, test.status, test.wantPath)
			}
			if test.detect.Capacity.IsKnown() != test.wantKnown {
				t.Fatalf("capacityDetection() capacity = %#v, want known=%t", test.detect.Capacity, test.wantKnown)
			}
			if test.detect.Capacity.HasGauge() != test.wantGauge {
				t.Fatalf("capacityDetection() capacity = %#v, want gauge=%t", test.detect.Capacity, test.wantGauge)
			}
		})
	}
}

func TestGrokAuthenticationProbeParsesOnlyRecognizedWords(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   grokAuthenticationStatus
	}{
		{name: "logged in", output: "You are logged in with grok.com.\nAvailable models:", want: grokAuthenticationAuthenticated},
		{name: "not authenticated with zero exit status", output: "You are not authenticated.\n", want: grokAuthenticationUnauthenticated},
		{name: "empty output", want: grokAuthenticationUnknown},
		{name: "unrecognized output", output: "Please sign in to continue.", want: grokAuthenticationUnknown},
		{name: "non-zero exit", output: "You are not authenticated.", err: os.ErrPermission, want: grokAuthenticationUnknown},
		{name: "timeout", output: "You are not authenticated.", err: context.DeadlineExceeded, want: grokAuthenticationUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := grokAuthenticationFromProbe([]byte(test.output), test.err); got != test.want {
				t.Fatalf("grokAuthenticationFromProbe(%q, %v) = %q, want %q", test.output, test.err, got, test.want)
			}
		})
	}
}

func TestGrokAuthenticationProbeCachesResult(t *testing.T) {
	reads := 0
	adapter := &grokAdapter{
		readAuthentication: func(context.Context, string) ([]byte, error) {
			reads++
			return []byte("You are not authenticated.\n"), nil
		},
	}

	if got := adapter.probeGrokAuthentication(context.Background(), "grok"); got != grokAuthenticationUnauthenticated {
		t.Fatalf("first probe = %q, want %q", got, grokAuthenticationUnauthenticated)
	}
	if got := adapter.probeGrokAuthentication(context.Background(), "grok"); got != grokAuthenticationUnauthenticated {
		t.Fatalf("second probe = %q, want %q", got, grokAuthenticationUnauthenticated)
	}
	if reads != 1 {
		t.Fatalf("probe read grok models %d times, want 1 cached read", reads)
	}
}

func TestAdapterModelsAreModuleOwned(t *testing.T) {
	grokOutput, err := os.ReadFile(filepath.Join("testdata", "grok-models.txt"))
	if err != nil {
		t.Fatal(err)
	}
	grok := &grokAdapter{
		command: "grok",
		readModels: func(context.Context, string) ([]byte, error) {
			return grokOutput, nil
		},
	}
	// A module owning its catalog means it is free to add a reachable model
	// without a test objecting. So the literal-catalog adapters assert that the
	// models they must reach are present, not that the list is frozen; only
	// grok, whose catalog is parsed from harness output, is matched exactly,
	// because there the exact list IS the behavior under test.
	tests := []struct {
		name      string
		adapter   harness.Adapter
		contains  []string
		exact     []string
		authority harness.ModelCatalogAuthority
	}{
		{name: "codex advisory catalog", adapter: NewCodex(), contains: []string{"gpt-5.6-terra", "gpt-5.3-codex-spark", "gpt-5.6-luna"}, authority: harness.ModelCatalogAdvisory},
		{name: "claude advisory catalog", adapter: NewClaude(), contains: []string{"claude-sonnet-5", "claude-haiku-4-5-20251001"}, authority: harness.ModelCatalogAdvisory},
		{name: "grok live listing catalog", adapter: grok, exact: []string{"grok-4.6", "grok-4.5"}, authority: harness.ModelCatalogAuthoritative},
		{name: "cline no model catalog", adapter: NewCline(), exact: nil, authority: harness.ModelCatalogAuthoritative},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := test.adapter.ModelCatalog()
			if catalog.Authority != test.authority {
				t.Fatalf("%s.ModelCatalog().Authority = %q, want %q", test.adapter.ID(), catalog.Authority, test.authority)
			}
			got := catalog.Models
			if test.contains != nil {
				for _, model := range test.contains {
					if !slices.Contains(got, model) {
						t.Fatalf("%s.ModelCatalog().Models = %#v, want it to contain %q", test.adapter.ID(), got, model)
					}
				}
				return
			}
			if !slices.Equal(got, test.exact) {
				t.Fatalf("%s.ModelCatalog().Models = %#v, want %#v", test.adapter.ID(), got, test.exact)
			}
		})
	}
}

func TestGrokModelsFallsBackWhenListingFails(t *testing.T) {
	adapter := &grokAdapter{
		command: "grok",
		readModels: func(context.Context, string) ([]byte, error) {
			return nil, os.ErrNotExist
		},
	}

	if got := adapter.ModelCatalog().Models; !slices.Equal(got, []string{"grok-4.6", "grok-4.5"}) {
		t.Fatalf("ModelCatalog().Models = %#v, want fallback models", got)
	}
}

func TestGrokModelsCachesLiveListing(t *testing.T) {
	output, err := os.ReadFile(filepath.Join("testdata", "grok-models.txt"))
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	adapter := &grokAdapter{
		command: "grok",
		readModels: func(context.Context, string) ([]byte, error) {
			reads++
			return output, nil
		},
	}

	adapter.ModelCatalog()
	adapter.ModelCatalog()
	if reads != 1 {
		t.Fatalf("ModelCatalog() read grok models %d times, want 1 cached read", reads)
	}
}

// This catches a locally configured Codex default being hidden from catalog
// output, which would make a usable route needlessly undiscoverable.
func TestCodexModelCatalogIncludesConfiguredModel(t *testing.T) {
	home := t.TempDir()
	// os.UserHomeDir reads HOME on unix and USERPROFILE on Windows, and this
	// suite runs on both. Setting only one leaves the other platform reading
	// the real home directory, where this test's config file does not exist.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configDirectory := filepath.Join(home, ".codex")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "config.toml"), []byte("model = \"gpt-5.6-sol\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog := (&codexAdapter{}).ModelCatalog()
	if !slices.Contains(catalog.Models, "gpt-5.6-sol") {
		t.Fatalf("ModelCatalog().Models = %#v, want configured model", catalog.Models)
	}
}

func hasBucketResetAt(buckets []harness.Bucket, reset time.Time) bool {
	for _, bucket := range buckets {
		if bucket.ResetsAt.Equal(reset) {
			return true
		}
	}

	return false
}

func TestClaudePermissionDenialsFailAnOtherwiseSuccessfulRun(t *testing.T) {
	adapter := &claudeAdapter{}
	result, err := adapter.Finalize(context.Background(), harness.FinalizeParams{ExitCode: 0, Stdout: `{"is_error":false,"result":"blocked","session_id":"thread-1","permission_denials":[{"tool_name":"Bash"}]}`})
	if err != nil || result.Status != harness.JobStatusFailed || result.ThreadID == nil || *result.ThreadID != "thread-1" || !strings.Contains(result.Error, "Bash") {
		t.Fatalf("Finalize() = %#v, %v; want denied failure with captured thread", result, err)
	}
}

// Landing never collects provider credentials: each harness owns its own
// authentication. The message describes that ownership independently of the
// working directory, so another project's credential-sync guidance cannot leak
// into it.
func TestGrokAuthenticationFailureDescribesCommandOwnership(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "grok-auth-error.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		cwd  string
	}{
		{name: "inside a repository", cwd: "/Users/example/project"},
		{name: "anywhere else", cwd: "/tmp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, finalizeErr := (&grokAdapter{}).Finalize(context.Background(), harness.FinalizeParams{
				ExitCode: 1,
				Stdout:   string(fixture),
				Record:   &harness.JobRecord{CWD: test.cwd},
			})
			if finalizeErr != nil || result.Status != harness.JobStatusFailed {
				t.Fatalf("Finalize() = %#v, %v; want a failed status", result, finalizeErr)
			}
			for _, want := range []string{"Grok is not authenticated on this machine.", "Grok authentication is owned by `grok login`."} {
				if !strings.Contains(result.Error, want) {
					t.Fatalf("Finalize() error = %q; want it to contain %q", result.Error, want)
				}
			}
			if strings.Contains(result.Error, "Run `grok login`, then retry.") {
				t.Fatalf("Finalize() error = %q; want no prescribed recovery", result.Error)
			}
			if strings.Contains(result.Error, "miss vm") {
				t.Fatalf("Finalize() error = %q; want no reference to another project's credential sync", result.Error)
			}
		})
	}
}

func TestClineTimeoutArgument(t *testing.T) {
	timeout := 1500 * time.Millisecond
	if got := clineTimeoutArgument(nil); got != "900" {
		t.Fatalf("default timeout = %q, want 900", got)
	}
	if got := clineTimeoutArgument(&timeout); got != "1.5" {
		t.Fatalf("explicit timeout = %q, want 1.5", got)
	}
}
