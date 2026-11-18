package v1

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/engine"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
)

func apiTestConfig(t testing.TB) app.Config {
	t.Helper()
	cfg := app.DefaultConfig()
	cfg.BitcoinNodes = []app.BitcoinNodeConfig{{
		ID: "bitcoin.knots", Enabled: true, DisplayAlias: "Bitcoin Knots",
		RPCEndpoint: "http://127.0.0.1:1", DataDirectory: t.TempDir(),
	}}
	return cfg
}

type streamRecorder struct {
	mu     sync.Mutex
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *streamRecorder) Header() http.Header { return r.header }
func (r *streamRecorder) WriteHeader(v int)   { r.mu.Lock(); r.status = v; r.mu.Unlock() }
func (r *streamRecorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(b)
}
func (r *streamRecorder) Flush()         {}
func (r *streamRecorder) String() string { r.mu.Lock(); defer r.mu.Unlock(); return r.body.String() }

func TestScanEndpoints(t *testing.T) {
	s, err := sqlite.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := &Server{Runner: engine.Runner{Config: apiTestConfig(t)}, Store: s}
	h := a.Handler()
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/api/v1/scans", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("post status %d", r.Code)
	}
	meta := httptest.NewRecorder()
	h.ServeHTTP(meta, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))
	if meta.Code != http.StatusOK {
		t.Fatalf("meta status %d", meta.Code)
	}
}

func validateAPIResponse(t *testing.T, request *http.Request, response *httptest.ResponseRecorder) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	specPath := filepath.Join(filepath.Dir(file), "../../../schemas/api/v1/openapi.yaml")
	doc, e := openapi3.NewLoader().LoadFromFile(specPath)
	if e != nil {
		t.Fatal(e)
	}
	router, e := legacy.NewRouter(doc)
	if e != nil {
		t.Fatal(e)
	}
	route, params, e := router.FindRoute(request)
	if e != nil {
		if response.Code != http.StatusMethodNotAllowed && response.Code != http.StatusNotFound {
			t.Fatalf("request did not match contract: %v", e)
		}
		var document any
		if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
			t.Fatal(err)
		}
		if err := doc.Components.Schemas["ErrorEnvelope"].Value.VisitJSON(document); err != nil {
			t.Fatalf("error response violates OpenAPI: %v", err)
		}
		return
	}
	input := &openapi3filter.RequestValidationInput{Request: request, PathParams: params, Route: route}
	output := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: response.Code, Header: response.Header()}
	output.SetBodyBytes(response.Body.Bytes())
	if e = openapi3filter.ValidateResponse(context.Background(), output); e != nil {
		t.Fatalf("response violates OpenAPI: %v\n%s", e, response.Body.String())
	}
}

