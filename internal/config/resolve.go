package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/on-mission/landing/internal/paths"
	"strings"

	"github.com/on-mission/landing/internal/harness"
)

type projectConfigFile struct {
	Version     int             `json:"version"`
	DefaultTier string          `json:"defaultTier"`
	Tiers       json.RawMessage `json:"tiers"`
}

type projectTier struct {
	Description string         `json:"description,omitempty"`
	Routes      []projectRoute `json:"routes"`
}

type projectRoute struct {
	Harness              string   `json:"harness"`
	Model                *string  `json:"model,omitempty"`
	FallbackBelowPercent *float64 `json:"fallbackBelowPercent,omitempty"`
}

type tiersAbsentError struct{}

func (tiersAbsentError) Error() string {
	return "field \"tiers\" is absent; configuration file names no tiers"
}

// resolve is the configuration boundary. See [Load] for the public contract.
func resolve(ctx context.Context, invocationDir string, harnesses Harnesses) (Config, error) {
	searchFrom, err := filepath.Abs(invocationDir)
	if err != nil {
		return Config{}, invalidConfigError(
			fmt.Sprintf("configuration search directory %s could not be made absolute", paths.Display(invocationDir)),
			err,
		)
	}

	path, searchedTo, err := findConfigFile(ctx, searchFrom)
	if err != nil {
		return Config{}, err
	}
	// A cancelled context is not a statement about the configuration. Reporting
	// it as CONFIG_INVALID would tell the caller its file is broken when the
	// file may be entirely correct and merely unread.
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, invalidConfigError(
			fmt.Sprintf("could not read configuration file %s", paths.Display(path)),
			err,
		)
	}

	file, err := decodeProjectConfig(contents)
	if err != nil {
		return Config{}, invalidConfigError(
			fmt.Sprintf("could not parse configuration file %s", paths.Display(path)),
			err,
		)
	}
	if file.Version != SchemaVersion {
		return Config{}, invalidConfigError(
			fmt.Sprintf(
				"configuration file %s field %q has value %d; supported value is %d",
				paths.Display(path),
				"version",
				file.Version,
				SchemaVersion,
			),
			nil,
		)
	}

	resolved, err := resolveProjectFile(file)
	if err != nil {
		return Config{}, projectConfigResolutionError(path, err)
	}
	resolved.Source = Source{
		Path:         path,
		SearchedFrom: searchFrom,
		SearchedTo:   searchedTo,
	}

	if err := validate(resolved, harnesses); err != nil {
		return Config{}, invalidConfigError(
			fmt.Sprintf("configuration file %s is invalid", paths.Display(path)),
			err,
		)
	}

	return resolved, nil
}

func resolveProjectFile(file projectConfigFile) (Config, error) {
	if file.Tiers == nil {
		return Config{}, tiersAbsentError{}
	}

	projectTiers, err := decodeProjectTiers(file.Tiers)
	if err != nil {
		return Config{}, err
	}

	resolved := Config{
		Version:     SchemaVersion,
		DefaultTier: file.DefaultTier,
		Tiers:       make(map[string]Tier, len(projectTiers)),
	}
	for name, tier := range projectTiers {
		resolved.Tiers[name] = Tier{
			Name:        name,
			Description: tier.Description,
			Routes:      resolvedRoutes(tier.Routes),
			Origin:      OriginProject,
		}
	}

	return resolved, nil
}

func projectConfigResolutionError(path string, cause error) error {
	var absent tiersAbsentError
	if errors.As(cause, &absent) {
		return invalidConfigError(fmt.Sprintf("configuration file %s is invalid", paths.Display(path)), cause)
	}

	return invalidConfigError(fmt.Sprintf("could not parse configuration file %s", paths.Display(path)), cause)
}

func findConfigFile(ctx context.Context, searchFrom string) (string, string, error) {
	for directory := searchFrom; ; directory = filepath.Dir(directory) {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}

		path := filepath.Join(directory, ConfigFileName)
		info, err := os.Stat(path)
		if err == nil {
			if !info.Mode().IsRegular() {
				return "", "", invalidConfigError(
					fmt.Sprintf("configuration path %s has mode %s, not a regular file", paths.Display(path), info.Mode()),
					nil,
				)
			}

			return path, directory, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", invalidConfigError(
				fmt.Sprintf("could not examine configuration path %s", paths.Display(path)),
				err,
			)
		}

		parent := filepath.Dir(directory)
		if parent == directory {
			return "", "", harness.NewError(
				harness.ErrorCodeConfigNotFound,
				fmt.Sprintf("no %s exists between %s and %s", ConfigFileName, paths.Display(searchFrom), paths.Display(directory)),
				nil,
			)
		}
	}
}

func decodeProjectConfig(contents []byte) (projectConfigFile, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()

	var file projectConfigFile
	if err := decoder.Decode(&file); err != nil {
		return projectConfigFile{}, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return projectConfigFile{}, err
	}

	return file, nil
}

