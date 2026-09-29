package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/persona"
	"github.com/on-mission/landing/internal/router"
)

type harnessReport struct {
	Name         string                        `json:"name"`
	Status       harness.DetectionStatus       `json:"status"`
	Path         string                        `json:"path,omitempty"`
	Capacity     capacityReport                `json:"capacity"`
	Models       []string                      `json:"models"`
	Latest       string                        `json:"latest,omitempty"`
	ModelCatalog harness.ModelCatalogAuthority `json:"modelCatalog"`
	Detail       string                        `json:"detail,omitempty"`
}

type capacityReport struct {
	Known   bool             `json:"known"`
	NoGauge bool             `json:"noGauge,omitempty"`
	Buckets []harness.Bucket `json:"buckets"`
}

// sortedHarnessIDs names every harness Landing supports, for errors that tell
// a caller what they could have written instead.
func sortedHarnessIDs(registry router.Registry) []string {
	ids := registry.IDs()
	slices.Sort(ids)

	return ids
}

func listHarnesses(ctx context.Context, registry router.Registry, asJSON bool, stdout io.Writer) (int, error) {
	ids := sortedHarnessIDs(registry)
	reports := make([]harnessReport, 0, len(ids))
	for _, id := range ids {
		adapter, err := registry.Resolve(id)
		if err != nil {
			return exitFailed, err
		}
		detection := adapter.Detect(ctx)
		catalog := adapter.ModelCatalog()
		reports = append(reports, harnessReport{Name: id, Status: detection.Status, Path: detection.Path, Capacity: capacityReport{Known: detection.Capacity.IsKnown(), NoGauge: !detection.Capacity.HasGauge(), Buckets: detection.Capacity.Buckets()}, Models: catalog.Models, Latest: catalog.Latest, ModelCatalog: catalog.Authority, Detail: detection.Detail})
	}
	if asJSON {
		encoded, err := json.MarshalIndent(reports, "", "  ")
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	return writeHarnessReports(reports, stdout)
}

func writeHarnessReports(reports []harnessReport, stdout io.Writer) (int, error) {
	for _, report := range reports {
		if _, err := fmt.Fprintf(stdout, "%s: %s\n", report.Name, report.Status); err != nil {
			return exitFailed, err
		}
		if report.Path != "" {
			if _, err := fmt.Fprintf(stdout, "  path: %s\n", report.Path); err != nil {
				return exitFailed, err
			}
		}
		if _, err := fmt.Fprintf(stdout, "  capacity: %s\n  models: %s\n", capacityText(report.Capacity), modelsText(report.Models, report.ModelCatalog)); err != nil {
			return exitFailed, err
		}
		if report.Latest != "" {
			if _, err := fmt.Fprintf(stdout, "  latest: %s\n", report.Latest); err != nil {
				return exitFailed, err
			}
		}
		if report.Detail != "" {
			if _, err := fmt.Fprintf(stdout, "  detail: %s\n", report.Detail); err != nil {
				return exitFailed, err
			}
		}
	}

	return exitOK, nil
}

func capacityText(capacity capacityReport) string {
	if capacity.NoGauge {
		return "no gauge"
	}
	if !capacity.Known {
		return "unknown"
	}
	if len(capacity.Buckets) == 0 {
		return "known"
	}
	parts := make([]string, 0, len(capacity.Buckets))
	for _, bucket := range capacity.Buckets {
		parts = append(parts, fmt.Sprintf("%s %.2f%% used", bucket.ID, bucket.UsedPercent))
	}

	return strings.Join(parts, ", ")
}

func modelsText(models []string, authority harness.ModelCatalogAuthority) string {
	if len(models) == 0 {
		return "none"
	}

	text := strings.Join(models, ", ")
	if authority == harness.ModelCatalogAdvisory {
		return text + " (advisory; may be incomplete)"
	}

	return text
}

type modelReport struct {
	Route        string                        `json:"route"`
	Harness      string                        `json:"harness"`
	Model        string                        `json:"model,omitempty"`
	Status       harness.DetectionStatus       `json:"status"`
	Tiers        []string                      `json:"tiers"`
	Selectable   bool                          `json:"selectable"`
	ModelCatalog harness.ModelCatalogAuthority `json:"modelCatalog"`
	Latest       string                        `json:"latest,omitempty"`
	LatestSource string                        `json:"latestSource,omitempty"`
}

// listModels reports every route a caller can name, from the harnesses Landing
// supports and the tiers this project configured, in the harness or
// harness/model form --model takes.
//
// It runs harness detection so the report says what is usable now, but
// deliberately skips capacity probes. This is the lookup a caller makes before
// choosing a route, so it stays cheap; measured availability is what
// `landing harness list` is for.
func listModels(ctx context.Context, invocationDir string, registry router.Registry, asJSON bool, stdout io.Writer) (int, error) {
	membership := tierMembership(ctx, invocationDir, registry)
	configuration, configurationErr := loadConfiguration(ctx, invocationDir, registry)
	reports := make([]modelReport, 0, len(membership))
	for _, id := range sortedHarnessIDs(registry) {
		adapter, err := registry.Resolve(id)
		if err != nil {
			return exitFailed, err
		}
		status := adapter.Detect(ctx).Status
		catalog := adapter.ModelCatalog()
		latest, latestSource := effectiveLatest(id, catalog, configuration, configurationErr)
		models := catalog.Models
		for _, model := range models {
			route := config.Route{Harness: id, Model: &model}
			reports = append(reports, modelReport{Route: route.String(), Harness: id, Model: model, Status: status, Tiers: membership[route.String()], Selectable: true, ModelCatalog: catalog.Authority, Latest: latest, LatestSource: latestSource})
		}
		for route, tiers := range membership {
			harnessID, model, hasModel := strings.Cut(route, "/")
			if harnessID != id || !hasModel || slices.Contains(models, model) {
				continue
			}
			reports = append(reports, modelReport{Route: route, Harness: id, Model: model, Status: status, Tiers: tiers, Selectable: true, ModelCatalog: catalog.Authority, Latest: latest, LatestSource: latestSource})
		}
		// A harness may expose no model at all, and a tier may configure one
		// without naming a model. Either way it is an execution path a caller
		// can name with --model.
		_, configuredBare := membership[id]
		if configuredBare || len(models) == 0 {
			reports = append(reports, modelReport{Route: id, Harness: id, Status: status, Tiers: membership[id], Selectable: true, ModelCatalog: catalog.Authority, Latest: latest, LatestSource: latestSource})
		}
	}
	slices.SortFunc(reports, func(first modelReport, second modelReport) int {
		return strings.Compare(first.Route, second.Route)
	})
	if asJSON {
		encoded, err := json.MarshalIndent(reports, "", "  ")
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
			return exitFailed, err
		}

		return exitOK, nil
	}

	return writeModelReports(reports, stdout)
}

