package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/on-mission/landing/internal/comms"
	"github.com/on-mission/landing/internal/jobs"
)

func TestRunCommsQueuesMessageForRegisteredParticipant(t *testing.T) {
	project, store := commsTestStore(t)
	registerCommsParticipant(t, store, comms.Participant{
		Name:     "alice",
		Kind:     comms.KindSession,
		Harness:  "codex",
		PID:      os.Getpid(),
		Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()},
	})
	registerCommsParticipant(t, store, comms.Participant{
		Name:     "bob",
		Kind:     comms.KindSession,
		Harness:  "claude",
		Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()},
	})

	code, err, stdout, _ := runCLI(t, project, []string{"comms", "--agent", "bob", "--message", "review the store"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(comms send) = %d, %v; want %d, nil", code, err, exitOK)
	}
	if !strings.Contains(stdout.String(), "queued message") || !strings.Contains(stdout.String(), "bob: session, claude, ended") {
		t.Fatalf("Run(comms send) output = %q, want message id and bob state", stdout.String())
	}
	pending, err := store.Pending(context.Background(), "bob")
	if err != nil || len(pending) != 1 || pending[0].From != "alice" || pending[0].Body != "review the store" {
		t.Fatalf("Pending(bob) = %#v, %v; want alice message", pending, err)
	}
}

func TestRunWhoAndCommsHistoryJSON(t *testing.T) {
	project, store := commsTestStore(t)
	participant := comms.Participant{
		Name:      "alice",
		Kind:      comms.KindSession,
		Harness:   "codex",
		PID:       os.Getpid(),
		CWD:       project,
		StartedAt: time.Now().UTC(),
		Activity:  comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()},
	}
	registerCommsParticipant(t, store, participant)
	message, err := store.Send(context.Background(), comms.Message{From: "alice", To: "all", Body: "status"})
	if err != nil {
		t.Fatalf("Send() returned unexpected error: %v", err)
	}

	code, err, stdout, _ := runCLI(t, project, []string{"who", "--json"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(who --json) = %d, %v; want %d, nil", code, err, exitOK)
	}
	var participants []participantReport
	if err := json.Unmarshal(stdout.Bytes(), &participants); err != nil || len(participants) != 1 || participants[0].Name != participant.Name || participants[0].Activity.State != string(comms.StateIdle) {
		t.Fatalf("who --json = %q, %v; want alice participant report", stdout.String(), err)
	}

	code, err, stdout, _ = runCLI(t, project, []string{"comms", "--history", "--json"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(comms --history --json) = %d, %v; want %d, nil", code, err, exitOK)
	}
	var messages []messageReport
	if err := json.Unmarshal(stdout.Bytes(), &messages); err != nil || len(messages) != 1 || messages[0].ID != message.ID || messages[0].Body != message.Body {
		t.Fatalf("comms --history --json = %q, %v; want status message report", stdout.String(), err)
	}
}

func TestCommandOptionErrorsNameTheTypedOptions(t *testing.T) {
	project, _ := commsTestStore(t)
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"who", "--tier=review"}, want: `option --tier=review does not apply to command "who"`},
		{args: []string{"comms", "--history", "--name", "probe"}, want: `option --name does not apply to command "comms"`},
	} {
		code, err, _, _ := runCLI(t, project, test.args)
		if err == nil || code != exitUsage || err.Error() != test.want {
			t.Fatalf("Run(%q) = %d, %v; want %d, %q", test.args, code, err, exitUsage, test.want)
		}
	}
}

