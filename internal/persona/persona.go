// Package persona resolves project-defined personas.
package persona

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/on-mission/landing/internal/harness"
)

const (
	personasDirectoryRelativePath = ".landing/personas"
	definitionFileName            = "PERSONA.md"
)

// Persona is one project-defined perspective with its instructions and reference material.
type Persona struct {
	Name           string
	Description    string
	Directory      string
	Instructions   string
	ReferenceFiles []string
}

// Resolve reads the named persona from the closest .landing/personas directory.
func Resolve(ctx context.Context, name string, cwd string) (Persona, error) {
	if !validPersonaName(name) {
		return Persona{}, invalidPersonaError(
			fmt.Sprintf("persona name %q is invalid; names begin with an ASCII letter or digit and contain only ASCII letters, digits, and hyphens", name),
		)
	}

	directory, searchFrom, searchedTo, err := findPersonasDirectory(ctx, cwd)
	if err != nil {
		return Persona{}, err
	}
	if directory == "" {
		return Persona{}, personaNotFoundError(name, searchFrom, searchedTo, nil)
	}

	persona, err := loadPersona(ctx, directory, name)
	if err == nil {
		return persona, nil
	}
	if !errorsIsNotExist(err) {
		return Persona{}, err
	}

	available, listErr := listPersonas(ctx, directory)
	if listErr != nil {
		return Persona{}, listErr
	}

	return Persona{}, personaNotFoundError(name, searchFrom, searchedTo, available)
}

// List returns the personas in the closest .landing/personas directory.
func List(ctx context.Context, cwd string) ([]Persona, error) {
	directory, searchFrom, searchedTo, err := findPersonasDirectory(ctx, cwd)
	if err != nil {
		return nil, err
	}
	if directory == "" {
		return nil, harness.NewError(
			harness.ErrorCodePersonaNotFound,
			fmt.Sprintf("no .landing/personas directory exists between %q and %q", searchFrom, searchedTo),
			nil,
		)
	}

	return listPersonas(ctx, directory)
}

func findPersonasDirectory(ctx context.Context, cwd string) (string, string, string, error) {
	searchFrom, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", "", invalidPersonaError(
			fmt.Sprintf("persona search directory %q could not be made absolute", cwd),
		)
	}

	for directory := searchFrom; ; directory = filepath.Dir(directory) {
		if err := ctx.Err(); err != nil {
			return "", "", "", err
		}

		candidate := filepath.Join(directory, personasDirectoryRelativePath)
		info, err := os.Stat(candidate)
		if err == nil {
			if !info.IsDir() {
				return "", "", "", invalidPersonaError(
					fmt.Sprintf("personas path %q has mode %s, not a directory", candidate, info.Mode()),
				)
			}

			return candidate, searchFrom, directory, nil
		}
		if !errorsIsNotExist(err) {
			return "", "", "", harness.WrapError(
				harness.ErrorCodeInvalidPersona,
				fmt.Sprintf("could not examine personas path %q", candidate),
				nil,
				err,
			)
		}

		parent := filepath.Dir(directory)
		if parent == directory {
			return "", searchFrom, directory, nil
		}
	}
}

func listPersonas(ctx context.Context, directory string) ([]Persona, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, harness.WrapError(
			harness.ErrorCodeInvalidPersona,
			fmt.Sprintf("could not list personas in %q", directory),
			nil,
			err,
		)
	}

	personas := make([]Persona, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() || !validPersonaName(entry.Name()) {
			continue
		}

		persona, err := loadPersona(ctx, directory, entry.Name())
		if err != nil {
			return nil, err
		}
		personas = append(personas, persona)
	}
	sort.Slice(personas, func(left int, right int) bool {
		return personas[left].Name < personas[right].Name
	})

	return personas, nil
}

func loadPersona(ctx context.Context, personasDirectory string, name string) (Persona, error) {
	if err := ctx.Err(); err != nil {
		return Persona{}, err
	}

	directory := filepath.Join(personasDirectory, name)
	definitionPath := filepath.Join(directory, definitionFileName)
	contents, err := os.ReadFile(definitionPath)
	if err != nil {
		if errorsIsNotExist(err) {
			info, statErr := os.Stat(directory)
			if statErr == nil && info.IsDir() {
				return Persona{}, invalidDefinitionError(definitionPath, "file does not exist")
			}
			if statErr != nil && !errorsIsNotExist(statErr) {
				return Persona{}, harness.WrapError(
					harness.ErrorCodeInvalidPersona,
					fmt.Sprintf("could not examine persona directory %q", directory),
					nil,
					statErr,
				)
			}

			return Persona{}, err
		}

		return Persona{}, harness.WrapError(
			harness.ErrorCodeInvalidPersona,
			fmt.Sprintf("could not read persona definition %q", definitionPath),
			nil,
			err,
		)
	}

	description, instructions, err := parseDefinition(definitionPath, string(contents))
	if err != nil {
		return Persona{}, err
	}
	references, err := referenceFiles(ctx, directory)
	if err != nil {
		return Persona{}, err
	}

	return Persona{
		Name:           name,
		Description:    description,
		Directory:      directory,
		Instructions:   instructions,
		ReferenceFiles: references,
	}, nil
}

