package modeltarget

import (
	"context"
	"strings"
	"testing"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/router"
)

func TestResolve(t *testing.T) {
	model := func(value string) *string { return &value }
	reserve := 20.0
	configuration := config.Config{
		Latest: map[string]string{"codex": "gpt-6-sol"},
		Tiers: map[string]config.Tier{
			"frontier": {Name: "frontier", Routes: []config.Route{
				{Harness: "codex", Model: model("latest")},
				{Harness: "grok", Model: model("latest")},
				{Harness: "claude", Model: model("claude-opus-5-5")},
				{Harness: "codex", Model: model("gpt-6-astra"), FallbackBelowPercent: &reserve},
			}},
			"shared": {Name: "shared", Routes: []config.Route{{Harness: "codex", Model: model("gpt-6-sol")}}},
		},
	}
	registry := router.NewMapRegistry(map[string]harness.Adapter{
		"claude": targetAdapter{id: "claude", catalog: harness.ModelCatalog{Models: []string{"claude-opus-5-5"}, Aliases: map[string]string{"opus-5.5": "claude-opus-5-5"}, Latest: "claude-opus-5-5"}},
		"codex":  targetAdapter{id: "codex", catalog: harness.ModelCatalog{Models: []string{"gpt-6-sol", "gpt-6-astra"}, Latest: "gpt-6-astra"}},
		"cline":  targetAdapter{id: "cline", catalog: harness.ModelCatalog{}},
		"grok":   targetAdapter{id: "grok", catalog: harness.ModelCatalog{Models: []string{"grok-4.7"}, Latest: "grok-4.7"}},
	})
	resolver := New(configuration, registry)
	tests := map[string]struct {
		target     string
		want       []string
		wantSource []LatestSource
		wantError  string
	}{
		"accepts an explicit unknown model":                 {target: "codex/new-model", want: []string{"codex/new-model"}},
		"accepts a slash inside a model":                    {target: "cline/cline-pass/deepseek-v4-pro", want: []string{"cline/cline-pass/deepseek-v4-pro"}},
		"infers one known model":                            {target: "grok-4.7", want: []string{"grok/grok-4.7"}},
		"infers short name aliases":                         {target: "opus-5.5", want: []string{"claude/claude-opus-5-5"}},
		"resolves one harness latest with project override": {target: "codex/latest", want: []string{"codex/gpt-6-sol"}, wantSource: []LatestSource{LatestProjectOverride}},
		"resolves all latest with defaults":                 {target: "latest", want: []string{"claude/claude-opus-5-5", "codex/gpt-6-sol", "grok/grok-4.7"}, wantSource: []LatestSource{LatestLandingDefault, LatestProjectOverride, LatestLandingDefault}},
		"resolves tier primary routes and latest":           {target: "frontier", want: []string{"codex/gpt-6-sol", "grok/grok-4.7", "claude/claude-opus-5-5"}, wantSource: []LatestSource{LatestProjectOverride, LatestLandingDefault, ""}},
		"deduplicates comma lists":                          {target: "codex/latest,codex/gpt-6-sol", want: []string{"codex/gpt-6-sol"}, wantSource: []LatestSource{LatestProjectOverride}},
		"rejects a bare unknown model":                      {target: "new-model", wantError: "unknown bare model \"new-model\"; name it as harness/new-model"},
		"rejects a tier and model namespace collision":      {target: "shared", wantError: "model target \"shared\" is ambiguous: it names tier \"shared\" and model \"shared\""},
	}
	configuration.Tiers["shared"] = config.Tier{Name: "shared", Routes: []config.Route{{Harness: "codex", Model: model("gpt-6-sol")}}}
	registry = router.NewMapRegistry(map[string]harness.Adapter{
		"claude": targetAdapter{id: "claude", catalog: harness.ModelCatalog{Models: []string{"claude-opus-5-5"}, Aliases: map[string]string{"opus-5.5": "claude-opus-5-5"}, Latest: "claude-opus-5-5"}},
		"codex":  targetAdapter{id: "codex", catalog: harness.ModelCatalog{Models: []string{"gpt-6-sol", "gpt-6-astra", "shared"}, Latest: "gpt-6-astra"}},
		"cline":  targetAdapter{id: "cline", catalog: harness.ModelCatalog{}},
		"grok":   targetAdapter{id: "grok", catalog: harness.ModelCatalog{Models: []string{"grok-4.7"}, Latest: "grok-4.7"}},
	})
	resolver = New(configuration, registry)
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolver.Resolve(test.target)
			if test.wantError != "" {
				if err == nil || err.Error() != test.wantError {
					t.Fatalf("Resolve(%q) error = %v, want %q", test.target, err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%q) returned unexpected error: %v", test.target, err)
			}
			gotRoutes := make([]string, 0, len(got))
			gotSources := make([]LatestSource, 0, len(got))
			for _, resolution := range got {
				gotRoutes = append(gotRoutes, resolution.Route.String())
				gotSources = append(gotSources, resolution.LatestSource)
			}
			if strings.Join(gotRoutes, ",") != strings.Join(test.want, ",") {
				t.Fatalf("Resolve(%q) routes = %q, want %q", test.target, gotRoutes, test.want)
			}
			if strings.Join(sourceStrings(gotSources), ",") != strings.Join(sourceStrings(test.wantSource), ",") {
				t.Fatalf("Resolve(%q) sources = %q, want %q", test.target, gotSources, test.wantSource)
			}
		})
	}
}

func sourceStrings(sources []LatestSource) []string {
	values := make([]string, 0, len(sources))
	for _, source := range sources {
		values = append(values, string(source))
	}
	return values
}

type targetAdapter struct {
	id      string
	catalog harness.ModelCatalog
}

func (adapter targetAdapter) ID() string                         { return adapter.id }
func (adapter targetAdapter) ModelCatalog() harness.ModelCatalog { return adapter.catalog }
func (adapter targetAdapter) ValidateModel(context.Context, string) harness.ModelValidation {
	return harness.ValidModel("test validation")
}
func (adapter targetAdapter) Detect(context.Context) harness.Detection { return harness.Detection{} }
func (adapter targetAdapter) Capabilities() harness.Capabilities       { return harness.Capabilities{} }
func (adapter targetAdapter) Validate(harness.StartParams) error       { return nil }
func (adapter targetAdapter) BuildStart(harness.StartParams) (harness.Request, error) {
	return harness.Request{}, nil
}
func (adapter targetAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return harness.Request{}, nil
}
func (adapter targetAdapter) OnStdoutLine(string, *harness.JobRecord) {}
func (adapter targetAdapter) Finalize(context.Context, harness.FinalizeParams) (harness.Finalized, error) {
	return harness.Finalized{}, nil
}
func (adapter targetAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return harness.UnknownCapacity()
}
func (adapter targetAdapter) SpawnPath() string { return "" }
