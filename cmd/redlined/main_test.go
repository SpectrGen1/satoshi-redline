package main

import (
	"context"
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
	cancel()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, newHTTPServer("127.0.0.1:0", http.NewServeMux())) }()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown timed out")
	}
}
