package livebitcoin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	apiv1 "github.com/satoshi-redline/redline/internal/api/v1"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/engine"
	reportv1 "github.com/satoshi-redline/redline/internal/reports/v1"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
)

const liveCanary = "REDLINE_LIVE_LOCAL_CANARY_7e69c1f0"

func TestLiveBitcoinScan(t *testing.T) {
	if os.Getenv("REDLINE_LIVE_TEST") != "1" {
		t.Skip("set REDLINE_LIVE_TEST=1 and all live target variables to opt in")
	}
	endpoint := requiredLiveSetting(t, "REDLINE_LIVE_RPC_ENDPOINT")
	cookieFile := requiredLiveSetting(t, "REDLINE_LIVE_COOKIE_FILE")
	dataDirectory := requiredLiveSetting(t, "REDLINE_LIVE_DATA_DIRECTORY")

	databasePath := filepath.Join(t.TempDir(), "live-smoke.db")
	config := app.DefaultConfig()
	config.Bitcoin.RPCEndpoint = endpoint
	config.Bitcoin.CookieFile = cookieFile
	config.Bitcoin.DataDirectory = dataDirectory
	config.DatabasePath = databasePath
	config.API.ListenAddress = liveCanary
	if err := config.Validate(); err != nil {
		t.Fatal("live test configuration did not validate")
	}

	store, err := sqlite.Open(databasePath)
	if err != nil {
		t.Fatal("could not open temporary live-test database")
	}
	defer store.Close()

	scanned, err := (engine.Runner{Config: config}).ScanAndPersist(context.Background(), store)
	if err != nil {
		t.Fatal("live scan-and-persist workflow failed")
	}
	if err := scanned.Validate(); err != nil {
		t.Fatal("live scan produced an invalid diagnostic graph")
	}
	loaded, err := store.Get(context.Background(), scanned.ID)
	if err != nil {
		t.Fatal("persisted live scan could not be loaded")
	}
	if err := loaded.Validate(); err != nil {
		t.Fatal("loaded live scan produced an invalid diagnostic graph")
	}

	assertExpectedEvidence(t, loaded)
	assertEvidenceRoundTrip(t, scanned, loaded)

	reportJSON, err := reportv1.Marshal(loaded)
	if err != nil {
		t.Fatal("live report export or validation failed")
	}
	var report reportv1.Report
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		t.Fatal("validated live report could not be decoded")
	}
	if err := reportv1.Validate(report); err != nil {
		t.Fatal("live report failed explicit validation")
	}
	assertReportEvidence(t, loaded, report)

	apiJSON := exerciseAPI(t, store, loaded.ID)
	forbidden := []string{endpoint, cookieFile, dataDirectory, liveCanary}
	assertAbsent(t, "report JSON", reportJSON, forbidden)
	assertAbsent(t, "API JSON", apiJSON, forbidden)

	if err := store.Close(); err != nil {
		t.Fatal("could not close temporary live-test database")
	}
	databaseBytes, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatal("could not inspect temporary live-test database")
	}
	assertAbsent(t, "SQLite database", databaseBytes, forbidden)

	summary := sanitizedSummary(loaded)
	assertAbsent(t, "sanitized live-test summary", []byte(summary), forbidden)
	t.Log(summary)
}

func requiredLiveSetting(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("required opt-in environment variable %s is not set", name)
	}
	return value
}

func assertExpectedEvidence(t *testing.T, scan domain.Scan) {
	t.Helper()
	expectedFindings := map[string]bool{
		"bitcoin.rpc.reachable":        false,
		"bitcoin.chain.synchronized":   false,
		"bitcoin.chain.tip-freshness":  false,
		"filesystem.capacity-pressure": false,
	}
	observations := map[string]bool{}
	transportCategory := "absent"
	for _, observation := range scan.Observations {
		observations[observation.Kind] = true
		if value, ok := observation.Value.(domain.RPCTransportResult); ok {
			transportCategory = value.Category
			if !value.Reached || value.Authentication != domain.AuthenticationAccepted {
				t.Fatal("live RPC transport evidence did not prove an authenticated reached endpoint")
			}
		}
	}
	for _, finding := range scan.Findings {
		if _, ok := expectedFindings[finding.Code]; ok {
			expectedFindings[finding.Code] = true
		}
		if finding.Status == domain.Unknown {
			if len(finding.Reasons) == 0 || len(finding.MissingEvidence) == 0 {
				t.Fatal("live unknown finding lacked structured reasons or missing evidence")
			}
		} else if len(finding.Evidence) == 0 {
			t.Fatal("live assessed finding lacked supporting evidence")
		}
	}
	for code, present := range expectedFindings {
		if !present {
			t.Fatalf("expected finding code %s was absent", code)
		}
	}
	for _, kind := range []string{"bitcoin.rpc.transport", "bitcoin.chain.state", "filesystem.capacity"} {
		if !observations[kind] {
			if kind == "bitcoin.chain.state" {
				t.Fatalf("expected typed observation kind %s was absent (transport category %s)", kind, transportCategory)
			}
			t.Fatalf("expected typed observation kind %s was absent", kind)
		}
	}
}

