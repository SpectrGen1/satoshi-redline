package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/engine"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
)

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
