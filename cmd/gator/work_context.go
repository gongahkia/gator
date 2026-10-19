package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worktui"
)

func readWorkContext(stateDir, workID, goalID string) (worktui.ContextState, error) {
	result := worktui.ContextState{WorkID: workID, GoalID: goalID}
	memories, err := memory.Open(stateDir)
	if err != nil {
		return result, err
	}
	notes, err := knowledge.Open(stateDir)
	if err != nil {
		return result, err
	}
	load := func(id string) worktui.KnowledgeItem {
		item := worktui.KnowledgeItem{ID: id, Title: id, Text: "This item is no longer saved.", Scope: "unknown"}
		if strings.HasPrefix(id, "mem-") {
			if r, err := memories.Load(id); err == nil && (r.GoalID == "" || r.GoalID == result.GoalID) {
				item.Kind, item.Title, item.Text, item.CurrentText, item.Scope, item.Source, item.Available = "memory", r.Kind, r.Text, r.Text, contextScope(r.GoalID), r.Source, true
				item.Updated = r.UpdatedAt.Local().Format("Jan 2 15:04")
			}
		} else if strings.HasPrefix(id, "note-") {
			if r, err := notes.LoadNote(id); err == nil && (r.GoalID == "" || r.GoalID == result.GoalID) {
				item.Kind, item.Title, item.Text, item.CurrentText, item.Scope, item.Source, item.Available = "note", r.Title, r.Body, r.Body, contextScope(r.GoalID), strings.Join(r.SourceRefs, ", "), true
				item.Updated = r.UpdatedAt.Local().Format("Jan 2 15:04")
			}
		}
		return item
	}
	if workID == "" {
		result.Outcome = "Saved knowledge available to this goal"
		records, err := memories.List(goalID, false)
		if err != nil {
			return result, err
		}
		for _, r := range records {
			if len(result.Items) == 100 {
				break
			}
			result.Items = append(result.Items, load(r.ID))
		}
		documents, err := notes.ListNotes(goalID)
		if err != nil {
			return result, err
		}
		for _, r := range documents {
			if len(result.Items) == 100 {
				break
			}
			result.Items = append(result.Items, load(r.ID))
		}
		return result, nil
	}
	history, err := workhistory.Open(stateDir)
	if err != nil {
		return result, err
	}
	record, err := history.Load(workID)
	if err != nil {
		return result, err
	}
	result.Outcome = record.ObjectiveSummary
	if decision := record.Decision; decision != nil {
		result.GoalID, result.Permission, result.Recall = decision.GoalID, string(decision.Mode), decision.RecallStatus
		if len(decision.ContextItems) > 0 {
			for _, snapshot := range decision.ContextItems {
				item := load(snapshot.ID)
				item.Kind, item.Title, item.Text, item.Scope, item.Source = snapshot.Kind, snapshot.Title, snapshot.Excerpt, snapshot.Scope, snapshot.Source
				item.Updated = "Supplied at run start · saved revision " + snapshot.UpdatedAt.Local().Format("Jan 2 15:04")
				result.Items = append(result.Items, item)
			}
		} else {
			for _, id := range decision.ContextRefs {
				item := load(id)
				item.Updated = "Current saved text; this older run retained identifiers only"
				result.Items = append(result.Items, item)
			}
		}
	}
	if record.Evidence.ArtifactManifestPath != "" {
		bundle, err := artifact.OpenBundle(filepath.Dir(record.Evidence.ArtifactManifestPath))
		if err != nil {
			return result, err
		}
		result.Usage = fmt.Sprintf("%d model requests · %d input tokens · %d output tokens; provider reporting may be incomplete", bundle.Manifest.Usage.ModelRequests, bundle.Manifest.Usage.InputTokens, bundle.Manifest.Usage.OutputTokens)
		for _, evidence := range bundle.Manifest.Evidence {
			if len(result.Sources) == 16 {
				break
			}
			result.Sources = append(result.Sources, evidence.Locator)
		}
	}
	return result, nil
}

func changeWorkKnowledge(stateDir, id, action, text, goalID string) error {
	if strings.HasPrefix(id, "mem-") {
		store, err := memory.Open(stateDir)
		if err != nil {
			return err
		}
		r, err := store.Load(id)
		if err != nil {
			return err
		}
		if r.GoalID != "" && r.GoalID != goalID {
			return errors.New("saved memory is outside this goal")
		}
		if action == "forget" {
			return store.Forget(id)
		}
		if action == "correct" {
			_, err = store.Correct(id, text, r.Source)
			return err
		}
	} else if strings.HasPrefix(id, "note-") {
		store, err := knowledge.Open(stateDir)
		if err != nil {
			return err
		}
		r, err := store.LoadNote(id)
		if err != nil {
			return err
		}
		if r.GoalID != "" && r.GoalID != goalID {
			return errors.New("saved note is outside this goal")
		}
		if action == "forget" {
			return store.RemoveNote(id)
		}
		if action == "correct" {
			_, err = store.UpdateNote(id, r.Title, text)
			return err
		}
	}
	return errors.New("unsupported saved knowledge action")
}
