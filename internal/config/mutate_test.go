package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/on-mission/landing/internal/paths"
)

func TestMutations(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		mutate      func(context.Context, string, Harnesses) error
		wantTiers   []string
		wantDefault string
		verify      func(*testing.T, Config)
	}{
		{
			name:    "adds the first tier to an initialized configuration",
			fixture: "empty-tiers.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return AddTier(ctx, path, harnesses, Tier{
					Name:        "reviewer",
					Description: "Review completed changes.",
					Routes:      []Route{{Harness: "codex"}},
				})
			},
			wantTiers: []string{"reviewer"},
		},
		{
			name:    "updates an existing tier",
			fixture: "default-tier.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				description := "Review completed changes for release risk."
				_, err := UpdateTier(ctx, path, harnesses, TierUpdate{
					Name:          "reviewer",
					Description:   &description,
					Routes:        []Route{{Harness: "claude", Model: testStringPointer("claude-sonnet-5")}},
					ReplaceRoutes: true,
				})

				return err
			},
			wantTiers:   []string{"reviewer"},
			wantDefault: "reviewer",
			verify: func(t *testing.T, configuration Config) {
				t.Helper()
				tier := configuration.Tiers["reviewer"]
				if tier.Description != "Review completed changes for release risk." {
					t.Fatalf("Load().Tiers[\"reviewer\"].Description = %q, want updated description", tier.Description)
				}
				if len(tier.Routes) != 1 || tier.Routes[0].Harness != "claude" {
					t.Fatalf("Load().Tiers[\"reviewer\"].Routes = %#v, want one claude route", tier.Routes)
				}
			},
		},
		{
			name:    "removes an existing tier",
			fixture: "multiple-tiers.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return RemoveTier(ctx, path, harnesses, "editor")
			},
			wantTiers:   []string{"reviewer"},
			wantDefault: "reviewer",
		},
		{
			name:    "adds a tier to a configured policy",
			fixture: "added-tier.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return AddTier(ctx, path, harnesses, Tier{Name: "editor", Routes: []Route{{Harness: "codex"}}})
			},
			wantTiers: []string{"editor", "reviewer"},
		},
		{
			name:    "sets a configured default tier",
			fixture: "multiple-tiers.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return SetDefaultTier(ctx, path, harnesses, "editor")
			},
			wantTiers:   []string{"editor", "reviewer"},
			wantDefault: "editor",
		},
		{
			name:    "clears a configured default tier",
			fixture: "multiple-tiers.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return SetDefaultTier(ctx, path, harnesses, "")
			},
			wantTiers: []string{"editor", "reviewer"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ConfigFileName)
			writeFixture(t, test.fixture, path)

			if err := test.mutate(context.Background(), path, testHarnesses); err != nil {
				t.Fatalf("configuration mutation returned unexpected error: %v", err)
			}

			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%q) returned unexpected error: %v", path, err)
			}
			assertWrittenJSONShape(t, contents)

			got, err := Load(context.Background(), root, testHarnesses)
			if err != nil {
				t.Fatalf("Load() after configuration mutation returned unexpected error: %v", err)
			}
			if !slices.Equal(got.TierNames(), test.wantTiers) {
				t.Fatalf("Load().TierNames() after configuration mutation = %q, want %q", got.TierNames(), test.wantTiers)
			}
			if got.DefaultTier != test.wantDefault {
				t.Fatalf("Load().DefaultTier after configuration mutation = %q, want %q", got.DefaultTier, test.wantDefault)
			}
			if test.verify != nil {
				test.verify(t, got)
			}
		})
	}
}

