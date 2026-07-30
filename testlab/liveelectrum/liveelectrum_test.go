package liveelectrum_test

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

const localCanary = "REDLINE_LIVE_ELECTRUM_LOCAL_CANARY_0841"

func TestLiveElectrumScan(t *testing.T) {
	settings := []string{
		"REDLINE_LIVE_ELECTRUM_TEST", "REDLINE_LIVE_RPC_ENDPOINT", "REDLINE_LIVE_COOKIE_FILE",
		"REDLINE_LIVE_DATA_DIRECTORY", "REDLINE_LIVE_ELECTRUM_ENDPOINT",
	}
	for _, name := range settings {
		if os.Getenv(name) == "" {
			t.Skip("all live Electrum opt-in variables are required")
		}
	}
	if os.Getenv("REDLINE_LIVE_ELECTRUM_TEST") != "1" {
		t.Skip("set REDLINE_LIVE_ELECTRUM_TEST=1 to opt in")
	}
	rpcEndpoint := os.Getenv("REDLINE_LIVE_RPC_ENDPOINT")
	cookieFile := os.Getenv("REDLINE_LIVE_COOKIE_FILE")
	dataDirectory := os.Getenv("REDLINE_LIVE_DATA_DIRECTORY")
	electrumEndpoint := os.Getenv("REDLINE_LIVE_ELECTRUM_ENDPOINT")

	databasePath := filepath.Join(t.TempDir(), "live-electrum.db")
	config := app.DefaultConfig()
	config.Bitcoin.RPCEndpoint = rpcEndpoint
	config.Bitcoin.CookieFile = cookieFile
	config.Bitcoin.DataDirectory = dataDirectory
	config.DatabasePath = databasePath
	config.Electrum.Enabled = true
	config.Electrum.Endpoint = electrumEndpoint
	config.Electrum.BitcoinComponentID = "bitcoin.primary"
	config.API.ListenAddress = localCanary
	if err := config.Validate(); err != nil {
		t.Fatal("live Electrum configuration did not validate")
	}

	store, err := sqlite.Open(databasePath)
	if err != nil {
		t.Fatal("could not open temporary live Electrum database")
	}
	defer store.Close()
	scanned, err := (engine.Runner{Config: config}).ScanAndPersist(context.Background(), store)
	if err != nil {
		t.Fatal("live Electrum scan-and-persist failed")
	}
	if err = scanned.Validate(); err != nil {
		t.Fatal("live Electrum scan graph was invalid")
	}
	loaded, err := store.Get(context.Background(), scanned.ID)
	if err != nil || loaded.Validate() != nil {
		t.Fatal("live Electrum persisted graph was invalid")
	}
	assertGraph(t, loaded)
	assertRoundTrip(t, scanned, loaded)

	reportJSON, err := reportv1.Marshal(loaded)
	if err != nil {
		t.Fatal("live Electrum report validation failed")
	}
	var report reportv1.Report
	if json.Unmarshal(reportJSON, &report) != nil || reportv1.Validate(report) != nil {
		t.Fatal("live Electrum report round trip failed")
	}
	apiJSON := exerciseAPI(t, store, loaded.ID, config)
	forbidden := []string{rpcEndpoint, cookieFile, dataDirectory, electrumEndpoint, localCanary}
	assertAbsent(t, "report", reportJSON, forbidden)
	assertAbsent(t, "API", apiJSON, forbidden)
	if err = store.Close(); err != nil {
		t.Fatal("could not close live Electrum database")
	}
	databaseBytes, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatal("could not inspect live Electrum database")
	}
	assertAbsent(t, "SQLite", databaseBytes, forbidden)
	summary := sanitizedSummary(loaded)
	assertAbsent(t, "summary", []byte(summary), forbidden)
	t.Log(summary)
}

