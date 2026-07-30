package v1

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/engine"
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
	events []chan string
}

func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/api/v1/meta", s.meta)
	m.HandleFunc("/api/v1/scans", s.scans)
	m.HandleFunc("/api/v1/scans/", s.scan)
	m.HandleFunc("/api/v1/events", s.eventsHandler)
	return m
}
func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	write(w, map[string]string{"api_version": "v1", "service": "redlined"})
}
func (s *Server) scans(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	s.notify("started")
	x, e := s.Runner.ScanAndPersist(r.Context(), s.Store)
	if e != nil {
		http.Error(w, "scan could not be completed", 500)
		return
	}
	s.notify("completed:" + x.ID)
	write(w, scanDTO(x))
}
func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, "/api/v1/scans/")
	parts := strings.Split(tail, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	x, e := s.Store.Get(r.Context(), parts[0])
	if e == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		http.Error(w, "scan unavailable", 500)
		return
	}
	if len(parts) == 1 {
		write(w, scanDTO(x))
		return
	}
	switch parts[1] {
	case "observations":
		write(w, observationsDTO(x.Observations))
	case "findings":
		write(w, findingsDTO(x.Findings))
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := make(chan string, 4)
	s.mu.Lock()
	s.events = append(s.events, ch)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		for i, v := range s.events {
			if v == ch {
				s.events = append(s.events[:i], s.events[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
	}()
	select {
	case m := <-ch:
		_, _ = w.Write([]byte("event: scan\ndata: " + m + "\n\n"))
		f.Flush()
	case <-r.Context().Done():
	}
}
func (s *Server) notify(m string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.events {
		select {
		case c <- m:
		default:
		}
	}
}

type ScanResponse struct {
	ID         string           `json:"id"`
	StartedAt  string           `json:"started_at"`
	FinishedAt string           `json:"finished_at"`
	Health     string           `json:"health"`
	Coverage   CoverageResponse `json:"coverage"`
}
type CoverageResponse struct {
	State                                                 string `json:"state"`
	ApplicableRequired, AssessedRequired, UnknownRequired int
}

func scanDTO(x domain.Scan) ScanResponse {
	return ScanResponse{x.ID, x.StartedAt.UTC().Format(time.RFC3339), x.FinishedAt.UTC().Format(time.RFC3339), string(x.Summary.Health), CoverageResponse{string(x.Summary.Coverage.State), x.Summary.Coverage.ApplicableRequired, x.Summary.Coverage.AssessedRequired, x.Summary.Coverage.UnknownRequired}}
}

type ObservationResponse struct {
	ID               string `json:"id"`
	ProbeExecutionID string `json:"probe_execution_id"`
	ComponentID      string `json:"component_id"`
	Kind             string `json:"kind"`
	SchemaVersion    string `json:"schema_version"`
	ObservedAt       string `json:"observed_at"`
	CapabilityID     string `json:"capability_id"`
	ProviderID       string `json:"provider_id"`
	Sensitivity      string `json:"sensitivity"`
	Value            any    `json:"value"`
}
type RPCTransportValue struct {
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
type FilesystemValue struct {
	TotalBytes     int64 `json:"total_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	ReadOnly       bool  `json:"read_only"`
	MountPresent   bool  `json:"mount_present"`
}
type FindingResponse struct {
	ID               string                   `json:"id"`
	Code             string                   `json:"code"`
	Status           string                   `json:"status"`
	Applicability    string                   `json:"applicability"`
	Severity         string                   `json:"severity"`
	Confidence       string                   `json:"confidence"`
	EvaluatorID      string                   `json:"evaluator_id"`
	EvaluatorVersion string                   `json:"evaluator_version"`
	EvaluatedAt      string                   `json:"evaluated_at"`
	CoverageRole     string                   `json:"coverage_role"`
	Reasons          []domain.UnknownReason   `json:"unknown_reasons"`
	Evidence         []domain.EvidenceRef     `json:"evidence"`
	Missing          []domain.MissingEvidence `json:"missing_evidence"`
	Thresholds       domain.Thresholds        `json:"thresholds"`
}

func observationsDTO(a []domain.Observation) []ObservationResponse {
	out := make([]ObservationResponse, 0, len(a))
	for _, o := range a {
		d := ObservationResponse{ID: o.ID, ProbeExecutionID: o.ProbeExecutionID, ComponentID: o.ComponentID, Kind: o.Kind, SchemaVersion: o.SchemaVersion, ObservedAt: o.ObservedAt.UTC().Format(time.RFC3339), CapabilityID: o.CapabilityID, ProviderID: o.ProviderID, Sensitivity: o.Sensitivity}
		switch v := o.Value.(type) {
		case domain.RPCTransportResult:
			d.Value = RPCTransportValue{v.Reached, v.Authenticated, v.Category}
		case domain.BitcoinChainState:
			d.Value = ChainValue{v.Blocks, v.Headers, v.VerificationProgress, v.InitialBlockDownload, v.TipTime.UTC().Format(time.RFC3339)}
		case domain.FilesystemCapacity:
			d.Value = FilesystemValue{v.TotalBytes, v.AvailableBytes, v.ReadOnly, v.MountPresent}
		}
		out = append(out, d)
	}
	return out
}
func findingsDTO(a []domain.Finding) []FindingResponse {
	out := make([]FindingResponse, 0, len(a))
	for _, f := range a {
		out = append(out, FindingResponse{f.ID, f.Code, string(f.Status), string(f.Applicability), string(f.Severity), string(f.Confidence), f.EvaluatorID, f.EvaluatorVersion, f.EvaluatedAt.UTC().Format(time.RFC3339), string(f.Role), f.Reasons, f.Evidence, f.MissingEvidence, f.Thresholds})
	}
	return out
}

var _ context.Context