func TestMutationsRejectInvalidChangesWithoutWriting(t *testing.T) {
	tests := []struct {
		name      string
		fixture   string
		mutate    func(context.Context, string, Harnesses) error
		wantError string
	}{
		{
			name:    "rejects adding an existing tier",
			fixture: "default-tier.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return AddTier(ctx, path, harnesses, Tier{Name: "reviewer", Routes: []Route{{Harness: "codex"}}})
			},
			wantError: "tier \"reviewer\" already exists; configured tiers are \"reviewer\"",
		},
		{
			name:    "leaves a file with no tiers key unchanged",
			fixture: "absent-tiers.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return AddTier(ctx, path, harnesses, Tier{Name: "reviewer", Routes: []Route{{Harness: "codex"}}})
			},
			wantError: "configuration file %s is invalid: field \"tiers\" is absent; configuration file names no tiers",
		},
		{
			name:    "rejects updating a missing tier",
			fixture: "default-tier.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				_, err := UpdateTier(ctx, path, harnesses, TierUpdate{Name: "missing", Routes: []Route{{Harness: "codex"}}, ReplaceRoutes: true})

				return err
			},
			wantError: "tier \"missing\" does not exist; configured tiers are \"reviewer\"",
		},
		{
			name:    "rejects removing a missing tier",
			fixture: "default-tier.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return RemoveTier(ctx, path, harnesses, "missing")
			},
			wantError: "tier \"missing\" does not exist; configured tiers are \"reviewer\"",
		},
		{
			name:    "keeps the file unchanged when removing the final tier",
			fixture: "one-tier.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return RemoveTier(ctx, path, harnesses, "reviewer")
			},
			wantError: "configuration file %s is invalid: field \"tiers\" is empty; configuration file names no tiers",
		},
		{
			name:    "keeps the file unchanged when the default would be missing",
			fixture: "default-tier.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return SetDefaultTier(ctx, path, harnesses, "missing")
			},
			wantError: "configuration file %s is invalid: field \"defaultTier\" has value \"missing\"; configured tiers are \"reviewer\"",
		},
		{
			name:    "keeps the file unchanged when a route harness is unsupported",
			fixture: "default-tier.json",
			mutate: func(ctx context.Context, path string, harnesses Harnesses) error {
				return AddTier(ctx, path, harnesses, Tier{Name: "writer", Routes: []Route{{Harness: "unknown"}}})
			},
			wantError: "configuration file %s is invalid: tier \"writer\" route 0 field \"harness\" has unsupported value \"unknown\"",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ConfigFileName)
			writeFixture(t, test.fixture, path)

			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%q) before configuration mutation returned unexpected error: %v", path, err)
			}
			err = test.mutate(context.Background(), path, testHarnesses)
			if err == nil {
				t.Fatal("configuration mutation returned nil error, want configuration error")
			}
			wantError := test.wantError
			if strings.Contains(wantError, "%s") {
				wantError = fmt.Sprintf(wantError, paths.Display(path))
			}
			if err.Error() != wantError {
				t.Fatalf("configuration mutation error = %q, want %q", err.Error(), wantError)
			}

			after, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile(%q) after configuration mutation returned unexpected error: %v", path, readErr)
			}
			if string(after) != string(before) {
				t.Fatalf("configuration mutation changed %q after returning an error", path)
			}
		})
	}
}

