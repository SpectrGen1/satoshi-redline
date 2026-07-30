// Package domain contains the private normalized diagnostic model.
package domain

import (
	"fmt"
	"time"
)

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

type Severity string

const (
	SeverityNone          Severity = "none"
	SeverityInformational Severity = "informational"
	SeverityLow           Severity = "low"
	SeverityMedium        Severity = "medium"
	SeverityHigh          Severity = "high"
	SeverityCritical      Severity = "critical"
)

type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

type CoverageRole string

const (
	Required      CoverageRole = "required"
	Optional      CoverageRole = "optional"
	Informational CoverageRole = "informational"
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
	ExecutionPartial     ExecutionOutcome = "partial"
	ExecutionUnavailable ExecutionOutcome = "unavailable"
	ExecutionFailed      ExecutionOutcome = "failed"
	ExecutionCancelled   ExecutionOutcome = "cancelled"
)

type UnknownReasonCode string

const (
	SecretReferenceUnavailable UnknownReasonCode = "access.secret_reference_unavailable"
	PermissionDenied           UnknownReasonCode = "access.permission_denied"
	NotConfigured              UnknownReasonCode = "configuration.not_configured"
	Timeout                    UnknownReasonCode = "collection.timeout"
	Cancelled                  UnknownReasonCode = "collection.cancelled"
	ProtocolError              UnknownReasonCode = "collection.protocol_error"
	InvalidResponse            UnknownReasonCode = "collection.invalid_response"
	UnsupportedSchema          UnknownReasonCode = "compatibility.unsupported_response_schema"
	StaleEvidence              UnknownReasonCode = "evidence.stale"
	TemporallyInconsistent     UnknownReasonCode = "evidence.temporally_inconsistent"
	RequiredObservationMissing UnknownReasonCode = "evidence.required_observation_missing"
	EvaluationInternalError    UnknownReasonCode = "evaluation.internal_error"
	AuthenticationRejected     UnknownReasonCode = "access.authentication_rejected"
	ConnectionRefused          UnknownReasonCode = "collection.connection_refused"
	RouteUnreachable           UnknownReasonCode = "collection.route_unreachable"
	RPCError                   UnknownReasonCode = "collection.rpc_error"
)

type DiagnosticStage string

const (
	StageDiscovery            DiagnosticStage = "discovery"
	StageCapabilityResolution DiagnosticStage = "capability_resolution"
	StageCollection           DiagnosticStage = "collection"
	StageNormalization        DiagnosticStage = "normalization"
	StageEvaluation           DiagnosticStage = "evaluation"
	StageCorrelation          DiagnosticStage = "correlation"
)

type Retryability string

const (
	Retryable              Retryability = "retryable"
	NotRetryable           Retryability = "not_retryable"
	ConditionallyRetryable Retryability = "conditionally_retryable"
	RetryabilityUnknown    Retryability = "unknown"
)

type ResolutionClass string

const (
	ResolutionRetryScan            ResolutionClass = "retry_scan"
	ResolutionCorrectConfiguration ResolutionClass = "correct_configuration"
	ResolutionGrantHostAccess      ResolutionClass = "grant_host_access"
	ResolutionUpdateRedline        ResolutionClass = "update_redline"
	ResolutionManualInspection     ResolutionClass = "manual_local_inspection"
	ResolutionNoneKnown            ResolutionClass = "none_known"
)

type UnknownReason struct {
	Code                     UnknownReasonCode
	Stage                    DiagnosticStage
	Retryability             Retryability
	Resolution               ResolutionClass
	CapabilityID, ProviderID string
}

func Reason(code UnknownReasonCode) UnknownReason {
	r := UnknownReason{Code: code, Stage: StageCollection, Retryability: RetryabilityUnknown, Resolution: ResolutionNoneKnown}
	switch code {
	case NotConfigured, SecretReferenceUnavailable:
		r.Stage = StageCapabilityResolution
		r.Retryability = ConditionallyRetryable
		r.Resolution = ResolutionCorrectConfiguration
	case PermissionDenied:
		r.Stage = StageCapabilityResolution
		r.Retryability = ConditionallyRetryable
		r.Resolution = ResolutionGrantHostAccess
	case Timeout, ConnectionRefused, RouteUnreachable:
		r.Retryability = Retryable
		r.Resolution = ResolutionRetryScan
	case UnsupportedSchema:
		r.Stage = StageNormalization
		r.Retryability = NotRetryable
		r.Resolution = ResolutionUpdateRedline
	case StaleEvidence, TemporallyInconsistent, RequiredObservationMissing:
		r.Stage = StageEvaluation
		r.Resolution = ResolutionManualInspection
	}
	return r
}

type EvidenceRole string

const (
	EvidenceSupports              EvidenceRole = "supports"
	EvidenceContradicts           EvidenceRole = "contradicts"
	EvidenceBounds                EvidenceRole = "bounds"
	EvidenceContextualizes        EvidenceRole = "contextualizes"
	EvidenceEstablishesDependency EvidenceRole = "establishes_dependency"
	EvidenceEstablishesTiming     EvidenceRole = "establishes_timing"
)

