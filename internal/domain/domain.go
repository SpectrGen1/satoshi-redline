// Package domain contains the private, normalized diagnostic model.
package domain

import "time"

type Status string

const (
	Pass    Status = "pass"
	Warning Status = "warning"
	Failure Status = "failure"
	Unknown Status = "unknown"
)

type Applicability string

const (
	Applicable    Applicability = "applicable"
	NotApplicable Applicability = "not_applicable"
	Undetermined  Applicability = "undetermined"
)

type Health string

const (
	Healthy       Health = "healthy"
	HealthWarning Health = "warning"
	HealthFailure Health = "failure"
	NoConclusion  Health = "no_conclusion"
)

type CoverageState string

const (
	Complete    CoverageState = "complete"
	Partial     CoverageState = "partial"
	Minimal     CoverageState = "minimal"
	Unavailable CoverageState = "unavailable"
)

type ExecutionOutcome string

const (
	ExecutionCompleted   ExecutionOutcome = "completed"
	ExecutionUnavailable ExecutionOutcome = "unavailable"
	ExecutionFailed      ExecutionOutcome = "failed"
	ExecutionCancelled   ExecutionOutcome = "cancelled"
)

type UnknownReason string

const (
	SecretReferenceUnavailable UnknownReason = "access.secret_reference_unavailable"
	PermissionDenied           UnknownReason = "access.permission_denied"
	NotConfigured              UnknownReason = "configuration.not_configured"
	Timeout                    UnknownReason = "collection.timeout"
	ProtocolError              UnknownReason = "collection.protocol_error"
	InvalidResponse            UnknownReason = "collection.invalid_response"
	UnsupportedSchema          UnknownReason = "compatibility.unsupported_response_schema"
	StaleEvidence              UnknownReason = "evidence.stale"
	RequiredObservationMissing UnknownReason = "evidence.required_observation_missing"
	EvaluationInternalError    UnknownReason = "evaluation.internal_error"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type CoverageRole string

const (
	Required      CoverageRole = "required"
	Optional      CoverageRole = "optional"
	Informational CoverageRole = "informational"
)

type EvidenceRef struct{ ObservationID, Kind string }
type MissingEvidence struct {
	Kind   string
	Reason UnknownReason
}
type Thresholds struct {
	MaxTipAgeSeconds        int64
	MinVerificationProgress float64
	MaxHeaderLag            int64
	WarningAvailablePercent float64
	FailureAvailablePercent float64
}
type ObservationValue interface {
	observationValue()
	Validate() error
}
type RPCTransportResult struct {
	Reached, Authenticated bool
	Category               string
}

func (RPCTransportResult) observationValue() {}
func (v RPCTransportResult) Validate() error { return nil }

type BitcoinChainState struct {
	Blocks, Headers      int64
	VerificationProgress float64
	InitialBlockDownload bool
	TipTime              time.Time
}

func (BitcoinChainState) observationValue() {}
func (v BitcoinChainState) Validate() error {
	if v.Blocks < 0 || v.Headers < v.Blocks || v.VerificationProgress < 0 || v.VerificationProgress > 1 || v.TipTime.IsZero() {
		return ErrInvalidObservation
	}
	return nil
}

type FilesystemCapacity struct {
	TotalBytes, AvailableBytes int64
	ReadOnly, MountPresent     bool
}

func (FilesystemCapacity) observationValue() {}
func (v FilesystemCapacity) Validate() error {
	if v.TotalBytes <= 0 || v.AvailableBytes < 0 || v.AvailableBytes > v.TotalBytes {
		return ErrInvalidObservation
	}
	return nil
}

type ClockState struct{ Current time.Time }

func (ClockState) observationValue() {}
func (v ClockState) Validate() error {
	if v.Current.IsZero() {
		return ErrInvalidObservation
	}
	return nil
}

type validationError string

func (e validationError) Error() string { return string(e) }

const ErrInvalidObservation validationError = "invalid observation"

type TypedObservation struct {
	ID, ScanID, ProbeExecutionID, ComponentID, Kind, SchemaVersion, ProviderID, Sensitivity string
	ObservedAt                                                                              time.Time
	Value                                                                                   ObservationValue
}

func (o TypedObservation) Validate() error {
	if o.ID == "" || o.ScanID == "" || o.ProbeExecutionID == "" || o.ComponentID == "" || o.Kind == "" || o.SchemaVersion == "" || o.ObservedAt.IsZero() || o.Value == nil {
		return ErrInvalidObservation
	}
	return o.Value.Validate()
}

type ProbeExecution struct {
	ID, ScanID, ProbeID   string
	Outcome               ExecutionOutcome
	StartedAt, FinishedAt time.Time
	DurationMS            int64
	Reason                UnknownReason
}

// Observation is an approved, normalized fact. It deliberately has no generic payload field.
type Observation struct {
	ID, ScanID, Kind, ComponentID string
	ObservedAt                    time.Time
	RPCReachable                  *bool
	Blocks, Headers               *int64
	VerificationProgress          *float64
	InitialBlockDownload          *bool
	TipTime                       *time.Time
	TotalBytes, AvailableBytes    *int64
	ReadOnly, MountPresent        *bool
}
type Finding struct {
	ID, ScanID, Code, ComponentID string
	Status                        Status
	Applicability                 Applicability
	Reasons                       []UnknownReason
	Evaluator                     string
	Provenance                    string
	EvaluatedAt                   time.Time
	Required                      bool
	Severity                      Severity
	Confidence                    float64
	EvaluatorVersion              string
	Evidence                      []EvidenceRef
	MissingEvidence               []MissingEvidence
	Thresholds                    Thresholds
	Role                          CoverageRole
}

func (f Finding) Validate() error {
	if f.ID == "" || f.Code == "" || f.ComponentID == "" || f.Evaluator == "" || f.EvaluatorVersion == "" || f.EvaluatedAt.IsZero() {
		return validationError("invalid finding")
	}
	if f.Status == Unknown && (len(f.Reasons) == 0 || len(f.MissingEvidence) == 0) {
		return validationError("unknown finding lacks structured evidence")
	}
	return nil
}

type CoverageSummary struct {
	ApplicableRequired, AssessedRequired, UnknownRequired int
	State                                                 CoverageState
}
type ComponentSummary struct {
	ComponentID string
	Health      Health
	Coverage    CoverageSummary
}
type Scan struct {
	ID                    string
	StartedAt, FinishedAt time.Time
	Status                string
	Executions            []ProbeExecution
	Observations          []Observation
	Findings              []Finding
	Summary               ComponentSummary
}
