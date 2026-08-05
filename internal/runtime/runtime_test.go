package runtime

import (
	"context"
	"strings"
	"testing"
)

type lifecycleRunner struct{ calls []string }

func (r *lifecycleRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--format {{json .}}") {
		return []byte(`{"Names":"norbot-run-frontend-1","State":"running"}`), nil
	}
	if len(args) > 0 && args[0] == "ps" && strings.Contains(joined, " -q") {
		return []byte("container-1\n"), nil
	}
	return nil, nil
}

func TestDeploymentLifecycleAndJSONArrayStatus(t *testing.T) {
	runner := &lifecycleRunner{}
	deployment := Deployment{DockerBin: "docker", Runner: runner}
	root := t.TempDir()
	status, err := deployment.Status(context.Background(), "run", root)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Services) != 1 {
		t.Fatalf("status=%#v", status)
	}
	if err := deployment.Start(context.Background(), "run", root); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Stop(context.Background(), "run", root); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Delete(context.Background(), "run", root); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	for _, expected := range []string{"start container-1", "stop container-1", "rm -f container-1", "network rm norbot-run-network"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q: %s", expected, joined)
		}
	}
}

func TestWorkspaceMirrorToVolumeCreatesTargetDirectory(t *testing.T) {
	runner := &lifecycleRunner{}
	workspace := Workspace{DockerBin: "docker", ArtifactsDir: t.TempDir(), Runner: runner}
	if _, err := workspace.WriteArtifact("run", "stage-output/planner-1.json", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := workspace.MirrorToVolume(context.Background(), "run", "stage-output/planner-1.json"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls, "\n")
	if !strings.Contains(joined, "exec norbot-ws-run mkdir -p /workspace/stage-output") {
		t.Fatalf("target directory was not created: %s", joined)
	}
	if !strings.Contains(joined, "cp "+workspace.RunPath("run")+"/stage-output/planner-1.json norbot-ws-run:/workspace/stage-output/planner-1.json") {
		t.Fatalf("artifact was not copied: %s", joined)
	}
}
