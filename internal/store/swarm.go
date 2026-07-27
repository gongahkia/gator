package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gongahkia/norbot/internal/domain"
)

type PlanningSwarmSpec struct {
	RunID         string
	Attempt       int
	ProviderID    string
	Model         string
	PromptDigest  string
	ConfigDigest  string
	Roles         []string
	TaskInputHash map[string]string
}

func (s *Store) EnsurePlanningSwarm(ctx context.Context, spec PlanningSwarmSpec, staleAfter time.Duration) (domain.PlanningSwarmExecution, error) {
	if spec.RunID == "" || spec.Attempt < 1 || spec.ProviderID == "" || spec.Model == "" || spec.PromptDigest == "" || spec.ConfigDigest == "" || len(spec.Roles) < 2 {
		return domain.PlanningSwarmExecution{}, fmt.Errorf("invalid planning swarm")
	}
	var value domain.PlanningSwarmExecution
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO planning_swarm_executions(run_id,attempt,provider_id,model,prompt_digest,config_digest) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(run_id,attempt) DO NOTHING`, spec.RunID, spec.Attempt, spec.ProviderID, spec.Model, spec.PromptDigest, spec.ConfigDigest); err != nil {
			return err
		}
		if staleAfter > 0 {
			if _, err := tx.Exec(ctx, `UPDATE planning_swarm_tasks SET state='queued',error='recovered after interrupted task lease',updated_at=now() WHERE execution_id=(SELECT id FROM planning_swarm_executions WHERE run_id=$1 AND attempt=$2) AND state='running' AND started_at < now()-$3::interval`, spec.RunID, spec.Attempt, staleAfter.String()); err != nil {
				return err
			}
		}
		var executionID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM planning_swarm_executions WHERE run_id=$1 AND attempt=$2 FOR UPDATE`, spec.RunID, spec.Attempt).Scan(&executionID); err != nil {
			return err
		}
		for ordinal, role := range spec.Roles {
			if strings.TrimSpace(role) == "" || spec.TaskInputHash[role] == "" {
				return fmt.Errorf("invalid swarm role")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO planning_swarm_tasks(execution_id,role,ordinal,provider_id,model,input_digest) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(execution_id,role) DO NOTHING`, executionID, role, ordinal, spec.ProviderID, spec.Model, spec.TaskInputHash[role]); err != nil {
				return err
			}
		}
		var err error
		value, err = scanPlanningSwarmExecution(ctx, tx, spec.RunID, spec.Attempt, true)
		return err
	})
	return value, err
}

func (s *Store) PlanningSwarm(ctx context.Context, runID string) (domain.PlanningSwarmExecution, error) {
	return scanPlanningSwarmExecution(ctx, s.pool, runID, 0, false)
}

type swarmQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func scanPlanningSwarmExecution(ctx context.Context, q swarmQueryer, runID string, attempt int, lock bool) (domain.PlanningSwarmExecution, error) {
	where, args := "run_id=$1", []any{runID}
	if attempt > 0 {
		where += " AND attempt=$2"
		args = append(args, attempt)
	}
	suffix := " ORDER BY attempt DESC LIMIT 1"
	if lock {
		suffix += " FOR UPDATE"
	}
	var value domain.PlanningSwarmExecution
	err := q.QueryRow(ctx, `SELECT id,run_id,attempt,state,provider_id,model,prompt_digest,config_digest,selected_task_id,ranker_error,created_at,updated_at,completed_at FROM planning_swarm_executions WHERE `+where+suffix, args...).Scan(&value.ID, &value.RunID, &value.Attempt, &value.State, &value.ProviderID, &value.Model, &value.PromptDigest, &value.ConfigDigest, &value.SelectedTaskID, &value.RankerError, &value.CreatedAt, &value.UpdatedAt, &value.CompletedAt)
	if err == pgx.ErrNoRows {
		return domain.PlanningSwarmExecution{}, ErrNotFound
	}
	if err != nil {
		return domain.PlanningSwarmExecution{}, err
	}
	rows, err := q.Query(ctx, `SELECT id,execution_id,role,ordinal,state,provider_id,model,input_digest,output_digest,architecture,rationale,assumptions,risks,score,rank_reason,error,started_at,completed_at,created_at,updated_at FROM planning_swarm_tasks WHERE execution_id=$1 ORDER BY ordinal ASC`, value.ID)
	if err != nil {
		return domain.PlanningSwarmExecution{}, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanPlanningSwarmTask(rows)
		if err != nil {
			return domain.PlanningSwarmExecution{}, err
		}
		value.Tasks = append(value.Tasks, item)
	}
	if err := rows.Err(); err != nil {
		return domain.PlanningSwarmExecution{}, err
	}
	return value, nil
}

func scanPlanningSwarmTask(row interface{ Scan(...any) error }) (domain.PlanningSwarmTask, error) {
	var value domain.PlanningSwarmTask
	var architecture, assumptions, risks []byte
	if err := row.Scan(&value.ID, &value.ExecutionID, &value.Role, &value.Ordinal, &value.State, &value.ProviderID, &value.Model, &value.InputDigest, &value.OutputDigest, &architecture, &value.Rationale, &assumptions, &risks, &value.Score, &value.RankReason, &value.Error, &value.StartedAt, &value.CompletedAt, &value.CreatedAt, &value.UpdatedAt); err != nil {
		return domain.PlanningSwarmTask{}, err
	}
	if len(architecture) > 0 && string(architecture) != "{}" {
		if err := json.Unmarshal(architecture, &value.Architecture); err != nil {
			return domain.PlanningSwarmTask{}, err
		}
	}
	if err := json.Unmarshal(assumptions, &value.Assumptions); err != nil {
		return domain.PlanningSwarmTask{}, err
	}
	if err := json.Unmarshal(risks, &value.Risks); err != nil {
		return domain.PlanningSwarmTask{}, err
	}
	return value, nil
}

func (s *Store) StartPlanningSwarmTask(ctx context.Context, taskID int64) (bool, error) {
	result, err := s.pool.Exec(ctx, `UPDATE planning_swarm_tasks SET state='running',started_at=now(),updated_at=now(),error='' WHERE id=$1 AND state='queued'`, taskID)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

func (s *Store) CompletePlanningSwarmTask(ctx context.Context, taskID int64, architecture domain.Architecture, rationale string, assumptions, risks []string, outputDigest string) error {
	if err := architecture.Validate(); err != nil {
		return err
	}
	if outputDigest == "" {
		return fmt.Errorf("swarm output digest is required")
	}
	architectureJSON, err := json.Marshal(architecture)
	if err != nil {
		return err
	}
	assumptionJSON, err := json.Marshal(assumptions)
	if err != nil {
		return err
	}
	riskJSON, err := json.Marshal(risks)
	if err != nil {
		return err
	}
	result, err := s.pool.Exec(ctx, `UPDATE planning_swarm_tasks SET state='completed',architecture=$2,rationale=$3,assumptions=$4,risks=$5,output_digest=$6,completed_at=now(),updated_at=now() WHERE id=$1 AND state='running'`, taskID, architectureJSON, strings.TrimSpace(rationale), assumptionJSON, riskJSON, outputDigest)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("planning swarm task completion conflict")
	}
	return nil
}

func (s *Store) FailPlanningSwarmTask(ctx context.Context, taskID int64, failure string) error {
	if strings.TrimSpace(failure) == "" {
		failure = "candidate failed"
	}
	_, err := s.pool.Exec(ctx, `UPDATE planning_swarm_tasks SET state='failed',error=$2,completed_at=now(),updated_at=now() WHERE id=$1 AND state IN ('queued','running')`, taskID, strings.TrimSpace(failure))
	return err
}

func (s *Store) RankPlanningSwarmTasks(ctx context.Context, executionID int64, scores map[int64]PlanningSwarmScore, rankerError string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		for taskID, score := range scores {
			if score.Score < 0 || score.Score > 100 {
				return fmt.Errorf("invalid swarm score")
			}
			if _, err := tx.Exec(ctx, `UPDATE planning_swarm_tasks SET score=$2,rank_reason=$3,updated_at=now() WHERE id=$1 AND execution_id=$4 AND state='completed'`, taskID, score.Score, strings.TrimSpace(score.Reason), executionID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE planning_swarm_executions SET ranker_error=$2,updated_at=now() WHERE id=$1`, executionID, strings.TrimSpace(rankerError))
		return err
	})
}

