package v1

import (
	"encoding/json"
	"fmt"
	"github.com/satoshi-redline/redline/internal/domain"
	"time"
)

type Report struct {
	Format          string           `json:"format"`
	SchemaID        string           `json:"schema_id"`
	FormatVersion   int              `json:"format_version"`
	ReportID        string           `json:"report_id"`
	GeneratedAt     string           `json:"generated_at"`
	Producer        Producer         `json:"producer"`
	Privacy         Privacy          `json:"privacy"`
	Scan            ScanDTO          `json:"scan"`
	Coverage        CoverageDTO      `json:"coverage"`
	ProbeExecutions []ExecutionDTO   `json:"probe_executions"`
	Observations    []ObservationDTO `json:"observations"`
	Findings        []FindingDTO     `json:"findings"`
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
	ID               string           `json:"id"`
	ProbeExecutionID string           `json:"probe_execution_id"`
	ComponentID      string           `json:"component_id"`
	Kind             string           `json:"kind"`
	SchemaVersion    string           `json:"schema_version"`
	ObservedAt       string           `json:"observed_at"`
	CapabilityID     string           `json:"capability_id"`
	ProviderID       string           `json:"provider_id"`
	Sensitivity      string           `json:"sensitivity"`
	RPCTransport     *RPCTransportDTO `json:"bitcoin_rpc_transport,omitempty"`
	ChainState       *ChainDTO        `json:"bitcoin_chain_state,omitempty"`
	Filesystem       *FilesystemDTO   `json:"filesystem_capacity,omitempty"`
}
type RPCTransportDTO struct {
	Reached       bool   `json:"reached"`
	Authenticated bool   `json:"authenticated"`
	Category      string `json:"category"`
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
type ReasonDTO struct {
	Code            string `json:"code"`
	Stage           string `json:"stage"`
	Retryability    string `json:"retryability"`
	ResolutionClass string `json:"resolution_class"`
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
	ID               string            `json:"id"`
	Code             string            `json:"code"`
	ComponentID      string            `json:"component_id"`
	Status           string            `json:"status"`
	Applicability    string            `json:"applicability"`
	Severity         string            `json:"severity"`
	Confidence       string            `json:"confidence"`
	EvaluatorID      string            `json:"evaluator_id"`
	EvaluatorVersion string            `json:"evaluator_version"`
	EvaluatedAt      string            `json:"evaluated_at"`
	CoverageRole     string            `json:"coverage_role"`
	Reasons          []ReasonDTO       `json:"unknown_reasons"`
	Evidence         []EvidenceDTO     `json:"evidence"`
	Missing          []MissingDTO      `json:"missing_evidence"`
	Thresholds       domain.Thresholds `json:"thresholds"`
}

func FromDomain(x domain.Scan) Report {
	r := Report{Format: "satoshi-redline-report", SchemaID: "urn:satoshi-redline:schema:report:v1", FormatVersion: 1, ReportID: "report_" + x.ID, GeneratedAt: x.FinishedAt.UTC().Format(time.RFC3339), Producer: Producer{"SATOSHI//REDLINE", "0.1.0", "development"}, Privacy: Privacy{"shareable", true}, Scan: ScanDTO{x.ID, x.StartedAt.UTC().Format(time.RFC3339), x.FinishedAt.UTC().Format(time.RFC3339), x.Status, string(x.Summary.Health)}, Coverage: CoverageDTO{string(x.Summary.Coverage.State), x.Summary.Coverage.ApplicableRequired, x.Summary.Coverage.AssessedRequired, x.Summary.Coverage.UnknownRequired}}
	for _, p := range x.Executions {
		r.ProbeExecutions = append(r.ProbeExecutions, ExecutionDTO{p.ID, p.ProbeID, p.ProbeVersion, string(p.Outcome), p.StartedAt.UTC().Format(time.RFC3339), p.FinishedAt.UTC().Format(time.RFC3339), p.DurationMS, string(p.Limitation)})
	}
	for _, o := range x.Observations {
		d := ObservationDTO{ID: o.ID, ProbeExecutionID: o.ProbeExecutionID, ComponentID: o.ComponentID, Kind: o.Kind, SchemaVersion: o.SchemaVersion, ObservedAt: o.ObservedAt.UTC().Format(time.RFC3339), CapabilityID: o.CapabilityID, ProviderID: o.ProviderID, Sensitivity: o.Sensitivity}
		switch v := o.Value.(type) {
		case domain.RPCTransportResult:
			d.RPCTransport = &RPCTransportDTO{v.Reached, v.Authenticated, v.Category}
		case domain.BitcoinChainState:
			d.ChainState = &ChainDTO{v.Blocks, v.Headers, v.VerificationProgress, v.InitialBlockDownload, v.TipTime.UTC().Format(time.RFC3339)}
		case domain.FilesystemCapacity:
			d.Filesystem = &FilesystemDTO{v.TotalBytes, v.AvailableBytes, v.ReadOnly, v.MountPresent}
		}
		r.Observations = append(r.Observations, d)
	}
	for _, f := range x.Findings {
		d := FindingDTO{ID: f.ID, Code: f.Code, ComponentID: f.ComponentID, Status: string(f.Status), Applicability: string(f.Applicability), Severity: string(f.Severity), Confidence: string(f.Confidence), EvaluatorID: f.EvaluatorID, EvaluatorVersion: f.EvaluatorVersion, EvaluatedAt: f.EvaluatedAt.UTC().Format(time.RFC3339), CoverageRole: string(f.Role), Thresholds: f.Thresholds}
		for _, v := range f.Reasons {
			d.Reasons = append(d.Reasons, ReasonDTO{string(v.Code), string(v.Stage), string(v.Retryability), string(v.Resolution)})
		}
		for _, v := range f.Evidence {
			d.Evidence = append(d.Evidence, EvidenceDTO{v.ObservationID, v.ObservationKind, string(v.Role)})
		}
		for _, v := range f.MissingEvidence {
			d.Missing = append(d.Missing, MissingDTO{v.ObservationKind, string(v.Problem), string(v.ReasonCode)})
		}
		r.Findings = append(r.Findings, d)
	}
	return r
}
func Validate(r Report) error {
	if r.Format != "satoshi-redline-report" || r.SchemaID != "urn:satoshi-redline:schema:report:v1" || r.FormatVersion != 1 {
		return fmt.Errorf("invalid report envelope")
	}
	obs := map[string]bool{}
	for _, o := range r.Observations {
		obs[o.ID] = true
	}
	for _, f := range r.Findings {
		if f.Status == "unknown" && (len(f.Reasons) == 0 || len(f.Missing) == 0) {
			return fmt.Errorf("invalid unknown finding")
		}
		for _, e := range f.Evidence {
			if !obs[e.ObservationID] {
				return fmt.Errorf("invalid evidence reference")
			}
		}
	}
	return nil
}
func Marshal(x domain.Scan) ([]byte, error) {
	r := FromDomain(x)
	if e := Validate(r); e != nil {
		return nil, e
	}
	return json.MarshalIndent(r, "", "  ")
}
