// Package meeting convenes one participant-and-arbiter deliberation round.
package meeting

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/dispatch"
	"github.com/on-mission/landing/internal/harness"
)

// Dispatcher is the execution boundary a meeting needs. Routes are concrete
// before a round starts, so validation can finish before any seat dispatches.
type Dispatcher interface {
	Dispatch(context.Context, dispatch.Request) (dispatch.Response, error)
	ValidateRoute(context.Context, config.Route) error
}

// Seat is one persona on one concrete route in a single meeting round.
type Seat struct {
	Persona      string
	Route        config.Route
	Target       string
	LatestSource string
}

type Request struct {
	Question string
	Seats    []Seat
	Arbiter  Seat
	Tier     string
	CWD      string
	Label    *string
	Timeout  time.Duration
}

type Position struct {
	Seat     Seat
	Response *dispatch.Response
	Failure  string
}

type Reading struct {
	Seat     Seat
	Response *dispatch.Response
	Failure  string
}

type Failure struct {
	Stage string
	Seat  Seat
	Error string
}

type Result struct {
	Question  string
	Seats     []Seat
	Positions []Position
	Arbiter   *Reading
	Failures  []Failure
}

type positionResult struct {
	index    int
	seat     Seat
	response dispatch.Response
	err      error
}

type validationResult struct {
	route config.Route
	err   error
}

func Validate(seats []Seat, arbiter Seat) error {
	if arbiter.Persona == "" {
		return fmt.Errorf("meeting has no arbiter")
	}
	if arbiter.Route.Harness == "" {
		return fmt.Errorf("meeting arbiter has no route")
	}
	if len(seats) < 2 {
		return fmt.Errorf("meeting needs at least two participant seats; it has %d", len(seats))
	}
	for _, seat := range seats {
		if seat.Persona == "" {
			return fmt.Errorf("meeting names an empty persona")
		}
		if seat.Route.Harness == "" {
			return fmt.Errorf("meeting seat for %q has no route", seat.Persona)
		}
	}

	return nil
}

func Run(ctx context.Context, dispatcher Dispatcher, request Request) (Result, error) {
	if err := Validate(request.Seats, request.Arbiter); err != nil {
		return Result{}, err
	}
	if request.Question == "" {
		return Result{}, fmt.Errorf("meeting question is empty")
	}
	if request.Timeout < 0 {
		return Result{}, fmt.Errorf("meeting timeout is negative")
	}

	meetingContext, cancel := meetingContext(ctx, request.Timeout)
	defer cancel()
	if err := validateRoutes(meetingContext, dispatcher, request.Seats, request.Arbiter); err != nil {
		return Result{}, err
	}
	positions := dispatchSeats(meetingContext, dispatcher, request)
	result := Result{
		Question:  request.Question,
		Seats:     copySeats(request.Seats),
		Positions: positions,
		Failures:  participantFailures(positions),
	}
	if len(answered(positions)) < 2 {
		result.Failures = append(result.Failures, Failure{Stage: "meeting", Error: "fewer than two participant seats answered; the round produced nothing to deliberate"})
		return result, nil
	}
	reading := readPositions(meetingContext, dispatcher, request, positions)
	result.Arbiter = &reading
	if reading.Failure != "" {
		result.Failures = append(result.Failures, Failure{Stage: "arbiter", Seat: reading.Seat, Error: reading.Failure})
	}

	return result, nil
}

func meetingContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
}

func validateRoutes(ctx context.Context, dispatcher Dispatcher, seats []Seat, arbiter Seat) error {
	routes := uniqueRoutes(seats, arbiter)
	targets := routeTargets(seats, arbiter)
	results := make(chan validationResult, len(routes))
	for _, route := range routes {
		go func(route config.Route) {
			results <- validationResult{route: route, err: dispatcher.ValidateRoute(ctx, route)}
		}(route)
	}
	failures := make([]string, 0)
	for range routes {
		result := <-results
		if result.err == nil {
			continue
		}
		failures = append(failures, fmt.Sprintf("target %s resolved to %s is unavailable: %v", strings.Join(targets[result.route.String()], ", "), result.route.String(), result.err))
	}
	if len(failures) == 0 {
		return nil
	}

	return fmt.Errorf("meeting model preflight refused before dispatching seats: %s", strings.Join(failures, "; "))
}

