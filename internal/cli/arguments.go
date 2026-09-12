package cli

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/on-mission/landing/internal/paths"
)

type routeOption struct {
	Harness              string
	Model                *string
	FallbackBelowPercent *float64
}

type castOption struct {
	Persona string
	Route   routeOption
}

type suppliedOption struct {
	Name    string
	Written string
}

type options struct {
	Tier             parsedOption
	Model            *routeOption
	Reply            parsedOption
	Persona          parsedOption
	Personas         []string
	Arbiter          parsedOption
	Casts            []castOption
	CWD              parsedOption
	Label            parsedOption
	Timeout          parsedOption
	PromptFile       parsedOption
	InstructionsFile parsedOption
	Instructions     parsedOption
	Name             parsedOption
	Description      parsedOption
	Agent            parsedOption
	Message          parsedOption
	MaxWait          parsedOption
	As               parsedOption
	Harness          parsedOption
	Event            parsedOption
	Routes           []routeOption
	JSON             bool
	Help             bool
	Version          bool
	Default          bool
	RequireReply     bool
	Inbox            bool
	History          bool
	Install          bool
	Uninstall        bool
	ForcedPrompt     bool
	Supplied         []suppliedOption
}

func parseArguments(args []string) (options, []string, error) {
	values := options{Routes: make([]routeOption, 0)}
	positionals := make([]string, 0, len(args))
	parsingFlags := true
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if parsingFlags && argument == "--" {
			parsingFlags = false
			values.ForcedPrompt = true
			continue
		}
		if parsingFlags && argument == "-h" {
			values.Help = true
			values.Supplied = append(values.Supplied, suppliedOption{Name: "help", Written: argument})
			continue
		}
		if parsingFlags && strings.HasPrefix(argument, "--") {
			name, inlineValue, hasInlineValue := strings.Cut(argument[2:], "=")
			values.Supplied = append(values.Supplied, suppliedOption{Name: name, Written: argument})
			switch name {
			case "json", "help", "version", "default", "require-response", "inbox", "history", "install", "uninstall":
				if hasInlineValue {
					return options{}, nil, unknownOption(argument)
				}
				setBooleanOption(&values, name)
			case "tier", "model", "reply", "persona", "arbiter", "cast", "cwd", "label", "timeout", "prompt-file", "instructions-file", "name", "description", "route", "fallback-below", "agent", "message", "max-wait", "as", "harness", "event":
				value, nextIndex, err := optionValue(args, index, name, inlineValue, hasInlineValue)
				if err != nil {
					return options{}, nil, err
				}
				index = nextIndex
				if err := setOption(&values, name, value); err != nil {
					return options{}, nil, err
				}
			default:
				return options{}, nil, unknownOption(argument)
			}
			continue
		}
		if parsingFlags && strings.HasPrefix(argument, "-") && argument != "-" {
			return options{}, nil, unknownOption(argument)
		}
		positionals = append(positionals, argument)
	}

	return values, positionals, nil
}

func optionValue(args []string, index int, name string, inlineValue string, hasInlineValue bool) (string, int, error) {
	if hasInlineValue {
		return inlineValue, index, nil
	}
	if index+1 >= len(args) {
		return "", index, &usageError{message: fmt.Sprintf("option --%s has no value", name)}
	}

	return args[index+1], index + 1, nil
}

func setBooleanOption(values *options, name string) {
	switch name {
	case "json":
		values.JSON = true
	case "help":
		values.Help = true
	case "version":
		values.Version = true
	case "default":
		values.Default = true
	case "require-response":
		values.RequireReply = true
	case "inbox":
		values.Inbox = true
	case "history":
		values.History = true
	case "install":
		values.Install = true
	case "uninstall":
		values.Uninstall = true
	}
}

func setOption(values *options, name string, value string) error {
	option := parsedOption{Value: value, Set: true}
	switch name {
	case "tier":
		values.Tier = option
	case "model":
		if values.Model != nil {
			return &usageError{message: "--model is present more than once"}
		}
		route, err := parseModelRoute(value)
		if err != nil {
			return err
		}
		values.Model = &route
	case "reply":
		values.Reply = option
	case "persona":
		values.Persona = option
		values.Personas = append(values.Personas, value)
	case "arbiter":
		values.Arbiter = option
	case "cast":
		cast, err := parseCast(value)
		if err != nil {
			return err
		}
		values.Casts = append(values.Casts, cast)
	case "cwd":
		values.CWD = option
	case "label":
		values.Label = option
	case "timeout":
		values.Timeout = option
	case "prompt-file":
		values.PromptFile = option
	case "instructions-file":
		values.InstructionsFile = option
	case "name":
		values.Name = option
	case "description":
		values.Description = option
	case "route":
		route, err := parseRoute(value)
		if err != nil {
			return err
		}
		values.Routes = append(values.Routes, route)
	case "fallback-below":
		if len(values.Routes) == 0 {
			return &usageError{message: "--fallback-below has no preceding --route"}
		}
		percent, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 100 {
			return &usageError{message: fmt.Sprintf("--fallback-below has invalid percent %q", value)}
		}
		values.Routes[len(values.Routes)-1].FallbackBelowPercent = &percent
	case "agent":
		values.Agent = option
	case "message":
		values.Message = option
	case "max-wait":
		values.MaxWait = option
	case "as":
		values.As = option
	case "harness":
		values.Harness = option
	case "event":
		values.Event = option
	}

	return nil
}

