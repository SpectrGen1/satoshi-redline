package v1

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/satoshi-redline/redline/internal/engine"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
)

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
	a := &Server{Runner: engine.Runner{Config: app.DefaultConfig()}, Store: s}
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
		t.Fatal(e)
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
	server := (&Server{Runner: engine.Runner{Config: app.DefaultConfig()}, Store: s}).Handler()
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
	for _, suffix := range []string{"", "/observations", "/findings"} {
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
	contractReq := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	validateAPIResponse(t, contractReq, methodRec)
	if methodRec.Header().Get("Allow") != http.MethodGet {
		t.Fatal("missing Allow header")
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
	value := EventDTO{SchemaVersion: 1, EventID: "event_1", EventType: "scan.failed", OccurredAt: time.Now().UTC().Format(time.RFC3339), ScanID: "scan_1", Payload: EventPayloadDTO{Outcome: "engine_failed"}}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err = json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(document); err != nil {
		t.Fatal(err)
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
