package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/runtime"
)

type verifyRunner struct{ commands []string }

func (r *verifyRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	return nil, nil
}

func TestGenerateAgenticProfile(t *testing.T) {
	root := t.TempDir()
	workspace := runtime.Workspace{ArtifactsDir: root}
	run := domain.Run{ID: "run-1", Profile: domain.ProfileAgentic}
	files, err := generateApp(workspace, run, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no generated files")
	}
	if _, err := os.Stat(filepath.Join(root, "run-1", "generated-app", "backend", "harness.go")); err != nil {
		t.Fatal(err)
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

func TestVerifierRunsDeterministicChecks(t *testing.T) {
	root := t.TempDir()
	runner := &verifyRunner{}
	workspace := runtime.Workspace{ArtifactsDir: root, DockerBin: "docker", Runner: runner}
	run := domain.Run{ID: "run-verify", Profile: domain.ProfileAgentic}
	if _, err := generateApp(workspace, run, nil); err != nil {
		t.Fatal(err)
	}
	report, err := verifyApp(context.Background(), workspace, run)
	if err != nil || report["status"] != "pass" {
		t.Fatalf("report=%v err=%v", report, err)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, required := range []string{"npm audit --omit=dev --audit-level=high", "govulncheck", "compose", "curlimages/curl"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing verifier command %q: %s", required, joined)
		}
	}
}

func TestAgenticTemplateWritesEnforcedToolPolicy(t *testing.T) {
	root := t.TempDir()
	policy := map[string]config.ToolPolicy{"http_get": {Enabled: true, Roles: []string{"researcher"}, AllowedHosts: []string{"api.example.test"}}}
	if _, err := generateApp(runtime.Workspace{ArtifactsDir: root}, domain.Run{ID: "policy", Profile: domain.ProfileAgentic}, policy); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "policy", "generated-app", "backend", "tool_policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "api.example.test") {
		t.Fatalf("policy=%s", data)
	}
}

func TestGeneratedProfileDockerE2E(t *testing.T) {
	if os.Getenv("NORBOT_E2E_DOCKER") != "1" {
		t.Skip("set NORBOT_E2E_DOCKER=1 to run Docker verification")
	}
	if output, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("Docker unavailable: %s", output)
	}
	root := t.TempDir()
	workspace := runtime.Workspace{ArtifactsDir: root, DockerBin: "docker", Runner: runtime.OSRunner{}}
	run := domain.Run{ID: "e2e" + time.Now().UTC().Format("150405"), Profile: domain.ProfileAgentic}
	if err := workspace.Ensure(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup(context.Background(), run.ID)
	if _, err := generateApp(workspace, run, nil); err != nil {
		t.Fatal(err)
	}
	if err := workspace.MirrorGeneratedApp(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	report, err := verifyApp(context.Background(), workspace, run)
	if err != nil {
		t.Fatalf("report=%v err=%v", report, err)
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
