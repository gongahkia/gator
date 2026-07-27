package engine

import (
	"context"
	"errors"
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
	if _, err := os.Stat(filepath.Join(root, "run-1", "generated-app", "backend", "AGENT_RUNTIME.md")); err != nil {
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
	if !strings.Contains(joined, "norbot_verify_node_") || !strings.Contains(joined, "norbot_verify_go_") {
		t.Fatalf("dependency cache volumes were not used: %s", joined)
	}
	if !strings.Contains(joined, ".norbot-lock") {
		t.Fatalf("dependency cache population is not serialized: %s", joined)
	}
	if cache, ok := report["cache"].(map[string]any); !ok || cache["node"] != false || cache["go"] != false {
		t.Fatalf("cache report=%#v", report["cache"])
	}
}

func TestProviderRateLimited(t *testing.T) {
	if !providerRateLimited(errors.New("provider status 429: retry later")) || providerRateLimited(errors.New("provider status 500")) {
		t.Fatal("provider rate-limit classification is incorrect")
	}
}

func TestDependencyCacheKeyTracksLockfilesAndToolchain(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "frontend"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "frontend", "package.json"), []byte(`{"name":"app"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "frontend", "package-lock.json"), []byte(`{"lockfileVersion":3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := dependencyCacheKey(root, "node:22-alpine", "frontend/package.json", "frontend/package-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	second, err := dependencyCacheKey(root, "node:23-alpine", "frontend/package.json", "frontend/package-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("toolchain did not invalidate cache")
	}
	if err := os.WriteFile(filepath.Join(root, "frontend", "package-lock.json"), []byte(`{"lockfileVersion":4}`), 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := dependencyCacheKey(root, "node:22-alpine", "frontend/package.json", "frontend/package-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("lockfile did not invalidate cache")
	}
}

func TestAgenticTemplateDoesNotShipToolExecutor(t *testing.T) {
	root := t.TempDir()
	policy := map[string]config.ToolPolicy{"http_get": {Enabled: true, Roles: []string{"researcher"}, AllowedHosts: []string{"api.example.test"}}}
	if _, err := generateApp(runtime.Workspace{ArtifactsDir: root}, domain.Run{ID: "policy", Profile: domain.ProfileAgentic}, policy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "policy", "generated-app", "backend", "harness.go")); !os.IsNotExist(err) {
		t.Fatalf("agent tool executor must not ship: %v", err)
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
	_, err = applyBuilderResponse(workspace, run, `{"files":{"../escape":"bad"}}`)
	if err == nil {
		t.Fatal("expected unsafe path failure")
	}
	validation, ok := err.(builderResponseError)
	if !ok || validation.reason != "path_outside_generated_app" || validation.invalidPath != "../escape" || validation.fileCount != 1 {
		t.Fatalf("validation=%#v", validation)
	}
	diagnostics := builderResponseDiagnostics(err)
	if diagnostics["required_path_prefix"] != "generated-app/" || diagnostics["invalid_path"] != "../escape" {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
}

func TestWriteStageArtifactPersistsBuilderResponse(t *testing.T) {
	runner := &verifyRunner{}
	workspace := runtime.Workspace{ArtifactsDir: t.TempDir(), DockerBin: "docker", Runner: runner}
	artifact := map[string]any{"response": `{"files":{"index.html":"<main />"}}`, "builder_validation": map[string]any{"reason": "path_outside_generated_app"}}
	if err := writeStageArtifact(context.Background(), workspace, "run-artifact", "stage-output/builder-1.json", artifact); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(workspace.RunPath("run-artifact"), "stage-output", "builder-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "builder_validation") || !strings.Contains(string(content), "index.html") {
		t.Fatalf("artifact=%s", content)
	}
	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "cp "+workspace.RunPath("run-artifact")+"/stage-output/builder-1.json norbot-ws-run-artifact:/workspace/stage-output/builder-1.json") {
		t.Fatalf("artifact was not mirrored: %s", joined)
	}
}

func TestPlannerGraphResponse(t *testing.T) {
	graph, ok := graphFromResponse(`{"graph":{"nodes":[{"id":"input","label":"Input","kind":"input"},{"id":"output","label":"Output","kind":"output"}],"edges":[{"id":"input-to-output","source":"input","target":"output"}]}}`)
	if !ok || len(graph.Nodes) != 2 {
		t.Fatalf("graph=%#v ok=%t", graph, ok)
	}
}
