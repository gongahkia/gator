package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func (s Store) objects(base string) (map[string]map[string]json.RawMessage, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	out := map[string]map[string]json.RawMessage{}
	err = fs.WalkDir(root.FS(), base, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 16<<20 {
			return errors.New("invalid private metadata")
		}
		f, err := root.Open(path)
		if err != nil {
			return err
		}
		var object map[string]json.RawMessage
		err = json.NewDecoder(f).Decode(&object)
		f.Close()
		if err != nil {
			return err
		}
		out[filepath.ToSlash(path)] = object
		return nil
	})
	return out, err
}
func stringField(object map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(object[key], &value)
	return value
}
func timeField(object map[string]json.RawMessage, key string) time.Time {
	var value time.Time
	_ = json.Unmarshal(object[key], &value)
	return value
}
func (s Store) selectRetention(ctx context.Context, now time.Time, p Policy) (Plan, error) {
	plan := Plan{Scope: "retention", CreatedAt: now.UTC(), Notes: []string{"Retention uses last modification of saved facts and notes, and last activity of completed Work and conversations.", "Ongoing goals, their conversations, the latest successful check per job, pending suggestions, and referenced source snapshots are protected.", "Expired canonical knowledge can still appear in retained historical inputs until history is removed. Skills and learnings are kept until explicitly deleted.", "External projects, credentials, exports, and shared usage counters are preserved."}}
	plan.Notes = append(plan.Notes, "File working versions and edit drafts are kept until you explicitly delete history or all private content; automatic age retention does not remove them.")
	var selected []string
	goals, err := goal.Open(s.stateDir)
	if err != nil {
		return plan, err
	}
	records, err := goals.List()
	if err != nil {
		return plan, err
	}
	protectedGoal := map[string]bool{}
	protectedConversation := map[string]bool{}
	protectedWork := map[string]bool{}
	for _, g := range records {
		if g.Status != goal.Completed {
			protectedGoal[g.ID] = true
			if g.ConversationID != "" {
				protectedConversation[g.ConversationID] = true
			}
		}
	}
	journal, err := actionlog.Open(s.stateDir)
	if err != nil {
		return plan, err
	}
	effects, err := journal.List("")
	if err != nil {
		return plan, err
	}
	for _, effect := range effects {
		if effect.Phase == "executing" || effect.Action.Status == action.Unknown {
			protectedWork[effect.RunID] = true
			protectedConversation[effect.ConversationID] = true
		}
	}
	plan.Notes = append(plan.Notes, "Unconfirmed external requests and their chats are protected from automatic age retention. Explicit history deletion removes their receipts without undoing any external effect.")
	jobStore, err := jobs.Open(s.stateDir)
	if err != nil {
		return plan, err
	}
	definitions, err := jobStore.List()
	if err != nil {
		return plan, err
	}
	for _, definition := range definitions {
		history, err := jobStore.History(definition.ID, 0)
		if err != nil {
			return plan, err
		}
		keep := ""
		for _, a := range history {
			if a.Status == "completed" {
				keep = a.ID
				if a.Conversation != "" {
					protectedConversation[a.Conversation] = true
				}
				for _, run := range a.Runs {
					protectedWork[run.RunID] = true
				}
				break
			}
		}
		if p.HistoryDays > 0 {
			cut := now.Add(-time.Duration(p.HistoryDays) * 24 * time.Hour)
			for _, a := range history {
				if a.ID != keep && a.Status != "running" && a.StartedAt.Before(cut) {
					selected = append(selected, filepath.Join("jobs/history", definition.ID, a.ID))
				}
			}
		}
	}
	suggestions, err := s.objects("suggestions")
	if err != nil {
		return plan, err
	}
	for _, o := range suggestions {
		if stringField(o, "status") == "pending" {
			protectedWork[stringField(o, "work_id")] = true
		}
	}
	historyStore, err := workhistory.Open(s.stateDir)
	if err != nil {
		return plan, err
	}
	history, err := historyStore.List(0)
	if err != nil {
		return plan, err
	}
	for _, run := range history {
		// A retained run must not lose its conversation, including an interrupted
		// run whose last conversation update predates the retention cutoff.
		keep := protectedWork[run.ID] || run.Status == workhistory.Running || run.FinishedAt.IsZero()
		if p.HistoryDays > 0 && !run.FinishedAt.Before(now.Add(-time.Duration(p.HistoryDays)*24*time.Hour)) {
			keep = true
		}
		if keep && run.ConversationID != "" {
			protectedConversation[run.ConversationID] = true
		}
	}
	removedWork := map[string]bool{}
	removedConversations := map[string]bool{}
	if p.HistoryDays > 0 {
		cut := now.Add(-time.Duration(p.HistoryDays) * 24 * time.Hour)
		sessions, err := worksession.Open(s.stateDir)
		if err != nil {
			return plan, err
		}
		active, err := sessions.RetainedActive()
		if err != nil {
			return plan, err
		}
		if active != "" {
			protectedConversation[active] = true
		}
		conversations, err := sessions.List(0)
		if err != nil {
			return plan, err
		}
		for _, conversation := range conversations {
			if conversation.UpdatedAt.Before(cut) && !protectedConversation[conversation.ID] {
				removedConversations[conversation.ID] = true
				selected = append(selected, filepath.Join("conversations", conversation.ID))
			}
		}
		for _, run := range history {
			if run.Status == workhistory.Running || protectedWork[run.ID] || protectedConversation[run.ConversationID] {
				continue
			}
			if run.ConversationID != "" && !removedConversations[run.ConversationID] {
				continue
			}
			if run.FinishedAt.IsZero() || !run.FinishedAt.Before(cut) {
				continue
			}
			removedWork[run.ID] = true
			selected = append(selected, filepath.Join("work-history", run.ID+".json"), filepath.Join("work-history", run.ID+".lock"))
			for _, base := range []string{"traces", "interactions", "tasks", "candidates", "delivery", "external-actions", "reconciliations", "work-control"} {
				selected = append(selected, filepath.Join(base, run.ID))
			}
			if run.Evidence.ArtifactManifestPath != "" {
				rel, err := filepath.Rel(s.root, filepath.Dir(run.Evidence.ArtifactManifestPath))
				if err != nil || !strings.HasPrefix(rel, "workspaces"+string(filepath.Separator)) {
					return plan, errors.New("retained Work points outside private workspaces; review before cleanup")
				}
				selected = append(selected, rel)
			}
		}
		for _, base := range []string{"suggestions", "inbox", "pilot", "observations"} {
			objects, err := s.objects(base)
			if err != nil {
				return plan, err
			}
			for path, o := range objects {
				created := timeField(o, "created_at")
				if created.IsZero() {
					created = timeField(o, "UpdatedAt")
				}
				if created.IsZero() {
					created = timeField(o, "updated_at")
				}
				work := stringField(o, "work_id")
				conversation := stringField(o, "conversation_id")
				if stringField(o, "status") == "pending" {
					continue
				}
				if removedWork[work] || removedConversations[conversation] || (!created.IsZero() && created.Before(cut) && work == "" && conversation == "") {
					selected = append(selected, path)
				}
			}
		}
	}
	if p.KnowledgeDays > 0 {
		cut := now.Add(-time.Duration(p.KnowledgeDays) * 24 * time.Hour)
		changed := false
		for _, base := range []string{"memory", "knowledge/notes"} {
			objects, err := s.objects(base)
			if err != nil {
				return plan, err
			}
			for path, o := range objects {
				updated := timeField(o, "updated_at")
				if !updated.IsZero() && updated.Before(cut) && !protectedGoal[stringField(o, "goal_id")] {
					if base == "knowledge/notes" {
						selected = append(selected, filepath.Dir(path))
					} else {
						selected = append(selected, path)
					}
					changed = true
				}
			}
		}
		if changed {
			selected = append(selected, "retrieval")
		}
	}
	items, err := s.walk(ctx, selected, true)
	if err != nil {
		return plan, err
	}
	if p.SnapshotDays > 0 {
		removed := map[string]bool{}
		for _, item := range items {
			removed[filepath.ToSlash(item.Path)] = true
		}
		refs, err := s.snapshotReferences(removed)
		if err != nil {
			return plan, err
		}
		manifests, err := snapshot.List(s.stateDir)
		if err != nil {
			return plan, err
		}
		cut := now.Add(-time.Duration(p.SnapshotDays) * 24 * time.Hour)
		keepTrees := map[string]bool{}
		keepBlobs := map[string]bool{}
		var dropped []snapshot.Manifest
		for _, manifest := range manifests {
			if refs[manifest.ID] || !manifest.CreatedAt.Before(cut) {
				keepTrees[manifest.TreeID] = true
				for _, entry := range manifest.Entries {
					keepBlobs[entry.SHA256] = true
				}
			} else {
				dropped = append(dropped, manifest)
			}
		}
		for _, manifest := range dropped {
			selected = append(selected, filepath.Join("snapshots/manifests", manifest.ID+".json"))
			if !keepTrees[manifest.TreeID] {
				selected = append(selected, filepath.Join("snapshots/trees", manifest.TreeID))
			}
			for _, entry := range manifest.Entries {
				if !keepBlobs[entry.SHA256] {
					selected = append(selected, filepath.Join("snapshots/blobs", entry.SHA256))
				}
			}
		}
		items, err = s.walk(ctx, selected, true)
		if err != nil {
			return plan, err
		}
	}
	plan.Items = items
	return plan, nil
}
func (s Store) PreviewRetention(ctx context.Context, now time.Time) (Plan, error) {
	release, err := state.Guard(s.stateDir, true)
	if err != nil {
		return Plan{}, err
	}
	defer release()
	p, err := s.Policy()
	if err != nil {
		return Plan{}, err
	}
	plan, err := s.selectRetention(ctx, now, p)
	if err != nil {
		return plan, err
	}
	return s.savePlan(plan)
}
