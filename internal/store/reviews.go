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

func (s *Store) CreateRevision(ctx context.Context, value domain.Revision) (domain.Revision, error) {
	if !value.Kind.Valid() || value.RunID == "" || value.Attempt < 1 || value.BaselineDigest == "" || value.PatchDigest == "" {
		return domain.Revision{}, fmt.Errorf("invalid revision")
	}
	files, err := json.Marshal(value.Files)
	if err != nil {
		return domain.Revision{}, err
	}
	report, err := json.Marshal(value.Report)
	if err != nil {
		return domain.Revision{}, err
	}
	correlation := observability.From(ctx)
	if value.TraceID == "" {
		value.TraceID, value.SpanID, value.Traceparent = correlation.TraceID, correlation.SpanID, correlation.Traceparent
	}
	var rawFiles, rawReport []byte
	err = s.pool.QueryRow(ctx, `INSERT INTO revisions(run_id,kind,attempt,baseline_digest,patch_digest,files,report,state,trace_id,span_id,traceparent)
VALUES($1,$2,$3,$4,$5,$6,$7,'proposed',$8,$9,$10)
RETURNING id,created_at,files,report,state`, value.RunID, value.Kind, value.Attempt, value.BaselineDigest, value.PatchDigest, files, report, value.TraceID, value.SpanID, value.Traceparent).
		Scan(&value.ID, &value.CreatedAt, &rawFiles, &rawReport, &value.State)
	if err != nil {
		return domain.Revision{}, err
	}
	if err := json.Unmarshal(rawFiles, &value.Files); err != nil {
		return domain.Revision{}, err
	}
	if err := json.Unmarshal(rawReport, &value.Report); err != nil {
		return domain.Revision{}, err
	}
	if _, err := s.RecordTrace(ctx, domain.TraceEvent{RunID: value.RunID, Type: "revision_created", Summary: "Code or verification revision recorded", Stage: string(value.Kind), Attempt: value.Attempt, RevisionID: value.ID, Status: value.State, Payload: map[string]any{"kind": value.Kind, "baseline_digest": value.BaselineDigest, "patch_digest": value.PatchDigest}}, map[string]any{"files": value.Files, "report": value.Report}, 30); err != nil {
		return domain.Revision{}, err
	}
	return value, nil
}

func (s *Store) LatestRevision(ctx context.Context, runID string) (domain.Revision, error) {
	return scanRevision(s.pool.QueryRow(ctx, revisionQuery+` WHERE run_id=$1 ORDER BY id DESC LIMIT 1`, runID))
}

func (s *Store) Revision(ctx context.Context, runID string, id int64) (domain.Revision, error) {
	return scanRevision(s.pool.QueryRow(ctx, revisionQuery+` WHERE run_id=$1 AND id=$2`, runID, id))
}

func (s *Store) ListRevisions(ctx context.Context, runID string) ([]domain.Revision, error) {
	rows, err := s.pool.Query(ctx, revisionQuery+` WHERE run_id=$1 ORDER BY id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Revision{}
	for rows.Next() {
		value, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}

func (s *Store) ApproveRevision(ctx context.Context, runID string, id int64) error {
	result, err := s.pool.Exec(ctx, `UPDATE revisions SET state='applied',approved_at=now() WHERE id=$1 AND run_id=$2 AND state='proposed'`, id, runID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RecordRevisionReport(ctx context.Context, runID string, id int64, report map[string]any, state string) error {
	encoded, err := json.Marshal(report)
	if err != nil {
		return err
	}
	result, err := s.pool.Exec(ctx, `UPDATE revisions SET report=$3,state=$4 WHERE id=$1 AND run_id=$2`, id, runID, encoded, state)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RecordUsage(ctx context.Context, value domain.UsageRecord) (domain.UsageRecord, error) {
	if value.RunID == "" || !value.Stage.Valid() || value.ProviderID == "" || value.Model == "" || (value.Source != "reported" && value.Source != "estimated") || value.InputTokens < 0 || value.OutputTokens < 0 || value.CachedTokens < 0 {
		return domain.UsageRecord{}, fmt.Errorf("invalid provider usage")
	}
	metadata, err := json.Marshal(value.Metadata)
	if err != nil {
		return domain.UsageRecord{}, err
	}
	var raw []byte
	correlation := observability.From(ctx)
	if value.TraceID == "" {
		value.TraceID, value.SpanID, value.Traceparent = correlation.TraceID, correlation.SpanID, correlation.Traceparent
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO provider_usage(run_id,stage,revision_id,provider_id,model,input_tokens,output_tokens,cached_tokens,source,estimator,metadata,trace_id,span_id,traceparent)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
RETURNING id,created_at,metadata`, value.RunID, value.Stage, value.RevisionID, value.ProviderID, value.Model, value.InputTokens, value.OutputTokens, value.CachedTokens, value.Source, value.Estimator, metadata, value.TraceID, value.SpanID, value.Traceparent).Scan(&value.ID, &value.CreatedAt, &raw)
	if err != nil {
		return domain.UsageRecord{}, err
	}
	if err := json.Unmarshal(raw, &value.Metadata); err != nil {
		return domain.UsageRecord{}, err
	}
	return value, nil
}

func (s *Store) Usage(ctx context.Context, runID string) ([]domain.UsageRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,run_id,stage,revision_id,provider_id,model,input_tokens,output_tokens,cached_tokens,source,estimator,metadata,trace_id,span_id,traceparent,created_at FROM provider_usage WHERE run_id=$1 ORDER BY id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.UsageRecord{}
	for rows.Next() {
		var value domain.UsageRecord
		var raw []byte
		if err := rows.Scan(&value.ID, &value.RunID, &value.Stage, &value.RevisionID, &value.ProviderID, &value.Model, &value.InputTokens, &value.OutputTokens, &value.CachedTokens, &value.Source, &value.Estimator, &raw, &value.TraceID, &value.SpanID, &value.Traceparent, &value.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &value.Metadata); err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}

const revisionQuery = `SELECT id,run_id,kind,attempt,baseline_digest,patch_digest,files,report,state,trace_id,span_id,traceparent,created_at,approved_at FROM revisions`

func scanRevision(row interface{ Scan(...any) error }) (domain.Revision, error) {
	var value domain.Revision
	var files, report []byte
	err := row.Scan(&value.ID, &value.RunID, &value.Kind, &value.Attempt, &value.BaselineDigest, &value.PatchDigest, &files, &report, &value.State, &value.TraceID, &value.SpanID, &value.Traceparent, &value.CreatedAt, &value.ApprovedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Revision{}, ErrNotFound
	}
	if err != nil {
		return domain.Revision{}, err
	}
	if err := json.Unmarshal(files, &value.Files); err != nil {
		return domain.Revision{}, err
	}
	if err := json.Unmarshal(report, &value.Report); err != nil {
		return domain.Revision{}, err
	}
	return value, nil
}