type PlanningSwarmScore struct {
	Score  int
	Reason string
}

func (s *Store) FinalizePlanningSwarm(ctx context.Context, runID string, attempt int, selectedTaskID int64, source string) (domain.PlannerRevision, error) {
	var revision domain.PlannerRevision
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var executionID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM planning_swarm_executions WHERE run_id=$1 AND attempt=$2 AND state='running' FOR UPDATE`, runID, attempt).Scan(&executionID); err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("planning swarm finalization conflict")
			}
			return err
		}
		task, err := planningSwarmTaskForSelection(ctx, tx, executionID, selectedTaskID)
		if err != nil {
			return err
		}
		revision, err = s.appendPlannerRevisionTx(ctx, tx, runID, attempt, source, task.Architecture, domain.StatusRunning)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE planning_swarm_executions SET state='completed',selected_task_id=$2,completed_at=now(),updated_at=now() WHERE id=$1`, executionID, task.ID); err != nil {
			return err
		}
		if err := s.insertEvent(ctx, tx, runID, "planning_swarm_selected", "Planning swarm selected a candidate for review", map[string]any{"execution_id": executionID, "task_id": task.ID, "role": task.Role, "planner_revision_id": revision.ID}); err != nil {
			return err
		}
		return s.insertAuditEvent(ctx, tx, "system", "planning_swarm.auto_selected", "run", runID, map[string]any{"execution_id": executionID, "task_id": task.ID, "planner_revision_id": revision.ID})
	})
	return revision, err
}

