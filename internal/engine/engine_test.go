package engine

import (
	"context"
	"errors"
	"fmt"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/probes"
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
		_, _ = w.Write([]byte(`{"result":{"blocks":100,"headers":100,"verificationprogress":1,"initialblockdownload":false,"time":` + fmt.Sprintf("%d", now.Unix()) + `},"error":null,"id":1}`))
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