func parseDefinition(path string, contents string) (string, string, error) {
	openingLine, openingEnd, ok := nextLine(contents, 0)
	if !ok || openingLine != "---" {
		return "", "", invalidDefinitionError(path, "front matter does not begin with ---")
	}

	closingStart := openingEnd
	for {
		line, next, ok := nextLine(contents, closingStart)
		if !ok {
			return "", "", invalidDefinitionError(path, "front matter has no closing ---")
		}
		if line == "---" {
			description, err := parseFrontMatter(path, contents[openingEnd:closingStart])
			if err != nil {
				return "", "", err
			}

			return description, contents[next:], nil
		}
		closingStart = next
	}
}

func nextLine(contents string, start int) (string, int, bool) {
	if start >= len(contents) {
		return "", start, false
	}

	end := strings.IndexByte(contents[start:], '\n')
	if end == -1 {
		return strings.TrimSuffix(contents[start:], "\r"), len(contents), true
	}
	end += start

	return strings.TrimSuffix(contents[start:end], "\r"), end + 1, true
}

func parseFrontMatter(path string, contents string) (string, error) {
	lines := strings.Split(contents, "\n")
	description := ""
	foundDescription := false
	for _, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != key || key == "" {
			return "", invalidDefinitionError(path, fmt.Sprintf("front matter line %q is not a key-value pair", line))
		}
		if key != "description" {
			if key == "routes" || key == "models" || key == "harnesses" || key == "tier" {
				return "", invalidDefinitionError(path, fmt.Sprintf("front matter key %q is not allowed: a persona has none of routes, models, harnesses, or tier", key))
			}

			return "", invalidDefinitionError(path, fmt.Sprintf("front matter has unknown key %q", key))
		}
		if foundDescription {
			return "", invalidDefinitionError(path, "front matter defines description more than once")
		}

		parsedDescription, err := parseDescription(value)
		if err != nil {
			return "", invalidDefinitionError(path, err.Error())
		}
		description = parsedDescription
		foundDescription = true
	}

	return description, nil
}

func parseDescription(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("front matter description is empty")
	}
	if strings.HasPrefix(value, "\"") {
		parsed, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("front matter description is not a valid quoted string")
		}
		if strings.Contains(parsed, "\n") || parsed == "" {
			return "", fmt.Errorf("front matter description is not a single line")
		}

		return parsed, nil
	}
	if strings.HasPrefix(value, "'") {
		if !strings.HasSuffix(value, "'") || len(value) == 1 {
			return "", fmt.Errorf("front matter description is not a valid quoted string")
		}
		value = strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	}
	if strings.Contains(value, "\n") {
		return "", fmt.Errorf("front matter description is not a single line")
	}

	return value, nil
}

func referenceFiles(ctx context.Context, directory string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, harness.WrapError(
			harness.ErrorCodeInvalidPersona,
			fmt.Sprintf("could not list persona directory %q", directory),
			nil,
			err,
		)
	}

	references := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Name() == definitionFileName || entry.IsDir() {
			continue
		}
		references = append(references, entry.Name())
	}
	sort.Strings(references)

	return references, nil
}

func personaNotFoundError(name string, searchFrom string, searchedTo string, available []Persona) error {
	names := make([]string, len(available))
	for index, persona := range available {
		names[index] = persona.Name
	}
	availableText := "none"
	if len(names) > 0 {
		availableText = strings.Join(names, ", ")
	}

	return harness.NewError(
		harness.ErrorCodePersonaNotFound,
		fmt.Sprintf("persona %q does not exist between %q and %q; available personas: %s", name, searchFrom, searchedTo, availableText),
		nil,
	)
}

func invalidDefinitionError(path string, problem string) error {
	return invalidPersonaError(fmt.Sprintf("persona definition %q is invalid: %s", path, problem))
}

func invalidPersonaError(message string) error {
	return harness.NewError(harness.ErrorCodeInvalidPersona, message, nil)
}

func validPersonaName(name string) bool {
	if name == "" {
		return false
	}
	for index, character := range name {
		if index == 0 && !isASCIIAlphanumeric(character) {
			return false
		}
		if isASCIIAlphanumeric(character) || character == '-' {
			continue
		}
		return false
	}

	return true
}

func isASCIIAlphanumeric(character rune) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

func errorsIsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
