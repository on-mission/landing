// Package jobs owns child-process execution and completed-job retention.
package jobs

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/journal"
)

const jobTTL = time.Hour

const (
	CommsParticipantEnvironment = "LANDING_COMMS_PARTICIPANT"
	CommsProbeEnvironment       = "LANDING_COMMS_PROBE"
	commsProbeValue             = "1"
)

func CommsParticipantEnvironmentEntry(name string) string {
	return CommsParticipantEnvironment + "=" + name
}

func CommsProbeEnvironmentEntry() string {
	return CommsProbeEnvironment + "=" + commsProbeValue
}

func WithCommsProbeEnvironment(environment []string) []string {
	updated := append([]string(nil), environment...)
	return replaceEnvironmentEntry(updated, CommsProbeEnvironmentEntry())
}

func IsCommsProbeEnvironment() bool {
	return os.Getenv(CommsProbeEnvironment) == commsProbeValue
}

type StartOptions struct {
	Prompt          string
	Persona         *harness.Persona
	Model           *string
	ReasoningEffort *string
	Sandbox         *string
	CWD             string
	Label           *string
	ExtraConfig     []string
	Provider        *string
	Timeout         *time.Duration
	Role            *string
	RoutedBecause   *string
	Capacity        harness.Capacity
	RoutingScore    *float64
	RerouteCount    int
	ReroutedFrom    *string
	ThreadID        *string
	ConversationID  string
	Environment     []string
	BeforeSpawn     func(harness.JobRecord) []string
}

type ResumeOptions struct {
	ThreadID        string
	ConversationID  string
	Prompt          string
	Persona         *harness.Persona
	Model           *string
	ReasoningEffort *string
	Sandbox         *string
	CWD             string
	Label           *string
	ExtraConfig     []string
	Provider        *string
	Timeout         *time.Duration
	Role            *string
	Environment     []string
	BeforeSpawn     func(harness.JobRecord) []string
}

type Projection struct {
	JobID      string
	Role       *string
	Provider   string
	Model      *string
	Label      *string
	Status     harness.JobStatus
	StartedAt  time.Time
	FinishedAt *time.Time
	Duration   time.Duration
	ExitCode   *int
	Output     *string
	Error      *string
	// RoutedBecause explains why this route was selected. It travels with the
	// projection so a caller can report it without reading the journal back
	// off disk to recover a value it already produced.
	RoutedBecause   *string
	PersonaDelivery *harness.PersonaDelivery
}

type Store struct {
	mu   sync.RWMutex
	jobs map[string]*job
}

type job struct {
	mu sync.RWMutex

	record         harness.JobRecord
	conversationID string
	done           chan struct{}
	journaled      bool
	timedOut       bool
	completed      bool
}

func NewStore(_ context.Context) *Store {
	return &Store{jobs: make(map[string]*job)}
}

func (store *Store) Start(ctx context.Context, adapter harness.Adapter, options StartOptions) (*harness.JobRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	params := harness.StartParams{
		JobID:           newJobID(),
		Prompt:          options.Prompt,
		Persona:         clonePersona(options.Persona),
		Model:           cloneString(options.Model),
		ReasoningEffort: cloneString(options.ReasoningEffort),
		Sandbox:         cloneString(options.Sandbox),
		CWD:             options.CWD,
		ExtraConfig:     append([]string(nil), options.ExtraConfig...),
		Provider:        cloneString(options.Provider),
		Timeout:         cloneDuration(options.Timeout),
	}
	if err := adapter.Validate(params); err != nil {
		return nil, err
	}
	request, err := adapter.BuildStart(params)
	if err != nil {
		return nil, fmt.Errorf("build %s start request: %w", adapter.ID(), err)
	}

	return store.spawn(ctx, adapter, params.JobID, params.Model, params.CWD, request, options)
}

