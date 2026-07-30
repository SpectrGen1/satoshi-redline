package v1

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/satoshi-redline/redline/internal/buildinfo"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/engine"
	"github.com/satoshi-redline/redline/internal/probes"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxJSONResponseBytes = 4 << 20

type ScanStore interface {
	engine.ScanStore
	Get(context.Context, string) (domain.Scan, error)
}

type Server struct {
	Runner             engine.Runner
	Store              ScanStore
	HeartbeatInterval  time.Duration
	StreamWriteTimeout time.Duration
	mu                 sync.Mutex
	hub                *eventHub
}

func (s *Server) Handler() http.Handler {
	s.mu.Lock()
	if s.hub == nil {
		s.hub = newEventHub()
	}
	s.mu.Unlock()
	m := http.NewServeMux()
	m.HandleFunc("/api/v1/meta", s.meta)
	m.HandleFunc("/api/v1/scans", s.scans)
	m.HandleFunc("/api/v1/scans/", s.scan)
	m.HandleFunc("/api/v1/events", s.eventsHandler)
	return m
}

type EmptyDetails struct{}
type ErrorBody struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id"`
	Retryable bool         `json:"retryable"`
	Details   EmptyDetails `json:"details"`
}
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

func requestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "request_unavailable"
	}
	return fmt.Sprintf("request_%x", random[:])
}
func write(w http.ResponseWriter, status int, v any) {
	encoded, err := json.Marshal(v)
	if err != nil || len(encoded) > maxJSONResponseBytes {
		encoded = []byte(`{"error":{"code":"response_unavailable","message":"The response could not be encoded.","request_id":"request_unavailable","retryable":false,"details":{}}}`)
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(encoded, '\n'))
}
func method(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method == allowed {
		return true
	}
	w.Header().Set("Allow", allowed)
	write(w, http.StatusMethodNotAllowed, ErrorEnvelope{ErrorBody{"method_not_allowed", "The requested method is not supported.", requestID(), false, EmptyDetails{}}})
	return false
}
func apiError(w http.ResponseWriter, status int, code, msg string, retry bool) {
	write(w, status, ErrorEnvelope{ErrorBody{code, msg, requestID(), retry, EmptyDetails{}}})
}

type ProbeCatalogDTO struct {
	ID                   string   `json:"probe_id"`
	Version              string   `json:"version"`
	ScanMode             string   `json:"scan_mode"`
	RequiredCapabilities []string `json:"required_capabilities"`
	OptionalCapabilities []string `json:"optional_capabilities"`
	SideEffects          []string `json:"side_effects"`
	ObservationKinds     []string `json:"observation_kinds"`
}
type MetaDTO struct {
	ProductVersion         string            `json:"product_version"`
	BuildID                string            `json:"build_id"`
	APIMajor               int               `json:"api_major"`
	APIContractRevision    string            `json:"api_contract_revision"`
	EventSchemaVersion     int               `json:"event_schema_version"`
	ReportVersionsReadable []int             `json:"report_versions_readable"`
	ReportVersionsWritable []int             `json:"report_versions_writable"`
	EnabledFeatures        []string          `json:"enabled_features"`
	CompiledProbes         []ProbeCatalogDTO `json:"compiled_probe_catalog"`
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	c, e := probes.CompiledCatalog()
	if e != nil {
		apiError(w, 500, "catalog_unavailable", "The compiled probe catalog is unavailable.", false)
		return
	}
	d := MetaDTO{buildinfo.Version, buildinfo.BuildID, 1, "2", 1, []int{1}, []int{1}, []string{"bitcoin_rpc", "filesystem_capacity", "electrum_protocol", "dependency_graph", "diagnoses"}, nil}
	for _, p := range c.Probes() {
		x := p.Descriptor()
		v := ProbeCatalogDTO{ID: x.ID, Version: x.Version, ScanMode: string(x.Mode), RequiredCapabilities: []string{}, OptionalCapabilities: []string{}, SideEffects: []string{}, ObservationKinds: append([]string{}, x.ObservationKinds...)}
		for _, id := range x.Required {
			v.RequiredCapabilities = append(v.RequiredCapabilities, string(id))
		}
		for _, id := range x.Optional {
			v.OptionalCapabilities = append(v.OptionalCapabilities, string(id))
		}
		for _, se := range x.SideEffects {
			v.SideEffects = append(v.SideEffects, string(se))
		}
		d.CompiledProbes = append(d.CompiledProbes, v)
	}
	write(w, 200, d)
}
func (s *Server) scans(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	runner := s.Runner
	runner.Events = s.hub
	x, e := runner.ScanAndPersist(r.Context(), s.Store)
	if e != nil {
		apiError(w, 500, "scan_failed", "The scan could not be completed.", true)
		return
	}
	write(w, 200, scanDTO(x))
}
func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	tail := strings.TrimPrefix(r.URL.Path, "/api/v1/scans/")
	parts := strings.Split(tail, "/")
	if len(parts) < 1 || parts[0] == "" || len(parts) > 2 {
		apiError(w, 404, "route_not_found", "The requested API route does not exist.", false)
		return
	}
	if len(parts) == 2 && parts[1] != "observations" && parts[1] != "findings" && parts[1] != "components" && parts[1] != "graph" && parts[1] != "diagnoses" {
		apiError(w, 404, "route_not_found", "The requested API route does not exist.", false)
		return
	}
	x, e := s.Store.Get(r.Context(), parts[0])
	if e == sql.ErrNoRows {
		apiError(w, 404, "scan_not_found", "The requested scan does not exist.", false)
		return
	}
	if e != nil {
		apiError(w, 500, "scan_unavailable", "The requested scan is unavailable.", true)
		return
	}
	if len(parts) == 1 {
		write(w, 200, scanDTO(x))
		return
	}
	if parts[1] == "observations" {
		write(w, 200, observationsDTO(x.Observations))
		return
	}
	if parts[1] == "findings" {
		write(w, 200, findingsDTO(x.Findings))
		return
	}
	if parts[1] == "components" {
		write(w, 200, componentsDTO(x.Components))
		return
	}
	if parts[1] == "graph" {
		write(w, 200, GraphDTO{Components: componentsDTO(x.Components), DependencyEdges: edgesDTO(x.DependencyEdges)})
		return
	}
	write(w, 200, diagnosesDTO(x.Diagnoses))
}

