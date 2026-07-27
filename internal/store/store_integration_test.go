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
	t.Cleanup(st.Close)
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

func TestMigrationsRecordImmutableLedgerIntegration(t *testing.T) {
	databaseURL := os.Getenv("NORBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set NORBOT_TEST_DATABASE_URL to run Postgres integration coverage")
	}
	ctx := context.Background()
	st, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second migration must be a no-op: %v", err)
	}
	var count int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(migrations) {
		t.Fatalf("migration ledger count=%d migrations=%d", count, len(migrations))
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
	t.Cleanup(st.Close)
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
	t.Cleanup(func() { _ = st.DeleteRun(context.Background(), id) })
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
	var outboxID int64
	if err := st.pool.QueryRow(ctx, `SELECT id FROM outbox_events WHERE run_id=$1 AND event_type='workspace.provision'`, id).Scan(&outboxID); err != nil {
		t.Fatal(err)
	}
	if recorded, err := st.RecordOutboxReceipt(ctx, outboxID, "integration-sink", map[string]any{"result": "ok"}); err != nil || !recorded {
		t.Fatalf("first receipt recorded=%t err=%v", recorded, err)
	}
	if recorded, err := st.RecordOutboxReceipt(ctx, outboxID, "integration-sink", map[string]any{"result": "duplicate"}); err != nil || recorded {
		t.Fatalf("duplicate receipt recorded=%t err=%v", recorded, err)
	}
	if _, err := st.ReplayDeadOutbox(ctx, outboxID, "operator", "no"); err == nil {
		t.Fatal("short replay reason accepted")
	}
	if _, err := st.pool.Exec(ctx, `UPDATE outbox_events SET state='dead',dead_lettered_at=now() WHERE id=$1`, outboxID); err != nil {
		t.Fatal(err)
	}
	if replayed, err := st.ReplayDeadOutbox(ctx, outboxID, "operator", "fixed downstream configuration"); err != nil || replayed.State != "queued" || replayed.Attempts != 0 {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
}

func TestFinalizeApprovalOperationIntegration(t *testing.T) {
	databaseURL := os.Getenv("NORBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set NORBOT_TEST_DATABASE_URL to run Postgres integration coverage")
	}
	ctx := context.Background()
	st, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	id := "approval-" + time.Now().UTC().Format("20060102150405.000000000")
	run := domain.Run{ID: id, Prompt: "approval", Profile: domain.ProfileFrontend, Stage: domain.StageBuilder, Status: domain.StatusAwaiting, Providers: map[domain.Stage]string{domain.StagePlanner: "test", domain.StageBuilder: "test", domain.StageVerifier: "test", domain.StageDeployer: "local-deployer"}, Graph: domain.DefaultGraph(), Architecture: domain.DefaultArchitecture(domain.ProfileFrontend, domain.DefaultGraph()), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteRun(context.Background(), id) })
	revision, err := st.CreateRevision(ctx, domain.Revision{RunID: id, Kind: domain.ReviewCode, Attempt: 1, BaselineDigest: "sha256:baseline", PatchDigest: "sha256:patch", Files: map[string]string{"generated-app/frontend/src/main.jsx": "after"}, Report: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	op, err := st.CreateApprovalOperation(ctx, domain.ApprovalOperation{RunID: id, RevisionID: revision.ID, BaselineDigest: revision.BaselineDigest, PostDigest: "sha256:post", BaselineFiles: map[string]string{"generated-app/frontend/src/main.jsx": "before"}})
	if err != nil {
		t.Fatal(err)
	}
	if op.State != "prepared" {
		t.Fatalf("journal state=%q", op.State)
	}
	if current, err := st.Revision(ctx, id, revision.ID); err != nil || current.State != "proposed" {
		t.Fatalf("journal mutated revision: %#v err=%v", current, err)
	}
	if err := st.MarkApprovalWorkspaceApplied(ctx, op.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := st.FinalizeApprovalOperation(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Stage != domain.StageVerifier || updated.Status != domain.StatusQueued {
		t.Fatalf("finalized run=%#v", updated)
	}
	if current, err := st.Revision(ctx, id, revision.ID); err != nil || current.State != "applied" {
		t.Fatalf("revision=%#v err=%v", current, err)
	}
	if current, err := st.ApprovalOperation(ctx, op.ID); err != nil || current.State != "finalized" {
		t.Fatalf("operation=%#v err=%v", current, err)
	}
	var jobs, events int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE run_id=$1 AND stage='verifier' AND state='queued'`, id).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE run_id=$1 AND event_type='run.event'`, id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || events < 2 {
		t.Fatalf("jobs=%d run event outbox=%d", jobs, events)
	}
}
