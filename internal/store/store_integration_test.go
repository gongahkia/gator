package store

import (
	"context"
	"errors"
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

func TestCreateRunWithInitialJobIsAtomicIntegration(t *testing.T) {
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
	id := "atomic-" + time.Now().UTC().Format("20060102150405.000000000")
	run := domain.Run{ID: id, Prompt: "atomic", Profile: domain.ProfileFrontend, Stage: domain.StagePlanner, Status: domain.StatusQueued, Providers: map[domain.Stage]string{domain.StagePlanner: "test", domain.StageBuilder: "test", domain.StageVerifier: "test", domain.StageDeployer: "local-deployer"}, Graph: domain.DefaultGraph(), Architecture: domain.DefaultArchitecture(domain.ProfileFrontend, domain.DefaultGraph()), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := st.CreateRunWithInitialJob(ctx, run, []string{"missing-skill"}, ""); err == nil {
		t.Fatal("expected inactive skill failure")
	}
	if _, err := st.GetRun(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("run survived failed transaction: %v", err)
	}
	for _, table := range []string{"jobs", "run_events", "outbox_events"} {
		var count int
		if err := st.pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table+" WHERE run_id=$1", id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d partial rows", table, count)
		}
	}
	if err := st.CreateRunWithInitialJob(ctx, run, nil, ""); err != nil {
		t.Fatal(err)
	}
	created, err := st.GetRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if created.WorkspaceStatus != "provisioning" {
		t.Fatalf("workspace status=%q", created.WorkspaceStatus)
	}
	var state string
	if err := st.pool.QueryRow(ctx, `SELECT state FROM jobs WHERE run_id=$1`, id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "blocked" {
		t.Fatalf("initial job state=%q", state)
	}
	var provisioned int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE run_id=$1 AND event_type='workspace.provision'`, id).Scan(&provisioned); err != nil {
		t.Fatal(err)
	}
	if provisioned != 1 {
		t.Fatalf("workspace outbox count=%d", provisioned)
	}
}
