// Package cli implements Landing's command-line boundary.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/on-mission/landing/internal/harness"
)

// version and commit are set at build time via -ldflags
// "-X github.com/on-mission/landing/internal/cli.version=... -X
// github.com/on-mission/landing/internal/cli.commit=...". Builds that skip
// that flag (go run, go install without ldflags, an unreleased checkout)
// keep these honest defaults rather than a stale hardcoded version.
var (
	version = "dev"
	commit  = "unknown"
)

const (
	progressInterval = 30 * time.Second
	exitOK           = 0
	exitFailed       = 1
	exitUsage        = 2
)

const modelListHelp = `landing model list — list known model routes and effective latest choices.

USAGE
  landing model list

Each row is a known route, the harness's observed state, catalog authority, and
the tiers that configure it. Each harness's effective latest model and source
appear once before its routes.
An advisory catalog may be incomplete: Landing passes a valid unlisted model to
that harness, which decides whether it can run it. Detection runs; capacity is
not probed. Use ` + "`landing harness list`" + ` for measured availability.

OPTIONS
  --json                        JSON output.
  --help, -h                    Help.
`

const help = `landing — dispatch work through a configured tier or a named route.

USAGE
  landing "<prompt>"
  landing --tier <name> "<prompt>"
  landing --model <target> "<prompt>"
  landing --reply <id> "<prompt>"
  landing meeting --arbiter <persona>[=<target>] [--persona <persona>]... [--cast <persona>=<target>]... "<question>"
  landing messages send | monitor | who | inbox | install | uninstall
  landing config init | show
  landing tier list | add | update | remove
  landing harness list
  landing model list
  landing persona list | show | add | update | remove

INPUT
  Landing first matches an exact management command. Other input is dispatched
  as a prompt. -- forces all following input to be a prompt.

OPTIONS
  --tier <name>                 Tier for a dispatch.
  --model <target>              Model target for a dispatch, in place of a tier.
  --reply <id>                  Continue an existing thread.
  --persona <name>              Persona for a dispatch.
  --arbiter <persona>[=<target>] Persona that reads a meeting round, optionally pinned to one route.
  --cast <persona>=<target>     Seat a meeting participant on one or more target routes.
  --cwd <path>                  Dispatch working directory.
  --label <text>                Dispatch label.
  --timeout <ms>                Dispatch timeout; no limit by default.
  --prompt-file <path>          Prompt file; takes precedence over argv and stdin.
  --instructions-file <path>    Persona instructions for persona add or update.
  --route <harness>/<model>     Tier route; model may be omitted.
  --fallback-below <percent>    Reserve the preceding tier route.
  --description <text>          Tier or persona description.
  --name <name>                 Tier or persona name for add, update, remove, or show.
  --to <name>                   Message recipient.
  --message <text>              Message body.
  --as <name>                   Name for this chat.
  --default                     Mark an added or updated tier as default.
  --json                        JSON output for commands that report data.
  --help, -h                    Help.
  --version                     Version.
`

const meetingHelp = `landing meeting — convene one deliberation round.

USAGE
  landing meeting --arbiter <persona>[=<target>] [--persona <persona>]... [--cast <persona>=<target>]... [--tier <name>] "<question>"

A meeting is useful when independent perspectives can expose a real decision
conflict. Landing dispatches every participant in parallel with only the
question and that participant's own persona material. It then gives their
positions to the arbiter, which responds in prose. The result contains every
position and the arbiter's reading.

The caller reads the round and decides whether to convene another. For a
further round, put the prior positions verbatim in the new question together
with what the arbiter flagged. Once deliberation is complete, request a
synthesis from the arbiter as ordinary work.

The arbiter is required; at least two participant seats are required. An arbiter
may also be a participant, in a separate instance and context. A cast makes its
persona a participant and may resolve to several routes; duplicate routes for
that persona collapse. --arbiter may pin one route, while its target may not
resolve to several routes. Landing validates every seat and arbiter route before
it dispatches any seat.

OPTIONS
  --arbiter <persona>[=<target>] Persona that reads positions, optionally pinned to one target route.
  --persona <persona>           Participant; uncast personas receive one tier-routed seat.
  --cast <persona>=<target>     Seat a persona on every route the target resolves to.
  --tier <name>                 Tier for uncast participants and the arbiter.
  --prompt-file <path>          Question file; takes precedence over argv and stdin.
  --cwd <path>                  Dispatch working directory.
  --label <text>                Dispatch label.
  --timeout <ms>                Round timeout; no limit by default.
  --json                        JSON output.
  --help, -h                    Help.
`

