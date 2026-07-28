package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/observability"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceAndForensicsIntegration(t *testing.T) {
	databaseURL := os.Getenv("NORBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set NORBOT_TEST_DATABASE_URL to run Postgres integration coverage")
	}
	key := bytes.Repeat([]byte{9}, 32)
	t.Setenv("NORBOT_FORENSICS_TEST_KEY", base64.StdEncoding.EncodeToString(key))
	ctx := context.Background()
	st, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.ConfigureForensics(true, "NORBOT_FORENSICS_TEST_KEY"); err != nil {
		t.Fatal(err)
	}
	id := "trace-" + time.Now().UTC().Format("20060102150405.000000000")
	run := domain.Run{ID: id, Prompt: "trace", Profile: domain.ProfileFrontend, Stage: domain.StagePlanner, Status: domain.StatusQueued, Providers: map[domain.Stage]string{domain.StagePlanner: "test", domain.StageBuilder: "test", domain.StageVerifier: "test", domain.StageDeployer: "local-deployer"}, Graph: domain.DefaultGraph(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteRun(context.Background(), id) })
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	ctx = trace.ContextWithSpanContext(ctx, span)
	ctx = observability.With(ctx, observability.Correlation{RunID: id, Stage: "planner", Attempt: 1, TurnID: "turn-1"})
	event, err := st.RecordTrace(ctx, domain.TraceEvent{RunID: id, Type: "agent_turn_created", Summary: "agent received prompt"}, map[string]any{"prompt": "local secret"}, 30)
	if err != nil {
		t.Fatal(err)
	}
	page, err := st.TracePage(ctx, id, domain.TraceFilter{Query: "agent prompt"}, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].TraceID != span.TraceID().String() || !page.Items[0].RawAvailable {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	raw, err := st.TraceRaw(ctx, id, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if raw.(map[string]any)["prompt"] != "local secret" {
		t.Fatalf("raw=%#v", raw)
	}
}

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

func TestEventsAfterIntegration(t *testing.T) {
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
	before, err := st.LatestEventID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	run := domain.Run{ID: "events-" + time.Now().UTC().Format("20060102150405.000000000"), Prompt: "events", Profile: domain.ProfileFrontend, Stage: domain.StagePlanner, Status: domain.StatusQueued, Providers: map[domain.Stage]string{domain.StagePlanner: "test", domain.StageBuilder: "test", domain.StageVerifier: "test", domain.StageDeployer: "local-deployer"}, Graph: domain.DefaultGraph(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteRun(context.Background(), run.ID) })
	if err := st.RecordEvent(ctx, run.ID, "first", "first event", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordEvent(ctx, run.ID, "second", "second event", nil); err != nil {
		t.Fatal(err)
	}
	events, err := st.EventsAfter(ctx, before, 500)
	owned := []domain.Event{}
	for _, event := range events {
		if event.RunID == run.ID {
			owned = append(owned, event)
		}
	}
	if err != nil || len(owned) != 2 || owned[0].Type != "first" || owned[1].Type != "second" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	latest, err := st.LatestEventID(ctx)
	if err != nil || latest < owned[1].ID {
		t.Fatalf("latest=%d event=%#v err=%v", latest, owned[1], err)
	}
}

func TestBuilderRevisionRequestIntegration(t *testing.T) {
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
	id := "builder-revision-" + time.Now().UTC().Format("20060102150405.000000000")
	run := domain.Run{ID: id, Prompt: "test", Profile: domain.ProfileFrontend, Stage: domain.StageBuilder, Status: domain.StatusAwaiting, Providers: map[domain.Stage]string{domain.StagePlanner: "test", domain.StageBuilder: "test", domain.StageVerifier: "test", domain.StageDeployer: "local-deployer"}, Graph: domain.DefaultGraph(), Architecture: domain.DefaultArchitecture(domain.ProfileFrontend, domain.DefaultGraph()), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteRun(context.Background(), id) })
	updated, err := st.Approve(ctx, id, domain.ApprovalRevise, "move browser assets into generated-app/frontend")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Stage != domain.StageBuilder || updated.Status != domain.StatusQueued || updated.Feedback == "" {
		t.Fatalf("run=%#v", updated)
	}
	var attempt int
	if err := st.pool.QueryRow(ctx, `SELECT attempt FROM jobs WHERE run_id=$1 AND stage='builder'`, id).Scan(&attempt); err != nil || attempt != 1 {
		t.Fatalf("attempt=%d err=%v", attempt, err)
	}
	var events int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM run_events WHERE run_id=$1 AND event_type='builder_revision_requested'`, id).Scan(&events); err != nil || events != 1 {
		t.Fatalf("events=%d err=%v", events, err)
	}
}

func TestRecoverExpiredJobsIntegration(t *testing.T) {
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
	id := "expired-job-" + time.Now().UTC().Format("20060102150405.000000000")
	run := domain.Run{ID: id, Prompt: "test", Profile: domain.ProfileFrontend, Stage: domain.StageVerifier, Status: domain.StatusQueued, Providers: map[domain.Stage]string{domain.StagePlanner: "test", domain.StageBuilder: "test", domain.StageVerifier: "test", domain.StageDeployer: "local-deployer"}, Graph: domain.DefaultGraph(), Architecture: domain.DefaultArchitecture(domain.ProfileFrontend, domain.DefaultGraph()), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteRun(context.Background(), id) })
	if err := st.Enqueue(ctx, id, domain.StageVerifier, 1); err != nil {
		t.Fatal(err)
	}
	job, claimed, err := st.ClaimJob(ctx, "expired-worker", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claimed=%t err=%v", claimed, err)
	}
	if err := st.MarkStageRunning(ctx, job, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, `UPDATE jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := st.RecoverExpiredJobs(ctx)
	if err != nil || recovered < 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	updated, err := st.GetRun(ctx, id)
	if err != nil || updated.Status != domain.StatusInterrupted {
		t.Fatalf("run=%#v err=%v", updated, err)
	}
	var state, eventType string
	if err := st.pool.QueryRow(ctx, `SELECT state FROM jobs WHERE id=$1`, job.ID).Scan(&state); err != nil || state != "interrupted" {
		t.Fatalf("state=%q err=%v", state, err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT event_type FROM run_events WHERE run_id=$1 ORDER BY id DESC LIMIT 1`, id).Scan(&eventType); err != nil || eventType != "stage_interrupted" {
		t.Fatalf("event=%q err=%v", eventType, err)
	}
}

func TestFailedVerificationRetryIntegration(t *testing.T) {
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
	id := "verification-retry-" + time.Now().UTC().Format("20060102150405.000000000")
	run := domain.Run{ID: id, Prompt: "test", Profile: domain.ProfileFrontend, Stage: domain.StageVerifier, Status: domain.StatusAwaiting, Providers: map[domain.Stage]string{domain.StagePlanner: "test", domain.StageBuilder: "test", domain.StageVerifier: "test", domain.StageDeployer: "local-deployer"}, Graph: domain.DefaultGraph(), Architecture: domain.DefaultArchitecture(domain.ProfileFrontend, domain.DefaultGraph()), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteRun(context.Background(), id) })
	revision, err := st.CreateRevision(ctx, domain.Revision{RunID: id, Kind: domain.ReviewCode, Attempt: 1, BaselineDigest: "sha256:baseline", PatchDigest: "sha256:patch", Files: map[string]string{"generated-app/frontend/index.html": "after"}, Report: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecordRevisionReport(ctx, id, revision.ID, map[string]any{"status": "fail", "error": "verification infrastructure unavailable"}, "tested"); err != nil {
		t.Fatal(err)
	}
	updated, err := st.Approve(ctx, id, domain.ApprovalRetry, "")
	if err != nil || updated.Stage != domain.StageVerifier || updated.Status != domain.StatusQueued {
		t.Fatalf("run=%#v err=%v", updated, err)
	}
	var jobs, events int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE run_id=$1 AND stage='verifier' AND state='queued'`, id).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FROM run_events WHERE run_id=$1 AND event_type='verification_retry_requested'`, id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || events != 1 {
		t.Fatalf("jobs=%d events=%d", jobs, events)
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
