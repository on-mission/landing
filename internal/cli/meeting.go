package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/meeting"
)

type meetingResult struct {
	Question  string            `json:"question"`
	Positions []meetingPosition `json:"positions"`
	Arbiter   *meetingReading   `json:"arbiter"`
	Failures  []meetingFailure  `json:"failures"`
}

type meetingPosition struct {
	Persona      string            `json:"persona"`
	Route        meetingRoute      `json:"route"`
	Target       string            `json:"target,omitempty"`
	LatestSource string            `json:"latestSource,omitempty"`
	Status       harness.JobStatus `json:"status"`
	Answer       *string           `json:"answer"`
	Error        string            `json:"error,omitempty"`
}

type meetingReading struct {
	Persona      string            `json:"persona"`
	Route        meetingRoute      `json:"route"`
	Target       string            `json:"target,omitempty"`
	LatestSource string            `json:"latestSource,omitempty"`
	Status       harness.JobStatus `json:"status"`
	Answer       *string           `json:"answer"`
	Error        string            `json:"error,omitempty"`
}

type meetingFailure struct {
	Stage   string        `json:"stage"`
	Persona string        `json:"persona,omitempty"`
	Route   *meetingRoute `json:"route,omitempty"`
	Error   string        `json:"error"`
}

type meetingRoute struct {
	Harness string  `json:"harness"`
	Model   *string `json:"model"`
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
		positions = append(positions, projectMeetingPosition(position))
	}
	failures := make([]meetingFailure, 0, len(result.Failures))
	for _, failure := range result.Failures {
		projected := meetingFailure{Stage: failure.Stage, Error: failure.Error}
		if failure.Seat.Persona != "" {
			projected.Persona = failure.Seat.Persona
			route := meetingRouteFor(failure.Seat.Route)
			projected.Route = &route
		}
		failures = append(failures, projected)
	}
	projected := meetingResult{Question: result.Question, Positions: positions, Failures: failures}
	if result.Arbiter == nil {
		return projected
	}
	reading := meetingReading{
		Persona:      result.Arbiter.Seat.Persona,
		Route:        meetingRouteFor(result.Arbiter.Seat.Route),
		Target:       result.Arbiter.Seat.Target,
		LatestSource: result.Arbiter.Seat.LatestSource,
		Status:       harness.JobStatusFailed,
		Error:        result.Arbiter.Failure,
	}
	if result.Arbiter.Response != nil {
		reading.Status = result.Arbiter.Response.Status
		reading.Answer = result.Arbiter.Response.Output
	}
	projected.Arbiter = &reading

	return projected
}

func projectMeetingPosition(position meeting.Position) meetingPosition {
	projected := meetingPosition{
		Persona:      position.Seat.Persona,
		Route:        meetingRouteFor(position.Seat.Route),
		Target:       position.Seat.Target,
		LatestSource: position.Seat.LatestSource,
		Status:       harness.JobStatusFailed,
		Error:        position.Failure,
	}
	if position.Response != nil {
		projected.Status = position.Response.Status
		projected.Answer = position.Response.Output
	}

	return projected
}

func meetingRouteFor(route config.Route) meetingRoute {
	return meetingRoute{Harness: route.Harness, Model: route.Model}
}

func writeMeetingResult(result meetingResult, stdout io.Writer) error {
	for _, position := range result.Positions {
		if _, err := fmt.Fprintf(stdout, "seat: %s  %s%s\n", position.Persona, routeName(position.Route), latestNote(position.LatestSource)); err != nil {
			return err
		}
	}
	if result.Arbiter != nil {
		if _, err := fmt.Fprintf(stdout, "arbiter: %s  %s%s\n", result.Arbiter.Persona, routeName(result.Arbiter.Route), latestNote(result.Arbiter.LatestSource)); err != nil {
			return err
		}
	}
	for _, position := range result.Positions {
		if position.Answer == nil || *position.Answer == "" {
			continue
		}
		if _, err := fmt.Fprintf(stdout, "## %s · %s\n%s\n", position.Persona, routeName(position.Route), strings.TrimRight(*position.Answer, "\n")); err != nil {
			return err
		}
	}
	if result.Arbiter != nil && result.Arbiter.Answer != nil && *result.Arbiter.Answer != "" {
		if _, err := fmt.Fprintf(stdout, "## arbiter · %s · %s\n%s\n", result.Arbiter.Persona, routeName(result.Arbiter.Route), strings.TrimRight(*result.Arbiter.Answer, "\n")); err != nil {
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
		if _, err := fmt.Fprintf(stdout, "%s %s · %s: %s\n", failure.Stage, failure.Persona, routeName(*failure.Route), failure.Error); err != nil {
			return err
		}
	}

	return nil
}

func routeName(route meetingRoute) string {
	if route.Model == nil || *route.Model == "" {
		return route.Harness
	}

	return route.Harness + "/" + *route.Model
}

func latestNote(source string) string {
	if source == "" {
		return ""
	}

	return " (latest; " + source + ")"
}

func meetingFailed(result meeting.Result) bool {
	return len(result.Failures) > 0
}
