package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/paths"
	"github.com/on-mission/landing/internal/persona"
)

const (
	personaDirectoryPath = ".landing/personas"
	personaDefinition    = "PERSONA.md"
)

func showPersona(ctx context.Context, invocationDir string, values options, stdout io.Writer) (int, error) {
	if !values.Name.Set || values.Name.Value == "" {
		return exitUsage, &usageError{message: "persona show has no --name"}
	}
	if values.Description.Set {
		return exitUsage, &usageError{message: "persona show has a description option"}
	}

	selected, err := persona.Resolve(ctx, values.Name.Value, invocationDir)
	if err != nil {
		return exitUsage, err
	}
	if values.JSON {
		report := personaReport{Name: selected.Name, Description: selected.Description, Instructions: selected.Instructions, ReferenceFiles: selected.ReferenceFiles}
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
			return exitFailed, err
		}

		return exitOK, nil
	}
	if _, err := fmt.Fprintf(stdout, "%s\n", selected.Name); err != nil {
		return exitFailed, err
	}
	if selected.Description != "" {
		if _, err := fmt.Fprintf(stdout, "  description: %s\n", selected.Description); err != nil {
			return exitFailed, err
		}
	}
	if _, err := fmt.Fprintf(stdout, "  instructions:\n%s", indentPersonaInstructions(selected.Instructions)); err != nil {
		return exitFailed, err
	}
	if len(selected.ReferenceFiles) == 0 {
		if _, err := fmt.Fprintln(stdout, "  reference files: none"); err != nil {
			return exitFailed, err
		}

		return exitOK, nil
	}
	if _, err := fmt.Fprintln(stdout, "  reference files:"); err != nil {
		return exitFailed, err
	}
	for _, reference := range selected.ReferenceFiles {
		if _, err := fmt.Fprintf(stdout, "    - %s\n", reference); err != nil {
			return exitFailed, err
		}
	}

	return exitOK, nil
}

func mutatePersona(ctx context.Context, selected command, values options, invocationDir string, stdout io.Writer) (int, error) {
	if !values.Name.Set || values.Name.Value == "" {
		return exitUsage, &usageError{message: fmt.Sprintf("command %q has no --name", selected)}
	}
	if !validCLIpersonaName(values.Name.Value) {
		return exitUsage, invalidCLIpersonaNameError(values.Name.Value)
	}
	if values.Description.Set && strings.Contains(values.Description.Value, "\n") {
		return exitUsage, harness.NewError(harness.ErrorCodeInvalidPersona, "persona description is not a single line", nil)
	}
	if selected == commandPersonaAdd {
		return addPersona(ctx, invocationDir, values, stdout)
	}
	if selected == commandPersonaUpdate {
		return updatePersona(ctx, invocationDir, values, stdout)
	}
	if values.Description.Set {
		return exitUsage, &usageError{message: "persona remove has a description option"}
	}

	return removePersona(ctx, invocationDir, values.Name.Value, stdout)
}

func addPersona(ctx context.Context, invocationDir string, values options, stdout io.Writer) (int, error) {
	if err := ctx.Err(); err != nil {
		return exitFailed, err
	}
	directory := managedPersonaDirectory(invocationDir, values.Name.Value)
	if err := os.MkdirAll(filepath.Dir(directory), 0o755); err != nil {
		return exitFailed, fmt.Errorf("create personas directory %s: %w", paths.Display(filepath.Dir(directory)), err)
	}
	if err := os.Mkdir(directory, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return exitUsage, harness.NewError(harness.ErrorCodeInvalidPersona, fmt.Sprintf("persona %q already exists at %s", values.Name.Value, paths.Display(directory)), nil)
		}

		return exitFailed, fmt.Errorf("create persona directory %s: %w", paths.Display(directory), err)
	}
	path := filepath.Join(directory, personaDefinition)
	if err := os.WriteFile(path, personaDefinitionContents(values.Description.Value, values.Instructions.Value), 0o644); err != nil {
		if removeErr := os.RemoveAll(directory); removeErr != nil {
			return exitFailed, errors.Join(
				fmt.Errorf("write persona definition %s: %w", paths.Display(path), err),
				fmt.Errorf("remove incomplete persona directory %s: %w", paths.Display(directory), removeErr),
			)
		}

		return exitFailed, fmt.Errorf("write persona definition %s: %w", paths.Display(path), err)
	}
	if _, err := fmt.Fprintf(stdout, "added persona: %s\npersona definition: %s\n", values.Name.Value, path); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func updatePersona(ctx context.Context, invocationDir string, values options, stdout io.Writer) (int, error) {
	selected, path, contents, err := managedPersona(ctx, invocationDir, values.Name.Value)
	if err != nil {
		return exitUsage, err
	}
	if !values.Description.Set && !values.Instructions.Set {
		if _, err := fmt.Fprintln(stdout, "persona update: no fields supplied; persona unchanged"); err != nil {
			return exitFailed, err
		}

		return exitOK, nil
	}
	body, err := personaInstructionsBody(contents)
	if err != nil {
		return exitFailed, err
	}
	description := selected.Description
	if values.Description.Set {
		description = values.Description.Value
	}
	if values.Instructions.Set {
		body = values.Instructions.Value
	}
	if err := os.WriteFile(path, personaDefinitionContents(description, body), 0o644); err != nil {
		return exitFailed, fmt.Errorf("write persona definition %s: %w", paths.Display(path), err)
	}
	if _, err := fmt.Fprintf(stdout, "updated persona: %s\n", selected.Name); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func removePersona(ctx context.Context, invocationDir string, name string, stdout io.Writer) (int, error) {
	selected, _, _, err := managedPersona(ctx, invocationDir, name)
	if err != nil {
		return exitUsage, err
	}
	if err := os.RemoveAll(selected.Directory); err != nil {
		return exitFailed, fmt.Errorf("remove persona directory %s: %w", paths.Display(selected.Directory), err)
	}
	references := "no reference files"
	if len(selected.ReferenceFiles) != 0 {
		references = "reference files: " + strings.Join(selected.ReferenceFiles, ", ")
	}
	if _, err := fmt.Fprintf(stdout, "removed persona: %s (%s)\n", selected.Name, references); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func managedPersona(ctx context.Context, invocationDir string, name string) (persona.Persona, string, []byte, error) {
	directory := managedPersonaDirectory(invocationDir, name)
	path := filepath.Join(directory, personaDefinition)
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return persona.Persona{}, "", nil, missingManagedPersona(ctx, invocationDir, name)
		}

		return persona.Persona{}, "", nil, fmt.Errorf("read persona definition %s: %w", paths.Display(path), err)
	}
	selected, err := persona.Resolve(ctx, name, invocationDir)
	if err != nil {
		return persona.Persona{}, "", nil, err
	}
	if selected.Directory != directory {
		return persona.Persona{}, "", nil, harness.NewError(harness.ErrorCodePersonaNotFound, fmt.Sprintf("persona %q does not exist at %s", name, paths.Display(directory)), nil)
	}

	return selected, path, contents, nil
}

