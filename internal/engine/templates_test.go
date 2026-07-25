package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/runtime"
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
	command := exec.Command("go", "test", "./...")
	command.Dir = filepath.Join(root, "run-1", "generated-app", "backend")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated backend must compile: %v\n%s", err, output)
	}
	compose := exec.Command("docker", "compose", "config", "--quiet")
	compose.Dir = filepath.Join(root, "run-1", "generated-app")
	compose.Env = append(os.Environ(), "NORBOT_PUBLIC_PORT=31000")
	if output, err := compose.CombinedOutput(); err != nil {
		t.Fatalf("generated compose must validate: %v\n%s", err, output)
	}
}

func TestBuilderResponseIsBoundedToGeneratedApp(t *testing.T) {
	workspace := runtime.Workspace{ArtifactsDir: t.TempDir()}
	run := domain.Run{ID: "run-2"}
	files, err := applyBuilderResponse(workspace, run, `{"files":{"generated-app/frontend/src/app.jsx":"export default null"}}`)
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	if _, err := applyBuilderResponse(workspace, run, `{"files":{"../escape":"bad"}}`); err == nil {
		t.Fatal("expected unsafe path failure")
	}
}

func TestPlannerGraphResponse(t *testing.T) {
	graph, ok := graphFromResponse(`{"graph":{"nodes":[{"id":"input","label":"Input","kind":"input"},{"id":"output","label":"Output","kind":"output"}],"edges":[{"id":"input-to-output","source":"input","target":"output"}]}}`)
	if !ok || len(graph.Nodes) != 2 {
		t.Fatalf("graph=%#v ok=%t", graph, ok)
	}
}
