package main

import (
	"bytes"
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
func TestVersionNeedsNoConfiguration(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "v0.1.0") {
		t.Fatalf("%d %s", code, out.String())
	}
}