func (s *Store) SelectPlanningSwarmCandidate(ctx context.Context, runID string, taskID int64, actor string) (domain.PlannerRevision, error) {
	var revision domain.PlannerRevision
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var executionID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM planning_swarm_executions WHERE run_id=$1 AND state='completed' ORDER BY attempt DESC LIMIT 1 FOR UPDATE`, runID).Scan(&executionID); err != nil {
			if err == pgx.ErrNoRows {
				return ErrNotFound
			}
			return err
		}
		task, err := planningSwarmTaskForSelection(ctx, tx, executionID, taskID)
		if err != nil {
			return err
		}
		revision, err = s.appendPlannerRevisionTx(ctx, tx, runID, 0, "swarm:operator:"+task.Role, task.Architecture, domain.StatusAwaiting)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE planning_swarm_executions SET selected_task_id=$2,updated_at=now() WHERE id=$1`, executionID, task.ID); err != nil {
			return err
		}
		if err := s.insertEvent(ctx, tx, runID, "planning_swarm_candidate_selected", "Operator selected a planning swarm candidate", map[string]any{"execution_id": executionID, "task_id": task.ID, "role": task.Role, "planner_revision_id": revision.ID}); err != nil {
			return err
		}
		return s.insertAuditEvent(ctx, tx, actor, "planning_swarm.candidate_selected", "run", runID, map[string]any{"execution_id": executionID, "task_id": task.ID, "planner_revision_id": revision.ID})
	})
	return revision, err
}

func planningSwarmTaskForSelection(ctx context.Context, tx pgx.Tx, executionID, taskID int64) (domain.PlanningSwarmTask, error) {
	if taskID == 0 {
		if err := tx.QueryRow(ctx, `SELECT id FROM planning_swarm_tasks WHERE execution_id=$1 AND state='completed' ORDER BY score DESC,ordinal ASC LIMIT 1`, executionID).Scan(&taskID); err != nil {
			return domain.PlanningSwarmTask{}, fmt.Errorf("planning swarm has no valid candidate")
		}
	}
	row := tx.QueryRow(ctx, `SELECT id,execution_id,role,ordinal,state,provider_id,model,input_digest,output_digest,architecture,rationale,assumptions,risks,score,rank_reason,error,started_at,completed_at,created_at,updated_at FROM planning_swarm_tasks WHERE id=$1 AND execution_id=$2 AND state='completed' FOR UPDATE`, taskID, executionID)
	task, err := scanPlanningSwarmTask(row)
	if err == pgx.ErrNoRows {
		return domain.PlanningSwarmTask{}, fmt.Errorf("planning swarm candidate is unavailable")
	}
	return task, err
}

func (s *Store) DegradePlanningSwarm(ctx context.Context, runID string, attempt int, reason string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE planning_swarm_executions SET state='degraded',ranker_error=$3,completed_at=now(),updated_at=now() WHERE run_id=$1 AND attempt=$2 AND state='running'`, runID, attempt, strings.TrimSpace(reason))
		if err != nil {
			return err
		}
		return s.insertEvent(ctx, tx, runID, "planning_swarm_degraded", "Planning swarm fell back to the single planner", map[string]any{"reason": strings.TrimSpace(reason)})
	})
}

func (s *Store) AcquirePlanningSwarmProviderSlot(ctx context.Context, providerID, holder string, limit int, lease time.Duration) (bool, error) {
	if providerID == "" || holder == "" || limit < 1 || lease < time.Second {
		return false, fmt.Errorf("invalid planning swarm provider slot")
	}
	acquired := false
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, providerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM planning_swarm_provider_slots WHERE expires_at <= now()`); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM planning_swarm_provider_slots WHERE provider_id=$1`, providerID).Scan(&count); err != nil {
			return err
		}
		if count >= limit {
			return nil
		}
		result, err := tx.Exec(ctx, `INSERT INTO planning_swarm_provider_slots(provider_id,holder,expires_at) VALUES($1,$2,now()+$3::interval) ON CONFLICT(provider_id,holder) DO UPDATE SET expires_at=EXCLUDED.expires_at`, providerID, holder, lease.String())
		if err != nil {
			return err
		}
		acquired = result.RowsAffected() == 1
		return nil
	})
	return acquired, err
}

func (s *Store) ReleasePlanningSwarmProviderSlot(ctx context.Context, providerID, holder string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM planning_swarm_provider_slots WHERE provider_id=$1 AND holder=$2`, providerID, holder)
	return err
}
