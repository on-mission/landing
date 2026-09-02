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

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/router"
)

func TestRunPersonaManagement(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))

	code, err, stdout, _ := runCLI(t, directory, []string{"persona", "list"})
	if err != nil || code != exitOK || stdout.String() != "no personas\n" {
		t.Fatalf("Run(persona list with no directory) = %d, %v, %q; want %d, nil, no-personas output", code, err, stdout.String(), exitOK)
	}

	code, err, stdout, _ = runCLI(t, directory, []string{"persona", "add", "--name", "skeptic", "--description", "Challenges a decision rather than agreeing."})
	if err != nil || code != exitOK {
		t.Fatalf("Run(persona add) = %d, %v; want %d, nil", code, err, exitOK)
	}
	definition := filepath.Join(directory, ".landing", "personas", "skeptic", "PERSONA.md")
	contents, readErr := os.ReadFile(definition)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) after persona add returned unexpected error: %v", definition, readErr)
	}
	if string(contents) != "---\ndescription: \"Challenges a decision rather than agreeing.\"\n---\n" || !strings.Contains(stdout.String(), definition) {
		t.Fatalf("Run(persona add) contents = %q, output = %q; want validated definition and path", contents, stdout.String())
	}

	code, err, _, _ = runCLI(t, directory, []string{"persona", "add", "--name", "skeptic"})
	if code != exitUsage || err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Run(duplicate persona add) = %d, %v; want duplicate error", code, err)
	}
	code, err, _, _ = runCLI(t, directory, []string{"persona", "add", "--name", "not/a-name"})
	if code != exitUsage || err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("Run(persona add invalid name) = %d, %v; want invalid-name error", code, err)
	}

	body := "Preserve this exact body.\n\nIt has spacing.\n"
	if err := os.WriteFile(definition, append(personaDefinitionContents("old description"), []byte(body)...), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) returned unexpected error: %v", definition, err)
	}
	code, err, _, _ = runCLI(t, directory, []string{"persona", "update", "--name", "skeptic", "--description", "new description"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(persona update) = %d, %v; want %d, nil", code, err, exitOK)
	}
	updated, readErr := os.ReadFile(definition)
	if readErr != nil {
		t.Fatalf("ReadFile(%q) after persona update returned unexpected error: %v", definition, readErr)
	}
	updatedBody, bodyErr := personaInstructionsBody(updated)
	if bodyErr != nil || updatedBody != body {
		t.Fatalf("persona update body = %q, %v; want byte-identical %q, nil", updatedBody, bodyErr, body)
	}
	beforeNoop := string(updated)
	code, err, stdout, _ = runCLI(t, directory, []string{"persona", "update", "--name", "skeptic"})
	if err != nil || code != exitOK || stdout.String() != "persona update: no fields supplied; persona unchanged\n" {
		t.Fatalf("Run(persona update with no fields) = %d, %v, %q; want no-op message", code, err, stdout.String())
	}
	afterNoop, readErr := os.ReadFile(definition)
	if readErr != nil || string(afterNoop) != beforeNoop {
		t.Fatalf("persona update with no fields read error = %v and rewrote definition = %t", readErr, string(afterNoop) != beforeNoop)
	}

	if err := os.WriteFile(filepath.Join(filepath.Dir(definition), "references.md"), []byte("reference"), 0o600); err != nil {
		t.Fatalf("WriteFile(reference) returned unexpected error: %v", err)
	}
	code, err, stdout, _ = runCLI(t, directory, []string{"persona", "show", "--name", "skeptic"})
	if err != nil || code != exitOK || !strings.Contains(stdout.String(), "new description") || !strings.Contains(stdout.String(), "Preserve this exact body.") || !strings.Contains(stdout.String(), "It has spacing.") || !strings.Contains(stdout.String(), "references.md") {
		t.Fatalf("Run(persona show) = %d, %v, %q; want description, instructions, and references", code, err, stdout.String())
	}
	code, err, stdout, _ = runCLI(t, directory, []string{"persona", "list"})
	if err != nil || code != exitOK || stdout.String() != "skeptic: new description\n" {
		t.Fatalf("Run(persona list) = %d, %v, %q; want name and description", code, err, stdout.String())
	}
	code, err, stdout, _ = runCLI(t, directory, []string{"persona", "list", "--json"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(persona list --json) = %d, %v; want %d, nil", code, err, exitOK)
	}
	var reports []personaReport
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &reports); unmarshalErr != nil || len(reports) != 1 || reports[0].Name != "skeptic" || reports[0].Description != "new description" {
		t.Fatalf("persona list JSON = %q, %v; want skeptic report", stdout.String(), unmarshalErr)
	}
	code, err, stdout, _ = runCLI(t, directory, []string{"persona", "show", "--name", "skeptic", "--json"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(persona show --json) = %d, %v; want %d, nil", code, err, exitOK)
	}
	var report personaReport
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &report); unmarshalErr != nil || report.Instructions != body || len(report.ReferenceFiles) != 1 {
		t.Fatalf("persona show JSON = %q, %v; want full persona report", stdout.String(), unmarshalErr)
	}

	code, err, stdout, _ = runCLI(t, directory, []string{"persona", "remove", "--name", "skeptic"})
	if err != nil || code != exitOK || !strings.Contains(stdout.String(), "references.md") {
		t.Fatalf("Run(persona remove) = %d, %v, %q; want removed reference material named", code, err, stdout.String())
	}
	if _, statErr := os.Stat(filepath.Dir(definition)); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(%q) after persona remove = %v; want directory removed", filepath.Dir(definition), statErr)
	}
	code, err, _, _ = runCLI(t, directory, []string{"persona", "remove", "--name", "nosuch"})
	if code != exitUsage || err == nil || !strings.Contains(err.Error(), `persona "nosuch" does not exist; available personas: none`) {
		t.Fatalf("Run(persona remove missing) = %d, %v; want missing persona with available names", code, err)
	}
}

