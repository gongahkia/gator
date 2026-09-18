// Package googlework provides Gator's private, connected-only Google Work
// mirror. It deliberately owns no OAuth credentials and never queues writes:
// Google remains the source of truth and every user-visible operation must be
// preceded by a live Google request.
package googlework

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

var connectorIDPattern = regexp.MustCompile(`\A[a-z][a-z0-9-]{0,63}\z`)

// Store is a private per-connector SQLite mirror. Its data is an internal
// performance and conflict-comparison aid; callers must not treat it as an
// offline source of truth.
type Store struct {
	db   *sql.DB
	path string
}

// Record is a compact, normalized mirror entry. Raw holds the exact bounded
// response object received from Google and is never a credential.
type Record struct {
	Kind      string          `json:"kind"`
	ID        string          `json:"id"`
	ParentID  string          `json:"parent_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	UpdatedAt time.Time       `json:"updated_at,omitempty"`
	ETag      string          `json:"etag,omitempty"`
	Raw       json.RawMessage `json:"raw"`
}

// Open creates or opens a versioned private mirror below Gator's state root.
// It intentionally has no code path for importing Hot Cross Buns state.
func Open(stateDir, connectorID string) (*Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return nil, errors.New("Google Work state directory is required")
	}
	if !connectorIDPattern.MatchString(connectorID) {
		return nil, errors.New("Google Work connector ID is invalid")
	}
	base, err := filepath.Abs(stateDir)
	if err != nil {
		return nil, fmt.Errorf("resolve Google Work state directory: %w", err)
	}
	directory := filepath.Join(base, "gator", "google-work")
	if err := privateDirectory(directory); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, connectorID+".sqlite")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("Google Work mirror is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat Google Work mirror: %w", err)
	}
	database, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open Google Work mirror: %w", err)
	}
	store := &Store{db: database, path: path}
	if err := store.initialize(); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("secure Google Work mirror: %w", err)
	}
	return store, nil
}

// Path returns the private database path for diagnostics and tests only.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Close flushes and releases the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func privateDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create Google Work state directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("stat Google Work state directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Google Work state directory is unsafe")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("secure Google Work state directory: %w", err)
	}
	return nil
}

func (s *Store) initialize() error {
	if s == nil || s.db == nil {
		return errors.New("Google Work mirror is not open")
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read Google Work mirror version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("Google Work mirror version %d is newer than this Gator", version)
	}
	if version == 0 {
		statements := []string{
			`CREATE TABLE mirror_records (
                kind TEXT NOT NULL,
                id TEXT NOT NULL,
                parent_id TEXT NOT NULL DEFAULT '',
                name TEXT NOT NULL DEFAULT '',
                updated_at_ms INTEGER NOT NULL DEFAULT 0,
                etag TEXT NOT NULL DEFAULT '',
                raw_json BLOB NOT NULL,
                synced_at_ms INTEGER NOT NULL,
                PRIMARY KEY (kind, id)
            )`,
			`CREATE INDEX mirror_records_name ON mirror_records(kind, name)`,
			`CREATE TABLE sync_cursors (
                scope TEXT PRIMARY KEY,
                cursor TEXT NOT NULL,
                updated_at_ms INTEGER NOT NULL
            )`,
			`CREATE TABLE saved_searches (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                query TEXT NOT NULL,
                kinds_json BLOB NOT NULL,
                updated_at_ms INTEGER NOT NULL
            )`,
			`CREATE TABLE reminder_deliveries (
                delivery_key TEXT PRIMARY KEY,
                delivered_at_ms INTEGER NOT NULL
            )`,
			`PRAGMA user_version = 1`,
		}
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin Google Work mirror schema: %w", err)
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("create Google Work mirror schema: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit Google Work mirror schema: %w", err)
		}
	}
	return nil
}

// Mirror records data received from a successful live Google call. Unknown
// response shapes are retained as no records rather than guessed at.
func (s *Store) Mirror(ctx context.Context, operation string, data json.RawMessage, at time.Time) error {
	if s == nil || s.db == nil || !json.Valid(data) {
		return errors.New("Google Work mirror requires valid response JSON")
	}
	records, cursor, err := recordsForOperation(operation, data)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Google Work mirror update: %w", err)
	}
	for _, record := range records {
		if err := validateRecord(record); err != nil {
			_ = tx.Rollback()
			return err
		}
		updated := record.UpdatedAt.UnixMilli()
		if record.UpdatedAt.IsZero() {
			updated = 0
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO mirror_records(kind,id,parent_id,name,updated_at_ms,etag,raw_json,synced_at_ms)
            VALUES(?,?,?,?,?,?,?,?)
            ON CONFLICT(kind,id) DO UPDATE SET parent_id=excluded.parent_id,name=excluded.name,
              updated_at_ms=excluded.updated_at_ms,etag=excluded.etag,raw_json=excluded.raw_json,synced_at_ms=excluded.synced_at_ms`,
			record.Kind, record.ID, record.ParentID, record.Name, updated, record.ETag, []byte(record.Raw), at.UnixMilli()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upsert Google Work mirror record: %w", err)
		}
	}
	if cursor != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sync_cursors(scope,cursor,updated_at_ms) VALUES(?,?,?)
            ON CONFLICT(scope) DO UPDATE SET cursor=excluded.cursor,updated_at_ms=excluded.updated_at_ms`, operation, cursor, at.UnixMilli()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("save Google Work sync cursor: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Google Work mirror update: %w", err)
	}
	return nil
}

// Search is intentionally local-only plumbing. The connector runtime calls a
// live Google health request before exposing its result to a Work run.
func (s *Store) Search(ctx context.Context, query string, kinds []string, limit int) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("Google Work mirror is not open")
	}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 512 {
		return nil, errors.New("Google Work search query is invalid")
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("Google Work search limit must be 1 through 100")
	}
	args := []any{"%" + strings.ToLower(query) + "%"}
	where := "(lower(name) LIKE ? OR lower(CAST(raw_json AS TEXT)) LIKE ?)"
	args = append(args, "%"+strings.ToLower(query)+"%")
	if len(kinds) > 0 {
		unique := make(map[string]struct{}, len(kinds))
		placeholders := make([]string, 0, len(kinds))
		for _, kind := range kinds {
			kind = strings.TrimSpace(kind)
			if kind == "" || len(kind) > 64 {
				return nil, errors.New("Google Work search kind is invalid")
			}
			if _, exists := unique[kind]; exists {
				continue
			}
			unique[kind] = struct{}{}
			placeholders = append(placeholders, "?")
			args = append(args, kind)
		}
		if len(placeholders) > 0 {
			where += " AND kind IN (" + strings.Join(placeholders, ",") + ")"
		}
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT kind,id,parent_id,name,updated_at_ms,etag,raw_json
        FROM mirror_records WHERE `+where+` ORDER BY updated_at_ms DESC,name,id LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("search Google Work mirror: %w", err)
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		var record Record
		var updated int64
		var raw []byte
		if err := rows.Scan(&record.Kind, &record.ID, &record.ParentID, &record.Name, &updated, &record.ETag, &raw); err != nil {
			return nil, fmt.Errorf("read Google Work mirror result: %w", err)
		}
		if updated > 0 {
			record.UpdatedAt = time.UnixMilli(updated).UTC()
		}
		record.Raw = append(json.RawMessage(nil), raw...)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Google Work mirror result: %w", err)
	}
	return records, nil
}

// SaveSearch persists a named local query. It has no remote side effect.
func (s *Store) SaveSearch(ctx context.Context, id, name, query string, kinds []string, at time.Time) error {
	if s == nil || s.db == nil || !connectorIDPattern.MatchString(id) || strings.TrimSpace(name) == "" || len(name) > 256 || strings.TrimSpace(query) == "" || len(query) > 512 {
		return errors.New("Google Work saved search is invalid")
	}
	cleanKinds := append([]string(nil), kinds...)
	sort.Strings(cleanKinds)
	payload, err := json.Marshal(cleanKinds)
	if err != nil {
		return fmt.Errorf("encode Google Work saved search kinds: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO saved_searches(id,name,query,kinds_json,updated_at_ms) VALUES(?,?,?,?,?)
        ON CONFLICT(id) DO UPDATE SET name=excluded.name,query=excluded.query,kinds_json=excluded.kinds_json,updated_at_ms=excluded.updated_at_ms`, id, strings.TrimSpace(name), strings.TrimSpace(query), payload, at.UnixMilli())
	if err != nil {
		return fmt.Errorf("save Google Work search: %w", err)
	}
	return nil
}