func routeTargets(seats []Seat, arbiter Seat) map[string][]string {
	allSeats := append(append([]Seat(nil), seats...), arbiter)
	targets := make(map[string][]string, len(allSeats))
	for _, seat := range allSeats {
		key := seat.Route.String()
		name := seat.Target
		if name == "" {
			name = key
		}
		if slices.Contains(targets[key], name) {
			continue
		}
		targets[key] = append(targets[key], name)
	}

	return targets
}

func uniqueRoutes(seats []Seat, arbiter Seat) []config.Route {
	allSeats := append(append([]Seat(nil), seats...), arbiter)
	seen := make(map[string]struct{}, len(allSeats))
	routes := make([]config.Route, 0, len(allSeats))
	for _, seat := range allSeats {
		key := seat.Route.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		routes = append(routes, copyRoute(seat.Route))
	}

	return routes
}

func dispatchSeats(ctx context.Context, dispatcher Dispatcher, request Request) []Position {
	results := make(chan positionResult, len(request.Seats))
	for index, seat := range request.Seats {
		go func(index int, seat Seat) {
			route := copyRoute(seat.Route)
			response, err := dispatcher.Dispatch(ctx, dispatch.Request{
				Tier:         request.Tier,
				Prompt:       request.Question,
				CWD:          request.CWD,
				Label:        request.Label,
				Persona:      seat.Persona,
				AwaitTimeout: request.Timeout,
				Route:        &route,
			})
			results <- positionResult{index: index, seat: seat, response: response, err: err}
		}(index, seat)
	}
	positions := make([]Position, len(request.Seats))
	for range request.Seats {
		position := <-results
		positions[position.index] = positionFrom(position)
	}

	return positions
}

func readPositions(ctx context.Context, dispatcher Dispatcher, request Request, positions []Position) Reading {
	route := copyRoute(request.Arbiter.Route)
	response, err := dispatcher.Dispatch(ctx, dispatch.Request{
		Tier:         request.Tier,
		Prompt:       arbiterPrompt(request.Question, positions),
		CWD:          request.CWD,
		Label:        request.Label,
		Persona:      request.Arbiter.Persona,
		AwaitTimeout: request.Timeout,
		Route:        &route,
	})
	if err != nil {
		return Reading{Seat: request.Arbiter, Failure: err.Error()}
	}
	if response.Status != harness.JobStatusDone {
		return Reading{Seat: request.Arbiter, Response: &response, Failure: responseFailure(response)}
	}

	return Reading{Seat: request.Arbiter, Response: &response}
}

func arbiterPrompt(question string, positions []Position) string {
	sections := []string{
		"You are the arbiter for one meeting round. Read the independent participant positions and respond in prose for the lead agent.",
		"The question:",
		question,
		"The participant positions:",
	}
	for _, position := range answered(positions) {
		sections = append(sections, "## "+seatName(position.Seat), outputFor(position))
	}
	sections = append(sections,
		"Identify only genuine conflicts: positions that cannot both be acted on. Different ground, emphasis, or disagreement outside the question is not a conflict.",
		"For each genuine conflict, name the sharpest point it turns on. Do not decide whether another round is needed and do not write a synthesis.",
	)

	return strings.Join(sections, "\n\n")
}

func seatName(seat Seat) string {
	return seat.Persona + " · " + seat.Route.String()
}

func copySeats(seats []Seat) []Seat {
	copied := make([]Seat, 0, len(seats))
	for _, seat := range seats {
		seat.Route = copyRoute(seat.Route)
		copied = append(copied, seat)
	}

	return copied
}

func copyRoute(route config.Route) config.Route {
	copied := route
	if route.Model != nil {
		model := *route.Model
		copied.Model = &model
	}

	return copied
}

func positionFrom(result positionResult) Position {
	if result.err != nil {
		return Position{Seat: result.seat, Failure: result.err.Error()}
	}
	response := result.response
	if response.Status == harness.JobStatusDone {
		return Position{Seat: result.seat, Response: &response}
	}

	return Position{Seat: result.seat, Response: &response, Failure: responseFailure(response)}
}

func participantFailures(positions []Position) []Failure {
	failures := make([]Failure, 0)
	for _, position := range positions {
		if position.Failure != "" {
			failures = append(failures, Failure{Stage: "participant", Seat: position.Seat, Error: position.Failure})
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
