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

type ComponentKind string

const (
	ComponentBitcoinNode     ComponentKind = "bitcoin-node"
	ComponentElectrumBackend ComponentKind = "electrum-backend"
	ComponentLocalSystem     ComponentKind = "local-system"
	ComponentFilesystem      ComponentKind = "filesystem"
)

type ImplementationFamily string

const (
	ImplementationBitcoin    ImplementationFamily = "bitcoin"
	ImplementationElectrum   ImplementationFamily = "electrum"
	ImplementationSystem     ImplementationFamily = "local-system"
	ImplementationFilesystem ImplementationFamily = "filesystem"
)

type InventoryProvenance string

const (
	ProvenanceConfiguration InventoryProvenance = "configuration"
	ProvenanceBuiltIn       InventoryProvenance = "built_in"
)

type InventoryState string

const (
	InventoryConfigured InventoryState = "configured"
	InventoryDiscovered InventoryState = "discovered"
)

type Component struct {
	ID                    string
	Kind                  ComponentKind
	ImplementationFamily  ImplementationFamily
	ImplementationVersion string
	DisplayAlias          string
	Provenance            InventoryProvenance
	State                 InventoryState
}

func (c Component) Validate() error {
	if c.ID == "" || c.DisplayAlias == "" || c.Provenance == "" || c.State == "" {
		return fmt.Errorf("invalid component")
	}
	switch c.Kind {
	case ComponentBitcoinNode:
		if c.ID != "bitcoin.primary" || c.ImplementationFamily != ImplementationBitcoin {
			return fmt.Errorf("invalid bitcoin component")
		}
	case ComponentElectrumBackend:
		if c.ID != "electrum.primary" || c.ImplementationFamily != ImplementationElectrum {
			return fmt.Errorf("invalid electrum component")
		}
	case ComponentLocalSystem:
		if c.ID != "system.local" || c.ImplementationFamily != ImplementationSystem {
			return fmt.Errorf("invalid system component")
		}
	case ComponentFilesystem:
		if c.ID != "filesystem.bitcoin-data" || c.ImplementationFamily != ImplementationFilesystem {
			return fmt.Errorf("invalid filesystem component")
		}
	default:
		return fmt.Errorf("invalid component kind")
	}
	switch c.Provenance {
	case ProvenanceConfiguration, ProvenanceBuiltIn:
	default:
		return fmt.Errorf("invalid component provenance")
	}
	switch c.State {
	case InventoryConfigured, InventoryDiscovered:
	default:
		return fmt.Errorf("invalid component state")
	}
	return nil
}

type RelationshipKind string

const UsesChainFrom RelationshipKind = "uses_chain_from"

type DependencyEdge struct {
	ID                     string
	SourceComponentID      string
	DestinationComponentID string
	Relationship           RelationshipKind
	Provenance             InventoryProvenance
	Confidence             Confidence
	RecordedAt             time.Time
}

func (e DependencyEdge) Validate() error {
	if e.ID == "" || e.SourceComponentID == "" || e.DestinationComponentID == "" || e.RecordedAt.IsZero() {
		return fmt.Errorf("invalid dependency edge")
	}
	if e.Relationship != UsesChainFrom || e.Provenance != ProvenanceConfiguration || e.Confidence != ConfidenceHigh {
		return fmt.Errorf("invalid dependency relationship")
	}
	return nil
}

