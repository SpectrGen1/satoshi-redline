package engine

import (
	"context"
	"fmt"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
		if o.Kind == "bitcoin.rpc.response/v1" && o.RPCReachable == nil {
			t.Fatal("missing rpc observation")
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
