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
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/engine"
	reportv1 "github.com/satoshi-redline/redline/internal/reports/v1"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
)

func TestCanariesDoNotCrossBoundaries(t *testing.T) {
	const cookieValue = "CANARY_COOKIE_7f91"
	const malformedBody = "CANARY_MALFORMED_2a44"
	const responseHeader = "CANARY_HEADER_91bb"
	const rpcError = "CANARY_RPC_ERROR_442a"
	const endpointUserinfo = "CANARY_USERINFO_73ce"
	malformedRPC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Unsafe", responseHeader)
		_, _ = w.Write([]byte(malformedBody + " malformed"))
	}))
	defer malformedRPC.Close()
	errorRPC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":null,"error":{"code":-1,"message":"` + rpcError + `"},"id":1}`))
	}))
	defer errorRPC.Close()
	root := t.TempDir()
	dataDir := filepath.Join(root, "CANARY_PATH_52cc")
	if e := os.Mkdir(dataDir, 0700); e != nil {
		t.Fatal(e)
	}
	cookie := filepath.Join(root, "cookie.fixture")
	if e := os.WriteFile(cookie, []byte("user:"+cookieValue), 0600); e != nil {
		t.Fatal(e)
	}
	dbPath := filepath.Join(root, "redline.db")
	store, e := sqlite.Open(dbPath)
	if e != nil {
		t.Fatal(e)
	}
	now := func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	var scans []domain.Scan
	for _, endpoint := range []string{malformedRPC.URL, errorRPC.URL, "http://" + endpointUserinfo + ":password@127.0.0.1:8332"} {
		cfg := app.DefaultConfig()
		cfg.Bitcoin.RPCEndpoint = endpoint
		cfg.Bitcoin.CookieFile = cookie
		cfg.Bitcoin.DataDirectory = dataDir
		cfg.DatabasePath = dbPath
		scan, scanErr := (engine.Runner{Config: cfg, Now: now}).ScanAndPersist(context.Background(), store)
		if scanErr != nil {
			t.Fatal(scanErr)
		}
		scans = append(scans, scan)
	}
	api := (&apiv1.Server{Store: store}).Handler()
	var boundary bytes.Buffer
	for _, scan := range scans {
		report, reportErr := reportv1.Marshal(scan)
		if reportErr != nil {
			t.Fatal(reportErr)
		}
		boundary.Write(report)
		for _, suffix := range []string{"", "/observations", "/findings"} {
			rec := httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/scans/"+scan.ID+suffix, nil))
			boundary.Write(rec.Body.Bytes())
		}
	}
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	canaries := []string{cookieValue, malformedBody, responseHeader, rpcError, endpointUserinfo, "CANARY_PATH_52cc"}
	for _, canary := range canaries {
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
			for _, canary := range canaries {
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
