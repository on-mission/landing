package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/on-mission/landing/internal/comms"
	"github.com/on-mission/landing/internal/comms/hooks"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
)

const (
	unboundedWaitStep = 24 * time.Hour
	hookTimeout       = 5 * time.Second
	historyWindow     = 24 * time.Hour
)

func runComms(ctx context.Context, values options, positionals []string, invocationDir string, stdin io.Reader, stdout io.Writer) (int, error) {
	if len(positionals) > 0 && positionals[0] == "hook" {
		if len(positionals) != 1 {
			return exitUsage, &usageError{message: "comms hook has unexpected arguments"}
		}
		return runCommsHook(ctx, values, invocationDir, stdin, stdout), nil
	}
	if len(positionals) != 0 {
		return exitUsage, &usageError{message: "comms has unexpected arguments"}
	}
	if err := commsOptions(values); err != nil {
		return exitUsage, err
	}
	store, err := comms.Open(ctx, invocationDir)
	if err != nil {
		return exitFailed, err
	}
	if values.Install {
		plan, err := hooks.Plan(ctx, invocationDir)
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, plan.Render()); err != nil {
			return exitFailed, err
		}
		result, err := hooks.Install(ctx, invocationDir)
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, result.Render()); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if values.Uninstall {
		result, err := hooks.Uninstall(ctx, invocationDir)
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintln(stdout, result.Render()); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if values.History {
		return writeHistory(ctx, store, values.JSON, stdout)
	}
	caller, err := callingParticipant(ctx, store)
	if err != nil {
		return exitUsage, err
	}
	if values.Reply.Set {
		return replyComms(ctx, store, caller, values, stdout)
	}
	if values.Agent.Set {
		return sendComms(ctx, store, caller, values, stdout)
	}
	return writeInbox(ctx, store, caller, values.JSON, stdout)
}

func commsOptions(values options) error {
	if err := unexpectedOptions(values, "comms", "agent", "reply", "message", "require-response", "max-wait", "inbox", "history", "install", "uninstall", "json"); err != nil {
		return err
	}
	actions := 0
	if values.Agent.Set || values.Reply.Set {
		actions++
	}
	if values.Inbox {
		actions++
	}
	if values.History {
		actions++
	}
	if values.Install {
		actions++
	}
	if values.Uninstall {
		actions++
	}
	if actions > 1 {
		return &usageError{message: "comms has more than one action"}
	}
	if values.Message.Set && !values.Agent.Set && !values.Reply.Set {
		return &usageError{message: "--message requires --agent or --reply"}
	}
	if (values.Agent.Set || values.Reply.Set) && (!values.Message.Set || strings.TrimSpace(values.Message.Value) == "") {
		return &usageError{message: "--agent and --reply require a non-empty --message"}
	}
	if values.RequireReply && !values.Agent.Set {
		return &usageError{message: "--require-response requires --agent"}
	}
	if values.MaxWait.Set && !values.RequireReply {
		return &usageError{message: "--max-wait requires --require-response"}
	}
	if values.Install && values.Uninstall {
		return &usageError{message: "--install and --uninstall cannot be combined"}
	}
	if values.JSON && values.Agent.Set {
		return &usageError{message: "option --json does not apply to comms send"}
	}
	if values.JSON && values.Reply.Set {
		return &usageError{message: "option --json does not apply to comms reply"}
	}
	if values.JSON && values.Install {
		return &usageError{message: "option --json does not apply to comms installation"}
	}
	if values.JSON && values.Uninstall {
		return &usageError{message: "option --json does not apply to comms removal"}
	}
	return nil
}