func effectiveLatest(id string, catalog harness.ModelCatalog, configuration *config.Config, configurationErr error) (string, string) {
	if configurationErr == nil && configuration != nil {
		if model, ok := configuration.Latest[id]; ok {
			return model, "project override"
		}
	}
	if catalog.Latest == "" {
		return "", ""
	}

	return catalog.Latest, "landing default"
}

// tierMembership maps a route to the tiers that configure it. A project whose
// configuration does not resolve still gets a model list: which routes exist
// is a property of the installed harnesses, not of this project's policy.
func tierMembership(ctx context.Context, invocationDir string, registry router.Registry) map[string][]string {
	membership := make(map[string][]string)
	configuration, err := loadConfiguration(ctx, invocationDir, registry)
	if err != nil {
		return membership
	}
	for _, name := range configuration.TierNames() {
		for _, route := range configuration.Tiers[name].Routes {
			identifier := route.String()
			if slices.Contains(membership[identifier], name) {
				continue
			}
			membership[identifier] = append(membership[identifier], name)
		}
	}

	return membership
}

func writeModelReports(reports []modelReport, stdout io.Writer) (int, error) {
	if len(reports) == 0 {
		if _, err := fmt.Fprintln(stdout, "no models"); err != nil {
			return exitFailed, err
		}

		return exitOK, nil
	}
	routeWidth := 0
	statusWidth := 0
	for _, report := range reports {
		routeWidth = max(routeWidth, len(report.Route))
		statusWidth = max(statusWidth, len(report.Status))
	}
	for _, report := range reports {
		line := fmt.Sprintf("%-*s  %-*s  tiers: %s", routeWidth, report.Route, statusWidth, report.Status, tiersText(report.Tiers))
		if report.ModelCatalog == harness.ModelCatalogAdvisory {
			line += "  (advisory catalog; other models may be available)"
		}
		if report.Latest != "" {
			line += "  latest: " + report.Latest + " (" + report.LatestSource + ")"
		}
		if _, err := fmt.Fprintln(stdout, line); err != nil {
			return exitFailed, err
		}
	}

	return exitOK, nil
}

func tiersText(tiers []string) string {
	if len(tiers) == 0 {
		return "none"
	}

	return strings.Join(tiers, ", ")
}

func listPersonas(ctx context.Context, invocationDir string, asJSON bool, stdout io.Writer) (int, error) {
	personas, err := persona.List(ctx, invocationDir)
	if err != nil {
		var harnessError *harness.Error
		if errors.As(err, &harnessError) && harnessError.Code == harness.ErrorCodePersonaNotFound {
			return writePersonaReports(nil, asJSON, stdout)
		}
		return exitFailed, err
	}
	return writePersonaReports(personas, asJSON, stdout)
}

type personaReport struct {
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	Instructions   string   `json:"instructions,omitempty"`
	ReferenceFiles []string `json:"referenceFiles"`
}

func writePersonaReports(personas []persona.Persona, asJSON bool, stdout io.Writer) (int, error) {
	reports := make([]personaReport, 0, len(personas))
	for _, selected := range personas {
		reports = append(reports, personaReport{Name: selected.Name, Description: selected.Description, Instructions: selected.Instructions, ReferenceFiles: selected.ReferenceFiles})
	}
	if asJSON {
		encoded, err := json.MarshalIndent(reports, "", "  ")
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if len(reports) == 0 {
		if _, err := fmt.Fprintln(stdout, "no personas"); err != nil {
			return exitFailed, err
		}

		return exitOK, nil
	}
	for _, report := range reports {
		if report.Description == "" {
			if _, err := fmt.Fprintln(stdout, report.Name); err != nil {
				return exitFailed, err
			}
			continue
		}
		if _, err := fmt.Fprintf(stdout, "%s: %s\n", report.Name, report.Description); err != nil {
			return exitFailed, err
		}
	}

	return exitOK, nil
}
