package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/persona"
	"github.com/on-mission/landing/internal/router"
)

type harnessReport struct {
	Name     string                  `json:"name"`
	Status   harness.DetectionStatus `json:"status"`
	Path     string                  `json:"path,omitempty"`
	Capacity capacityReport          `json:"capacity"`
	Models   []string                `json:"models"`
	Detail   string                  `json:"detail,omitempty"`
}

type capacityReport struct {
	Known   bool             `json:"known"`
	Buckets []harness.Bucket `json:"buckets"`
}

func listHarnesses(ctx context.Context, registry router.Registry, asJSON bool, stdout io.Writer) (int, error) {
	ids := registry.IDs()
	slices.Sort(ids)
	reports := make([]harnessReport, 0, len(ids))
	for _, id := range ids {
		adapter, err := registry.Resolve(id)
		if err != nil {
			return exitFailed, err
		}
		detection := adapter.Detect(ctx)
		reports = append(reports, harnessReport{Name: id, Status: detection.Status, Path: detection.Path, Capacity: capacityReport{Known: detection.Capacity.IsKnown(), Buckets: detection.Capacity.Buckets()}, Models: adapter.Models(), Detail: detection.Detail})
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
		if _, err := fmt.Fprintf(stdout, "  capacity: %s\n  models: %s\n", capacityText(report.Capacity), modelsText(report.Models)); err != nil {
			return exitFailed, err
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

func modelsText(models []string) string {
	if len(models) == 0 {
		return "none"
	}

	return strings.Join(models, ", ")
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
