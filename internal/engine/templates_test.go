package engine

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
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

func TestAcceptanceContractAndScreenshotDifference(t *testing.T) {
	architecture := domain.DefaultArchitecture(domain.ProfileFrontend, domain.DefaultGraph())
	architecture.Acceptance = domain.CompileAcceptance(architecture)
	if err := architecture.Acceptance.Validate(); err != nil || architecture.Acceptance.Digest() == "" {
		t.Fatalf("acceptance=%#v err=%v", architecture.Acceptance, err)
	}
	imageBytes := func(value color.Color) []byte {
		image := image.NewRGBA(image.Rect(0, 0, 2, 2))
		image.Set(0, 0, value)
		var output bytes.Buffer
		if err := png.Encode(&output, image); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	if difference, err := screenshotDifference(imageBytes(color.Black), imageBytes(color.Black)); err != nil || difference != 0 {
		t.Fatalf("difference=%v err=%v", difference, err)
	}
	if difference, err := screenshotDifference(imageBytes(color.Black), imageBytes(color.White)); err != nil || difference <= 0 {
		t.Fatalf("difference=%v err=%v", difference, err)
	}
}

func TestRepairProposalIsBounded(t *testing.T) {
	proposal := proposeRepair(errors.New("frontend test/build/audit: npm test failed"), nil)
	if proposal["classification"] != "frontend_verification" || proposal["digest"] == "" {
		t.Fatalf("proposal=%#v", proposal)
	}
	run := domain.Run{Profile: domain.ProfileFrontend, Feedback: "bounded remediation: {\"allowed_paths\":[\"generated-app/frontend/\"]}"}
	if err := validateBuilderFiles(run, map[string]string{"generated-app/backend/main.go": "bad", "generated-app/frontend/src/main.jsx": "ok"}); err == nil {
		t.Fatal("repair scope accepted backend change")
	}
}

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
	for _, path := range []string{"docker-compose.yml", "frontend/Dockerfile", "backend/Dockerfile"} {
		if _, err := os.Stat(filepath.Join(root, "run-1", "generated-app", path)); !os.IsNotExist(err) {
			t.Fatalf("model-owned deployment descriptor %s must not be generated: %v", path, err)
		}
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
	for _, required := range []string{"npm audit --omit=dev --audit-level=high", "govulncheck", ".norbot/deployment/frontend.Dockerfile", "curlimages/curl"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing verifier command %q: %s", required, joined)
		}
	}
	if !strings.Contains(joined, "export PATH GOMODCACHE GOCACHE") {
		t.Fatalf("Go verifier environment is not preserved across commands: %s", joined)
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
	run := domain.Run{ID: "e2e" + time.Now().UTC().Format("150405"), Profile: domain.ProfileAgentic, Architecture: domain.Architecture{Acceptance: domain.AcceptanceContract{Version: 1, Flows: []domain.AcceptanceFlow{{ID: "home", Name: "generated home", Steps: []domain.AcceptanceStep{{Kind: "goto", URL: "/"}, {Kind: "expect_text", Text: "Norbot generated app"}}}}, Accessibility: []domain.AccessibilityCheck{{ID: "home", FlowID: "home"}}, Screenshots: []domain.ScreenshotExpectation{{ID: "home", FlowID: "home", Path: "/"}}}}}
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

func TestBuilderResponseTargetsDeployableSource(t *testing.T) {
	run := domain.Run{Profile: domain.ProfileFullStack}
	if err := validateBuilderFiles(run, map[string]string{"generated-app/frontend/src/main.jsx": "export default null"}); err != nil {
		t.Fatal(err)
	}
	err := validateBuilderFiles(run, map[string]string{"generated-app/index.html": "<main />"})
	validation, ok := err.(builderResponseError)
	if !ok || validation.reason != "path_outside_deployable_source" || validation.invalidPath != "generated-app/index.html" {
		t.Fatalf("validation=%#v", validation)
	}
	if diagnostics := builderResponseDiagnostics(err); diagnostics["required_path_prefix"] != "generated-app/frontend/" {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
}

func TestBuilderResponseRejectsDeploymentDescriptors(t *testing.T) {
	run := domain.Run{Profile: domain.ProfileFullStack}
	for _, path := range []string{"generated-app/docker-compose.yml", "generated-app/frontend/Dockerfile", "generated-app/backend/.dockerignore", "generated-app/.norbot/deployment/frontend.Dockerfile", "generated-app/.norbot/deployment/ingress.Dockerfile"} {
		err := validateBuilderFiles(run, map[string]string{path: "attacker", "generated-app/frontend/src/main.jsx": "export default null"})
		validation, ok := err.(builderResponseError)
		if !ok || validation.reason != "server_owned_deployment_descriptor" || validation.invalidPath != path {
			t.Fatalf("path=%s validation=%#v", path, validation)
		}
	}
}

func TestVerifyRejectsRevisionWithoutDeployableSource(t *testing.T) {
	report, err := verifyApp(context.Background(), nil, domain.Run{Profile: domain.ProfileFullStack}, map[string]string{"generated-app/index.html": "<main />"})
	if err == nil || report["status"] != "fail" || report["summary"] != "Builder revision does not modify deployable application source." {
		t.Fatalf("report=%#v err=%v", report, err)
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

func TestFirstPlannerPromptDoesNotTreatTemplateAsApproved(t *testing.T) {
	run := domain.Run{ID: "fresh", Profile: domain.ProfileFrontend, Prompt: "Build a task tracker", Architecture: domain.DefaultArchitecture(domain.ProfileFrontend, domain.DefaultGraph())}
	prompt := stagePrompt(run, domain.StagePlanner, false, nil)
	if !strings.Contains(prompt, "first planner pass") || !strings.Contains(prompt, "press_key requires key and may omit a target") || !strings.Contains(prompt, "local_storage supports {\"op\":\"clear\"}") || !strings.Contains(prompt, "Every workflow edge requires id, source, and target; do not use from or to.") || strings.Contains(prompt, "Approved architecture:") {
		t.Fatalf("prompt=%s", prompt)
	}
	revision := run
	revision.Feedback = "Add local persistence"
	prompt = stagePrompt(revision, domain.StagePlanner, false, nil)
	if !strings.Contains(prompt, "Previous planner draft") || !strings.Contains(prompt, revision.Feedback) {
		t.Fatalf("revision prompt=%s", prompt)
	}
}

func TestPlannerResponseAcceptsSelectorContract(t *testing.T) {
	run := domain.Run{Profile: domain.ProfileFrontend}
	response := `{"architecture":{"app_name":"Task Tracker","app_type":"frontend-only","stack":["HTML5","Vanilla JavaScript"],"integrations":[],"core_features":[{"id":"tasks","name":"Task CRUD","description":"Add and complete tasks","role":"app_logic","selected":true}],"optional_features":[],"workflow":{"nodes":[{"id":"input","label":"Input","kind":"input"},{"id":"logic","label":"Logic","kind":"logic"},{"id":"output","label":"Output","kind":"output"}],"edges":[{"id":"input-to-logic","source":"input","target":"logic"},{"id":"logic-to-output","source":"logic","target":"output"}]},"acceptance":{"version":1,"flows":[{"id":"tasks","name":"Task CRUD","steps":[{"kind":"local_storage","op":"clear"},{"kind":"goto","url":"/"},{"kind":"set_value","selector":"[data-testid=\"new-task\"]","value":"Buy milk"},{"kind":"press_key","selector":"[data-testid=\"new-task\"]","key":"Enter"},{"kind":"expect_text","selector":"[data-testid=\"task-title\"]","text":"Buy milk"},{"kind":"expect_count","selector":"[data-testid=\"task\"]","count":1},{"kind":"reload"}]}],"api_contracts":[],"seed_data":[],"accessibility":[{"id":"home","selector":"[data-testid=\"app-root\"]"}],"screenshots":[{"id":"home","path":"/"}]}}}`
	architecture, err := architectureFromResponse(response, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(architecture.Stack) != 2 || len(architecture.Acceptance.Flows) != 1 {
		t.Fatalf("architecture=%#v", architecture)
	}
}

func TestPlannerResponseNormalizesLegacyWorkflowEdges(t *testing.T) {
	run := domain.Run{Profile: domain.ProfileFrontend}
	response := `{"architecture":{"app_name":"Task Tracker","app_type":"frontend-only","stack":["HTML5","Vanilla JavaScript"],"integrations":[],"core_features":[{"id":"tasks","name":"Task CRUD","description":"Add and complete tasks","role":"app_logic","selected":true}],"optional_features":[],"workflow":{"nodes":[{"id":"input","label":"Input","kind":"input"},{"id":"logic","label":"Logic","kind":"logic"},{"id":"output","label":"Output","kind":"output"}],"edges":[{"from":"input","to":"logic"},{"from":"logic","to":"output"}]},"acceptance":{"version":1,"flows":[{"id":"tasks","name":"Task CRUD","steps":[{"kind":"goto","url":"/"},{"kind":"expect_visible","selector":"[data-testid=\"app-root\"]"}]}],"api_contracts":[],"seed_data":[],"accessibility":[{"id":"home","selector":"[data-testid=\"app-root\"]"}],"screenshots":[{"id":"home","path":"/"}]}}}`
	architecture, err := architectureFromResponse(response, run)
	if err != nil {
		t.Fatal(err)
	}
	edges := architecture.Workflow.Edges
	if len(edges) != 2 || edges[0] != (domain.GraphEdge{ID: "planner-edge-1", Source: "input", Target: "logic"}) || edges[1] != (domain.GraphEdge{ID: "planner-edge-2", Source: "logic", Target: "output"}) {
		t.Fatalf("edges=%#v", edges)
	}
}

func TestPlannerResponseRejectsIncompleteLegacyWorkflowEdge(t *testing.T) {
	response := `{"architecture":{"app_name":"Task Tracker","app_type":"frontend-only","stack":["HTML5"],"integrations":[],"core_features":[{"id":"tasks","name":"Task CRUD","description":"Add tasks","role":"app_logic","selected":true}],"optional_features":[],"workflow":{"nodes":[{"id":"input","label":"Input","kind":"input"},{"id":"output","label":"Output","kind":"output"}],"edges":[{"from":"input"}]},"acceptance":{"version":1,"flows":[{"id":"tasks","name":"Task CRUD","steps":[{"kind":"goto","url":"/"},{"kind":"expect_visible","selector":"[data-testid=\"app-root\"]"}]}],"api_contracts":[],"seed_data":[],"accessibility":[{"id":"home","selector":"[data-testid=\"app-root\"]"}],"screenshots":[{"id":"home","path":"/"}]}}}`
	_, err := architectureFromResponse(response, domain.Run{Profile: domain.ProfileFrontend})
	if err == nil || !strings.Contains(err.Error(), "every edge requires id, source, and target") {
		t.Fatalf("err=%v", err)
	}
}

func TestPlannerResponseReportsInvalidContract(t *testing.T) {
	_, err := architectureFromResponse(`{"architecture":{"app_name":"Task Tracker"}}`, domain.Run{Profile: domain.ProfileFrontend})
	if err == nil {
		t.Fatal("invalid planner contract accepted")
	}
	diagnostics := plannerResponseDiagnostics(err)
	if diagnostics["reason"] != "invalid_contract" {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
}
