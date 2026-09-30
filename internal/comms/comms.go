// Package comms addresses the agents working in one project and carries
// messages between them.
//
// Two populations share one namespace. A session is a live interactive agent
// somebody started. Landing does not push a message into that session; the
// session reads its inbox by running Landing. A thread is work Landing
// dispatched. It is dormant and resumable, so Landing can wake it and get an
// answer. Landing reports which kind a participant is.
//
// Messages are durable, attributed, and timestamped. They are delivered once
// and then become history that ages out. Nothing here is a lock: an agent reads
// what was said, by whom, and when, and judges whether it still applies.
package comms

import (
	"context"
	"time"
)

// AllAgents addresses every participant in the project. It is the literal word
// rather than a wildcard because a shell expands "*" to filenames before
// Landing is invoked.
const AllAgents = "all"

// Kind is which population a participant belongs to.
type Kind string

const (
	// KindSession is a live interactive agent. It reads its own inbox.
	KindSession Kind = "session"
	// KindThread is dispatched work, dormant and resumable on demand.
	KindThread Kind = "thread"
)

// State is what a participant was doing when its activity was last recorded.
type State string

const (
	StateInTurn State = "in turn"
	StateIdle   State = "idle"
	StateEnded  State = "ended"
)

// Activity is the last reported state of a participant. It is only ever as
// current as the last recorded activity, and callers render it with Since so a
// reader can judge how stale it is.
type Activity struct {
	State    State
	Since    time.Time
	LastTool string
}

// Participant is one addressable agent. Name is unique within a project and is
// how every other command refers to it.
type Participant struct {
	Name      string
	Kind      Kind
	Harness   string
	Model     string
	CWD       string
	StartedAt time.Time
	Activity  Activity

	// PID and SessionID identify a session. ThreadID identifies a thread. The
	// fields that do not apply to a participant's Kind are zero.
	PID       int
	SessionID string
	ThreadID  string
}

// Message is one communication. A message with RequiresResponse blocks its
// sender until an answer arrives or the sender's bound elapses; the message
// stays pending either way.
type Message struct {
	ID     string
	From   string
	To     string
	Body   string
	SentAt time.Time

	RequiresResponse bool
	// InResponseTo is the ID of the message this answers, empty otherwise.
	InResponseTo string
	// DeliveredAt is nil until the message has reached its recipient.
	DeliveredAt *time.Time
}

// Delivered reports whether the message has reached its recipient.
func (message Message) Delivered() bool {
	return message.DeliveredAt != nil
}

// Broadcast reports whether the message addresses every participant.
func (message Message) Broadcast() bool {
	return message.To == AllAgents
}

// Store holds one project's participants and messages. Implementations are
// safe for concurrent use across processes: every dispatched thread and every
// interactive command is a separate process reaching the same state.
type Store interface {
	// Register records a participant, replacing any prior registration under
	// the same name. Deregister removes one.
	Register(ctx context.Context, participant Participant) error
	Deregister(ctx context.Context, name string) error

	// RegisterUnique registers a participant under participant.Name if that name
	// is free, and otherwise under the first available name derived from it. It
	// returns the name taken.
	//
	// Choosing the name and claiming it happen together, so two processes
	// registering the same preferred name at the same moment end up with
	// different names instead of one silently replacing the other. Callers that
	// derive a name from something a human chose — a dispatch label, say — use
	// this rather than checking and then registering.
	RegisterUnique(ctx context.Context, participant Participant) (string, error)

	// Rename moves a participant to a new name, carrying its messages with it.
	Rename(ctx context.Context, from string, to string) error

	// ReportActivity records what a participant is doing.
	ReportActivity(ctx context.Context, name string, activity Activity) error

	// Participants returns everyone in the project. A session whose process is
	// gone is reported with StateEnded rather than omitted, so a reader can
	// tell "finished" from "never existed".
	Participants(ctx context.Context) ([]Participant, error)

	// Send records a message and returns it with its assigned ID and time.
	Send(ctx context.Context, message Message) (Message, error)

	// Pending returns undelivered messages addressed to a recipient, oldest
	// first, including broadcasts it has not yet received.
	Pending(ctx context.Context, recipient string) ([]Message, error)

	// MarkDelivered records that a recipient has received messages. Delivery is
	// recorded per recipient because a broadcast reaches each one separately.
	MarkDelivered(ctx context.Context, recipient string, ids []string) error

	// History returns messages sent within the window, oldest first, whether or
	// not they were delivered.
	History(ctx context.Context, window time.Duration) ([]Message, error)

	// AwaitResponse blocks until a message answering id arrives or the bound
	// elapses. Exceeding the bound is not an error: it returns ok false, and
	// the original message stays pending.
	AwaitResponse(ctx context.Context, id string, bound time.Duration) (Message, bool, error)
}