func (store *Store) Resume(ctx context.Context, adapter harness.Adapter, options ResumeOptions) (*harness.JobRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.ThreadID == "" {
		return nil, harness.NewError(harness.ErrorCodeThreadNotFound, "thread id is required.", nil)
	}
	conversationID := options.ConversationID
	if conversationID == "" {
		conversationID = conversationIDForThread(adapter.ID(), options.ThreadID)
	}
	params := harness.ResumeParams{
		JobID:           newJobID(),
		ThreadID:        options.ThreadID,
		Prompt:          options.Prompt,
		Persona:         clonePersona(options.Persona),
		Model:           cloneString(options.Model),
		ReasoningEffort: cloneString(options.ReasoningEffort),
		Sandbox:         cloneString(options.Sandbox),
		CWD:             options.CWD,
		ExtraConfig:     append([]string(nil), options.ExtraConfig...),
		Provider:        cloneString(options.Provider),
		Timeout:         cloneDuration(options.Timeout),
	}
	if err := adapter.Validate(harness.StartParams{
		JobID:           params.JobID,
		Prompt:          params.Prompt,
		Persona:         clonePersona(params.Persona),
		Model:           params.Model,
		ReasoningEffort: params.ReasoningEffort,
		Sandbox:         params.Sandbox,
		CWD:             params.CWD,
		ExtraConfig:     params.ExtraConfig,
		Provider:        params.Provider,
		Timeout:         params.Timeout,
	}); err != nil {
		return nil, err
	}
	request, err := adapter.BuildResume(params)
	if err != nil {
		return nil, fmt.Errorf("build %s continuation request: %w", adapter.ID(), err)
	}

	return store.spawn(ctx, adapter, params.JobID, params.Model, params.CWD, request, StartOptions{
		Model: options.Model, CWD: options.CWD, Label: options.Label, Role: options.Role, ThreadID: stringPointer(options.ThreadID), ConversationID: conversationID, Persona: options.Persona, Environment: options.Environment, BeforeSpawn: options.BeforeSpawn,
	})
}

func (store *Store) Get(jobID string) (*harness.JobRecord, bool) {
	job, ok := store.lookup(jobID)
	if !ok {
		return nil, false
	}

	return job.snapshot(), true
}

func (store *Store) Wait(ctx context.Context, jobID string) (*harness.JobRecord, error) {
	job, ok := store.lookup(jobID)
	if !ok {
		return nil, harness.NewError(harness.ErrorCodeJobLost, "Job not found after start.", nil)
	}
	select {
	case <-job.done:
		return job.snapshot(), nil
	case <-ctx.Done():
		return job.snapshot(), ctx.Err()
	}
}

func (store *Store) Adopt(ctx context.Context, record journal.Record, cwd string) (*harness.JobRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if record.JobID == "" {
		return nil, nil
	}
	if existing, ok := store.Get(record.JobID); ok {
		return existing, nil
	}
	adoptedCWD := cwd
	if adoptedCWD == "" && record.CWD != nil {
		adoptedCWD = *record.CWD
	}
	startedAt := time.Time{}
	if record.StartedAt != nil {
		startedAt = *record.StartedAt
	}
	recordCopy := harness.JobRecord{
		JobID:      record.JobID,
		Provider:   stringValue(record.Provider),
		Model:      cloneString(record.Model),
		Role:       cloneString(record.Role),
		Label:      cloneString(record.Label),
		Status:     record.Status,
		StartedAt:  startedAt,
		FinishedAt: cloneTime(record.FinishedAt),
		ExitCode:   cloneInt(record.ExitCode),
		CWD:        adoptedCWD,
		ThreadID:   cloneString(record.ThreadID),
		Args:       []string{},
		Capacity:   harness.UnknownCapacity(),
	}
	state := &job{record: recordCopy, conversationID: record.JobID, done: make(chan struct{}), completed: true, journaled: true}
	close(state.done)
	store.mu.Lock()
	if existing, ok := store.jobs[record.JobID]; ok {
		store.mu.Unlock()
		return existing.snapshot(), nil
	}
	store.jobs[record.JobID] = state
	store.mu.Unlock()

	return state.snapshot(), nil
}