type EvidenceRef struct {
	ObservationID, ObservationKind string
	Role                           EvidenceRole
}
type EvidenceProblem string

const (
	EvidenceMissing     EvidenceProblem = "missing"
	EvidenceStale       EvidenceProblem = "stale"
	EvidenceConflicting EvidenceProblem = "conflicting"
	EvidenceUnusable    EvidenceProblem = "unusable"
)

type MissingEvidence struct {
	ObservationKind string
	Problem         EvidenceProblem
	ReasonCode      UnknownReasonCode
}
type Thresholds struct {
	MaxTipAgeSeconds                                 int64
	MinVerificationProgress                          float64
	MaxHeaderLag                                     int64
	WarningAvailablePercent, FailureAvailablePercent float64
}

type ObservationValue interface {
	observationValue()
	Validate() error
}

type AuthenticationState string

const (
	AuthenticationAccepted      AuthenticationState = "accepted"
	AuthenticationRejectedState AuthenticationState = "rejected"
	AuthenticationUnknown       AuthenticationState = "unknown"
)

type RPCTransportResult struct {
	Reached        bool
	Authentication AuthenticationState
	Category       string
}

func (RPCTransportResult) observationValue() {}
func (v RPCTransportResult) Validate() error {
	switch v.Authentication {
	case AuthenticationAccepted, AuthenticationRejectedState:
		if !v.Reached {
			return fmt.Errorf("authentication evidence without reached endpoint")
		}
	case AuthenticationUnknown:
	default:
		return fmt.Errorf("invalid authentication state")
	}
	switch v.Category {
	case "valid_json_rpc_response",
		string(NotConfigured),
		string(SecretReferenceUnavailable),
		string(PermissionDenied),
		string(Timeout),
		string(Cancelled),
		string(ProtocolError),
		string(InvalidResponse),
		string(UnsupportedSchema),
		string(AuthenticationRejected),
		string(ConnectionRefused),
		string(RouteUnreachable),
		string(RPCError):
	default:
		return fmt.Errorf("invalid RPC transport category")
	}
	return nil
}

type BitcoinChainState struct {
	Blocks, Headers      int64
	VerificationProgress float64
	InitialBlockDownload bool
	TipTime              time.Time
}

func (BitcoinChainState) observationValue() {}
func (v BitcoinChainState) Validate() error {
	if v.Blocks < 0 || v.Headers < v.Blocks || v.VerificationProgress < 0 || v.VerificationProgress > 1 || v.TipTime.IsZero() {
		return fmt.Errorf("invalid bitcoin chain state")
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
		return fmt.Errorf("invalid filesystem capacity")
	}
	return nil
}

type ClockState struct{ Current time.Time }

func (ClockState) observationValue() {}
func (v ClockState) Validate() error {
	if v.Current.IsZero() {
		return fmt.Errorf("invalid clock state")
	}
	return nil
}

type Observation struct {
	ID, ScanID, ProbeExecutionID, ComponentID, Kind, SchemaVersion, CapabilityID, ProviderID, Sensitivity string
	ObservedAt                                                                                            time.Time
	Value                                                                                                 ObservationValue
}

func (o Observation) Validate() error {
	if o.ID == "" || o.ScanID == "" || o.ProbeExecutionID == "" || o.ComponentID == "" || o.Kind == "" || o.SchemaVersion == "" || o.CapabilityID == "" || o.ProviderID == "" || o.Sensitivity == "" || o.ObservedAt.IsZero() || o.Value == nil {
		return fmt.Errorf("invalid observation envelope")
	}
	expected := ""
	switch o.Value.(type) {
	case RPCTransportResult:
		expected = "bitcoin.rpc.transport"
	case BitcoinChainState:
		expected = "bitcoin.chain.state"
	case FilesystemCapacity:
		expected = "filesystem.capacity"
	case ClockState:
		expected = "system.clock.current"
	default:
		return fmt.Errorf("unknown observation value")
	}
	if o.Kind != expected {
		return fmt.Errorf("observation kind does not match value")
	}
	return o.Value.Validate()
}

type ProbeExecution struct {
	ID, ScanID, ProbeID, ProbeVersion string
	Outcome                           ExecutionOutcome
	StartedAt, FinishedAt             time.Time
	DurationMS                        int64
	Limitation                        UnknownReasonCode
}

func (p ProbeExecution) Validate() error {
	if p.ID == "" || p.ScanID == "" || p.ProbeID == "" || p.ProbeVersion == "" || p.StartedAt.IsZero() || p.FinishedAt.Before(p.StartedAt) || p.DurationMS < 0 {
		return fmt.Errorf("invalid probe execution")
	}
	switch p.Outcome {
	case ExecutionCompleted, ExecutionPartial, ExecutionUnavailable, ExecutionFailed, ExecutionCancelled:
	default:
		return fmt.Errorf("invalid probe execution outcome")
	}
	return nil
}

