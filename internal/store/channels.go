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

func (s *Store) CreateChannelAccount(ctx context.Context, value domain.ChannelAccount) (domain.ChannelAccount, error) {
	if value.ID == "" || value.RunID == "" || !validChannelAdapter(value.Adapter) || value.Name == "" {
		return domain.ChannelAccount{}, fmt.Errorf("invalid channel account")
	}
	secrets, err := json.Marshal(value.SecretRefs)
	if err != nil {
		return domain.ChannelAccount{}, err
	}
	settings, err := json.Marshal(value.Settings)
	if err != nil {
		return domain.ChannelAccount{}, err
	}
	var rawSecrets, rawSettings []byte
	err = s.pool.QueryRow(ctx, `INSERT INTO channel_accounts(id,run_id,adapter,name,secret_refs,settings,enabled) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING secret_refs,settings,created_at,updated_at`, value.ID, value.RunID, value.Adapter, value.Name, secrets, settings, value.Enabled).Scan(&rawSecrets, &rawSettings, &value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return domain.ChannelAccount{}, err
	}
	if err := json.Unmarshal(rawSecrets, &value.SecretRefs); err != nil {
		return domain.ChannelAccount{}, err
	}
	if err := json.Unmarshal(rawSettings, &value.Settings); err != nil {
		return domain.ChannelAccount{}, err
	}
	return value, nil
}