const messagesHelp = `landing messages — send a message, or watch for one.

USAGE
  landing messages send --to <name> --message <text> [--as <name>]
  landing messages monitor [--as <name>]
  landing messages who [<name>]
  landing messages inbox [--as <name>]
  landing messages install
  landing messages uninstall

send writes one message and returns. monitor prints unacknowledged messages
and exits. inbox prints what is waiting and leaves it there. who lists
registered names. install and uninstall edit an existing AGENTS.md or CLAUDE.md
at the project root.

OPTIONS
  --to <name>                   Recipient. all is forwarded unchanged.
  --message <text>              Message body.
  --as <name>                   Name for this chat.
  --json                        JSON output for send and monitor.
  --help, -h                    Help.
`

const configInitHelp = `landing config init — create a project configuration.

USAGE
  landing config init

This writes Landing’s initial configuration in the current project.

OPTIONS
  --help, -h                    Help.
`

const configShowHelp = `landing config show — report the resolved project configuration.

USAGE
  landing config show

OPTIONS
  --json                        JSON output.
  --help, -h                    Help.
`

const tierListHelp = `landing tier list — list configured execution tiers.

USAGE
  landing tier list

OPTIONS
  --json                        JSON output.
  --help, -h                    Help.
`

const tierAddHelp = `landing tier add — add a configured execution tier.

USAGE
  landing tier add --name <name> --route <harness>[/<model>] [--fallback-below <percent>] [--description <text>] [--default]

Each --fallback-below applies to the preceding --route.

OPTIONS
  --name <name>                 Tier name.
  --route <harness>/<model>     Route; model may be omitted.
  --fallback-below <percent>    Reserve the preceding route below this capacity.
  --description <text>          Tier description.
  --default                     Mark this tier as the default.
  --help, -h                    Help.
`

const tierUpdateHelp = `landing tier update — change an existing execution tier.

USAGE
  landing tier update --name <name> [--route <harness>[/<model>]] [--fallback-below <percent>] [--description <text>] [--default]

Supplying one or more --route options replaces the tier’s routes. Each
--fallback-below applies to the preceding --route.

OPTIONS
  --name <name>                 Tier name.
  --route <harness>/<model>     Replacement route; model may be omitted.
  --fallback-below <percent>    Reserve the preceding route below this capacity.
  --description <text>          Replacement tier description.
  --default                     Mark this tier as the default.
  --help, -h                    Help.
`

const tierRemoveHelp = `landing tier remove — remove a configured execution tier.

USAGE
  landing tier remove --name <name>

OPTIONS
  --name <name>                 Tier name.
  --help, -h                    Help.
`

const harnessListHelp = `landing harness list — list installed harnesses and observed capacity.

USAGE
  landing harness list

OPTIONS
  --json                        JSON output.
  --help, -h                    Help.
`

const personaListHelp = `landing persona list — list the project’s personas.

USAGE
  landing persona list

OPTIONS
  --json                        JSON output.
  --help, -h                    Help.
`

const personaShowHelp = `landing persona show — report a persona definition and its reference files.

USAGE
  landing persona show --name <name>

OPTIONS
  --name <name>                 Persona name.
  --json                        JSON output.
  --help, -h                    Help.
`

