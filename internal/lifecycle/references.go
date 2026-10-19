package lifecycle

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/workhistory"
)

func (s Store) ensureIdle() error {
	if _, err := os.Stat(filepath.Join(s.root, "work-history")); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	history, err := workhistory.Open(s.stateDir)
	if err != nil {
		return err
	}
	records, err := history.List(0)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Status == workhistory.Running && record.LiveStatus == "active" {
			return errors.New("a retained Work lease is active; finish or stop it before data maintenance")
		}
	}
	return nil
}

func (s Store) validateReferences(p Plan) error {
	if p.Scope == "all" {
		return nil
	}
	removed := map[string]bool{}
	hasSnapshots := false
	for _, item := range p.Items {
		removed[filepath.ToSlash(item.Path)] = true
		if strings.HasPrefix(filepath.ToSlash(item.Path), "snapshots/") {
			hasSnapshots = true
		}
	}
	if !hasSnapshots {
		return nil
	}
	refs, err := s.snapshotReferences(removed)
	if err != nil {
		return err
	}
	for id := range refs {
		if removed["snapshots/manifests/"+id+".json"] {
			return errors.New("a retained Work or schedule now refers to a selected snapshot; preview cleanup again")
		}
	}
	manifests, err := snapshot.List(s.stateDir)
	if err != nil {
		return err
	}
	for _, m := range manifests {
		if removed["snapshots/manifests/"+m.ID+".json"] {
			continue
		}
		if removed["snapshots/trees/"+m.TreeID] {
			return errors.New("a retained snapshot shares a selected tree; preview cleanup again")
		}
		for _, entry := range m.Entries {
			if removed["snapshots/blobs/"+entry.SHA256] {
				return errors.New("a retained snapshot shares a selected blob; preview cleanup again")
			}
		}
	}
	return nil
}
func (s Store) snapshotReferences(removed map[string]bool) (map[string]bool, error) {
	refs := map[string]bool{}
	for _, base := range []string{"work-history", "conversations", "jobs/definitions"} {
		err := filepath.WalkDir(filepath.Join(s.root, base), func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(s.root, path)
			if err != nil {
				return err
			}
			if removed[filepath.ToSlash(rel)] {
				return nil
			}
			if filepath.Ext(path) != ".json" {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Size() > 16<<20 {
				return errors.New("invalid retained snapshot reference")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var value map[string]json.RawMessage
			if err = json.Unmarshal(data, &value); err != nil {
				return err
			}
			var id string
			if raw := value["snapshot_id"]; raw != nil {
				if err = json.Unmarshal(raw, &id); err != nil {
					return err
				}
				if id != "" {
					refs[id] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return refs, nil
}
func (s Store) clearConversationLinks(only map[string]bool) error {
	entries, err := os.ReadDir(filepath.Join(s.root, "goals"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.root, "goals", entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value map[string]json.RawMessage
		if err = json.Unmarshal(data, &value); err != nil {
			return err
		}
		if only != nil && !only[stringField(value, "conversation_id")] {
			continue
		}
		delete(value, "conversation_id")
		if err = privateJSON(path, value); err != nil {
			return err
		}
	}
	return nil
}
