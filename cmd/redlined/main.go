package main

import (
	"flag"
	"fmt"
	"github.com/satoshi-redline/redline/internal/api/v1"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/engine"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
	"net/http"
	"os"
)

func main() {
	p := flag.String("config", "redline.json", "configuration file")
	flag.Parse()
	c, e := app.Load(*p)
	if e != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", e)
		os.Exit(2)
	}
	s, e := sqlite.Open(c.DatabasePath)
	if e != nil {
		fmt.Fprintln(os.Stderr, "database error")
		os.Exit(1)
	}
	defer s.Close()
	api := &v1.Server{Runner: engine.Runner{Config: c}, Store: s}
	server := &http.Server{Addr: c.API.ListenAddress, Handler: api.Handler()}
	fmt.Printf("redlined listening on %s\n", c.API.ListenAddress)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "server error")
	}
}