func missingManagedPersona(ctx context.Context, invocationDir string, name string) error {
	personas, err := persona.List(ctx, invocationDir)
	if err != nil {
		var harnessError *harness.Error
		if !errors.As(err, &harnessError) || harnessError.Code != harness.ErrorCodePersonaNotFound {
			return err
		}
	}
	names := make([]string, 0, len(personas))
	for _, selected := range personas {
		names = append(names, selected.Name)
	}
	available := "none"
	if len(names) != 0 {
		available = strings.Join(names, ", ")
	}

	return harness.NewError(harness.ErrorCodePersonaNotFound, fmt.Sprintf("persona %q does not exist; available personas: %s", name, available), nil)
}

func managedPersonaDirectory(invocationDir string, name string) string {
	return filepath.Join(invocationDir, personaDirectoryPath, name)
}

func personaDefinitionContents(description string, body ...string) []byte {
	var definition strings.Builder
	definition.WriteString("---\n")
	if description != "" {
		fmt.Fprintf(&definition, "description: %s\n", strconv.Quote(description))
	}
	definition.WriteString("---\n")
	if len(body) != 0 {
		definition.WriteString(body[0])
	}

	return []byte(definition.String())
}

func personaInstructionsBody(contents []byte) (string, error) {
	text := string(contents)
	firstEnd := strings.IndexByte(text, '\n')
	if firstEnd == -1 || strings.TrimSuffix(text[:firstEnd], "\r") != "---" {
		return "", fmt.Errorf("persona definition has no opening front matter delimiter")
	}
	for offset := firstEnd + 1; offset < len(text); {
		relativeEnd := strings.IndexByte(text[offset:], '\n')
		if relativeEnd == -1 {
			if strings.TrimSuffix(text[offset:], "\r") == "---" {
				return "", nil
			}

			return "", fmt.Errorf("persona definition has no closing front matter delimiter")
		}
		end := offset + relativeEnd
		if strings.TrimSuffix(text[offset:end], "\r") == "---" {
			return text[end+1:], nil
		}
		offset = end + 1
	}

	return "", fmt.Errorf("persona definition has no closing front matter delimiter")
}

func indentPersonaInstructions(instructions string) string {
	if instructions == "" {
		return "    (empty)\n"
	}

	return "    " + strings.ReplaceAll(instructions, "\n", "\n    ") + "\n"
}

func validCLIpersonaName(name string) bool {
	if name == "" {
		return false
	}
	for index, character := range name {
		if index == 0 && !isASCIIAlphaNumeric(character) {
			return false
		}
		if isASCIIAlphaNumeric(character) || character == '-' {
			continue
		}

		return false
	}

	return true
}

func isASCIIAlphaNumeric(character rune) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

func invalidCLIpersonaNameError(name string) error {
	return harness.NewError(harness.ErrorCodeInvalidPersona, fmt.Sprintf("persona name %q is invalid; names begin with an ASCII letter or digit and contain only ASCII letters, digits, and hyphens", name), nil)
}