func TestRunPersonaManagementReadsInstructionsFromFileAndStandardInput(t *testing.T) {
	directory := t.TempDir()
	instructionsFile := filepath.Join(t.TempDir(), "instructions.md")
	fileBody := "You are a probe. Argue for simplicity.\n"
	if err := os.WriteFile(instructionsFile, []byte(fileBody), 0o600); err != nil {
		t.Fatal(err)
	}

	code, err, _, _ := runCLI(t, directory, []string{"persona", "add", "--name", "file-probe", "--description", "a probe", "--instructions-file", instructionsFile})
	if err != nil || code != exitOK {
		t.Fatalf("Run(persona add instructions file) = %d, %v; want %d, nil", code, err, exitOK)
	}
	fileDefinition, readErr := os.ReadFile(filepath.Join(directory, ".landing", "personas", "file-probe", "PERSONA.md"))
	if readErr != nil {
		t.Fatalf("ReadFile(file persona definition) returned unexpected error: %v", readErr)
	}
	fileInstructions, bodyErr := personaInstructionsBody(fileDefinition)
	if bodyErr != nil || fileInstructions != fileBody {
		t.Fatalf("personaInstructionsBody(file persona) = %q, %v; want %q, nil", fileInstructions, bodyErr, fileBody)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdinBody := "from stdin\n"
	code, err = Run(context.Background(), Inputs{Args: []string{"persona", "add", "--name", "stdin-probe", "--description", "b"}, InvocationDir: directory, Stdin: strings.NewReader(stdinBody), StdinIsTerminal: false, Stdout: stdout, Stderr: stderr})
	if err != nil || code != exitOK {
		t.Fatalf("Run(persona add standard input) = %d, %v; want %d, nil", code, err, exitOK)
	}
	stdinDefinition, readErr := os.ReadFile(filepath.Join(directory, ".landing", "personas", "stdin-probe", "PERSONA.md"))
	if readErr != nil {
		t.Fatalf("ReadFile(standard-input persona definition) returned unexpected error: %v", readErr)
	}
	stdinInstructions, bodyErr := personaInstructionsBody(stdinDefinition)
	if bodyErr != nil || stdinInstructions != stdinBody {
		t.Fatalf("personaInstructionsBody(standard-input persona) = %q, %v; want %q, nil", stdinInstructions, bodyErr, stdinBody)
	}

	updatedBody := "replacement instructions\n"
	if err := os.WriteFile(instructionsFile, []byte(updatedBody), 0o600); err != nil {
		t.Fatal(err)
	}
	code, err, _, _ = runCLI(t, directory, []string{"persona", "update", "--name", "stdin-probe", "--instructions-file", instructionsFile})
	if err != nil || code != exitOK {
		t.Fatalf("Run(persona update instructions file) = %d, %v; want %d, nil", code, err, exitOK)
	}
	updatedDefinition, readErr := os.ReadFile(filepath.Join(directory, ".landing", "personas", "stdin-probe", "PERSONA.md"))
	if readErr != nil {
		t.Fatalf("ReadFile(updated persona definition) returned unexpected error: %v", readErr)
	}
	updatedInstructions, bodyErr := personaInstructionsBody(updatedDefinition)
	if bodyErr != nil || updatedInstructions != updatedBody {
		t.Fatalf("personaInstructionsBody(updated persona) = %q, %v; want %q, nil", updatedInstructions, bodyErr, updatedBody)
	}
}

func TestRunDispatchPassesPersonaWithoutChangingTier(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	writeTestConfiguration(t, directory, `{"version":1,"defaultTier":"review","tiers":{"review":{"routes":[{"harness":"recording"}]}}}`)
	personaPath := filepath.Join(directory, ".landing", "personas", "skeptic", "PERSONA.md")
	if err := os.MkdirAll(filepath.Dir(personaPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned unexpected error: %v", filepath.Dir(personaPath), err)
	}
	if err := os.WriteFile(personaPath, []byte("---\ndescription: checks assumptions\n---\nChallenge the proposal."), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) returned unexpected error: %v", personaPath, err)
	}
	adapter := &personaDispatchAdapter{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code, err := runDispatch(
		context.Background(),
		Inputs{Stdin: strings.NewReader(""), Stdout: stdout, Stderr: stderr},
		options{Persona: parsedOption{Value: "skeptic", Set: true}},
		[]string{"Review this decision."},
		directory,
		router.NewMapRegistry(map[string]harness.Adapter{"recording": adapter}),
	)
	if code != exitFailed || err == nil || !strings.Contains(err.Error(), "stop before spawn") {
		t.Fatalf("runDispatch() = %d, %v; want fake adapter stop before any job starts", code, err)
	}
	if adapter.params.Prompt != "Review this decision." {
		t.Fatalf("runDispatch() prompt = %q, want original prompt", adapter.params.Prompt)
	}
	if adapter.params.Persona == nil || adapter.params.Persona.Instructions != "Challenge the proposal." {
		t.Fatalf("runDispatch() persona = %#v, want selected persona instructions", adapter.params.Persona)
	}
}

type personaDispatchAdapter struct {
	params harness.StartParams
}

func (adapter *personaDispatchAdapter) ID() string {
	return "recording"
}

func (adapter *personaDispatchAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{Authority: harness.ModelCatalogAuthoritative}
}

func (adapter *personaDispatchAdapter) Detect(context.Context) harness.Detection {
	return harness.Detection{Status: harness.DetectionReady, Capacity: adapter.ProbeCapacity(context.Background())}
}

func (adapter *personaDispatchAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{}
}

func (adapter *personaDispatchAdapter) Validate(params harness.StartParams) error {
	adapter.params = params

	return nil
}

func (adapter *personaDispatchAdapter) BuildStart(harness.StartParams) (harness.Request, error) {
	return harness.Request{}, errors.New("stop before spawn")
}

func (adapter *personaDispatchAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return harness.Request{}, errors.New("stop before spawn")
}

func (adapter *personaDispatchAdapter) OnStdoutLine(string, *harness.JobRecord) {}

func (adapter *personaDispatchAdapter) Finalize(context.Context, harness.FinalizeParams) (harness.Finalized, error) {
	return harness.Finalized{}, nil
}

func (adapter *personaDispatchAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return harness.KnownCapacity([]harness.Bucket{{ID: "capacity", UsedPercent: 0}})
}

func (adapter *personaDispatchAdapter) SpawnPath() string {
	return ""
}