func TestRunCommsHookUsesHarnessSpecificOutputAndMarksDelivery(t *testing.T) {
	project, store := commsTestStore(t)
	registerCommsParticipant(t, store, comms.Participant{Name: "sender", Kind: comms.KindThread, Harness: "codex", Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()}})

	tests := map[string]struct {
		harness  string
		wantText bool
	}{
		"Claude injects session-start context": {harness: "claude", wantText: true},
		"Codex injects session-start context":  {harness: "codex", wantText: true},
		"Grok stays silent at session start":   {harness: "grok", wantText: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sessionID := test.harness + "-session"
			code, err, stdout := runCommsHookForTest(project, []string{"comms", "hook", "--harness", test.harness, "--event", "session-start"}, hookPayload(t, sessionID, project))
			if err != nil || code != exitOK {
				t.Fatalf("Run(comms hook session-start) = %d, %v; want %d, nil", code, err, exitOK)
			}
			if test.wantText && !strings.Contains(stdout.String(), `"hookEventName":"SessionStart"`) {
				t.Fatalf("Run(comms hook session-start) output = %q, want structured SessionStart context", stdout.String())
			}
			if !test.wantText && stdout.Len() != 0 {
				t.Fatalf("Run(comms hook session-start) output = %q, want empty Grok output", stdout.String())
			}

			participantName := test.harness + "-" + shortSessionID(sessionID)
			message, sendErr := store.Send(context.Background(), comms.Message{From: "sender", To: participantName, Body: "status update"})
			if sendErr != nil {
				t.Fatalf("Send() returned unexpected error: %v", sendErr)
			}
			code, err, stdout = runCommsHookForTest(project, []string{"comms", "hook", "--harness", test.harness, "--event", "turn-end"}, hookPayload(t, sessionID, project))
			if err != nil || code != exitOK {
				t.Fatalf("Run(comms hook turn-end) = %d, %v; want %d, nil", code, err, exitOK)
			}
			var output hookOutput
			if decodeErr := json.Unmarshal(stdout.Bytes(), &output); decodeErr != nil || output.Decision != "block" || !strings.Contains(output.Reason, message.ID) {
				t.Fatalf("Run(comms hook turn-end) output = %q, %v; want blocking message delivery", stdout.String(), decodeErr)
			}
			pending, pendingErr := store.Pending(context.Background(), participantName)
			if pendingErr != nil || len(pending) != 0 {
				t.Fatalf("Pending(%q) after hook delivery = %#v, %v; want none", participantName, pending, pendingErr)
			}
		})
	}
}

func TestRunCommsHookFailsOpenWhenStateRootIsUnreadable(t *testing.T) {
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".landing"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(t.TempDir(), "state-file")
	if err := os.WriteFile(stateFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LANDING_STATE_DIR", stateFile)
	code, err, stdout := runCommsHookForTest(project, []string{"comms", "hook", "--harness", "codex", "--event", "session-start"}, hookPayload(t, "broken-store", project))
	if err != nil || code != exitOK || stdout.Len() != 0 {
		t.Fatalf("Run(comms hook with unreadable state root) = %d, %v, %q; want %d, nil, empty output", code, err, stdout.String(), exitOK)
	}
}

func TestCommsProvenanceKeepsDispatchedChildOnItsThread(t *testing.T) {
	project, store := commsTestStore(t)
	registerCommsParticipant(t, store, comms.Participant{
		Name:     "thread",
		Kind:     comms.KindThread,
		Harness:  "claude",
		CWD:      project,
		Activity: comms.Activity{State: comms.StateInTurn, Since: time.Now().UTC()},
	})
	registerCommsParticipant(t, store, comms.Participant{
		Name:     "reviewer",
		Kind:     comms.KindSession,
		Harness:  "codex",
		Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()},
	})
	t.Setenv(jobs.CommsParticipantEnvironment, "thread")

	code, err, _, _ := runCLI(t, project, []string{"comms", "--agent", "reviewer", "--message", "status"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(comms send from dispatched child) = %d, %v; want %d, nil", code, err, exitOK)
	}
	pending, err := store.Pending(context.Background(), "reviewer")
	if err != nil || len(pending) != 1 || pending[0].From != "thread" {
		t.Fatalf("Pending(reviewer) = %#v, %v; want message from thread", pending, err)
	}
	if _, err := store.Send(context.Background(), comms.Message{From: "reviewer", To: "thread", Body: "received"}); err != nil {
		t.Fatalf("Send(thread message) returned unexpected error: %v", err)
	}
	code, err, stdout := runCommsHookForTest(project, []string{"comms", "hook", "--harness", "claude", "--event", "turn-end"}, hookPayload(t, "child-session", project))
	if err != nil || code != exitOK || !strings.Contains(stdout.String(), "received") {
		t.Fatalf("Run(comms hook for dispatched child) = %d, %v, %q; want delivered thread message", code, err, stdout.String())
	}
	code, err, _ = runCommsHookForTest(project, []string{"comms", "hook", "--harness", "claude", "--event", "session-end"}, hookPayload(t, "child-session", project))
	if err != nil || code != exitOK {
		t.Fatalf("Run(comms hook session-end for dispatched child) = %d, %v; want %d, nil", code, err, exitOK)
	}
	participants, err := store.Participants(context.Background())
	thread := comms.Participant{}
	for _, participant := range participants {
		if participant.Name == "thread" {
			thread = participant
		}
	}
	if err != nil || len(participants) != 2 || thread.Kind != comms.KindThread {
		t.Fatalf("Participants() after dispatched child hooks = %#v, %v; want thread and reviewer only", participants, err)
	}
}

func TestCommsProbeProvenanceDoesNotRegisterHookSession(t *testing.T) {
	project, store := commsTestStore(t)
	t.Setenv(jobs.CommsProbeEnvironment, "1")

	code, err, stdout := runCommsHookForTest(project, []string{"comms", "hook", "--harness", "claude", "--event", "session-start"}, hookPayload(t, "capacity-probe", project))
	if err != nil || code != exitOK || stdout.Len() != 0 {
		t.Fatalf("Run(comms hook for capacity probe) = %d, %v, %q; want %d, nil, empty output", code, err, stdout.String(), exitOK)
	}
	participants, err := store.Participants(context.Background())
	if err != nil || len(participants) != 0 {
		t.Fatalf("Participants() after capacity probe hook = %#v, %v; want no participants", participants, err)
	}
}

func TestRunCommsRequireResponseResumesThread(t *testing.T) {
	project, store := commsTestStore(t)
	installFakeCodex(t)
	registerCommsParticipant(t, store, comms.Participant{
		Name:     "alice",
		Kind:     comms.KindSession,
		Harness:  "codex",
		PID:      os.Getpid(),
		Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()},
	})
	registerCommsParticipant(t, store, comms.Participant{
		Name:     "thread",
		Kind:     comms.KindThread,
		Harness:  "codex",
		Model:    "gpt-5.6-terra",
		CWD:      project,
		ThreadID: "thread-123",
		Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()},
	})

	code, err, stdout, _ := runCLI(t, project, []string{"comms", "--agent", "thread", "--message", "please answer", "--require-response"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(comms require-response thread) = %d, %v; want %d, nil", code, err, exitOK)
	}
	if !strings.Contains(stdout.String(), "spends model capacity") || !strings.Contains(stdout.String(), "response from thread") || !strings.Contains(stdout.String(), "thread answer") {
		t.Fatalf("Run(comms require-response thread) output = %q, want wake and resumed response", stdout.String())
	}
	pending, pendingErr := store.Pending(context.Background(), "thread")
	if pendingErr != nil || len(pending) != 0 {
		t.Fatalf("Pending(thread) after resume = %#v, %v; want none", pending, pendingErr)
	}
}

