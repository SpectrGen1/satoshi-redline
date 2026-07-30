package engine

import (
	"context"
	"errors"
	"fmt"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
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
	found := false
	for _, v := range sink.events {
		if v.Type == "scan.failed" {
			found = true
		}
		if strings.Contains(v.Outcome, "CANARY_SECRET") {
			t.Fatal("error leaked into event")
		}
	}
	if !found {
		t.Fatal("scan.failed not emitted")
	}
}
func TestEventSinkCannotBreakScan(t *testing.T) {
	c := app.DefaultConfig()
	if _, e := (Runner{Config: c, Events: &eventRecorder{panicOnEmit: true}}).Scan(context.Background()); e != nil {
		t.Fatal(e)
	}
}
