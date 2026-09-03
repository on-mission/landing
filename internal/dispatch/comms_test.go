package dispatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/on-mission/landing/internal/comms"
	"github.com/on-mission/landing/internal/config"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
	"github.com/on-mission/landing/internal/router"
)

func TestDispatchRegistersThreadParticipantForItsLifecycle(t *testing.T) {
	project := testProject(t)
	t.Setenv("LANDING_STATE_DIR", t.TempDir())
	t.Setenv("LANDING_DISPATCH_COMMS_HELPER", "1")
	storeForComms, err := comms.Open(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &commsLifecycleAdapter{mode: "pause", started: make(chan struct{})}
	engine, store := newCommsTestEngine(t, adapter)
	t.Cleanup(func() { store.Shutdown(context.Background()) })
	label := "reviewer"
	result := make(chan error, 1)
	go func() {
		_, err := engine.Dispatch(context.Background(), Request{
			Tier:   "engineer",
			Prompt: "inspect the change",
			CWD:    project,
			Label:  &label,
			Route:  &config.Route{Harness: "fake"},
		})
		result <- err
	}()
	select {
	case <-adapter.started:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	runningParticipant := participant(t, project, label)
	if runningParticipant.Kind != comms.KindThread || runningParticipant.Harness != "fake" || runningParticipant.CWD != project {
		t.Fatalf("thread participant = %#v, want thread for fake in %q", runningParticipant, project)
	}
	if runningParticipant.Activity.State != comms.StateInTurn {
		t.Fatalf("thread participant activity while running = %q, want %q", runningParticipant.Activity.State, comms.StateInTurn)
	}
	if runningParticipant.PID != 0 {
		t.Fatalf("thread participant PID = %d, want 0", runningParticipant.PID)
	}
	if adapter.processID == 0 {
		t.Fatal("dispatch helper process ID is empty")
	}
	if adapter.provenance != label {
		t.Fatalf("dispatch helper %s = %q, want %q", jobs.CommsParticipantEnvironment, adapter.provenance, label)
	}
	if err := storeForComms.Register(context.Background(), comms.Participant{
		Name:      "fake-session",
		Kind:      comms.KindSession,
		Harness:   "fake",
		PID:       adapter.processID,
		SessionID: "session-123",
		Activity:  comms.Activity{State: comms.StateIdle, Since: time.Now().UTC()},
	}); err != nil {
		t.Fatalf("Register(hook session) returned unexpected error: %v", err)
	}
	participants, err := storeForComms.Participants(context.Background())
	if err != nil {
		t.Fatalf("Participants() after hook registration returned unexpected error: %v", err)
	}
	if len(participants) != 1 || participants[0].Name != label || participants[0].Kind != comms.KindThread {
		t.Fatalf("Participants() after hook registration = %#v, want only thread %q", participants, label)
	}
	message, err := storeForComms.Send(context.Background(), comms.Message{From: "sender", To: label, Body: "review this"})
	if err != nil {
		t.Fatalf("Send(thread) returned unexpected error: %v", err)
	}
	pending, err := storeForComms.Pending(context.Background(), "fake-session")
	if err != nil {
		t.Fatalf("Pending(hook session) returned unexpected error: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != message.ID {
		t.Fatalf("Pending(hook session) = %#v, want message %q", pending, message.ID)
	}
	if err := storeForComms.MarkDelivered(context.Background(), "fake-session", []string{message.ID}); err != nil {
		t.Fatalf("MarkDelivered(hook session) returned unexpected error: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("Dispatch() returned unexpected error: %v", err)
	}
	finishedParticipant := participant(t, project, label)
	if finishedParticipant.ThreadID != "thread-123" {
		t.Fatalf("thread participant id = %q, want thread-123", finishedParticipant.ThreadID)
	}
	if finishedParticipant.Activity.State != comms.StateIdle {
		t.Fatalf("thread participant activity after completion = %q, want %q", finishedParticipant.Activity.State, comms.StateIdle)
	}
}

func TestDispatchCompletesWhenCommsStoreIsUnusable(t *testing.T) {
	project := testProject(t)
	stateFile := filepath.Join(t.TempDir(), "state-file")
	if err := os.WriteFile(stateFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LANDING_STATE_DIR", stateFile)
	t.Setenv("LANDING_DISPATCH_COMMS_HELPER", "1")
	adapter := &commsLifecycleAdapter{mode: "exit", started: make(chan struct{})}
	engine, store := newCommsTestEngine(t, adapter)
	t.Cleanup(func() { store.Shutdown(context.Background()) })

	response, err := engine.Dispatch(context.Background(), Request{
		Tier:   "engineer",
		Prompt: "complete without comms",
		CWD:    project,
		Route:  &config.Route{Harness: "fake"},
	})
	if err != nil {
		t.Fatalf("Dispatch() returned error with unusable comms store: %v", err)
	}
	if response.Status != harness.JobStatusDone {
		t.Fatalf("Dispatch() status with unusable comms store = %q, want %q", response.Status, harness.JobStatusDone)
	}
}

func TestDispatchResolvesThreadNameCollisions(t *testing.T) {
	project := testProject(t)
	t.Setenv("LANDING_STATE_DIR", t.TempDir())
	t.Setenv("LANDING_DISPATCH_COMMS_HELPER", "1")
	storeForComms, err := comms.Open(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if err := storeForComms.Register(context.Background(), comms.Participant{
		Name:     "reviewer",
		Kind:     comms.KindSession,
		Activity: comms.Activity{State: comms.StateIdle, Since: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	adapter := &commsLifecycleAdapter{mode: "exit", started: make(chan struct{})}
	engine, store := newCommsTestEngine(t, adapter)
	t.Cleanup(func() { store.Shutdown(context.Background()) })
	label := "reviewer"
	if _, err := engine.Dispatch(context.Background(), Request{
		Tier:   "engineer",
		Prompt: "complete with a name collision",
		CWD:    project,
		Label:  &label,
		Route:  &config.Route{Harness: "fake"},
	}); err != nil {
		t.Fatalf("Dispatch() returned unexpected error: %v", err)
	}
	participants, err := storeForComms.Participants(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if existing := participantNamed(participants, "reviewer"); existing.Kind != comms.KindSession {
		t.Fatalf("existing participant after Dispatch() = %#v, want original session", existing)
	}
	if resolved := participantNamed(participants, "reviewer-2"); resolved.Kind != comms.KindThread {
		t.Fatalf("collision-resolved participant after Dispatch() = %#v, want thread reviewer-2", resolved)
	}
}

func testProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".landing"), 0o700); err != nil {
		t.Fatal(err)
	}
	return project
}

func newCommsTestEngine(t *testing.T, adapter *commsLifecycleAdapter) (*Engine, *jobs.Store) {
	t.Helper()
	store := jobs.NewStore(context.Background())
	registry := router.NewMapRegistry(map[string]harness.Adapter{"fake": adapter})
	return New(router.New(registry), store, config.Config{Tiers: map[string]config.Tier{"engineer": {Name: "engineer"}}}), store
}

func participant(t *testing.T, project string, name string) comms.Participant {
	t.Helper()
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		store, err := comms.Open(context.Background(), project)
		if err != nil {
			t.Fatal(err)
		}
		participants, err := store.Participants(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		participant := participantNamed(participants, name)
		if participant.Name != "" {
			return participant
		}
		select {
		case <-timeout.C:
			t.Fatalf("participant %q was not registered", name)
		case <-ticker.C:
		}
	}
}

func participantNamed(participants []comms.Participant, name string) comms.Participant {
	for _, participant := range participants {
		if participant.Name == name {
			return participant
		}
	}
	return comms.Participant{}
}

type commsLifecycleAdapter struct {
	mode        string
	started     chan struct{}
	startedOnce sync.Once
	processID   int
	provenance  string
}

func (adapter *commsLifecycleAdapter) ID() string {
	return "fake"
}

func (adapter *commsLifecycleAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{Authority: harness.ModelCatalogAuthoritative}
}

func (adapter *commsLifecycleAdapter) Detect(context.Context) harness.Detection {
	return harness.Detection{Status: harness.DetectionAbsent, Capacity: harness.UnknownCapacity()}
}

func (adapter *commsLifecycleAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{Continuation: true}
}

func (adapter *commsLifecycleAdapter) Validate(harness.StartParams) error {
	return nil
}

func (adapter *commsLifecycleAdapter) BuildStart(harness.StartParams) (harness.Request, error) {
	return harness.Request{Command: os.Args[0], Args: []string{"-test.run=^TestDispatchCommsHelper$", "--", adapter.mode}}, nil
}

func (adapter *commsLifecycleAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return adapter.BuildStart(harness.StartParams{})
}

func (adapter *commsLifecycleAdapter) OnStdoutLine(line string, record *harness.JobRecord) {
	if provenance, ok := strings.CutPrefix(line, "provenance="); ok {
		adapter.provenance = provenance
	}
	if threadID, ok := strings.CutPrefix(line, "thread="); ok {
		record.ThreadID = &threadID
		adapter.startedOnce.Do(func() {
			if record.Proc != nil && record.Proc.Process != nil {
				adapter.processID = record.Proc.Process.Pid
			}
			close(adapter.started)
		})
	}
}

func (adapter *commsLifecycleAdapter) Finalize(context.Context, harness.FinalizeParams) (harness.Finalized, error) {
	return harness.Finalized{Status: harness.JobStatusDone}, nil
}

func (adapter *commsLifecycleAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return harness.UnknownCapacity()
}

func (adapter *commsLifecycleAdapter) SpawnPath() string {
	return ""
}

func TestDispatchCommsHelper(t *testing.T) {
	if os.Getenv("LANDING_DISPATCH_COMMS_HELPER") != "1" {
		return
	}
	if _, err := fmt.Fprintf(os.Stdout, "provenance=%s\n", os.Getenv(jobs.CommsParticipantEnvironment)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(os.Stdout, "thread=thread-123"); err != nil {
		t.Fatal(err)
	}
	if os.Args[len(os.Args)-1] == "pause" {
		time.Sleep(200 * time.Millisecond)
	}
}