func sendComms(ctx context.Context, store comms.Store, caller comms.Participant, values options, stdout io.Writer) (int, error) {
	if values.Agent.Value == "*" {
		return exitUsage, &usageError{message: `--agent "*" is not accepted; all is the broadcast address`}
	}
	if values.Agent.Value == caller.Name {
		return exitUsage, &usageError{message: fmt.Sprintf("--agent names the calling participant %q", caller.Name)}
	}
	participants, err := store.Participants(ctx)
	if err != nil {
		return exitFailed, err
	}
	if values.Agent.Value == comms.AllAgents {
		message, err := store.Send(ctx, comms.Message{From: caller.Name, To: comms.AllAgents, Body: values.Message.Value, RequiresResponse: values.RequireReply})
		if err != nil {
			return exitFailed, err
		}
		if _, err := fmt.Fprintf(stdout, "queued broadcast %s for %d participants\n", message.ID, len(participants)-1); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	target, ok := participantNamed(participants, values.Agent.Value)
	if !ok {
		return exitUsage, &usageError{message: fmt.Sprintf("participant %q is not registered", values.Agent.Value)}
	}
	message, err := store.Send(ctx, comms.Message{From: caller.Name, To: target.Name, Body: values.Message.Value, RequiresResponse: values.RequireReply})
	if err != nil {
		return exitFailed, err
	}
	if _, err := fmt.Fprintf(stdout, "queued message %s for %s\n%s\n", message.ID, target.Name, participantText(target)); err != nil {
		return exitFailed, err
	}
	if target.Kind == comms.KindThread {
		if _, err := fmt.Fprintf(stdout, "message %s wakes dormant thread %s; this is a dispatch that spends model capacity\n", message.ID, target.Name); err != nil {
			return exitFailed, err
		}
		maxWait, err := commsMaxWait(values.MaxWait)
		if err != nil {
			return exitUsage, err
		}
		response, received, err := wakeThread(ctx, store, caller, target, message, maxWait, values.RequireReply)
		if err != nil {
			return exitFailed, err
		}
		if values.RequireReply && received {
			if _, err := fmt.Fprintf(stdout, "response from %s at %s:\n%s\n", response.From, response.SentAt.UTC().Format(time.RFC3339), response.Body); err != nil {
				return exitFailed, err
			}
			return exitOK, nil
		}
		if values.RequireReply {
			if _, err := fmt.Fprintf(stdout, "participant %s is %s; reached --max-wait %s with queued message %s\n", target.Name, activityText(target.Activity), *maxWait, message.ID); err != nil {
				return exitFailed, err
			}
		}
		return exitOK, nil
	}
	if !values.RequireReply {
		return exitOK, nil
	}
	maxWait, err := commsMaxWait(values.MaxWait)
	if err != nil {
		return exitUsage, err
	}
	response, ok, err := awaitCommsResponse(ctx, store, message.ID, maxWait)
	if err != nil {
		return exitFailed, err
	}
	if ok {
		if _, err := fmt.Fprintf(stdout, "response from %s at %s:\n%s\n", response.From, response.SentAt.UTC().Format(time.RFC3339), response.Body); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if _, err := fmt.Fprintf(stdout, "participant %s is %s; reached --max-wait %s with queued message %s\n", target.Name, activityText(target.Activity), *maxWait, message.ID); err != nil {
		return exitFailed, err
	}
	return exitOK, nil
}

func wakeThread(ctx context.Context, store comms.Store, caller comms.Participant, target comms.Participant, message comms.Message, maxWait *time.Duration, requireResponse bool) (comms.Message, bool, error) {
	if target.ThreadID == "" {
		return comms.Message{}, false, fmt.Errorf("thread participant %q has no thread id", target.Name)
	}
	if target.Harness == "" {
		return comms.Message{}, false, fmt.Errorf("thread participant %q has no harness", target.Name)
	}
	adapter, err := newRegistry().Resolve(target.Harness)
	if err != nil {
		return comms.Message{}, false, fmt.Errorf("resolve harness %q for thread participant %q: %w", target.Harness, target.Name, err)
	}
	if !adapter.Capabilities().Continuation {
		return comms.Message{}, false, harness.NewError(harness.ErrorCodeContinuationUnsupported, fmt.Sprintf("thread participant %q uses harness %q, which cannot resume a thread", target.Name, target.Harness), nil)
	}
	wakeCtx := ctx
	cancel := func() {}
	if requireResponse && maxWait != nil {
		wakeCtx, cancel = context.WithTimeout(ctx, *maxWait)
	}
	defer cancel()
	storeForJob := jobs.NewStore(wakeCtx)
	defer storeForJob.Shutdown(context.Background())
	record, err := storeForJob.Resume(wakeCtx, adapter, jobs.ResumeOptions{ThreadID: target.ThreadID, Prompt: threadPrompt(caller, message), Model: stringPointer(target.Model), CWD: target.CWD, Environment: []string{jobs.CommsParticipantEnvironmentEntry(target.Name)}})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return comms.Message{}, false, nil
		}
		return comms.Message{}, false, fmt.Errorf("resume thread participant %q: %w", target.Name, err)
	}
	finished, err := storeForJob.Wait(wakeCtx, record.JobID)
	if errors.Is(err, context.DeadlineExceeded) {
		return comms.Message{}, false, nil
	}
	if err != nil {
		return comms.Message{}, false, fmt.Errorf("wait for thread participant %q: %w", target.Name, err)
	}
	if finished.Status != harness.JobStatusDone {
		return comms.Message{}, false, fmt.Errorf("thread participant %q completed with state %q", target.Name, finished.Status)
	}
	if err := store.MarkDelivered(ctx, target.Name, []string{message.ID}); err != nil {
		return comms.Message{}, false, err
	}
	if !requireResponse {
		return comms.Message{}, false, nil
	}
	if finished.Output == nil || strings.TrimSpace(*finished.Output) == "" {
		return comms.Message{}, false, fmt.Errorf("thread participant %q completed without an answer", target.Name)
	}
	response, err := store.Send(ctx, comms.Message{From: target.Name, To: caller.Name, Body: *finished.Output, InResponseTo: message.ID})
	if err != nil {
		return comms.Message{}, false, err
	}
	return response, true, nil
}

