package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/engine"
	"github.com/satoshi-redline/redline/internal/reports/v1"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
	"os"
)

func main() {
	fs := flag.NewFlagSet("redline", flag.ExitOnError)
	cfg := fs.String("config", "redline.json", "configuration file")
	fs.Parse(os.Args[1:])
	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: redline [--config file] scan [--json] | report <scan-id> | version")
		os.Exit(2)
	}
	if args[0] == "version" {
		fmt.Println("redline mvp v0.1.0")
		return
	}
	c, e := app.Load(*cfg)
	if e != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", e)
		os.Exit(2)
	}
	if args[0] == "scan" {
		s, openErr := sqlite.Open(c.DatabasePath)
		if openErr != nil {
			fmt.Fprintln(os.Stderr, "database unavailable")
			os.Exit(1)
		}
		defer s.Close()
		x, e := (engine.Runner{Config: c}).ScanAndPersist(context.Background(), s)
		if e != nil {
			fmt.Fprintln(os.Stderr, "scan failed")
			os.Exit(1)
		}
		if len(args) > 1 && args[1] == "--json" {
			_ = json.NewEncoder(os.Stdout).Encode(v1.FromDomain(x))
			return
		}
		fmt.Printf("Scan ID: %s\nHealth: %s\nRequired coverage: %s (%d/%d assessed)\n", x.ID, x.Summary.Health, x.Summary.Coverage.State, x.Summary.Coverage.AssessedRequired, x.Summary.Coverage.ApplicableRequired)
		return
	}
	if args[0] == "report" && len(args) == 2 {
		s, e := sqlite.Open(c.DatabasePath)
		if e != nil {
			fmt.Fprintln(os.Stderr, "database unavailable")
			os.Exit(1)
		}
		defer s.Close()
		x, e := s.Get(context.Background(), args[1])
		if e != nil {
			fmt.Fprintln(os.Stderr, "scan not found")
			os.Exit(1)
		}
		b, e := v1.Marshal(x)
		if e != nil {
			fmt.Fprintln(os.Stderr, "report error")
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}
	fmt.Fprintln(os.Stderr, "invalid command")
	os.Exit(2)
}
