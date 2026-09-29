package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/journal"
)

func TestWaitCompletionHasDurableJournalRecord(t *testing.T) {
	store, adapter := newLifecycleStore(t, "exit")
	record := startLifecycleJob(t, store, adapter)

	completed, err := store.Wait(context.Background(), record.JobID)
	if err != nil {
		t.Fatal(err)
	}
	journaled, err := journal.ReadJob(context.Background(), record.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if journaled == nil {
		t.Fatal("completion was observable before its terminal journal record was readable")
	}
	if journaled.Status != completed.Status || journaled.ThreadID == nil || *journaled.ThreadID != "thread-from-stdout" {
		t.Fatalf("journal record = %#v; want completed status %q and stdout thread", journaled, completed.Status)
	}
}

func TestRunningJobHasNoJournalRecord(t *testing.T) {
	store, adapter := newLifecycleStore(t, "block")
	record := startLifecycleJob(t, store, adapter)
	waitForHelperStart(t, adapter)

	journaled, err := journal.ReadJob(context.Background(), record.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if journaled != nil {
		t.Fatalf("running job journal record = %#v; want no durable record", journaled)
	}

	cancelled, err := store.Cancel(context.Background(), record.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != harness.JobStatusCancelled {
		t.Fatalf("Cancel() status = %q; want cancelled", cancelled.Status)
	}
	if _, err := store.Wait(context.Background(), record.JobID); err != nil {
		t.Fatal(err)
	}
}

func TestAbandonTimedOutPersistsTerminalTimeout(t *testing.T) {
	store, adapter := newLifecycleStore(t, "block")
	record := startLifecycleJob(t, store, adapter)
	waitForHelperStart(t, adapter)

	deadline, cancel := context.WithDeadline(context.Background(), time.Now())
	defer cancel()
	if _, err := store.Wait(deadline, record.JobID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() error = %v; want deadline exceeded", err)
	}
	timedOut, _, err := store.AbandonTimedOut(context.Background(), record.JobID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if timedOut.Status != harness.JobStatusTimeout {
		t.Fatalf("AbandonTimedOut() status = %q; want timeout", timedOut.Status)
	}
	journaled, err := journal.ReadJob(context.Background(), record.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if journaled == nil || journaled.Status != harness.JobStatusTimeout {
		t.Fatalf("timeout journal record = %#v; want terminal timeout", journaled)
	}

	terminateHelper(t, store, timedOut)
}

func TestAbandonTimedOutDoesNotLeakSubprocess(t *testing.T) {
	store, adapter := newLifecycleStore(t, "block")
	record := startLifecycleJob(t, store, adapter)
	waitForHelperStart(t, adapter)

	timedOut, _, err := store.AbandonTimedOut(context.Background(), record.JobID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if timedOut.Proc == nil || timedOut.Proc.Process == nil {
		t.Fatal("timed-out job has no subprocess to verify")
	}
	// Abandonment signals the harness; it does not wait for it to die, because
	// waiting would reinstate the very block the timeout exists to escape. So
	// the assertion is that the process reaches a terminated state, not that it
	// has already reached one by the time the call returns. On Unix, a
	// terminated-but-unreaped child still reports alive, so this polls until
	// the store's own goroutine has reaped it.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if !processAlive(timedOut.Proc.Process) {
			return
		}
		if time.Now().After(deadline) {
			terminateHelper(t, store, timedOut)
			t.Fatal("timed-out job left its subprocess running")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCancelIsRecordedAsCancelledRatherThanAdapterFailure(t *testing.T) {
	store, adapter := newLifecycleStore(t, "block")
	adapter.finalStatus = harness.JobStatusFailed
	record := startLifecycleJob(t, store, adapter)
	waitForHelperStart(t, adapter)

	cancelled, err := store.Cancel(context.Background(), record.JobID)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Wait(context.Background(), record.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != harness.JobStatusCancelled || completed.Status != harness.JobStatusCancelled {
		t.Fatalf("cancellation statuses = %q, %q; want cancelled", cancelled.Status, completed.Status)
	}
	if adapter.finalizeCalls.Load() != 0 {
		t.Fatalf("Finalize() calls = %d; want cancellation to remain distinct from adapter failure", adapter.finalizeCalls.Load())
	}
	journaled, err := journal.ReadJob(context.Background(), record.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if journaled == nil || journaled.Status != harness.JobStatusCancelled {
		t.Fatalf("cancellation journal record = %#v; want cancelled", journaled)
	}
}

func TestStdoutThreadCaptureIsPersistedWithoutInventingMissingIDs(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantThread *string
	}{
		{name: "captures adapter stdout thread", mode: "exit", wantThread: stringPointer("thread-from-stdout")},
		{name: "preserves absent thread", mode: "exit-without-thread"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, adapter := newLifecycleStore(t, test.mode)
			record := startLifecycleJob(t, store, adapter)
			completed, err := store.Wait(context.Background(), record.JobID)
			if err != nil {
				t.Fatal(err)
			}
			journaled, err := journal.ReadJob(context.Background(), record.JobID)
			if err != nil {
				t.Fatal(err)
			}
			if !equalStrings(completed.ThreadID, test.wantThread) || journaled == nil || !equalStrings(journaled.ThreadID, test.wantThread) {
				t.Fatalf("thread ids = completed %#v, journal %#v; want %#v", completed.ThreadID, journaled, test.wantThread)
			}
		})
	}
}

func TestStoreCompletesConcurrentJobsSafely(t *testing.T) {
	store, adapter := newLifecycleStore(t, "exit-without-thread")
	const jobs = 16
	results := make(chan *harness.JobRecord, jobs)
	errs := make(chan error, jobs)
	var workers sync.WaitGroup
	for range jobs {
		workers.Add(1)
		go func() {
			defer workers.Done()
			record, err := store.Start(context.Background(), adapter, StartOptions{CWD: t.TempDir()})
			if err != nil {
				errs <- err
				return
			}
			completed, err := store.Wait(context.Background(), record.JobID)
			if err != nil {
				errs <- err
				return
			}
			results <- completed
		}()
	}
	workers.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := make(map[string]struct{}, jobs)
	for record := range results {
		if record.Status != harness.JobStatusDone {
			t.Fatalf("concurrent job status = %q; want done", record.Status)
		}
		if _, exists := seen[record.JobID]; exists {
			t.Fatalf("duplicate concurrent job id %q", record.JobID)
		}
		seen[record.JobID] = struct{}{}
		journaled, err := journal.ReadJob(context.Background(), record.JobID)
		if err != nil || journaled == nil || journaled.Status != harness.JobStatusDone {
			t.Fatalf("concurrent journal record = %#v, %v; want done record", journaled, err)
		}
	}
	if len(seen) != jobs {
		t.Fatalf("completed concurrent jobs = %d; want %d", len(seen), jobs)
	}
}

func TestStartPassesCommsParticipantProvenanceToChild(t *testing.T) {
	store, adapter := newLifecycleStore(t, "environment")
	t.Setenv(CommsParticipantEnvironment, "ambient-thread")
	record, err := store.Start(context.Background(), adapter, StartOptions{
		CWD:         t.TempDir(),
		Environment: []string{CommsParticipantEnvironmentEntry("dispatched-thread")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Wait(context.Background(), record.JobID); err != nil {
		t.Fatal(err)
	}
	if adapter.provenance != "dispatched-thread" {
		t.Fatalf("child %s = %q, want %q", CommsParticipantEnvironment, adapter.provenance, "dispatched-thread")
	}
}

type lifecycleAdapter struct {
	mode          string
	started       chan struct{}
	startedOnce   sync.Once
	finalStatus   harness.JobStatus
	finalizeCalls atomic.Int32
	provenance    string
}

func (adapter *lifecycleAdapter) ID() string {
	return "fake"
}

func (adapter *lifecycleAdapter) Capabilities() harness.Capabilities {
	return harness.Capabilities{}
}

// The lifecycle tests drive a local helper process, never a real harness, so
// this fake reports no catalog and no installation rather than pretending to
// one it does not have.
func (adapter *lifecycleAdapter) ModelCatalog() harness.ModelCatalog {
	return harness.ModelCatalog{Authority: harness.ModelCatalogAuthoritative}
}

func (adapter *lifecycleAdapter) ValidateModel(context.Context, string) harness.ModelValidation {
	return harness.ValidModel("test validation")
}

func (adapter *lifecycleAdapter) Detect(context.Context) harness.Detection {
	return harness.Detection{Status: harness.DetectionAbsent, Capacity: harness.UnknownCapacity()}
}

func (adapter *lifecycleAdapter) Validate(harness.StartParams) error {
	return nil
}

func (adapter *lifecycleAdapter) BuildStart(harness.StartParams) (harness.Request, error) {
	return harness.Request{Command: os.Args[0], Args: []string{"-test.run=^TestJobsProcess$", "--", adapter.mode}}, nil
}

func (adapter *lifecycleAdapter) BuildResume(harness.ResumeParams) (harness.Request, error) {
	return adapter.BuildStart(harness.StartParams{})
}

func (adapter *lifecycleAdapter) OnStdoutLine(line string, record *harness.JobRecord) {
	if line == "ready" {
		adapter.startedOnce.Do(func() { close(adapter.started) })
	}
	if threadID, ok := strings.CutPrefix(line, "thread="); ok {
		record.ThreadID = stringPointer(threadID)
	}
	if provenance, ok := strings.CutPrefix(line, "provenance="); ok {
		adapter.provenance = provenance
	}
}

func (adapter *lifecycleAdapter) Finalize(context.Context, harness.FinalizeParams) (harness.Finalized, error) {
	adapter.finalizeCalls.Add(1)
	return harness.Finalized{Status: adapter.finalStatus}, nil
}

func (adapter *lifecycleAdapter) ProbeCapacity(context.Context) harness.Capacity {
	return harness.UnknownCapacity()
}

func (adapter *lifecycleAdapter) SpawnPath() string {
	return ""
}

func newLifecycleStore(t *testing.T, mode string) (*Store, *lifecycleAdapter) {
	t.Helper()
	t.Setenv("LANDING_STATE_DIR", t.TempDir())
	t.Setenv("LANDING_JOBS_HELPER", "1")
	return NewStore(context.Background()), &lifecycleAdapter{mode: mode, started: make(chan struct{}), finalStatus: harness.JobStatusDone}
}

func startLifecycleJob(t *testing.T, store *Store, adapter *lifecycleAdapter) *harness.JobRecord {
	t.Helper()
	record, err := store.Start(context.Background(), adapter, StartOptions{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func waitForHelperStart(t *testing.T, adapter *lifecycleAdapter) {
	t.Helper()
	select {
	case <-adapter.started:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
}

func terminateHelper(t *testing.T, store *Store, record *harness.JobRecord) {
	t.Helper()
	if record.Proc == nil || record.Proc.Process == nil {
		t.Fatal("job has no subprocess to terminate")
	}
	if err := terminateProcess(record.Proc.Process); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatal(err)
	}
	if _, err := store.Wait(context.Background(), record.JobID); err != nil {
		t.Fatal(err)
	}
}

func equalStrings(got *string, want *string) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func TestJobsProcess(t *testing.T) {
	if os.Getenv("LANDING_JOBS_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "exit":
		_, _ = fmt.Fprintln(os.Stdout, "thread=thread-from-stdout")
	case "exit-without-thread":
		_, _ = fmt.Fprintln(os.Stdout, "complete")
	case "block":
		_, _ = fmt.Fprintln(os.Stdout, "ready")
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM)
		defer signal.Stop(signals)
		<-signals
	case "environment":
		_, _ = fmt.Fprintf(os.Stdout, "provenance=%s\n", os.Getenv(CommsParticipantEnvironment))
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
}

func TestProjectCarriesPersonaDelivery(t *testing.T) {
	delivery := harness.PersonaDeliveryAppendSystemPrompt
	projected := Project(harness.JobRecord{PersonaDelivery: &delivery})
	if projected.PersonaDelivery == nil || *projected.PersonaDelivery != delivery {
		t.Fatalf("Project() persona delivery = %v, want %q", projected.PersonaDelivery, delivery)
	}
}