func TestActualResponsesConformToOpenAPI(t *testing.T) {
	s, e := sqlite.Open(filepath.Join(t.TempDir(), "contract.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server := (&Server{Runner: engine.Runner{Config: apiTestConfig(t)}, Store: s}).Handler()
	metaReq := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	meta := httptest.NewRecorder()
	server.ServeHTTP(meta, metaReq)
	validateAPIResponse(t, metaReq, meta)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewReader(nil))
	created := httptest.NewRecorder()
	server.ServeHTTP(created, createReq)
	validateAPIResponse(t, createReq, created)
	var scan ScanDTO
	if e = json.Unmarshal(created.Body.Bytes(), &scan); e != nil {
		t.Fatal(e)
	}
	for _, suffix := range []string{"", "/observations", "/findings", "/components", "/graph", "/diagnoses"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/"+scan.ID+suffix, nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		validateAPIResponse(t, req, rec)
	}
	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/scans/missing", nil)
	missing := httptest.NewRecorder()
	server.ServeHTTP(missing, missingReq)
	validateAPIResponse(t, missingReq, missing)
	methodReq := httptest.NewRequest(http.MethodDelete, "/api/v1/meta", nil)
	methodRec := httptest.NewRecorder()
	server.ServeHTTP(methodRec, methodReq)
	validateAPIResponse(t, methodReq, methodRec)
	if methodRec.Header().Get("Allow") != http.MethodGet {
		t.Fatal("missing Allow header")
	}
}

type fakeStore struct {
	scan    domain.Scan
	saveErr error
	getErr  error
}

func (s *fakeStore) Save(context.Context, domain.Scan) error { return s.saveErr }
func (s *fakeStore) Get(context.Context, string) (domain.Scan, error) {
	return s.scan, s.getErr
}

func apiContractScan() domain.Scan {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	execution := domain.ProbeExecution{ID: "execution", ScanID: "scan", ProbeID: "test/v1", ProbeVersion: "1.0.0", Outcome: domain.ExecutionCompleted, StartedAt: now, FinishedAt: now}
	observations := []domain.Observation{
		{ID: "rpc", ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "bitcoin.knots", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.RPCTransportResult{Reached: true, Authentication: domain.AuthenticationAccepted, Category: "valid_json_rpc_response"}},
		{ID: "chain", ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "bitcoin.knots", Kind: "bitcoin.chain.state", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.BitcoinChainState{Blocks: 1, Headers: 1, VerificationProgress: 1, TipTime: now}},
		{ID: "filesystem", ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "bitcoin.knots", Kind: "filesystem.capacity", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "filesystem.stat/v1", ProviderID: "test", Sensitivity: "none", Value: domain.FilesystemCapacity{TotalBytes: 10, AvailableBytes: 5, MountPresent: true}},
		{ID: "clock", ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "system.local", Kind: "system.clock.current", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "clock.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.ClockState{Current: now}},
		{ID: "hash", ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "bitcoin.knots", Kind: "bitcoin.chain.block-hash", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.BitcoinBlockHash{Height: 1, BlockHash: "0011111111111111111111111111111111111111111111111111111111111111"}},
		{ID: "electrum-transport", ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "electrum.electrs", Kind: "electrum.protocol.transport", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "electrum.protocol.read/v1", ProviderID: "test", Sensitivity: "local_sensitive", Value: domain.ElectrumProtocolTransport{Reached: true, Transport: domain.ElectrumTransportTCP, TLSVerification: domain.TLSNotApplicable, ProtocolNegotiation: domain.ProtocolAccepted, Category: "valid_protocol_response"}},
		{ID: "electrum-identity", ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "electrum.electrs", Kind: "electrum.server.identity", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "electrum.protocol.read/v1", ProviderID: "test", Sensitivity: "local_sensitive", Value: domain.ElectrumServerIdentity{ImplementationFamily: domain.ElectrumFulcrum, ProtocolVersion: "1.4.3"}},
		{ID: "electrum-tip", ScanID: "scan", ProbeExecutionID: "execution", ComponentID: "electrum.electrs", Kind: "electrum.chain.tip", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "electrum.protocol.read/v1", ProviderID: "test", Sensitivity: "local_sensitive", Value: domain.ElectrumChainTip{Height: 1, BlockHash: "0011111111111111111111111111111111111111111111111111111111111111", HeaderTime: now}},
	}
	assessed := domain.Finding{ID: "assessed", ScanID: "scan", Code: "bitcoin.rpc.reachable", ComponentID: "bitcoin.knots", Status: domain.Pass, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceHigh, EvaluatorID: "test", EvaluatorVersion: "1", EvaluatedAt: now, Role: domain.Required, Evidence: []domain.EvidenceRef{{ObservationID: "rpc", ObservationKind: "bitcoin.rpc.transport", Role: domain.EvidenceSupports}}}
	unknown := domain.Finding{ID: "unknown", ScanID: "scan", Code: "bitcoin.chain.synchronized", ComponentID: "bitcoin.knots", Status: domain.Unknown, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceLow, EvaluatorID: "test", EvaluatorVersion: "1", EvaluatedAt: now, Role: domain.Required, Reasons: []domain.UnknownReason{domain.Reason(domain.StaleEvidence)}, MissingEvidence: []domain.MissingEvidence{{ObservationKind: "bitcoin.chain.state", Problem: domain.EvidenceStale, ReasonCode: domain.StaleEvidence}}}
	diagnosis := domain.Diagnosis{ID: "diagnosis", ScanID: "scan", Code: "wallet-backend.electrum-diagnosis-inconclusive", Classification: domain.DiagnosisInconclusive, Confidence: domain.ConfidenceLow, SummaryKey: "diagnosis.wallet-backend.electrum-diagnosis-inconclusive", ComponentIDs: []string{"bitcoin.knots", "electrum.electrs"}, FindingIDs: []string{"assessed", "unknown"}, Evidence: []domain.EvidenceRef{{ObservationID: "rpc", ObservationKind: "bitcoin.rpc.transport", Role: domain.EvidenceContextualizes}}, BlockingUnknownFindingIDs: []string{"unknown"}, PlausibleAlternativeCodes: []string{"wallet-backend.electrum-protocol-breakpoint"}, CorrelationRuleID: "correlation.test", CorrelationRuleVersion: "1.0.0", DiagnosedAt: now}
	return domain.Scan{ID: "scan", StartedAt: now, FinishedAt: now, Status: "completed", Components: domain.FixtureComponents(true), DependencyEdges: []domain.DependencyEdge{{ID: "edge", SourceComponentID: "electrum.electrs", DestinationComponentID: "bitcoin.knots", Relationship: domain.UsesChainFrom, Provenance: domain.ProvenanceConfiguration, Confidence: domain.ConfidenceHigh, RecordedAt: now}}, Executions: []domain.ProbeExecution{execution}, Observations: observations, Findings: []domain.Finding{assessed, unknown}, Diagnoses: []domain.Diagnosis{diagnosis}, Summary: domain.ComponentSummary{ComponentID: "bitcoin.knots", Health: domain.Healthy, Coverage: domain.CoverageSummary{State: domain.Partial, ApplicableRequired: 2, AssessedRequired: 1, UnknownRequired: 1}}}
}

func TestAPIContractCoversTypedVariantsAndErrors(t *testing.T) {
	scan := apiContractScan()
	handler := (&Server{Runner: engine.Runner{Config: apiTestConfig(t)}, Store: &fakeStore{scan: scan}}).Handler()
	for _, path := range []string{"/api/v1/scans/scan", "/api/v1/scans/scan/observations", "/api/v1/scans/scan/findings", "/api/v1/scans/scan/components", "/api/v1/scans/scan/graph", "/api/v1/scans/scan/diagnoses"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		validateAPIResponse(t, request, response)
	}
	for _, path := range []string{"/api/v1/scans/scan", "/api/v1/scans/scan/observations", "/api/v1/scans/scan/findings", "/api/v1/scans/scan/components", "/api/v1/scans/scan/graph", "/api/v1/scans/scan/diagnoses"} {
		request := httptest.NewRequest(http.MethodDelete, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		validateAPIResponse(t, request, response)
	}

	unavailable := (&Server{Store: &fakeStore{getErr: errors.New("CANARY_DATABASE")}}).Handler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/scans/scan", nil)
	response := httptest.NewRecorder()
	unavailable.ServeHTTP(response, request)
	validateAPIResponse(t, request, response)

	missing := (&Server{Store: &fakeStore{getErr: sql.ErrNoRows}}).Handler()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/scans/missing", nil)
	response = httptest.NewRecorder()
	missing.ServeHTTP(response, request)
	validateAPIResponse(t, request, response)
	request = httptest.NewRequest(http.MethodGet, "/api/v1/scans/missing/extra/segment", nil)
	response = httptest.NewRecorder()
	missing.ServeHTTP(response, request)
	validateAPIResponse(t, request, response)

	failing := (&Server{Runner: engine.Runner{Config: apiTestConfig(t)}, Store: &fakeStore{saveErr: errors.New("CANARY_SAVE")}}).Handler()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/scans", nil)
	response = httptest.NewRecorder()
	failing.ServeHTTP(response, request)
	validateAPIResponse(t, request, response)
}

func TestOpenAPIRejectsInvalidObservationVariants(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "../../../schemas/api/v1/openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{"id": "o", "probe_execution_id": "p", "component_id": "c", "kind": "bitcoin.rpc.transport", "schema_version": "v1", "observed_at": "2026-01-02T03:04:05Z", "capability_id": "cap", "provider_id": "provider", "sensitivity": "none"}
	for name, mutate := range map[string]func(map[string]any){
		"no value": func(map[string]any) {},
		"two values": func(v map[string]any) {
			v["bitcoin_rpc_transport"] = map[string]any{"reached": true, "authentication": "accepted", "category": "valid_json_rpc_response"}
			v["filesystem_capacity"] = map[string]any{"total_bytes": 10, "available_bytes": 5, "read_only": false, "mount_present": true}
		},
		"mismatched value": func(v map[string]any) {
			v["filesystem_capacity"] = map[string]any{"total_bytes": 10, "available_bytes": 5, "read_only": false, "mount_present": true}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := map[string]any{}
			for key, item := range base {
				value[key] = item
			}
			mutate(value)
			if err := doc.Components.Schemas["Observation"].Value.VisitJSON(value); err == nil {
				t.Fatal("invalid observation accepted")
			}
		})
	}
}

func TestJSONSerializationFailsBeforeSuccessHeaders(t *testing.T) {
	response := httptest.NewRecorder()
	write(response, http.StatusOK, make(chan int))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	var envelope ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "response_unavailable" {
		t.Fatalf("error = %+v", envelope)
	}
}

func TestRequestIDsAreCollisionResistantAndOpaque(t *testing.T) {
	first, second := requestID(), requestID()
	if first == second || !strings.HasPrefix(first, "request_") || strings.Contains(first, "/") {
		t.Fatalf("unsafe request IDs %q %q", first, second)
	}
}

func TestSSEStreamsMultipleSafeEventsAndUnsubscribes(t *testing.T) {
	s := &Server{}
	handler := s.Handler()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	rec := &streamRecorder{header: make(http.Header)}
	done := make(chan struct{})
	go func() { handler.ServeHTTP(rec, req); close(done) }()
	deadline := time.Now().Add(time.Second)
	for {
		s.hub.mu.RLock()
		n := len(s.hub.subs)
		s.hub.mu.RUnlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subscriber not registered")
		}
		runtime.Gosched()
	}
	n := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s.hub.Emit(engine.Event{ID: "e1", Type: "scan.started", ScanID: "s", OccurredAt: n})
	s.hub.Emit(engine.Event{ID: "e2", Type: "scan.failed", ScanID: "s", OccurredAt: n, Outcome: "engine_failed"})
	deadline = time.Now().Add(time.Second)
	for !strings.Contains(rec.String(), "e2") {
		if time.Now().After(deadline) {
			t.Fatalf("events not streamed: %s", rec.String())
		}
		runtime.Gosched()
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not disconnect")
	}
	body := rec.String()
	if strings.Count(body, "event: scan.") != 2 {
		t.Fatalf("expected multiple events: %s", body)
	}
	if strings.Contains(body, "CANARY_SSE_ERROR") {
		t.Fatal("canary leaked")
	}
	s.hub.mu.RLock()
	defer s.hub.mu.RUnlock()
	if len(s.hub.subs) != 0 {
		t.Fatal("subscriber leaked")
	}
}

func TestPersistenceCanaryDoesNotEnterAPIOrSSEFailure(t *testing.T) {
	const canary = "CANARY_STORAGE_FAILURE_b491"
	server := &Server{Runner: engine.Runner{Config: apiTestConfig(t)}, Store: &fakeStore{saveErr: errors.New(canary)}}
	handler := server.Handler()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	streamRequest := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	stream := &streamRecorder{header: make(http.Header)}
	streamDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(stream, streamRequest)
		close(streamDone)
	}()
	deadline := time.Now().Add(time.Second)
	for {
		server.hub.mu.RLock()
		subscribers := len(server.hub.subs)
		server.hub.mu.RUnlock()
		if subscribers == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("SSE subscriber not registered")
		}
		runtime.Gosched()
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/scans", nil))
	deadline = time.Now().Add(time.Second)
	for !strings.Contains(stream.String(), "scan.failed") {
		if time.Now().After(deadline) {
			t.Fatalf("failure event not streamed: %s", stream.String())
		}
		runtime.Gosched()
	}
	cancel()
	<-streamDone
	if strings.Contains(response.Body.String(), canary) || strings.Contains(stream.String(), canary) {
		t.Fatal("storage error canary leaked")
	}
	if !strings.Contains(stream.String(), "persistence_failed") {
		t.Fatalf("safe outcome missing: %s", stream.String())
	}
}

