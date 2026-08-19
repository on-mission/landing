// Package meeting convenes one participant-and-arbiter deliberation round.
package meeting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/dispatch"
	"github.com/on-mission/landing/internal/harness"
)

type Dispatcher interface {
	Dispatch(context.Context, dispatch.Request) (dispatch.Response, error)
}

type Request struct {
	Question string
	Personas []string
	Arbiter  string
	Casts    map[string]config.Route
	Tier     string
	CWD      string
	Label    *string
	Timeout  time.Duration
}

type Position struct {
	Persona  string
	Response *dispatch.Response
	Failure  string
}

type Reading struct {
	Response *dispatch.Response
	Failure  string
}

type Failure struct {
	Stage   string
	Persona string
	Error   string
}

type Result struct {
	Question     string
	Participants []string
	Casts        map[string]config.Route
	Positions    []Position
	Arbiter      *Reading
	Failures     []Failure
}

type positionResult struct {
	index    int
	persona  string
	response dispatch.Response
	err      error
}

func Validate(personas []string, arbiter string) error {
	if arbiter == "" {
		return fmt.Errorf("meeting has no arbiter")
	}
	if len(personas) < 2 {
		return fmt.Errorf("meeting needs at least two participants; it has %d", len(personas))
	}
	seen := make(map[string]struct{}, len(personas))
	for _, persona := range personas {
		if persona == "" {
			return fmt.Errorf("meeting names an empty persona")
		}
		if _, ok := seen[persona]; ok {
			return fmt.Errorf("meeting names persona %q more than once", persona)
		}
		seen[persona] = struct{}{}
	}

	return nil
}

func Run(ctx context.Context, dispatcher Dispatcher, request Request) (Result, error) {
	if err := Validate(request.Personas, request.Arbiter); err != nil {
		return Result{}, err
	}
	if request.Question == "" {
		return Result{}, fmt.Errorf("meeting question is empty")
	}
	if request.Timeout <= 0 {
		return Result{}, fmt.Errorf("meeting timeout is not positive")
	}

	meetingContext, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	positions := dispatchParticipants(meetingContext, dispatcher, request)
	result := Result{
		Question:     request.Question,
		Participants: append([]string(nil), request.Personas...),
		Casts:        copyCasts(request.Casts),
		Positions:    positions,
		Failures:     participantFailures(positions),
	}
	if len(answered(positions)) < 2 {
		result.Failures = append(result.Failures, Failure{Stage: "meeting", Error: "fewer than two participants answered; the round produced nothing to deliberate"})
		return result, nil
	}
	reading := readPositions(meetingContext, dispatcher, request, positions)
	result.Arbiter = &reading
	if reading.Failure != "" {
		result.Failures = append(result.Failures, Failure{Stage: "arbiter", Persona: request.Arbiter, Error: reading.Failure})
	}

	return result, nil
}

func dispatchParticipants(ctx context.Context, dispatcher Dispatcher, request Request) []Position {
	results := make(chan positionResult, len(request.Personas))
	for index, persona := range request.Personas {
		go func(index int, persona string) {
			response, err := dispatcher.Dispatch(ctx, dispatch.Request{
				Tier:         request.Tier,
				Prompt:       request.Question,
				CWD:          request.CWD,
				Label:        request.Label,
				Persona:      persona,
				AwaitTimeout: request.Timeout,
				Route:        castFor(request.Casts, persona),
			})
			results <- positionResult{index: index, persona: persona, response: response, err: err}
		}(index, persona)
	}
	positions := make([]Position, len(request.Personas))
	for range request.Personas {
		position := <-results
		positions[position.index] = positionFrom(position)
	}

	return positions
}

func readPositions(ctx context.Context, dispatcher Dispatcher, request Request, positions []Position) Reading {
	response, err := dispatcher.Dispatch(ctx, dispatch.Request{
		Tier:         request.Tier,
		Prompt:       arbiterPrompt(request.Question, positions),
		CWD:          request.CWD,
		Label:        request.Label,
		Persona:      request.Arbiter,
		AwaitTimeout: request.Timeout,
	})
	if err != nil {
		return Reading{Failure: err.Error()}
	}
	if response.Status != harness.JobStatusDone {
		return Reading{Response: &response, Failure: responseFailure(response)}
	}

	return Reading{Response: &response}
}

func arbiterPrompt(question string, positions []Position) string {
	sections := []string{
		"You are the arbiter for one meeting round. Read the independent participant positions and respond in prose for the lead agent.",
		"The question:",
		question,
		"The participant positions:",
	}
	for _, position := range answered(positions) {
		sections = append(sections, "## "+position.Persona, outputFor(position))
	}
	sections = append(sections,
		"Identify only genuine conflicts: positions that cannot both be acted on. Different ground, emphasis, or disagreement outside the question is not a conflict.",
		"For each genuine conflict, name the sharpest point it turns on. Do not decide whether another round is needed and do not write a synthesis.",
	)

	return strings.Join(sections, "\n\n")
}

func copyCasts(casts map[string]config.Route) map[string]config.Route {
	if len(casts) == 0 {
		return nil
	}
	copyOfCasts := make(map[string]config.Route, len(casts))
	for persona, route := range casts {
		copyOfRoute := route
		if route.Model != nil {
			model := *route.Model
			copyOfRoute.Model = &model
		}
		copyOfCasts[persona] = copyOfRoute
	}

	return copyOfCasts
}

func castFor(casts map[string]config.Route, persona string) *config.Route {
	route, ok := casts[persona]
	if !ok {
		return nil
	}
	copyOfRoute := route
	if route.Model != nil {
		model := *route.Model
		copyOfRoute.Model = &model
	}

	return &copyOfRoute
}

func positionFrom(result positionResult) Position {
	if result.err != nil {
		return Position{Persona: result.persona, Failure: result.err.Error()}
	}
	response := result.response
	if response.Status == harness.JobStatusDone {
		return Position{Persona: result.persona, Response: &response}
	}

	return Position{Persona: result.persona, Response: &response, Failure: responseFailure(response)}
}

func participantFailures(positions []Position) []Failure {
	failures := make([]Failure, 0)
	for _, position := range positions {
		if position.Failure != "" {
			failures = append(failures, Failure{Stage: "participant", Persona: position.Persona, Error: position.Failure})
		}
	}

	return failures
}

func answered(positions []Position) []Position {
	answers := make([]Position, 0, len(positions))
	for _, position := range positions {
		if position.Response == nil || position.Failure != "" {
			continue
		}
		answers = append(answers, position)
	}

	return answers
}

func outputFor(position Position) string {
	if position.Response == nil || position.Response.Output == nil {
		return ""
	}

	return *position.Response.Output
}

func responseFailure(response dispatch.Response) string {
	if response.Error != nil && *response.Error != "" {
		return *response.Error
	}

	return fmt.Sprintf("dispatch completed with status %q", response.Status)
}