const personaAddHelp = `landing persona add — create a project persona definition.

USAGE
  landing persona add --name <name> [--description <text>] [--instructions-file <path>]

When --instructions-file is omitted, non-terminal standard input becomes the
persona instructions. An empty persona is valid while its definition is being built.

OPTIONS
  --name <name>                 Persona name.
  --description <text>          Single-line persona description.
  --instructions-file <path>    File containing persona instructions.
  --help, -h                    Help.
`

const personaUpdateHelp = `landing persona update — change a project persona definition.

USAGE
  landing persona update --name <name> [--description <text>] [--instructions-file <path>]

When --instructions-file is omitted, non-terminal standard input replaces the
instructions when it contains bytes. A description-only update preserves the body.

OPTIONS
  --name <name>                 Persona name.
  --description <text>          Replacement single-line description.
  --instructions-file <path>    File containing replacement instructions.
  --help, -h                    Help.
`

const personaRemoveHelp = `landing persona remove — remove a project persona definition.

USAGE
  landing persona remove --name <name>

OPTIONS
  --name <name>                 Persona name.
  --help, -h                    Help.
`

type Inputs struct {
	Args            []string
	InvocationDir   string
	Stdin           io.Reader
	StdinIsTerminal bool
	Stdout          io.Writer
	Stderr          io.Writer
}

type parsedOption struct {
	Value string
	Set   bool
}

type result struct {
	JobID           string                   `json:"jobId"`
	Role            *string                  `json:"role"`
	Provider        *string                  `json:"provider"`
	Model           *string                  `json:"model"`
	Label           *string                  `json:"label"`
	Status          harness.JobStatus        `json:"status"`
	StartedAt       time.Time                `json:"startedAt"`
	FinishedAt      *time.Time               `json:"finishedAt"`
	DurationMS      int64                    `json:"durationMs"`
	ExitCode        *int                     `json:"exitCode"`
	Output          *string                  `json:"output"`
	Error           *string                  `json:"error"`
	RoutedBecause   *string                  `json:"routedBecause"`
	PersonaDelivery *harness.PersonaDelivery `json:"personaDelivery,omitempty"`
}

type usageError struct {
	message string
}

func (err *usageError) Error() string {
	return err.message
}

