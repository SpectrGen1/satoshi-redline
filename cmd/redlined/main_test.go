package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRootGuard(t *testing.T) {
	if e := run("unused", func() int { return 0 }); e == nil || !strings.Contains(e.Error(), "refuses to run as root") {
		t.Fatalf("got %v", e)
	}
}

func TestServerSecurityTimeouts(t *testing.T) {
	s := newHTTPServer("127.0.0.1:0", http.NewServeMux())
	if s.ReadHeaderTimeout == 0 || s.ReadTimeout == 0 || s.WriteTimeout == 0 || s.IdleTimeout == 0 || s.MaxHeaderBytes == 0 {
		t.Fatalf("unsafe server config: %+v", s)
	}
}
func TestGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := newHTTPServer(listener.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ready")
	}))
	done := make(chan error, 1)
	go func() { done <- serveListener(ctx, server, listener) }()
	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("listener did not accept request: %v", err)
	}
	_ = response.Body.Close()
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown timed out")
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Fatal("listener still accepts connections")
	}
}