func (store *Store) Cancel(ctx context.Context, jobID string) (*harness.JobRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	job, ok := store.lookup(jobID)
	if !ok {
		return nil, nil
	}
	job.mu.Lock()
	if job.record.Status != harness.JobStatusRunning {
		record := cloneRecord(job.record)
		job.mu.Unlock()
		return &record, nil
	}
	now := time.Now()
	job.record.Status = harness.JobStatusCancelled
	job.record.FinishedAt = &now
	record := cloneRecord(job.record)
	conversationID := job.conversationID
	alreadyJournaled := job.journaled
	job.journaled = true
	command := job.record.Proc
	job.mu.Unlock()
	if !alreadyJournaled {
		journal.RecordConversation(context.Background(), conversationID, record)
	}
	if command != nil && command.Process != nil {
		if err := command.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return &record, fmt.Errorf("terminate job %q: %w", jobID, err)
		}
	}

	return &record, nil
}

func (store *Store) AbandonTimedOut(ctx context.Context, jobID string, timeout time.Duration) (*harness.JobRecord, string, error) {
	if err := ctx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, "", err
	}
	job, ok := store.lookup(jobID)
	if !ok {
		return nil, "", harness.NewError(harness.ErrorCodeJobLost, "Job not found after start.", nil)
	}
	job.mu.Lock()
	if job.record.Status == harness.JobStatusRunning {
		now := time.Now()
		job.record.Status = harness.JobStatusTimeout
		job.record.FinishedAt = &now
		job.timedOut = true
	}
	record := cloneRecord(job.record)
	conversationID := job.conversationID
	journalled := job.journaled
	job.journaled = true
	command := job.record.Proc
	job.mu.Unlock()
	if !journalled {
		journal.RecordConversation(context.Background(), conversationID, record)
	}
	// Abandonment terminates the harness. Landing exists to spend model capacity
	// deliberately, and a subprocess nobody is waiting on keeps consuming it to
	// produce a result no caller will ever read. The thread id is captured from
	// the harness before this point, so continuation survives the termination.
	if command != nil && command.Process != nil {
		if err := command.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return &record, "", fmt.Errorf("terminate abandoned job %q: %w", jobID, err)
		}
	}
	message := fmt.Sprintf("gave up waiting after %dms; the job was still running and its harness was terminated. A thread it opened before termination remains continuable with 'landing reply --thread %s'.", timeout.Milliseconds(), record.JobID)

	return &record, message, nil
}

func (store *Store) UpdateRerouting(ctx context.Context, jobID string, routedBecause string, rerouteCount int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	job, ok := store.lookup(jobID)
	if !ok {
		return harness.NewError(harness.ErrorCodeJobLost, "Job not found after start.", nil)
	}
	job.mu.Lock()
	job.record.RoutedBecause = stringPointer(routedBecause)
	job.record.RerouteCount = rerouteCount
	record := cloneRecord(job.record)
	conversationID := job.conversationID
	job.mu.Unlock()
	journal.RecordConversation(context.Background(), conversationID, record)

	return nil
}

func (store *Store) LinkReroute(ctx context.Context, jobID string, replacementID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	job, ok := store.lookup(jobID)
	if !ok {
		return harness.NewError(harness.ErrorCodeJobLost, "Job not found after start.", nil)
	}
	job.mu.Lock()
	job.record.ReroutedTo = stringPointer(replacementID)
	record := cloneRecord(job.record)
	conversationID := job.conversationID
	job.mu.Unlock()
	journal.RecordConversation(context.Background(), conversationID, record)

	return nil
}

func (store *Store) Purge(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	now := time.Now()
	store.mu.Lock()
	for jobID, job := range store.jobs {
		record := job.snapshot()
		if record.Status.IsTerminal() && record.FinishedAt != nil && now.Sub(*record.FinishedAt) > jobTTL {
			delete(store.jobs, jobID)
		}
	}
	store.mu.Unlock()
}

