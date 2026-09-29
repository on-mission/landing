package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/on-mission/landing/internal/paths"
)

// AddTier adds tier to the complete project tier set at configPath.
func AddTier(ctx context.Context, configPath string, harnesses Harnesses, tier Tier) error {
	_, err := mutate(ctx, configPath, harnesses, func(configuration *Config) (bool, error) {
		if _, ok := configuration.Tiers[tier.Name]; ok {
			return false, tierExistsError(tier.Name, configuration.TierNames())
		}

		configuration.Tiers[tier.Name] = projectTierFrom(tier)

		return true, nil
	})

	return err
}

// TierUpdate describes the explicitly supplied fields of an existing tier.
type TierUpdate struct {
	Name           string
	Description    *string
	Routes         []Route
	ReplaceRoutes  bool
	SetDefaultTier bool
}

// UpdateTier applies the explicitly supplied fields to the configured tier with the
// same name as update.
func UpdateTier(ctx context.Context, configPath string, harnesses Harnesses, update TierUpdate) (bool, error) {
	return mutate(ctx, configPath, harnesses, func(configuration *Config) (bool, error) {
		tier, ok := configuration.Tiers[update.Name]
		if !ok {
			return false, tierMissingError(update.Name, configuration.TierNames())
		}
		if update.Description == nil && !update.ReplaceRoutes && !update.SetDefaultTier {
			return false, nil
		}

		if update.Description != nil {
			tier.Description = *update.Description
		}
		if update.ReplaceRoutes {
			tier.Routes = append([]Route(nil), update.Routes...)
		}
		configuration.Tiers[update.Name] = projectTierFrom(tier)
		if update.SetDefaultTier {
			configuration.DefaultTier = update.Name
		}

		return true, nil
	})
}

// RemoveTier removes name from the complete project tier set at configPath.
func RemoveTier(ctx context.Context, configPath string, harnesses Harnesses, name string) error {
	_, err := mutate(ctx, configPath, harnesses, func(configuration *Config) (bool, error) {
		if _, ok := configuration.Tiers[name]; !ok {
			return false, tierMissingError(name, configuration.TierNames())
		}

		delete(configuration.Tiers, name)

		return true, nil
	})

	return err
}

// SetDefaultTier sets the default tier. An empty name removes the configured default.
func SetDefaultTier(ctx context.Context, configPath string, harnesses Harnesses, name string) error {
	_, err := mutate(ctx, configPath, harnesses, func(configuration *Config) (bool, error) {
		configuration.DefaultTier = name

		return true, nil
	})

	return err
}

type writableProjectConfigFile struct {
	Version     int                    `json:"version"`
	DefaultTier string                 `json:"defaultTier,omitempty"`
	Latest      map[string]string      `json:"latest,omitempty"`
	Tiers       map[string]projectTier `json:"tiers"`
}

