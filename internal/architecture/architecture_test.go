package architecture_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
func productionFiles(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	root := projectRoot(t)
	e := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		out[path] = string(b)
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestEvaluatorHasNoClockOrExternalIO(t *testing.T) {
	for path, src := range productionFiles(t) {
		if !strings.Contains(path, filepath.Join("internal", "evaluation")) {
			continue
		}
		for _, bad := range []string{"time.Now(", "net/http", "os.Open(", "os.ReadFile(", "database/sql"} {
			if strings.Contains(src, bad) {
				t.Fatalf("%s contains forbidden %q", path, bad)
			}
		}
	}
}
func TestProbeArchitectureBoundaries(t *testing.T) {
	for path, src := range productionFiles(t) {
		if !strings.Contains(path, filepath.Join("internal", "probes")) {
			continue
		}
		for _, bad := range []string{"internal/storage", "internal/api", "internal/reports", "modernc.org/sqlite", "plugin.Open(", "os/exec"} {
			if strings.Contains(src, bad) {
				t.Fatalf("%s contains forbidden %q", path, bad)
			}
		}
	}
}
func TestNoForbiddenPersistenceOrPluginFields(t *testing.T) {
	for path, src := range productionFiles(t) {
		for _, bad := range []string{"raw_payload", "raw_response", "raw_log", "debug_data", "extra_json", "arbitrary_metadata", "plugin.Open("} {
			if strings.Contains(src, bad) {
				t.Fatalf("%s contains forbidden %q", path, bad)
			}
		}
	}
}