func TestEachMutationRejectsUnsupportedModelsWithoutWriting(t *testing.T) {
	tests := []struct {
		name      string
		contents  string
		mutate    func(context.Context, string) error
		wantError string
	}{
		{
			name:     "add tier",
			contents: `{"version":1,"tiers":{"reviewer":{"routes":[{"harness":"codex"}]}}}`,
			mutate: func(ctx context.Context, path string) error {
				return AddTier(ctx, path, testHarnesses, Tier{Name: "writer", Routes: []Route{{Harness: "grok", Model: testStringPointer("grok-unknown")}}})
			},
			wantError: `configuration file %s is invalid: tier "writer" route 0 field "model" has unsupported value "grok-unknown" for harness "grok"; supported models are "grok-4.6", "grok-4.5"`,
		},
		{
			name:     "update tier",
			contents: `{"version":1,"tiers":{"reviewer":{"routes":[{"harness":"codex"}]}}}`,
			mutate: func(ctx context.Context, path string) error {
				_, err := UpdateTier(ctx, path, testHarnesses, TierUpdate{Name: "reviewer", Routes: []Route{{Harness: "grok", Model: testStringPointer("grok-unknown")}}, ReplaceRoutes: true})

				return err
			},
			wantError: `configuration file %s is invalid: tier "reviewer" route 0 field "model" has unsupported value "grok-unknown" for harness "grok"; supported models are "grok-4.6", "grok-4.5"`,
		},
		{
			name:     "remove tier",
			contents: `{"version":1,"tiers":{"editor":{"routes":[{"harness":"codex"}]},"reviewer":{"routes":[{"harness":"grok","model":"grok-unknown"}]}}}`,
			mutate: func(ctx context.Context, path string) error {
				return RemoveTier(ctx, path, testHarnesses, "editor")
			},
			wantError: `configuration file %s is invalid: tier "reviewer" route 0 field "model" has unsupported value "grok-unknown" for harness "grok"; supported models are "grok-4.6", "grok-4.5"`,
		},
		{
			name:     "set default tier",
			contents: `{"version":1,"tiers":{"reviewer":{"routes":[{"harness":"grok","model":"grok-unknown"}]}}}`,
			mutate: func(ctx context.Context, path string) error {
				return SetDefaultTier(ctx, path, testHarnesses, "reviewer")
			},
			wantError: `configuration file %s is invalid: tier "reviewer" route 0 field "model" has unsupported value "grok-unknown" for harness "grok"; supported models are "grok-4.6", "grok-4.5"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ConfigFileName)
			writeConfiguration(t, path, []byte(test.contents))
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%q) before mutation returned unexpected error: %v", path, err)
			}

			err = test.mutate(context.Background(), path)
			if err == nil {
				t.Fatal("configuration mutation returned nil error, want configuration error")
			}
			wantError := fmt.Sprintf(test.wantError, paths.Display(path))
			if err.Error() != wantError {
				t.Fatalf("configuration mutation error = %q, want %q", err.Error(), wantError)
			}

			after, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("ReadFile(%q) after mutation returned unexpected error: %v", path, readErr)
			}
			if string(after) != string(before) {
				t.Fatalf("configuration mutation changed %q after returning an error", path)
			}
		})
	}
}

func assertWrittenJSONShape(t *testing.T, contents []byte) {
	t.Helper()

	written := string(contents)
	if !strings.HasSuffix(written, "\n") {
		t.Fatalf("written configuration does not end with a newline: %q", written)
	}
	if strings.Contains(written, "\t") || !strings.Contains(written, "\n  \"tiers\": {") {
		t.Fatalf("written configuration does not use two-space indentation: %q", written)
	}
	if strings.Index(written, "\"version\"") > strings.Index(written, "\"tiers\"") {
		t.Fatalf("written configuration keys are not in stable order: %q", written)
	}
}

func TestMarshalProjectConfigOmitsUnsetFieldsAndIsStable(t *testing.T) {
	configuration := Config{
		Version: SchemaVersion,
		Tiers: map[string]Tier{
			"engineer": {
				Name:   "engineer",
				Routes: []Route{{Harness: "codex"}},
			},
		},
	}

	written, err := marshalProjectConfig(configuration)
	if err != nil {
		t.Fatalf("marshalProjectConfig() returned unexpected error: %v", err)
	}
	for _, field := range []string{"\"description\"", "\"model\"", "\"fallbackBelowPercent\"", "\"defaultTier\""} {
		if strings.Contains(string(written), field) {
			t.Fatalf("marshalProjectConfig() wrote unset field %s: %s", field, written)
		}
	}

	root := t.TempDir()
	path := filepath.Join(root, ConfigFileName)
	writeConfiguration(t, path, written)
	loaded, err := Load(context.Background(), root, testHarnesses)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	rewritten, err := marshalProjectConfig(loaded)
	if err != nil {
		t.Fatalf("marshalProjectConfig() after Load() returned unexpected error: %v", err)
	}
	if string(rewritten) != string(written) {
		t.Fatalf("marshalProjectConfig() after Load() = %s, want %s", rewritten, written)
	}
}