func Run(ctx context.Context, inputs Inputs) (int, error) {
	if err := validateInputs(inputs); err != nil {
		return exitFailed, err
	}
	selftestCode, handled, err := runSelftest(inputs.Stdout, inputs.Stderr)
	if err != nil {
		return exitFailed, err
	}
	if handled {
		return selftestCode, nil
	}

	values, positionals, err := parseArguments(inputs.Args)
	if err != nil {
		return exitUsage, err
	}
	if values.Version {
		if _, err := fmt.Fprintf(inputs.Stdout, "%s (%s)\n", version, commit); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if values.Help {
		if commandHelp, ok := helpForCommand(positionals, values.ForcedPrompt); ok {
			if _, err := fmt.Fprint(inputs.Stdout, commandHelp); err != nil {
				return exitFailed, err
			}
			return exitOK, nil
		}
	}
	invocationDir, err := resolveInvocationDir(inputs.InvocationDir)
	if err != nil {
		return exitFailed, err
	}
	registry := newRegistry()
	if values.Help {
		return writeHelp(ctx, invocationDir, registry, inputs.Stdout)
	}
	if messagesCommand(positionals, values.ForcedPrompt) {
		return runMessages(ctx, values, positionals[1:], invocationDir, inputs.Stdout)
	}

	if command, ok := managementCommand(positionals, values.ForcedPrompt); ok {
		if err := managementCommandOptions(values, command); err != nil {
			return exitUsage, err
		}
		if command == commandPersonaAdd || command == commandPersonaUpdate {
			instructions, err := resolvePersonaInstructions(values.InstructionsFile, inputs.Stdin, inputs.StdinIsTerminal)
			if err != nil {
				return exitUsage, err
			}
			values.Instructions = instructions
		}
		return runManagement(ctx, command, values, invocationDir, registry, inputs.Stdout)
	}
	if meetingCommand(positionals, values.ForcedPrompt) {
		return runMeeting(ctx, inputs, values, positionals[1:], invocationDir, registry)
	}

	return runDispatch(ctx, inputs, values, positionals, invocationDir, registry)
}

func helpForCommand(positionals []string, forcedPrompt bool) (string, bool) {
	if meetingCommand(positionals, forcedPrompt) {
		return meetingHelp, true
	}
	if messagesCommand(positionals, forcedPrompt) {
		return messagesHelp, true
	}
	if command, ok := managementCommand(positionals, forcedPrompt); ok {
		return managementHelp(command), true
	}

	return "", false
}

func managementHelp(command command) string {
	switch command {
	case commandConfigInit:
		return configInitHelp
	case commandConfigShow:
		return configShowHelp
	case commandTierList:
		return tierListHelp
	case commandTierAdd:
		return tierAddHelp
	case commandTierUpdate:
		return tierUpdateHelp
	case commandTierRemove:
		return tierRemoveHelp
	case commandHarnessList:
		return harnessListHelp
	case commandModelList:
		return modelListHelp
	case commandPersonaList:
		return personaListHelp
	case commandPersonaShow:
		return personaShowHelp
	case commandPersonaAdd:
		return personaAddHelp
	case commandPersonaUpdate:
		return personaUpdateHelp
	case commandPersonaRemove:
		return personaRemoveHelp
	default:
		return help
	}
}

func managementCommandOptions(values options, command command) error {
	switch command {
	case commandConfigInit:
		return unexpectedOptions(values, string(command))
	case commandConfigShow, commandTierList, commandHarnessList, commandModelList, commandPersonaList:
		return unexpectedOptions(values, string(command), "json")
	case commandTierAdd, commandTierUpdate:
		return unexpectedOptions(values, string(command), "name", "description", "route", "fallback-below", "default")
	case commandTierRemove:
		return unexpectedOptions(values, string(command), "name")
	case commandPersonaShow:
		return unexpectedOptions(values, string(command), "name", "json")
	case commandPersonaAdd, commandPersonaUpdate:
		return unexpectedOptions(values, string(command), "name", "description", "instructions-file")
	case commandPersonaRemove:
		return unexpectedOptions(values, string(command), "name")
	default:
		return nil
	}
}

func messagesCommand(positionals []string, forcedPrompt bool) bool {
	return !forcedPrompt && len(positionals) > 0 && positionals[0] == "messages"
}

func ReportError(stderr io.Writer, err error) int {
	var usage *usageError
	if errors.As(err, &usage) {
		if _, writeErr := fmt.Fprintf(stderr, "error: %s\n", usage.message); writeErr != nil {
			return exitFailed
		}
		return exitUsage
	}

	var harnessError *harness.Error
	if errors.As(err, &harnessError) {
		if _, writeErr := fmt.Fprintf(stderr, "error: %s: %s\n", harnessError.Code, harnessError.Message); writeErr != nil {
			return exitFailed
		}
		if harnessError.Details != nil {
			encoded, marshalErr := json.Marshal(harnessError.Details)
			if marshalErr != nil {
				return exitFailed
			}
			if _, writeErr := fmt.Fprintf(stderr, "%s\n", encoded); writeErr != nil {
				return exitFailed
			}
		}
		if isUsageCode(harnessError.Code) {
			return exitUsage
		}
		return exitFailed
	}

	if _, writeErr := fmt.Fprintf(stderr, "error: %v\n", err); writeErr != nil {
		return exitFailed
	}
	return exitFailed
}

func validateInputs(inputs Inputs) error {
	if inputs.Stdin == nil || inputs.Stdout == nil || inputs.Stderr == nil {
		return fmt.Errorf("cli inputs have a nil standard stream")
	}

	return nil
}

func IsTerminalStdin(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}
