package engine

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/capabilities"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/probes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type eventRecorder struct {
	events      []Event
	panicOnEmit bool
}

func (e *eventRecorder) Emit(v Event) {
	if e.panicOnEmit {
		panic("CANARY_SECRET")
	}
	e.events = append(e.events, v)
}

type failingStore struct{}

func (failingStore) Save(context.Context, domain.Scan) error { return errors.New("CANARY_SECRET") }

type successfulStore struct{}

func (successfulStore) Save(context.Context, domain.Scan) error { return nil }

func TestScanSynchronizedHealthy(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "u" || p != "CANARY_SECRET" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Method == "getblockchaininfo" {
			_, _ = w.Write([]byte(`{"result":{"blocks":100,"headers":100,"verificationprogress":1,"initialblockdownload":false,"bestblockhash":"00000001"},"error":null,"id":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"time":` + fmt.Sprintf("%d", now.Unix()) + `},"error":null,"id":1}`))
	}))
	defer srv.Close()
	d := t.TempDir()
	cookie := filepath.Join(d, "cookie")
	if e := os.WriteFile(cookie, []byte("u:CANARY_SECRET"), 0600); e != nil {
		t.Fatal(e)
	}
	c := app.DefaultConfig()
	c.Bitcoin = app.BitcoinConfig{RPCEndpoint: srv.URL, CookieFile: cookie, DataDirectory: d}
	x, e := Runner{Config: c, Now: func() time.Time { return now }}.Scan(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if x.Summary.Health != domain.Healthy || x.Summary.Coverage.State != domain.Complete {
		t.Fatalf("got %s/%s", x.Summary.Health, x.Summary.Coverage.State)
	}
	for _, o := range x.Observations {
		if o.Kind == "bitcoin.rpc.transport" {
			if _, ok := o.Value.(domain.RPCTransportResult); !ok {
				t.Fatal("missing rpc transport value")
			}
		}
	}
}