func (s *Store) ChannelAccount(ctx context.Context, id string) (domain.ChannelAccount, error) {
	return scanChannelAccount(s.pool.QueryRow(ctx, channelAccountQuery+` WHERE id=$1`, id))
}
func (s *Store) ChannelAccounts(ctx context.Context) ([]domain.ChannelAccount, error) {
	rows, err := s.pool.Query(ctx, channelAccountQuery+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ChannelAccount{}
	for rows.Next() {
		value, err := scanChannelAccount(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}
func (s *Store) ChannelAccountsForRun(ctx context.Context, runID string) ([]domain.ChannelAccount, error) {
	rows, err := s.pool.Query(ctx, channelAccountQuery+` WHERE run_id=$1 ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ChannelAccount{}
	for rows.Next() {
		value, err := scanChannelAccount(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}

func (s *Store) PairChannelIdentity(ctx context.Context, value domain.ChannelPairing) (domain.ChannelPairing, error) {
	if value.AccountID == "" || value.ExternalID == "" {
		return domain.ChannelPairing{}, fmt.Errorf("account and external identity are required")
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO channel_pairings(account_id,external_id,expires_at) VALUES($1,$2,$3) ON CONFLICT(account_id,external_id) DO UPDATE SET paired_at=now(),expires_at=EXCLUDED.expires_at RETURNING paired_at`, value.AccountID, value.ExternalID, value.ExpiresAt).Scan(&value.PairedAt)
	return value, err
}
func (s *Store) UnpairChannelIdentity(ctx context.Context, accountID, externalID string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM channel_pairings WHERE account_id=$1 AND external_id=$2`, accountID, externalID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) IsPaired(ctx context.Context, accountID, externalID string) (bool, error) {
	var value bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM channel_pairings WHERE account_id=$1 AND external_id=$2 AND (expires_at IS NULL OR expires_at>now()))`, accountID, externalID).Scan(&value)
	return value, err
}

func (s *Store) Session(ctx context.Context, accountID, externalID, id string, expiresAt time.Time) (domain.ChannelSession, error) {
	var value domain.ChannelSession
	err := s.pool.QueryRow(ctx, `INSERT INTO channel_sessions(id,account_id,external_id,expires_at) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,external_id) DO UPDATE SET expires_at=EXCLUDED.expires_at,updated_at=now() RETURNING id,account_id,external_id,summary,expires_at,created_at,updated_at`, id, accountID, externalID, expiresAt).Scan(&value.ID, &value.AccountID, &value.ExternalID, &value.Summary, &value.ExpiresAt, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}
func (s *Store) ResetSession(ctx context.Context, accountID, externalID string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM channel_sessions WHERE account_id=$1 AND external_id=$2`, accountID, externalID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) ExpireSessions(ctx context.Context) (int, error) {
	result, err := s.pool.Exec(ctx, `DELETE FROM channel_sessions WHERE expires_at<=now()`)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}
func (s *Store) UpdateSessionSummary(ctx context.Context, id, summary string) error {
	_, err := s.pool.Exec(ctx, `UPDATE channel_sessions SET summary=$2,updated_at=now() WHERE id=$1`, id, summary)
	return err
}

func (s *Store) CreateChannelMessage(ctx context.Context, value domain.ChannelMessage) (domain.ChannelMessage, bool, error) {
	if value.AccountID == "" || value.ExternalID == "" || (value.Direction != "inbound" && value.Direction != "outbound") || value.IdempotencyKey == "" || value.State == "" {
		return domain.ChannelMessage{}, false, fmt.Errorf("invalid channel message")
	}
	attachments, err := json.Marshal(value.Attachments)
	if err != nil {
		return domain.ChannelMessage{}, false, err
	}
	var raw []byte
	err = s.pool.QueryRow(ctx, `INSERT INTO channel_messages(account_id,external_id,direction,platform_id,idempotency_key,text,attachments,state,error) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(account_id,idempotency_key) DO NOTHING RETURNING id,attachments,created_at,delivered_at`, value.AccountID, value.ExternalID, value.Direction, value.PlatformID, value.IdempotencyKey, value.Text, attachments, value.State, value.Error).Scan(&value.ID, &raw, &value.CreatedAt, &value.DeliveredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ChannelMessage{}, false, nil
	}
	if err != nil {
		return domain.ChannelMessage{}, false, err
	}
	if err := json.Unmarshal(raw, &value.Attachments); err != nil {
		return domain.ChannelMessage{}, false, err
	}
	return value, true, nil
}
func (s *Store) PendingOutboundMessages(ctx context.Context, limit int) ([]domain.ChannelMessage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT id,account_id,external_id,direction,platform_id,idempotency_key,text,attachments,state,error,created_at,delivered_at FROM channel_messages WHERE state='pending' AND direction='outbound' ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ChannelMessage{}
	for rows.Next() {
		value, err := scanChannelMessage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}
func (s *Store) CompleteChannelMessage(ctx context.Context, id int64, state, errText string) error {
	result, err := s.pool.Exec(ctx, `UPDATE channel_messages SET state=$2,error=$3,delivered_at=CASE WHEN $2='delivered' THEN now() ELSE NULL END WHERE id=$1 AND state='pending'`, id, state, errText)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) ChannelMessages(ctx context.Context, accountID, externalID string) ([]domain.ChannelMessage, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,account_id,external_id,direction,platform_id,idempotency_key,text,attachments,state,error,created_at,delivered_at FROM channel_messages WHERE account_id=$1 AND external_id=$2 ORDER BY id`, accountID, externalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ChannelMessage{}
	for rows.Next() {
		value, err := scanChannelMessage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}

const channelAccountQuery = `SELECT id,run_id,adapter,name,secret_refs,settings,enabled,created_at,updated_at FROM channel_accounts`

func scanChannelAccount(row interface{ Scan(...any) error }) (domain.ChannelAccount, error) {
	var value domain.ChannelAccount
	var secrets, settings []byte
	err := row.Scan(&value.ID, &value.RunID, &value.Adapter, &value.Name, &secrets, &settings, &value.Enabled, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ChannelAccount{}, ErrNotFound
	}
	if err != nil {
		return domain.ChannelAccount{}, err
	}
	if err := json.Unmarshal(secrets, &value.SecretRefs); err != nil {
		return domain.ChannelAccount{}, err
	}
	if err := json.Unmarshal(settings, &value.Settings); err != nil {
		return domain.ChannelAccount{}, err
	}
	return value, nil
}
func scanChannelMessage(row interface{ Scan(...any) error }) (domain.ChannelMessage, error) {
	var value domain.ChannelMessage
	var raw []byte
	err := row.Scan(&value.ID, &value.AccountID, &value.ExternalID, &value.Direction, &value.PlatformID, &value.IdempotencyKey, &value.Text, &raw, &value.State, &value.Error, &value.CreatedAt, &value.DeliveredAt)
	if err != nil {
		return domain.ChannelMessage{}, err
	}
	if err := json.Unmarshal(raw, &value.Attachments); err != nil {
		return domain.ChannelMessage{}, err
	}
	return value, nil
}
func validChannelAdapter(value string) bool {
	return value == "telegram" || value == "slack" || value == "discord" || value == "whatsapp"
}