func assertGraph(t *testing.T, scan domain.Scan) {
	t.Helper()
	expectedFindings := map[string]bool{
		"bitcoin.rpc.reachable": false, "bitcoin.chain.synchronized": false,
		"bitcoin.chain.tip-freshness": false, "filesystem.capacity-pressure": false,
		"electrum.protocol.reachable": false, "electrum.protocol.compatible": false,
		"electrum.chain.tip-freshness": false, "electrum.chain.sync-lag": false,
		"electrum.chain.matches-bitcoin": false,
	}
	expectedObservations := map[string]bool{
		"bitcoin.rpc.transport": false, "bitcoin.chain.state": false,
		"bitcoin.chain.block-hash": false, "filesystem.capacity": false,
		"electrum.protocol.transport": false, "electrum.server.identity": false,
		"electrum.chain.tip": false,
	}
	bound := false
	for _, edge := range scan.DependencyEdges {
		if edge.SourceComponentID == "electrum.primary" && edge.DestinationComponentID == "bitcoin.primary" && edge.Relationship == domain.UsesChainFrom {
			bound = true
		}
	}
	if !bound {
		t.Fatal("configured Electrum dependency edge was absent")
	}
	for _, observation := range scan.Observations {
		if _, expected := expectedObservations[observation.Kind]; expected {
			expectedObservations[observation.Kind] = true
		}
		if transport, ok := observation.Value.(domain.ElectrumProtocolTransport); ok && (!transport.Reached || transport.ProtocolNegotiation != domain.ProtocolAccepted) {
			t.Fatal("live Electrum transport did not establish compatible protocol access")
		}
	}
	for _, finding := range scan.Findings {
		if _, expected := expectedFindings[finding.Code]; expected {
			expectedFindings[finding.Code] = true
		}
		if finding.Status == domain.Unknown {
			if len(finding.Reasons) == 0 || len(finding.MissingEvidence) == 0 {
				t.Fatal("unknown finding lacked structured uncertainty")
			}
		} else if len(finding.Evidence) == 0 {
			t.Fatal("assessed finding lacked supporting evidence")
		}
	}
	for code, present := range expectedFindings {
		if !present {
			t.Fatalf("expected finding %s was absent", code)
		}
	}
	for kind, present := range expectedObservations {
		if !present {
			t.Fatalf("expected observation %s was absent", kind)
		}
	}
	if len(scan.Diagnoses) == 0 {
		for _, finding := range scan.Findings {
			if strings.HasPrefix(finding.Code, "electrum.") && finding.Status == domain.Unknown {
				t.Fatal("material unknown had no inconclusive diagnosis")
			}
		}
	}
}

func assertRoundTrip(t *testing.T, before, after domain.Scan) {
	t.Helper()
	if len(before.Components) != len(after.Components) || len(before.DependencyEdges) != len(after.DependencyEdges) ||
		len(before.Observations) != len(after.Observations) || len(before.Findings) != len(after.Findings) ||
		len(before.Diagnoses) != len(after.Diagnoses) {
		t.Fatal("persistence changed the normalized graph")
	}
	beforeEvidence, afterEvidence := evidence(before), evidence(after)
	if len(beforeEvidence) != len(afterEvidence) {
		t.Fatal("persistence changed evidence lineage")
	}
	for key := range beforeEvidence {
		if !afterEvidence[key] {
			t.Fatal("persistence changed evidence lineage")
		}
	}
}

func evidence(scan domain.Scan) map[string]bool {
	out := map[string]bool{}
	for _, finding := range scan.Findings {
		for _, reference := range finding.Evidence {
			out[finding.ID+"\x00"+reference.ObservationID+"\x00"+reference.ObservationKind] = true
		}
	}
	for _, diagnosis := range scan.Diagnoses {
		for _, reference := range diagnosis.Evidence {
			out[diagnosis.ID+"\x00"+reference.ObservationID+"\x00"+reference.ObservationKind] = true
		}
	}
	return out
}

func exerciseAPI(t *testing.T, store *sqlite.Store, scanID string, config app.Config) []byte {
	t.Helper()
	handler := (&apiv1.Server{Store: store, Runner: engine.Runner{Config: config}}).Handler()
	_, sourceFile, _, _ := runtime.Caller(0)
	document, err := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(sourceFile), "../../schemas/api/v1/openapi.yaml"))
	if err != nil {
		t.Fatal("could not load OpenAPI")
	}
	router, err := legacy.NewRouter(document)
	if err != nil {
		t.Fatal("could not route OpenAPI")
	}
	var combined bytes.Buffer
	for _, suffix := range []string{"", "/observations", "/findings", "/components", "/graph", "/diagnoses"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/scans/"+scanID+suffix, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) {
			t.Fatal("live Electrum API response was invalid")
		}
		route, parameters, routeErr := router.FindRoute(request)
		if routeErr != nil {
			t.Fatal("live Electrum API route was absent from OpenAPI")
		}
		input := &openapi3filter.RequestValidationInput{Request: request, PathParams: parameters, Route: route}
		output := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: response.Code, Header: response.Header()}
		output.SetBodyBytes(response.Body.Bytes())
		if openapi3filter.ValidateResponse(context.Background(), output) != nil {
			t.Fatal("live Electrum API response violated OpenAPI")
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
	diagnoses := make([]string, 0, len(scan.Diagnoses))
	for _, diagnosis := range scan.Diagnoses {
		diagnoses = append(diagnoses, diagnosis.Code+"="+string(diagnosis.Classification))
	}
	sort.Strings(diagnoses)
	return "live Electrum health=" + string(scan.Summary.Health) + " coverage=" + string(scan.Summary.Coverage.State) +
		" findings=[" + strings.Join(findings, ",") + "] diagnoses=[" + strings.Join(diagnoses, ",") + "] report_validation=passed"
}
