package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/dispatch"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/meeting"
)

type meetingResult struct {
	Question     string            `json:"question"`
	Participants []string          `json:"participants"`
	Positions    []meetingPosition `json:"positions"`
	Arbiter      *meetingReading   `json:"arbiter"`
	Failures     []meetingFailure  `json:"failures"`
}

type meetingPosition struct {
	Persona string            `json:"persona"`
	Route   *meetingRoute     `json:"route"`
	Status  harness.JobStatus `json:"status"`
	Answer  *string           `json:"answer"`
	Error   string            `json:"error,omitempty"`
	Cast    bool              `json:"cast"`
}

type meetingReading struct {
	Route  *meetingRoute     `json:"route"`
	Status harness.JobStatus `json:"status"`
	Answer *string           `json:"answer"`
	Error  string            `json:"error,omitempty"`
}

type meetingFailure struct {
	Stage   string `json:"stage"`
	Persona string `json:"persona,omitempty"`
	Error   string `json:"error"`
}

type meetingRoute struct {
	Harness       string  `json:"harness"`
	Model         *string `json:"model"`
	RoutedBecause *string `json:"routedBecause"`
}

func reportMeeting(result meeting.Result, asJSON bool, stdout io.Writer) (int, error) {
	projected := projectMeetingResult(result)
	if asJSON {
		encoded, err := json.MarshalIndent(projected, "", "  ")
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
			return exitFailed, err
		}
	} else if err := writeMeetingResult(projected, stdout); err != nil {
		return exitFailed, err
	}
	if meetingFailed(result) {
		return exitFailed, nil
	}

	return exitOK, nil
}

func projectMeetingResult(result meeting.Result) meetingResult {
	positions := make([]meetingPosition, 0, len(result.Positions))
	for _, position := range result.Positions {
		positions = append(positions, projectMeetingPosition(position, result.Casts))
	}
	failures := make([]meetingFailure, 0, len(result.Failures))
	for _, failure := range result.Failures {
		failures = append(failures, meetingFailure{Stage: failure.Stage, Persona: failure.Persona, Error: failure.Error})
	}
	projected := meetingResult{Question: result.Question, Participants: append([]string(nil), result.Participants...), Positions: positions, Failures: failures}
	if result.Arbiter == nil {
		return projected
	}
	reading := meetingReading{Status: harness.JobStatusFailed, Error: result.Arbiter.Failure}
	if result.Arbiter.Response != nil {
		reading.Route = meetingRouteFor(result.Arbiter.Response)
		reading.Status = result.Arbiter.Response.Status
		reading.Answer = result.Arbiter.Response.Output
	}
	projected.Arbiter = &reading

	return projected
}

func projectMeetingPosition(position meeting.Position, casts map[string]config.Route) meetingPosition {
	projected := meetingPosition{Persona: position.Persona, Status: harness.JobStatusFailed, Error: position.Failure}
	_, projected.Cast = casts[position.Persona]
	if position.Response != nil {
		projected.Route = meetingRouteFor(position.Response)
		projected.Status = position.Response.Status
		projected.Answer = position.Response.Output
	}

	return projected
}

func meetingRouteFor(response *dispatch.Response) *meetingRoute {
	if response == nil || response.Provider == "" {
		return nil
	}

	return &meetingRoute{Harness: response.Provider, Model: response.Model, RoutedBecause: response.RoutedBecause}
}

func writeMeetingResult(result meetingResult, stdout io.Writer) error {
	for _, position := range result.Positions {
		if position.Answer == nil || *position.Answer == "" {
			continue
		}
		if _, err := fmt.Fprintf(stdout, "## %s\n%s\n", position.Persona, strings.TrimRight(*position.Answer, "\n")); err != nil {
			return err
		}
	}
	if result.Arbiter != nil && result.Arbiter.Answer != nil && *result.Arbiter.Answer != "" {
		if _, err := fmt.Fprintf(stdout, "## arbiter\n%s\n", strings.TrimRight(*result.Arbiter.Answer, "\n")); err != nil {
			return err
		}
	}
	for _, persona := range result.Participants {
		position, ok := positionFor(persona, result.Positions)
		if !ok || position.Route == nil {
			continue
		}
		model := "unpinned"
		if position.Route.Model != nil {
			model = *position.Route.Model
		}
		cast := "not cast"
		if position.Cast {
			cast = "cast"
		}
		if _, err := fmt.Fprintf(stdout, "participant: %s (harness: %s, model: %s, %s)\n", persona, position.Route.Harness, model, cast); err != nil {
			return err
		}
	}
	for _, failure := range result.Failures {
		if failure.Persona == "" {
			if _, err := fmt.Fprintf(stdout, "%s: %s\n", failure.Stage, failure.Error); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(stdout, "%s %s: %s\n", failure.Stage, failure.Persona, failure.Error); err != nil {
			return err
		}
	}

	return nil
}

func positionFor(persona string, positions []meetingPosition) (meetingPosition, bool) {
	for _, position := range positions {
		if position.Persona == persona {
			return position, true
		}
	}

	return meetingPosition{}, false
}

func meetingFailed(result meeting.Result) bool {
	return len(result.Failures) > 0
}
