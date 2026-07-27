package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/observability"
	"github.com/jackc/pgx/v5"
)

type traceCursor struct {
	ID int64 `json:"id"`
}

func (s *Store) RecordTrace(ctx context.Context, event domain.TraceEvent, raw any, rawDays int) (domain.TraceEvent, error) {
	var recorded domain.TraceEvent
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		var err error
		recorded, err = s.insertTrace(ctx, tx, event, raw, rawDays)
		return err
	})
	return recorded, err
}

func (s *Store) insertTrace(ctx context.Context, tx pgx.Tx, event domain.TraceEvent, raw any, rawDays int) (domain.TraceEvent, error) {
	if event.RunID == "" || event.Type == "" || event.Summary == "" {
		return domain.TraceEvent{}, fmt.Errorf("trace run_id, type, and summary are required")
	}
	if event.Severity == "" {
		event.Severity = "info"
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	if event.EntityRefs == nil {
		event.EntityRefs = map[string]string{}
	}
	correlation := observability.From(ctx)
	if event.Stage == "" {
		event.Stage = correlation.Stage
	}
	if event.Attempt == 0 {
		event.Attempt = correlation.Attempt
	}
	if event.ProviderID == "" {
		event.ProviderID = correlation.ProviderID
	}
	if event.RevisionID == 0 {
		event.RevisionID = correlation.RevisionID
	}
	if event.ApprovalID == 0 {
		event.ApprovalID = correlation.ApprovalID
	}
	if event.TurnID == "" {
		event.TurnID = correlation.TurnID
	}
	if event.ActionID == "" {
		event.ActionID = correlation.ActionID
	}
	if event.SandboxID == "" {
		event.SandboxID = correlation.SandboxID
	}
	if event.Actor == "" {
		event.Actor = correlation.Actor
	}
	if event.TraceID == "" {
		event.TraceID = correlation.TraceID
	}
	if event.SpanID == "" {
		event.SpanID = correlation.SpanID
	}
	if event.Traceparent == "" {
		event.Traceparent = correlation.Traceparent
	}
	for key, value := range observability.EntityRefs(correlation) {
		if event.EntityRefs[key] == "" {
			event.EntityRefs[key] = value
		}
	}
	payload, err := json.Marshal(redactTraceValue(event.Payload))
	if err != nil {
		return domain.TraceEvent{}, err
	}
	refs, err := json.Marshal(event.EntityRefs)
	if err != nil {
		return domain.TraceEvent{}, err
	}
	search := event.Summary + " " + event.Type + " " + event.Stage + " " + event.ProviderID + " " + event.Actor + " " + strings.Join(mapStringValues(event.EntityRefs), " ")
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	err = tx.QueryRow(ctx, `INSERT INTO trace_events(run_id,event_type,severity,status,summary,stage,attempt,provider_id,revision_id,approval_id,turn_id,action_id,sandbox_id,actor,trace_id,span_id,traceparent,entity_refs,payload,search_vector,occurred_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,to_tsvector('simple',$20),$21)
RETURNING id,recorded_at`, event.RunID, event.Type, event.Severity, event.Status, event.Summary, event.Stage, event.Attempt, event.ProviderID, event.RevisionID, event.ApprovalID, event.TurnID, event.ActionID, event.SandboxID, event.Actor, event.TraceID, event.SpanID, event.Traceparent, refs, payload, search, event.OccurredAt).Scan(&event.ID, &event.RecordedAt)
	if err != nil {
		return domain.TraceEvent{}, err
	}
	if raw == nil || s.forensics == nil {
		return event, nil
	}
	if rawDays <= 0 {
		rawDays = 30
	}
	plaintext, err := json.Marshal(raw)
	if err != nil {
		return domain.TraceEvent{}, err
	}
	nonce, ciphertext, err := s.forensics.seal(plaintext, []byte(strconv.FormatInt(event.ID, 10)))
	if err != nil {
		return domain.TraceEvent{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO forensic_payloads(trace_event_id,key_version,nonce,ciphertext,expires_at) VALUES($1,$2,$3,$4,now()+$5::interval)`, event.ID, s.forensics.keyVersion, nonce, ciphertext, fmt.Sprintf("%d days", rawDays)); err != nil {
		return domain.TraceEvent{}, err
	}
	event.RawAvailable = true
	return event, nil
}

func (s *Store) TracePage(ctx context.Context, runID string, filter domain.TraceFilter, limit int) (domain.TracePage, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	where := []string{"run_id=$1"}
	args := []any{runID}
	if filter.Cursor != "" {
		id, err := decodeTraceCursor(filter.Cursor)
		if err != nil {
			return domain.TracePage{}, fmt.Errorf("invalid trace cursor")
		}
		args = append(args, id)
		where = append(where, "id>$"+strconv.Itoa(len(args)))
	}
	for _, item := range []struct{ value, column string }{{filter.Stage, "stage"}, {filter.Type, "event_type"}, {filter.Severity, "severity"}, {filter.ProviderID, "provider_id"}, {filter.Actor, "actor"}} {
		if item.value != "" {
			args = append(args, item.value)
			where = append(where, item.column+"=$"+strconv.Itoa(len(args)))
		}
	}
	if filter.Tool != "" {
		args = append(args, filter.Tool)
		where = append(where, "entity_refs->>'tool'=$"+strconv.Itoa(len(args)))
	}
	if filter.EntityID != "" {
		args = append(args, filter.EntityID)
		where = append(where, "($"+strconv.Itoa(len(args))+"=ANY(ARRAY[turn_id,action_id,sandbox_id]) OR EXISTS(SELECT 1 FROM jsonb_each_text(entity_refs) AS ref WHERE ref.value=$"+strconv.Itoa(len(args))+")")
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		where = append(where, "occurred_at>=$"+strconv.Itoa(len(args)))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		where = append(where, "occurred_at<=$"+strconv.Itoa(len(args)))
	}
	if strings.TrimSpace(filter.Query) != "" {
		args = append(args, filter.Query)
		where = append(where, "search_vector @@ plainto_tsquery('simple',$"+strconv.Itoa(len(args))+")")
	}
	args = append(args, limit+1)
	rows, err := s.pool.Query(ctx, traceSelect+" WHERE "+strings.Join(where, " AND ")+" ORDER BY id ASC LIMIT $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return domain.TracePage{}, err
	}
	defer rows.Close()
	page := domain.TracePage{}
	for rows.Next() {
		value, err := scanTrace(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, value)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		page.Items = page.Items[:limit]
		page.NextCursor = encodeTraceCursor(last.ID)
	}
	return page, nil
}

func (s *Store) TraceRaw(ctx context.Context, runID string, id int64) (any, error) {
	if s.forensics == nil {
		return nil, fmt.Errorf("local forensic mode is disabled")
	}
	var nonce, ciphertext []byte
	err := s.pool.QueryRow(ctx, `SELECT p.nonce,p.ciphertext FROM forensic_payloads p JOIN trace_events e ON e.id=p.trace_event_id WHERE p.trace_event_id=$1 AND e.run_id=$2 AND p.expires_at>now()`, id, runID).Scan(&nonce, &ciphertext)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	plaintext, err := s.forensics.open(nonce, ciphertext, []byte(strconv.FormatInt(id, 10)))
	if err != nil {
		return nil, fmt.Errorf("decrypt forensic payload: %w", err)
	}
	var value any
	if err := json.Unmarshal(plaintext, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (s *Store) PurgeTraceData(ctx context.Context, traceDays, forensicDays int) (int64, int64, error) {
	var traces, payloads int64
	var err error
	if forensicDays > 0 {
		payloads, err = s.deleteRetained(ctx, `DELETE FROM forensic_payloads WHERE created_at < now()-$1::interval`, forensicDays)
		if err != nil {
			return traces, payloads, err
		}
	}
	if traceDays > 0 {
		traces, err = s.deleteRetained(ctx, `DELETE FROM trace_events WHERE recorded_at < now()-$1::interval`, traceDays)
	}
	return traces, payloads, err
}

const traceSelect = `SELECT e.id,e.run_id,e.event_type,e.severity,e.status,e.summary,e.stage,e.attempt,e.provider_id,e.revision_id,e.approval_id,e.turn_id,e.action_id,e.sandbox_id,e.actor,e.trace_id,e.span_id,e.traceparent,e.entity_refs,e.payload,EXISTS(SELECT 1 FROM forensic_payloads p WHERE p.trace_event_id=e.id AND p.expires_at>now()),e.occurred_at,e.recorded_at FROM trace_events e`

func scanTrace(row interface{ Scan(...any) error }) (domain.TraceEvent, error) {
	var v domain.TraceEvent
	var refs, payload []byte
	err := row.Scan(&v.ID, &v.RunID, &v.Type, &v.Severity, &v.Status, &v.Summary, &v.Stage, &v.Attempt, &v.ProviderID, &v.RevisionID, &v.ApprovalID, &v.TurnID, &v.ActionID, &v.SandboxID, &v.Actor, &v.TraceID, &v.SpanID, &v.Traceparent, &refs, &payload, &v.RawAvailable, &v.OccurredAt, &v.RecordedAt)
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal(refs, &v.EntityRefs); err != nil {
		return v, err
	}
	if err = json.Unmarshal(payload, &v.Payload); err != nil {
		return v, err
	}
	return v, nil
}
func encodeTraceCursor(id int64) string {
	raw, _ := json.Marshal(traceCursor{ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func decodeTraceCursor(value string) (int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, err
	}
	var c traceCursor
	if err = json.Unmarshal(raw, &c); err != nil || c.ID < 1 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return c.ID, nil
}
func mapStringValues(value map[string]string) []string {
	result := make([]string, 0, len(value))
	for _, item := range value {
		result = append(result, item)
	}
	return result
}

func redactTraceValue(value any) any {
	switch item := value.(type) {
	case string:
		return redactSensitiveText(item)
	case map[string]any:
		result := map[string]any{}
		for key, value := range item {
			if isSensitiveKey(key) {
				result[key] = "[REDACTED]"
			} else {
				result[key] = redactTraceValue(value)
			}
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i, value := range item {
			result[i] = redactTraceValue(value)
		}
		return result
	default:
		return value
	}
}
func isSensitiveKey(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "token") || strings.Contains(value, "secret") || strings.Contains(value, "password") || strings.Contains(value, "api_key") || strings.Contains(value, "authorization")
}
func redactSensitiveText(value string) string {
	for _, marker := range []string{"bearer ", "api_key=", "api-key=", "password=", "secret="} {
		if index := strings.Index(strings.ToLower(value), marker); index >= 0 {
			end := index + len(marker)
			for end < len(value) && value[end] != ' ' && value[end] != '\n' && value[end] != '\t' {
				end++
			}
			value = value[:index] + value[index:index+len(marker)] + "[REDACTED]" + value[end:]
		}
	}
	return value
}
