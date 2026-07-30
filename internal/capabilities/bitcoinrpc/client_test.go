package bitcoinrpc

import (
	"context"
	"github.com/satoshi-redline/redline/internal/domain"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func cookie(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cookie")
	if e := os.WriteFile(p, []byte("u:CANARY_SECRET"), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestEndpointValidation(t *testing.T) {
	for _, v := range []string{"ftp://host", "http://u:p@host", "://"} {
		if _, r := New(v, cookie(t), "", time.Second); r != domain.NotConfigured {
			t.Fatalf("%q: %s", v, r)
		}
	}
}
func TestAuthMalformedRPCAndRedirect(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    domain.UnknownReasonCode
		reached bool
	}{{"auth", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }, domain.AuthenticationRejected, true}, {"malformed", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("CANARY_SECRET not json")) }, domain.InvalidResponse, true}, {"rpc error", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":null,"error":{"code":-1,"message":"CANARY_SECRET"},"id":1}`))
	}, domain.RPCError, true}, {"redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/other", 302) }, domain.InvalidResponse, true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewServer(tt.handler)
			defer s.Close()
			c, r := New(s.URL, cookie(t), "", time.Second)
			if r != "" {
				t.Fatal(r)
			}
			_, got, reached := c.Chain(context.Background())
			if got != tt.want || reached != tt.reached {
				t.Fatalf("%s %v", got, reached)
			}
		})
	}
}
func TestAmbientProxyIgnored(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"blocks":1,"headers":1,"verificationprogress":1,"initialblockdownload":false,"time":1700000000},"error":null,"id":1}`))
	}))
	defer s.Close()
	c, _ := New(s.URL, cookie(t), "", time.Second)
	if _, r, _ := c.Chain(context.Background()); r != "" {
		t.Fatal(r)
	}
}
