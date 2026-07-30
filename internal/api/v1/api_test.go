package v1

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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