func mutate(ctx context.Context, configPath string, harnesses Harnesses, change func(*Config) (bool, error)) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	path, err := filepath.Abs(configPath)
	if err != nil {
		return false, invalidConfigError(
			fmt.Sprintf("configuration path %s could not be made absolute", paths.Display(configPath)),
			err,
		)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return false, invalidConfigError(
			fmt.Sprintf("could not read configuration file %s", paths.Display(path)),
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	file, err := decodeProjectConfig(contents)
	if err != nil {
		return false, invalidConfigError(
			fmt.Sprintf("could not parse configuration file %s", paths.Display(path)),
			err,
		)
	}
	if file.Version != SchemaVersion {
		return false, invalidConfigError(
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

	configuration, err := resolveProjectFile(file)
	if err != nil {
		return false, projectConfigResolutionError(path, err)
	}
	changed, err := change(&configuration)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	if err := validate(configuration, harnesses); err != nil {
		return false, invalidConfigError(
			fmt.Sprintf("configuration file %s is invalid", paths.Display(path)),
			err,
		)
	}
	if problems := validateModels(ctx, configuration, harnesses); len(problems) > 0 {
		return false, invalidConfigError(
			fmt.Sprintf("configuration file %s has models that could not be validated", paths.Display(path)),
			errors.New(strings.Join(problems, "; ")),
		)
	}

	written, err := marshalProjectConfig(configuration)
	if err != nil {
		return false, invalidConfigError(
			fmt.Sprintf("could not encode configuration file %s", paths.Display(path)),
			err,
		)
	}
	if err := writeProjectConfigAtomically(ctx, path, written); err != nil {
		return false, err
	}

	return true, nil
}

func projectTierFrom(tier Tier) Tier {
	return Tier{
		Name:        tier.Name,
		Description: tier.Description,
		Routes:      append([]Route(nil), tier.Routes...),
		Origin:      OriginProject,
	}
}

func tierExistsError(name string, tiers []string) error {
	return invalidConfigError(
		fmt.Sprintf("tier %q already exists; configured tiers are %s", name, formatTierNames(tiers)),
		nil,
	)
}

func tierMissingError(name string, tiers []string) error {
	return invalidConfigError(
		fmt.Sprintf("tier %q does not exist; configured tiers are %s", name, formatTierNames(tiers)),
		nil,
	)
}

func marshalProjectConfig(configuration Config) ([]byte, error) {
	tiers := make(map[string]projectTier, len(configuration.Tiers))
	for _, name := range configuration.TierNames() {
		tier := configuration.Tiers[name]
		tiers[name] = projectTier{
			Description: tier.Description,
			Routes:      projectRoutes(tier.Routes),
		}
	}

	contents, err := json.MarshalIndent(writableProjectConfigFile{
		Version:     configuration.Version,
		DefaultTier: configuration.DefaultTier,
		Latest:      configuration.Latest,
		Tiers:       tiers,
	}, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(contents, '\n'), nil
}

func projectRoutes(routes []Route) []projectRoute {
	project := make([]projectRoute, len(routes))
	for index, route := range routes {
		project[index] = projectRoute{
			Harness:              route.Harness,
			Model:                route.Model,
			FallbackBelowPercent: route.FallbackBelowPercent,
		}
	}

	return project
}

func writeProjectConfigAtomically(ctx context.Context, path string, contents []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return invalidConfigError(fmt.Sprintf("could not examine configuration file %s", paths.Display(path)), err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), ".config.json-")
	if err != nil {
		return invalidConfigError(
			fmt.Sprintf("could not create temporary configuration file beside %s", paths.Display(path)),
			err,
		)
	}
	temporaryPath := temporary.Name()
	if _, err := temporary.Write(contents); err != nil {
		return removeTemporaryProjectConfig(temporary, temporaryPath, invalidConfigError(
			fmt.Sprintf("could not write temporary configuration file %s", paths.Display(temporaryPath)),
			err,
		))
	}
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return removeTemporaryProjectConfig(temporary, temporaryPath, invalidConfigError(
			fmt.Sprintf("could not set permissions on temporary configuration file %s", paths.Display(temporaryPath)),
			err,
		))
	}
	if err := temporary.Sync(); err != nil {
		return removeTemporaryProjectConfig(temporary, temporaryPath, invalidConfigError(
			fmt.Sprintf("could not sync temporary configuration file %s", paths.Display(temporaryPath)),
			err,
		))
	}
	if err := temporary.Close(); err != nil {
		return removeTemporaryProjectConfig(nil, temporaryPath, invalidConfigError(
			fmt.Sprintf("could not close temporary configuration file %s", paths.Display(temporaryPath)),
			err,
		))
	}
	if err := ctx.Err(); err != nil {
		return removeTemporaryProjectConfig(nil, temporaryPath, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return removeTemporaryProjectConfig(nil, temporaryPath, invalidConfigError(
			fmt.Sprintf("could not replace configuration file %s", paths.Display(path)),
			err,
		))
	}

	return nil
}

func removeTemporaryProjectConfig(temporary *os.File, path string, cause error) error {
	if temporary != nil {
		if err := temporary.Close(); err != nil && !errors.Is(err, fs.ErrInvalid) {
			cause = errors.Join(cause, invalidConfigError(
				fmt.Sprintf("could not close temporary configuration file %s", paths.Display(path)),
				err,
			))
		}
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errors.Join(cause, invalidConfigError(
			fmt.Sprintf("could not remove temporary configuration file %s", paths.Display(path)),
			err,
		))
	}

	return cause
}
