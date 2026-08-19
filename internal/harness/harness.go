// Package harness defines the provider boundary shared by routing and execution.
package harness

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

type CapacityState uint8

const (
	CapacityUnknown CapacityState = iota
	CapacityKnown
)

type Bucket struct {
	ID          string
	UsedPercent float64
	ResetsAt    time.Time
}

type Capacity struct {
	state   CapacityState
	buckets []Bucket
}

func UnknownCapacity() Capacity {
	return Capacity{state: CapacityUnknown}
}

func KnownCapacity(buckets []Bucket) Capacity {
	return Capacity{state: CapacityKnown, buckets: append([]Bucket(nil), buckets...)}
}

func (capacity Capacity) IsKnown() bool {
	return capacity.state == CapacityKnown
}

func (capacity Capacity) Buckets() []Bucket {
	return append([]Bucket(nil), capacity.buckets...)
}

type JobStatus string

const (
	JobStatusRunning   JobStatus = "running"
	JobStatusDone      JobStatus = "done"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
	JobStatusTimeout   JobStatus = "timeout"
)

func ParseJobStatus(value string) (JobStatus, error) {
	status := JobStatus(value)
	switch status {
	case JobStatusRunning, JobStatusDone, JobStatusFailed, JobStatusCancelled, JobStatusTimeout:
		return status, nil
	default:
		return "", fmt.Errorf("invalid job status %q", value)
	}
}

func (status JobStatus) IsTerminal() bool {
	switch status {
	case JobStatusDone, JobStatusFailed, JobStatusCancelled, JobStatusTimeout:
		return true
	case JobStatusRunning:
		return false
	default:
		return false
	}
}

type JobRecord struct {
	JobID           string
	Provider        string
	Model           *string
	Role            *string
	Label           *string
	Status          JobStatus
	StartedAt       time.Time
	FinishedAt      *time.Time
	ExitCode        *int
	Signal          *string
	Output          *string
	Stderr          string
	ThreadID        *string
	Args            []string
	CWD             string
	Proc            *exec.Cmd
	RoutedBecause   *string
	PersonaDelivery *PersonaDelivery
	Capacity        Capacity
	RoutingScore    *float64
	RerouteCount    int
	ReroutedFrom    *string
	ReroutedTo      *string
	Rerouting       <-chan error
	Exhausted       bool
}

type StartParams struct {
	JobID  string
	Prompt string
	// Persona is the requested durable perspective. The adapter chooses the
	// harness-specific delivery mechanism without changing these values.
	Persona         *Persona
	Model           *string
	ReasoningEffort *string
	Sandbox         *string
	CWD             string
	ExtraConfig     []string
	Provider        *string
	Timeout         *time.Duration
}

type ResumeParams struct {
	JobID    string
	ThreadID string
	Prompt   string
	// Persona is the requested durable perspective. The adapter chooses the
	// harness-specific delivery mechanism without changing these values.
	Persona         *Persona
	Model           *string
	ReasoningEffort *string
	Sandbox         *string
	CWD             string
	ExtraConfig     []string
	Provider        *string
	Timeout         *time.Duration
}

type Request struct {
	Command         string
	Args            []string
	PersonaDelivery *PersonaDelivery
}

// Persona is the adapter input for a requested durable perspective.
type Persona struct {
	Instructions   string
	Directory      string
	ReferenceFiles []string
}

// PersonaDelivery identifies the mechanism an adapter used to apply a persona.
type PersonaDelivery string

const (
	PersonaDeliveryAppendSystemPrompt PersonaDelivery = "append-system-prompt"
	PersonaDeliveryPromptComposition  PersonaDelivery = "prompt-composition"
)

type FinalizeParams struct {
	ExitCode int
	Signal   *string
	Stdout   string
	Stderr   string
	Record   *JobRecord
}

type Finalized struct {
	Status    JobStatus
	Output    *string
	Error     string
	Exhausted bool
	ThreadID  *string
}

type Capabilities struct {
	Continuation bool
	Sandbox      bool
}

// DetectionStatus is what Landing can honestly say about a harness on this
// machine without spending capacity.
type DetectionStatus string

const (
	// DetectionAbsent means no executable for this harness was found.
	DetectionAbsent DetectionStatus = "absent"
	// DetectionUnauthenticated means the harness is installed and said so
	// itself — it reported an authentication failure, rather than Landing
	// inferring one from silence.
	DetectionUnauthenticated DetectionStatus = "unauthenticated"
	// DetectionUnreadable means the harness is installed and did not report an
	// authentication failure, but its capacity could not be read. Landing does
	// not resolve this into a claim about authentication, because it does not
	// know which of the two it is.
	DetectionUnreadable DetectionStatus = "unreadable"
	// DetectionReady means the harness answered with a capacity reading.
	DetectionReady DetectionStatus = "ready"
)