func TestRunCommsWaitsWithoutDefaultLimitUntilResponse(t *testing.T) {
	project, store := commsTestStore(t)
	registerCommsParticipant(t, store, comms.Participant{Name: "alice", Kind: comms.KindSession, PID: os.Getpid(), Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()}})
	registerCommsParticipant(t, store, comms.Participant{Name: "bob", Kind: comms.KindSession, Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()}})

	type commandResult struct {
		code   int
		err    error
		stdout *bytes.Buffer
	}
	result := make(chan commandResult, 1)
	go func() {
		stdout := &bytes.Buffer{}
		code, err := Run(context.Background(), Inputs{Args: []string{"comms", "--agent", "bob", "--message", "please answer", "--require-response"}, InvocationDir: project, Stdin: strings.NewReader(""), Stdout: stdout, Stderr: &bytes.Buffer{}})
		result <- commandResult{code: code, err: err, stdout: stdout}
	}()
	request := pendingCommsMessage(t, store, "bob")
	if _, err := store.Send(context.Background(), comms.Message{From: "bob", To: "alice", Body: "considered answer", InResponseTo: request.ID}); err != nil {
		t.Fatalf("Send(response) returned unexpected error: %v", err)
	}

	select {
	case completed := <-result:
		if completed.err != nil || completed.code != exitOK {
			t.Fatalf("Run(comms require-response without max wait) = %d, %v; want %d, nil", completed.code, completed.err, exitOK)
		}
		if !strings.Contains(completed.stdout.String(), "response from bob") || !strings.Contains(completed.stdout.String(), "considered answer") {
			t.Fatalf("Run(comms require-response without max wait) output = %q, want response", completed.stdout.String())
		}
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
}

func TestRunCommsMaxWaitLeavesMessageQueued(t *testing.T) {
	project, store := commsTestStore(t)
	registerCommsParticipant(t, store, comms.Participant{Name: "alice", Kind: comms.KindSession, PID: os.Getpid(), Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()}})
	registerCommsParticipant(t, store, comms.Participant{Name: "bob", Kind: comms.KindSession, Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()}})

	code, err, stdout, _ := runCLI(t, project, []string{"comms", "--agent", "bob", "--message", "please answer", "--require-response", "--max-wait", "25ms"})
	if err != nil || code != exitOK {
		t.Fatalf("Run(comms require-response max wait) = %d, %v; want %d, nil", code, err, exitOK)
	}
	if !strings.Contains(stdout.String(), "reached --max-wait 25ms") || !strings.Contains(stdout.String(), "queued message") {
		t.Fatalf("Run(comms require-response max wait) output = %q, want capped wait description", stdout.String())
	}
	pending := pendingCommsMessage(t, store, "bob")
	if pending.Body != "please answer" || !pending.RequiresResponse {
		t.Fatalf("Pending(bob) after max wait = %#v, want queued response request", pending)
	}
}

