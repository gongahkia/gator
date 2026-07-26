package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"

	"github.com/gongahkia/norbot/internal/domain"
)

func (s *Store) AppendPlannerRevision(ctx context.Context, runID string, attempt int, source string, architecture domain.Architecture, expected domain.Status) (domain.PlannerRevision, error) {
	if attempt < 0 || source == "" {
		return domain.PlannerRevision{}, fmt.Errorf("invalid planner revision")
	}
	if err := architecture.Validate(); err != nil {
		return domain.PlannerRevision{}, err
	}
	var value domain.PlannerRevision
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		run, err := scanRun(tx.QueryRow(ctx, runQuery+` WHERE id=$1 FOR UPDATE`, runID))
		if err != nil {
			return err
		}
		if run.Stage != domain.StagePlanner || (expected != "" && run.Status != expected) {
			return fmt.Errorf("planner revision update conflict")
		}
		value.RunID, value.Attempt, value.Source = runID, attempt, source
		var previous domain.PlannerRevision
		var rawArchitecture, rawGraph, rawDiff []byte
		err = tx.QueryRow(ctx, `SELECT id,run_id,attempt,source,parent_id,architecture,graph,digest,diff,state,created_at,approved_at FROM planner_revisions WHERE run_id=$1 ORDER BY id DESC LIMIT 1`, runID).Scan(&previous.ID, &previous.RunID, &previous.Attempt, &previous.Source, &previous.ParentID, &rawArchitecture, &rawGraph, &previous.Digest, &rawDiff, &previous.State, &previous.CreatedAt, &previous.ApprovedAt)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil {
			if err := json.Unmarshal(rawArchitecture, &previous.Architecture); err != nil {
				return err
			}
			if err := json.Unmarshal(rawGraph, &previous.Graph); err != nil {
				return err
			}
			value.ParentID = &previous.ID
		}
		if attempt == 0 {
			attempt = previous.Attempt + 1
			if attempt < 1 {
				attempt = 1
			}
		}
		value.Attempt = attempt
		value.Architecture, value.Graph = architecture, architecture.Workflow
		value.Digest, err = plannerDigest(value.Architecture, value.Graph)
		if err != nil {
			return err
		}
		before := any(map[string]any{})
		if previous.ID != 0 {
			before = map[string]any{"architecture": previous.Architecture, "graph": previous.Graph}
		}
		after := any(map[string]any{"architecture": value.Architecture, "graph": value.Graph})
		value.Diff = jsonDiff(before, after, "")
		architectureJSON, err := json.Marshal(value.Architecture)
		if err != nil {
			return err
		}
		graphJSON, err := json.Marshal(value.Graph)
		if err != nil {
			return err
		}
		diffJSON, err := json.Marshal(value.Diff)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO planner_revisions(run_id,attempt,source,parent_id,architecture,graph,digest,diff) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,state,created_at,approved_at`, value.RunID, value.Attempt, value.Source, value.ParentID, architectureJSON, graphJSON, value.Digest, diffJSON).Scan(&value.ID, &value.State, &value.CreatedAt, &value.ApprovedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE runs SET architecture=$2,graph=$3,updated_at=now() WHERE id=$1`, runID, architectureJSON, graphJSON); err != nil {
			return err
		}
		return s.insertEvent(ctx, tx, runID, "planner_revision_created", "Planner revision recorded", map[string]any{"planner_revision_id": value.ID, "attempt": attempt, "source": source, "digest": value.Digest})
	})
	return value, err
}

func (s *Store) ListPlannerRevisions(ctx context.Context, runID string) ([]domain.PlannerRevision, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,run_id,attempt,source,parent_id,architecture,graph,digest,diff,state,created_at,approved_at FROM planner_revisions WHERE run_id=$1 ORDER BY id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.PlannerRevision{}
	for rows.Next() {
		item, err := scanPlannerRevision(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) approvePlannerRevisionTx(ctx context.Context, tx pgx.Tx, runID string) error {
	result, err := tx.Exec(ctx, `UPDATE planner_revisions SET state='approved',approved_at=now() WHERE id=(SELECT id FROM planner_revisions WHERE run_id=$1 ORDER BY id DESC LIMIT 1) AND state='proposed'`, runID)
	if err != nil {
		return err
	}
	_ = result
	return nil
}

func scanPlannerRevision(row interface{ Scan(...any) error }) (domain.PlannerRevision, error) {
	var value domain.PlannerRevision
	var architecture, graph, diff []byte
	if err := row.Scan(&value.ID, &value.RunID, &value.Attempt, &value.Source, &value.ParentID, &architecture, &graph, &value.Digest, &diff, &value.State, &value.CreatedAt, &value.ApprovedAt); err != nil {
		return domain.PlannerRevision{}, err
	}
	if err := json.Unmarshal(architecture, &value.Architecture); err != nil {
		return domain.PlannerRevision{}, err
	}
	if err := json.Unmarshal(graph, &value.Graph); err != nil {
		return domain.PlannerRevision{}, err
	}
	if err := json.Unmarshal(diff, &value.Diff); err != nil {
		return domain.PlannerRevision{}, err
	}
	return value, nil
}

func plannerDigest(architecture domain.Architecture, graph domain.Graph) (string, error) {
	encoded, err := json.Marshal(map[string]any{"architecture": architecture, "graph": graph})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func jsonDiff(before, after any, path string) []any {
	if reflect.DeepEqual(before, after) {
		return nil
	}
	beforeMap, beforeOK := before.(map[string]any)
	afterMap, afterOK := after.(map[string]any)
	if !beforeOK || !afterOK {
		return []any{map[string]any{"op": "replace", "path": diffPath(path), "value": after}}
	}
	keys := map[string]bool{}
	for key := range beforeMap {
		keys[key] = true
	}
	for key := range afterMap {
		keys[key] = true
	}
	encoded, _ := json.Marshal(mapKeysAny(keys))
	var ordered []string
	_ = json.Unmarshal(encoded, &ordered)
	result := []any{}
	for _, key := range ordered {
		next := path + "/" + escapeJSONPointer(key)
		beforeValue, existsBefore := beforeMap[key]
		afterValue, existsAfter := afterMap[key]
		switch {
		case !existsBefore:
			result = append(result, map[string]any{"op": "add", "path": next, "value": afterValue})
		case !existsAfter:
			result = append(result, map[string]any{"op": "remove", "path": next})
		default:
			result = append(result, jsonDiff(beforeValue, afterValue, next)...)
		}
	}
	return result
}

func mapKeysAny(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	for left := 0; left < len(keys); left++ {
		for right := left + 1; right < len(keys); right++ {
			if keys[right] < keys[left] {
				keys[left], keys[right] = keys[right], keys[left]
			}
		}
	}
	return keys
}

func diffPath(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func escapeJSONPointer(value string) string {
	value = stringReplaceAll(value, "~", "~0")
	return stringReplaceAll(value, "/", "~1")
}

func stringReplaceAll(value, old, replacement string) string {
	for {
		index := -1
		for i := 0; i+len(old) <= len(value); i++ {
			if value[i:i+len(old)] == old {
				index = i
				break
			}
		}
		if index < 0 {
			return value
		}
		value = value[:index] + replacement + value[index+len(old):]
	}
}
