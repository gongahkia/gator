package store

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gongahkia/norbot/internal/domain"
)

func (s *Store) AcceptanceBaseline(ctx context.Context, appID, contractDigest, screenshotID string) (domain.AcceptanceBaseline, error) {
	return scanAcceptanceBaseline(s.pool.QueryRow(ctx, `SELECT app_id,contract_digest,screenshot_id,run_id,state,digest,png,created_at,approved_at FROM acceptance_baselines WHERE app_id=$1 AND contract_digest=$2 AND screenshot_id=$3`, appID, contractDigest, screenshotID))
}

func (s *Store) CreateAcceptanceBaseline(ctx context.Context, value domain.AcceptanceBaseline) (domain.AcceptanceBaseline, error) {
	if value.AppID == "" || value.ContractDigest == "" || value.ScreenshotID == "" || value.RunID == "" || len(value.PNG) == 0 || len(value.PNG) > 5<<20 {
		return domain.AcceptanceBaseline{}, fmt.Errorf("invalid acceptance baseline")
	}
	if value.State == "" {
		value.State = "pending"
	}
	if value.State != "pending" {
		return domain.AcceptanceBaseline{}, fmt.Errorf("new acceptance baseline must be pending")
	}
	value.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(value.PNG))
	created, err := scanAcceptanceBaseline(s.pool.QueryRow(ctx, `INSERT INTO acceptance_baselines(app_id,contract_digest,screenshot_id,run_id,state,digest,png) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(app_id,contract_digest,screenshot_id) DO NOTHING RETURNING app_id,contract_digest,screenshot_id,run_id,state,digest,png,created_at,approved_at`, value.AppID, value.ContractDigest, value.ScreenshotID, value.RunID, value.State, value.Digest, value.PNG))
	if err == ErrNotFound {
		return s.AcceptanceBaseline(ctx, value.AppID, value.ContractDigest, value.ScreenshotID)
	}
	return created, err
}

func (s *Store) ApproveAcceptanceBaselines(ctx context.Context, runID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE acceptance_baselines SET state='approved',approved_at=now() WHERE run_id=$1 AND state='pending'`, runID)
	return err
}

func scanAcceptanceBaseline(row interface{ Scan(...any) error }) (domain.AcceptanceBaseline, error) {
	var value domain.AcceptanceBaseline
	err := row.Scan(&value.AppID, &value.ContractDigest, &value.ScreenshotID, &value.RunID, &value.State, &value.Digest, &value.PNG, &value.CreatedAt, &value.ApprovedAt)
	if err == pgx.ErrNoRows {
		return domain.AcceptanceBaseline{}, ErrNotFound
	}
	if err != nil {
		return domain.AcceptanceBaseline{}, err
	}
	return value, nil
}