func TestRunCommsUnlimitedWaitStopsWhenContextIsCancelled(t *testing.T) {
	project, store := commsTestStore(t)
	registerCommsParticipant(t, store, comms.Participant{Name: "alice", Kind: comms.KindSession, PID: os.Getpid(), Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()}})
	registerCommsParticipant(t, store, comms.Participant{Name: "bob", Kind: comms.KindSession, Activity: comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := Run(ctx, Inputs{Args: []string{"comms", "--agent", "bob", "--message", "please answer", "--require-response"}, InvocationDir: project, Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
		result <- err
	}()
	pendingCommsMessage(t, store, "bob")
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run(comms unlimited wait) error = %v, want context canceled", err)
		}
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
}

func commsTestStore(t *testing.T) (string, comms.Store) {
	t.Helper()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".landing"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	store, err := comms.Open(context.Background(), project)
	if err != nil {
		t.Fatalf("Open(%q) returned unexpected error: %v", project, err)
	}
	return project, store
}

func registerCommsParticipant(t *testing.T, store comms.Store, participant comms.Participant) {
	t.Helper()
	if err := store.Register(context.Background(), participant); err != nil {
		t.Fatalf("Register(%q) returned unexpected error: %v", participant.Name, err)
	}
}

func pendingCommsMessage(t *testing.T, store comms.Store, participant string) comms.Message {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		pending, err := store.Pending(context.Background(), participant)
		if err != nil {
			t.Fatalf("Pending(%q) returned unexpected error: %v", participant, err)
		}
		if len(pending) != 0 {
			return pending[0]
		}
		select {
		case <-timer.C:
			t.Fatalf("Pending(%q) did not receive a message", participant)
		case <-ticker.C:
		}
	}
}

func runCommsHookForTest(project string, args []string, payload string) (int, error, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code, err := Run(context.Background(), Inputs{Args: args, InvocationDir: project, Stdin: strings.NewReader(payload), Stdout: stdout, Stderr: stderr})
	return code, err, stdout
}

// hookPayload builds hook stdin JSON through the encoder rather than string
// concatenation: a Windows cwd contains backslashes, which are invalid raw
// inside a JSON string and corrupt a hand-built literal.
func hookPayload(t *testing.T, sessionID string, cwd string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{"session_id": sessionID, "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestCommsInstallReportsOutcomeDistinctFromThePlan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses POSIX executable shims")
	}
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".landing"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LANDING_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	installHookHarnessShims(t)

	stdout, _ := &bytes.Buffer{}, &bytes.Buffer{}
	code, err := Run(context.Background(), Inputs{Args: []string{"comms", "--install"}, InvocationDir: project, Stdin: strings.NewReader(""), Stdout: stdout, Stderr: &bytes.Buffer{}})
	if err != nil || code != exitOK {
		t.Fatalf("Run(comms --install) = %d, %v; want %d, nil", code, err, exitOK)
	}
	output := stdout.String()
	if !strings.Contains(output, "Codex: create ") {
		t.Fatalf("Run(comms --install) output = %q, want a planned create line", output)
	}
	if !strings.Contains(output, "Codex: created ") {
		t.Fatalf("Run(comms --install) output = %q, want an outcome line stating what was created", output)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if line == "" {
			continue
		}
		if seen[line] {
			t.Fatalf("Run(comms --install) output repeats an identical line %q; full output:\n%s", line, output)
		}
		seen[line] = true
	}
}

func installHookHarnessShims(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(self, filepath.Join(bin, "landing")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude", "codex", "grok"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func installFakeCodex(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a POSIX shell script to fake the codex executable")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "codex")
	contents := "#!/bin/sh\n" +
		"output_file=\"\"\n" +
		"previous=\"\"\n" +
		"for argument in \"$@\"; do\n" +
		"  if [ \"$previous\" = \"--output-last-message\" ]; then\n" +
		"    output_file=\"$argument\"\n" +
		"  fi\n" +
		"  previous=\"$argument\"\n" +
		"done\n" +
		"printf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"thread-123\"}'\n" +
		"printf '%s' 'thread answer' > \"$output_file\"\n"
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("WriteFile(%q) returned unexpected error: %v", path, err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}
