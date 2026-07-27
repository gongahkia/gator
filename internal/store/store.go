package store

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) QueueDepth(ctx context.Context) (int, error) {
	var value int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE state IN ('queued','running')`).Scan(&value)
	return value, err
}

const migration001InitialSchema = `
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
	app_id TEXT NOT NULL DEFAULT '',
	parent_run_id TEXT NOT NULL DEFAULT '',
	base_snapshot_digest TEXT NOT NULL DEFAULT '',
  prompt TEXT NOT NULL,
  profile TEXT NOT NULL,
	deployment_target TEXT NOT NULL DEFAULT 'docker',
	public_ingress BOOLEAN NOT NULL DEFAULT FALSE,
	max_fixes INT NOT NULL DEFAULT 2,
  stage TEXT NOT NULL,
  status TEXT NOT NULL,
	workspace_status TEXT NOT NULL DEFAULT 'ready',
  providers JSONB NOT NULL,
  graph JSONB NOT NULL,
	architecture JSONB NOT NULL DEFAULT '{}'::jsonb,
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
	app_id TEXT NOT NULL DEFAULT '',
	is_current BOOLEAN NOT NULL DEFAULT FALSE,
  project_name TEXT NOT NULL,
  public_url TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS app_snapshots (
  digest TEXT PRIMARY KEY,
  source_run_id TEXT NOT NULL,
  app_id TEXT NOT NULL,
  path TEXT NOT NULL,
  file_count INT NOT NULL,
  byte_count BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
CREATE TABLE IF NOT EXISTS planner_revisions (
  id BIGSERIAL PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  attempt INT NOT NULL,
  source TEXT NOT NULL,
  parent_id BIGINT REFERENCES planner_revisions(id) ON DELETE SET NULL,
  architecture JSONB NOT NULL,
  graph JSONB NOT NULL,
  digest TEXT NOT NULL,
  diff JSONB NOT NULL DEFAULT '[]'::jsonb,
  state TEXT NOT NULL DEFAULT 'proposed',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  approved_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS planner_revisions_run_id_idx ON planner_revisions(run_id,id DESC);
CREATE TABLE IF NOT EXISTS approval_operations (
  id BIGSERIAL PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  revision_id BIGINT NOT NULL REFERENCES revisions(id) ON DELETE CASCADE,
  baseline_digest TEXT NOT NULL,
  post_digest TEXT NOT NULL,
  baseline_files JSONB NOT NULL,
  state TEXT NOT NULL DEFAULT 'prepared',
  error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS approval_operations_active_idx ON approval_operations(run_id,revision_id) WHERE state IN ('prepared','applying','workspace_applied');
CREATE INDEX IF NOT EXISTS approval_operations_recovery_idx ON approval_operations(state,updated_at) WHERE state IN ('prepared','applying','workspace_applied');
CREATE TABLE IF NOT EXISTS outbox_events (
  id BIGSERIAL PRIMARY KEY,
  run_id TEXT REFERENCES runs(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  state TEXT NOT NULL DEFAULT 'queued',
  attempts INT NOT NULL DEFAULT 0,
  worker_id TEXT NOT NULL DEFAULT '',
  lease_expires_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  delivered_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS outbox_events_claim_idx ON outbox_events(state,created_at) WHERE state='queued';
CREATE INDEX IF NOT EXISTS outbox_events_lease_idx ON outbox_events(lease_expires_at) WHERE state='running';
CREATE TABLE IF NOT EXISTS verification_caches (
  cache_key TEXT PRIMARY KEY,
  runtime TEXT NOT NULL,
  ecosystem TEXT NOT NULL,
  location TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'building',
  worker_id TEXT NOT NULL DEFAULT '',
  lease_expires_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
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
	bundle_path TEXT NOT NULL DEFAULT '',
	mode TEXT NOT NULL DEFAULT 'native',
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
CREATE TABLE IF NOT EXISTS channel_accounts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  adapter TEXT NOT NULL,
  name TEXT NOT NULL,
  secret_refs JSONB NOT NULL DEFAULT '{}'::jsonb,
  settings JSONB NOT NULL DEFAULT '{}'::jsonb,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(run_id,adapter,name)
);
CREATE TABLE IF NOT EXISTS channel_pairings (
  account_id TEXT NOT NULL REFERENCES channel_accounts(id) ON DELETE CASCADE,
  external_id TEXT NOT NULL,
  paired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ,
  PRIMARY KEY(account_id,external_id)
);
CREATE TABLE IF NOT EXISTS channel_sessions (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES channel_accounts(id) ON DELETE CASCADE,
  external_id TEXT NOT NULL,
	 reply_id TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(account_id,external_id)
);
CREATE TABLE IF NOT EXISTS channel_messages (
  id BIGSERIAL PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES channel_accounts(id) ON DELETE CASCADE,
  external_id TEXT NOT NULL,
  direction TEXT NOT NULL,
  platform_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  text TEXT NOT NULL DEFAULT '',
  attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
  state TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  delivered_at TIMESTAMPTZ,
  UNIQUE(account_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS channel_messages_pending_idx ON channel_messages(state,created_at) WHERE state='pending';
ALTER TABLE channel_messages ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0;
ALTER TABLE channel_messages ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ;
ALTER TABLE channel_sessions ADD COLUMN IF NOT EXISTS reply_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS channel_messages_retry_idx ON channel_messages(state,next_attempt_at,id) WHERE state='pending' AND direction='outbound';
CREATE TABLE IF NOT EXISTS managed_artifacts (
  id TEXT PRIMARY KEY,
  run_id TEXT REFERENCES runs(id) ON DELETE SET NULL,
  owner_type TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  object_key TEXT NOT NULL UNIQUE,
  filename TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
  size_bytes BIGINT NOT NULL,
  digest TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS managed_artifacts_expiry_idx ON managed_artifacts(expires_at);
CREATE TABLE IF NOT EXISTS agent_turns (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL,
  external_id TEXT NOT NULL,
  role TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  prompt TEXT NOT NULL,
  history JSONB NOT NULL DEFAULT '[]'::jsonb,
  state TEXT NOT NULL,
  final TEXT NOT NULL DEFAULT '',
  provider_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(run_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS agent_turns_session_idx ON agent_turns(session_id,created_at DESC);
CREATE TABLE IF NOT EXISTS agent_actions (
  id TEXT PRIMARY KEY,
  turn_id TEXT NOT NULL REFERENCES agent_turns(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  tool TEXT NOT NULL,
  role TEXT NOT NULL,
  params JSONB NOT NULL,
  digest TEXT NOT NULL,
  state TEXT NOT NULL,
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  error TEXT NOT NULL DEFAULT '',
  approved_by TEXT NOT NULL DEFAULT '',
  decided_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_actions_pending_idx ON agent_actions(state,created_at) WHERE state='pending';
CREATE TABLE IF NOT EXISTS sandbox_executions (
  id TEXT PRIMARY KEY,
  action_id TEXT REFERENCES agent_actions(id) ON DELETE SET NULL,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  target TEXT NOT NULL,
  tool TEXT NOT NULL,
  state TEXT NOT NULL,
  exit_code INT NOT NULL DEFAULT 0,
  output TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS worker_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

ALTER TABLE runs ADD COLUMN IF NOT EXISTS deployment_target TEXT NOT NULL DEFAULT 'docker';
ALTER TABLE runs ADD COLUMN IF NOT EXISTS public_ingress BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE runs ADD COLUMN IF NOT EXISTS max_fixes INT NOT NULL DEFAULT 2;
ALTER TABLE runs ADD COLUMN IF NOT EXISTS parent_run_id TEXT NOT NULL DEFAULT '';

ALTER TABLE runs ADD COLUMN IF NOT EXISTS app_id TEXT NOT NULL DEFAULT '';
ALTER TABLE runs ADD COLUMN IF NOT EXISTS base_snapshot_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS app_id TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS is_current BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE runs ADD COLUMN IF NOT EXISTS architecture JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE runs ADD COLUMN IF NOT EXISTS workspace_status TEXT NOT NULL DEFAULT 'ready';
ALTER TABLE skill_imports ADD COLUMN IF NOT EXISTS bundle_path TEXT NOT NULL DEFAULT '';
ALTER TABLE skill_imports ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT 'native';
UPDATE runs SET deployment_target='docker' WHERE deployment_target IS NULL OR deployment_target='';
UPDATE runs SET architecture=jsonb_build_object(
  'app_name','Generated app','app_type',profile,'stack','[]'::jsonb,'integrations','[]'::jsonb,
  'core_features',jsonb_build_array(jsonb_build_object('id','core-request','name','Requested application','description','Deliver the approved user request.','role','app_logic','selected',true)),
  'optional_features','[]'::jsonb,'workflow',graph
) WHERE architecture='{}'::jsonb;
WITH RECURSIVE app_roots AS (
  SELECT id, id AS app_id FROM runs WHERE parent_run_id=''
  UNION ALL
  SELECT child.id, app_roots.app_id FROM runs child JOIN app_roots ON child.parent_run_id=app_roots.id
)
UPDATE runs SET app_id=app_roots.app_id FROM app_roots WHERE runs.id=app_roots.id AND runs.app_id='';
UPDATE runs SET app_id=id WHERE app_id='';
UPDATE deployments SET app_id=runs.app_id FROM runs WHERE deployments.run_id=runs.id AND deployments.app_id='';
WITH apps_without_current AS (
  SELECT app_id FROM deployments GROUP BY app_id HAVING NOT bool_or(is_current)
), latest_deployments AS (
  SELECT DISTINCT ON (d.app_id) d.run_id
  FROM deployments d JOIN apps_without_current a ON a.app_id=d.app_id
  WHERE d.status IN ('running','stopped','deleted')
  ORDER BY d.app_id,d.updated_at DESC
)
UPDATE deployments SET is_current=TRUE FROM latest_deployments WHERE deployments.run_id=latest_deployments.run_id;
CREATE INDEX IF NOT EXISTS runs_app_updated_at_idx ON runs(app_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS deployments_app_updated_idx ON deployments(app_id, updated_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS deployments_current_app_idx ON deployments(app_id) WHERE is_current;
CREATE INDEX IF NOT EXISTS jobs_lease_idx ON jobs(lease_expires_at) WHERE state = 'running';`

const migration002OperationalHardening = `
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS dead_lettered_at TIMESTAMPTZ;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS outbox_events_due_idx ON outbox_events(state,next_attempt_at,id) WHERE state='queued';
CREATE UNIQUE INDEX IF NOT EXISTS outbox_events_idempotency_idx ON outbox_events(event_type,idempotency_key) WHERE idempotency_key<>'';
ALTER TABLE agent_actions ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE agent_actions ADD COLUMN IF NOT EXISTS approval_context JSONB NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS agent_actions_expiry_idx ON agent_actions(state,expires_at) WHERE state='pending';
ALTER TABLE skill_imports ADD COLUMN IF NOT EXISTS resolved_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE skill_imports ADD COLUMN IF NOT EXISTS tree_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE skill_imports ADD COLUMN IF NOT EXISTS trust_level TEXT NOT NULL DEFAULT 'untrusted';
ALTER TABLE skill_imports ADD COLUMN IF NOT EXISTS reviewed_by TEXT NOT NULL DEFAULT '';
ALTER TABLE skill_imports ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS skill_imports_provenance_idx ON skill_imports(source_uri,resolved_ref,tree_digest);
CREATE TABLE IF NOT EXISTS audit_events (
  id BIGSERIAL PRIMARY KEY,
  actor TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_events_target_idx ON audit_events(target_type,target_id,id DESC);
`

const migration003CacheLedgerCleanup = `
DROP TABLE IF EXISTS verification_caches;
`

const migration004LifecycleIndexes = `
CREATE INDEX IF NOT EXISTS agent_turns_retention_idx ON agent_turns(created_at) WHERE state<>'awaiting_approval';
CREATE INDEX IF NOT EXISTS channel_messages_retention_idx ON channel_messages(created_at);
CREATE INDEX IF NOT EXISTS run_events_retention_idx ON run_events(created_at);
CREATE INDEX IF NOT EXISTS provider_usage_retention_idx ON provider_usage(created_at);
`

const migration005OutboxReceipts = `
CREATE TABLE IF NOT EXISTS outbox_delivery_receipts (
  outbox_event_id BIGINT NOT NULL REFERENCES outbox_events(id) ON DELETE CASCADE,
  sink TEXT NOT NULL,
  receipt JSONB NOT NULL DEFAULT '{}'::jsonb,
  delivered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(outbox_event_id,sink)
);
`

const migration006PlanningSwarms = `
CREATE TABLE IF NOT EXISTS planning_swarm_executions (
  id BIGSERIAL PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  attempt INT NOT NULL,
  state TEXT NOT NULL DEFAULT 'running',
  provider_id TEXT NOT NULL,
  model TEXT NOT NULL,
  prompt_digest TEXT NOT NULL,
  config_digest TEXT NOT NULL,
  selected_task_id BIGINT,
  ranker_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE(run_id,attempt)
);
CREATE TABLE IF NOT EXISTS planning_swarm_tasks (
  id BIGSERIAL PRIMARY KEY,
  execution_id BIGINT NOT NULL REFERENCES planning_swarm_executions(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  ordinal INT NOT NULL,
  state TEXT NOT NULL DEFAULT 'queued',
  provider_id TEXT NOT NULL,
  model TEXT NOT NULL,
  input_digest TEXT NOT NULL,
  output_digest TEXT NOT NULL DEFAULT '',
  architecture JSONB NOT NULL DEFAULT '{}'::jsonb,
  rationale TEXT NOT NULL DEFAULT '',
  assumptions JSONB NOT NULL DEFAULT '[]'::jsonb,
  risks JSONB NOT NULL DEFAULT '[]'::jsonb,
  score INT NOT NULL DEFAULT 0,
  rank_reason TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(execution_id,role)
);
CREATE INDEX IF NOT EXISTS planning_swarm_run_attempt_idx ON planning_swarm_executions(run_id,attempt DESC);
CREATE INDEX IF NOT EXISTS planning_swarm_task_execution_idx ON planning_swarm_tasks(execution_id,ordinal);
ALTER TABLE planning_swarm_executions ADD CONSTRAINT planning_swarm_selected_task_fk FOREIGN KEY (selected_task_id) REFERENCES planning_swarm_tasks(id) ON DELETE SET NULL;
CREATE TABLE IF NOT EXISTS planning_swarm_provider_slots (
  provider_id TEXT NOT NULL,
  holder TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(provider_id,holder)
);
CREATE INDEX IF NOT EXISTS planning_swarm_provider_slots_expiry_idx ON planning_swarm_provider_slots(expires_at);
`

const migration007RunAgentPolicies = `
CREATE TABLE IF NOT EXISTS run_agent_policies (
  run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
  version INT NOT NULL,
  policy JSONB NOT NULL,
  digest TEXT NOT NULL,
  updated_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS run_agent_policy_events (
  id BIGSERIAL PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  version INT NOT NULL,
  policy JSONB NOT NULL,
  digest TEXT NOT NULL,
  actor TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(run_id,version)
);
CREATE INDEX IF NOT EXISTS run_agent_policy_events_run_idx ON run_agent_policy_events(run_id,version DESC);
`

type migration struct {
	Version int
	Name    string
	SQL     string
}

var migrations = []migration{
	{Version: 1, Name: "initial_schema", SQL: migration001InitialSchema},
	{Version: 2, Name: "operational_hardening", SQL: migration002OperationalHardening},
	{Version: 3, Name: "cache_ledger_cleanup", SQL: migration003CacheLedgerCleanup},
	{Version: 4, Name: "lifecycle_indexes", SQL: migration004LifecycleIndexes},
	{Version: 5, Name: "outbox_receipts", SQL: migration005OutboxReceipts},
	{Version: 6, Name: "planning_swarms", SQL: migration006PlanningSwarms},
	{Version: 7, Name: "run_agent_policies", SQL: migration007RunAgentPolicies},
}

func (s *Store) Migrate(ctx context.Context) error {
	connection, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtext('norbot-schema-migrations'))`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer connection.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('norbot-schema-migrations'))`)
	if _, err := connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INT PRIMARY KEY,name TEXT NOT NULL,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	for _, item := range migrations {
		checksum := migrationChecksum(item.SQL)
		var appliedChecksum string
		err := connection.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE version=$1`, item.Version).Scan(&appliedChecksum)
		if errors.Is(err, pgx.ErrNoRows) {
			tx, beginErr := connection.Begin(ctx)
			if beginErr != nil {
				return fmt.Errorf("begin migration %d: %w", item.Version, beginErr)
			}
			if _, execErr := tx.Exec(ctx, item.SQL); execErr != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("apply migration %d (%s): %w", item.Version, item.Name, execErr)
			}
			if _, execErr := tx.Exec(ctx, `INSERT INTO schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, item.Version, item.Name, checksum); execErr != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("record migration %d: %w", item.Version, execErr)
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return fmt.Errorf("commit migration %d: %w", item.Version, commitErr)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("read migration %d: %w", item.Version, err)
		}
		if appliedChecksum != checksum {
			return fmt.Errorf("migration %d (%s) checksum changed; migrations are immutable", item.Version, item.Name)
		}
	}
	return nil
}

func migrationChecksum(sql string) string {
	digest := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(digest[:])
}

func (s *Store) CreateRun(ctx context.Context, run domain.Run) error {
	return s.createRun(ctx, run, nil, "", false, nil)
}

func (s *Store) CreateRunWithInitialJob(ctx context.Context, run domain.Run, skillDigests []string, inheritSkillsFrom string) error {
	return s.createRun(ctx, run, skillDigests, inheritSkillsFrom, true, nil)
}

func (s *Store) CreateRunWithInitialJobAndPolicy(ctx context.Context, run domain.Run, skillDigests []string, inheritSkillsFrom string, policy domain.RunAgentPolicy) error {
	return s.createRun(ctx, run, skillDigests, inheritSkillsFrom, true, &policy)
}

func (s *Store) createRun(ctx context.Context, run domain.Run, skillDigests []string, inheritSkillsFrom string, enqueue bool, policy *domain.RunAgentPolicy) error {
	if run.AppID == "" {
		run.AppID = run.ID
	}
	providers, err := json.Marshal(run.Providers)
	if err != nil {
		return err
	}
	graph, err := json.Marshal(run.Graph)
	if err != nil {
		return err
	}
	architecture, err := json.Marshal(run.Architecture)
	if err != nil {
		return err
	}
	return s.withTx(ctx, func(tx pgx.Tx) error {
		workspaceStatus := run.WorkspaceStatus
		if workspaceStatus == "" {
			workspaceStatus = "ready"
			if enqueue {
				workspaceStatus = "provisioning"
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO runs (id,app_id,parent_run_id,base_snapshot_digest,prompt,profile,deployment_target,public_ingress,max_fixes,stage,status,workspace_status,providers,graph,architecture,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)`, run.ID, run.AppID, run.ParentRunID, run.BaseSnapshotDigest, run.Prompt, run.Profile, run.DeploymentTarget, run.PublicIngress, run.MaxFixes, run.Stage, run.Status, workspaceStatus, providers, graph, architecture, run.CreatedAt)
		if err != nil {
			return err
		}
		if policy != nil {
			policy.RunID = run.ID
			if policy.Version == 0 {
				policy.Version = 1
			}
			if err := insertRunAgentPolicy(ctx, tx, *policy, policy.UpdatedBy); err != nil {
				return err
			}
		}
		if inheritSkillsFrom != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO run_skills(run_id,skill_digest) SELECT $1,skill_digest FROM run_skills WHERE run_id=$2 ON CONFLICT DO NOTHING`, run.ID, inheritSkillsFrom); err != nil {
				return err
			}
		}
		for _, digest := range skillDigests {
			result, err := tx.Exec(ctx, `INSERT INTO run_skills(run_id,skill_digest) SELECT $1,$2 WHERE EXISTS(SELECT 1 FROM skill_imports WHERE digest=$2 AND state='active' AND trust_level='reviewed') ON CONFLICT DO NOTHING`, run.ID, digest)
			if err != nil {
				return err
			}
			if result.RowsAffected() != 1 {
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM run_skills WHERE run_id=$1 AND skill_digest=$2)`, run.ID, digest).Scan(&exists); err != nil || !exists {
					return fmt.Errorf("skill %q is not active", digest)
				}
			}
		}
		message := "Run created"
		if enqueue {
			message += " and " + string(run.Stage) + " queued"
		}
		if err := s.insertEvent(ctx, tx, run.ID, "run_created", message, map[string]any{"app_id": run.AppID, "profile": run.Profile, "providers": run.Providers, "deployment_target": run.DeploymentTarget, "public_ingress": run.PublicIngress, "parent_run_id": run.ParentRunID, "base_snapshot_digest": run.BaseSnapshotDigest}); err != nil {
			return err
		}
		if !enqueue {
			return nil
		}
		if _, err := tx.Exec(ctx, `INSERT INTO jobs (run_id,stage,attempt,state) VALUES ($1,$2,1,'blocked')`, run.ID, run.Stage); err != nil {
			return err
		}
		if err := s.insertEvent(ctx, tx, run.ID, "stage_queued", string(run.Stage)+" waiting for workspace", map[string]any{"stage": run.Stage, "attempt": 1, "workspace_status": workspaceStatus}); err != nil {
			return err
		}
		return s.insertOutbox(ctx, tx, run.ID, "workspace.provision", map[string]any{"run_id": run.ID})
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
	page, err := s.ListRunsPage(ctx, "", "", "", 100)
	return page.Items, err
}

func (s *Store) ListRunsPage(ctx context.Context, cursor, status, appID string, limit int) (domain.RunPage, error) {
	return s.ListRunsFilteredPage(ctx, cursor, RunFilter{Status: status, AppID: appID}, limit)
}

type RunFilter struct {
	Status, AppID, Search       string
	CreatedAfter, CreatedBefore *time.Time
}

func (s *Store) ListRunsFilteredPage(ctx context.Context, cursor string, filter RunFilter, limit int) (domain.RunPage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	where, args, err := runPageWhere(cursor, filter)
	if err != nil {
		return domain.RunPage{}, err
	}
	args = append(args, limit+1)
	rows, err := s.pool.Query(ctx, runQuery+where+` ORDER BY updated_at DESC,id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return domain.RunPage{}, err
	}
	defer rows.Close()
	runs := []domain.Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return domain.RunPage{}, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return domain.RunPage{}, err
	}
	page := domain.RunPage{Items: runs}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		page.Items = page.Items[:limit]
		page.NextCursor = encodePageCursor(last.UpdatedAt, last.ID)
	}
	return page, nil
}

func (s *Store) DeleteRun(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM runs WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Events(ctx context.Context, runID string, afterID int64) ([]domain.Event, error) {
	page, err := s.EventsPage(ctx, runID, strconv.FormatInt(afterID, 10), 500)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *Store) EventsPage(ctx context.Context, runID, cursor string, limit int) (domain.EventPage, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	afterID := int64(0)
	if cursor != "" {
		parsed, err := decodeEventPageCursor(cursor)
		if err != nil {
			return domain.EventPage{}, fmt.Errorf("invalid event cursor")
		}
		afterID = parsed
	}
	rows, err := s.pool.Query(ctx, `SELECT id,run_id,event_type,message,metadata,created_at FROM run_events WHERE run_id=$1 AND id>$2 ORDER BY id ASC LIMIT $3`, runID, afterID, limit+1)
	if err != nil {
		return domain.EventPage{}, err
	}
	defer rows.Close()
	events := []domain.Event{}
	for rows.Next() {
		var event domain.Event
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.RunID, &event.Type, &event.Message, &metadata, &event.CreatedAt); err != nil {
			return domain.EventPage{}, err
		}
		if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
			return domain.EventPage{}, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return domain.EventPage{}, err
	}
	page := domain.EventPage{Items: events}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		page.Items = page.Items[:limit]
		page.NextCursor = encodeEventPageCursor(last.ID)
	}
	return page, nil
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

func (s *Store) UpdateArchitecture(ctx context.Context, runID string, architecture domain.Architecture) (domain.Run, error) {
	_, err := s.AppendPlannerRevision(ctx, runID, 0, "operator", architecture, domain.StatusAwaiting)
	if err != nil {
		return domain.Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func (s *Store) SetPlannerArchitecture(ctx context.Context, runID string, attempt int, architecture domain.Architecture) error {
	_, err := s.AppendPlannerRevision(ctx, runID, attempt, "provider", architecture, domain.StatusRunning)
	return err
}

func (s *Store) SetPlannerGraph(ctx context.Context, runID string, graph domain.Graph) error {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	run.Architecture.Workflow = graph
	_, err = s.AppendPlannerRevision(ctx, runID, 0, "provider", run.Architecture, domain.StatusRunning)
	return err
}

func (s *Store) Approve(ctx context.Context, runID string, action domain.ApprovalAction, feedback string) (domain.Run, error) {
	err := s.withTx(ctx, func(tx pgx.Tx) error { return s.approveTx(ctx, tx, runID, action, feedback) })
	if err != nil {
		return domain.Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func (s *Store) approveTx(ctx context.Context, tx pgx.Tx, runID string, action domain.ApprovalAction, feedback string) error {
	run, err := scanRun(tx.QueryRow(ctx, runQuery+` WHERE id=$1 FOR UPDATE`, runID))
	if err != nil {
		return err
	}
	switch action {
	case domain.ApprovalApprove:
		if run.Status != domain.StatusAwaiting {
			return fmt.Errorf("run is not awaiting approval")
		}
		if run.Stage == domain.StagePlanner {
			if err := s.approvePlannerRevisionTx(ctx, tx, runID); err != nil {
				return err
			}
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
		if run.WorkspaceStatus == "failed" {
			if _, err := tx.Exec(ctx, `UPDATE runs SET status='queued',workspace_status='provisioning',failure_reason='',updated_at=now() WHERE id=$1`, runID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO jobs(run_id,stage,attempt,state) SELECT $1,$2,COALESCE(MAX(attempt),0)+1,'blocked' FROM jobs WHERE run_id=$1`, runID, run.Stage); err != nil {
				return err
			}
			if err := s.insertEvent(ctx, tx, runID, "workspace_provision_retry_requested", "Workspace provisioning retry queued", map[string]any{"stage": run.Stage}); err != nil {
				return err
			}
			return s.insertOutbox(ctx, tx, runID, "workspace.provision", map[string]any{"run_id": runID})
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

func (s *Store) UpsertDeployment(ctx context.Context, runID, appID, project, publicURL, status, errorMessage string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO deployments (run_id,app_id,project_name,public_url,status,error_message,is_current) VALUES ($1,$2,$3,$4,$5,$6,FALSE)
ON CONFLICT (run_id) DO UPDATE SET app_id=EXCLUDED.app_id,project_name=EXCLUDED.project_name,public_url=EXCLUDED.public_url,status=EXCLUDED.status,error_message=EXCLUDED.error_message,updated_at=now()`, runID, appID, project, publicURL, status, errorMessage)
	return err
}

func (s *Store) PromoteDeployment(ctx context.Context, runID, appID string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE deployments SET is_current=FALSE WHERE app_id=$1 AND is_current`, appID); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE deployments SET is_current=TRUE,updated_at=now() WHERE run_id=$1 AND app_id=$2`, runID, appID)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) GetDeployment(ctx context.Context, runID string) (domain.Deployment, error) {
	var deployment domain.Deployment
	err := s.pool.QueryRow(ctx, `SELECT d.run_id,d.app_id,d.project_name,d.public_url,d.status,d.error_message,d.updated_at
FROM deployments d JOIN runs requested ON requested.id=$1
WHERE d.app_id=requested.app_id AND d.is_current ORDER BY d.updated_at DESC LIMIT 1`, runID).Scan(&deployment.RunID, &deployment.AppID, &deployment.ProjectName, &deployment.PublicURL, &deployment.Status, &deployment.ErrorMessage, &deployment.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Deployment{}, ErrNotFound
	}
	if err != nil {
		return domain.Deployment{}, err
	}
	return deployment, nil
}

func (s *Store) ListApps(ctx context.Context) ([]domain.App, error) {
	page, err := s.ListAppsPage(ctx, "", 100)
	return page.Items, err
}

func (s *Store) ListAppsPage(ctx context.Context, cursor string, limit int) (domain.AppPage, error) {
	return s.ListAppsFilteredPage(ctx, cursor, AppFilter{}, limit)
}

type AppFilter struct {
	Status, Search              string
	UpdatedAfter, UpdatedBefore *time.Time
}

func (s *Store) ListAppsFilteredPage(ctx context.Context, cursor string, filter AppFilter, limit int) (domain.AppPage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var args []any
	clauses := []string{}
	if status := strings.TrimSpace(filter.Status); status != "" {
		clauses = append(clauses, "d.status=$"+strconv.Itoa(len(args)+1))
		args = append(args, status)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		clauses = append(clauses, "(d.app_id ILIKE $"+strconv.Itoa(len(args)+1)+" OR d.project_name ILIKE $"+strconv.Itoa(len(args)+1)+" OR r.prompt ILIKE $"+strconv.Itoa(len(args)+1)+")")
		args = append(args, "%"+search+"%")
	}
	if filter.UpdatedAfter != nil {
		clauses = append(clauses, "d.updated_at>=$"+strconv.Itoa(len(args)+1))
		args = append(args, *filter.UpdatedAfter)
	}
	if filter.UpdatedBefore != nil {
		clauses = append(clauses, "d.updated_at<$"+strconv.Itoa(len(args)+1))
		args = append(args, *filter.UpdatedBefore)
	}
	if cursor != "" {
		updatedAt, appID, err := decodePageCursor(cursor)
		if err != nil {
			return domain.AppPage{}, err
		}
		args = append(args, updatedAt, appID)
		clauses = append(clauses, "(d.updated_at,d.app_id)<($"+strconv.Itoa(len(args)-1)+",$"+strconv.Itoa(len(args))+")")
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit+1)
	rows, err := s.pool.Query(ctx, `WITH latest_deployments AS (
  SELECT run_id,app_id,project_name,public_url,status,error_message,updated_at
  FROM deployments WHERE is_current
)
SELECT d.app_id,r.id,r.parent_run_id,r.prompt,r.profile,r.status,r.created_at,d.run_id,d.app_id,d.project_name,d.public_url,d.status,d.error_message,d.updated_at
FROM latest_deployments d JOIN runs r ON d.run_id=r.id`+where+` ORDER BY d.updated_at DESC,d.app_id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return domain.AppPage{}, err
	}
	defer rows.Close()
	apps := []domain.App{}
	for rows.Next() {
		var app domain.App
		if err := rows.Scan(&app.AppID, &app.RunID, &app.ParentRunID, &app.Prompt, &app.Profile, &app.RunStatus, &app.CreatedAt, &app.Deployment.RunID, &app.Deployment.AppID, &app.Deployment.ProjectName, &app.Deployment.PublicURL, &app.Deployment.Status, &app.Deployment.ErrorMessage, &app.Deployment.UpdatedAt); err != nil {
			return domain.AppPage{}, err
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return domain.AppPage{}, err
	}
	page := domain.AppPage{Items: apps}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		page.Items = page.Items[:limit]
		page.NextCursor = encodePageCursor(last.Deployment.UpdatedAt, last.AppID)
	}
	return page, nil
}

func (s *Store) UpsertAppSnapshot(ctx context.Context, snapshot domain.AppSnapshot) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO app_snapshots (digest,source_run_id,app_id,path,file_count,byte_count,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (digest) DO UPDATE SET path=EXCLUDED.path,file_count=EXCLUDED.file_count,byte_count=EXCLUDED.byte_count`, snapshot.Digest, snapshot.SourceRunID, snapshot.AppID, snapshot.Path, snapshot.FileCount, snapshot.ByteCount, snapshot.CreatedAt)
	return err
}

func (s *Store) AppSnapshot(ctx context.Context, digest string) (domain.AppSnapshot, error) {
	var snapshot domain.AppSnapshot
	err := s.pool.QueryRow(ctx, `SELECT digest,source_run_id,app_id,path,file_count,byte_count,created_at FROM app_snapshots WHERE digest=$1`, digest).Scan(&snapshot.Digest, &snapshot.SourceRunID, &snapshot.AppID, &snapshot.Path, &snapshot.FileCount, &snapshot.ByteCount, &snapshot.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AppSnapshot{}, ErrNotFound
	}
	return snapshot, err
}

func (s *Store) DeleteAppSnapshot(ctx context.Context, digest string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM app_snapshots WHERE digest=$1`, digest)
	return err
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
	if _, err = tx.Exec(ctx, `INSERT INTO run_events (run_id,event_type,message,metadata) VALUES ($1,$2,$3,$4)`, runID, typ, message, encoded); err != nil {
		return err
	}
	return s.insertOutbox(ctx, tx, runID, "run.event", map[string]any{"event_type": typ, "message": message, "metadata": metadata})
}

const runQuery = `SELECT id,app_id,parent_run_id,base_snapshot_digest,prompt,profile,deployment_target,public_ingress,max_fixes,stage,status,workspace_status,providers,graph,architecture,feedback,failure_reason,created_at,updated_at FROM runs`

type pageCursor struct {
	UpdatedAt time.Time `json:"updated_at"`
	ID        string    `json:"id"`
}

type eventPageCursor struct {
	ID int64 `json:"id"`
}

func runPageWhere(cursor string, filter RunFilter) (string, []any, error) {
	clauses := []string{}
	args := []any{}
	if status := strings.TrimSpace(filter.Status); status != "" {
		clauses = append(clauses, "status=$"+strconv.Itoa(len(args)+1))
		args = append(args, status)
	}
	if appID := strings.TrimSpace(filter.AppID); appID != "" {
		clauses = append(clauses, "app_id=$"+strconv.Itoa(len(args)+1))
		args = append(args, appID)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		clauses = append(clauses, "(prompt ILIKE $"+strconv.Itoa(len(args)+1)+" OR app_id ILIKE $"+strconv.Itoa(len(args)+1)+" OR architecture->>'app_name' ILIKE $"+strconv.Itoa(len(args)+1)+")")
		args = append(args, "%"+search+"%")
	}
	if filter.CreatedAfter != nil {
		clauses = append(clauses, "created_at>=$"+strconv.Itoa(len(args)+1))
		args = append(args, filter.CreatedAfter.UTC())
	}
	if filter.CreatedBefore != nil {
		clauses = append(clauses, "created_at<$"+strconv.Itoa(len(args)+1))
		args = append(args, filter.CreatedBefore.UTC())
	}
	if cursor != "" {
		updatedAt, id, err := decodePageCursor(cursor)
		if err != nil {
			return "", nil, err
		}
		clauses = append(clauses, "(updated_at,id)<($"+strconv.Itoa(len(args)+1)+",$"+strconv.Itoa(len(args)+2)+")")
		args = append(args, updatedAt, id)
	}
	if len(clauses) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

func encodePageCursor(updatedAt time.Time, id string) string {
	encoded, _ := json.Marshal(pageCursor{UpdatedAt: updatedAt.UTC(), ID: id})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodePageCursor(value string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid page cursor")
	}
	var cursor pageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.ID == "" || cursor.UpdatedAt.IsZero() {
		return time.Time{}, "", fmt.Errorf("invalid page cursor")
	}
	return cursor.UpdatedAt, cursor.ID, nil
}

func encodeEventPageCursor(id int64) string {
	encoded, _ := json.Marshal(eventPageCursor{ID: id})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeEventPageCursor(value string) (int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, err
	}
	var cursor eventPageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.ID < 0 {
		return 0, fmt.Errorf("invalid event cursor")
	}
	return cursor.ID, nil
}

type rowScanner interface{ Scan(...any) error }

func scanRun(row rowScanner) (domain.Run, error) {
	var run domain.Run
	var providers, graph, architecture []byte
	err := row.Scan(&run.ID, &run.AppID, &run.ParentRunID, &run.BaseSnapshotDigest, &run.Prompt, &run.Profile, &run.DeploymentTarget, &run.PublicIngress, &run.MaxFixes, &run.Stage, &run.Status, &run.WorkspaceStatus, &providers, &graph, &architecture, &run.Feedback, &run.FailureReason, &run.CreatedAt, &run.UpdatedAt)
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
	if err := json.Unmarshal(architecture, &run.Architecture); err != nil {
		return domain.Run{}, err
	}
	if run.Architecture.AppName == "" {
		run.Architecture = domain.DefaultArchitecture(run.Profile, run.Graph)
	}
	if run.AppID == "" {
		run.AppID = run.ID
	}
	return run, nil
}

func RetryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return time.Duration(attempt) * time.Second
}