func validateRecord(record Record) error {
	if strings.TrimSpace(record.Kind) == "" || len(record.Kind) > 64 || strings.TrimSpace(record.ID) == "" || len(record.ID) > 2048 || len(record.ParentID) > 2048 || len(record.Name) > 16*1024 || len(record.ETag) > 4096 || !json.Valid(record.Raw) {
		return errors.New("Google Work mirror record is invalid")
	}
	return nil
}

func recordsForOperation(operation string, data json.RawMessage) ([]Record, string, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, "", fmt.Errorf("decode Google response for mirror: %w", err)
	}
	kind, listKey, parentKey := "", "", ""
	switch operation {
	case "drive_search":
		kind, listKey = "drive_file", "files"
	case "drive_get":
		kind = "drive_file"
	case "tasklists_list":
		kind, listKey = "task_list", "items"
	case "tasks_list":
		kind, listKey, parentKey = "task", "items", "tasklist"
	case "tasks_get":
		kind, parentKey = "task", "tasklist"
	case "calendars_list":
		kind, listKey = "calendar", "items"
	case "events_list":
		kind, listKey, parentKey = "event", "items", "calendar"
	case "events_get":
		kind, parentKey = "event", "calendar"
	case "docs_get":
		kind = "document"
	case "sheets_get":
		kind = "spreadsheet"
	default:
		return nil, "", nil
	}
	parent := stringValue(envelope[parentKey])
	if listKey == "" {
		record, err := recordFromObject(kind, parent, data)
		if err != nil {
			return nil, "", err
		}
		return []Record{record}, stringValue(envelope["nextSyncToken"]), nil
	}
	var objects []json.RawMessage
	if raw := envelope[listKey]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &objects); err != nil {
			return nil, "", fmt.Errorf("decode Google %s items: %w", operation, err)
		}
	}
	records := make([]Record, 0, len(objects))
	for _, object := range objects {
		record, err := recordFromObject(kind, parent, object)
		if err != nil {
			return nil, "", err
		}
		records = append(records, record)
	}
	return records, stringValue(envelope["nextSyncToken"]), nil
}

func recordFromObject(kind, parent string, raw json.RawMessage) (Record, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return Record{}, err
	}
	id := stringValue(object["id"])
	if id == "" {
		id = stringValue(object["documentId"])
	}
	if id == "" {
		id = stringValue(object["spreadsheetId"])
	}
	if id == "" {
		return Record{}, errors.New("Google response object has no ID")
	}
	name := stringValue(object["name"])
	if name == "" {
		name = stringValue(object["title"])
	}
	updated := parseGoogleTime(stringValue(object["updated"]))
	if updated.IsZero() {
		updated = parseGoogleTime(stringValue(object["modifiedTime"]))
	}
	return Record{Kind: kind, ID: id, ParentID: parent, Name: name, UpdatedAt: updated, ETag: stringValue(object["etag"]), Raw: append(json.RawMessage(nil), raw...)}, nil
}

func stringValue(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return strings.TrimSpace(value)
}

func parseGoogleTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}