func (store *Store) Shutdown(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	store.mu.RLock()
	jobs := make([]*job, 0, len(store.jobs))
	for _, job := range store.jobs {
		jobs = append(jobs, job)
	}
	store.mu.RUnlock()
	for _, job := range jobs {
		job.mu.RLock()
		command := job.record.Proc
		completed := job.completed
		job.mu.RUnlock()
		if completed || command == nil || command.Process == nil {
			continue
		}
		if err := command.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			continue
		}
	}
}

func Project(record harness.JobRecord) Projection {
	finishedAt := cloneTime(record.FinishedAt)
	duration := time.Since(record.StartedAt)
	if finishedAt != nil {
		duration = finishedAt.Sub(record.StartedAt)
	}
	var projectedError *string
	if record.Status == harness.JobStatusCancelled {
		projectedError = stringPointer("cancelled on request")
	}
	if record.Status == harness.JobStatusFailed {
		errorMessage := record.Stderr
		if errorMessage == "" {
			if record.ExitCode == nil {
				errorMessage = "exit code unknown"
			} else {
				errorMessage = fmt.Sprintf("exit code %d", *record.ExitCode)
			}
		}
		projectedError = stringPointer(errorMessage)
	}

	return Projection{
		JobID:           record.JobID,
		Role:            cloneString(record.Role),
		Provider:        record.Provider,
		Model:           cloneString(record.Model),
		Label:           cloneString(record.Label),
		Status:          record.Status,
		StartedAt:       record.StartedAt,
		FinishedAt:      finishedAt,
		Duration:        duration,
		ExitCode:        cloneInt(record.ExitCode),
		Output:          cloneString(record.Output),
		Error:           projectedError,
		RoutedBecause:   cloneString(record.RoutedBecause),
		PersonaDelivery: clonePersonaDelivery(record.PersonaDelivery),
	}
}

func (store *Store) spawn(ctx context.Context, adapter harness.Adapter, jobID string, model *string, cwd string, request harness.Request, options StartOptions) (*harness.JobRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	record := harness.JobRecord{
		JobID:           jobID,
		Provider:        adapter.ID(),
		Model:           cloneString(model),
		Role:            cloneString(options.Role),
		Label:           cloneString(options.Label),
		Status:          harness.JobStatusRunning,
		StartedAt:       time.Now(),
		Args:            append([]string(nil), request.Args...),
		CWD:             cwd,
		RoutedBecause:   cloneString(options.RoutedBecause),
		PersonaDelivery: clonePersonaDelivery(request.PersonaDelivery),
		Capacity:        options.Capacity,
		RoutingScore:    cloneFloat(options.RoutingScore),
		RerouteCount:    options.RerouteCount,
		ReroutedFrom:    cloneString(options.ReroutedFrom),
		ThreadID:        cloneString(options.ThreadID),
	}
	conversationID := options.ConversationID
	if conversationID == "" {
		conversationID = jobID
	}
	state := &job{record: record, conversationID: conversationID, done: make(chan struct{})}
	store.mu.Lock()
	store.jobs[record.JobID] = state
	store.mu.Unlock()

	environment := append([]string(nil), options.Environment...)
	if options.BeforeSpawn != nil {
		environment = append(environment, options.BeforeSpawn(record)...)
	}
	command := exec.Command(request.Command, request.Args...)
	command.Dir = record.CWD
	command.Env = commandEnvironment(adapter.SpawnPath(), environment)
	stdout, err := command.StdoutPipe()
	if err != nil {
		store.finish(state, adapter, -1, nil, "", fmt.Sprintf("spawn error: %v", err))
		return state.snapshot(), nil
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		store.finish(state, adapter, -1, nil, "", fmt.Sprintf("spawn error: %v", err))
		return state.snapshot(), nil
	}
	state.mu.Lock()
	state.record.Proc = command
	state.mu.Unlock()
	if err := command.Start(); err != nil {
		detail := fmt.Sprintf("spawn error: %v", err)
		if errors.Is(err, exec.ErrNotFound) {
			detail = fmt.Sprintf("spawn error: %q not found", request.Command)
			if adapter.SpawnPath() != "" {
				detail += fmt.Sprintf(" on PATH. Searched: %s", adapter.SpawnPath())
			}
		}
		store.finish(state, adapter, -1, nil, "", detail)
		return state.snapshot(), nil
	}

	go store.awaitProcess(state, adapter, command, stdout, stderr)
	return state.snapshot(), nil
}