func decodeProjectTiers(raw json.RawMessage) (map[string]projectTier, error) {
	if raw == nil {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("field %q has value null, expected an object", "tiers")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var tiers map[string]projectTier
	if err := decoder.Decode(&tiers); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}

	return tiers, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var additionalValue struct{}
	if err := decoder.Decode(&additionalValue); err != io.EOF {
		if err != nil {
			return err
		}

		return errors.New("contains an additional JSON value")
	}

	return nil
}

func resolvedRoutes(routes []projectRoute) []Route {
	resolved := make([]Route, len(routes))
	for index, route := range routes {
		resolved[index] = Route{
			Harness:              route.Harness,
			Model:                route.Model,
			FallbackBelowPercent: route.FallbackBelowPercent,
		}
	}

	return resolved
}

func validate(config Config, harnesses Harnesses) error {
	problems := validatePolicy(config)
	if harnesses != nil {
		problems = append(problems, validateHarnesses(config, harnesses)...)
		problems = append(problems, validateHarnessConfiguration(config, harnesses)...)
	}

	if len(problems) == 0 {
		return nil
	}

	return errors.New(strings.Join(problems, "; "))
}

func validatePolicy(config Config) []string {
	problems := make([]string, 0)
	if len(config.Tiers) == 0 {
		problems = append(problems, "field \"tiers\" is empty; configuration file names no tiers")
	}
	if config.DefaultTier != "" {
		if _, ok := config.Tiers[config.DefaultTier]; !ok {
			problems = append(problems, fmt.Sprintf(
				"field \"defaultTier\" has value %q; configured tiers are %s",
				config.DefaultTier,
				formatTierNames(config.TierNames()),
			))
		}
	}

	for _, name := range config.TierNames() {
		tier := config.Tiers[name]
		if tier.Name == "" {
			problems = append(problems, "tier name has value \"\"")
		}
		if len(tier.Routes) == 0 {
			problems = append(problems, fmt.Sprintf("tier %q field \"routes\" has zero routes", tier.Name))
		}
		for index, route := range tier.Routes {
			if route.Model != nil && !validModelName(*route.Model) {
				problems = append(problems, fmt.Sprintf(
					"tier %q route %d field \"model\" has invalid value %q",
					tier.Name,
					index,
					*route.Model,
				))
			}
		}
	}

	return problems
}

func validateHarnesses(config Config, harnesses Harnesses) []string {
	known := make(map[string]struct{}, len(harnesses.IDs()))
	for _, name := range harnesses.IDs() {
		known[name] = struct{}{}
	}

	problems := make([]string, 0)
	for _, name := range config.TierNames() {
		tier := config.Tiers[name]
		for index, route := range tier.Routes {
			if _, ok := known[route.Harness]; !ok {
				problems = append(problems, fmt.Sprintf(
					"tier %q route %d field \"harness\" has unsupported value %q",
					tier.Name,
					index,
					route.Harness,
				))
				continue
			}
			if route.Model == nil {
				continue
			}
			catalog := harnesses.ModelCatalog(route.Harness)
			if catalog.Supports(*route.Model) {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"tier %q route %d field \"model\" has unsupported value %q for harness %q; supported models are %s",
				tier.Name,
				index,
				*route.Model,
				route.Harness,
				formatModels(catalog.Models),
			))
		}
	}

	return problems
}

type harnessConfigurationValidator interface {
	ValidateConfiguration(string, []harness.ConfiguredRoute) error
}

func validateHarnessConfiguration(config Config, harnesses Harnesses) []string {
	validator, ok := harnesses.(harnessConfigurationValidator)
	if !ok {
		return nil
	}

	ids := harnesses.IDs()
	slices.Sort(ids)
	known := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		known[id] = struct{}{}
	}
	routesByHarness := make(map[string][]harness.ConfiguredRoute)
	for _, name := range config.TierNames() {
		tier := config.Tiers[name]
		for index, route := range tier.Routes {
			if _, supported := known[route.Harness]; !supported {
				continue
			}
			routesByHarness[route.Harness] = append(routesByHarness[route.Harness], harness.ConfiguredRoute{
				Location: fmt.Sprintf("tier %q route %d", tier.Name, index),
				Model:    route.Model,
			})
		}
	}

	problems := make([]string, 0)
	for _, id := range ids {
		routes := routesByHarness[id]
		if len(routes) == 0 {
			continue
		}
		if err := validator.ValidateConfiguration(id, routes); err != nil {
			problems = append(problems, err.Error())
		}
	}

	return problems
}

func validModelName(model string) bool {
	return model != ""
}

func formatModels(models []string) string {
	if len(models) == 0 {
		return "none"
	}

	quoted := make([]string, len(models))
	for index, model := range models {
		quoted[index] = fmt.Sprintf("%q", model)
	}

	return strings.Join(quoted, ", ")
}

func formatTierNames(names []string) string {
	quoted := make([]string, len(names))
	for index, name := range names {
		quoted[index] = fmt.Sprintf("%q", name)
	}

	return strings.Join(quoted, ", ")
}

func invalidConfigError(message string, cause error) error {
	if cause == nil {
		return harness.NewError(harness.ErrorCodeConfigInvalid, message, nil)
	}

	return harness.WrapError(harness.ErrorCodeConfigInvalid, message+": "+cause.Error(), nil, cause)
}