func assertEvidenceRoundTrip(t *testing.T, before, after domain.Scan) {
	t.Helper()
	beforeEvidence := evidenceSet(before)
	afterEvidence := evidenceSet(after)
	if len(beforeEvidence) != len(afterEvidence) {
		t.Fatal("persistence round trip changed the evidence reference count")
	}
	for key := range beforeEvidence {
		if !afterEvidence[key] {
			t.Fatal("persistence round trip changed an evidence reference")
		}
	}
}

func evidenceSet(scan domain.Scan) map[string]bool {
	result := make(map[string]bool)
	for _, finding := range scan.Findings {
		for _, evidence := range finding.Evidence {
			key := finding.ID + "\x00" + evidence.ObservationID + "\x00" + evidence.ObservationKind + "\x00" + string(evidence.Role)
			result[key] = true
		}
	}
	return result
}

func assertReportEvidence(t *testing.T, scan domain.Scan, report reportv1.Report) {
	t.Helper()
	expected := evidenceSet(scan)
	actual := make(map[string]bool)
	for _, finding := range report.Findings {
		for _, evidence := range finding.Evidence {
			key := finding.ID + "\x00" + evidence.ObservationID + "\x00" + evidence.ObservationKind + "\x00" + evidence.Role
			actual[key] = true
		}
	}
	if len(expected) != len(actual) {
		t.Fatal("report projection changed the evidence reference count")
	}
	for key := range expected {
		if !actual[key] {
			t.Fatal("report projection changed an evidence reference")
		}
	}
}

func exerciseAPI(t *testing.T, store *sqlite.Store, scanID string) []byte {
	t.Helper()
	handler := (&apiv1.Server{Store: store}).Handler()
	_, sourceFile, _, _ := runtime.Caller(0)
	specPath := filepath.Join(filepath.Dir(sourceFile), "../../schemas/api/v1/openapi.yaml")
	document, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		t.Fatal("could not load the API contract for the live test")
	}
	router, err := legacy.NewRouter(document)
	if err != nil {
		t.Fatal("could not route the API contract for the live test")
	}
	var combined bytes.Buffer
	for _, path := range []string{
		"/api/v1/meta",
		"/api/v1/scans/" + scanID,
		"/api/v1/scans/" + scanID + "/observations",
		"/api/v1/scans/" + scanID + "/findings",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatal("live API handler did not return a successful typed response")
		}
		if !json.Valid(response.Body.Bytes()) {
			t.Fatal("live API handler returned invalid JSON")
		}
		route, pathParameters, err := router.FindRoute(request)
		if err != nil {
			t.Fatal("live API request did not match the API contract")
		}
		input := &openapi3filter.RequestValidationInput{Request: request, PathParams: pathParameters, Route: route}
		output := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: response.Code, Header: response.Header()}
		output.SetBodyBytes(response.Body.Bytes())
		if err := openapi3filter.ValidateResponse(context.Background(), output); err != nil {
			t.Fatal("live API response did not conform to OpenAPI")
		}
		combined.Write(response.Body.Bytes())
	}
	return combined.Bytes()
}

func assertAbsent(t *testing.T, location string, content []byte, forbidden []string) {
	t.Helper()
	for _, value := range forbidden {
		if value != "" && bytes.Contains(content, []byte(value)) {
			t.Fatalf("%s contained a forbidden live configuration value", location)
		}
	}
}

func sanitizedSummary(scan domain.Scan) string {
	findings := make([]string, 0, len(scan.Findings))
	for _, finding := range scan.Findings {
		findings = append(findings, finding.Code+"="+string(finding.Status))
	}
	sort.Strings(findings)
	executions := make([]string, 0, len(scan.Executions))
	for _, execution := range scan.Executions {
		executions = append(executions, execution.ProbeID+"="+string(execution.Outcome))
	}
	sort.Strings(executions)
	return "live scan health=" + string(scan.Summary.Health) +
		" coverage=" + string(scan.Summary.Coverage.State) +
		" findings=[" + strings.Join(findings, ",") + "]" +
		" executions=[" + strings.Join(executions, ",") + "]" +
		" report_validation=passed"
}
