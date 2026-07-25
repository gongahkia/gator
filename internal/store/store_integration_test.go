package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gongahkia/norbot/internal/domain"
)

func TestRunApprovalLifecycleIntegration(t *testing.T) {
	databaseURL := os.Getenv("NORBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set NORBOT_TEST_DATABASE_URL to run Postgres integration coverage")
	}
	ctx := context.Background()
	st, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	run := domain.Run{
		ID: "test-" + time.Now().UTC().Format("20060102150405.000000000"), Prompt: "test", Profile: domain.ProfileFullStack,
		Stage: domain.StagePlanner, Status: domain.StatusQueued,
		Providers: map[domain.Stage]string{domain.StagePlanner: "test", domain.StageBuilder: "test", domain.StageVerifier: "test", domain.StageDeployer: "local-deployer"},
		Graph:     domain.DefaultGraph(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := st.Enqueue(ctx, run.ID, domain.StagePlanner, 1); err != nil {
		t.Fatal(err)
	}
	job, claimed, err := st.ClaimJob(ctx, "integration-worker", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim=%t err=%v", claimed, err)
	}
	if err := st.MarkStageRunning(ctx, job, "test"); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkStageAwaitingApproval(ctx, job, nil); err != nil {
		t.Fatal(err)
	}
	updated, err := st.Approve(ctx, run.ID, domain.ApprovalApprove, "")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Stage != domain.StageBuilder || updated.Status != domain.StatusQueued {
		t.Fatalf("run=%#v", updated)
	}
}
