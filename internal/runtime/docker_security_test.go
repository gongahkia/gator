package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
)

type dockerSecurityRunner struct {
	calls []string
	envs  [][]string
}

func (r *dockerSecurityRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	if len(args) > 0 && args[0] == "ps" && strings.Contains(strings.Join(args, " "), " -q") {
		return []byte{}, nil
	}
	return nil, nil
}

func (r *dockerSecurityRunner) RunWithEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	r.envs = append(r.envs, append([]string(nil), env...))
	if len(args) >= 2 && args[0] == "info" {
		return []byte(`["name=rootless"]`), nil
	}
	return r.Run(ctx, name, args...)
}

func TestPrepareDeploymentSourceArchivesModelDescriptors(t *testing.T) {
	root := t.TempDir()
	run := domain.Run{ID: "run-1", Profile: domain.ProfileFullStack}
	for path, content := range map[string]string{
		"frontend/package.json": "{}", "frontend/package-lock.json": "{}", "frontend/src/main.jsx": "export default null", "backend/main.go": "package main", "backend/go.mod": "module generated/backend\n\ngo 1.26.0\n", "backend/go.sum": "",
		"docker-compose.yml": "services:\n  attacker:\n    privileged: true\n", "frontend/Dockerfile": "FROM attacker", ".norbot/deployment/frontend.Dockerfile": "FROM attacker",
	} {
		full := filepath.Join(root, "generated-app", path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := PrepareDeploymentSource(root, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Archived) != 3 {
		t.Fatalf("archived=%#v", report.Archived)
	}
	for _, path := range []string{"docker-compose.yml", "frontend/Dockerfile"} {
		if _, err := os.Stat(filepath.Join(root, "generated-app", path)); !os.IsNotExist(err) {
			t.Fatalf("reserved descriptor remains %s: %v", path, err)
		}
	}
	protected, err := os.ReadFile(filepath.Join(root, "generated-app", ".norbot", "deployment", "frontend.Dockerfile"))
	if err != nil || strings.Contains(string(protected), "attacker") || !strings.Contains(string(protected), "COPY frontend/") {
		t.Fatalf("protected descriptor=%q err=%v", protected, err)
	}
}

func TestDeploymentNeverInvokesComposeAndHardensContainers(t *testing.T) {
	root := t.TempDir()
	run := domain.Run{ID: "run-1", AppID: "app-1", Profile: domain.ProfileFullStack}
	for path, content := range map[string]string{
		"frontend/package.json": "{}", "frontend/package-lock.json": "{}", "frontend/src/main.jsx": "export default null", "backend/main.go": "package main", "backend/go.mod": "module generated/backend\n\ngo 1.26.0\n", "backend/go.sum": "",
	} {
		full := filepath.Join(root, "generated-app", path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runner := &dockerSecurityRunner{}
	deployment := Deployment{Docker: LegacyDockerClient("docker", runner)}
	if _, err := deployment.Deploy(context.Background(), run, root); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	if strings.Contains(joined, " compose ") {
		t.Fatalf("deployment executed Compose:\n%s", joined)
	}
	for _, required := range []string{"--file " + filepath.Join(root, "generated-app", ".norbot", "deployment", "frontend.Dockerfile"), "--file " + filepath.Join(root, "generated-app", ".norbot", "deployment", "ingress.Dockerfile"), "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--publish 127.0.0.1:", "network create --internal", "network connect --alias ingress"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing %q:\n%s", required, joined)
		}
	}
	for _, call := range runner.calls {
		if strings.Contains(call, "--label norbot.component=frontend") && strings.Contains(call, "--publish") {
			t.Fatalf("frontend must remain on the internal network:\n%s", call)
		}
	}
	if strings.Contains(joined, "--publish 0.0.0.0") || strings.Contains(joined, "--publish 127.0.0.1:0:8000") {
		t.Fatalf("backend or public binding is unsafe:\n%s", joined)
	}
}

func TestLocalDockerClientRequiresExplicitAcknowledgement(t *testing.T) {
	t.Setenv("NORBOT_ALLOW_UNSAFE_LOCAL_DOCKER_SOCKET", "true")
	runner := &dockerSecurityRunner{}
	client := NewDockerClient("docker", config.Docker{Mode: config.DockerModeUnsafeLocalSocket}, runner)
	if err := client.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Run(context.Background(), "ps"); err != nil {
		t.Fatal(err)
	}
	if len(runner.envs) != 1 || len(runner.envs[0]) != 0 {
		t.Fatalf("local Docker client should not inject remote TLS env: %#v", runner.envs)
	}
}

func TestDockerClientRecordsCommands(t *testing.T) {
	runner := &dockerSecurityRunner{}
	recorder := &CommandRecorder{}
	client := LegacyDockerClient("docker", runner).WithRecorder(recorder)
	if _, err := client.Run(context.Background(), "ps", "-a"); err != nil {
		t.Fatal(err)
	}
	records := recorder.Records()
	if len(records) != 1 || records[0].Command != "docker" || records[0].Args[0] != "ps" || records[0].ExitCode != 0 {
		t.Fatalf("records=%#v", records)
	}
}

func TestCommandRecorderArchivesFullRedactedOutput(t *testing.T) {
	const output = "token=super-secret " + "x"
	runner := commandOutputRunner{output: []byte(output + strings.Repeat("z", 70<<10))}
	recorder := &CommandRecorder{}
	client := LegacyDockerClient("docker", runner).WithRecorder(recorder)
	if _, err := client.Run(context.Background(), "logs", "app"); err != nil {
		t.Fatal(err)
	}
	compact := recorder.Records()[0]
	if !compact.OutputTruncated || strings.Contains(compact.Output, "super-secret") {
		t.Fatalf("compact=%#v", compact)
	}
	full := recorder.FullRecords()[0]
	if full.OutputTruncated || len(full.Output) <= 70<<10 || strings.Contains(full.Output, "super-secret") || !strings.Contains(full.Output, "token=[REDACTED]") {
		t.Fatalf("full=%#v", full)
	}
}

type commandOutputRunner struct{ output []byte }

func (r commandOutputRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return r.output, nil
}
