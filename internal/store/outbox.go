package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gongahkia/norbot/internal/domain"
)

func (s *Store) insertOutbox(ctx context.Context, tx pgx.Tx, runID, typ string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(run_id,event_type,payload) VALUES($1,$2,$3)`, nullRunID(runID), typ, encoded)
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
  SELECT id FROM outbox_events WHERE state='queued' ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1
) UPDATE outbox_events SET state='running',worker_id=$1,attempts=attempts+1,lease_expires_at=now()+$2::interval WHERE id=(SELECT id FROM next)
RETURNING id,COALESCE(run_id,''),event_type,payload,state,attempts,worker_id,last_error,created_at,delivered_at`, workerID, lease.String())
		return row.Scan(&item.ID, &item.RunID, &item.Type, &payload, &item.State, &item.Attempts, &item.WorkerID, &item.LastError, &item.CreatedAt, &item.DeliveredAt)
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
	result, err := s.pool.Exec(ctx, `UPDATE outbox_events SET state='queued',worker_id='',lease_expires_at=NULL,last_error=$3 WHERE id=$1 AND state='running' AND worker_id=$2`, item.ID, item.WorkerID, failure)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("outbox event %d is no longer owned by %q", item.ID, item.WorkerID)
	}
	return nil
}

func (s *Store) RecoverExpiredOutbox(ctx context.Context) (int, error) {
	result, err := s.pool.Exec(ctx, `UPDATE outbox_events SET state='queued',worker_id='',lease_expires_at=NULL,last_error=CASE WHEN last_error='' THEN 'relay lease expired' ELSE last_error END WHERE state='running' AND lease_expires_at < now()`)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
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