func BaseComponents(includeElectrum bool) []Component {
	out := []Component{
		{ID: "bitcoin.primary", Kind: ComponentBitcoinNode, ImplementationFamily: ImplementationBitcoin, DisplayAlias: "Bitcoin node", Provenance: ProvenanceConfiguration, State: InventoryConfigured},
		{ID: "system.local", Kind: ComponentLocalSystem, ImplementationFamily: ImplementationSystem, DisplayAlias: "Local system", Provenance: ProvenanceBuiltIn, State: InventoryConfigured},
		{ID: "filesystem.bitcoin-data", Kind: ComponentFilesystem, ImplementationFamily: ImplementationFilesystem, DisplayAlias: "Bitcoin data filesystem", Provenance: ProvenanceConfiguration, State: InventoryConfigured},
	}
	if includeElectrum {
		out = append(out, Component{ID: "electrum.primary", Kind: ComponentElectrumBackend, ImplementationFamily: ImplementationElectrum, DisplayAlias: "Electrum backend", Provenance: ProvenanceConfiguration, State: InventoryConfigured})
	}
	return out
}

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
	TLSFailure                 UnknownReasonCode = "collection.tls_failure"
	UnsupportedProtocol        UnknownReasonCode = "compatibility.unsupported_protocol"
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
	case UnsupportedProtocol:
		r.Stage = StageNormalization
		r.Retryability = NotRetryable
		r.Resolution = ResolutionUpdateRedline
	case TLSFailure:
		r.Retryability = ConditionallyRetryable
		r.Resolution = ResolutionManualInspection
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
	ElectrumTipWarningAgeSeconds                     int64
	ElectrumTipFailureAgeSeconds                     int64
	ElectrumLagWarningBlocks                         int64
	ElectrumLagFailureBlocks                         int64
	MaxObservationSkewSeconds                        int64
	MaxObservationAgeSeconds                         int64
	MaxFutureSkewSeconds                             int64
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

type BitcoinBlockHash struct {
	Height    int64
	BlockHash string
}

