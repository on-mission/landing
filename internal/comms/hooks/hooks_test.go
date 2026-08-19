package hooks

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallPreservesUnrelatedHookAndUninstallRestoresOriginalBytes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses POSIX executable shims")
	}
	project := t.TempDir()
	original := []byte(`{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "*",
        "hooks": [
          {"type": "command", "command": "/opt/herdr/session", "timeout": 10}
        ]
      }
    ]
  }
}
`)
	settings := filepath.Join(project, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, original, 0o600); err != nil {
		t.Fatal(err)
	}
	installHarnessShims(t)

	result, err := Install(context.Background(), project)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(result.Files) != len(harnesses()) {
		t.Fatalf("Install() files = %d, want %d", len(result.Files), len(harnesses()))
	}
	installed, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(installed, []byte("/opt/herdr/session")) {
		t.Fatalf("Install() settings = %s, want unrelated hook", installed)
	}
	if !containsLandingHook(installed) {
		t.Fatalf("Install() settings = %s, want Landing hook", installed)
	}
	if _, err := Install(context.Background(), project); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}

	if _, err := Uninstall(context.Background(), project); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	restored, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, original) {
		t.Fatalf("Uninstall() settings = %q, want byte-identical %q", restored, original)
	}
	t.Logf("pre-install bytes:\n%s", original)
	t.Logf("post-uninstall bytes:\n%s", restored)
}

func TestPlanReportsUnsafeAndUnsupportedConfigurations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses POSIX executable shims")
	}
	project := t.TempDir()
	installHarnessShims(t)
	settings := filepath.Join(project, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	unsafe := []byte("not JSON")
	if err := os.WriteFile(settings, unsafe, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := Plan(context.Background(), project)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Files[0].Operation != OperationBlocked {
		t.Fatalf("Plan() first operation = %q, want %q", plan.Files[0].Operation, OperationBlocked)
	}
	if !strings.Contains(plan.Render(), "comms hook --harness") || strings.Contains(plan.Render(), "resolved at installation") {
		t.Fatalf("Plan().Render() = %q, want exact resolved hook commands", plan.Render())
	}
	cline := plan.Files[len(plan.Files)-1]
	if cline.Operation != OperationSkipped || !strings.Contains(cline.Reason, "no context-injection") {
		t.Fatalf("Plan() Cline = %#v, want explicit unsupported explanation", cline)
	}
	if _, err := Install(context.Background(), project); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	got, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, unsafe) {
		t.Fatalf("Install() unsafe settings = %q, want unchanged %q", got, unsafe)
	}
}

func TestInstallCreatesOnlyValidHookDocuments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses POSIX executable shims")
	}
	project := t.TempDir()
	installHarnessShims(t)

	if _, err := Install(context.Background(), project); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for _, spec := range harnesses()[:3] {
		contents, err := os.ReadFile(filepath.Join(project, spec.relative))
		if err != nil {
			t.Fatalf("reading %s: %v", spec.name, err)
		}
		if err := validateHooksDocument(contents); err != nil {
			t.Fatalf("%s document invalid: %v", spec.name, err)
		}
		if !containsLandingHook(contents) {
			t.Fatalf("%s document = %s, want Landing hook", spec.name, contents)
		}
	}
}

func TestInstallReportsHarnessTrustRequirements(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses POSIX executable shims")
	}
	project := t.TempDir()
	installHarnessShims(t)

	plan, err := Plan(context.Background(), project)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	want := map[string]string{
		"Claude Code": "Claude Code records workspace-trust acceptance for project configuration; Landing does not create that acceptance",
		"Codex":       "Codex runs project hooks only after Codex has persisted trust for their source; Landing cannot create that trust",
		"Grok":        "Grok runs project hooks only after persisted folder trust includes the project; Landing does not create that trust",
	}
	for _, file := range plan.Files[:3] {
		if file.Reason != want[file.Harness] {
			t.Fatalf("Plan() %s reason = %q, want %q", file.Harness, file.Reason, want[file.Harness])
		}
		if !strings.Contains(plan.Render(), want[file.Harness]) {
			t.Fatalf("Plan().Render() = %q, want %s trust requirement", plan.Render(), file.Harness)
		}
	}

	result, err := Install(context.Background(), project)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for _, file := range result.Files[:3] {
		if file.Reason != want[file.Harness] {
			t.Fatalf("Install() %s reason = %q, want %q", file.Harness, file.Reason, want[file.Harness])
		}
	}
}

func installHarnessShims(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"landing", "claude", "codex", "grok"} {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