func (store *Store) awaitProcess(state *job, adapter harness.Adapter, command *exec.Cmd, stdout io.ReadCloser, stderr io.ReadCloser) {
	var output strings.Builder
	var diagnostics strings.Builder
	var outputMu sync.Mutex
	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		readErr := readStdout(stdout, func(chunk string) {
			outputMu.Lock()
			output.WriteString(chunk)
			outputMu.Unlock()
		}, func(line string) {
			state.mu.Lock()
			adapter.OnStdoutLine(line, &state.record)
			state.mu.Unlock()
		})
		if readErr != nil {
			outputMu.Lock()
			diagnostics.WriteString(fmt.Sprintf("\nstdout read error: %v", readErr))
			outputMu.Unlock()
		}
	}()
	go func() {
		defer readers.Done()
		contents, readErr := io.ReadAll(stderr)
		outputMu.Lock()
		diagnostics.WriteString(string(contents))
		if readErr != nil {
			diagnostics.WriteString(fmt.Sprintf("\nstderr read error: %v", readErr))
		}
		outputMu.Unlock()
	}()
	waitErr := command.Wait()
	readers.Wait()
	outputMu.Lock()
	stdoutText := output.String()
	stderrText := diagnostics.String()
	outputMu.Unlock()
	exitCode, signal := processResult(waitErr)
	store.finish(state, adapter, exitCode, signal, stdoutText, stderrText)
}

func (store *Store) finish(state *job, adapter harness.Adapter, exitCode int, signal *string, stdout string, stderr string) {
	state.mu.Lock()
	if state.completed {
		state.mu.Unlock()
		return
	}
	now := time.Now()
	state.record.FinishedAt = &now
	state.record.ExitCode = intPointer(exitCode)
	state.record.Signal = cloneString(signal)
	if state.record.Status == harness.JobStatusCancelled {
		state.record.Stderr = tail(stderr, 8000)
	} else {
		finalized, err := adapter.Finalize(context.Background(), harness.FinalizeParams{
			ExitCode: exitCode,
			Signal:   cloneString(signal),
			Stdout:   stdout,
			Stderr:   stderr,
			Record:   &state.record,
		})
		if err != nil {
			state.record.Status = harness.JobStatusFailed
			state.record.Stderr = fmt.Sprintf("finalize %s execution: %v", adapter.ID(), err)
		} else if !finalized.Status.IsTerminal() {
			state.record.Status = harness.JobStatusFailed
			state.record.Stderr = fmt.Sprintf("finalize %s execution: invalid terminal status %q", adapter.ID(), finalized.Status)
		} else {
			state.record.Status = finalized.Status
			state.record.Output = cloneString(finalized.Output)
			state.record.Stderr = finalized.Error
			state.record.Exhausted = finalized.Exhausted
			if state.record.ThreadID == nil {
				state.record.ThreadID = cloneString(finalized.ThreadID)
			}
		}
	}
	record := cloneRecord(state.record)
	conversationID := state.conversationID
	shouldJournal := !state.journaled || state.timedOut
	state.journaled = true
	state.completed = true
	state.mu.Unlock()

	// Journal BEFORE signalling completion, never after.
	//
	// Closing done releases the blocked dispatch, which returns to the CLI,
	// which prints the result and exits the process. Anything left after the
	// close is racing that exit and loses: the record was never written, so the
	// thread id printed on the way out referred to a job no later process could
	// find, and `reply` could not continue anything. The journal is the only
	// place a continuation survives, so it is persisted first and the waiter is
	// woken second.
	if shouldJournal {
		journal.RecordConversation(context.Background(), conversationID, record)
	}
	close(state.done)
}

func conversationIDForThread(provider string, threadID string) string {
	sum := sha256.Sum256([]byte(provider + "\x00" + threadID))
	return "thread-" + hex.EncodeToString(sum[:])
}

