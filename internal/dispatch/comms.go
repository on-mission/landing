package dispatch

import (
	"context"
	"strings"
	"time"

	"github.com/on-mission/landing/internal/comms"
	"github.com/on-mission/landing/internal/harness"
	"github.com/on-mission/landing/internal/jobs"
)

// commsOperationTimeout bounds every comms call on the dispatch path. Comms is
// an addressing convenience, so a failed registration only drops its marker;
// the child still starts once this bound elapses.
//
// A cold open-and-register measures ~9ms, so this is generous by two orders of
// magnitude on purpose: a dispatch runs for seconds or minutes, which makes a
// second of headroom invisible, while a bound tight enough to expire under load
// would drop the marker silently and leave the child without its thread identity.
const commsOperationTimeout = time.Second

type threadParticipant struct {
	store       comms.Store
	participant comms.Participant
	resumable   bool
}

type threadRegistration struct {
	resumable bool
	thread    *threadParticipant
}

func (registration *threadRegistration) beforeSpawn(record harness.JobRecord) []string {
	registration.thread = trackThread(&record, registration.resumable)
	if registration.thread == nil {
		return nil
	}

	return []string{jobs.CommsParticipantEnvironmentEntry(registration.thread.participant.Name)}
}

func (registration *threadRegistration) update(record *harness.JobRecord) {
	if registration.thread == nil {
		registration.thread = trackThread(record, registration.resumable)
		return
	}
	registration.thread.update(record)
}

func trackThread(record *harness.JobRecord, resumable bool) *threadParticipant {
	if record == nil {
		return nil
	}
	operationCtx, cancel := bestEffortCommsContext()
	defer cancel()
	store, err := comms.Open(operationCtx, record.CWD)
	if err != nil {
		return nil
	}
	participant := participantFor(preferredThreadName(record.Label, record.JobID), *record, resumable)
	name, err := store.RegisterUnique(operationCtx, participant)
	if err != nil {
		return nil
	}
	participant.Name = name

	return &threadParticipant{store: store, participant: participant, resumable: resumable}
}

func (thread *threadParticipant) finish(record *harness.JobRecord) {
	thread.update(record)
}

func (thread *threadParticipant) update(record *harness.JobRecord) {
	if thread == nil || record == nil {
		return
	}
	operationCtx, cancel := bestEffortCommsContext()
	defer cancel()
	thread.participant = participantFor(thread.participant.Name, *record, thread.resumable)
	if err := thread.store.Register(operationCtx, thread.participant); err != nil {
		return
	}
}

func bestEffortCommsContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), commsOperationTimeout)
}

func participantFor(name string, record harness.JobRecord, resumable bool) comms.Participant {
	pid := 0
	if record.Proc != nil && record.Proc.Process != nil {
		pid = record.Proc.Process.Pid
	}
	threadID := ""
	if record.ThreadID != nil {
		threadID = *record.ThreadID
	}
	model := ""
	if record.Model != nil {
		model = *record.Model
	}
	activitySince := record.StartedAt
	activityState := comms.StateInTurn
	if record.Status.IsTerminal() {
		activityState = comms.StateEnded
		if threadID != "" && resumable {
			activityState = comms.StateIdle
		}
		if record.FinishedAt != nil {
			activitySince = *record.FinishedAt
		}
	}

	// Register consumes a thread PID as a liveness fact and does not expose it
	// from the durable thread participant.
	return comms.Participant{
		Name:      name,
		Kind:      comms.KindThread,
		Harness:   record.Provider,
		Model:     model,
		CWD:       record.CWD,
		StartedAt: record.StartedAt,
		Activity: comms.Activity{
			State: activityState,
			Since: activitySince,
		},
		PID:      pid,
		ThreadID: threadID,
	}
}

func preferredThreadName(label *string, jobID string) string {
	base := "thread-" + threadNameSuffix(jobID)
	if label != nil && strings.TrimSpace(*label) != "" && *label != comms.AllAgents {
		base = *label
	}

	return base
}

func threadNameSuffix(jobID string) string {
	if index := strings.IndexByte(jobID, '-'); index > 0 {
		return jobID[:index]
	}
	if jobID != "" {
		return jobID
	}

	return "dispatch"
}
