package bitcoinrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/satoshi-redline/redline/internal/domain"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeChainRPC(w http.ResponseWriter, r *http.Request, tipTime int64) {
	var request struct {
		Method string `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch request.Method {
	case "getblockchaininfo":
		_, _ = w.Write([]byte(`{"result":{"blocks":1,"headers":1,"verificationprogress":1,"initialblockdownload":false,"bestblockhash":"00000001"},"error":null,"id":1}`))
	case "getblockheader":
		_, _ = fmt.Fprintf(w, `{"result":{"time":%d},"error":null,"id":1}`, tipTime)
	default:
		_, _ = w.Write([]byte(`{"result":null,"error":{"code":-32601},"id":1}`))
	}
}

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
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChainRPC(w, r, 1700000000)
	}))
	defer s.Close()
	c, _ := New(s.URL, cookie(t), "", time.Second)
	if _, r, _ := c.Chain(context.Background()); r != "" {
		t.Fatal(r)
	}
}

func TestCollectionFailureMatrix(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		timeout time.Duration
		want    domain.UnknownReasonCode
		reached bool
	}{
		{"valid", func(w http.ResponseWriter, r *http.Request) { writeChainRPC(w, r, 1700000000) }, time.Second, "", true},
		{"missing fields", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"result":{"blocks":1},"error":null,"id":1}`))
		}, time.Second, domain.InvalidResponse, true},
		{"oversized", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(bytes.Repeat([]byte("x"), maxResponse+1)) }, time.Second, domain.InvalidResponse, true},
		{"timeout", func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte(`{"result":null,"error":null,"id":1}`))
		}, 10 * time.Millisecond, domain.Timeout, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()
			client, reason := New(server.URL, cookie(t), "", tt.timeout)
			if reason != "" {
				t.Fatal(reason)
			}
			_, reason, reached := client.Chain(context.Background())
			if reason != tt.want || reached != tt.reached {
				t.Fatalf("got %q/%v want %q/%v", reason, reached, tt.want, tt.reached)
			}
		})
	}
}

func TestBitcoinKnotsChainResponseUsesBestBlockHeaderTime(t *testing.T) {
	const tipTime = int64(1750000000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChainRPC(w, r, tipTime)
	}))
	defer server.Close()
	client, reason := New(server.URL, cookie(t), "", time.Second)
	if reason != "" {
		t.Fatal(reason)
	}
	info, reason, reached := client.Chain(context.Background())
	if reason != "" || !reached {
		t.Fatalf("collection failed: %q/%v", reason, reached)
	}
	if info.TipTime.Unix() != tipTime {
		t.Fatalf("got tip time %d", info.TipTime.Unix())
	}
}

func TestConnectionRefusal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "http://" + listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	client, reason := New(endpoint, cookie(t), "", time.Second)
	if reason != "" {
		t.Fatal(reason)
	}
	if _, reason, reached := client.Chain(context.Background()); reason != domain.ConnectionRefused || reached {
		t.Fatalf("got %q/%v", reason, reached)
	}
}
