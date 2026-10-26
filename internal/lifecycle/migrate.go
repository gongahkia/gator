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

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func canonicalSourcePath(value string) string {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return value
	}
	probe := absolute
	for {
		if resolved, err := filepath.EvalSymlinks(probe); err == nil {
			suffix, _ := filepath.Rel(probe, absolute)
			return filepath.Join(resolved, suffix)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return absolute
		}
		probe = parent
	}
}

func normalizeMappings(mappings []SourceMap, target string) ([]SourceMap, error) {
	out := append([]SourceMap(nil), mappings...)
	seen := map[string]bool{}
	for i, m := range out {
		if !filepath.IsAbs(m.From) || strings.ContainsAny(m.From+m.To, "\x00\r\n") {
			return nil, errors.New("source mappings require absolute paths")
		}
		m.From = canonicalSourcePath(m.From)
		if seen[m.From] {
			return nil, errors.New("a source path was mapped more than once")
		}
		seen[m.From] = true
		if !filepath.IsAbs(m.To) {
			return nil, errors.New("mapped source destinations must be absolute")
		}
		destination, err := filepath.EvalSymlinks(m.To)
		if err != nil {
			return nil, fmt.Errorf("mapped source must exist: %w", err)
		}
		info, err := os.Stat(destination)
		if err != nil || !info.IsDir() {
			return nil, errors.New("mapped source must be a directory")
		}
		if withinPath(target, destination) || withinPath(destination, target) {
			return nil, errors.New("mapped sources and the new profile must not overlap")
		}
		m.To = destination
		out[i] = m
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].From) != len(out[j].From) {
			return len(out[i].From) > len(out[j].From)
		}
		return out[i].From < out[j].From
	})
	return out, nil
}
func withinPath(root, value string) bool {
	rel, err := filepath.Rel(root, value)
	return err == nil && (rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."))
}
func replaceRoot(value, from, to string) string {
	if !filepath.IsAbs(value) || !withinPath(from, value) {
		return value
	}
	rel, _ := filepath.Rel(from, value)
	return filepath.Join(to, rel)
}

// Only host-owned navigation metadata is rewritten. Frozen data, artifact
// manifests, patches, excerpts, project bundles, and replay messages stay exact.
func migrationMetadata(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if !strings.HasSuffix(rel, ".json") {
		return false
	}
	switch parts[0] {
	case "goals":
		return len(parts) == 2
	case "conversations":
		return len(parts) == 3 && parts[2] == "conversation.json" || len(parts) == 4 && parts[2] == "revisions"
	case "workspaces":
		return len(parts) == 4 && parts[3] == "workspace.json"
	case "snapshots":
		return len(parts) == 3 && parts[1] == "manifests"
	case "work-history", "jobs", "suggestions", "inbox", "tasks", "candidates", "delivery":
		return true
	}
	return false
}
func migrateProfile(ctx context.Context, profile string, p RestorePlan, apply bool) ([]RestoreSource, int, int, error) {
	rootPath := filepath.Join(profile, "gator")
	sources := map[string]RestoreSource{}
	checks, runs := 0, 0
	paths := []string{}
	err := filepath.WalkDir(rootPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}
		if migrationMetadata(rel) {
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		return nil, 0, 0, err
	}
	mapSource := func(value string) string {
		comparable := canonicalSourcePath(value)
		destination := replaceRoot(value, p.Origin, p.Target)
		if destination == value {
			destination = replaceRoot(comparable, p.Origin, p.Target)
		}
		if destination == comparable || destination == value {
			for _, mapping := range p.Mappings {
				if withinPath(mapping.From, comparable) {
					destination = replaceRoot(comparable, mapping.From, mapping.To)
					break
				}
			}
		}
		checkPath := destination
		if withinPath(p.Target, destination) {
			checkPath = replaceRoot(destination, p.Target, profile)
		}
		info, statErr := os.Stat(checkPath)
		sources[value] = RestoreSource{Original: value, Destination: destination, Available: statErr == nil && info.IsDir()}
		return destination
	}
	for _, rel := range paths {
		path := filepath.Join(rootPath, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, 0, 0, err
		}
		if len(data) > 16<<20 {
			return nil, 0, 0, fmt.Errorf("migration metadata is too large: %s", rel)
		}
		var object map[string]json.RawMessage
		if err = json.Unmarshal(data, &object); err != nil {
			return nil, 0, 0, fmt.Errorf("invalid retained metadata %s: %w", rel, err)
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		// Rebase internal references recursively, but leave historical inputs intact.
		var rebase func(map[string]json.RawMessage)
		rebase = func(o map[string]json.RawMessage) {
			for key, raw := range o {
				if key == "replay" || key == "project_configuration" || key == "project" || key == "Project" || key == "decision" {
					continue
				}
				var value string
				if json.Unmarshal(raw, &value) == nil {
					switch key {
					case "bundle_path", "artifact_manifest_path", "trace_path", "interactions_path", "tasks_path", "candidates_path", "deliveries_path", "workspace_path", "worktree_path":
						o[key], _ = json.Marshal(replaceRoot(value, p.Origin, p.Target))
					}
					continue
				}
				var nested map[string]json.RawMessage
				if json.Unmarshal(raw, &nested) == nil && nested != nil {
					rebase(nested)
					o[key], _ = json.Marshal(nested)
					continue
				}
				var list []map[string]json.RawMessage
				if json.Unmarshal(raw, &list) == nil && list != nil {
					for _, n := range list {
						rebase(n)
					}
					o[key], _ = json.Marshal(list)
				}
			}
		}
		rebase(object)
		sourceKey := ""
		switch parts[0] {
		case "goals", "snapshots":
			sourceKey = "source_path"
		case "conversations":
			if parts[len(parts)-1] == "conversation.json" {
				sourceKey = "source_path"
				object["source_refresh_required"] = json.RawMessage("true")
			} else {
				// Imported configuration cannot reactivate grants, browser sessions, or
				// tool authority. The original revision is retained under migrations/.
				var replay map[string]json.RawMessage
				if raw, ok := object["replay"]; ok && json.Unmarshal(raw, &replay) == nil && replay != nil {
					delete(replay, "configuration")
					object["replay"], _ = json.Marshal(replay)
				}
			}
		case "workspaces":
			sourceKey = "source"
		case "jobs":
			if len(parts) == 3 && parts[1] == "definitions" {
				sourceKey = "source_path"
				var enabled bool
				_ = json.Unmarshal(object["enabled"], &enabled)
				if enabled {
					checks++
				}
				object["enabled"] = json.RawMessage("false")
				delete(object, "last_scheduled_at")
				delete(object, "oauth_refresh_pins")
				delete(object, "oauth_account_pins")
			}
		case "suggestions":
			sourceKey = "goal_source_path"
		case "work-history":
			if stringField(object, "status") == "running" {
				runs++
				object["status"] = json.RawMessage(`"failed"`)
				object["finished_at"], _ = json.Marshal(p.CreatedAt)
				object["verification_status"] = json.RawMessage(`"unavailable"`)
			}
		}
		if sourceKey != "" {
			value := stringField(object, sourceKey)
			if value != "" {
				destination := replaceRoot(value, p.Origin, p.Target)
				// Empty private goal sources have no archived files, but remain usable.
				private := replaceRoot(destination, p.Target, profile)
				if destination != value && (strings.Contains(filepath.ToSlash(private), "/gator/goals/sources/") || filepath.Base(private) == "empty-workspace") {
					if err = os.MkdirAll(private, 0700); err != nil {
						return nil, 0, 0, err
					}
				}
				object[sourceKey], _ = json.Marshal(mapSource(value))
			}
		}
		if parts[0] == "workspaces" {
			for _, dir := range []string{"output", "scratch"} {
				if err = os.MkdirAll(filepath.Join(filepath.Dir(path), dir), 0700); err != nil {
					return nil, 0, 0, err
				}
			}
		}
		if parts[0] == "snapshots" {
			tree := stringField(object, "tree_id")
			if tree == "" {
				tree = stringField(object, "id")
			}
			if tree != "" && !strings.ContainsAny(tree, "/\\") {
				if err = os.MkdirAll(filepath.Join(rootPath, "snapshots", "trees", tree), 0700); err != nil {
					return nil, 0, 0, err
				}
			}
		}
		if apply {
			updated, err := json.MarshalIndent(object, "", "  ")
			if err != nil {
				return nil, 0, 0, err
			}
			original := filepath.Join(rootPath, "migrations", p.ID, "originals", rel)
			if err = os.MkdirAll(filepath.Dir(original), 0700); err != nil {
				return nil, 0, 0, err
			}
			if err = os.WriteFile(original, data, 0600); err != nil {
				return nil, 0, 0, err
			}
			if err = os.WriteFile(path, append(updated, '\n'), 0600); err != nil {
				return nil, 0, 0, err
			}
		}
	}
	out := make([]RestoreSource, 0, len(sources))
	for _, source := range sources {
		checkPath := replaceRoot(source.Destination, p.Target, profile)
		info, err := os.Stat(checkPath)
		source.Available = err == nil && info.IsDir()
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Original < out[j].Original })
	return out, checks, runs, nil
}

func validateRestoredProfile(ctx context.Context, profile, final string) error {
	goals, err := goal.Open(profile)
	if err != nil {
		return err
	}
	records, err := goals.List()
	if err != nil {
		return err
	}
	for _, g := range records {
		if _, err = goals.LoadPlan(g.ID); err != nil {
			return err
		}
	}
	sessions, err := worksession.Open(profile)
	if err != nil {
		return err
	}
	conversations, err := sessions.List(0)
	if err != nil {
		return err
	}
	for _, c := range conversations {
		// Backups retain files, so an unsent chat's empty revisions directory
		// is absent after extraction. Recreate it only when no head is claimed;
		// conversations referring to missing revisions must still fail validation.
		if c.HeadRevision == "" {
			if err = os.MkdirAll(filepath.Join(profile, "gator", "conversations", c.ID, "revisions"), 0700); err != nil {
				return err
			}
		}
		revisions, err := sessions.Revisions(c.ID)
		if err != nil {
			return err
		}
		if c.HeadRevision != "" {
			if _, err = sessions.LoadRevision(c.ID, c.HeadRevision); err != nil {
				return err
			}
		}
		for _, r := range revisions {
			if !withinPath(filepath.Join(final, "gator", "workspaces"), r.BundlePath) {
				return errors.New("restored revision points outside private Work")
			}
			bundle := replaceRoot(r.BundlePath, final, profile)
			opened, openErr := artifact.OpenBundle(bundle)
			if openErr != nil {
				return fmt.Errorf("restored revision %s: %w", r.ID, openErr)
			}
			if err = artifact.VerifyBundle(opened); err != nil {
				return fmt.Errorf("restored revision %s evidence: %w", r.ID, err)
			}
			if r.ParentRevisionID != "" {
				if _, err = sessions.LoadRevision(c.ID, r.ParentRevisionID); err != nil {
					return err
				}
			}
		}
	}
	err = filepath.WalkDir(filepath.Join(profile, "gator", "conversations"), func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Join(profile, "gator", "conversations"), path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) == 2 && parts[1] == "conversation.json" {
			_, err = sessions.Load(parts[0])
			return err
		}
		if len(parts) == 3 && parts[1] == "revisions" && strings.HasSuffix(parts[2], ".json") {
			_, err = sessions.LoadRevision(parts[0], strings.TrimSuffix(parts[2], ".json"))
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	history, err := workhistory.Open(profile)
	if err != nil {
		return err
	}
	if _, err = history.List(0); err != nil {
		return err
	}
	definitions, err := jobs.Open(profile)
	if err != nil {
		return err
	}
	if _, err = definitions.List(); err != nil {
		return err
	}
	manifests, err := snapshot.List(profile)
	if err != nil {
		return err
	}
	checked := map[string]bool{}
	root, err := os.OpenRoot(profile)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, m := range manifests {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, entry := range m.Entries {
			if _, err = hex.DecodeString(entry.SHA256); err != nil {
				return errors.New("snapshot has an invalid blob digest")
			}
			for _, relative := range []string{filepath.Join("gator", "snapshots", "blobs", entry.SHA256), filepath.Join("gator", "snapshots", "trees", m.TreeID, filepath.FromSlash(entry.Path))} {
				if checked[relative] {
					continue
				}
				file, err := root.Open(relative)
				if err != nil {
					return fmt.Errorf("snapshot content missing: %w", err)
				}
				hash := sha256.New()
				n, err := io.Copy(hash, cancelReader{ctx, file})
				file.Close()
				if err != nil {
					return err
				}
				if n != entry.Bytes || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
					return errors.New("restored snapshot content does not match its evidence")
				}
				checked[relative] = true
			}
		}
	}
	return filepath.WalkDir(filepath.Join(profile, "gator", "workspaces"), func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "manifest.json" {
			return nil
		}
		// Only a workspace root manifest is a sealed bundle, never an output file
		// whose user-selected name happens to be manifest.json.
		rel, _ := filepath.Rel(filepath.Join(profile, "gator", "workspaces"), path)
		if len(strings.Split(filepath.ToSlash(rel), "/")) != 3 {
			return nil
		}
		opened, err := artifact.OpenBundle(filepath.Dir(path))
		if err != nil {
			return err
		}
		return artifact.VerifyBundle(opened)
	})
}

// SourceMappings parses one OLD=NEW mapping per line for the terminal form.
func SourceMappings(value string) ([]SourceMap, error) {
	var out []SourceMap
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		from, to, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
			return nil, errors.New("source mappings use one absolute OLD=NEW pair per line")
		}
		out = append(out, SourceMap{From: strings.TrimSpace(from), To: strings.TrimSpace(to)})
	}
	return out, nil
}
