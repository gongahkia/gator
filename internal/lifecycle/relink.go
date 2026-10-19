package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/state"
)

var ErrRecovery = errors.New("an interrupted source relink needs recovery")

type RelinkEdit struct {
	Path   string
	Before []byte
	After  []byte
}
type RelinkPlan struct {
	ID        string
	CreatedAt time.Time
	From      string
	To        string
	Edits     []RelinkEdit
}
type relinkJournal struct {
	Plan      RelinkPlan
	Committed bool
}

func relinkID(p RelinkPlan) string {
	p.ID = ""
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (s Store) PreviewRelink(ctx context.Context, from, to string) (RelinkPlan, error) {
	p := RelinkPlan{CreatedAt: time.Now().UTC()}
	mappings, err := normalizeMappings([]SourceMap{{from, to}}, s.stateDir)
	if err != nil {
		return p, err
	}
	p.From, p.To = mappings[0].From, mappings[0].To
	// A private state source is relocated only by a fresh-profile restore.
	if withinPath(s.stateDir, p.From) {
		return p, errors.New("private profile paths cannot be relinked; use restore into a fresh profile")
	}
	release, err := state.Guard(s.stateDir, true)
	if err != nil {
		return p, err
	}
	defer release()
	if err = s.ensureIdle(); err != nil {
		return p, err
	}
	items, err := s.walk(ctx, []string{"goals", "conversations", "jobs/definitions", "snapshots/manifests", "workspaces", "suggestions"}, false)
	if err != nil {
		return p, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return p, err
	}
	defer root.Close()
	for _, item := range items {
		if item.Directory || !migrationMetadata(item.Path) {
			continue
		}
		if item.Bytes > 16<<20 {
			return p, fmt.Errorf("source metadata is too large: %s", item.Path)
		}
		file, err := root.Open(item.Path)
		if err != nil {
			return p, err
		}
		data, err := io.ReadAll(cancelReader{ctx, file})
		file.Close()
		if err != nil {
			return p, err
		}
		var object map[string]json.RawMessage
		if err = json.Unmarshal(data, &object); err != nil {
			return p, err
		}
		parts := strings.Split(filepath.ToSlash(item.Path), "/")
		key := ""
		switch parts[0] {
		case "goals":
			key = "source_path"
		case "conversations":
			if parts[len(parts)-1] == "conversation.json" {
				key = "source_path"
			}
		case "jobs":
			key = "source_path"
		case "snapshots":
			key = "source_path"
		case "workspaces":
			key = "source"
		case "suggestions":
			key = "goal_source_path"
		}
		if key == "" {
			continue
		}
		value := stringField(object, key)
		comparable := canonicalSourcePath(value)
		if !withinPath(p.From, comparable) {
			continue
		}
		replacement := replaceRoot(comparable, p.From, p.To)
		if info, err := os.Stat(replacement); err != nil || !info.IsDir() {
			return p, fmt.Errorf("mapped source directory is unavailable: %s", replacement)
		}
		object[key], _ = json.Marshal(replacement)
		if parts[0] == "conversations" {
			object["source_refresh_required"] = json.RawMessage("true")
		}
		if parts[0] == "jobs" {
			object["enabled"] = json.RawMessage("false")
			delete(object, "last_scheduled_at")
			delete(object, "oauth_refresh_pins")
			delete(object, "oauth_account_pins")
		}
		updated, err := json.MarshalIndent(object, "", "  ")
		if err != nil {
			return p, err
		}
		p.Edits = append(p.Edits, RelinkEdit{item.Path, data, append(updated, '\n')})
	}
	if len(p.Edits) == 0 {
		return p, errors.New("no source references match that old folder")
	}
	p.ID = relinkID(p)
	data, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if len(data) > 32<<20 {
		return p, errors.New("source relink exceeds its 32 MiB metadata review bound")
	}
	return p, privateJSON(filepath.Join(s.control, "plans", "relink-"+p.ID+".json"), p)
}
func (s Store) LoadRelinkPlan(id string) (RelinkPlan, error) {
	var p RelinkPlan
	if len(id) != 64 {
		return p, errors.New("invalid source relink review")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return p, err
	}
	data, err := os.ReadFile(filepath.Join(s.control, "plans", "relink-"+id+".json"))
	if err != nil {
		return p, err
	}
	if len(data) > 32<<20 {
		return p, errors.New("source relink review is too large")
	}
	if err = json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	if relinkID(p) != id || p.ID != id || time.Since(p.CreatedAt) > time.Hour {
		return p, errors.New("source relink review changed or expired")
	}
	return p, nil
}
func (s Store) Relink(ctx context.Context, id string) (int, error) {
	p, err := s.LoadRelinkPlan(id)
	if err != nil {
		return 0, err
	}
	release, err := state.Guard(s.stateDir, true)
	if err != nil {
		return 0, err
	}
	defer release()
	if err = s.ensureIdle(); err != nil {
		return 0, err
	}
	maps, err := normalizeMappings([]SourceMap{{p.From, p.To}}, s.stateDir)
	if err != nil {
		return 0, err
	}
	if maps[0].From != p.From || maps[0].To != p.To {
		return 0, errors.New("source mapping changed during review")
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	for _, edit := range p.Edits {
		current, err := root.ReadFile(edit.Path)
		if err != nil {
			return 0, err
		}
		if string(current) != string(edit.Before) {
			return 0, errors.New("source references changed after review; preview again")
		}
		var object map[string]json.RawMessage
		if err = json.Unmarshal(edit.After, &object); err != nil {
			return 0, err
		}
		for _, key := range []string{"source_path", "source", "goal_source_path"} {
			value := stringField(object, key)
			if withinPath(p.To, value) {
				if info, err := os.Stat(value); err != nil || !info.IsDir() {
					return 0, errors.New("mapped source is no longer available")
				}
			}
		}
	}
	journal := relinkJournal{Plan: p}
	journalPath := filepath.Join(s.control, "relink-pending.json")
	if err = privateJSON(journalPath, journal); err != nil {
		return 0, err
	}
	rollback := func(cause error) (int, error) {
		recoveryErr := s.recoverRelinkLocked()
		return 0, errors.Join(cause, recoveryErr)
	}
	for _, edit := range p.Edits {
		if err = ctx.Err(); err != nil {
			return rollback(err)
		}
		original := filepath.Join("migrations", "relink-"+p.ID, "originals", edit.Path)
		if err = root.MkdirAll(filepath.Dir(original), 0700); err != nil {
			return rollback(err)
		}
		if err = writeRootBytes(root, original, edit.Before); err != nil {
			return rollback(err)
		}
		if err = writeRootBytes(root, edit.Path, edit.After); err != nil {
			return rollback(err)
		}
	}
	journal.Committed = true
	if err = privateJSON(journalPath, journal); err != nil {
		return rollback(err)
	}
	_ = os.Remove(journalPath)
	_ = os.Remove(filepath.Join(s.control, "plans", "relink-"+id+".json"))
	return len(p.Edits), nil
}
func writeRootBytes(root *os.Root, path string, data []byte) error {
	sum := sha256.Sum256([]byte(path + time.Now().String()))
	temp := filepath.Join(filepath.Dir(path), ".relink-"+hex.EncodeToString(sum[:8]))
	file, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = root.Rename(temp, path); err != nil {
		return err
	}
	dir, err := root.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (s Store) recoverRelink() error {
	if _, err := os.Lstat(filepath.Join(s.control, "relink-pending.json")); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	release, err := state.Guard(s.stateDir, true)
	if err != nil {
		return err
	}
	defer release()
	return s.recoverRelinkLocked()
}
func (s Store) recoverRelinkLocked() error {
	var journal relinkJournal
	data, err := os.ReadFile(filepath.Join(s.control, "relink-pending.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) > 32<<20 {
		return errors.New("invalid source relink recovery journal")
	}
	if err = json.Unmarshal(data, &journal); err != nil {
		return err
	}
	if relinkID(journal.Plan) != journal.Plan.ID {
		return errors.New("source relink journal changed; inspect it before continuing")
	}
	if !journal.Committed {
		root, err := os.OpenRoot(s.root)
		if err != nil {
			return err
		}
		defer root.Close()
		for _, edit := range journal.Plan.Edits {
			if !migrationMetadata(edit.Path) || !filepath.IsLocal(edit.Path) {
				return errors.New("unsafe source relink journal path")
			}
			current, err := root.ReadFile(edit.Path)
			if err != nil {
				return err
			}
			if string(current) != string(edit.Before) && string(current) != string(edit.After) {
				return errors.New("source metadata changed outside an interrupted relink; inspect the recovery journal")
			}
			if err = writeRootBytes(root, edit.Path, edit.Before); err != nil {
				return err
			}
		}
	}
	return os.Remove(filepath.Join(s.control, "relink-pending.json"))
}

// SourceFolders reports current live navigation roots, rather than an old
// restore receipt, so a later relink is immediately reflected in /data.
func (s Store) SourceFolders(ctx context.Context) ([]RestoreSource, error) {
	release, err := state.Guard(s.stateDir, false)
	if err != nil {
		return nil, err
	}
	defer release()
	items, err := s.walk(ctx, []string{"goals", "conversations", "jobs/definitions"}, false)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	seen := map[string]bool{}
	var out []RestoreSource
	for _, item := range items {
		if item.Directory || !migrationMetadata(item.Path) {
			continue
		}
		if strings.Contains(filepath.ToSlash(item.Path), "/revisions/") {
			continue
		}
		file, err := root.Open(item.Path)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(cancelReader{ctx, file}, (16<<20)+1))
		file.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > 16<<20 {
			return nil, errors.New("source metadata is too large")
		}
		var object map[string]json.RawMessage
		if err = json.Unmarshal(data, &object); err != nil {
			return nil, err
		}
		value := stringField(object, "source_path")
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		info, err := os.Stat(value)
		out = append(out, RestoreSource{Original: value, Destination: value, Available: err == nil && info.IsDir()})
	}
	return out, nil
}
