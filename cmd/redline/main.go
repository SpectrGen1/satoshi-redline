package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/engine"
	v1 "github.com/satoshi-redline/redline/internal/reports/v1"
	"github.com/satoshi-redline/redline/internal/storage/sqlite"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(argv []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("redline", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg := fs.String("config", "redline.json", "configuration file")
	if e := fs.Parse(argv); e != nil {
		fmt.Fprintln(errOut, "invalid arguments")
		return 2
	}
	args := fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: redline [--config file] scan [--json] | report <scan-id> | version")
		return 2
	}
	if args[0] == "version" {
		fmt.Fprintln(out, "redline mvp v0.1.0")
		return 0
	}
	c, e := app.Load(*cfg)
	if e != nil {
		fmt.Fprintln(errOut, "configuration error")
		return 2
	}
	switch args[0] {
	case "scan":
		if len(args) > 2 || (len(args) == 2 && args[1] != "--json") {
			fmt.Fprintln(errOut, "invalid command")
			return 2
		}
		s, e := sqlite.Open(c.DatabasePath)
		if e != nil {
			fmt.Fprintln(errOut, "database unavailable")
			return 1
		}
		defer s.Close()
		x, e := (engine.Runner{Config: c}).ScanAndPersist(context.Background(), s)
		if e != nil {
			fmt.Fprintln(errOut, "scan failed")
			return 1
		}
		if len(args) == 2 {
			b, e := v1.Marshal(x)
			if e != nil {
				fmt.Fprintln(errOut, "output error")
				return 1
			}
			if _, e = fmt.Fprintln(out, string(b)); e != nil {
				return 1
			}
			return 0
		}
		if e = writeHumanScan(out, x); e != nil {
			return 1
		}
		return 0
	case "report":
		if len(args) != 2 {
			fmt.Fprintln(errOut, "invalid command")
			return 2
		}
		s, e := sqlite.Open(c.DatabasePath)
		if e != nil {
			fmt.Fprintln(errOut, "database unavailable")
			return 1
		}
		defer s.Close()
		x, e := s.Get(context.Background(), args[1])
		if e != nil {
			fmt.Fprintln(errOut, "scan not found")
			return 1
		}
		b, e := v1.Marshal(x)
		if e != nil {
			fmt.Fprintln(errOut, "report error")
			return 1
		}
		if _, e = fmt.Fprintln(out, string(b)); e != nil {
			return 1
		}
		return 0
	default:
		fmt.Fprintln(errOut, "invalid command")
		return 2
	}
}

func writeHumanScan(out io.Writer, scan domain.Scan) error {
	if _, err := fmt.Fprintf(out, "Scan ID: %s\nHealth: %s\nRequired coverage: %s (%d/%d assessed)\n", scan.ID, scan.Summary.Health, scan.Summary.Coverage.State, scan.Summary.Coverage.AssessedRequired, scan.Summary.Coverage.ApplicableRequired); err != nil {
		return err
	}
	if len(scan.Diagnoses) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(out, "Diagnoses:"); err != nil {
		return err
	}
	for _, diagnosis := range scan.Diagnoses {
		if _, err := fmt.Fprintf(out, "- %s (%s)\n", diagnosis.Code, diagnosis.Classification); err != nil {
			return err
		}
	}
	return nil
}