func TestCompleteBitcoinElectrumSlice(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	header := make([]byte, 80)
	header[0] = 1
	binary.LittleEndian.PutUint32(header[68:72], uint32(now.Unix()))
	first := sha256.Sum256(header)
	second := sha256.Sum256(first[:])
	for left, right := 0, len(second)-1; left < right; left, right = left+1, right-1 {
		second[left], second[right] = second[right], second[left]
	}
	blockHash := hex.EncodeToString(second[:])

	bitcoin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch request.Method {
		case "getblockchaininfo":
			_, _ = fmt.Fprintf(w, `{"result":{"blocks":100,"headers":100,"verificationprogress":1,"initialblockdownload":false,"time":%d},"error":null,"id":1}`, now.Unix())
		case "getblockhash":
			_, _ = fmt.Fprintf(w, `{"result":%q,"error":null,"id":1}`, blockHash)
		default:
			_, _ = w.Write([]byte(`{"result":null,"error":{"code":-32601},"id":1}`))
		}
	}))
	defer bitcoin.Close()
	electrumListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer electrumListener.Close()
	go func() {
		connection, acceptErr := electrumListener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		_, _ = reader.ReadBytes('\n')
		_, _ = connection.Write([]byte(`{"jsonrpc":"2.0","result":["Fulcrum CANARY_BANNER","1.4.3"],"id":1}` + "\n"))
		_, _ = reader.ReadBytes('\n')
		_, _ = fmt.Fprintf(connection, `{"jsonrpc":"2.0","result":{"height":100,"hex":%q},"id":2}`+"\n", hex.EncodeToString(header))
	}()

	root := t.TempDir()
	cookie := filepath.Join(root, "cookie")
	if err = os.WriteFile(cookie, []byte("u:p"), 0600); err != nil {
		t.Fatal(err)
	}
	config := app.DefaultConfig()
	config.Bitcoin = app.BitcoinConfig{RPCEndpoint: bitcoin.URL, CookieFile: cookie, DataDirectory: root}
	config.Electrum.Enabled = true
	config.Electrum.Endpoint = "tcp://" + electrumListener.Addr().String()
	config.Electrum.BitcoinComponentID = "bitcoin.primary"
	scan, err := (Runner{Config: config, Now: func() time.Time { return now }}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = scan.Validate(); err != nil {
		t.Fatal(err)
	}
	if scan.Summary.Health != domain.Healthy || scan.Summary.Coverage.State != domain.Complete || len(scan.Diagnoses) != 0 {
		t.Fatalf("unexpected complete slice: summary=%+v diagnoses=%+v", scan.Summary, scan.Diagnoses)
	}
	expected := map[string]bool{
		"electrum.protocol.reachable": false, "electrum.protocol.compatible": false,
		"electrum.chain.tip-freshness": false, "electrum.chain.sync-lag": false,
		"electrum.chain.matches-bitcoin": false,
	}
	for _, finding := range scan.Findings {
		if _, ok := expected[finding.Code]; ok {
			expected[finding.Code] = finding.Status == domain.Pass && len(finding.Evidence) > 0
		}
	}
	for code, passed := range expected {
		if !passed {
			t.Fatalf("%s did not produce a supported pass", code)
		}
	}
	forbidden := fmt.Sprintf("%+v", scan)
	if strings.Contains(forbidden, "CANARY_BANNER") || strings.Contains(forbidden, config.Electrum.Endpoint) {
		t.Fatal("Electrum configuration or banner entered normalized scan")
	}
}
func TestMissingEvidenceIsNotPass(t *testing.T) {
	c := app.DefaultConfig()
	x, e := Runner{Config: c}.Scan(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range x.Findings {
		if f.Status == domain.Pass {
			t.Fatalf("unexpected pass: %s", f.Code)
		}
		if f.Status == domain.Unknown && len(f.Reasons) == 0 {
			t.Fatal("unknown lacks reason")
		}
	}
}

func TestScanFailedEventOnPersistenceFailure(t *testing.T) {
	sink := &eventRecorder{}
	c := app.DefaultConfig()
	_, e := (Runner{Config: c, Events: sink}).ScanAndPersist(context.Background(), failingStore{})
	if e == nil {
		t.Fatal("expected error")
	}
	var terminal []string
	for _, v := range sink.events {
		if v.Type == "scan.failed" || v.Type == "scan.completed" {
			terminal = append(terminal, v.Type)
		}
		if strings.Contains(v.Outcome, "CANARY_SECRET") {
			t.Fatal("error leaked into event")
		}
	}
	if fmt.Sprint(terminal) != "[scan.failed]" {
		t.Fatalf("terminal events = %v", terminal)
	}
}
func TestEventSinkCannotBreakScan(t *testing.T) {
	c := app.DefaultConfig()
	if _, e := (Runner{Config: c, Events: &eventRecorder{panicOnEmit: true}}).Scan(context.Background()); e != nil {
		t.Fatal(e)
	}
}

func TestScanTerminalEventSequences(t *testing.T) {
	tests := []struct {
		name string
		run  func(Runner) error
	}{
		{"scan", func(r Runner) error { _, err := r.Scan(context.Background()); return err }},
		{"scan and persist", func(r Runner) error {
			_, err := r.ScanAndPersist(context.Background(), successfulStore{})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &eventRecorder{}
			if err := tt.run(Runner{Config: app.DefaultConfig(), Events: sink}); err != nil {
				t.Fatal(err)
			}
			if len(sink.events) == 0 || sink.events[0].Type != "scan.started" {
				t.Fatalf("events = %#v", sink.events)
			}
			var terminal []string
			for _, event := range sink.events {
				if event.Type == "scan.completed" || event.Type == "scan.failed" {
					terminal = append(terminal, event.Type)
				}
			}
			if fmt.Sprint(terminal) != "[scan.completed]" {
				t.Fatalf("terminal events = %v", terminal)
			}
		})
	}
}

func TestEvaluationTimestampFollowsCollectionAndIsShared(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	call := 0
	now := func() time.Time {
		value := base.Add(time.Duration(call) * time.Second)
		call++
		return value
	}
	scan, err := (Runner{Config: app.DefaultConfig(), Now: now}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Findings) == 0 {
		t.Fatal("findings missing")
	}
	evaluatedAt := scan.Findings[0].EvaluatedAt
	for _, execution := range scan.Executions {
		if evaluatedAt.Before(execution.FinishedAt) {
			t.Fatalf("evaluation %s preceded collection finish %s", evaluatedAt, execution.FinishedAt)
		}
	}
	for _, finding := range scan.Findings {
		if !finding.EvaluatedAt.Equal(evaluatedAt) {
			t.Fatal("evaluators did not share one evaluation timestamp")
		}
	}
}

func TestExecutionOutcomeClassification(t *testing.T) {
	observation := domain.Observation{ID: "usable"}
	tests := []struct {
		name                string
		result              probes.Result
		parentErr, probeErr error
		want                domain.ExecutionOutcome
	}{
		{"completed", probes.Result{}, nil, nil, domain.ExecutionCompleted},
		{"unavailable", probes.Result{Reason: domain.RequiredObservationMissing}, nil, nil, domain.ExecutionUnavailable},
		{"partial", probes.Result{Values: []domain.Observation{observation}, Reason: domain.ProtocolError}, nil, nil, domain.ExecutionPartial},
		{"provider failure", probes.Result{Reason: domain.ProtocolError}, nil, nil, domain.ExecutionFailed},
		{"probe deadline", probes.Result{Reason: domain.Timeout}, nil, context.DeadlineExceeded, domain.ExecutionFailed},
		{"caller cancellation", probes.Result{Reason: domain.Cancelled}, context.Canceled, context.Canceled, domain.ExecutionCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := executionOutcome(tt.result, tt.parentErr, tt.probeErr); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestCancelledScanAndPartialProviderFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelSink := &eventRecorder{}
	cancelled, err := (Runner{Config: app.DefaultConfig(), Events: cancelSink}).Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, execution := range cancelled.Executions {
		if execution.Outcome != domain.ExecutionCancelled {
			t.Fatalf("outcome = %s", execution.Outcome)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("malformed"))
	}))
	defer server.Close()
	root := t.TempDir()
	cookiePath := filepath.Join(root, "cookie")
	if err = os.WriteFile(cookiePath, []byte("u:p"), 0600); err != nil {
		t.Fatal(err)
	}
	config := app.DefaultConfig()
	config.Bitcoin.RPCEndpoint = server.URL
	config.Bitcoin.CookieFile = cookiePath
	config.Bitcoin.DataDirectory = root
	partial, err := (Runner{Config: config}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, execution := range partial.Executions {
		if execution.ProbeID == "bitcoin.chain-state/v1" {
			found = true
			if execution.Outcome != domain.ExecutionPartial || execution.Limitation != domain.InvalidResponse {
				t.Fatalf("execution = %+v", execution)
			}
		}
	}
	if !found {
		t.Fatal("bitcoin execution missing")
	}
}

func TestConfirmedRPCFailureSurvivesUnknownFilesystem(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "http://" + listener.Addr().String()
	_ = listener.Close()
	cookiePath := filepath.Join(t.TempDir(), "cookie")
	if err = os.WriteFile(cookiePath, []byte("u:p"), 0600); err != nil {
		t.Fatal(err)
	}
	config := app.DefaultConfig()
	config.Bitcoin.RPCEndpoint = endpoint
	config.Bitcoin.CookieFile = cookiePath
	scan, err := (Runner{Config: config}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Summary.Health != domain.HealthFailure || scan.Summary.Coverage.State != domain.Minimal {
		t.Fatalf("summary = %+v", scan.Summary)
	}
}

func TestElectrumDiagnosisIsCreatedFromActualScanLifecycle(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "tcp://" + listener.Addr().String()
	_ = listener.Close()
	config := app.DefaultConfig()
	config.Electrum.Enabled = true
	config.Electrum.Endpoint = endpoint
	config.Electrum.BitcoinComponentID = "bitcoin.primary"
	sink := &eventRecorder{}
	scan, err := (Runner{Config: config, Events: sink}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = scan.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(scan.Diagnoses) != 1 || scan.Diagnoses[0].Classification != domain.DiagnosisInconclusive {
		t.Fatalf("unexpected diagnoses: %+v", scan.Diagnoses)
	}
	diagnosisEvents := 0
	for _, event := range sink.events {
		if event.Type == "scan.diagnosis.created" {
			diagnosisEvents++
			if event.DiagnosisCode != scan.Diagnoses[0].Code {
				t.Fatalf("wrong diagnosis event: %+v", event)
			}
		}
	}
	if diagnosisEvents != 1 {
		t.Fatalf("got %d diagnosis events", diagnosisEvents)
	}
}

type invalidInternalProbe struct{}

func (invalidInternalProbe) Descriptor() probes.Descriptor {
	return probes.Descriptor{ID: "test.invalid/v1", Version: "1.0.0", ComponentKinds: []string{"bitcoin-node"}, Required: []capabilities.ID{capabilities.ClockRead}, Mode: probes.Safe, Timeout: time.Second, SideEffects: []probes.SideEffect{probes.ReadOnly}, Sensitivity: probes.SensitiveNone, ObservationKinds: []string{"bitcoin.chain.state"}}
}
func (invalidInternalProbe) Collect(context.Context, capabilities.Set, string, string, time.Time) probes.Result {
	return probes.Result{Values: []domain.Observation{{ID: "CANARY_INTERNAL_PROBE_85fc", Kind: "bitcoin.chain.state", Value: domain.FilesystemCapacity{TotalBytes: 1, MountPresent: true}}}}
}

func TestInternalProbeCanaryDoesNotEnterFailureEventOrError(t *testing.T) {
	catalog, err := probes.NewCatalog(invalidInternalProbe{})
	if err != nil {
		t.Fatal(err)
	}
	sink := &eventRecorder{}
	_, err = (Runner{Config: app.DefaultConfig(), Events: sink, catalog: &catalog}).Scan(context.Background())
	if err == nil {
		t.Fatal("expected invalid observation failure")
	}
	var output strings.Builder
	output.WriteString(err.Error())
	for _, event := range sink.events {
		output.WriteString(event.ID)
		output.WriteString(event.Type)
		output.WriteString(event.Outcome)
	}
	if strings.Contains(output.String(), "CANARY_INTERNAL_PROBE_85fc") {
		t.Fatal("internal probe canary leaked")
	}
}

func TestReachedEndpointDiagnosticDistinctions(t *testing.T) {
	tests := []struct {
		name           string
		handler        http.HandlerFunc
		authentication domain.AuthenticationState
	}{
		{"authentication rejected", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }, domain.AuthenticationRejectedState},
		{"malformed response", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("malformed")) }, domain.AuthenticationUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()
			root := t.TempDir()
			cookiePath := filepath.Join(root, "cookie")
			if err := os.WriteFile(cookiePath, []byte("u:p"), 0600); err != nil {
				t.Fatal(err)
			}
			config := app.DefaultConfig()
			config.Bitcoin.RPCEndpoint = server.URL
			config.Bitcoin.CookieFile = cookiePath
			config.Bitcoin.DataDirectory = root
			scan, err := (Runner{Config: config}).Scan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, finding := range scan.Findings {
				if finding.Code == "bitcoin.rpc.reachable" && finding.Status != domain.Pass {
					t.Fatalf("transport finding = %s", finding.Status)
				}
				if strings.HasPrefix(finding.Code, "bitcoin.chain.") && finding.Status != domain.Unknown {
					t.Fatalf("chain finding = %s", finding.Status)
				}
			}
			for _, observation := range scan.Observations {
				if value, ok := observation.Value.(domain.RPCTransportResult); ok && value.Authentication != tt.authentication {
					t.Fatalf("authentication = %s", value.Authentication)
				}
			}
		})
	}
}