func threadPrompt(caller comms.Participant, message comms.Message) string {
	return fmt.Sprintf("Landing message %s from %s at %s:\n%s", message.ID, caller.Name, message.SentAt.UTC().Format(time.RFC3339), message.Body)
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func commsMaxWait(supplied parsedOption) (*time.Duration, error) {
	if !supplied.Set {
		return nil, nil
	}
	duration, err := time.ParseDuration(supplied.Value)
	if err != nil || duration <= 0 {
		return nil, &usageError{message: fmt.Sprintf("--max-wait has invalid duration %q", supplied.Value)}
	}
	return &duration, nil
}

func awaitCommsResponse(ctx context.Context, store comms.Store, messageID string, maxWait *time.Duration) (comms.Message, bool, error) {
	if maxWait != nil {
		return store.AwaitResponse(ctx, messageID, *maxWait)
	}
	for {
		response, received, err := store.AwaitResponse(ctx, messageID, unboundedWaitStep)
		if err != nil || received {
			return response, received, err
		}
	}
}

func replyComms(ctx context.Context, store comms.Store, caller comms.Participant, values options, stdout io.Writer) (int, error) {
	original, ok, err := messageByID(ctx, store, values.Reply.Value)
	if err != nil {
		return exitFailed, err
	}
	if !ok {
		return exitUsage, &usageError{message: fmt.Sprintf("message %q is not in recent history", values.Reply.Value)}
	}
	if !original.RequiresResponse {
		return exitUsage, &usageError{message: fmt.Sprintf("message %q did not request a response", original.ID)}
	}
	if original.To != caller.Name && original.To != comms.AllAgents {
		return exitUsage, &usageError{message: fmt.Sprintf("message %q is not addressed to %q", original.ID, caller.Name)}
	}
	message, err := store.Send(ctx, comms.Message{From: caller.Name, To: original.From, Body: values.Message.Value, InResponseTo: original.ID})
	if err != nil {
		return exitFailed, err
	}
	if _, err := fmt.Fprintf(stdout, "queued response %s for %s\n", message.ID, original.From); err != nil {
		return exitFailed, err
	}
	return exitOK, nil
}

func writeInbox(ctx context.Context, store comms.Store, caller comms.Participant, asJSON bool, stdout io.Writer) (int, error) {
	messages, err := store.Pending(ctx, caller.Name)
	if err != nil {
		return exitFailed, err
	}
	if len(messages) == 0 {
		if asJSON {
			return writeMessageReports(messages, stdout)
		}
		if _, err := fmt.Fprintf(stdout, "inbox for %s: no pending messages\n", caller.Name); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
		if asJSON {
			continue
		}
		if _, err := fmt.Fprintf(stdout, "%s from %s at %s%s:\n%s\n", message.ID, message.From, message.SentAt.UTC().Format(time.RFC3339), responseRequiredText(message), message.Body); err != nil {
			return exitFailed, err
		}
	}
	if err := store.MarkDelivered(ctx, caller.Name, ids); err != nil {
		return exitFailed, err
	}
	if asJSON {
		return writeMessageReports(messages, stdout)
	}
	return exitOK, nil
}

func writeHistory(ctx context.Context, store comms.Store, asJSON bool, stdout io.Writer) (int, error) {
	messages, err := store.History(ctx, historyWindow)
	if err != nil {
		return exitFailed, err
	}
	if asJSON {
		return writeMessageReports(messages, stdout)
	}
	if len(messages) == 0 {
		if _, err := fmt.Fprintln(stdout, "recent traffic: none"); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	for _, message := range messages {
		if _, err := fmt.Fprintf(stdout, "%s %s -> %s at %s%s:\n%s\n", message.ID, message.From, message.To, message.SentAt.UTC().Format(time.RFC3339), responseRequiredText(message), message.Body); err != nil {
			return exitFailed, err
		}
	}
	return exitOK, nil
}

func responseRequiredText(message comms.Message) string {
	if message.RequiresResponse {
		return " (response requested)"
	}
	return ""
}

func messageByID(ctx context.Context, store comms.Store, id string) (comms.Message, bool, error) {
	messages, err := store.History(ctx, 30*24*time.Hour)
	if err != nil {
		return comms.Message{}, false, err
	}
	for _, message := range messages {
		if message.ID == id {
			return message, true, nil
		}
	}
	return comms.Message{}, false, nil
}

func runWho(ctx context.Context, values options, positionals []string, invocationDir string, stdout io.Writer) (int, error) {
	if err := whoOptions(values, positionals); err != nil {
		return exitUsage, err
	}
	store, err := comms.Open(ctx, invocationDir)
	if err != nil {
		return exitFailed, err
	}
	if values.As.Set {
		if values.JSON {
			return exitUsage, &usageError{message: "option --json does not apply to a who rename"}
		}
		caller, err := callingParticipant(ctx, store)
		if err != nil {
			return exitUsage, err
		}
		if values.As.Value == "" || values.As.Value == comms.AllAgents {
			return exitUsage, &usageError{message: fmt.Sprintf("--as name %q is not available", values.As.Value)}
		}
		if err := store.Rename(ctx, caller.Name, values.As.Value); err != nil {
			return exitUsage, err
		}
		if _, err := fmt.Fprintf(stdout, "renamed calling participant %s to %s\n", caller.Name, values.As.Value); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	participants, err := store.Participants(ctx)
	if err != nil {
		return exitFailed, err
	}
	if len(positionals) == 1 {
		participant, ok := participantNamed(participants, positionals[0])
		if !ok {
			return exitUsage, &usageError{message: fmt.Sprintf("participant %q is not registered", positionals[0])}
		}
		if values.JSON {
			return writeParticipantReport(participant, true, stdout)
		}
		if _, err := fmt.Fprintf(stdout, "%s\n  pid: %d\n  cwd: %s\n  started: %s\n", participantText(participant), participant.PID, participant.CWD, participant.StartedAt.UTC().Format(time.RFC3339)); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if len(participants) == 0 {
		if values.JSON {
			return writeParticipantReports(participants, stdout)
		}
		if _, err := fmt.Fprintln(stdout, "no participants"); err != nil {
			return exitFailed, err
		}
		return exitOK, nil
	}
	if values.JSON {
		return writeParticipantReports(participants, stdout)
	}
	for _, participant := range participants {
		if _, err := fmt.Fprintln(stdout, participantText(participant)); err != nil {
			return exitFailed, err
		}
	}
	return exitOK, nil
}

func whoOptions(values options, positionals []string) error {
	if len(positionals) > 1 {
		return &usageError{message: "who has unexpected arguments"}
	}
	if values.As.Set && len(positionals) != 0 {
		return &usageError{message: "who --as has a participant argument"}
	}
	return unexpectedOptions(values, "who", "as", "json")
}

type messageReport struct {
	ID               string    `json:"id"`
	From             string    `json:"from"`
	To               string    `json:"to"`
	Body             string    `json:"body"`
	SentAt           time.Time `json:"sentAt"`
	RequiresResponse bool      `json:"requiresResponse"`
}

func writeMessageReports(messages []comms.Message, stdout io.Writer) (int, error) {
	reports := make([]messageReport, 0, len(messages))
	for _, message := range messages {
		reports = append(reports, messageReport{ID: message.ID, From: message.From, To: message.To, Body: message.Body, SentAt: message.SentAt, RequiresResponse: message.RequiresResponse})
	}
	encoded, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return exitFailed, err
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

type activityReport struct {
	State string     `json:"state"`
	Since *time.Time `json:"since,omitempty"`
}

type participantReport struct {
	Name     string         `json:"name"`
	Kind     comms.Kind     `json:"kind"`
	Harness  string         `json:"harness"`
	Activity activityReport `json:"activity"`
	PID      *int           `json:"pid,omitempty"`
	CWD      string         `json:"cwd,omitempty"`
	Started  *time.Time     `json:"started,omitempty"`
}

func reportParticipant(participant comms.Participant, detailed bool) participantReport {
	var since *time.Time
	if !participant.Activity.Since.IsZero() {
		since = &participant.Activity.Since
	}
	report := participantReport{Name: participant.Name, Kind: participant.Kind, Harness: participant.Harness, Activity: activityReport{State: string(participant.Activity.State), Since: since}}
	if !detailed {
		return report
	}
	report.PID = &participant.PID
	report.CWD = participant.CWD
	report.Started = &participant.StartedAt

	return report
}

func writeParticipantReport(participant comms.Participant, detailed bool, stdout io.Writer) (int, error) {
	encoded, err := json.MarshalIndent(reportParticipant(participant, detailed), "", "  ")
	if err != nil {
		return exitFailed, err
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func writeParticipantReports(participants []comms.Participant, stdout io.Writer) (int, error) {
	reports := make([]participantReport, 0, len(participants))
	for _, participant := range participants {
		reports = append(reports, reportParticipant(participant, false))
	}
	encoded, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return exitFailed, err
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return exitFailed, err
	}

	return exitOK, nil
}

func participantNamed(participants []comms.Participant, name string) (comms.Participant, bool) {
	for _, participant := range participants {
		if participant.Name == name {
			return participant, true
		}
	}
	return comms.Participant{}, false
}

func participantText(participant comms.Participant) string {
	return fmt.Sprintf("%s: %s, %s, %s", participant.Name, participant.Kind, participant.Harness, activityText(participant.Activity))
}

func activityText(activity comms.Activity) string {
	if activity.Since.IsZero() {
		return string(activity.State)
	}
	return fmt.Sprintf("%s since %s", activity.State, activity.Since.UTC().Format(time.RFC3339))
}

func callingParticipant(ctx context.Context, store comms.Store) (comms.Participant, error) {
	participants, err := store.Participants(ctx)
	if err != nil {
		return comms.Participant{}, err
	}
	if jobs.IsCommsProbeEnvironment() {
		return comms.Participant{}, &usageError{message: "calling process is a Landing capacity probe"}
	}
	if name := os.Getenv(jobs.CommsParticipantEnvironment); name != "" {
		for _, participant := range participants {
			if participant.Name == name && participant.Kind == comms.KindThread {
				return participant, nil
			}
		}
		return comms.Participant{}, &usageError{message: "calling process has no registered Landing thread"}
	}
	pids, err := callerAncestorPIDs(ctx)
	if err != nil {
		for _, participant := range participants {
			if participant.Kind == comms.KindSession && participant.PID == os.Getpid() {
				return participant, nil
			}
		}
		return comms.Participant{}, &usageError{message: "calling process is not inside a registered session"}
	}
	matches := make([]comms.Participant, 0, 1)
	for _, participant := range participants {
		if participant.Kind != comms.KindSession || participant.PID == 0 {
			continue
		}
		if _, ok := pids[participant.PID]; ok {
			matches = append(matches, participant)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return comms.Participant{}, &usageError{message: "calling process is not inside a registered session"}
}

type process struct {
	pid     int
	parent  int
	command string
}

func callerAncestorPIDs(ctx context.Context) (map[int]struct{}, error) {
	processes, err := processes(ctx)
	if err != nil {
		return nil, err
	}
	ancestors := make(map[int]struct{})
	for pid := os.Getpid(); pid > 0; {
		if _, seen := ancestors[pid]; seen {
			break
		}
		ancestors[pid] = struct{}{}
		entry, ok := processes[pid]
		if !ok || entry.parent == pid {
			break
		}
		pid = entry.parent
	}
	return ancestors, nil
}

func sessionPID(ctx context.Context, harness string) int {
	entries, err := processes(ctx)
	if err != nil {
		return 0
	}
	for pid := os.Getpid(); pid > 0; {
		entry, ok := entries[pid]
		if !ok {
			return 0
		}
		if processRunsHarness(entry.command, harness) {
			return entry.pid
		}
		if entry.parent == pid {
			return 0
		}
		pid = entry.parent
	}
	return 0
}

func processes(ctx context.Context) (map[int]process, error) {
	command := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,command=")
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	entries := make(map[int]process)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		parent, parentErr := strconv.Atoi(fields[1])
		if pidErr != nil || parentErr != nil {
			continue
		}
		entries[pid] = process{pid: pid, parent: parent, command: strings.Join(fields[2:], " ")}
	}
	return entries, nil
}

func processRunsHarness(command string, harness string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	name := strings.TrimSuffix(strings.ToLower(fields[0]), ".exe")
	return strings.TrimSuffix(name, "/"+harness) == harness || strings.HasSuffix(name, "/"+harness)
}

type hookInput struct {
	SessionID string
	CWD       string
	PID       int
	Tool      string
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

type hookOutput struct {
	Decision           string              `json:"decision,omitempty"`
	Reason             string              `json:"reason,omitempty"`
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

func runCommsHook(ctx context.Context, values options, invocationDir string, stdin io.Reader, stdout io.Writer) int {
	if !values.Harness.Set || !values.Event.Set || !validHookHarness(values.Harness.Value) || !validHookEvent(values.Event.Value) {
		return exitOK
	}
	if jobs.IsCommsProbeEnvironment() {
		return exitOK
	}
	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()
	input, err := decodeHookInput(stdin)
	if err != nil {
		return exitOK
	}
	store, err := comms.Open(hookCtx, invocationDir)
	if err != nil {
		return exitOK
	}
	participant, err := hookParticipant(hookCtx, store, values.Harness.Value, input, invocationDir)
	if err != nil {
		return exitOK
	}
	if err := applyHookEvent(hookCtx, store, participant, values.Event.Value, input.Tool); err != nil {
		return exitOK
	}
	pending, err := store.Pending(hookCtx, participant.Name)
	if err != nil {
		return exitOK
	}
	if values.Event.Value == "session-start" {
		if values.Harness.Value == "grok" {
			return exitOK
		}
		text, err := sessionStartText(hookCtx, store, pending)
		if err != nil || !emitHookJSON(stdout, hookOutput{HookSpecificOutput: &hookSpecificOutput{HookEventName: hookEventName(values.Event.Value), AdditionalContext: text}}) {
			return exitOK
		}
		return markHookDelivered(hookCtx, store, participant.Name, pending)
	}
	if len(pending) == 0 {
		return exitOK
	}
	text := hookMessages(pending)
	if values.Event.Value == "turn-end" {
		if !emitHookJSON(stdout, hookOutput{Decision: "block", Reason: text}) {
			return exitOK
		}
		return markHookDelivered(hookCtx, store, participant.Name, pending)
	}
	if values.Event.Value == "pre-tool-use" || values.Event.Value == "post-tool-use" {
		if values.Harness.Value == "grok" {
			return exitOK
		}
		if !emitHookJSON(stdout, hookOutput{HookSpecificOutput: &hookSpecificOutput{HookEventName: hookEventName(values.Event.Value), AdditionalContext: text}}) {
			return exitOK
		}
		return markHookDelivered(hookCtx, store, participant.Name, pending)
	}
	return exitOK
}

func validHookHarness(harness string) bool {
	return harness == "claude" || harness == "codex" || harness == "grok"
}

func validHookEvent(event string) bool {
	switch event {
	case "session-start", "pre-tool-use", "post-tool-use", "turn-end", "session-end":
		return true
	default:
		return false
	}
}

func decodeHookInput(reader io.Reader) (hookInput, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 1<<20))
	decoder.DisallowUnknownFields()
	var payload map[string]json.RawMessage
	if err := decoder.Decode(&payload); err != nil {
		return hookInput{}, err
	}
	input := hookInput{SessionID: hookString(payload, "session_id", "sessionId"), CWD: hookString(payload, "cwd"), Tool: hookString(payload, "tool_name", "toolName")}
	input.PID = hookInt(payload, "pid", "process_id", "processId")
	if input.SessionID == "" {
		return hookInput{}, errors.New("hook session id is empty")
	}
	return input, nil
}

func hookString(payload map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		var value string
		if err := json.Unmarshal(payload[name], &value); err == nil {
			return value
		}
	}
	return ""
}

func hookInt(payload map[string]json.RawMessage, names ...string) int {
	for _, name := range names {
		var value int
		if err := json.Unmarshal(payload[name], &value); err == nil {
			return value
		}
	}
	return 0
}

func hookParticipant(ctx context.Context, store comms.Store, harness string, input hookInput, invocationDir string) (comms.Participant, error) {
	participants, err := store.Participants(ctx)
	if err != nil {
		return comms.Participant{}, err
	}
	if name := os.Getenv(jobs.CommsParticipantEnvironment); name != "" {
		for _, participant := range participants {
			if participant.Name == name && participant.Kind == comms.KindThread {
				return participant, nil
			}
		}
		return comms.Participant{}, errors.New("hook process has no registered Landing thread")
	}
	for _, participant := range participants {
		if participant.Kind == comms.KindSession && participant.SessionID == input.SessionID {
			updated := participant
			if input.PID != 0 {
				updated.PID = input.PID
			}
			if input.CWD != "" {
				updated.CWD = input.CWD
			}
			if updated != participant {
				if err := store.Register(ctx, updated); err != nil {
					return comms.Participant{}, err
				}
			}
			return updated, nil
		}
	}
	pid := input.PID
	if pid == 0 {
		pid = sessionPID(ctx, harness)
	}
	cwd := input.CWD
	if cwd == "" {
		cwd = invocationDir
	}
	name := harness + "-" + shortSessionID(input.SessionID)
	participant := comms.Participant{Name: name, Kind: comms.KindSession, Harness: harness, CWD: cwd, StartedAt: time.Now().UTC(), PID: pid, SessionID: input.SessionID, Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()}}
	if err := store.Register(ctx, participant); err != nil {
		return comms.Participant{}, err
	}
	return participant, nil
}

func shortSessionID(id string) string {
	cleaned := strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			return character
		}
		return -1
	}, id)
	if len(cleaned) > 12 {
		return cleaned[:12]
	}
	return cleaned
}

func applyHookEvent(ctx context.Context, store comms.Store, participant comms.Participant, event string, tool string) error {
	switch event {
	case "session-start":
		return nil
	case "pre-tool-use", "post-tool-use":
		return store.ReportActivity(ctx, participant.Name, comms.Activity{State: comms.StateInTurn, Since: time.Now().UTC(), LastTool: tool})
	case "turn-end":
		return store.ReportActivity(ctx, participant.Name, comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()})
	case "session-end":
		if participant.Kind == comms.KindThread {
			return nil
		}
		return store.Deregister(ctx, participant.Name)
	default:
		return errors.New("unknown hook event")
	}
}

func hookMessages(messages []comms.Message) string {
	lines := make([]string, 0, len(messages)+1)
	lines = append(lines, "Landing messages waiting:")
	for _, message := range messages {
		lines = append(lines, fmt.Sprintf("%s from %s at %s%s: %s", message.ID, message.From, message.SentAt.UTC().Format(time.RFC3339), responseRequiredText(message), message.Body))
	}
	return strings.Join(lines, "\n")
}

func sessionStartText(ctx context.Context, store comms.Store, pending []comms.Message) (string, error) {
	lines := []string{"Landing communication is available for this project."}
	history, err := store.History(ctx, historyWindow)
	if err != nil {
		return "", err
	}
	if len(history) != 0 {
		lines = append(lines, "Recent traffic:")
		for _, message := range history[max(0, len(history)-5):] {
			lines = append(lines, fmt.Sprintf("%s from %s to %s at %s: %s", message.ID, message.From, message.To, message.SentAt.UTC().Format(time.RFC3339), message.Body))
		}
	}
	if len(pending) != 0 {
		lines = append(lines, hookMessages(pending))
	}
	return strings.Join(lines, "\n"), nil
}

func markHookDelivered(ctx context.Context, store comms.Store, recipient string, messages []comms.Message) int {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	if err := store.MarkDelivered(ctx, recipient, ids); err != nil {
		return exitOK
	}
	return exitOK
}

func hookEventName(event string) string {
	switch event {
	case "session-start":
		return "SessionStart"
	case "pre-tool-use":
		return "PreToolUse"
	case "post-tool-use":
		return "PostToolUse"
	default:
		return ""
	}
}

func emitHookJSON(stdout io.Writer, output hookOutput) bool {
	encoded, err := json.Marshal(output)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return false
	}
	return true
}
