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
	"strings"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/router"
)

type command string

const (
	commandConfigInit    command = "config init"
	commandConfigShow    command = "config show"
	commandTierList      command = "tier list"
	commandTierAdd       command = "tier add"
	commandTierUpdate    command = "tier update"
	commandTierRemove    command = "tier remove"
	commandHarnessList   command = "harness list"
	commandPersonaList   command = "persona list"
	commandPersonaShow   command = "persona show"
	commandPersonaAdd    command = "persona add"
	commandPersonaUpdate command = "persona update"
	commandPersonaRemove command = "persona remove"
)

func managementCommand(positionals []string, forcedPrompt bool) (command, bool) {
	if forcedPrompt {
		return "", false
	}
	switch strings.Join(positionals, " ") {
	case "config":
		return commandConfigShow, true
	case string(commandConfigInit):
		return commandConfigInit, true
	case string(commandConfigShow):
		return commandConfigShow, true
	case string(commandTierList):
		return commandTierList, true
	case string(commandTierAdd):
		return commandTierAdd, true
	case string(commandTierUpdate):
		return commandTierUpdate, true
	case string(commandTierRemove):
		return commandTierRemove, true
	case string(commandHarnessList):
		return commandHarnessList, true
	case string(commandPersonaList):
		return commandPersonaList, true
	case string(commandPersonaShow):
		return commandPersonaShow, true
	case string(commandPersonaAdd):
		return commandPersonaAdd, true
	case string(commandPersonaUpdate):
		return commandPersonaUpdate, true
	case string(commandPersonaRemove):
		return commandPersonaRemove, true
	default:
		return "", false
	}
}

func meetingCommand(positionals []string, forcedPrompt bool) bool {
	return !forcedPrompt && len(positionals) > 0 && positionals[0] == "meeting"
}

func runManagement(ctx context.Context, selected command, values options, invocationDir string, registry router.Registry, stdout io.Writer) (int, error) {
	if err := managementOptions(values, selected); err != nil {
		return exitUsage, err
	}
	switch selected {
	case commandConfigInit:
		return initializeConfiguration(ctx, invocationDir, stdout)
	case commandConfigShow:
		configuration, err := loadConfiguration(ctx, invocationDir, registry)
		if err != nil {
			return exitUsage, err
		}
		return reportConfiguration(*configuration, values.JSON, stdout)
	case commandTierList:
		configuration, err := loadConfiguration(ctx, invocationDir, registry)
		if err != nil {
			return exitUsage, err
		}
		return reportTiers(*configuration, values.JSON, stdout)
	case commandTierAdd, commandTierUpdate, commandTierRemove:
		return mutateTier(ctx, selected, values, invocationDir, registry, stdout)
	case commandHarnessList:
		return listHarnesses(ctx, registry, values.JSON, stdout)
	case commandPersonaList:
		return listPersonas(ctx, invocationDir, values.JSON, stdout)
	case commandPersonaShow:
		return showPersona(ctx, invocationDir, values, stdout)
	case commandPersonaAdd, commandPersonaUpdate, commandPersonaRemove:
		return mutatePersona(ctx, selected, values, invocationDir, stdout)
	default:
		return exitFailed, fmt.Errorf("unrecognized management command %q", selected)
	}
}

func managementOptions(values options, selected command) error {
	if values.Tier.Set || values.Reply.Set || values.Persona.Set || values.CWD.Set || values.Label.Set || values.Timeout.Set || values.PromptFile.Set {
		return &usageError{message: fmt.Sprintf("dispatch options are present with command %q", selected)}
	}
	isTierMutation := selected == commandTierAdd || selected == commandTierUpdate || selected == commandTierRemove
	isPersonaMutation := selected == commandPersonaShow || selected == commandPersonaAdd || selected == commandPersonaUpdate || selected == commandPersonaRemove
	if !isTierMutation && !isPersonaMutation && (values.Name.Set || values.Description.Set || len(values.Routes) != 0 || values.Default) {
		return &usageError{message: fmt.Sprintf("tier options are present with command %q", selected)}
	}
	if isPersonaMutation && (len(values.Routes) != 0 || values.Default) {
		return &usageError{message: fmt.Sprintf("tier options are present with command %q", selected)}
	}
	return nil
}