type Finding struct {
	ID, ScanID, Code, ComponentID string
	Status                        Status
	Applicability                 Applicability
	Severity                      Severity
	Confidence                    Confidence
	EvaluatorID, EvaluatorVersion string
	EvaluatedAt                   time.Time
	Role                          CoverageRole
	Evidence                      []EvidenceRef
	MissingEvidence               []MissingEvidence
	Reasons                       []UnknownReason
	Thresholds                    Thresholds
}

func (f Finding) Validate() error {
	if f.ID == "" || f.ScanID == "" || f.Code == "" || f.ComponentID == "" || f.Status == "" || f.Applicability == "" || f.Severity == "" || f.Confidence == "" || f.EvaluatorID == "" || f.EvaluatorVersion == "" || f.EvaluatedAt.IsZero() || f.Role == "" {
		return fmt.Errorf("invalid finding")
	}
	if f.Status == Unknown && (len(f.Reasons) == 0 || len(f.MissingEvidence) == 0) {
		return fmt.Errorf("unknown finding lacks structured evidence")
	}
	for _, reason := range f.Reasons {
		if reason.Code == "" || reason.Stage == "" || reason.Retryability == "" || reason.Resolution == "" {
			return fmt.Errorf("unknown reason is incomplete")
		}
	}
	for _, missing := range f.MissingEvidence {
		if missing.ObservationKind == "" || missing.ObservationKind == "required" || missing.Problem == "" || missing.ReasonCode == "" {
			return fmt.Errorf("missing evidence is not specific")
		}
	}
	if f.Status != Unknown && f.Applicability == Applicable && len(f.Evidence) == 0 {
		return fmt.Errorf("assessed finding lacks evidence")
	}
	if f.Status != Unknown && (len(f.Reasons) != 0 || len(f.MissingEvidence) != 0) {
		return fmt.Errorf("assessed finding contains unknown evidence")
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

func (s Scan) Validate() error {
	if s.ID == "" || s.StartedAt.IsZero() || s.FinishedAt.Before(s.StartedAt) ||
		s.Summary.Coverage.ApplicableRequired < 0 || s.Summary.Coverage.AssessedRequired < 0 || s.Summary.Coverage.UnknownRequired < 0 ||
		s.Summary.Coverage.AssessedRequired+s.Summary.Coverage.UnknownRequired != s.Summary.Coverage.ApplicableRequired {
		return fmt.Errorf("invalid scan")
	}
	exec := map[string]bool{}
	for _, p := range s.Executions {
		if e := p.Validate(); e != nil {
			return e
		}
		if p.ScanID != s.ID || exec[p.ID] {
			return fmt.Errorf("invalid execution reference")
		}
		exec[p.ID] = true
	}
	obs := map[string]string{}
	for _, o := range s.Observations {
		if e := o.Validate(); e != nil {
			return e
		}
		if o.ScanID != s.ID {
			return fmt.Errorf("observation belongs to another scan")
		}
		if !exec[o.ProbeExecutionID] {
			return fmt.Errorf("observation execution missing")
		}
		if _, duplicate := obs[o.ID]; duplicate {
			return fmt.Errorf("duplicate observation")
		}
		obs[o.ID] = o.Kind
	}
	findings := map[string]bool{}
	var applicable, assessed, unknown int
	for _, f := range s.Findings {
		if e := f.Validate(); e != nil {
			return e
		}
		if f.ScanID != s.ID || findings[f.ID] {
			return fmt.Errorf("invalid finding reference")
		}
		findings[f.ID] = true
		if f.Role == Required && f.Applicability == Applicable {
			applicable++
			if f.Status == Unknown {
				unknown++
			} else {
				assessed++
			}
		}
		for _, r := range f.Evidence {
			kind, exists := obs[r.ObservationID]
			if !exists {
				return fmt.Errorf("finding evidence missing")
			}
			if kind != r.ObservationKind {
				return fmt.Errorf("finding evidence kind mismatch")
			}
		}
	}
	if applicable != s.Summary.Coverage.ApplicableRequired || assessed != s.Summary.Coverage.AssessedRequired || unknown != s.Summary.Coverage.UnknownRequired {
		return fmt.Errorf("coverage does not match findings")
	}
	switch s.Summary.Coverage.State {
	case Complete:
		if s.Summary.Coverage.UnknownRequired != 0 {
			return fmt.Errorf("complete coverage contains unknown findings")
		}
	case Partial, Minimal:
		if s.Summary.Coverage.AssessedRequired == 0 || s.Summary.Coverage.UnknownRequired == 0 {
			return fmt.Errorf("partial coverage counts are inconsistent")
		}
	case Unavailable:
		if s.Summary.Coverage.AssessedRequired != 0 {
			return fmt.Errorf("unavailable coverage contains assessed findings")
		}
	default:
		return fmt.Errorf("invalid coverage state")
	}
	return nil
}
