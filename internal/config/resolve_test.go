package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/paths"
)

type staticHarnesses map[string][]string

func testStringPointer(value string) *string { return &value }

func (harnesses staticHarnesses) IDs() []string {
	ids := make([]string, 0, len(harnesses))
	for id := range harnesses {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	return ids
}

func (harnesses staticHarnesses) ModelCatalog(id string) harness.ModelCatalog {
	return harness.ModelCatalog{Models: append([]string(nil), harnesses[id]...), Authority: harness.ModelCatalogAuthoritative}
}

type advisoryHarnesses struct {
	staticHarnesses
}

func (harnesses advisoryHarnesses) ModelCatalog(id string) harness.ModelCatalog {
	catalog := harnesses.staticHarnesses.ModelCatalog(id)
	catalog.Authority = harness.ModelCatalogAdvisory

	return catalog
}

var testHarnesses = staticHarnesses{
	"claude": {"claude-sonnet-5", "claude-haiku-4-5-20251001"},
	"cline":  nil,
	"codex":  {"gpt-5.6-terra", "gpt-5.3-codex-spark", "gpt-5.6-luna"},
	"grok":   {"grok-4.6", "grok-4.5"},
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		configDir     string
		invocationDir string
		wantCode      harness.ErrorCode
		wantError     func(string, string) string
		wantTiers     []string
		verify        func(*testing.T, Config)
	}{
		{
			name:      "finds configuration in the invocation directory",
			fixture:   "added-tier.json",
			wantTiers: []string{"reviewer"},
		},
		{
			name:          "finds configuration several directories above the invocation directory",
			fixture:       "added-tier.json",
			invocationDir: filepath.Join("work", "feature", "task"),
			wantTiers:     []string{"reviewer"},
		},
		{
			name:          "reports the full search range when no configuration exists",
			invocationDir: filepath.Join("work", "feature"),
			wantCode:      harness.ErrorCodeConfigNotFound,
			wantError: func(invocation, _ string) string {
				return fmt.Sprintf(
					"no %s exists between %s and %s",
					ConfigFileName,
					paths.Display(invocation),
					paths.Display(filesystemRoot(invocation)),
				)
			},
		},
		{
			name:     "reports malformed JSON with its configuration path",
			fixture:  "malformed.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf("could not parse configuration file %s: unexpected EOF", paths.Display(configPath))
			},
		},
		{
			name:     "rejects an unknown JSON field",
			fixture:  "unknown-field.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf("could not parse configuration file %s: json: unknown field %q", paths.Display(configPath), "misspelled")
			},
		},
		{
			name:     "rejects the legacy useWhen field",
			fixture:  "legacy-use-when.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf("could not parse configuration file %s: json: unknown field %q", paths.Display(configPath), "useWhen")
			},
		},
		{
			name:     "rejects the legacy notFor field",
			fixture:  "legacy-not-for.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf("could not parse configuration file %s: json: unknown field %q", paths.Display(configPath), "notFor")
			},
		},
		{
			name:     "rejects the legacy validatedBy field",
			fixture:  "legacy-validated-by.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf("could not parse configuration file %s: json: unknown field %q", paths.Display(configPath), "validatedBy")
			},
		},
		{
			name:     "reports an unsupported schema version and the supported version",
			fixture:  "wrong-version.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf(
					"configuration file %s field %q has value 2; supported value is %d",
					paths.Display(configPath),
					"version",
					SchemaVersion,
				)
			},
		},
		{
			name:      "uses a declared tier without an inferred tier set",
			fixture:   "replaced-tier.json",
			wantTiers: []string{"engineer"},
			verify: func(t *testing.T, got Config) {
				t.Helper()
				engineer, ok := got.Tier("engineer")
				if !ok {
					t.Fatal("Load() did not return the engineer tier")
				}
				if engineer.Origin != OriginProject {
					t.Fatalf("Load().Tier(\"engineer\").Origin = %q, want %q", engineer.Origin, OriginProject)
				}
				if len(engineer.Routes) != 1 || engineer.Routes[0].Harness != "codex" {
					t.Fatalf("Load().Tier(\"engineer\").Routes = %#v, want one codex route", engineer.Routes)
				}
				if engineer.Description != "" {
					t.Fatalf("Load().Tier(\"engineer\").Description = %q, want an empty description", engineer.Description)
				}
				if _, ok := got.Tier("intern"); ok {
					t.Fatal("Load() retained the omitted intern tier")
				}
			},
		},
		{
			name:      "uses a declared tiers object as the complete tier set",
			fixture:   "added-tier.json",
			wantTiers: []string{"reviewer"},
			verify: func(t *testing.T, got Config) {
				t.Helper()
				reviewer, ok := got.Tier("reviewer")
				if !ok {
					t.Fatal("Load() did not return the reviewer tier")
				}
				if reviewer.Origin != OriginProject {
					t.Fatalf("Load().Tier(\"reviewer\").Origin = %q, want %q", reviewer.Origin, OriginProject)
				}
				if reviewer.Description != "Review a completed change, not write the initial implementation." {
					t.Fatalf("Load().Tier(\"reviewer\").Description = %q, want project description", reviewer.Description)
				}
			},
		},
		{
			name:      "resolves a configured default tier",
			fixture:   "default-tier.json",
			wantTiers: []string{"reviewer"},
			verify: func(t *testing.T, got Config) {
				t.Helper()
				if got.DefaultTier != "reviewer" {
					t.Fatalf("Load().DefaultTier = %q, want %q", got.DefaultTier, "reviewer")
				}
			},
		},
		{
			name:     "rejects a default tier when the file omits tiers",
			fixture:  "default-inherited-tier.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf("configuration file %s is invalid: field \"tiers\" is absent; configuration file names no tiers", paths.Display(configPath))
			},
		},
		{
			name:     "reports a default tier that is not configured",
			fixture:  "missing-default-tier.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf(
					"configuration file %s is invalid: field \"defaultTier\" has value \"absent\"; configured tiers are \"reviewer\"",
					paths.Display(configPath),
				)
			},
		},
		{
			name:     "rejects a file whose tiers key is absent",
			fixture:  "absent-tiers.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf("configuration file %s is invalid: field \"tiers\" is absent; configuration file names no tiers", paths.Display(configPath))
			},
		},
		{
			name:     "rejects an explicit empty tiers object",
			fixture:  "empty-tiers.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf("configuration file %s is invalid: field \"tiers\" is empty; configuration file names no tiers", paths.Display(configPath))
			},
		},
		{
			name:     "reports an unknown harness in a route",
			fixture:  "unknown-harness.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf(
					"configuration file %s is invalid: tier \"custom\" route 0 field \"harness\" has unsupported value \"unknown\"",
					paths.Display(configPath),
				)
			},
		},
		{
			name:     "reports a tier with no routes",
			fixture:  "empty-routes.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf("configuration file %s is invalid: tier \"custom\" field \"routes\" has zero routes", paths.Display(configPath))
			},
		},
		{
			name:     "reports every resolved validation problem together",
			fixture:  "multiple-validation-errors.json",
			wantCode: harness.ErrorCodeConfigInvalid,
			wantError: func(_, configPath string) string {
				return fmt.Sprintf(
					"configuration file %s is invalid: tier name has value \"\"; tier \"\" field \"routes\" has zero routes; tier \"bad\" route 0 field \"harness\" has unsupported value \"unknown-one\"; tier \"bad\" route 1 field \"harness\" has unsupported value \"unknown-two\"",
					paths.Display(configPath),
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			configDir := filepath.Join(root, test.configDir)
			configPath := filepath.Join(configDir, ConfigFileName)
			if test.fixture != "" {
				writeFixture(t, test.fixture, configPath)
			}

			invocationDir := filepath.Join(root, test.invocationDir)
			if err := os.MkdirAll(invocationDir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) returned unexpected error: %v", invocationDir, err)
			}

			got, err := Load(context.Background(), invocationDir, testHarnesses)
			if test.wantError != nil {
				if err == nil {
					t.Fatal("Load() returned nil error, want configuration error")
				}
				assertError(t, err, test.wantCode, test.wantError(invocationDir, configPath))
				return
			}
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}

			if !slices.Equal(got.TierNames(), test.wantTiers) {
				t.Fatalf("Load().TierNames() = %q, want %q", got.TierNames(), test.wantTiers)
			}
			wantSource := Source{
				Path:         configPath,
				SearchedFrom: invocationDir,
				SearchedTo:   configDir,
			}
			if got.Source != wantSource {
				t.Fatalf("Load().Source = %#v, want %#v", got.Source, wantSource)
			}
			if test.verify != nil {
				test.verify(t, got)
			}
		})
	}
}

