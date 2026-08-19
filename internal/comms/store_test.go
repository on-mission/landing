package comms

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreDeliversDirectMessagesAndKeepsHistory(t *testing.T) {
	store := openTestStore(t)
	registerTestParticipant(t, store, "alice", KindSession)
	registerTestParticipant(t, store, "bob", KindThread)

	sent, err := store.Send(context.Background(), Message{
		From: "alice",
		To:   "bob",
		Body: "please review",
	})
	if err != nil {
		t.Fatalf("Send() returned unexpected error: %v", err)
	}
	if sent.ID == "" {
		t.Fatal("Send() returned an empty message ID")
	}
	if sent.SentAt.IsZero() {
		t.Fatal("Send() returned a zero sent time")
	}

	pending, err := store.Pending(context.Background(), "bob")
	if err != nil {
		t.Fatalf("Pending() returned unexpected error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("Pending() returned %d messages, want 1", len(pending))
	}
	if pending[0].ID != sent.ID {
		t.Fatalf("Pending() message ID = %q, want %q", pending[0].ID, sent.ID)
	}
	if pending[0].Delivered() {
		t.Fatal("Pending() returned a delivered message")
	}

	if err := store.MarkDelivered(context.Background(), "bob", []string{sent.ID}); err != nil {
		t.Fatalf("MarkDelivered() returned unexpected error: %v", err)
	}
	pending, err = store.Pending(context.Background(), "bob")
	if err != nil {
		t.Fatalf("Pending() after MarkDelivered() returned unexpected error: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("Pending() after MarkDelivered() returned %d messages, want 0", len(pending))
	}

	history, err := store.History(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("History() returned unexpected error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("History() returned %d messages, want 1", len(history))
	}
	if !history[0].Delivered() {
		t.Fatal("History() returned an undelivered direct message after MarkDelivered()")
	}
}

func TestStoreReconcilesTerminatedInTurnThreadAndKeepsItAddressable(t *testing.T) {
	if os.Getenv("LANDING_COMMS_THREAD_LIVENESS_HELPER") == "1" {
		select {}
	}

	store := openTestStore(t)
	command := exec.Command(os.Args[0], "-test.run=^TestStoreReconcilesTerminatedInTurnThreadAndKeepsItAddressable$")
	command.Env = append(os.Environ(), "LANDING_COMMS_THREAD_LIVENESS_HELPER=1")
	if err := command.Start(); err != nil {
		t.Fatalf("start thread liveness helper: %v", err)
	}
	t.Cleanup(func() {
		if command.ProcessState != nil {
			return
		}
		if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("kill thread liveness helper during cleanup: %v", err)
		}
		if err := command.Wait(); err == nil {
			t.Error("thread liveness helper exited successfully after cleanup kill")
		}
	})
	if err := store.Register(context.Background(), Participant{
		Name:     "thread",
		Kind:     KindThread,
		PID:      command.Process.Pid,
		ThreadID: "thread-123",
		Activity: Activity{State: StateInTurn, Since: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("Register(thread) returned unexpected error: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill thread liveness helper: %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("thread liveness helper exited without being killed")
	}

	participants, err := store.Participants(context.Background())
	if err != nil {
		t.Fatalf("Participants() returned unexpected error: %v", err)
	}
	if len(participants) != 1 {
		t.Fatalf("Participants() returned %d participants, want 1", len(participants))
	}
	thread := participants[0]
	if thread.Activity.State != StateEnded {
		t.Fatalf("Participants() thread activity after process death = %q, want %q", thread.Activity.State, StateEnded)
	}
	if thread.PID != 0 {
		t.Fatalf("Participants() thread PID = %d, want 0", thread.PID)
	}
	if thread.ThreadID != "thread-123" {
		t.Fatalf("Participants() thread ID = %q, want thread-123", thread.ThreadID)
	}

	message, err := store.Send(context.Background(), Message{From: "sender", To: thread.Name, Body: "continue"})
	if err != nil {
		t.Fatalf("Send(thread) returned unexpected error: %v", err)
	}
	pending, err := store.Pending(context.Background(), thread.Name)
	if err != nil {
		t.Fatalf("Pending(thread) returned unexpected error: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != message.ID {
		t.Fatalf("Pending(thread) = %#v, want message %q", pending, message.ID)
	}
}

func TestStoreReconcilesSessionRegisteredBeforeItsThread(t *testing.T) {
	store := openTestStore(t)
	if err := store.Register(context.Background(), Participant{
		Name:      "codex-session",
		Kind:      KindSession,
		PID:       os.Getpid(),
		SessionID: "session-123",
		Activity:  Activity{State: StateIdle, Since: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("Register(session) returned unexpected error: %v", err)
	}
	message, err := store.Send(context.Background(), Message{From: "sender", To: "codex-session", Body: "review this"})
	if err != nil {
		t.Fatalf("Send(session) returned unexpected error: %v", err)
	}
	name, err := store.RegisterUnique(context.Background(), Participant{
		Name:     "dispatched-review",
		Kind:     KindThread,
		PID:      os.Getpid(),
		ThreadID: "thread-123",
		Activity: Activity{State: StateInTurn, Since: time.Now().UTC()},
	})
	if err != nil {
		t.Fatalf("RegisterUnique(thread) returned unexpected error: %v", err)
	}
	if name != "dispatched-review" {
		t.Fatalf("RegisterUnique(thread) name = %q, want dispatched-review", name)
	}
	participants, err := store.Participants(context.Background())
	if err != nil {
		t.Fatalf("Participants() returned unexpected error: %v", err)
	}
	if len(participants) != 1 || participants[0].Name != name || participants[0].Kind != KindThread {
		t.Fatalf("Participants() = %#v, want only thread %q", participants, name)
	}
	pending, err := store.Pending(context.Background(), "codex-session")
	if err != nil {
		t.Fatalf("Pending(session alias) returned unexpected error: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != message.ID {
		t.Fatalf("Pending(session alias) = %#v, want message %q", pending, message.ID)
	}
}

func TestStoreBroadcastTracksEachRecipientAndRename(t *testing.T) {
	store := openTestStore(t)
	registerTestParticipant(t, store, "alice", KindSession)
	registerTestParticipant(t, store, "bob", KindThread)
	registerTestParticipant(t, store, "carol", KindThread)

	broadcast, err := store.Send(context.Background(), Message{
		From: "alice",
		To:   AllAgents,
		Body: "architecture changed",
	})
	if err != nil {
		t.Fatalf("Send(broadcast) returned unexpected error: %v", err)
	}
	for _, recipient := range []string{"bob", "carol"} {
		pending, err := store.Pending(context.Background(), recipient)
		if err != nil {
			t.Fatalf("Pending(%q) returned unexpected error: %v", recipient, err)
		}
		if len(pending) != 1 || pending[0].ID != broadcast.ID {
			t.Fatalf("Pending(%q) = %#v, want broadcast %q", recipient, pending, broadcast.ID)
		}
	}
	pending, err := store.Pending(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Pending(sender) returned unexpected error: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("Pending(sender) returned %d messages, want 0", len(pending))
	}

	if err := store.MarkDelivered(context.Background(), "bob", []string{broadcast.ID}); err != nil {
		t.Fatalf("MarkDelivered(bob) returned unexpected error: %v", err)
	}
	pending, err = store.Pending(context.Background(), "bob")
	if err != nil {
		t.Fatalf("Pending(bob) after MarkDelivered() returned unexpected error: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("Pending(bob) after MarkDelivered() returned %d messages, want 0", len(pending))
	}
	pending, err = store.Pending(context.Background(), "carol")
	if err != nil {
		t.Fatalf("Pending(carol) after MarkDelivered(bob) returned unexpected error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("Pending(carol) after MarkDelivered(bob) returned %d messages, want 1", len(pending))
	}

	if err := store.Rename(context.Background(), "carol", "drew"); err != nil {
		t.Fatalf("Rename() returned unexpected error: %v", err)
	}
	pending, err = store.Pending(context.Background(), "drew")
	if err != nil {
		t.Fatalf("Pending(drew) returned unexpected error: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != broadcast.ID {
		t.Fatalf("Pending(drew) = %#v, want broadcast %q", pending, broadcast.ID)
	}
}

func TestStoreReportsDeadSessionsAndDoesNotConsumeTimedOutRequests(t *testing.T) {
	store := openTestStore(t)
	registerTestParticipant(t, store, "live", KindSession)
	registerTestParticipant(t, store, "ended", KindSession)
	if err := store.Register(context.Background(), Participant{
		Name: "live",
		Kind: KindSession,
		PID:  os.Getpid(),
		Activity: Activity{
			State: StateIdle,
		},
	}); err != nil {
		t.Fatalf("Register(live) returned unexpected error: %v", err)
	}

	participants, err := store.Participants(context.Background())
	if err != nil {
		t.Fatalf("Participants() returned unexpected error: %v", err)
	}
	states := make(map[string]State, len(participants))
	for _, participant := range participants {
		states[participant.Name] = participant.Activity.State
	}
	if states["live"] != StateIdle {
		t.Fatalf("Participants() live state = %q, want %q", states["live"], StateIdle)
	}
	if states["ended"] != StateEnded {
		t.Fatalf("Participants() ended state = %q, want %q", states["ended"], StateEnded)
	}

	request, err := store.Send(context.Background(), Message{
		From:             "live",
		To:               "ended",
		Body:             "can you respond?",
		RequiresResponse: true,
	})
	if err != nil {
		t.Fatalf("Send(request) returned unexpected error: %v", err)
	}
	_, ok, err := store.AwaitResponse(context.Background(), request.ID, 0)
	if err != nil {
		t.Fatalf("AwaitResponse() returned unexpected error: %v", err)
	}
	if ok {
		t.Fatal("AwaitResponse() returned ok true without a response")
	}
	pending, err := store.Pending(context.Background(), "ended")
	if err != nil {
		t.Fatalf("Pending(ended) returned unexpected error: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != request.ID {
		t.Fatalf("Pending(ended) = %#v, want request %q after AwaitResponse() timed out", pending, request.ID)
	}
}

func TestStoreScopesStateToProject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(root, "state"))
	projectOne := createProjectAt(t, root, "one")
	projectTwo := createProjectAt(t, root, "two")
	storeOne := openStoreAt(t, projectOne)
	storeTwo := openStoreAt(t, projectTwo)
	registerTestParticipant(t, storeOne, "alice", KindThread)
	registerTestParticipant(t, storeTwo, "bob", KindThread)

	participants, err := storeOne.Participants(context.Background())
	if err != nil {
		t.Fatalf("Participants(project one) returned unexpected error: %v", err)
	}
	if len(participants) != 1 || participants[0].Name != "alice" {
		t.Fatalf("Participants(project one) = %#v, want only alice", participants)
	}
	participants, err = storeTwo.Participants(context.Background())
	if err != nil {
		t.Fatalf("Participants(project two) returned unexpected error: %v", err)
	}
	if len(participants) != 1 || participants[0].Name != "bob" {
		t.Fatalf("Participants(project two) = %#v, want only bob", participants)
	}
}

func TestStoreCrossProcess(t *testing.T) {
	if worker := os.Getenv("LANDING_COMMS_HELPER"); worker != "" {
		runCrossProcessWorker(t, worker)
		return
	}

	project := createTestProject(t, "cross-process")
	store := openStoreAt(t, project)
	registerTestParticipant(t, store, "receiver", KindThread)

	const workers = 6
	const messagesPerWorker = 15
	type workerProcess struct {
		command *exec.Cmd
		output  *bytes.Buffer
	}
	commands := make([]workerProcess, 0, workers)
	for worker := 0; worker < workers; worker++ {
		command := exec.Command(os.Args[0], "-test.run=^TestStoreCrossProcess$", "-test.v")
		output := &bytes.Buffer{}
		command.Stdout = output
		command.Stderr = output
		command.Env = append(os.Environ(),
			"LANDING_COMMS_HELPER="+fmt.Sprint(worker),
			"LANDING_COMMS_PROJECT="+project,
			"LANDING_COMMS_MESSAGES="+fmt.Sprint(messagesPerWorker),
		)
		if err := command.Start(); err != nil {
			t.Fatalf("start cross-process worker %d: %v", worker, err)
		}
		commands = append(commands, workerProcess{command: command, output: output})
	}
	for worker, process := range commands {
		if err := process.command.Wait(); err != nil {
			t.Fatalf("cross-process worker %d returned error: %v\n%s", worker, err, process.output.String())
		}
	}

	history, err := store.History(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("History() after cross-process writers returned unexpected error: %v", err)
	}
	want := workers * messagesPerWorker
	if len(history) != want {
		t.Fatalf("History() after cross-process writers returned %d messages, want %d", len(history), want)
	}
}

func TestStoreCrossProcessRegisterUnique(t *testing.T) {
	if worker := os.Getenv("LANDING_COMMS_UNIQUE_HELPER"); worker != "" {
		runCrossProcessRegisterUniqueWorker(t, worker)
		return
	}

	project := createTestProject(t, "cross-process-register-unique")
	store := openStoreAt(t, project)
	barrier := t.TempDir()

	const workers = 6
	type workerProcess struct {
		command *exec.Cmd
		output  *bytes.Buffer
	}
	processes := make([]workerProcess, 0, workers)
	for worker := 0; worker < workers; worker++ {
		command := exec.Command(os.Args[0], "-test.run=^TestStoreCrossProcessRegisterUnique$", "-test.v")
		output := &bytes.Buffer{}
		command.Stdout = output
		command.Stderr = output
		command.Env = append(os.Environ(),
			"LANDING_COMMS_UNIQUE_HELPER="+fmt.Sprint(worker),
			"LANDING_COMMS_PROJECT="+project,
			"LANDING_COMMS_BARRIER="+barrier,
		)
		if err := command.Start(); err != nil {
			t.Fatalf("start register-unique worker %d: %v", worker, err)
		}
		processes = append(processes, workerProcess{command: command, output: output})
	}
	waitForCrossProcessFiles(t, barrier, "ready-", workers)
	if err := os.WriteFile(filepath.Join(barrier, "release"), nil, 0o600); err != nil {
		t.Fatalf("release register-unique workers: %v", err)
	}
	for worker, process := range processes {
		if err := process.command.Wait(); err != nil {
			t.Fatalf("register-unique worker %d returned error: %v\n%s", worker, err, process.output.String())
		}
	}

	participants, err := store.Participants(context.Background())
	if err != nil {
		t.Fatalf("Participants() after concurrent RegisterUnique() returned unexpected error: %v", err)
	}
	if len(participants) != workers {
		t.Fatalf("Participants() after concurrent RegisterUnique() returned %d participants, want %d", len(participants), workers)
	}
	names := make(map[string]struct{}, len(participants))
	registeredNames := make([]string, 0, len(participants))
	for _, participant := range participants {
		if participant.Kind != KindThread {
			t.Fatalf("RegisterUnique() participant %q kind = %q, want %q", participant.Name, participant.Kind, KindThread)
		}
		if _, exists := names[participant.Name]; exists {
			t.Fatalf("RegisterUnique() retained duplicate name %q", participant.Name)
		}
		names[participant.Name] = struct{}{}
		registeredNames = append(registeredNames, participant.Name)
	}
	t.Logf("concurrent process names = %v", registeredNames)
}

func runCrossProcessWorker(t *testing.T, worker string) {
	project := os.Getenv("LANDING_COMMS_PROJECT")
	messages := 0
	if _, err := fmt.Sscan(os.Getenv("LANDING_COMMS_MESSAGES"), &messages); err != nil {
		t.Fatalf("parse worker message count: %v", err)
	}
	store := openStoreAt(t, project)
	registerTestParticipant(t, store, "writer-"+worker, KindThread)
	for message := 0; message < messages; message++ {
		if _, err := store.Send(context.Background(), Message{
			From: "writer-" + worker,
			To:   "receiver",
			Body: fmt.Sprintf("worker %s message %d", worker, message),
		}); err != nil {
			t.Fatalf("Send(worker %s message %d) returned unexpected error: %v", worker, message, err)
		}
		if _, err := store.Pending(context.Background(), "receiver"); err != nil {
			t.Fatalf("Pending(receiver) from worker %s returned unexpected error: %v", worker, err)
		}
	}
}

func runCrossProcessRegisterUniqueWorker(t *testing.T, worker string) {
	barrier := os.Getenv("LANDING_COMMS_BARRIER")
	if err := os.WriteFile(filepath.Join(barrier, "ready-"+worker), nil, 0o600); err != nil {
		t.Fatalf("signal register-unique worker %s ready: %v", worker, err)
	}
	waitForCrossProcessFiles(t, barrier, "release", 1)

	store := openStoreAt(t, os.Getenv("LANDING_COMMS_PROJECT"))
	name, err := store.RegisterUnique(context.Background(), Participant{
		Name: "reviewer",
		Kind: KindThread,
		Activity: Activity{
			State: StateIdle,
		},
	})
	if err != nil {
		t.Fatalf("RegisterUnique(reviewer) returned unexpected error: %v", err)
	}
	if name == "" {
		t.Fatal("RegisterUnique(reviewer) returned an empty name")
	}
}

func waitForCrossProcessFiles(t *testing.T, directory string, prefix string, want int) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("ReadDir(%q) returned unexpected error: %v", directory, err)
		}
		count := 0
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), prefix) {
				count++
			}
		}
		if count >= want {
			return
		}
		select {
		case <-timer.C:
			t.Fatalf("wait for %d %q files in %q timed out after finding %d", want, prefix, directory, count)
		case <-ticker.C:
		}
	}
}

func openTestStore(t *testing.T) Store {
	t.Helper()
	return openStoreAt(t, createTestProject(t, "project"))
}

func createTestProject(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("LANDING_STATE_DIR", filepath.Join(root, "state"))
	return createProjectAt(t, root, name)
}

func createProjectAt(t *testing.T, root string, name string) string {
	t.Helper()
	project := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(project, ".landing"), 0o700); err != nil {
		t.Fatalf("create project directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, "nested", "working"), 0o700); err != nil {
		t.Fatalf("create project working directory: %v", err)
	}

	return filepath.Join(project, "nested", "working")
}

func openStoreAt(t *testing.T, cwd string) Store {
	t.Helper()
	store, err := Open(context.Background(), cwd)
	if err != nil {
		t.Fatalf("Open(%q) returned unexpected error: %v", cwd, err)
	}

	return store
}

func registerTestParticipant(t *testing.T, store Store, name string, kind Kind) {
	t.Helper()
	if err := store.Register(context.Background(), Participant{
		Name: name,
		Kind: kind,
		Activity: Activity{
			State: StateIdle,
		},
	}); err != nil {
		t.Fatalf("Register(%q) returned unexpected error: %v", name, err)
	}
}
