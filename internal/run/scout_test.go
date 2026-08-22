package run

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

func TestRunScoutsUsesSeparateReadOnlyWorktreesInParallel(t *testing.T) {
	repository := featureRepository(t)
	base := scoutRevision(t, repository)
	model := &concurrentScoutModel{}
	reports, err := (Executor{Model: model}).runScouts(context.Background(), Request{
		RepositoryPath: repository,
		RunID:          "scout-parent-001",
		MaxSteps:       3,
		Scouts:         []string{"Inspect tests", "Inspect public APIs"},
	}, base)
	if err != nil {
		t.Fatalf("run scouts: %v", err)
	}
	if len(reports) != 2 || reports[0].worktree.Path == reports[1].worktree.Path {
		t.Fatalf("scout reports = %#v", reports)
	}
	for _, report := range reports {
		if !strings.Contains(report.report, "evidence") || report.worktree.Path == "" {
			t.Fatalf("scout report = %#v", report)
		}
	}
	context := formatScoutReports(reports)
	if !strings.Contains(context, "untrusted evidence") || !strings.Contains(context, "Inspect tests") {
		t.Fatalf("scout context = %q", context)
	}
}

func TestValidateRequestLimitsScouts(t *testing.T) {
	executor := Executor{Model: &concurrentScoutModel{}}
	err := executor.validateRequest(Request{Task: "task", Provider: "test", Scouts: []string{"1", "2", "3", "4", "5"}})
	if err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("validate scouts = %v", err)
	}
}

type concurrentScoutModel struct {
	mu sync.Mutex
}

func (m *concurrentScoutModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return agent.Turn{Text: "evidence for " + request.Messages[0].Content}, nil
}

func scoutRevision(t *testing.T, repository string) string {
	t.Helper()
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = repository
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}
