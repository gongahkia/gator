package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gongahkia/norbot/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

type ProviderObservation struct {
	ProviderID        string         `json:"provider_id"`
	RemainingRequests *int           `json:"remaining_requests,omitempty"`
	ResetAt           *time.Time     `json:"reset_at,omitempty"`
	Metadata          map[string]any `json:"metadata"`
	ObservedAt        time.Time      `json:"observed_at"`
}

type CapacityRecommendation struct {
	ID                 int64          `json:"id"`
	RecommendedWorkers int            `json:"recommended_workers"`
	AcceptedWorkers    *int           `json:"accepted_workers,omitempty"`
	Factors            map[string]any `json:"factors"`
	GeneratedAt        time.Time      `json:"generated_at"`
	AcceptedAt         *time.Time     `json:"accepted_at,omitempty"`
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  prompt TEXT NOT NULL,
  profile TEXT NOT NULL,
	deployment_target TEXT NOT NULL DEFAULT 'docker',
	public_ingress BOOLEAN NOT NULL DEFAULT FALSE,
	max_fixes INT NOT NULL DEFAULT 2,
  stage TEXT NOT NULL,
  status TEXT NOT NULL,
  providers JSONB NOT NULL,
  graph JSONB NOT NULL,
  feedback TEXT NOT NULL DEFAULT '',
  failure_reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS runs_updated_at_idx ON runs(updated_at DESC);
CREATE TABLE IF NOT EXISTS run_events (
  id BIGSERIAL PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  message TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS run_events_run_id_id_idx ON run_events(run_id, id);
CREATE TABLE IF NOT EXISTS jobs (
  id BIGSERIAL PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  stage TEXT NOT NULL,
  attempt INT NOT NULL DEFAULT 1,
  state TEXT NOT NULL DEFAULT 'queued',
  worker_id TEXT NOT NULL DEFAULT '',
  claimed_at TIMESTAMPTZ,
  lease_expires_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS jobs_claim_idx ON jobs(state, created_at) WHERE state = 'queued';
CREATE TABLE IF NOT EXISTS deployments (
  run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
  project_name TEXT NOT NULL,
  public_url TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS capacity_recommendations (
  id BIGSERIAL PRIMARY KEY,
  recommended_workers INT NOT NULL,
  accepted_workers INT,
  factors JSONB NOT NULL DEFAULT '{}'::jsonb,
  generated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS provider_observations (
  id BIGSERIAL PRIMARY KEY,
  provider_id TEXT NOT NULL,
  remaining_requests INT,
  reset_at TIMESTAMPTZ,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS revisions (
  id BIGSERIAL PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  attempt INT NOT NULL,
  baseline_digest TEXT NOT NULL,
  patch_digest TEXT NOT NULL,
  files JSONB NOT NULL DEFAULT '{}'::jsonb,
  report JSONB NOT NULL DEFAULT '{}'::jsonb,
  state TEXT NOT NULL DEFAULT 'proposed',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  approved_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS revisions_run_id_idx ON revisions(run_id,id DESC);
CREATE TABLE IF NOT EXISTS provider_usage (
  id BIGSERIAL PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  stage TEXT NOT NULL,
  revision_id BIGINT REFERENCES revisions(id) ON DELETE SET NULL,
  provider_id TEXT NOT NULL,
  model TEXT NOT NULL,
  input_tokens INT NOT NULL DEFAULT 0,
  output_tokens INT NOT NULL DEFAULT 0,
  cached_tokens INT NOT NULL DEFAULT 0,
  source TEXT NOT NULL,
  estimator TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS provider_usage_run_id_idx ON provider_usage(run_id,id DESC);
CREATE TABLE IF NOT EXISTS skill_packages (
  digest TEXT PRIMARY KEY,
  skill_id TEXT NOT NULL,
  version TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  manifest JSONB NOT NULL,
  path TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS skill_imports (
  id BIGSERIAL PRIMARY KEY,
  source_type TEXT NOT NULL,
  source_uri TEXT NOT NULL,
  source_ref TEXT NOT NULL DEFAULT '',
  credential_env TEXT NOT NULL DEFAULT '',
  digest TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL,
  findings JSONB NOT NULL DEFAULT '{}'::jsonb,
  activated_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS skill_imports_state_idx ON skill_imports(state,created_at DESC);
CREATE TABLE IF NOT EXISTS run_skills (
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  skill_digest TEXT NOT NULL REFERENCES skill_packages(digest) ON DELETE RESTRICT,
  selected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(run_id,skill_digest)
);
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS worker_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

ALTER TABLE runs ADD COLUMN IF NOT EXISTS deployment_target TEXT NOT NULL DEFAULT 'docker';
ALTER TABLE runs ADD COLUMN IF NOT EXISTS public_ingress BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE runs ADD COLUMN IF NOT EXISTS max_fixes INT NOT NULL DEFAULT 2;
UPDATE runs SET deployment_target='docker' WHERE deployment_target IS NULL OR deployment_target='';
CREATE INDEX IF NOT EXISTS jobs_lease_idx ON jobs(lease_expires_at) WHERE state = 'running';`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func (s *Store) CreateRun(ctx context.Context, run domain.Run) error {
	providers, err := json.Marshal(run.Providers)
	if err != nil {
		return err
	}
	graph, err := json.Marshal(run.Graph)
	if err != nil {
		return err
	}
	return s.withTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runs (id,prompt,profile,deployment_target,public_ingress,max_fixes,stage,status,providers,graph,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)`, run.ID, run.Prompt, run.Profile, run.DeploymentTarget, run.PublicIngress, run.MaxFixes, run.Stage, run.Status, providers, graph, run.CreatedAt)
		if err != nil {
			return err
		}
		return s.insertEvent(ctx, tx, run.ID, "run_created", "Run created and planner queued", map[string]any{"profile": run.Profile, "providers": run.Providers, "deployment_target": run.DeploymentTarget, "public_ingress": run.PublicIngress})
	})
}

func (s *Store) Enqueue(ctx context.Context, runID string, stage domain.Stage, attempt int) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO jobs (run_id,stage,attempt) VALUES ($1,$2,$3)`, runID, stage, attempt)
		if err != nil {
			return err
		}
		return s.insertEvent(ctx, tx, runID, "stage_queued", string(stage)+" queued", map[string]any{"stage": stage, "attempt": attempt})
	})
}

func (s *Store) GetRun(ctx context.Context, id string) (domain.Run, error) {
	row := s.pool.QueryRow(ctx, runQuery+` WHERE id=$1`, id)
	return scanRun(row)
}

func (s *Store) ListRuns(ctx context.Context) ([]domain.Run, error) {
	rows, err := s.pool.Query(ctx, runQuery+` ORDER BY updated_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []domain.Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) Events(ctx context.Context, runID string, afterID int64) ([]domain.Event, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,run_id,event_type,message,metadata,created_at FROM run_events WHERE run_id=$1 AND id>$2 ORDER BY id ASC LIMIT 500`, runID, afterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []domain.Event{}
	for rows.Next() {
		var event domain.Event
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.RunID, &event.Type, &event.Message, &metadata, &event.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) UpdateGraph(ctx context.Context, runID string, graph domain.Graph) (domain.Run, error) {
	encoded, err := json.Marshal(graph)
	if err != nil {
		return domain.Run{}, err
	}
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE runs SET graph=$2,updated_at=now() WHERE id=$1 AND stage='planner' AND status='awaiting_approval'`, runID, encoded)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return ErrNotFound
		}
		return s.insertEvent(ctx, tx, runID, "graph_updated", "Planner graph updated by operator", map[string]any{"graph": graph})
	})
	if err != nil {
		return domain.Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func (s *Store) SetPlannerGraph(ctx context.Context, runID string, graph domain.Graph) error {
	encoded, err := json.Marshal(graph)
	if err != nil {
		return err
	}
	return s.withTx(ctx, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE runs SET graph=$2,updated_at=now() WHERE id=$1 AND stage='planner' AND status='running'`, runID, encoded)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("planner graph update conflict")
		}
		return s.insertEvent(ctx, tx, runID, "planner_graph_generated", "Planner generated workflow graph", map[string]any{"graph": graph})
	})
}

func (s *Store) Approve(ctx context.Context, runID string, action domain.ApprovalAction, feedback string) (domain.Run, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		run, err := scanRun(tx.QueryRow(ctx, runQuery+` WHERE id=$1 FOR UPDATE`, runID))
		if err != nil {
			return err
		}
		switch action {
		case domain.ApprovalApprove:
			if run.Status != domain.StatusAwaiting {
				return fmt.Errorf("run is not awaiting approval")
			}
			if run.Stage == domain.StageVerifier {
				var report []byte
				err := tx.QueryRow(ctx, `SELECT report FROM revisions WHERE run_id=$1 ORDER BY id DESC LIMIT 1`, runID).Scan(&report)
				if err != nil {
					return fmt.Errorf("test report unavailable: %w", err)
				}
				var decoded map[string]any
				if err := json.Unmarshal(report, &decoded); err != nil {
					return err
				}
				if decoded["status"] == "fail" {
					return fmt.Errorf("failed test report requires explicit fix approval")
				}
			}
			if run.Stage == domain.StageDeployer {
				if _, err := tx.Exec(ctx, `UPDATE runs SET status='queued',feedback='',updated_at=now() WHERE id=$1`, runID); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `INSERT INTO jobs (run_id,stage,attempt) VALUES ($1,'deployer',1)`, runID); err != nil {
					return err
				}
				return s.insertEvent(ctx, tx, runID, "deployment_approved", "Deployment approved and queued", map[string]any{"stage": run.Stage})
			}
			next, hasNext := run.Stage.Next()
			if !hasNext {
				return fmt.Errorf("invalid stage")
			}
			if next == domain.StageDeployer {
				if _, err := tx.Exec(ctx, `UPDATE runs SET stage='deployer',status='awaiting_approval',feedback='',updated_at=now() WHERE id=$1`, runID); err != nil {
					return err
				}
				return s.insertEvent(ctx, tx, runID, "deployment_approval_required", "Verification approved; deployment requires explicit approval", map[string]any{"stage": next})
			}
			if _, err := tx.Exec(ctx, `UPDATE runs SET stage=$2,status='queued',feedback='',updated_at=now() WHERE id=$1`, runID, next); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO jobs (run_id,stage,attempt) VALUES ($1,$2,1)`, runID, next); err != nil {
				return err
			}
			return s.insertEvent(ctx, tx, runID, "stage_approved", string(run.Stage)+" approved; "+string(next)+" queued", map[string]any{"stage": run.Stage, "next_stage": next})
		case domain.ApprovalRevise:
			if run.Status != domain.StatusAwaiting || run.Stage != domain.StagePlanner {
				return fmt.Errorf("revision is available only while planner approval is pending")
			}
			if feedback == "" {
				return fmt.Errorf("revision feedback is required")
			}
			if _, err := tx.Exec(ctx, `UPDATE runs SET status='queued',feedback=$2,updated_at=now() WHERE id=$1`, runID, feedback); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO jobs (run_id,stage,attempt) SELECT $1,'planner',COALESCE(MAX(attempt),0)+1 FROM jobs WHERE run_id=$1`, runID); err != nil {
				return err
			}
			return s.insertEvent(ctx, tx, runID, "planner_revision_requested", "Planner revision queued", map[string]any{"feedback": feedback})
		case domain.ApprovalRetry:
			if run.Status != domain.StatusFailed && run.Status != domain.StatusInterrupted {
				return fmt.Errorf("retry is available only for failed or interrupted runs")
			}
			if _, err := tx.Exec(ctx, `UPDATE runs SET status='queued',failure_reason='',updated_at=now() WHERE id=$1`, runID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO jobs (run_id,stage,attempt) SELECT $1,$2,COALESCE(MAX(attempt),0)+1 FROM jobs WHERE run_id=$1`, runID, run.Stage); err != nil {
				return err
			}
			return s.insertEvent(ctx, tx, runID, "stage_retry_requested", string(run.Stage)+" retry queued", map[string]any{"stage": run.Stage})
		case domain.ApprovalAbandon:
			if run.Status == domain.StatusAbandoned || run.Status == domain.StatusCompleted {
				return fmt.Errorf("completed or abandoned run cannot be abandoned")
			}
			if _, err := tx.Exec(ctx, `UPDATE runs SET status='abandoned',updated_at=now() WHERE id=$1`, runID); err != nil {
				return err
			}
			return s.insertEvent(ctx, tx, runID, "run_abandoned", "Run abandoned by operator", nil)
		case domain.ApprovalFix:
			if run.Status != domain.StatusAwaiting || run.Stage != domain.StageVerifier {
				return fmt.Errorf("fix is available only while a failed test report is awaiting approval")
			}
			var report []byte
			if err := tx.QueryRow(ctx, `SELECT report FROM revisions WHERE run_id=$1 ORDER BY id DESC LIMIT 1`, runID).Scan(&report); err != nil {
				return fmt.Errorf("test report unavailable: %w", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(report, &decoded); err != nil {
				return err
			}
			if decoded["status"] != "fail" {
				return fmt.Errorf("fix requires a failed test report")
			}
			if feedback == "" {
				encoded, err := json.Marshal(decoded)
				if err != nil {
					return err
				}
				feedback = "failed verification diagnostics: " + string(encoded)
			}
			var attempts int
			if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM revisions WHERE run_id=$1 AND kind='fix'`, runID).Scan(&attempts); err != nil {
				return err
			}
			if attempts >= run.MaxFixes {
				return fmt.Errorf("maximum fix attempts (%d) reached", run.MaxFixes)
			}
			attempt := attempts + 2
			if _, err := tx.Exec(ctx, `UPDATE runs SET stage='builder',status='queued',feedback=$2,updated_at=now() WHERE id=$1`, runID, feedback); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO jobs (run_id,stage,attempt) VALUES ($1,'builder',$2)`, runID, attempt); err != nil {
				return err
			}
			return s.insertEvent(ctx, tx, runID, "fix_approved", "Operator approved bounded fix attempt", map[string]any{"attempt": attempt, "feedback": feedback})
		default:
			return fmt.Errorf("unknown approval action")
		}
	})
	if err != nil {
		return domain.Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func (s *Store) ClaimJob(ctx context.Context, workerID string, lease time.Duration) (domain.Job, bool, error) {
	if workerID == "" || lease <= 0 {
		return domain.Job{}, false, fmt.Errorf("worker id and positive lease are required")
	}
	var job domain.Job
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `WITH next AS (
  SELECT id FROM jobs WHERE state='queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1
) UPDATE jobs SET state='running',worker_id=$1,claimed_at=now(),lease_expires_at=now()+$2::interval WHERE id=(SELECT id FROM next)
RETURNING id,run_id,stage,attempt,worker_id,lease_expires_at`, workerID, lease.String())
		err := row.Scan(&job.ID, &job.RunID, &job.Stage, &job.Attempt, &job.WorkerID, &job.LeaseExpiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return domain.Job{}, false, err
	}
	if job.ID == 0 {
		return domain.Job{}, false, nil
	}
	return job, true, nil
}

func (s *Store) HeartbeatJob(ctx context.Context, job domain.Job, lease time.Duration) error {
	result, err := s.pool.Exec(ctx, `UPDATE jobs SET lease_expires_at=now()+$3::interval WHERE id=$1 AND state='running' AND worker_id=$2`, job.ID, job.WorkerID, lease.String())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("job %d lease is no longer owned by %q", job.ID, job.WorkerID)
	}
	return nil
}

