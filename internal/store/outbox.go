package store

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gongahkia/norbot/internal/domain"
)

const outboxMaxAttempts = 12

func (s *Store) insertOutbox(ctx context.Context, tx pgx.Tx, runID, typ string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	key := ""
	if typ == "workspace.provision" && runID != "" {
		key = typ + ":" + runID
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(run_id,event_type,payload,idempotency_key) VALUES($1,$2,$3,$4) ON CONFLICT(event_type,idempotency_key) WHERE idempotency_key<>'' DO NOTHING`, nullRunID(runID), typ, encoded, key)
	return err
}

func nullRunID(runID string) any {
	if runID == "" {
		return nil
	}
	return runID
}

func (s *Store) ClaimOutbox(ctx context.Context, workerID string, lease time.Duration) (domain.OutboxEvent, bool, error) {
	if workerID == "" || lease <= 0 {
		return domain.OutboxEvent{}, false, fmt.Errorf("worker id and positive lease are required")
	}
	var item domain.OutboxEvent
	var payload []byte
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `WITH next AS (
  SELECT id FROM outbox_events WHERE state='queued' AND next_attempt_at<=now() ORDER BY next_attempt_at,created_at,id FOR UPDATE SKIP LOCKED LIMIT 1
) UPDATE outbox_events SET state='running',worker_id=$1,attempts=attempts+1,lease_expires_at=now()+$2::interval WHERE id=(SELECT id FROM next)
RETURNING id,COALESCE(run_id,''),event_type,payload,state,attempts,worker_id,last_error,idempotency_key,created_at,next_attempt_at,delivered_at,dead_lettered_at`, workerID, lease.String())
		return row.Scan(&item.ID, &item.RunID, &item.Type, &payload, &item.State, &item.Attempts, &item.WorkerID, &item.LastError, &item.IdempotencyKey, &item.CreatedAt, &item.NextAttemptAt, &item.DeliveredAt, &item.DeadLetteredAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OutboxEvent{}, false, nil
	}
	if err != nil {
		return domain.OutboxEvent{}, false, err
	}
	if err := json.Unmarshal(payload, &item.Payload); err != nil {
		return domain.OutboxEvent{}, false, err
	}
	return item, true, nil
}

func (s *Store) DeliverOutbox(ctx context.Context, item domain.OutboxEvent) error {
	result, err := s.pool.Exec(ctx, `UPDATE outbox_events SET state='delivered',lease_expires_at=NULL,delivered_at=now(),last_error='' WHERE id=$1 AND state='running' AND worker_id=$2`, item.ID, item.WorkerID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("outbox event %d is no longer owned by %q", item.ID, item.WorkerID)
	}
	return nil
}

func (s *Store) RetryOutbox(ctx context.Context, item domain.OutboxEvent, failure string) error {
	state := "queued"
	var next any = time.Now().UTC().Add(outboxRetryDelay(item.Attempts))
	if item.Attempts >= outboxMaxAttempts {
		state = "dead"
		next = nil
	}
	result, err := s.pool.Exec(ctx, `UPDATE outbox_events SET state=$4,worker_id='',lease_expires_at=NULL,last_error=$3,next_attempt_at=$5,dead_lettered_at=CASE WHEN $4='dead' THEN now() ELSE NULL END WHERE id=$1 AND state='running' AND worker_id=$2`, item.ID, item.WorkerID, failure, state, next)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("outbox event %d is no longer owned by %q", item.ID, item.WorkerID)
	}
	return nil
}

func (s *Store) RecoverExpiredOutbox(ctx context.Context) (int, error) {
	result, err := s.pool.Exec(ctx, `UPDATE outbox_events SET state=CASE WHEN attempts>=$1 THEN 'dead' ELSE 'queued' END,worker_id='',lease_expires_at=NULL,next_attempt_at=CASE WHEN attempts>=$1 THEN NULL ELSE now()+interval '5 seconds' END,dead_lettered_at=CASE WHEN attempts>=$1 THEN now() ELSE NULL END,last_error=CASE WHEN last_error='' THEN 'relay lease expired' ELSE last_error END WHERE state='running' AND lease_expires_at < now()`, outboxMaxAttempts)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}

func (s *Store) DeadOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT id,COALESCE(run_id,''),event_type,payload,state,attempts,worker_id,last_error,idempotency_key,created_at,next_attempt_at,delivered_at,dead_lettered_at FROM outbox_events WHERE state='dead' ORDER BY dead_lettered_at DESC,id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.OutboxEvent{}
	for rows.Next() {
		var item domain.OutboxEvent
		var payload []byte
		if err := rows.Scan(&item.ID, &item.RunID, &item.Type, &payload, &item.State, &item.Attempts, &item.WorkerID, &item.LastError, &item.IdempotencyKey, &item.CreatedAt, &item.NextAttemptAt, &item.DeliveredAt, &item.DeadLetteredAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &item.Payload); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ReplayDeadOutbox(ctx context.Context, id int64, operator string) (domain.OutboxEvent, error) {
	if id < 1 {
		return domain.OutboxEvent{}, fmt.Errorf("outbox id is required")
	}
	var item domain.OutboxEvent
	var payload []byte
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `UPDATE outbox_events SET state='queued',attempts=0,worker_id='',lease_expires_at=NULL,next_attempt_at=now(),dead_lettered_at=NULL,last_error='' WHERE id=$1 AND state='dead' RETURNING id,COALESCE(run_id,''),event_type,payload,state,attempts,worker_id,last_error,idempotency_key,created_at,next_attempt_at,delivered_at,dead_lettered_at`, id)
		if err := row.Scan(&item.ID, &item.RunID, &item.Type, &payload, &item.State, &item.Attempts, &item.WorkerID, &item.LastError, &item.IdempotencyKey, &item.CreatedAt, &item.NextAttemptAt, &item.DeliveredAt, &item.DeadLetteredAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		return s.insertAuditEvent(ctx, tx, operator, "outbox.replayed", "outbox_event", fmt.Sprint(id), map[string]any{"event_type": item.Type})
	})
	if err != nil {
		return domain.OutboxEvent{}, err
	}
	if err := json.Unmarshal(payload, &item.Payload); err != nil {
		return domain.OutboxEvent{}, err
	}
	return item, nil
}

func outboxRetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	seconds := math.Min(300, math.Pow(2, float64(attempts)))
	// avoid synchronized retry storms while preserving a bounded backoff window.
	jitter, err := cryptorand.Int(cryptorand.Reader, big.NewInt(1001))
	if err != nil {
		return time.Duration(seconds*1000) * time.Millisecond
	}
	return time.Duration(seconds*1000+float64(jitter.Int64())) * time.Millisecond
}

func (s *Store) CompleteWorkspaceProvision(ctx context.Context, runID string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE runs SET workspace_status='ready',updated_at=now() WHERE id=$1 AND workspace_status='provisioning'`, runID)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE jobs SET state='queued' WHERE run_id=$1 AND state='blocked'`, runID); err != nil {
			return err
		}
		return s.insertEvent(ctx, tx, runID, "workspace_provisioned", "Workspace provisioned; initial stage queued", nil)
	})
}

func (s *Store) FailWorkspaceProvision(ctx context.Context, runID, failure string) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE jobs SET state='failed',completed_at=now(),last_error=$2 WHERE run_id=$1 AND state='blocked'`, runID, failure); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE runs SET workspace_status='failed',status='failed',failure_reason=$2,updated_at=now() WHERE id=$1 AND workspace_status='provisioning'`, runID, failure); err != nil {
			return err
		}
		return s.insertEvent(ctx, tx, runID, "workspace_provision_failed", "Workspace provisioning failed", map[string]any{"error": failure})
	})
}