type CoverageDTO struct {
	State              string `json:"state"`
	ApplicableRequired int    `json:"applicable_required"`
	AssessedRequired   int    `json:"assessed_required"`
	UnknownRequired    int    `json:"unknown_required"`
}
type ScanDTO struct {
	ID         string      `json:"id"`
	StartedAt  string      `json:"started_at"`
	FinishedAt string      `json:"finished_at"`
	Status     string      `json:"status"`
	Health     string      `json:"health"`
	Coverage   CoverageDTO `json:"coverage"`
}

func scanDTO(x domain.Scan) ScanDTO {
	return ScanDTO{x.ID, x.StartedAt.UTC().Format(time.RFC3339), x.FinishedAt.UTC().Format(time.RFC3339), x.Status, string(x.Summary.Health), CoverageDTO{string(x.Summary.Coverage.State), x.Summary.Coverage.ApplicableRequired, x.Summary.Coverage.AssessedRequired, x.Summary.Coverage.UnknownRequired}}
}

type RPCValue struct {
	Reached        bool   `json:"reached"`
	Authentication string `json:"authentication"`
	Category       string `json:"category"`
}
type ChainValue struct {
	Blocks               int64   `json:"blocks"`
	Headers              int64   `json:"headers"`
	VerificationProgress float64 `json:"verification_progress"`
	InitialBlockDownload bool    `json:"initial_block_download"`
	TipTime              string  `json:"tip_time"`
}
type FSValue struct {
	TotalBytes     int64 `json:"total_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	ReadOnly       bool  `json:"read_only"`
	MountPresent   bool  `json:"mount_present"`
}
type ClockValue struct {
	CurrentTime string `json:"current_time"`
}
type BitcoinBlockHashValue struct {
	Height    int64  `json:"height"`
	BlockHash string `json:"block_hash"`
}
type ElectrumTransportValue struct {
	Reached             bool   `json:"reached"`
	TransportKind       string `json:"transport_kind"`
	TLSVerification     string `json:"tls_verification"`
	ProtocolNegotiation string `json:"protocol_negotiation"`
	Category            string `json:"category"`
}
type ElectrumIdentityValue struct {
	ImplementationFamily string `json:"implementation_family"`
	ProtocolVersion      string `json:"protocol_version"`
}
type ElectrumTipValue struct {
	Height     int64  `json:"height"`
	BlockHash  string `json:"block_hash"`
	HeaderTime string `json:"header_time"`
}
type ObservationDTO struct {
	ID                string                  `json:"id"`
	ProbeExecutionID  string                  `json:"probe_execution_id"`
	ComponentID       string                  `json:"component_id"`
	Kind              string                  `json:"kind"`
	SchemaVersion     string                  `json:"schema_version"`
	ObservedAt        string                  `json:"observed_at"`
	CapabilityID      string                  `json:"capability_id"`
	ProviderID        string                  `json:"provider_id"`
	Sensitivity       string                  `json:"sensitivity"`
	RPC               *RPCValue               `json:"bitcoin_rpc_transport,omitempty"`
	Chain             *ChainValue             `json:"bitcoin_chain_state,omitempty"`
	Filesystem        *FSValue                `json:"filesystem_capacity,omitempty"`
	Clock             *ClockValue             `json:"clock_state,omitempty"`
	BitcoinBlockHash  *BitcoinBlockHashValue  `json:"bitcoin_block_hash,omitempty"`
	ElectrumTransport *ElectrumTransportValue `json:"electrum_protocol_transport,omitempty"`
	ElectrumIdentity  *ElectrumIdentityValue  `json:"electrum_server_identity,omitempty"`
	ElectrumTip       *ElectrumTipValue       `json:"electrum_chain_tip,omitempty"`
}

func observationsDTO(a []domain.Observation) []ObservationDTO {
	out := make([]ObservationDTO, 0, len(a))
	for _, o := range a {
		d := ObservationDTO{ID: o.ID, ProbeExecutionID: o.ProbeExecutionID, ComponentID: o.ComponentID, Kind: o.Kind, SchemaVersion: o.SchemaVersion, ObservedAt: o.ObservedAt.UTC().Format(time.RFC3339), CapabilityID: o.CapabilityID, ProviderID: o.ProviderID, Sensitivity: o.Sensitivity}
		switch v := o.Value.(type) {
		case domain.RPCTransportResult:
			d.RPC = &RPCValue{v.Reached, string(v.Authentication), v.Category}
		case domain.BitcoinChainState:
			d.Chain = &ChainValue{v.Blocks, v.Headers, v.VerificationProgress, v.InitialBlockDownload, v.TipTime.UTC().Format(time.RFC3339)}
		case domain.FilesystemCapacity:
			d.Filesystem = &FSValue{v.TotalBytes, v.AvailableBytes, v.ReadOnly, v.MountPresent}
		case domain.ClockState:
			d.Clock = &ClockValue{v.Current.UTC().Format(time.RFC3339)}
		case domain.BitcoinBlockHash:
			d.BitcoinBlockHash = &BitcoinBlockHashValue{v.Height, v.BlockHash}
		case domain.ElectrumProtocolTransport:
			d.ElectrumTransport = &ElectrumTransportValue{v.Reached, string(v.Transport), string(v.TLSVerification), string(v.ProtocolNegotiation), v.Category}
		case domain.ElectrumServerIdentity:
			d.ElectrumIdentity = &ElectrumIdentityValue{string(v.ImplementationFamily), v.ProtocolVersion}
		case domain.ElectrumChainTip:
			d.ElectrumTip = &ElectrumTipValue{v.Height, v.BlockHash, v.HeaderTime.UTC().Format(time.RFC3339)}
		}
		out = append(out, d)
	}
	return out
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

func findingsDTO(a []domain.Finding) []FindingDTO {
	out := make([]FindingDTO, 0, len(a))
	for _, f := range a {
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
		out = append(out, d)
	}
	return out
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

func componentsDTO(components []domain.Component) []ComponentDTO {
	out := make([]ComponentDTO, 0, len(components))
	for _, component := range components {
		out = append(out, ComponentDTO{component.ID, string(component.Kind), string(component.ImplementationFamily), component.ImplementationVersion, component.DisplayAlias, string(component.Provenance), string(component.State)})
	}
	return out
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

func edgesDTO(edges []domain.DependencyEdge) []DependencyEdgeDTO {
	out := make([]DependencyEdgeDTO, 0, len(edges))
	for _, edge := range edges {
		out = append(out, DependencyEdgeDTO{edge.ID, edge.SourceComponentID, edge.DestinationComponentID, string(edge.Relationship), string(edge.Provenance), string(edge.Confidence), edge.RecordedAt.UTC().Format(time.RFC3339)})
	}
	return out
}

type GraphDTO struct {
	Components      []ComponentDTO      `json:"components"`
	DependencyEdges []DependencyEdgeDTO `json:"dependency_edges"`
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

func diagnosesDTO(diagnoses []domain.Diagnosis) []DiagnosisDTO {
	out := make([]DiagnosisDTO, 0, len(diagnoses))
	for _, diagnosis := range diagnoses {
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
		out = append(out, dto)
	}
	return out
}

type EventPayloadDTO struct {
	ProbeID       string `json:"probe_id,omitempty"`
	FindingCode   string `json:"finding_code,omitempty"`
	DiagnosisCode string `json:"diagnosis_code,omitempty"`
	Outcome       string `json:"outcome,omitempty"`
}
type EventDTO struct {
	SchemaVersion int             `json:"event_schema_version"`
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	OccurredAt    string          `json:"occurred_at"`
	ScanID        string          `json:"scan_id,omitempty"`
	Payload       EventPayloadDTO `json:"payload"`
}

func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		apiError(w, 500, "stream_unavailable", "The event stream is unavailable.", false)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	id, ch := s.hub.subscribe()
	defer s.hub.unsubscribe(id)
	heartbeatInterval := s.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = 15 * time.Second
	}
	writeTimeout := s.StreamWriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 10 * time.Second
	}
	controller := http.NewResponseController(w)
	setDeadline := func() bool {
		err := controller.SetWriteDeadline(time.Now().Add(writeTimeout))
		return err == nil || errors.Is(err, http.ErrNotSupported)
	}
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case e := <-ch:
			data, err := json.Marshal(EventDTO{1, e.ID, e.Type, e.OccurredAt.UTC().Format(time.RFC3339), e.ScanID, EventPayloadDTO{e.ProbeID, e.FindingCode, e.DiagnosisCode, e.Outcome}})
			if err != nil || !setDeadline() {
				return
			}
			if _, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", e.ID, e.Type, data); err != nil {
				return
			}
			f.Flush()
		case <-heartbeat.C:
			if !setDeadline() {
				return
			}
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
