package worksession

// SQLite is a disposable, profile-local read index. Conversation/revision and
// admission JSON remain authoritative. Every query reconciles file identities;
// unchanged files are statted rather than decoded again. A missing or corrupt
// index can be rebuilt without modifying a retained chat or starting work.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workspace"
	"modernc.org/sqlite"
)

const catalogIndexName = "conversations-v1.sqlite"
const catalogTextBytes = 2 * 1024 * 1024

// PageConversations uses a parameterized SQLite index, falling back to bounded
// canonical reads when a cache cannot be used. Cache failure cannot reject an
// admitted message or make its retained conversation unreadable.
func (s Store) PageConversations(query ConversationQuery) (ConversationPage, error) {
	return s.PageConversationsContext(context.Background(), query)
}

func (s Store) PageConversationsContext(parent context.Context, query ConversationQuery) (ConversationPage, error) {
	q, err := normalizeConversationQuery(query)
	if err != nil {
		return ConversationPage{}, err
	}
	release, err := state.Guard(filepath.Dir(filepath.Dir(s.root)), false)
	if err != nil {
		return ConversationPage{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	page, err := s.pageConversationIndex(ctx, q)
	if err == nil {
		return page, nil
	}
	if parent.Err() != nil {
		return ConversationPage{}, parent.Err()
	}
	page, fallbackErr := s.pageConversationFiles(q)
	page.IndexFallback = true
	return page, fallbackErr
}

type catalogFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Time int64  `json:"time"`
	Mode uint32 `json:"mode"`
}

func catalogStat(root *os.Root, path string) (catalogFile, error) {
	// Paths come from validated chat/revision identities, including dependency
	// records read back from SQLite. A tampered cache cannot grant file access.
	parts := strings.Split(filepath.ToSlash(path), "/")
	valid := len(parts) == 2 && catalogIdentity.MatchString(parts[0]) && (parts[1] == "conversation.json" || parts[1] == "display.json" || parts[1] == "requests.json")
	if len(parts) == 3 && catalogIdentity.MatchString(parts[0]) && parts[1] == "revisions" && strings.HasSuffix(parts[2], ".json") {
		valid = catalogIdentity.MatchString(strings.TrimSuffix(parts[2], ".json"))
	}
	if !valid {
		return catalogFile{}, errors.New("invalid indexed file identity")
	}
	info, err := root.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return catalogFile{Path: path}, nil
	}
	if err != nil {
		return catalogFile{}, err
	}
	if !info.Mode().IsRegular() {
		return catalogFile{}, errors.New("indexed input must be an ordinary file")
	}
	return catalogFile{path, info.Size(), info.ModTime().UnixNano(), uint32(info.Mode())}, nil
}