type configurationReport struct {
	Source      configurationSource `json:"source"`
	DefaultTier string              `json:"defaultTier"`
	Tiers       []tierReport        `json:"tiers"`
}

type configurationSource struct {
	Path         string `json:"path"`
	SearchedFrom string `json:"searchedFrom"`
	SearchedTo   string `json:"searchedTo"`
}

type tierReport struct {
	Name        string        `json:"name"`
	Origin      config.Origin `json:"origin"`
	Description string        `json:"description,omitempty"`
	Routes      []routeReport `json:"routes"`
}

type routeReport struct {
	Harness              string   `json:"harness"`
	Model                *string  `json:"model"`
	FallbackBelowPercent *float64 `json:"fallbackBelowPercent"`
}

func reportConfiguration(configuration config.Config, asJSON bool, stdout io.Writer) (int, error) {
	report := projectConfiguration(configuration)
	if asJSON {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if _, err := fmt.Fprintf(stdout, "source: %s\nsearched: %s to %s\ndefault tier: %s\n", report.Source.Path, report.Source.SearchedFrom, report.Source.SearchedTo, defaultTierText(report.DefaultTier)); err != nil {
		return exitFailed, err
	}
	return writeTiers(report.Tiers, stdout)
}

func reportTiers(configuration config.Config, asJSON bool, stdout io.Writer) (int, error) {
	tiers := projectConfiguration(configuration).Tiers
	if asJSON {
		encoded, err := json.MarshalIndent(tiers, "", "  ")
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	return writeTiers(tiers, stdout)
}

func writeTiers(tiers []tierReport, stdout io.Writer) (int, error) {
	for _, tier := range tiers {
		if _, err := fmt.Fprintf(stdout, "%s (%s)\n", tier.Name, tier.Origin); err != nil {
			return exitFailed, err
		}
		if tier.Description != "" {
			if _, err := fmt.Fprintf(stdout, "  description: %s\n", tier.Description); err != nil {
				return exitFailed, err
			}
		}
		if _, err := fmt.Fprintln(stdout, "  routes:"); err != nil {
			return exitFailed, err
		}
		for _, route := range tier.Routes {
			model := "unpinned"
			if route.Model != nil {
				model = *route.Model
			}
			fallback := ""
			if route.FallbackBelowPercent != nil {
				fallback = fmt.Sprintf(", fallback below %g%%", *route.FallbackBelowPercent)
			}
			if _, err := fmt.Fprintf(stdout, "    - %s (model: %s%s)\n", route.Harness, model, fallback); err != nil {
				return exitFailed, err
			}
		}
	}

	return exitOK, nil
}

func defaultTierText(defaultTier string) string {
	if defaultTier == "" {
		return "none"
	}

	return defaultTier
}

func projectConfiguration(configuration config.Config) configurationReport {
	tiers := make([]tierReport, 0, len(configuration.Tiers))
	for _, name := range configuration.TierNames() {
		tier := configuration.Tiers[name]
		routes := make([]routeReport, 0, len(tier.Routes))
		for _, route := range tier.Routes {
			routes = append(routes, routeReport{Harness: route.Harness, Model: route.Model, FallbackBelowPercent: route.FallbackBelowPercent})
		}
		tiers = append(tiers, tierReport{Name: tier.Name, Origin: tier.Origin, Description: tier.Description, Routes: routes})
	}

	return configurationReport{Source: configurationSource{Path: configuration.Source.Path, SearchedFrom: configuration.Source.SearchedFrom, SearchedTo: configuration.Source.SearchedTo}, DefaultTier: configuration.DefaultTier, Tiers: tiers}
}

func initializeConfiguration(ctx context.Context, invocationDir string, stdout io.Writer) (int, error) {
	if err := ctx.Err(); err != nil {
		return exitFailed, err
	}
	path := filepath.Join(invocationDir, config.ConfigFileName)
	if _, err := os.Lstat(path); err == nil {
		return exitUsage, harness.NewError(harness.ErrorCodeConfigInvalid, fmt.Sprintf("configuration file %q already exists", path), nil)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return exitFailed, fmt.Errorf("examine configuration path %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return exitFailed, fmt.Errorf("create configuration directory %q: %w", filepath.Dir(path), err)
	}
	configuration := config.Config{Version: config.SchemaVersion, Tiers: map[string]config.Tier{}}
	contents, err := json.MarshalIndent(projectConfigurationFile(configuration), "", "  ")
	if err != nil {
		return exitFailed, err
	}
	if err := writeConfigurationFile(path, append(contents, '\n')); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return exitUsage, harness.NewError(harness.ErrorCodeConfigInvalid, fmt.Sprintf("configuration file %q already exists", path), nil)
		}
		return exitFailed, err
	}
	if _, err := fmt.Fprintf(stdout, "configuration: %s\ntiers: none; configuration file names no tiers, and dispatch resolves only when at least one exists\ndefault tier: none\n", path); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

type configurationFile struct {
	Version     int                 `json:"version"`
	DefaultTier string              `json:"defaultTier,omitempty"`
	Tiers       map[string]tierFile `json:"tiers"`
}

type tierFile struct {
	Description string      `json:"description,omitempty"`
	Routes      []routeFile `json:"routes"`
}

type routeFile struct {
	Harness              string   `json:"harness"`
	Model                *string  `json:"model,omitempty"`
	FallbackBelowPercent *float64 `json:"fallbackBelowPercent,omitempty"`
}

func projectConfigurationFile(configuration config.Config) configurationFile {
	tiers := make(map[string]tierFile, len(configuration.Tiers))
	for _, name := range configuration.TierNames() {
		tier := configuration.Tiers[name]
		routes := make([]routeFile, 0, len(tier.Routes))
		for _, route := range tier.Routes {
			routes = append(routes, routeFile{Harness: route.Harness, Model: route.Model, FallbackBelowPercent: route.FallbackBelowPercent})
		}
		tiers[name] = tierFile{Description: tier.Description, Routes: routes}
	}

	return configurationFile{Version: configuration.Version, DefaultTier: configuration.DefaultTier, Tiers: tiers}
}

func writeConfigurationFile(path string, contents []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config.json-")
	if err != nil {
		return fmt.Errorf("create temporary configuration file in %q: %w", filepath.Dir(path), err)
	}
	temporaryPath := temporary.Name()
	if _, err := temporary.Write(contents); err != nil {
		return removeTemporaryConfiguration(temporary, temporaryPath, fmt.Errorf("write temporary configuration file %q: %w", temporaryPath, err))
	}
	if err := temporary.Chmod(0o644); err != nil {
		return removeTemporaryConfiguration(temporary, temporaryPath, fmt.Errorf("set mode on temporary configuration file %q: %w", temporaryPath, err))
	}
	if err := temporary.Sync(); err != nil {
		return removeTemporaryConfiguration(temporary, temporaryPath, fmt.Errorf("sync temporary configuration file %q: %w", temporaryPath, err))
	}
	if err := temporary.Close(); err != nil {
		return removeTemporaryConfiguration(nil, temporaryPath, fmt.Errorf("close temporary configuration file %q: %w", temporaryPath, err))
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return removeTemporaryConfiguration(nil, temporaryPath, err)
	}
	if err := os.Remove(temporaryPath); err != nil {
		return fmt.Errorf("remove temporary configuration file %q: %w", temporaryPath, err)
	}

	return nil
}

func removeTemporaryConfiguration(temporary *os.File, path string, cause error) error {
	if temporary != nil {
		if err := temporary.Close(); err != nil && !errors.Is(err, fs.ErrInvalid) {
			cause = errors.Join(cause, fmt.Errorf("close temporary configuration file %q: %w", path, err))
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.Join(cause, fmt.Errorf("remove temporary configuration file %q: %w", path, err))
	}

	return cause
}

func mutateTier(ctx context.Context, selected command, values options, invocationDir string, registry router.Registry, stdout io.Writer) (int, error) {
	path := filepath.Join(invocationDir, config.ConfigFileName)
	if !values.Name.Set || values.Name.Value == "" {
		return exitUsage, &usageError{message: fmt.Sprintf("command %q has no --name", selected)}
	}
	if selected == commandTierRemove {
		if values.Description.Set || len(values.Routes) != 0 || values.Default {
			return exitUsage, &usageError{message: "tier remove has route, description, or default options"}
		}
		configuration, err := loadConfiguration(ctx, invocationDir, registry)
		if err != nil {
			return exitUsage, err
		}
		if configuration.DefaultTier == values.Name.Value {
			if err := config.SetDefaultTier(ctx, path, config.NewHarnesses(registry), ""); err != nil {
				return exitUsage, err
			}
		}
		if err := config.RemoveTier(ctx, path, config.NewHarnesses(registry), values.Name.Value); err != nil {
			return exitUsage, err
		}
		if _, err := fmt.Fprintf(stdout, "removed tier: %s\n", values.Name.Value); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if selected == commandTierAdd {
		if len(values.Routes) == 0 {
			return exitUsage, &usageError{message: "tier add has no --route"}
		}
		tier := config.Tier{Name: values.Name.Value, Description: values.Description.Value, Routes: routesFrom(values.Routes)}
		if err := config.AddTier(ctx, path, config.NewHarnesses(registry), tier); err != nil {
			return exitUsage, err
		}
		if values.Default {
			if err := config.SetDefaultTier(ctx, path, config.NewHarnesses(registry), tier.Name); err != nil {
				return exitUsage, err
			}
		}
		if _, err := fmt.Fprintf(stdout, "added tier: %s\n", tier.Name); err != nil {
			return exitFailed, err
		}

		return exitOK, nil
	}

	var description *string
	if values.Description.Set {
		value := values.Description.Value
		description = &value
	}
	changed, err := config.UpdateTier(ctx, path, config.NewHarnesses(registry), config.TierUpdate{
		Name:           values.Name.Value,
		Description:    description,
		Routes:         routesFrom(values.Routes),
		ReplaceRoutes:  len(values.Routes) != 0,
		SetDefaultTier: values.Default,
	})
	if err != nil {
		return exitUsage, err
	}
	if !changed {
		if _, err := fmt.Fprintln(stdout, "tier update: no fields supplied; configuration unchanged"); err != nil {
			return exitFailed, err
		}

		return exitOK, nil
	}
	if _, err := fmt.Fprintf(stdout, "update tier: %s\n", values.Name.Value); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func routesFrom(options []routeOption) []config.Route {
	routes := make([]config.Route, len(options))
	for index, option := range options {
		routes[index] = config.Route{Harness: option.Harness, Model: option.Model, FallbackBelowPercent: option.FallbackBelowPercent}
	}

	return routes
}

func writeHelp(ctx context.Context, invocationDir string, registry router.Registry, stdout io.Writer) (int, error) {
	configuration, configErr := loadConfiguration(ctx, invocationDir, registry)
	if _, err := fmt.Fprint(stdout, renderHelp(configuration, configErr)); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func renderHelp(configuration *config.Config, configurationErr error) string {
	if configurationErr != nil || configuration == nil {
		return help + "\nCONFIGURED TIERS\n  none resolved\n"
	}

	var rendered strings.Builder
	rendered.WriteString(help)
	rendered.WriteString("\nCONFIGURED TIERS\n")
	for _, name := range configuration.TierNames() {
		tier := configuration.Tiers[name]
		fmt.Fprintf(&rendered, "  %s\n", tier.Name)
		if tier.Description != "" {
			fmt.Fprintf(&rendered, "    description: %s\n", tier.Description)
		}
	}

	return rendered.String()
}