func (s *Store) RecoverExpiredJobs(ctx context.Context) (int, error) {
	var recovered int
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,run_id,stage,attempt FROM jobs WHERE state='running' AND lease_expires_at < now() FOR UPDATE SKIP LOCKED`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var job domain.Job
			if err := rows.Scan(&job.ID, &job.RunID, &job.Stage, &job.Attempt); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE jobs SET state='interrupted',completed_at=now(),last_error='worker lease expired' WHERE id=$1`, job.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE runs SET status='interrupted',failure_reason='worker lease expired; operator retry required',updated_at=now() WHERE id=$1 AND stage=$2 AND status='running'`, job.RunID, job.Stage); err != nil {
				return err
			}
			if err := s.insertEvent(ctx, tx, job.RunID, "stage_interrupted", string(job.Stage)+" worker lease expired; operator retry required", map[string]any{"stage": job.Stage, "attempt": job.Attempt}); err != nil {
				return err
			}
			recovered++
		}
		return rows.Err()
	})
	return recovered, err
}

func (s *Store) MarkStageRunning(ctx context.Context, job domain.Job, provider string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		owned, err := tx.Exec(ctx, `UPDATE jobs SET lease_expires_at=lease_expires_at WHERE id=$1 AND state='running' AND worker_id=$2`, job.ID, job.WorkerID)
		if err != nil {
			return err
		}
		if owned.RowsAffected() != 1 {
			return fmt.Errorf("job %d is no longer owned by %q", job.ID, job.WorkerID)
		}
		result, err := tx.Exec(ctx, `UPDATE runs SET status='running',updated_at=now() WHERE id=$1 AND stage=$2 AND status='queued'`, job.RunID, job.Stage)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("run is no longer ready for %s", job.Stage)
		}
		return s.insertEvent(ctx, tx, job.RunID, "stage_started", string(job.Stage)+" started", map[string]any{"stage": job.Stage, "attempt": job.Attempt, "provider": provider})
	})
}

func (s *Store) MarkStageAwaitingApproval(ctx context.Context, job domain.Job, metadata map[string]any) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		jobResult, err := tx.Exec(ctx, `UPDATE jobs SET state='completed',completed_at=now(),lease_expires_at=NULL WHERE id=$1 AND state='running' AND worker_id=$2`, job.ID, job.WorkerID)
		if err != nil {
			return err
		}
		if jobResult.RowsAffected() != 1 {
			return fmt.Errorf("job %d is no longer owned by %q", job.ID, job.WorkerID)
		}
		runResult, err := tx.Exec(ctx, `UPDATE runs SET status='awaiting_approval',updated_at=now() WHERE id=$1 AND stage=$2 AND status='running'`, job.RunID, job.Stage)
		if err != nil {
			return err
		}
		if runResult.RowsAffected() != 1 {
			return fmt.Errorf("run is no longer running %s", job.Stage)
		}
		return s.insertEvent(ctx, tx, job.RunID, "stage_approval_required", string(job.Stage)+" completed; approval required", metadata)
	})
}

func (s *Store) CompleteRun(ctx context.Context, job domain.Job, metadata map[string]any) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		jobResult, err := tx.Exec(ctx, `UPDATE jobs SET state='completed',completed_at=now(),lease_expires_at=NULL WHERE id=$1 AND state='running' AND worker_id=$2`, job.ID, job.WorkerID)
		if err != nil {
			return err
		}
		if jobResult.RowsAffected() != 1 {
			return fmt.Errorf("job %d is no longer owned by %q", job.ID, job.WorkerID)
		}
		runResult, err := tx.Exec(ctx, `UPDATE runs SET status='completed',updated_at=now() WHERE id=$1 AND stage='deployer' AND status='running'`, job.RunID)
		if err != nil {
			return err
		}
		if runResult.RowsAffected() != 1 {
			return fmt.Errorf("deployer run is no longer running")
		}
		return s.insertEvent(ctx, tx, job.RunID, "run_completed", "Deployment completed", metadata)
	})
}

func (s *Store) FailJob(ctx context.Context, job domain.Job, failure string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		jobResult, err := tx.Exec(ctx, `UPDATE jobs SET state='failed',completed_at=now(),lease_expires_at=NULL,last_error=$2 WHERE id=$1 AND state='running' AND worker_id=$3`, job.ID, failure, job.WorkerID)
		if err != nil {
			return err
		}
		if jobResult.RowsAffected() != 1 {
			return fmt.Errorf("job %d is no longer owned by %q", job.ID, job.WorkerID)
		}
		runResult, err := tx.Exec(ctx, `UPDATE runs SET status='failed',failure_reason=$2,updated_at=now() WHERE id=$1 AND stage=$3 AND status='running'`, job.RunID, failure, job.Stage)
		if err != nil {
			return err
		}
		if runResult.RowsAffected() != 1 {
			return fmt.Errorf("run is no longer running %s", job.Stage)
		}
		return s.insertEvent(ctx, tx, job.RunID, "stage_failed", string(job.Stage)+" failed; operator decision required", map[string]any{"stage": job.Stage, "error": failure})
	})
}

func (s *Store) UpsertDeployment(ctx context.Context, runID, project, publicURL, status, errorMessage string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO deployments (run_id,project_name,public_url,status,error_message) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (run_id) DO UPDATE SET project_name=EXCLUDED.project_name,public_url=EXCLUDED.public_url,status=EXCLUDED.status,error_message=EXCLUDED.error_message,updated_at=now()`, runID, project, publicURL, status, errorMessage)
	return err
}

