package privacy_test

import (
	"bytes"
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	apiv1 "github.com/satoshi-redline/redline/internal/api/v1"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/engine"
	reportv1 "github.com/satoshi-redline/redline/internal/reports/v1"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
)

func TestCanariesDoNotCrossBoundaries(t *testing.T) {
	const credential = "CANARY_CREDENTIAL_7f91"
	const response = "CANARY_RESPONSE_2a44"
	const header = "CANARY_HEADER_91bb"
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Unsafe", header)
		_, _ = w.Write([]byte(response + " malformed"))
	}))
	defer rpc.Close()
	root := t.TempDir()
	dataDir := filepath.Join(root, "CANARY_PATH_52cc")
	if e := os.Mkdir(dataDir, 0700); e != nil {
		t.Fatal(e)
	}
	cookie := filepath.Join(root, "cookie.fixture")
	if e := os.WriteFile(cookie, []byte("user:"+credential), 0600); e != nil {
		t.Fatal(e)
	}
	dbPath := filepath.Join(root, "redline.db")
	store, e := sqlite.Open(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	cfg := app.DefaultConfig()
	cfg.Bitcoin.RPCEndpoint = rpc.URL
	cfg.Bitcoin.CookieFile = cookie
	cfg.Bitcoin.DataDirectory = dataDir
	cfg.DatabasePath = dbPath
	scan, e := (engine.Runner{Config: cfg, Now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }}).ScanAndPersist(context.Background(), store)
	if e != nil {
		t.Fatal(e)
	}
	report, e := reportv1.Marshal(scan)
	if e != nil {
		t.Fatal(e)
	}
	api := (&apiv1.Server{Runner: engine.Runner{Config: cfg}, Store: store}).Handler()
	var boundary bytes.Buffer
	boundary.Write(report)
	for _, suffix := range []string{"", "/observations", "/findings"} {
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/scans/"+scan.ID+suffix, nil))
		boundary.Write(rec.Body.Bytes())
	}
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	for _, canary := range []string{credential, response, header, "CANARY_PATH_52cc"} {
		if bytes.Contains(boundary.Bytes(), []byte(canary)) {
			t.Fatalf("canary crossed API/report boundary: %s", canary)
		}
	}
	e = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || path == cookie {
			return nil
		}
		if filepath.Base(path) == "redline.db" || bytes.Contains([]byte(filepath.Base(path)), []byte("redline.db-")) {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			for _, canary := range []string{credential, response, header, "CANARY_PATH_52cc"} {
				if bytes.Contains(b, []byte(canary)) {
					t.Fatalf("canary crossed SQLite boundary: %s", canary)
				}
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