func resolvePersonaInstructions(instructionsFile parsedOption, stdin io.Reader, stdinIsTerminal bool) (parsedOption, error) {
	if instructionsFile.Set {
		contents, err := os.ReadFile(instructionsFile.Value)
		if errors.Is(err, os.ErrNotExist) {
			return parsedOption{}, &usageError{message: fmt.Sprintf("instructions file %s does not exist", paths.Display(instructionsFile.Value))}
		}
		if err != nil {
			return parsedOption{}, fmt.Errorf("read instructions file %s: %w", paths.Display(instructionsFile.Value), err)
		}

		return parsedOption{Value: string(contents), Set: true}, nil
	}
	if stdinIsTerminal {
		return parsedOption{}, nil
	}
	contents, err := io.ReadAll(stdin)
	if err != nil {
		return parsedOption{}, fmt.Errorf("read persona instructions from standard input: %w", err)
	}
	if len(contents) == 0 {
		return parsedOption{}, nil
	}

	return parsedOption{Value: string(contents), Set: true}, nil
}

func unexpectedOptions(values options, command string, allowed ...string) error {
	allowedNames := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedNames[name] = struct{}{}
	}
	rejected := make([]string, 0)
	for _, supplied := range values.Supplied {
		if _, ok := allowedNames[supplied.Name]; ok {
			continue
		}
		rejected = append(rejected, supplied.Written)
	}
	if len(rejected) == 0 {
		return nil
	}
	if len(rejected) == 1 {
		return &usageError{message: fmt.Sprintf("option %s does not apply to command %q", rejected[0], command)}
	}

	return &usageError{message: fmt.Sprintf("options %s do not apply to command %q", strings.Join(rejected, ", "), command)}
}

func parseCast(value string) (castOption, error) {
	persona, routeValue, ok := strings.Cut(value, "=")
	if !ok || persona == "" || routeValue == "" {
		return castOption{}, &usageError{message: fmt.Sprintf("--cast has invalid value %q; expected persona=harness/model", value)}
	}
	route, err := parseRoute(routeValue)
	if err != nil {
		return castOption{}, err
	}

	return castOption{Persona: persona, Route: route}, nil
}

func parseRoute(value string) (routeOption, error) {
	harness, model, hasModel := strings.Cut(value, "/")
	if harness == "" || strings.Contains(model, "/") {
		return routeOption{}, &usageError{message: fmt.Sprintf("--route has invalid value %q; expected harness or harness/model", value)}
	}
	if !hasModel || model == "" {
		return routeOption{Harness: harness}, nil
	}

	return routeOption{Harness: harness, Model: &model}, nil
}

// parseModelRoute parses a dispatch's --model value. Some configured harnesses
// carry no model, so a bare harness names that execution path exactly; a
// harness/model value still pins both parts of the path.
func parseModelRoute(value string) (routeOption, error) {
	harnessName, model, hasModel := strings.Cut(value, "/")
	if harnessName == "cline" && strings.Contains(model, "/") {
		return routeOption{}, &usageError{message: fmt.Sprintf("--model cannot name Cline provider/model value %q; configure the Cline model in the project policy", value)}
	}
	if harnessName == "" || strings.Contains(model, "/") || (hasModel && model == "") {
		return routeOption{}, &usageError{message: fmt.Sprintf("--model has invalid value %q; expected harness or harness/model", value)}
	}
	if !hasModel {
		return routeOption{Harness: harnessName}, nil
	}

	return routeOption{Harness: harnessName, Model: &model}, nil
}

func unknownOption(argument string) error {
	return &usageError{message: fmt.Sprintf("unknown option %q", argument)}
}

func resolvePrompt(promptFile parsedOption, positionals []string, stdin io.Reader, stdinIsTerminal bool) (string, error) {
	if promptFile.Set {
		contents, err := os.ReadFile(promptFile.Value)
		if errors.Is(err, os.ErrNotExist) {
			return "", &usageError{message: fmt.Sprintf("prompt file %s does not exist", paths.Display(promptFile.Value))}
		}
		if err != nil {
			return "", fmt.Errorf("read prompt file %s: %w", paths.Display(promptFile.Value), err)
		}
		return strings.TrimSpace(string(contents)), nil
	}

	joined := strings.TrimSpace(strings.Join(positionals, " "))
	if joined != "-" {
		return joined, nil
	}
	if stdinIsTerminal {
		return "", &usageError{message: "prompt is '-' and standard input is a terminal"}
	}
	contents, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read prompt from standard input: %w", err)
	}

	return strings.TrimSpace(string(contents)), nil
}

func resolveTimeout(reply bool, supplied parsedOption) (time.Duration, error) {
	if !supplied.Set {
		if reply {
			return defaultReplyTimeout, nil
		}
		return defaultDispatchTimeout, nil
	}
	milliseconds, err := strconv.ParseFloat(supplied.Value, 64)
	if err != nil || math.IsNaN(milliseconds) || math.IsInf(milliseconds, 0) || milliseconds <= 0 || milliseconds > float64(math.MaxInt64)/float64(time.Millisecond) {
		return 0, &usageError{message: fmt.Sprintf("--timeout has invalid milliseconds value %q", supplied.Value)}
	}

	return time.Duration(milliseconds * float64(time.Millisecond)), nil
}

func resolveInvocationDir(supplied string) (string, error) {
	if supplied != "" {
		return filepath.Abs(supplied)
	}

	directory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("determine invocation directory: %w", err)
	}

	return directory, nil
}
