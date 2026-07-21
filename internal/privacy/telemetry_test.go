package privacy

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestTelemetryIsUnconditionallyDisabled(t *testing.T) {
	if TelemetryEnabled || CheckTelemetry() != ErrTelemetryDisabled {
		t.Fatalf("telemetry boundary = enabled:%t err:%v", TelemetryEnabled, CheckTelemetry())
	}
}

func TestRuntimeSourcesContainNoTelemetrySDKs(t *testing.T) {
	root := repositoryRoot(t)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			for _, forbidden := range ForbiddenTelemetryImportPrefixes {
				if strings.HasPrefix(importPath, forbidden) {
					t.Fatalf("telemetry import %q in %s", importPath, path)
				}
			}
			if importPath == "net/http" && !strings.HasPrefix(path, filepath.Join(root, "internal", "llm")+string(filepath.Separator)) {
				t.Fatalf("http runtime boundary bypass in %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repository root: %v", err)
	}
	return root
}
