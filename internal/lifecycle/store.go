// Package lifecycle owns reviewed maintenance of Gator's private content. It
// never follows a source path out of the selected private state directory.
package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/state"
)

// Historical excerpts are intentionally included in history, independently
// of saved knowledge. Removing the latter alone cannot erase earlier inputs.
var content = map[string][]string{
	"knowledge": {"memory", "knowledge", "learnings", "observations", "retrieval"},
	"history":   {"conversations", "work-history", "workspaces", "work-control", "traces", "interactions", "tasks", "candidates", "delivery", "suggestions", "inbox", "pilot", "jobs/history", "observations"},
	"snapshots": {"snapshots"},
	"goals":     {"goals", "jobs/definitions"},
}

type Policy struct{ HistoryDays, KnowledgeDays, SnapshotDays int }

func (p Policy) Validate() error {
	for _, days := range []int{p.HistoryDays, p.KnowledgeDays, p.SnapshotDays} {
		if days < 0 || days > 36500 {
			return errors.New("retention must be 0 (keep until deleted) or 1–36500 days")
		}
	}
	return nil
}

type Group struct {
	Name  string
	Files int
	Bytes int64
}
type Inventory struct {
	Groups   []Group
	Policy   Policy
	StateDir string
}
type Item struct {
	Path      string
	Bytes     int64
	Modified  int64
	Digest    string
	Directory bool
}
type Plan struct {
	Complete  bool
	ID, Scope string
	CreatedAt time.Time
	Items     []Item
	Files     int
	Bytes     int64
	Notes     []string
}
type Store struct{ stateDir, root, control string }

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("private state directory required")
	}
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, err
	}
	root := filepath.Join(abs, "gator")
	control := filepath.Join(root, "data-control")
	if err = os.MkdirAll(filepath.Join(control, "plans"), 0700); err != nil {
		return Store{}, err
	}
	return Store{abs, root, control}, nil
}
func (s Store) Policy() (Policy, error) {
	var p Policy
	data, err := os.ReadFile(filepath.Join(s.control, "retention.json"))
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	return p, p.Validate()
}
func privateJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".data-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
func (s Store) SavePolicy(p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	release, err := state.Guard(s.stateDir, true)
	if err != nil {
		return err
	}
	defer release()
	return privateJSON(filepath.Join(s.control, "retention.json"), p)
}
func roots(scope string) ([]string, error) {
	if group, ok := content[scope]; ok {
		return append([]string(nil), group...), nil
	}
	if scope != "all" && scope != "retention" {
		return nil, errors.New("scope must be knowledge, history, snapshots, or all")
	}
	var paths []string
	for _, group := range content {
		paths = append(paths, group...)
	}
	sort.Strings(paths)
	return paths, nil
}
func (s Store) walk(ctx context.Context, paths []string, hash bool) ([]Item, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	seen := map[string]bool{}
	var out []Item
	for _, base := range paths {
		err = fs.WalkDir(root.FS(), base, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if errors.Is(walkErr, fs.ErrNotExist) && path == base {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if seen[path] {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			seen[path] = true
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("private data contains a symbolic link at %s; review it before maintenance", path)
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported private data entry %s", path)
			}
			item := Item{Path: path, Modified: info.ModTime().UnixNano(), Directory: info.IsDir()}
			if !item.Directory {
				item.Bytes = info.Size()
				if hash {
					f, err := root.Open(path)
					if err != nil {
						return err
					}
					digest := sha256.New()
					_, err = io.Copy(digest, f)
					f.Close()
					if err != nil {
						return err
					}
					item.Digest = hex.EncodeToString(digest.Sum(nil))
				}
			}
			out = append(out, item)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
func (s Store) Inventory(ctx context.Context) (Inventory, error) {
	out := Inventory{StateDir: s.stateDir}
	var err error
	out.Policy, err = s.Policy()
	if err != nil {
		return out, err
	}
	for _, name := range []string{"knowledge", "history", "snapshots", "goals"} {
		items, err := s.walk(ctx, content[name], false)
		if err != nil {
			return out, err
		}
		g := Group{Name: name}
		for _, item := range items {
			if !item.Directory {
				g.Files++
				g.Bytes += item.Bytes
			}
		}
		out.Groups = append(out.Groups, g)
	}
	return out, nil
}
func planID(p Plan) string {
	p.ID = ""
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (s Store) savePlan(p Plan) (Plan, error) {
	for _, item := range p.Items {
		if !item.Directory {
			p.Files++
			p.Bytes += item.Bytes
		}
	}
	p.ID = planID(p)
	return p, privateJSON(filepath.Join(s.control, "plans", p.ID+".json"), p)
}
func (s Store) PreviewDelete(ctx context.Context, scope string) (Plan, error) {
	paths, err := roots(scope)
	if err != nil {
		return Plan{}, err
	}
	release, err := state.Guard(s.stateDir, true)
	if err != nil {
		return Plan{}, err
	}
	defer release()
	if scope == "snapshots" {
		p, err := s.selectRetention(ctx, time.Now().Add(24*time.Hour), Policy{SnapshotDays: 1})
		if err != nil {
			return p, err
		}
		p.Scope = "snapshots"
		p.CreatedAt = time.Now().UTC()
		p.Notes = append(p.Notes, "Only unreferenced snapshots are selected; shared trees and blobs remain.")
		return s.savePlan(p)
	}
	items, err := s.walk(ctx, paths, true)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Complete: true, Scope: scope, CreatedAt: time.Now().UTC(), Items: items, Notes: []string{"Only private Gator content is selected. Original project files, provider data, external exports, credentials, settings, and shared usage counters are not deleted.", "Filesystem deletion is not guaranteed secure erasure on SSDs or backups."}}
	if scope == "knowledge" {
		p.Notes = append(p.Notes, "Prior transcripts, Work outputs, suggestions and retained excerpts can still contain this knowledge. Choose all private content to remove those local copies too.")
	}
	if scope == "history" {
		p.Notes = append(p.Notes, "Saved facts and source snapshots remain. Goal and job conversation links will be cleared so future work starts a fresh conversation.")
	}
	if scope == "snapshots" {
		p.Notes = append(p.Notes, "Referenced snapshots are protected. Use all private content to delete snapshots and the retained Work that refers to them.")
	}
	return s.savePlan(p)
}
func (s Store) LoadPlan(id string) (Plan, error) {
	if len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return Plan{}, errors.New("invalid data review ID")
	}
	data, err := os.ReadFile(filepath.Join(s.control, "plans", id+".json"))
	if err != nil {
		return Plan{}, err
	}
	if len(data) > 64<<20 {
		return Plan{}, errors.New("data review is too large")
	}
	var p Plan
	if err = json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	if p.ID != id || planID(p) != id || time.Since(p.CreatedAt) > time.Hour {
		return p, errors.New("data review changed or expired; preview again")
	}
	return p, nil
}
func (s Store) Apply(ctx context.Context, id string) (int, error) {
	release, err := state.Guard(s.stateDir, true)
	if err != nil {
		return 0, err
	}
	defer release()
	return s.applyLocked(ctx, id)
}
func (s Store) applyLocked(ctx context.Context, id string) (int, error) {
	p, err := s.LoadPlan(id)
	if err != nil {
		return 0, err
	}
	// Snapshot and history reference handling is validated before removing any
	// selected file. A changed review always requires a new preview.
	if err = s.validateReferences(p); err != nil {
		return 0, err
	}
	paths, err := roots(p.Scope)
	if err != nil {
		return 0, err
	}
	if !p.Complete {
		paths = nil
		for _, item := range p.Items {
			paths = append(paths, item.Path)
		}
	}
	current, err := s.walk(ctx, paths, true)
	if err != nil {
		return 0, err
	}
	if !sameItems(current, p.Items) {
		return 0, errors.New("private data changed after review; preview the deletion again")
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	removed := 0
	for i := len(p.Items) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		item := p.Items[i]
		if err = root.Remove(item.Path); err != nil {
			return removed, err
		}
		if !item.Directory {
			removed++
		}
	}
	if p.Scope == "history" {
		if err = s.clearConversationLinks(nil); err != nil {
			return removed, err
		}
	}
	if p.Scope == "retention" {
		removedConversations := map[string]bool{}
		for _, item := range p.Items {
			if strings.HasPrefix(filepath.ToSlash(item.Path), "conversations/") {
				parts := strings.Split(filepath.ToSlash(item.Path), "/")
				if len(parts) >= 2 {
					removedConversations[parts[1]] = true
				}
			}
		}
		if err = s.clearConversationLinks(removedConversations); err != nil {
			return removed, err
		}
	}
	if p.Scope == "all" {
		if err = root.RemoveAll("data-control/plans"); err != nil {
			return removed, err
		}
	} else {
		_ = os.Remove(filepath.Join(s.control, "plans", id+".json"))
	}
	return removed, nil
}
func sameItems(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
