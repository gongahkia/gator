package engine

import (
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/runtime"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAgenticProfile(t *testing.T) {
	root := t.TempDir()
	workspace := runtime.Workspace{ArtifactsDir: root}
	run := domain.Run{ID: "run-1", Profile: domain.ProfileAgentic}
	files, err := generateApp(workspace, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no generated files")
	}
	if _, err := os.Stat(filepath.Join(root, "run-1", "generated-app", "backend", "harness.go")); err != nil {
		t.Fatal(err)
	}
	if report, err := verifyApp(workspace, run); err != nil || report["status"] != "pass" {
		t.Fatalf("report=%v err=%v", report, err)
	}
}