func (s *Store) GetDeployment(ctx context.Context, runID string) (domain.Deployment, error) {
	var deployment domain.Deployment
	err := s.pool.QueryRow(ctx, `SELECT run_id,project_name,public_url,status,error_message,updated_at FROM deployments WHERE run_id=$1`, runID).Scan(&deployment.RunID, &deployment.ProjectName, &deployment.PublicURL, &deployment.Status, &deployment.ErrorMessage, &deployment.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deployment{}, ErrNotFound
	}
	if err != nil {
		return domain.Deployment{}, err
	}
	return deployment, nil
}

func (s *Store) RecordProviderObservation(ctx context.Context, observation ProviderObservation) error {
	metadata, err := json.Marshal(observation.Metadata)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO provider_observations (provider_id,remaining_requests,reset_at,metadata) VALUES ($1,$2,$3,$4)`, observation.ProviderID, observation.RemainingRequests, observation.ResetAt, metadata)
	return err
}

func (s *Store) LatestProviderObservations(ctx context.Context) ([]ProviderObservation, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (provider_id) provider_id,remaining_requests,reset_at,metadata,observed_at FROM provider_observations ORDER BY provider_id,observed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []ProviderObservation{}
	for rows.Next() {
		var value ProviderObservation
		var metadata []byte
		if err := rows.Scan(&value.ProviderID, &value.RemainingRequests, &value.ResetAt, &metadata, &value.ObservedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadata, &value.Metadata); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) CreateCapacityRecommendation(ctx context.Context, recommended int, factors map[string]any) (CapacityRecommendation, error) {
	encoded, err := json.Marshal(factors)
	if err != nil {
		return CapacityRecommendation{}, err
	}
	var value CapacityRecommendation
	var raw []byte
	err = s.pool.QueryRow(ctx, `INSERT INTO capacity_recommendations (recommended_workers,factors) VALUES ($1,$2) RETURNING id,recommended_workers,accepted_workers,factors,generated_at,accepted_at`, recommended, encoded).Scan(&value.ID, &value.RecommendedWorkers, &value.AcceptedWorkers, &raw, &value.GeneratedAt, &value.AcceptedAt)
	if err != nil {
		return CapacityRecommendation{}, err
	}
	if err := json.Unmarshal(raw, &value.Factors); err != nil {
		return CapacityRecommendation{}, err
	}
	return value, nil
}

func (s *Store) AcceptCapacityRecommendation(ctx context.Context, id int64, workers int) (CapacityRecommendation, error) {
	if workers < 1 {
		return CapacityRecommendation{}, fmt.Errorf("accepted workers must be positive")
	}
	var value CapacityRecommendation
	var raw []byte
	err := s.pool.QueryRow(ctx, `UPDATE capacity_recommendations SET accepted_workers=$2,accepted_at=now() WHERE id=$1 RETURNING id,recommended_workers,accepted_workers,factors,generated_at,accepted_at`, id, workers).Scan(&value.ID, &value.RecommendedWorkers, &value.AcceptedWorkers, &raw, &value.GeneratedAt, &value.AcceptedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CapacityRecommendation{}, ErrNotFound
	}
	if err != nil {
		return CapacityRecommendation{}, err
	}
	if err := json.Unmarshal(raw, &value.Factors); err != nil {
		return CapacityRecommendation{}, err
	}
	return value, nil
}

func (s *Store) RecordEvent(ctx context.Context, runID, typ, message string, metadata map[string]any) error {
	return s.withTx(ctx, func(tx pgx.Tx) error { return s.insertEvent(ctx, tx, runID, typ, message, metadata) })
}

func (s *Store) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) insertEvent(ctx context.Context, tx pgx.Tx, runID, typ, message string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO run_events (run_id,event_type,message,metadata) VALUES ($1,$2,$3,$4)`, runID, typ, message, encoded)
	return err
}

const runQuery = `SELECT id,prompt,profile,deployment_target,public_ingress,max_fixes,stage,status,providers,graph,feedback,failure_reason,created_at,updated_at FROM runs`

type rowScanner interface{ Scan(...any) error }

func scanRun(row rowScanner) (domain.Run, error) {
	var run domain.Run
	var providers, graph []byte
	err := row.Scan(&run.ID, &run.Prompt, &run.Profile, &run.DeploymentTarget, &run.PublicIngress, &run.MaxFixes, &run.Stage, &run.Status, &providers, &graph, &run.Feedback, &run.FailureReason, &run.CreatedAt, &run.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Run{}, ErrNotFound
	}
	if err != nil {
		return domain.Run{}, err
	}
	if err := json.Unmarshal(providers, &run.Providers); err != nil {
		return domain.Run{}, err
	}
	if err := json.Unmarshal(graph, &run.Graph); err != nil {
		return domain.Run{}, err
	}
	return run, nil
}

func RetryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return time.Duration(attempt) * time.Second
}
