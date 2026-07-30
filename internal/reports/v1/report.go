package v1

import (
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/satoshi-redline/redline/internal/buildinfo"
	"github.com/satoshi-redline/redline/internal/domain"
	reportschema "github.com/satoshi-redline/redline/schemas/reports/v1"
	"time"
)

type Report struct {
	Format          string              `json:"format"`
	SchemaID        string              `json:"schema_id"`
	FormatVersion   int                 `json:"format_version"`
	ReportID        string              `json:"report_id"`
	GeneratedAt     string              `json:"generated_at"`
	Producer        Producer            `json:"producer"`
	Privacy         Privacy             `json:"privacy"`
	Scan            ScanDTO             `json:"scan"`
	Coverage        CoverageDTO         `json:"coverage"`
	Components      []ComponentDTO      `json:"components"`
	DependencyEdges []DependencyEdgeDTO `json:"dependency_edges"`
	ProbeExecutions []ExecutionDTO      `json:"probe_executions"`
	Observations    []ObservationDTO    `json:"observations"`
	Findings        []FindingDTO        `json:"findings"`
	Diagnoses       []DiagnosisDTO      `json:"diagnoses"`
}
type Producer struct {
	ProductName    string `json:"product_name"`
	ProductVersion string `json:"product_version"`
	BuildID        string `json:"build_id"`
}
type Privacy struct {
	Profile   string `json:"profile"`
	Validated bool   `json:"validated"`
}
type ScanDTO struct {
	ID         string `json:"id"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Status     string `json:"status"`
	Health     string `json:"health"`
}
type CoverageDTO struct {
	State              string `json:"state"`
	ApplicableRequired int    `json:"applicable_required"`
	AssessedRequired   int    `json:"assessed_required"`
	UnknownRequired    int    `json:"unknown_required"`
}
type ExecutionDTO struct {
	ID             string `json:"id"`
	ProbeID        string `json:"probe_id"`
	ProbeVersion   string `json:"probe_version"`
	Outcome        string `json:"outcome"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	DurationMS     int64  `json:"duration_ms"`
	LimitationCode string `json:"limitation_code,omitempty"`
}
type ObservationDTO struct {
	ID                string                `json:"id"`
	ProbeExecutionID  string                `json:"probe_execution_id"`
	ComponentID       string                `json:"component_id"`
	Kind              string                `json:"kind"`
	SchemaVersion     string                `json:"schema_version"`
	ObservedAt        string                `json:"observed_at"`
	CapabilityID      string                `json:"capability_id"`
	ProviderID        string                `json:"provider_id"`
	Sensitivity       string                `json:"sensitivity"`
	RPCTransport      *RPCTransportDTO      `json:"bitcoin_rpc_transport,omitempty"`
	ChainState        *ChainDTO             `json:"bitcoin_chain_state,omitempty"`
	Filesystem        *FilesystemDTO        `json:"filesystem_capacity,omitempty"`
	Clock             *ClockDTO             `json:"clock_state,omitempty"`
	BitcoinBlockHash  *BitcoinBlockHashDTO  `json:"bitcoin_block_hash,omitempty"`
	ElectrumTransport *ElectrumTransportDTO `json:"electrum_protocol_transport,omitempty"`
	ElectrumIdentity  *ElectrumIdentityDTO  `json:"electrum_server_identity,omitempty"`
	ElectrumTip       *ElectrumTipDTO       `json:"electrum_chain_tip,omitempty"`
}
type RPCTransportDTO struct {
	Reached        bool   `json:"reached"`
	Authentication string `json:"authentication"`
	Category       string `json:"category"`
}
type ChainDTO struct {
	Blocks               int64   `json:"blocks"`
	Headers              int64   `json:"headers"`
	VerificationProgress float64 `json:"verification_progress"`
	InitialBlockDownload bool    `json:"initial_block_download"`
	TipTime              string  `json:"tip_time"`
}
type FilesystemDTO struct {
	TotalBytes     int64 `json:"total_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	ReadOnly       bool  `json:"read_only"`
	MountPresent   bool  `json:"mount_present"`
}
type ClockDTO struct {
	CurrentTime string `json:"current_time"`
}
type BitcoinBlockHashDTO struct {
	Height    int64  `json:"height"`
	BlockHash string `json:"block_hash"`
}
type ElectrumTransportDTO struct {
	Reached             bool   `json:"reached"`
	TransportKind       string `json:"transport_kind"`
	TLSVerification     string `json:"tls_verification"`
	ProtocolNegotiation string `json:"protocol_negotiation"`
	Category            string `json:"category"`
}
type ElectrumIdentityDTO struct {
	ImplementationFamily string `json:"implementation_family"`
	ProtocolVersion      string `json:"protocol_version"`
}
type ElectrumTipDTO struct {
	Height     int64  `json:"height"`
	BlockHash  string `json:"block_hash"`
	HeaderTime string `json:"header_time"`
}
type ReasonDTO struct {
	Code            string `json:"code"`
	Stage           string `json:"stage"`
	Retryability    string `json:"retryability"`
	ResolutionClass string `json:"resolution_class"`
	CapabilityID    string `json:"capability_id,omitempty"`
	ProviderID      string `json:"provider_id,omitempty"`
}
type EvidenceDTO struct {
	ObservationID   string `json:"observation_id"`
	ObservationKind string `json:"observation_kind"`
	Role            string `json:"role"`
}
type MissingDTO struct {
	ObservationKind string `json:"observation_kind"`
	Problem         string `json:"problem"`
	ReasonCode      string `json:"reason_code"`
}
type FindingDTO struct {
	ID               string        `json:"id"`
	Code             string        `json:"code"`
	ComponentID      string        `json:"component_id"`
	Status           string        `json:"status"`
	Applicability    string        `json:"applicability"`
	Severity         string        `json:"severity"`
	Confidence       string        `json:"confidence"`
	EvaluatorID      string        `json:"evaluator_id"`
	EvaluatorVersion string        `json:"evaluator_version"`
	EvaluatedAt      string        `json:"evaluated_at"`
	CoverageRole     string        `json:"coverage_role"`
	Reasons          []ReasonDTO   `json:"unknown_reasons"`
	Evidence         []EvidenceDTO `json:"evidence"`
	Missing          []MissingDTO  `json:"missing_evidence"`
	Thresholds       ThresholdDTO  `json:"thresholds"`
}
type ThresholdDTO struct {
	MaxTipAgeSeconds             int64   `json:"max_tip_age_seconds"`
	MinVerificationProgress      float64 `json:"min_verification_progress"`
	MaxHeaderLag                 int64   `json:"max_header_lag"`
	WarningAvailablePercent      float64 `json:"warning_available_percent"`
	FailureAvailablePercent      float64 `json:"failure_available_percent"`
	ElectrumTipWarningAgeSeconds int64   `json:"electrum_tip_warning_age_seconds"`
	ElectrumTipFailureAgeSeconds int64   `json:"electrum_tip_failure_age_seconds"`
	ElectrumLagWarningBlocks     int64   `json:"electrum_lag_warning_blocks"`
	ElectrumLagFailureBlocks     int64   `json:"electrum_lag_failure_blocks"`
	MaxObservationSkewSeconds    int64   `json:"max_observation_skew_seconds"`
	MaxObservationAgeSeconds     int64   `json:"max_observation_age_seconds"`
	MaxFutureSkewSeconds         int64   `json:"max_future_skew_seconds"`
}

type ComponentDTO struct {
	ID                    string `json:"component_id"`
	Kind                  string `json:"component_kind"`
	ImplementationFamily  string `json:"implementation_family"`
	ImplementationVersion string `json:"implementation_version,omitempty"`
	DisplayAlias          string `json:"display_alias"`
	Provenance            string `json:"provenance"`
	State                 string `json:"state"`
}
type DependencyEdgeDTO struct {
	ID                     string `json:"edge_id"`
	SourceComponentID      string `json:"source_component_id"`
	DestinationComponentID string `json:"destination_component_id"`
	RelationshipKind       string `json:"relationship_kind"`
	Provenance             string `json:"provenance"`
	Confidence             string `json:"confidence"`
	RecordedAt             string `json:"recorded_at"`
}
type DiagnosisDTO struct {
	ID                        string        `json:"id"`
	Code                      string        `json:"code"`
	Classification            string        `json:"classification"`
	Confidence                string        `json:"confidence"`
	SummaryKey                string        `json:"summary_key"`
	ComponentIDs              []string      `json:"component_ids"`
	FindingIDs                []string      `json:"finding_ids"`
	Evidence                  []EvidenceDTO `json:"evidence"`
	BlockingUnknownFindingIDs []string      `json:"blocking_unknown_finding_ids"`
	PlausibleAlternativeCodes []string      `json:"plausible_alternative_codes"`
	CorrelationRuleID         string        `json:"correlation_rule_id"`
	CorrelationRuleVersion    string        `json:"correlation_rule_version"`
	DiagnosedAt               string        `json:"diagnosed_at"`
}

func FromDomain(x domain.Scan) Report {
	r := Report{Format: "satoshi-redline-report", SchemaID: "urn:satoshi-redline:schema:report:v1", FormatVersion: 1, ReportID: "report_" + x.ID, GeneratedAt: x.FinishedAt.UTC().Format(time.RFC3339), Producer: Producer{buildinfo.ProductName, buildinfo.Version, buildinfo.BuildID}, Privacy: Privacy{"shareable", true}, Scan: ScanDTO{x.ID, x.StartedAt.UTC().Format(time.RFC3339), x.FinishedAt.UTC().Format(time.RFC3339), x.Status, string(x.Summary.Health)}, Coverage: CoverageDTO{string(x.Summary.Coverage.State), x.Summary.Coverage.ApplicableRequired, x.Summary.Coverage.AssessedRequired, x.Summary.Coverage.UnknownRequired}, Components: []ComponentDTO{}, DependencyEdges: []DependencyEdgeDTO{}, ProbeExecutions: []ExecutionDTO{}, Observations: []ObservationDTO{}, Findings: []FindingDTO{}, Diagnoses: []DiagnosisDTO{}}
	for _, p := range x.Executions {
		r.ProbeExecutions = append(r.ProbeExecutions, ExecutionDTO{p.ID, p.ProbeID, p.ProbeVersion, string(p.Outcome), p.StartedAt.UTC().Format(time.RFC3339), p.FinishedAt.UTC().Format(time.RFC3339), p.DurationMS, string(p.Limitation)})
	}
	for _, component := range x.Components {
		r.Components = append(r.Components, ComponentDTO{component.ID, string(component.Kind), string(component.ImplementationFamily), component.ImplementationVersion, component.DisplayAlias, string(component.Provenance), string(component.State)})
	}
	for _, edge := range x.DependencyEdges {
		r.DependencyEdges = append(r.DependencyEdges, DependencyEdgeDTO{edge.ID, edge.SourceComponentID, edge.DestinationComponentID, string(edge.Relationship), string(edge.Provenance), string(edge.Confidence), edge.RecordedAt.UTC().Format(time.RFC3339)})
	}
	for _, o := range x.Observations {
		d := ObservationDTO{ID: o.ID, ProbeExecutionID: o.ProbeExecutionID, ComponentID: o.ComponentID, Kind: o.Kind, SchemaVersion: o.SchemaVersion, ObservedAt: o.ObservedAt.UTC().Format(time.RFC3339), CapabilityID: o.CapabilityID, ProviderID: o.ProviderID, Sensitivity: o.Sensitivity}
		switch v := o.Value.(type) {
		case domain.RPCTransportResult:
			d.RPCTransport = &RPCTransportDTO{v.Reached, string(v.Authentication), v.Category}
		case domain.BitcoinChainState:
			d.ChainState = &ChainDTO{v.Blocks, v.Headers, v.VerificationProgress, v.InitialBlockDownload, v.TipTime.UTC().Format(time.RFC3339)}
		case domain.FilesystemCapacity:
			d.Filesystem = &FilesystemDTO{v.TotalBytes, v.AvailableBytes, v.ReadOnly, v.MountPresent}
		case domain.ClockState:
			d.Clock = &ClockDTO{v.Current.UTC().Format(time.RFC3339)}
		case domain.BitcoinBlockHash:
			d.BitcoinBlockHash = &BitcoinBlockHashDTO{v.Height, v.BlockHash}
		case domain.ElectrumProtocolTransport:
			d.ElectrumTransport = &ElectrumTransportDTO{v.Reached, string(v.Transport), string(v.TLSVerification), string(v.ProtocolNegotiation), v.Category}
		case domain.ElectrumServerIdentity:
			d.ElectrumIdentity = &ElectrumIdentityDTO{string(v.ImplementationFamily), v.ProtocolVersion}
		case domain.ElectrumChainTip:
			d.ElectrumTip = &ElectrumTipDTO{v.Height, v.BlockHash, v.HeaderTime.UTC().Format(time.RFC3339)}
		}
		r.Observations = append(r.Observations, d)
	}
	for _, f := range x.Findings {
		t := f.Thresholds
		d := FindingDTO{ID: f.ID, Code: f.Code, ComponentID: f.ComponentID, Status: string(f.Status), Applicability: string(f.Applicability), Severity: string(f.Severity), Confidence: string(f.Confidence), EvaluatorID: f.EvaluatorID, EvaluatorVersion: f.EvaluatorVersion, EvaluatedAt: f.EvaluatedAt.UTC().Format(time.RFC3339), CoverageRole: string(f.Role), Reasons: []ReasonDTO{}, Evidence: []EvidenceDTO{}, Missing: []MissingDTO{}, Thresholds: ThresholdDTO{t.MaxTipAgeSeconds, t.MinVerificationProgress, t.MaxHeaderLag, t.WarningAvailablePercent, t.FailureAvailablePercent, t.ElectrumTipWarningAgeSeconds, t.ElectrumTipFailureAgeSeconds, t.ElectrumLagWarningBlocks, t.ElectrumLagFailureBlocks, t.MaxObservationSkewSeconds, t.MaxObservationAgeSeconds, t.MaxFutureSkewSeconds}}
		for _, v := range f.Reasons {
			d.Reasons = append(d.Reasons, ReasonDTO{string(v.Code), string(v.Stage), string(v.Retryability), string(v.Resolution), v.CapabilityID, v.ProviderID})
		}
		for _, v := range f.Evidence {
			d.Evidence = append(d.Evidence, EvidenceDTO{v.ObservationID, v.ObservationKind, string(v.Role)})
		}
		for _, v := range f.MissingEvidence {
			d.Missing = append(d.Missing, MissingDTO{v.ObservationKind, string(v.Problem), string(v.ReasonCode)})
		}
		r.Findings = append(r.Findings, d)
	}
	for _, diagnosis := range x.Diagnoses {
		dto := DiagnosisDTO{
			ID: diagnosis.ID, Code: diagnosis.Code, Classification: string(diagnosis.Classification),
			Confidence: string(diagnosis.Confidence), SummaryKey: diagnosis.SummaryKey,
			ComponentIDs: append([]string{}, diagnosis.ComponentIDs...),
			FindingIDs:   append([]string{}, diagnosis.FindingIDs...), Evidence: []EvidenceDTO{},
			BlockingUnknownFindingIDs: append([]string{}, diagnosis.BlockingUnknownFindingIDs...),
			PlausibleAlternativeCodes: append([]string{}, diagnosis.PlausibleAlternativeCodes...),
			CorrelationRuleID:         diagnosis.CorrelationRuleID, CorrelationRuleVersion: diagnosis.CorrelationRuleVersion,
			DiagnosedAt: diagnosis.DiagnosedAt.UTC().Format(time.RFC3339),
		}
		for _, evidence := range diagnosis.Evidence {
			dto.Evidence = append(dto.Evidence, EvidenceDTO{evidence.ObservationID, evidence.ObservationKind, string(evidence.Role)})
		}
		r.Diagnoses = append(r.Diagnoses, dto)
	}
	return r
}
func Validate(r Report) error {
	if len(reportschema.Schema) == 0 {
		return fmt.Errorf("report schema unavailable")
	}
	if r.Format != "satoshi-redline-report" || r.SchemaID != "urn:satoshi-redline:schema:report:v1" || r.FormatVersion != 1 {
		return fmt.Errorf("invalid report envelope")
	}
	if r.ReportID == "" || r.Scan.ID == "" || r.ReportID != "report_"+r.Scan.ID {
		return fmt.Errorf("invalid report identity")
	}
	executions := map[string]bool{}
	for _, execution := range r.ProbeExecutions {
		if execution.ID == "" || executions[execution.ID] {
			return fmt.Errorf("invalid probe execution identity")
		}
		executions[execution.ID] = true
	}
	components := map[string]bool{}
	for _, component := range r.Components {
		if component.ID == "" || components[component.ID] {
			return fmt.Errorf("invalid component identity")
		}
		components[component.ID] = true
	}
	edges := map[string]bool{}
	for _, edge := range r.DependencyEdges {
		if edge.ID == "" || edges[edge.ID] || !components[edge.SourceComponentID] || !components[edge.DestinationComponentID] {
			return fmt.Errorf("invalid dependency edge")
		}
		edges[edge.ID] = true
	}
	obs := map[string]string{}
	for _, o := range r.Observations {
		if o.ID == "" {
			return fmt.Errorf("invalid observation identity")
		}
		if _, duplicate := obs[o.ID]; duplicate {
			return fmt.Errorf("duplicate observation identity")
		}
		if !executions[o.ProbeExecutionID] {
			return fmt.Errorf("invalid observation execution reference")
		}
		if !components[o.ComponentID] {
			return fmt.Errorf("invalid observation component reference")
		}
		variants := 0
		matching := false
		if o.RPCTransport != nil {
			variants++
			matching = matching || o.Kind == "bitcoin.rpc.transport"
		}
		if o.ChainState != nil {
			variants++
			matching = matching || o.Kind == "bitcoin.chain.state"
		}
		if o.Filesystem != nil {
			variants++
			matching = matching || o.Kind == "filesystem.capacity"
		}
		if o.Clock != nil {
			variants++
			matching = matching || o.Kind == "system.clock.current"
		}
		if o.BitcoinBlockHash != nil {
			variants++
			matching = matching || o.Kind == "bitcoin.chain.block-hash"
		}
		if o.ElectrumTransport != nil {
			variants++
			matching = matching || o.Kind == "electrum.protocol.transport"
		}
		if o.ElectrumIdentity != nil {
			variants++
			matching = matching || o.Kind == "electrum.server.identity"
		}
		if o.ElectrumTip != nil {
			variants++
			matching = matching || o.Kind == "electrum.chain.tip"
		}
		if variants != 1 || !matching {
			return fmt.Errorf("invalid observation value variant")
		}
		obs[o.ID] = o.Kind
	}
	findings := map[string]bool{}
	findingStatus := map[string]string{}
	for _, f := range r.Findings {
		if f.ID == "" || findings[f.ID] {
			return fmt.Errorf("invalid finding identity")
		}
		if !components[f.ComponentID] {
			return fmt.Errorf("invalid finding component reference")
		}
		findings[f.ID] = true
		findingStatus[f.ID] = f.Status
		if f.Status == "unknown" && (len(f.Reasons) == 0 || len(f.Missing) == 0) {
			return fmt.Errorf("invalid unknown finding")
		}
		if f.Status != "unknown" && (len(f.Evidence) == 0 || len(f.Reasons) != 0 || len(f.Missing) != 0) {
			return fmt.Errorf("invalid assessed finding")
		}
		for _, e := range f.Evidence {
			kind, exists := obs[e.ObservationID]
			if !exists {
				return fmt.Errorf("invalid evidence reference")
			}
			if kind != e.ObservationKind {
				return fmt.Errorf("invalid evidence kind")
			}
		}
	}
	diagnoses := map[string]bool{}
	for _, diagnosis := range r.Diagnoses {
		if diagnosis.ID == "" || diagnoses[diagnosis.ID] || len(diagnosis.ComponentIDs) == 0 || len(diagnosis.FindingIDs) == 0 {
			return fmt.Errorf("invalid diagnosis identity")
		}
		diagnoses[diagnosis.ID] = true
		for _, componentID := range diagnosis.ComponentIDs {
			if !components[componentID] {
				return fmt.Errorf("invalid diagnosis component reference")
			}
		}
		for _, findingID := range diagnosis.FindingIDs {
			if !findings[findingID] {
				return fmt.Errorf("invalid diagnosis finding reference")
			}
		}
		for _, findingID := range diagnosis.BlockingUnknownFindingIDs {
			if findingStatus[findingID] != "unknown" {
				return fmt.Errorf("invalid diagnosis blocking reference")
			}
		}
		for _, evidence := range diagnosis.Evidence {
			if obs[evidence.ObservationID] != evidence.ObservationKind {
				return fmt.Errorf("invalid diagnosis evidence reference")
			}
		}
	}
	reportJSON, e := json.Marshal(r)
	if e != nil {
		return fmt.Errorf("encode report for validation: %w", e)
	}
	var schemaDocument, reportDocument any
	if e = json.Unmarshal(reportschema.Schema, &schemaDocument); e != nil {
		return fmt.Errorf("decode embedded report schema: %w", e)
	}
	if e = json.Unmarshal(reportJSON, &reportDocument); e != nil {
		return fmt.Errorf("decode report for validation: %w", e)
	}
	compiler := jsonschema.NewCompiler()
	const schemaURL = "urn:satoshi-redline:schema:report:v1"
	if e = compiler.AddResource(schemaURL, schemaDocument); e != nil {
		return fmt.Errorf("load report schema: %w", e)
	}
	schema, e := compiler.Compile(schemaURL)
	if e != nil {
		return fmt.Errorf("compile report schema: %w", e)
	}
	if e = schema.Validate(reportDocument); e != nil {
		return fmt.Errorf("validate report schema: %w", e)
	}
	return nil
}
func Marshal(x domain.Scan) ([]byte, error) {
	if e := x.Validate(); e != nil {
		return nil, fmt.Errorf("validate report scan: %w", e)
	}
	r := FromDomain(x)
	if e := Validate(r); e != nil {
		return nil, e
	}
	return json.MarshalIndent(r, "", "  ")
}
