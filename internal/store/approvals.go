package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/observability"
)

func (s *Store) CreateApprovalOperation(ctx context.Context, value domain.ApprovalOperation) (domain.ApprovalOperation, error) {
	if value.RunID == "" || value.RevisionID == 0 || value.BaselineDigest == "" || value.PostDigest == "" || len(value.BaselineFiles) == 0 {
		return domain.ApprovalOperation{}, fmt.Errorf("invalid approval operation")
	}
	encoded, err := json.Marshal(value.BaselineFiles)
	if err != nil {
		return domain.ApprovalOperation{}, err
	}
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		run, err := scanRun(tx.QueryRow(ctx, runQuery+` WHERE id=$1 FOR UPDATE`, value.RunID))
		if err != nil {
			return err
		}
		if run.Status != domain.StatusAwaiting || run.Stage != domain.StageBuilder {
			return fmt.Errorf("code approval is unavailable")
		}
		var state, baselineDigest string
		if err := tx.QueryRow(ctx, `SELECT state,baseline_digest FROM revisions WHERE id=$1 AND run_id=$2 FOR UPDATE`, value.RevisionID, value.RunID).Scan(&state, &baselineDigest); err != nil {
			return err
		}
		if state != "proposed" || baselineDigest != value.BaselineDigest {
			return fmt.Errorf("code revision is no longer proposed")
		}
		var existingID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM approval_operations WHERE run_id=$1 AND revision_id=$2 AND state IN ('prepared','applying','workspace_applied') ORDER BY id DESC LIMIT 1`, value.RunID, value.RevisionID).Scan(&existingID); err == nil {
			existing, err := scanApprovalOperation(tx.QueryRow(ctx, approvalOperationQuery+` WHERE id=$1`, existingID), nil)
			if err != nil {
				return err
			}
			value = existing
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		correlation := observability.From(ctx)
		value.TraceID, value.SpanID, value.Traceparent = correlation.TraceID, correlation.SpanID, correlation.Traceparent
		if err := tx.QueryRow(ctx, `INSERT INTO approval_operations(run_id,revision_id,baseline_digest,post_digest,baseline_files,state,trace_id,span_id,traceparent) VALUES($1,$2,$3,$4,$5,'prepared',$6,$7,$8) RETURNING id,state,error,created_at,updated_at,completed_at`, value.RunID, value.RevisionID, value.BaselineDigest, value.PostDigest, encoded, value.TraceID, value.SpanID, value.Traceparent).Scan(&value.ID, &value.State, &value.Error, &value.CreatedAt, &value.UpdatedAt, &value.CompletedAt); err != nil {
			return err
		}
		if _, err := s.insertTrace(ctx, tx, domain.TraceEvent{RunID: value.RunID, Type: "code_approval_journaled", Summary: "Code approval journaled before workspace mutation", ApprovalID: value.ID, RevisionID: value.RevisionID, Status: value.State, Payload: map[string]any{"post_digest": value.PostDigest}}, map[string]any{"baseline_files": value.BaselineFiles}, 30); err != nil {
			return err
		}
		return s.insertEvent(ctx, tx, value.RunID, "code_approval_journaled", "Code approval journaled before workspace mutation", map[string]any{"approval_operation_id": value.ID, "revision_id": value.RevisionID, "post_digest": value.PostDigest})
	})
	return value, err
}

func (s *Store) ApprovalOperation(ctx context.Context, id int64) (domain.ApprovalOperation, error) {
	return scanApprovalOperation(s.pool.QueryRow(ctx, approvalOperationQuery+` WHERE id=$1`, id), nil)
}

func (s *Store) RecoverableApprovalOperations(ctx context.Context) ([]domain.ApprovalOperation, error) {
	rows, err := s.pool.Query(ctx, approvalOperationQuery+` WHERE state IN ('prepared','applying','workspace_applied') ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ApprovalOperation{}
	for rows.Next() {
		value, err := scanApprovalOperation(rows, nil)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}

func (s *Store) MarkApprovalOperationApplying(ctx context.Context, id int64) error {
	result, err := s.pool.Exec(ctx, `UPDATE approval_operations SET state='applying',updated_at=now(),error='' WHERE id=$1 AND state='prepared'`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("approval operation %d is not prepared", id)
	}
	return nil
}

func (s *Store) MarkApprovalWorkspaceApplied(ctx context.Context, id int64) error {
	result, err := s.pool.Exec(ctx, `UPDATE approval_operations SET state='workspace_applied',updated_at=now(),error='' WHERE id=$1 AND state IN ('prepared','applying')`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("approval operation %d cannot record workspace state", id)
	}
	return nil
}

func (s *Store) FailApprovalOperation(ctx context.Context, id int64, failure string) error {
	_, err := s.pool.Exec(ctx, `UPDATE approval_operations SET state='failed',error=$2,updated_at=now() WHERE id=$1 AND state IN ('prepared','applying','workspace_applied')`, id, failure)
	return err
}

func (s *Store) FinalizeApprovalOperation(ctx context.Context, id int64) (domain.Run, error) {
	var runID string
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var state string
		var revisionID int64
		if err := tx.QueryRow(ctx, `SELECT run_id,revision_id,state FROM approval_operations WHERE id=$1 FOR UPDATE`, id).Scan(&runID, &revisionID, &state); err != nil {
			return err
		}
		if state == "finalized" {
			return nil
		}
		if state != "workspace_applied" {
			return fmt.Errorf("approval operation %d workspace is not applied", id)
		}
		result, err := tx.Exec(ctx, `UPDATE revisions SET state='applied',approved_at=now() WHERE id=$1 AND run_id=$2 AND state='proposed'`, revisionID, runID)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("code revision is no longer proposed")
		}
		if err := s.approveTx(ctx, tx, runID, domain.ApprovalApprove, ""); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_operations SET state='finalized',updated_at=now(),completed_at=now(),error='' WHERE id=$1`, id); err != nil {
			return err
		}
		if _, err := s.insertTrace(ctx, tx, domain.TraceEvent{RunID: runID, Type: "code_approval_finalized", Summary: "Code approval finalized after workspace mutation", ApprovalID: id, RevisionID: revisionID, Status: "finalized"}, nil, 0); err != nil {
			return err
		}
		return s.insertEvent(ctx, tx, runID, "code_approval_finalized", "Code approval finalized after workspace mutation", map[string]any{"approval_operation_id": id, "revision_id": revisionID})
	})
	if err != nil {
		return domain.Run{}, err
	}
	return s.GetRun(ctx, runID)
}

const approvalOperationQuery = `SELECT id,run_id,revision_id,baseline_digest,post_digest,baseline_files,state,error,trace_id,span_id,traceparent,created_at,updated_at,completed_at FROM approval_operations`

func scanApprovalOperation(row interface{ Scan(...any) error }, target *domain.ApprovalOperation) (domain.ApprovalOperation, error) {
	value := domain.ApprovalOperation{}
	if target != nil {
		value = *target
	}
	var files []byte
	err := row.Scan(&value.ID, &value.RunID, &value.RevisionID, &value.BaselineDigest, &value.PostDigest, &files, &value.State, &value.Error, &value.TraceID, &value.SpanID, &value.Traceparent, &value.CreatedAt, &value.UpdatedAt, &value.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ApprovalOperation{}, ErrNotFound
	}
	if err != nil {
		return domain.ApprovalOperation{}, err
	}
	if err := json.Unmarshal(files, &value.BaselineFiles); err != nil {
		return domain.ApprovalOperation{}, err
	}
	return value, nil
}
