package main

import (
	"bytes"
	"github.com/satoshi-redline/redline/internal/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIErrorDoesNotExposeConfigurationPath(t *testing.T) {
	canary := "CANARY_CLI_PATH_88ae"
	var out, errOut bytes.Buffer
	code := run([]string{"--config", filepath.Join(t.TempDir(), canary), "scan"}, &out, &errOut)
	if code == 0 {
		t.Fatal("expected failure")
	}
	if strings.Contains(errOut.String(), canary) {
		t.Fatalf("path leaked: %s", errOut.String())
	}
}

func TestCLIErrorDoesNotExposeDatabasePath(t *testing.T) {
	const canary = "CANARY_CLI_DATABASE_79bd"
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"database_path":"`+filepath.Join(root, canary, "db.sqlite")+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--config", configPath, "scan"}, &stdout, &stderr); code == 0 {
		t.Fatal("expected database failure")
	}
	if strings.Contains(stdout.String(), canary) || strings.Contains(stderr.String(), canary) {
		t.Fatalf("database path leaked: %s%s", stdout.String(), stderr.String())
	}
}
func TestVersionNeedsNoConfiguration(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "v0.1.0") {
		t.Fatalf("%d %s", code, out.String())
	}
}

func TestHumanScanIncludesSafeDiagnosisSummary(t *testing.T) {
	var output bytes.Buffer
	scan := domain.Scan{
		ID: "scan", Summary: domain.ComponentSummary{Health: domain.HealthFailure, Coverage: domain.CoverageSummary{State: domain.Partial, ApplicableRequired: 2, AssessedRequired: 1}},
		Diagnoses: []domain.Diagnosis{{Code: "wallet-backend.electrum-protocol-breakpoint", Classification: domain.DiagnosisBreakPoint}},
	}
	if err := writeHumanScan(&output, scan); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	if !strings.Contains(rendered, "Scan ID: scan") || !strings.Contains(rendered, "wallet-backend.electrum-protocol-breakpoint (break_point)") {
		t.Fatalf("diagnosis summary missing: %s", rendered)
	}
	for _, forbidden := range []string{"tcp://", "ssl://", "cookie", "/tmp/"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("unsafe detail in human output: %s", rendered)
		}
	}
}
