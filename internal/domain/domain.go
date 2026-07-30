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