func TestEventJSONConformsToSchema(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	schemaPath := filepath.Join(filepath.Dir(file), "../../../schemas/api/v1/events.schema.json")
	compiler := jsonschema.NewCompiler()
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocument any
	if err = json.Unmarshal(schemaBytes, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource("event.json", schemaDocument); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("event.json")
	if err != nil {
		t.Fatal(err)
	}
	values := []EventDTO{
		{SchemaVersion: 1, EventID: "event_1", EventType: "scan.failed", OccurredAt: time.Now().UTC().Format(time.RFC3339), ScanID: "scan_1", Payload: EventPayloadDTO{Outcome: "engine_failed"}},
		{SchemaVersion: 1, EventID: "event_2", EventType: "scan.diagnosis.created", OccurredAt: time.Now().UTC().Format(time.RFC3339), ScanID: "scan_1", Payload: EventPayloadDTO{DiagnosisCode: "wallet-backend.electrum-protocol-breakpoint", Outcome: "break_point"}},
	}
	for _, value := range values {
		encoded, encodeErr := json.Marshal(value)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		var document any
		if err = json.Unmarshal(encoded, &document); err != nil {
			t.Fatal(err)
		}
		if err = schema.Validate(document); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSSEHeartbeatsOutliveOrdinaryServerWriteTimeout(t *testing.T) {
	api := &Server{HeartbeatInterval: 10 * time.Millisecond, StreamWriteTimeout: 60 * time.Millisecond}
	httpServer := &http.Server{Handler: api.Handler(), WriteTimeout: 20 * time.Millisecond}
	listener := mustListen(t)
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+listener.Addr().String()+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	_ = httpServer.Close()
	<-done
	if bytes.Count(body, []byte(": heartbeat")) < 3 {
		t.Fatalf("stream did not remain active: %q", body)
	}
}

func mustListen(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}