// This catches configuration rejecting a newly released model solely because
// Landing's known-model examples are stale.
func TestLoadValidatesHarnessModels(t *testing.T) {
	tests := []struct {
		name      string
		harnesses Harnesses
		contents  string
		wantError string
	}{
		{
			name:      "accepts a registry harness absent from defaults",
			harnesses: staticHarnesses{"future": {"future-1"}},
			contents:  `{"version":1,"tiers":{"future":{"routes":[{"harness":"future","model":"future-1"}]}}}`,
		},
		{
			name:      "accepts a route with no model",
			harnesses: testHarnesses,
			contents:  `{"version":1,"tiers":{"local":{"routes":[{"harness":"cline"}]}}}`,
		},
		{
			name:      "rejects an unsupported model",
			harnesses: testHarnesses,
			contents:  `{"version":1,"tiers":{"custom":{"routes":[{"harness":"grok","model":"grok-unknown"}]}}}`,
			wantError: `configuration file %s is invalid: tier "custom" route 0 field "model" has unsupported value "grok-unknown" for harness "grok"; supported models are "grok-4.6", "grok-4.5"`,
		},
		{
			name:      "accepts an unlisted model from an advisory catalog",
			harnesses: advisoryHarnesses{staticHarnesses: testHarnesses},
			contents:  `{"version":1,"tiers":{"custom":{"routes":[{"harness":"codex","model":"gpt-5.6-sol"}]}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ConfigFileName)
			writeConfiguration(t, path, []byte(test.contents))

			_, err := Load(context.Background(), root, test.harnesses)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("Load() returned unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Load() returned nil error, want configuration error")
			}
			wantError := fmt.Sprintf(test.wantError, paths.Display(path))
			if err.Error() != wantError {
				t.Fatalf("Load() error = %q, want %q", err.Error(), wantError)
			}
		})
	}
}

func TestLoadSkipsLandingDirectoryWithoutConfiguration(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ConfigFileName)
	writeFixture(t, "added-tier.json", configPath)
	invocationDir := filepath.Join(root, "work", "feature", "task")
	personaDirectory := filepath.Join(root, "work", "feature", ".landing")
	if err := os.MkdirAll(personaDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned unexpected error: %v", personaDirectory, err)
	}
	if err := os.MkdirAll(invocationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned unexpected error: %v", invocationDir, err)
	}

	got, err := Load(context.Background(), invocationDir, testHarnesses)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	wantSource := Source{Path: configPath, SearchedFrom: invocationDir, SearchedTo: root}
	if got.Source != wantSource {
		t.Fatalf("Load().Source = %#v, want %#v", got.Source, wantSource)
	}
}

func TestLoadAcceptsExplicitNullOptionalFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigFileName)
	contents := `{"version":1,"defaultTier":null,"tiers":{"reviewer":{"description":null,"routes":[{"harness":"codex","model":null,"fallbackBelowPercent":null}]}}}`
	writeConfiguration(t, path, []byte(contents))

	configuration, err := Load(context.Background(), root, testHarnesses)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	tier := configuration.Tiers["reviewer"]
	if tier.Description != "" {
		t.Fatalf("Load().Tiers[\"reviewer\"].Description = %q, want empty string", tier.Description)
	}
	if configuration.DefaultTier != "" {
		t.Fatalf("Load().DefaultTier = %q, want empty string", configuration.DefaultTier)
	}
	if tier.Routes[0].Model != nil || tier.Routes[0].FallbackBelowPercent != nil {
		t.Fatalf("Load().Tiers[\"reviewer\"].Routes[0] = %#v, want nil optional values", tier.Routes[0])
	}
}

func writeFixture(t *testing.T, fixture string, path string) {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("ReadFile(%q) returned unexpected error: %v", fixture, err)
	}
	writeConfiguration(t, path, contents)
}

func writeConfiguration(t *testing.T, path string, contents []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned unexpected error: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("WriteFile(%q) returned unexpected error: %v", path, err)
	}
}

func assertError(t *testing.T, err error, wantCode harness.ErrorCode, wantMessage string) {
	t.Helper()

	var configError *harness.Error
	if !errors.As(err, &configError) {
		t.Fatalf("Load() error = %T %v, want *harness.Error", err, err)
	}
	if configError.Code != wantCode {
		t.Fatalf("Load() error code = %q, want %q", configError.Code, wantCode)
	}
	if err.Error() != wantMessage {
		t.Fatalf("Load() error message = %q, want %q", err.Error(), wantMessage)
	}
}

func filesystemRoot(path string) string {
	for {
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}