func catalogSignature(root *os.Root, paths ...string) (string, error) {
	files := make([]catalogFile, 0, len(paths))
	for _, path := range paths {
		file, err := catalogStat(root, path)
		if err != nil {
			return "", err
		}
		files = append(files, file)
	}
	data, _ := json.Marshal(files)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func catalogDependenciesCurrent(root *os.Root, id string, data []byte) bool {
	var files []catalogFile
	if json.Unmarshal(data, &files) != nil || len(files) == 0 || len(files) > 65 {
		return false
	}
	for _, prior := range files {
		if !strings.HasPrefix(filepath.ToSlash(prior.Path), id+"/") {
			return false
		}
		current, err := catalogStat(root, prior.Path)
		if err != nil || current != prior {
			return false
		}
	}
	return true
}

func (s Store) openCatalogIndex(ctx context.Context) (*sql.DB, func(), error) {
	parent := filepath.Dir(s.root)
	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	if err = root.Mkdir("cache", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, nil, err
	}
	info, err := root.Lstat("cache")
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("chat cache must be a private directory")
	}
	if err = root.Chmod("cache", 0700); err != nil {
		return nil, nil, err
	}
	lock, err := root.OpenFile("cache/.conversations.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, nil, err
	}
	for {
		err = filelock.Lock(lock, filelock.Exclusive|filelock.Nonblocking)
		if err == nil {
			break
		}
		if !errors.Is(err, filelock.ErrWouldBlock) {
			lock.Close()
			return nil, nil, err
		}
		select {
		case <-ctx.Done():
			lock.Close()
			return nil, nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	release := func() { _ = filelock.Unlock(lock); _ = lock.Close() }
	cache := filepath.Join(parent, "cache")
	path := filepath.Join(cache, catalogIndexName)
	for _, name := range []string{catalogIndexName, catalogIndexName + "-wal", catalogIndexName + "-shm"} {
		info, statErr := root.Lstat(filepath.Join("cache", name))
		if statErr == nil && (!info.Mode().IsRegular() || info.Size() > 512*1024*1024) {
			release()
			return nil, nil, errors.New("chat cache file is invalid or too large")
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			release()
			return nil, nil, statErr
		}
	}
	file, err := root.OpenFile(filepath.Join("cache", catalogIndexName), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		release()
		return nil, nil, err
	}
	err = file.Chmod(0600)
	file.Close()
	if err != nil {
		release()
		return nil, nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	values := url.Values{}
	for _, pragma := range []string{"busy_timeout(1000)", "journal_mode(WAL)", "synchronous(NORMAL)", "cache_size(-2048)", "temp_store(MEMORY)", "trusted_schema(OFF)", "secure_delete(ON)", "max_page_count(131072)"} {
		values.Add("_pragma", pragma)
	}
	u.RawQuery = values.Encode()
	open := func() (*sql.DB, error) {
		db, err := sql.Open("sqlite", u.String())
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		if err = initializeCatalog(ctx, db); err != nil {
			db.Close()
			return nil, err
		}
		return db, nil
	}
	db, err := open()
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.Code()&255 == 11 || sqliteErr.Code()&255 == 26) {
		// Only disposable files with known names are removed, under the cache
		// lock, after the connection has closed. Never repair canonical records.
		for _, name := range []string{catalogIndexName, catalogIndexName + "-wal", catalogIndexName + "-shm"} {
			if removeErr := root.Remove(filepath.Join("cache", name)); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				release()
				return nil, nil, removeErr
			}
		}
		file, err = root.OpenFile(filepath.Join("cache", catalogIndexName), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			release()
			return nil, nil, err
		}
		file.Close()
		db, err = open()
	}
	if err != nil {
		release()
		return nil, nil, err
	}
	return db, func() { _ = db.Close(); release() }, nil
}

func initializeCatalog(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return errors.New("chat cache was created by a newer release")
	}
	if version == 1 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
CREATE TABLE conversations (
 id TEXT PRIMARY KEY, title TEXT NOT NULL, source TEXT NOT NULL,
 updated TEXT NOT NULL, seconds INTEGER NOT NULL, nanos INTEGER NOT NULL,
 pinned INTEGER NOT NULL, archived INTEGER NOT NULL, version TEXT NOT NULL,
 title_fold TEXT NOT NULL, source_fold TEXT NOT NULL, signature TEXT NOT NULL,
 body TEXT NOT NULL DEFAULT '', body_fold TEXT NOT NULL DEFAULT '',
 dependencies BLOB NOT NULL DEFAULT '[]', body_state INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX conversation_order ON conversations(archived, pinned DESC, seconds DESC, nanos DESC, id);
CREATE VIRTUAL TABLE conversation_text USING fts5(title, source, body, tokenize='trigram case_sensitive 1');
INSERT INTO conversation_text(conversation_text, rank) VALUES('secure-delete', 1);
PRAGMA user_version=1;`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

type indexedConversation struct {
	Conversation
	signature    string
	dependencies []byte
	bodyState    int
}

func (s Store) pageConversationIndex(ctx context.Context, q ConversationQuery) (ConversationPage, error) {
	db, close, err := s.openCatalogIndex(ctx)
	if err != nil {
		return ConversationPage{}, err
	}
	defer close()
	page := ConversationPage{Items: []ConversationItem{}}
	if err = s.syncCatalog(ctx, db, q.Content && q.Text != "", &page); err != nil {
		return page, err
	}
	keyData, _ := json.Marshal(struct {
		Text, Scope string
		Content     bool
	}{q.Text, q.Scope, q.Content})
	sum := sha256.Sum256(keyData)
	key := hex.EncodeToString(sum[:])
	where := []string{"1=1"}
	args := []any{}
	if q.Scope != "all" {
		where = append(where, "archived=?")
		args = append(args, q.Scope == "archived")
	}
	if q.After != "" {
		var after catalogCursor
		data, err := base64.RawURLEncoding.DecodeString(q.After)
		if err != nil || json.Unmarshal(data, &after) != nil || after.Query != key || !catalogIdentity.MatchString(after.ID) || after.At.IsZero() {
			return page, errors.New("the conversation page does not match this search")
		}
		where = append(where, "(pinned < ? OR (pinned = ? AND (seconds < ? OR (seconds = ? AND (nanos < ? OR (nanos = ? AND id > ?))))))")
		args = append(args, after.Pinned, after.Pinned, after.At.Unix(), after.At.Unix(), after.At.Nanosecond(), after.At.Nanosecond(), after.ID)
	}
	term := strings.ToLower(q.Text)
	if term != "" {
		if utf8.RuneCountInString(term) >= 3 {
			columns := "{title source} : "
			if q.Content {
				columns = ""
			}
			where = append(where, "rowid IN (SELECT rowid FROM conversation_text WHERE conversation_text MATCH ?)")
			args = append(args, columns+`"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
		} else {
			clause := "(instr(title_fold,?)>0 OR instr(source_fold,?)>0"
			args = append(args, term, term)
			if q.Content {
				clause += " OR instr(body_fold,?)>0"
				args = append(args, term)
			}
			where = append(where, clause+")")
		}
	}
	args = append(args, q.Limit+1)
	bodyColumn := "''"
	if q.Content && term != "" {
		bodyColumn = "body"
	}
	rows, err := db.QueryContext(ctx, "SELECT id,title,source,updated,pinned,archived,version,"+bodyColumn+" FROM conversations WHERE "+strings.Join(where, " AND ")+" ORDER BY pinned DESC,seconds DESC,nanos DESC,id LIMIT ?", args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ConversationItem
		var updated, body string
		if err = rows.Scan(&item.ID, &item.Title, &item.Source, &updated, &item.Pinned, &item.Archived, &item.Version, &body); err != nil {
			return page, err
		}
		if item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
			return page, err
		}
		if len(page.Items) == q.Limit {
			last := page.Items[len(page.Items)-1]
			data, _ := json.Marshal(catalogCursor{key, last.ID, last.UpdatedAt, last.Pinned})
			page.Next = base64.RawURLEncoding.EncodeToString(data)
			break
		}
		if term != "" {
			switch {
			case strings.Contains(strings.ToLower(item.Title), term):
				item.Match = "title"
			case strings.Contains(strings.ToLower(item.Source), term):
				item.Match = "folder"
			default:
				item.Match = "messages"
				item.Snippet = catalogSnippet(body, term)
			}
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

func (s Store) syncCatalog(ctx context.Context, db *sql.DB, content bool, page *ConversationPage) error {
	statRoot, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer statRoot.Close()
	root, err := workspace.Open(s.root)
	if err != nil {
		return err
	}
	directory, err := os.Open(s.root)
	if err != nil {
		return err
	}
	entries, err := directory.ReadDir(catalogRecords + 1)
	directory.Close()
	if err != nil && err != io.EOF {
		return err
	}
	if len(entries) > catalogRecords {
		entries = entries[:catalogRecords]
		page.Incomplete = true
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	dependenciesColumn := "'[]'"
	if content {
		dependenciesColumn = "dependencies"
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,title,source,updated,pinned,archived,signature,"+dependenciesColumn+",body_state FROM conversations")
	if err != nil {
		return err
	}
	prior := map[string]indexedConversation{}
	for rows.Next() {
		var c indexedConversation
		var at string
		if err = rows.Scan(&c.ID, &c.Title, &c.SourcePath, &at, &c.Pinned, &c.Archived, &c.signature, &c.dependencies, &c.bodyState); err != nil {
			rows.Close()
			return err
		}
		c.UpdatedAt, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			rows.Close()
			return err
		}
		prior[c.ID] = c
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	metadataBytes, remaining := 0, catalogSearchBytes
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		id := entry.Name()
		if !entry.IsDir() || !catalogIdentity.MatchString(id) {
			continue
		}
		paths := []string{filepath.Join(id, "conversation.json"), filepath.Join(id, "display.json"), filepath.Join(id, "requests.json")}
		signature, sigErr := catalogSignature(statRoot, paths...)
		if sigErr != nil {
			page.Skipped++
			continue
		}
		cached, exists := prior[id]
		c := cached.Conversation
		if !exists || signature != cached.signature {
			data, readErr := root.ReadRegularFile(paths[0], 32*1024)
			metadataBytes += len(data)
			if metadataBytes > catalogMetadataBytes {
				page.Incomplete = true
				break
			}
			if readErr != nil || json.Unmarshal(data, &c) != nil || (c.Version != 1 && c.Version != Version) || c.ID != id || len(c.Title) > 256 || len(c.SourcePath) > 8192 || c.UpdatedAt.IsZero() {
				page.Skipped++
				continue
			}
			n, displayErr := readConversationDisplay(root, &c)
			metadataBytes += n
			if displayErr != nil {
				page.Skipped++
				continue
			}
			if after, e := catalogSignature(statRoot, paths...); e != nil || after != signature {
				page.Incomplete = true
				continue
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO conversations(id,title,source,updated,seconds,nanos,pinned,archived,version,title_fold,source_fold,signature)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET title=excluded.title,source=excluded.source,updated=excluded.updated,seconds=excluded.seconds,nanos=excluded.nanos,pinned=excluded.pinned,archived=excluded.archived,version=excluded.version,title_fold=excluded.title_fold,source_fold=excluded.source_fold,signature=excluded.signature,body='',body_fold='',dependencies='[]',body_state=0`, id, c.Title, c.SourcePath, c.UpdatedAt.Format(time.RFC3339Nano), c.UpdatedAt.Unix(), c.UpdatedAt.Nanosecond(), c.Pinned, c.Archived, ConversationDisplayVersion(c), strings.ToLower(c.Title), strings.ToLower(c.SourcePath), signature)
			if err != nil {
				return err
			}
			cached.bodyState = 0
			if err = updateCatalogFTS(ctx, tx, id); err != nil {
				return err
			}
		}
		seen[id] = true
		if !content {
			continue
		}
		if cached.bodyState > 0 && catalogDependenciesCurrent(statRoot, id, cached.dependencies) {
			page.Incomplete = page.Incomplete || cached.bodyState == 2
			continue
		}
		// A changed or deleted source invalidates its indexed text before any
		// search. A byte budget may delay rebuilding, but never returns old text.
		if remaining <= 0 {
			page.Incomplete = true
			_, err = tx.ExecContext(ctx, "UPDATE conversations SET body='',body_fold='',dependencies='[]',body_state=0 WHERE id=?", id)
		} else {
			if c.HeadRevision == "" { // Cached metadata intentionally omits replay/head.
				loaded, loadErr := s.Load(id)
				if loadErr != nil {
					page.Incomplete = true
					seen[id] = false
					continue
				}
				c = loaded
			}
			body, deps, partial := s.catalogBody(root, statRoot, c, &remaining)
			page.Incomplete = page.Incomplete || partial
			state := 1
			if partial {
				state = 2
				if remaining <= 0 {
					state = 0
				} // Continue a bounded initial build on the next query.
			}
			data, _ := json.Marshal(deps)
			_, err = tx.ExecContext(ctx, "UPDATE conversations SET body=?,body_fold=?,dependencies=?,body_state=? WHERE id=?", body, strings.ToLower(body), data, state, id)
		}
		if err != nil {
			return err
		}
		if err = updateCatalogFTS(ctx, tx, id); err != nil {
			return err
		}
	}
	for id := range prior {
		if !seen[id] {
			if _, err = tx.ExecContext(ctx, "DELETE FROM conversation_text WHERE rowid=(SELECT rowid FROM conversations WHERE id=?)", id); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "DELETE FROM conversations WHERE id=?", id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func updateCatalogFTS(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM conversation_text WHERE rowid=(SELECT rowid FROM conversations WHERE id=?)", id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO conversation_text(rowid,title,source,body) SELECT rowid,title_fold,source_fold,body_fold FROM conversations WHERE id=?", id)
	return err
}

func (s Store) catalogBody(root workspace.Root, statRoot *os.Root, c Conversation, remaining *int) (string, []catalogFile, bool) {
	deps := []catalogFile{}
	partial := false
	var body strings.Builder
	chatRemaining := 8 * 1024 * 1024
	add := func(text string) {
		if text == "" {
			return
		}
		available := catalogTextBytes - body.Len() - 1
		if available <= 0 {
			partial = true
			return
		}
		if len(text) > available {
			text = text[:available]
			for !utf8.ValidString(text) && len(text) > 0 {
				text = text[:len(text)-1]
			}
			partial = true
		}
		body.WriteString(text)
		body.WriteByte('\n')
	}
	read := func(path string) ([]byte, error) {
		before, err := catalogStat(statRoot, path)
		if err != nil {
			return nil, err
		}
		deps = append(deps, before)
		if before.Mode == 0 {
			return nil, os.ErrNotExist
		}
		if *remaining <= 0 || chatRemaining <= 0 {
			return nil, errors.New("chat indexing budget reached")
		}
		limit := min(*remaining, chatRemaining, 2*1024*1024)
		data, err := root.ReadRegularFile(path, int64(limit))
		consumed := min(int(before.Size), limit)
		*remaining -= consumed
		chatRemaining -= consumed
		after, statErr := catalogStat(statRoot, path)
		if statErr != nil || after != before {
			return nil, errors.New("chat changed during indexing")
		}
		return data, err
	}
	requests, err := read(filepath.Join(c.ID, "requests.json"))
	if err == nil {
		var journal struct {
			Requests []struct{ Text, State string } `json:"requests"`
		}
		if json.Unmarshal(requests, &journal) != nil {
			partial = true
		} else {
			for i := len(journal.Requests) - 1; i >= 0; i-- {
				if journal.Requests[i].State != "discarded" {
					add(journal.Requests[i].Text)
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		partial = true
	}
	id := c.HeadRevision
	seen := map[string]bool{}
	for i := 0; id != "" && i < 64; i++ {
		if !catalogIdentity.MatchString(id) || seen[id] {
			partial = true
			break
		}
		seen[id] = true
		data, err := read(filepath.Join(c.ID, "revisions", id+".json"))
		if err != nil {
			partial = true
			break
		}
		var revision struct {
			ConversationID string `json:"conversation_id"`
			Parent         string `json:"parent_revision_id"`
			Objective      string `json:"objective"`
			FinalText      string `json:"final_text"`
		}
		if json.Unmarshal(data, &revision) != nil || revision.ConversationID != c.ID {
			partial = true
			break
		}
		add(revision.Objective)
		add(revision.FinalText)
		id = revision.Parent
	}
	return body.String(), deps, partial || id != ""
}

func catalogSnippet(text, term string) string {
	lower := strings.ToLower(text)
	at := strings.Index(lower, term)
	if at < 0 {
		return ""
	}
	runes := []rune(text)
	before := len([]rune(lower[:at]))
	start := min(len(runes), max(0, before-48))
	end := min(len(runes), before+len([]rune(term))+160)
	snippet := strings.Join(strings.Fields(string(runes[start:end])), " ")
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}
