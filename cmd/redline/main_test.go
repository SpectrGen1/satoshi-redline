package main

import (
	"bytes"
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