func (BitcoinBlockHash) observationValue() {}
func (v BitcoinBlockHash) Validate() error {
	if v.Height < 0 || !validHex(v.BlockHash, 64) {
		return fmt.Errorf("invalid bitcoin block hash")
	}
	return nil
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

type ElectrumTransportKind string

const (
	ElectrumTransportTCP ElectrumTransportKind = "tcp"
	ElectrumTransportTLS ElectrumTransportKind = "tls"
)

type TLSVerificationState string

const (
	TLSNotApplicable       TLSVerificationState = "not_applicable"
	TLSVerified            TLSVerificationState = "verified"
	TLSVerificationFailed  TLSVerificationState = "failed"
	TLSVerificationUnknown TLSVerificationState = "unknown"
)

type ProtocolNegotiationState string

const (
	ProtocolAccepted    ProtocolNegotiationState = "accepted"
	ProtocolUnsupported ProtocolNegotiationState = "unsupported"
	ProtocolUnknown     ProtocolNegotiationState = "unknown"
)

type ElectrumProtocolTransport struct {
	Reached             bool
	Transport           ElectrumTransportKind
	TLSVerification     TLSVerificationState
	ProtocolNegotiation ProtocolNegotiationState
	Category            string
}

func (ElectrumProtocolTransport) observationValue() {}
func (v ElectrumProtocolTransport) Validate() error {
	if v.Transport != ElectrumTransportTCP && v.Transport != ElectrumTransportTLS {
		return fmt.Errorf("invalid Electrum transport")
	}
	if v.Transport == ElectrumTransportTCP && v.TLSVerification != TLSNotApplicable {
		return fmt.Errorf("invalid TCP TLS state")
	}
	if v.Transport == ElectrumTransportTLS {
		switch v.TLSVerification {
		case TLSVerified, TLSVerificationFailed, TLSVerificationUnknown:
		default:
			return fmt.Errorf("invalid TLS verification state")
		}
	}
	switch v.ProtocolNegotiation {
	case ProtocolAccepted, ProtocolUnsupported, ProtocolUnknown:
	default:
		return fmt.Errorf("invalid protocol negotiation state")
	}
	if !v.Reached && (v.TLSVerification == TLSVerified || v.ProtocolNegotiation == ProtocolAccepted || v.ProtocolNegotiation == ProtocolUnsupported) {
		return fmt.Errorf("Electrum evidence without reached endpoint")
	}
	switch v.Category {
	case "valid_protocol_response", string(NotConfigured), string(Timeout), string(Cancelled),
		string(ProtocolError), string(InvalidResponse), string(ConnectionRefused),
		string(RouteUnreachable), string(TLSFailure), string(UnsupportedProtocol), string(RPCError):
	default:
		return fmt.Errorf("invalid Electrum transport category")
	}
	return nil
}

type ElectrumImplementationFamily string

const (
	ElectrumElectrs ElectrumImplementationFamily = "electrs"
	ElectrumFulcrum ElectrumImplementationFamily = "fulcrum"
	ElectrumUnknown ElectrumImplementationFamily = "unknown"
)

type ElectrumServerIdentity struct {
	ImplementationFamily ElectrumImplementationFamily
	ProtocolVersion      string
}

func (ElectrumServerIdentity) observationValue() {}
func (v ElectrumServerIdentity) Validate() error {
	switch v.ImplementationFamily {
	case ElectrumElectrs, ElectrumFulcrum, ElectrumUnknown:
	default:
		return fmt.Errorf("invalid Electrum implementation family")
	}
	switch v.ProtocolVersion {
	case "1.4", "1.4.1", "1.4.2", "1.4.3":
	default:
		return fmt.Errorf("invalid Electrum protocol version")
	}
	return nil
}

type ElectrumChainTip struct {
	Height     int64
	BlockHash  string
	HeaderTime time.Time
}

func (ElectrumChainTip) observationValue() {}
func (v ElectrumChainTip) Validate() error {
	if v.Height < 0 || !validHex(v.BlockHash, 64) || v.HeaderTime.IsZero() {
		return fmt.Errorf("invalid Electrum chain tip")
	}
	return nil
}

func validHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
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
	case BitcoinBlockHash:
		expected = "bitcoin.chain.block-hash"
	case FilesystemCapacity:
		expected = "filesystem.capacity"
	case ClockState:
		expected = "system.clock.current"
	case ElectrumProtocolTransport:
		expected = "electrum.protocol.transport"
	case ElectrumServerIdentity:
		expected = "electrum.server.identity"
	case ElectrumChainTip:
		expected = "electrum.chain.tip"
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
		if !registeredUnknownReason(reason.Code) || reason.Stage == "" || reason.Retryability == "" || reason.Resolution == "" {
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

func registeredUnknownReason(code UnknownReasonCode) bool {
	switch code {
	case SecretReferenceUnavailable, PermissionDenied, NotConfigured, Timeout, Cancelled,
		ProtocolError, InvalidResponse, UnsupportedSchema, StaleEvidence, TemporallyInconsistent,
		RequiredObservationMissing, EvaluationInternalError, AuthenticationRejected,
		ConnectionRefused, RouteUnreachable, RPCError, TLSFailure, UnsupportedProtocol:
		return true
	default:
		return false
	}
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

type DiagnosisClassification string

const (
	DiagnosisRootCause          DiagnosisClassification = "root_cause"
	DiagnosisBreakPoint         DiagnosisClassification = "break_point"
	DiagnosisContributingFactor DiagnosisClassification = "contributing_factor"
	DiagnosisSymptomCluster     DiagnosisClassification = "symptom_cluster"
	DiagnosisInconclusive       DiagnosisClassification = "inconclusive"
)

type Diagnosis struct {
	ID                        string
	ScanID                    string
	Code                      string
	Classification            DiagnosisClassification
	Confidence                Confidence
	SummaryKey                string
	ComponentIDs              []string
	FindingIDs                []string
	Evidence                  []EvidenceRef
	BlockingUnknownFindingIDs []string
	PlausibleAlternativeCodes []string
	CorrelationRuleID         string
	CorrelationRuleVersion    string
	DiagnosedAt               time.Time
}

func (d Diagnosis) Validate() error {
	if d.ID == "" || d.ScanID == "" || d.Code == "" || d.Classification == "" || d.Confidence == "" ||
		d.SummaryKey == "" || len(d.ComponentIDs) == 0 || len(d.FindingIDs) == 0 ||
		d.CorrelationRuleID == "" || d.CorrelationRuleVersion == "" || d.DiagnosedAt.IsZero() {
		return fmt.Errorf("invalid diagnosis")
	}
	switch d.Classification {
	case DiagnosisRootCause, DiagnosisBreakPoint, DiagnosisContributingFactor, DiagnosisSymptomCluster, DiagnosisInconclusive:
	default:
		return fmt.Errorf("invalid diagnosis classification")
	}
	switch d.Confidence {
	case ConfidenceLow, ConfidenceMedium, ConfidenceHigh:
	default:
		return fmt.Errorf("invalid diagnosis confidence")
	}
	if d.Classification == DiagnosisInconclusive && len(d.BlockingUnknownFindingIDs) == 0 {
		return fmt.Errorf("inconclusive diagnosis lacks blocking unknown")
	}
	return nil
}

type Scan struct {
	ID                    string
	StartedAt, FinishedAt time.Time
	Status                string
	Components            []Component
	DependencyEdges       []DependencyEdge
	Executions            []ProbeExecution
	Observations          []Observation
	Findings              []Finding
	Diagnoses             []Diagnosis
	Summary               ComponentSummary
}

func (s Scan) Validate() error {
	if s.ID == "" || s.StartedAt.IsZero() || s.FinishedAt.Before(s.StartedAt) ||
		s.Summary.Coverage.ApplicableRequired < 0 || s.Summary.Coverage.AssessedRequired < 0 || s.Summary.Coverage.UnknownRequired < 0 ||
		s.Summary.Coverage.AssessedRequired+s.Summary.Coverage.UnknownRequired != s.Summary.Coverage.ApplicableRequired {
		return fmt.Errorf("invalid scan")
	}
	components := map[string]ComponentKind{}
	for _, component := range s.Components {
		if e := component.Validate(); e != nil {
			return e
		}
		if _, duplicate := components[component.ID]; duplicate {
			return fmt.Errorf("duplicate component")
		}
		components[component.ID] = component.Kind
	}
	if len(components) == 0 {
		return fmt.Errorf("scan has no component inventory")
	}
	edges := map[string]bool{}
	for _, edge := range s.DependencyEdges {
		if e := edge.Validate(); e != nil {
			return e
		}
		if edges[edge.ID] {
			return fmt.Errorf("duplicate dependency edge")
		}
		edges[edge.ID] = true
		source, sourceOK := components[edge.SourceComponentID]
		destination, destinationOK := components[edge.DestinationComponentID]
		if !sourceOK || !destinationOK {
			return fmt.Errorf("dependency edge component missing")
		}
		if edge.Relationship == UsesChainFrom && (source != ComponentElectrumBackend || destination != ComponentBitcoinNode) {
			return fmt.Errorf("invalid dependency component kinds")
		}
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
		if _, exists := components[o.ComponentID]; !exists {
			return fmt.Errorf("observation component missing")
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
	findingStatuses := map[string]Status{}
	var applicable, assessed, unknown int
	for _, f := range s.Findings {
		if e := f.Validate(); e != nil {
			return e
		}
		if f.ScanID != s.ID || findings[f.ID] {
			return fmt.Errorf("invalid finding reference")
		}
		if _, exists := components[f.ComponentID]; !exists {
			return fmt.Errorf("finding component missing")
		}
		findings[f.ID] = true
		findingStatuses[f.ID] = f.Status
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
	diagnoses := map[string]bool{}
	for _, diagnosis := range s.Diagnoses {
		if e := diagnosis.Validate(); e != nil {
			return e
		}
		if diagnosis.ScanID != s.ID || diagnoses[diagnosis.ID] {
			return fmt.Errorf("invalid diagnosis reference")
		}
		diagnoses[diagnosis.ID] = true
		for _, componentID := range diagnosis.ComponentIDs {
			if _, exists := components[componentID]; !exists {
				return fmt.Errorf("diagnosis component missing")
			}
		}
		for _, findingID := range diagnosis.FindingIDs {
			if !findings[findingID] {
				return fmt.Errorf("diagnosis finding missing")
			}
		}
		for _, findingID := range diagnosis.BlockingUnknownFindingIDs {
			if findingStatuses[findingID] != Unknown {
				return fmt.Errorf("diagnosis blocking unknown missing")
			}
		}
		for _, reference := range diagnosis.Evidence {
			kind, exists := obs[reference.ObservationID]
			if !exists {
				return fmt.Errorf("diagnosis evidence missing")
			}
			if kind != reference.ObservationKind {
				return fmt.Errorf("diagnosis evidence kind mismatch")
			}
		}
	}
	if applicable != s.Summary.Coverage.ApplicableRequired || assessed != s.Summary.Coverage.AssessedRequired || unknown != s.Summary.Coverage.UnknownRequired {
		return fmt.Errorf("coverage does not match findings")
	}
	if _, exists := components[s.Summary.ComponentID]; !exists {
		return fmt.Errorf("summary component missing")
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
