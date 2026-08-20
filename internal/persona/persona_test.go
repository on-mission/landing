package persona

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/on-mission/landing/internal/harness"
)

func TestResolveFindsPersonasInWorkingDirectoryAndAncestor(t *testing.T) {
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "project", "nested")
	writePersona(t, filepath.Join(root, "project"), "ancestor", "from an ancestor", "ancestor instructions")
	writePersona(t, workingDirectory, "local", "from the working directory", "local instructions")

	local, err := Resolve(context.Background(), "local", workingDirectory)
	if err != nil {
		t.Fatalf("Resolve(local) returned unexpected error: %v", err)
	}
	if local.Instructions != "local instructions" {
		t.Errorf("Resolve(local) instructions = %q, want %q", local.Instructions, "local instructions")
	}

	ancestor, err := Resolve(context.Background(), "ancestor", filepath.Join(root, "project", "other"))
	if err != nil {
		t.Fatalf("Resolve(ancestor) returned unexpected error: %v", err)
	}
	if ancestor.Instructions != "ancestor instructions" {
		t.Errorf("Resolve(ancestor) instructions = %q, want %q", ancestor.Instructions, "ancestor instructions")
	}
}

func TestResolveKeepsVerbatimInstructionsAndReferences(t *testing.T) {
	root := t.TempDir()
	instructions := "\nKeep this exact.\n\nDo not interpret {{anything}}.\n"
	directory := writePersona(t, root, "reviewer", "reviews decisions", instructions, "errors.md", "philosophy.md")

	selected, err := Resolve(context.Background(), "reviewer", root)
	if err != nil {
		t.Fatalf("Resolve() returned unexpected error: %v", err)
	}
	if selected.Instructions != instructions {
		t.Errorf("Resolve() instructions = %q, want verbatim %q", selected.Instructions, instructions)
	}
	if selected.Directory != directory {
		t.Errorf("Resolve() directory = %q, want %q", selected.Directory, directory)
	}
	if strings.Join(selected.ReferenceFiles, ",") != "errors.md,philosophy.md" {
		t.Errorf("Resolve() reference files = %q, want errors.md,philosophy.md", selected.ReferenceFiles)
	}
}

func TestResolveReportsNoReferences(t *testing.T) {
	root := t.TempDir()
	writePersona(t, root, "writer", "writes clearly", "writer instructions")

	selected, err := Resolve(context.Background(), "writer", root)
	if err != nil {
		t.Fatalf("Resolve() returned unexpected error: %v", err)
	}
	if len(selected.ReferenceFiles) != 0 {
		t.Errorf("Resolve() reference files = %q, want none", selected.ReferenceFiles)
	}
}

func TestResolveMissingPersonaNamesSearchAndAvailablePersonas(t *testing.T) {
	root := t.TempDir()
	writePersona(t, root, "available", "available persona", "instructions")
	workingDirectory := filepath.Join(root, "nested")
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned unexpected error: %v", workingDirectory, err)
	}

	_, err := Resolve(context.Background(), "missing", workingDirectory)
	assertHarnessError(t, err, harness.ErrorCodePersonaNotFound)
	for _, want := range []string{"missing", workingDirectory, root, "available"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Resolve(missing) error = %q, want it to contain %q", err, want)
		}
	}
}

func TestResolveRejectsPersonaPolicyKeys(t *testing.T) {
	keys := []string{"routes", "models", "harnesses", "tier"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			root := t.TempDir()
			writePersonaDefinition(t, root, "invalid", "---\n"+key+": codex\n---\ninstructions")

			_, err := Resolve(context.Background(), "invalid", root)
			assertHarnessError(t, err, harness.ErrorCodeInvalidPersona)
			if !strings.Contains(err.Error(), "a persona has none of routes, models, harnesses, or tier") {
				t.Errorf("Resolve(invalid) error = %q, want persona-policy explanation", err)
			}
		})
	}
}

func TestResolveRejectsUnknownFrontMatterKey(t *testing.T) {
	root := t.TempDir()
	writePersonaDefinition(t, root, "invalid", "---\nname: reviewer\n---\ninstructions")

	_, err := Resolve(context.Background(), "invalid", root)
	assertHarnessError(t, err, harness.ErrorCodeInvalidPersona)
	if !strings.Contains(err.Error(), "unknown key \"name\"") {
		t.Errorf("Resolve(invalid) error = %q, want unknown-key detail", err)
	}
}

func TestResolveMalformedFrontMatterNamesDefinition(t *testing.T) {
	root := t.TempDir()
	path := writePersonaDefinition(t, root, "invalid", "description: no opening delimiter")

	_, err := Resolve(context.Background(), "invalid", root)
	assertHarnessError(t, err, harness.ErrorCodeInvalidPersona)
	for _, want := range []string{path, "front matter"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Resolve(invalid) error = %q, want it to contain %q", err, want)
		}
	}
}

func TestInvalidDefinitionErrorPreservesWindowsPath(t *testing.T) {
	path := `C:\Users\Landing Model\.landing\personas\reviewer\PERSONA.md`
	want := `persona definition "C:\Users\Landing Model\.landing\personas\reviewer\PERSONA.md" is invalid: front matter does not begin with ---`
	if got := invalidDefinitionError(path, "front matter does not begin with ---").Error(); got != want {
		t.Fatalf("invalidDefinitionError() = %q, want %q", got, want)
	}
}

func TestListIncludesDescriptions(t *testing.T) {
	root := t.TempDir()
	writePersona(t, root, "zebra", "last", "zebra instructions")
	writePersona(t, root, "alpha", "first", "alpha instructions")

	personas, err := List(context.Background(), root)
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}
	if len(personas) != 2 {
		t.Fatalf("List() returned %d personas, want 2", len(personas))
	}
	if personas[0].Name != "alpha" || personas[0].Description != "first" {
		t.Errorf("List()[0] = %#v, want alpha with first description", personas[0])
	}
	if personas[1].Name != "zebra" || personas[1].Description != "last" {
		t.Errorf("List()[1] = %#v, want zebra with last description", personas[1])
	}
}

func writePersona(t *testing.T, root string, name string, description string, instructions string, references ...string) string {
	t.Helper()
	directory := filepath.Join(root, personasDirectoryRelativePath, name)
	path := writePersonaDefinition(t, root, name, "---\ndescription: "+description+"\n---\n"+instructions)
	for _, reference := range references {
		referencePath := filepath.Join(directory, reference)
		if err := os.WriteFile(referencePath, []byte("reference material"), 0o600); err != nil {
			t.Fatalf("WriteFile(%q) returned unexpected error: %v", referencePath, err)
		}
	}

	return filepath.Dir(path)
}

func writePersonaDefinition(t *testing.T, root string, name string, contents string) string {
	t.Helper()
	directory := filepath.Join(root, personasDirectoryRelativePath, name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned unexpected error: %v", directory, err)
	}
	path := filepath.Join(directory, definitionFileName)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) returned unexpected error: %v", path, err)
	}

	return path
}

func assertHarnessError(t *testing.T, err error, wantCode harness.ErrorCode) {
	t.Helper()
	var typed *harness.Error
	if !errors.As(err, &typed) || typed.Code != wantCode {
		t.Fatalf("error = %v, want harness code %s", err, wantCode)
	}
}
