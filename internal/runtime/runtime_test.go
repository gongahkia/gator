package runtime

import (
	"context"
	"strings"
	"testing"
)

type lifecycleRunner struct{ calls []string }

func (r *lifecycleRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	if strings.Contains(strings.Join(args, " "), " ps ") {
		return []byte(`[{"Name":"norbot-run-frontend-1","State":"running"}]`), nil
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
	for _, expected := range []string{"start --wait", " stop", "down --remove-orphans --volumes"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q: %s", expected, joined)
		}
	}
}
