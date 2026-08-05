package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) insertAuditEvent(ctx context.Context, tx pgx.Tx, actor, action, targetType, targetID string, metadata map[string]any) error {
	if action == "" || targetType == "" || targetID == "" {
		return fmt.Errorf("audit action and target are required")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(actor,action,target_type,target_id,metadata) VALUES($1,$2,$3,$4,$5)`, actor, action, targetType, targetID, encoded)
	return err
}

func (s *Store) RecordAuditEvent(ctx context.Context, actor, action, targetType, targetID string, metadata map[string]any) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return s.insertAuditEvent(ctx, tx, actor, action, targetType, targetID, metadata)
	})
}
