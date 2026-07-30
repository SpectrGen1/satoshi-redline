package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func productionGoFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	files := map[string]*ast.File{}
	root := projectRoot(t)
	set := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(set, path, nil, parser.ParseComments)
		if parseErr != nil {
			return parseErr
		}
		files[path] = file
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func imports(file *ast.File) []string {
	var paths []string
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err == nil {
			paths = append(paths, path)
		}
	}
	return paths
}

func TestProbeAndEvaluatorImportBoundaries(t *testing.T) {
	probeForbidden := []string{"/internal/storage", "/internal/api", "/internal/reports", "modernc.org/sqlite", "os/exec", "plugin"}
	evaluatorForbidden := []string{"net", "net/http", "os", "database/sql", "os/exec", "plugin"}
	for path, file := range productionGoFiles(t) {
		var forbidden []string
		switch {
		case strings.Contains(path, filepath.Join("internal", "probes")):
			forbidden = probeForbidden
		case strings.Contains(path, filepath.Join("internal", "evaluation")),
			strings.Contains(path, filepath.Join("internal", "correlation")):
			forbidden = evaluatorForbidden
		default:
			continue
		}
		for _, imported := range imports(file) {
			for _, denied := range forbidden {
				if imported == denied || strings.HasSuffix(imported, denied) {
					t.Fatalf("%s imports forbidden package %q", path, imported)
				}
			}
		}
	}
}

func TestDomainStructsHaveNoTransportTags(t *testing.T) {
	for path, file := range productionGoFiles(t) {
		if !strings.Contains(path, filepath.Join("internal", "domain")) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if ok && field.Tag != nil && strings.Contains(field.Tag.Value, "json:") {
				t.Errorf("%s contains a JSON transport tag", path)
			}
			return true
		})
	}
}

func TestProductionDoesNotLoadPluginsOrReadClockInEvaluators(t *testing.T) {
	for path, file := range productionGoFiles(t) {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			owner, _ := selector.X.(*ast.Ident)
			if owner != nil && owner.Name == "plugin" && selector.Sel.Name == "Open" {
				t.Errorf("%s calls plugin.Open", path)
			}
			if (strings.Contains(path, filepath.Join("internal", "evaluation")) || strings.Contains(path, filepath.Join("internal", "correlation"))) && owner != nil && owner.Name == "time" && selector.Sel.Name == "Now" {
				t.Errorf("%s reads the clock directly", path)
			}
			return true
		})
	}
}

func TestConfigurationCannotSupplyExecutableBehavior(t *testing.T) {
	for path, file := range productionGoFiles(t) {
		if !strings.Contains(path, filepath.Join("internal", "app")) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if !ok {
				return true
			}
			for _, name := range field.Names {
				lower := strings.ToLower(name.Name)
				for _, forbidden := range []string{"command", "script", "executable", "plugin", "wasm"} {
					if strings.Contains(lower, forbidden) {
						t.Errorf("%s exposes executable configuration field %q", path, name.Name)
					}
				}
			}
			return true
		})
	}
}

func TestMigrationsContainNoGenericRawDataColumns(t *testing.T) {
	root := filepath.Join(projectRoot(t), "internal", "storage", "sqlite", "migrations")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(body))
		for _, forbidden := range []string{"raw_payload", "raw_response", "raw_log", "debug_data", "extra_json", "arbitrary_metadata"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s contains forbidden storage field %q", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAPIAndReportUseExplicitDTOConstruction(t *testing.T) {
	required := map[string]map[string]bool{
		filepath.Join("internal", "api", "v1"):     {"ComponentDTO": false, "ObservationDTO": false, "FindingDTO": false, "DiagnosisDTO": false},
		filepath.Join("internal", "reports", "v1"): {"ComponentDTO": false, "ObservationDTO": false, "FindingDTO": false, "DiagnosisDTO": false, "Report": false},
	}
	for path, file := range productionGoFiles(t) {
		for directory, names := range required {
			if !strings.Contains(path, directory) {
				continue
			}
			for _, declaration := range file.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok || general.Tok != token.TYPE {
					continue
				}
				for _, specification := range general.Specs {
					typeSpec := specification.(*ast.TypeSpec)
					if _, exists := names[typeSpec.Name.Name]; exists {
						names[typeSpec.Name.Name] = true
					}
				}
			}
		}
	}
	for directory, names := range required {
		for name, found := range names {
			if !found {
				t.Errorf("%s lacks explicit %s transport type", directory, name)
			}
		}
	}
}