// Detection is what Route discovery reports for one harness. It exists so a
// caller composing a tier can see which routes are real on this machine.
//
// It never spends execution capacity: it resolves an executable and performs
// the same read-only capacity read routing already uses.
type Detection struct {
	// Status is the closed classification above.
	Status DetectionStatus

	// Path is the resolved executable, empty when Status is DetectionAbsent.
	Path string

	// Capacity is the reading behind DetectionReady, and unknown otherwise.
	Capacity Capacity

	// Detail is observed state worth reporting — the harness's own words when
	// it explained a failure. Empty when there is nothing to add.
	Detail string
}

type Adapter interface {
	ID() string

	// Models are the models Landing can route to through this harness. It is
	// deliberately what Landing supports rather than everything the provider
	// sells: a model Landing cannot reach is not a choice a caller can make.
	Models() []string

	// Detect reports whether this harness is usable on this machine. It must
	// not spend execution capacity.
	Detect(context.Context) Detection

	Capabilities() Capabilities
	Validate(StartParams) error
	BuildStart(StartParams) (Request, error)
	BuildResume(ResumeParams) (Request, error)
	// OnStdoutLine may set record.ThreadID when its harness emits the thread
	// identifier before process exit. The runtime owns every other record field.
	OnStdoutLine(line string, record *JobRecord)
	Finalize(context.Context, FinalizeParams) (Finalized, error)
	ProbeCapacity(context.Context) Capacity
	// SpawnPath returns the PATH value passed to this harness's child process.
	SpawnPath() string
}

type ErrorCode string

const (
	ErrorCodeInvalidArgs               ErrorCode = "INVALID_ARGS"
	ErrorCodeInvalidCWD                ErrorCode = "INVALID_CWD"
	ErrorCodeConfigNotFound            ErrorCode = "CONFIG_NOT_FOUND"
	ErrorCodeConfigInvalid             ErrorCode = "CONFIG_INVALID"
	ErrorCodeInvalidPersona            ErrorCode = "INVALID_PERSONA"
	ErrorCodePersonaNotFound           ErrorCode = "PERSONA_NOT_FOUND"
	ErrorCodePersonaDelegationUnclosed ErrorCode = "PERSONA_DELEGATION_UNCLOSED"
	ErrorCodeJobNotFound               ErrorCode = "JOB_NOT_FOUND"
	ErrorCodeThreadNotFound            ErrorCode = "THREAD_NOT_FOUND"
	ErrorCodeContinuationUnsupported   ErrorCode = "CONTINUATION_UNSUPPORTED"
	ErrorCodeNoProviderAvailable       ErrorCode = "NO_PROVIDER_AVAILABLE"
	ErrorCodeJobLost                   ErrorCode = "JOB_LOST"
	ErrorCodeInternal                  ErrorCode = "INTERNAL_ERROR"
)

func ParseErrorCode(value string) (ErrorCode, error) {
	code := ErrorCode(value)
	switch code {
	case ErrorCodeInvalidArgs,
		ErrorCodeInvalidCWD,
		ErrorCodeConfigNotFound,
		ErrorCodeConfigInvalid,
		ErrorCodeInvalidPersona,
		ErrorCodePersonaNotFound,
		ErrorCodePersonaDelegationUnclosed,
		ErrorCodeJobNotFound,
		ErrorCodeThreadNotFound,
		ErrorCodeContinuationUnsupported,
		ErrorCodeNoProviderAvailable,
		ErrorCodeJobLost,
		ErrorCodeInternal:
		return code, nil
	default:
		return "", fmt.Errorf("invalid error code %q", value)
	}
}

type Error struct {
	Code    ErrorCode
	Message string
	Details map[string]string
	Cause   error
}

func NewError(code ErrorCode, message string, details map[string]string) *Error {
	return &Error{Code: code, Message: message, Details: cloneDetails(details)}
}

func WrapError(code ErrorCode, message string, details map[string]string, cause error) *Error {
	return &Error{Code: code, Message: message, Details: cloneDetails(details), Cause: cause}
}

func (toolError *Error) Error() string {
	return toolError.Message
}

func (toolError *Error) Unwrap() error {
	return toolError.Cause
}

func cloneDetails(details map[string]string) map[string]string {
	if details == nil {
		return nil
	}

	clone := make(map[string]string, len(details))
	for key, value := range details {
		clone[key] = value
	}

	return clone
}
