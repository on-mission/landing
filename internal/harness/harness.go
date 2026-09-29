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
	CapacityNoGauge
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

func NoCapacityGauge() Capacity {
	return Capacity{state: CapacityNoGauge}
}

func KnownCapacity(buckets []Bucket) Capacity {
	return Capacity{state: CapacityKnown, buckets: append([]Bucket(nil), buckets...)}
}

func (capacity Capacity) IsKnown() bool {
	return capacity.state == CapacityKnown
}

func (capacity Capacity) HasGauge() bool {
	return capacity.state != CapacityNoGauge
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
	// DetectionUnreadable means the harness is installed and exposes a capacity
	// gauge, but the gauge could not be read. Landing does not resolve this into
	// a claim about authentication unless the harness reported one.
	DetectionUnreadable DetectionStatus = "unreadable"
	// DetectionReady means the installed harness is usable. Capacity may be
	// measured or explicitly have no gauge.
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

	// Capacity is measured or explicitly has no gauge for DetectionReady, and is
	// unknown otherwise.
	Capacity Capacity

	// Detail is observed state worth reporting — the harness's own words when
	// it explained a failure. Empty when there is nothing to add.
	Detail string
}

// ModelCatalogAuthority describes the provenance of a model hint list. Neither
// kind is a model-acceptance gate.
type ModelCatalogAuthority string

const (
	ModelCatalogAuthoritative ModelCatalogAuthority = "authoritative"
	ModelCatalogAdvisory      ModelCatalogAuthority = "advisory"
)

// ModelCatalog is the model-hint information a harness can report. It helps a
// caller discover routes but never proves another model cannot run.
type ModelCatalog struct {
	Models    []string
	Aliases   map[string]string
	Latest    string
	Authority ModelCatalogAuthority
}

// ModelValidationStatus is the outcome of asking a harness whether it can run
// one exact model. It is intentionally closed: callers may dispatch only a
// valid model and must preserve an inconclusive result rather than guessing.
type ModelValidationStatus string

const (
	ModelValid      ModelValidationStatus = "valid"
	ModelInvalid    ModelValidationStatus = "invalid"
	ModelUnverified ModelValidationStatus = "unverified"
)

// ModelValidation is normalized evidence from the adapter that owns a
// harness's provider protocol.
type ModelValidation struct {
	Status   ModelValidationStatus
	Evidence string
}

func ValidModel(evidence string) ModelValidation {
	return ModelValidation{Status: ModelValid, Evidence: evidence}
}

func InvalidModel(evidence string) ModelValidation {
	return ModelValidation{Status: ModelInvalid, Evidence: evidence}
}

func UnverifiedModel(evidence string) ModelValidation {
	return ModelValidation{Status: ModelUnverified, Evidence: evidence}
}

// ConfiguredRoute is the configuration a harness receives when it validates
// the routes that name it. Location is caller-facing context supplied by the
// configuration boundary.
type ConfiguredRoute struct {
	Location string
	Model    *string
}

// ConfigValidator is an optional adapter capability for invariants that only a
// harness can know. Configuration invokes it once for every supported harness
// named by a project policy.
type ConfigValidator interface {
	ValidateConfiguration([]ConfiguredRoute) error
}

type Adapter interface {
	ID() string

	// ModelCatalog reports known models, aliases, and a shipped latest default.
	// Its contents are always advisory for acceptance.
	ModelCatalog() ModelCatalog

	// ValidateModel determines whether this harness can serve model. It must
	// return ModelInvalid only for the harness's definitive rejection; every
	// other failed probe is ModelUnverified.
	ValidateModel(context.Context, string) ModelValidation

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
	ErrorCodeModelInvalid              ErrorCode = "MODEL_INVALID"
	ErrorCodeModelUnverified           ErrorCode = "MODEL_UNVERIFIED"
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
		ErrorCodeModelInvalid,
		ErrorCodeModelUnverified,
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
