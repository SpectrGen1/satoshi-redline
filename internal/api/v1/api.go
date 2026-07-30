package v1

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/satoshi-redline/redline/internal/buildinfo"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/engine"
	"github.com/satoshi-redline/redline/internal/probes"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Runner engine.Runner
	Store  *sqlite.Store
	mu     sync.Mutex
	hub    *eventHub
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

func requestID() string { return fmt.Sprintf("request_%d", time.Now().UnixNano()) }
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
	EventSchemaVersion     string            `json:"event_schema_version"`
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
	d := MetaDTO{buildinfo.Version, buildinfo.BuildID, 1, "1", "v1", []int{1}, []int{1}, []string{"bitcoin_rpc", "filesystem_capacity"}, nil}
	for _, p := range c.Probes() {
		x := p.Descriptor()
		v := ProbeCatalogDTO{ID: x.ID, Version: x.Version, ScanMode: string(x.Mode), ObservationKinds: append([]string(nil), x.ObservationKinds...)}
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
	if len(parts) == 2 && parts[1] != "observations" && parts[1] != "findings" {
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
	write(w, 200, findingsDTO(x.Findings))
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
	Reached       bool   `json:"reached"`
	Authenticated bool   `json:"authenticated"`
	Category      string `json:"category"`
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
type ObservationDTO struct {
	ID               string      `json:"id"`
	ProbeExecutionID string      `json:"probe_execution_id"`
	ComponentID      string      `json:"component_id"`
	Kind             string      `json:"kind"`
	SchemaVersion    string      `json:"schema_version"`
	ObservedAt       string      `json:"observed_at"`
	CapabilityID     string      `json:"capability_id"`
	ProviderID       string      `json:"provider_id"`
	Sensitivity      string      `json:"sensitivity"`
	RPC              *RPCValue   `json:"bitcoin_rpc_transport,omitempty"`
	Chain            *ChainValue `json:"bitcoin_chain_state,omitempty"`
	Filesystem       *FSValue    `json:"filesystem_capacity,omitempty"`
}

func observationsDTO(a []domain.Observation) []ObservationDTO {
	out := make([]ObservationDTO, 0, len(a))
	for _, o := range a {
		d := ObservationDTO{ID: o.ID, ProbeExecutionID: o.ProbeExecutionID, ComponentID: o.ComponentID, Kind: o.Kind, SchemaVersion: o.SchemaVersion, ObservedAt: o.ObservedAt.UTC().Format(time.RFC3339), CapabilityID: o.CapabilityID, ProviderID: o.ProviderID, Sensitivity: o.Sensitivity}
		switch v := o.Value.(type) {
		case domain.RPCTransportResult:
			d.RPC = &RPCValue{v.Reached, v.Authenticated, v.Category}
		case domain.BitcoinChainState:
			d.Chain = &ChainValue{v.Blocks, v.Headers, v.VerificationProgress, v.InitialBlockDownload, v.TipTime.UTC().Format(time.RFC3339)}
		case domain.FilesystemCapacity:
			d.Filesystem = &FSValue{v.TotalBytes, v.AvailableBytes, v.ReadOnly, v.MountPresent}
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
	MaxTipAgeSeconds        int64   `json:"max_tip_age_seconds"`
	MinVerificationProgress float64 `json:"min_verification_progress"`
	MaxHeaderLag            int64   `json:"max_header_lag"`
	WarningAvailablePercent float64 `json:"warning_available_percent"`
	FailureAvailablePercent float64 `json:"failure_available_percent"`
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
		d := FindingDTO{ID: f.ID, Code: f.Code, ComponentID: f.ComponentID, Status: string(f.Status), Applicability: string(f.Applicability), Severity: string(f.Severity), Confidence: string(f.Confidence), EvaluatorID: f.EvaluatorID, EvaluatorVersion: f.EvaluatorVersion, EvaluatedAt: f.EvaluatedAt.UTC().Format(time.RFC3339), CoverageRole: string(f.Role), Thresholds: ThresholdDTO{f.Thresholds.MaxTipAgeSeconds, f.Thresholds.MinVerificationProgress, f.Thresholds.MaxHeaderLag, f.Thresholds.WarningAvailablePercent, f.Thresholds.FailureAvailablePercent}}
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

type EventPayloadDTO struct {
	ProbeID     string `json:"probe_id,omitempty"`
	FindingCode string `json:"finding_code,omitempty"`
	Outcome     string `json:"outcome,omitempty"`
}
type EventDTO struct {
	SchemaVersion string          `json:"event_schema_version"`
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
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case e := <-ch:
			data, err := json.Marshal(EventDTO{"v1", e.ID, e.Type, e.OccurredAt.UTC().Format(time.RFC3339), e.ScanID, EventPayloadDTO{e.ProbeID, e.FindingCode, e.Outcome}})
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", e.ID, e.Type, data)
			f.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