func (store *Store) lookup(jobID string) (*job, bool) {
	store.mu.RLock()
	job, ok := store.jobs[jobID]
	store.mu.RUnlock()

	return job, ok
}

func (job *job) snapshot() *harness.JobRecord {
	job.mu.RLock()
	record := cloneRecord(job.record)
	job.mu.RUnlock()

	return &record
}

func commandEnvironment(path string, additions []string) []string {
	environment := append([]string(nil), os.Environ()...)
	if path != "" {
		environment = replaceEnvironmentEntry(environment, "PATH="+path)
	}
	for _, entry := range additions {
		environment = replaceEnvironmentEntry(environment, entry)
	}

	return environment
}

func replaceEnvironmentEntry(environment []string, entry string) []string {
	name, _, ok := strings.Cut(entry, "=")
	if !ok || name == "" {
		return environment
	}
	for index, existing := range environment {
		existingName, _, _ := strings.Cut(existing, "=")
		if existingName == name {
			environment[index] = entry
			return environment
		}
	}

	return append(environment, entry)
}

func readStdout(reader io.Reader, onChunk func(string), onLine func(string)) error {
	buffered := bufio.NewReader(reader)
	for {
		line, err := buffered.ReadString('\n')
		if len(line) > 0 {
			onChunk(line)
			if strings.HasSuffix(line, "\n") {
				onLine(strings.TrimSuffix(line, "\n"))
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func processResult(waitErr error) (int, *string) {
	if waitErr == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(waitErr, &exitError) {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				signal := signalName(status.Signal())
				return exitError.ExitCode(), &signal
			}
		}
		return exitError.ExitCode(), nil
	}

	return -1, nil
}

func signalName(signal syscall.Signal) string {
	switch signal {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGKILL:
		return "SIGKILL"
	default:
		return fmt.Sprintf("SIG%d", signal)
	}
}

func newJobID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		bytes[6] = bytes[6]&0x0f | 0x40
		bytes[8] = bytes[8]&0x3f | 0x80
		encoded := hex.EncodeToString(bytes)
		return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
	}

	return fmt.Sprintf("job-%d", time.Now().UnixNano())
}

func tail(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}

	return value[len(value)-maxBytes:]
}

func cloneRecord(record harness.JobRecord) harness.JobRecord {
	copyOfRecord := record
	copyOfRecord.Model = cloneString(record.Model)
	copyOfRecord.Role = cloneString(record.Role)
	copyOfRecord.Label = cloneString(record.Label)
	copyOfRecord.FinishedAt = cloneTime(record.FinishedAt)
	copyOfRecord.ExitCode = cloneInt(record.ExitCode)
	copyOfRecord.Signal = cloneString(record.Signal)
	copyOfRecord.Output = cloneString(record.Output)
	copyOfRecord.ThreadID = cloneString(record.ThreadID)
	copyOfRecord.Args = append([]string(nil), record.Args...)
	copyOfRecord.RoutedBecause = cloneString(record.RoutedBecause)
	copyOfRecord.PersonaDelivery = clonePersonaDelivery(record.PersonaDelivery)
	copyOfRecord.RoutingScore = cloneFloat(record.RoutingScore)
	copyOfRecord.ReroutedFrom = cloneString(record.ReroutedFrom)
	copyOfRecord.ReroutedTo = cloneString(record.ReroutedTo)

	return copyOfRecord
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}

func clonePersona(persona *harness.Persona) *harness.Persona {
	if persona == nil {
		return nil
	}

	return &harness.Persona{
		Instructions:   persona.Instructions,
		Directory:      persona.Directory,
		ReferenceFiles: append([]string(nil), persona.ReferenceFiles...),
	}
}

func clonePersonaDelivery(delivery *harness.PersonaDelivery) *harness.PersonaDelivery {
	if delivery == nil {
		return nil
	}

	value := *delivery
	return &value
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}

func cloneDuration(value *time.Duration) *time.Duration {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}

func stringPointer(value string) *string {
	return &value
}

func intPointer(value int) *int {
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
