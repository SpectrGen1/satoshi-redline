package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	v1 "github.com/satoshi-redline/redline/internal/api/v1"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/engine"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	p := flag.String("config", "redline.json", "configuration file")
	flag.Parse()
	if e := run(*p, os.Geteuid); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(path string, euid func() int) error {
	if euid() != -1 && euid() == 0 {
		return fmt.Errorf("redlined refuses to run as root")
	}
	c, e := app.Load(path)
	if e != nil {
		return fmt.Errorf("configuration error")
	}
	s, e := sqlite.Open(c.DatabasePath)
	if e != nil {
		return fmt.Errorf("database unavailable")
	}
	defer s.Close()
	api := &v1.Server{Runner: engine.Runner{Config: c}, Store: s}
	server := newHTTPServer(c.API.ListenAddress, api.Handler())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Printf("redlined listening on %s\n", c.API.ListenAddress)
	return serve(ctx, server)
}
func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
}
func serve(ctx context.Context, server *http.Server) error {
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if e := server.Shutdown(shutdownCtx); e != nil {
			return fmt.Errorf("server shutdown failed")
		}
		fmt.Println("redlined stopped")
		return nil
	case e := <-errCh:
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("server failed")
	}
}
