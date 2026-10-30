package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/suggestion"
)

const suggestionUsage = `usage:
  gator suggestion list [--all]
  gator suggestion show ID
  gator suggestion accept ID
  gator suggestion reject ID`

func suggestionCommand(args []string, out io.Writer) error {
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := suggestion.Open(stateDir)
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "list" {
		status := "pending"
		if len(args) == 2 && args[1] == "--all" {
			status = ""
		} else if len(args) > 1 {
			return errors.New(suggestionUsage)
		}
		records, err := store.List(status)
		if err != nil {
			return err
		}
		for _, record := range records {
			if _, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", record.ID, record.Kind, record.Status, suggestionLabel(record)); err != nil {
				return err
			}
		}
		return nil
	}
	if len(args) != 2 {
		return errors.New(suggestionUsage)
	}
	switch args[0] {
	case "show":
		record, err := store.Load(args[1])
		if err != nil {
			return err
		}
		var currentNote any
		if record.NoteID != "" {
			notes, err := knowledge.Open(stateDir)
			if err != nil {
				return err
			}
			note, err := notes.LoadNote(record.NoteID)
			if err != nil {
				return err
			}
			currentNote = map[string]any{"id": note.ID, "title": note.Title, "body": note.Body, "source_refs": note.SourceRefs, "revision": note.Revision(), "matches_proposal": note.Revision() == record.ExpectedNoteRevision}
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(struct {
			suggestion.Record
			ReferenceIntegrity suggestionEvidenceCheck `json:"reference_integrity"`
			CurrentNote        any                     `json:"current_note,omitempty"`
		}{Record: record, CurrentNote: currentNote, ReferenceIntegrity: checkSuggestionEvidence(stateDir, record)})
	case "accept", "reject":
		record, err := store.Resolve(args[1], args[0], func(record suggestion.Record) (string, error) {
			return applySuggestion(stateDir, record)
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s %s. Result: %s\n", strings.Title(record.Status), record.ID, record.ResultID)
		if err == nil && record.Kind == "goal" && record.Status == "accepted" {
			_, err = fmt.Fprintf(out, "Continue in terminal: gator goal chat %s\n", record.ResultID)
		}
		return err
	default:
		return errors.New(suggestionUsage)
	}
}

func suggestionLabel(record suggestion.Record) string {
	if record.NoteID != "" {
		return "Update note: " + record.Title
	}
	if record.Title != "" {
		return record.Title
	}
	if record.SkillID != "" {
		return record.SkillID
	}
	if record.Kind == "schedule" {
		return record.Cron + "  " + record.Task
	}
	return record.PlanChange.Kind
}

func applySuggestion(stateDir string, record suggestion.Record) (string, error) {
	switch record.Kind {
	case "goal":
		store, err := goal.Open(stateDir)
		if err != nil {
			return "", err
		}
		id := "goal-" + strings.TrimPrefix(record.ID, "suggestion-")
		created, err := store.AddWithID(id, record.Title, record.Body, record.GoalSourcePath)
		if err != nil {
			return "", err
		}
		if _, err := store.InitializePlan(created.ID, record.GoalOutcome, record.GoalSteps); err != nil {
			return "", err
		}
		if record.Cron != "" {
			scheduled := record
			scheduled.Kind, scheduled.GoalID = "schedule", created.ID
			if _, err := applySuggestion(stateDir, scheduled); err != nil {
				return "", err
			}
		}
		return created.ID, nil
	case "note":
		store, err := knowledge.Open(stateDir)
		if err != nil {
			return "", err
		}
		id := "note-" + strings.TrimPrefix(record.ID, "suggestion-")
		refs := append(append([]string(nil), record.SourceRefs...), "work:"+record.WorkID)
		if record.NoteID != "" {
			updated, err := store.ApplyNoteUpdate(record.NoteID, record.GoalID, record.ExpectedNoteRevision, record.ID, record.Title, record.Body, refs)
			return updated.ID, err
		}
		created, err := store.AddNoteWithIDAndRefs(id, record.Title, record.Body, record.GoalID, refs)
		if os.IsExist(err) {
			existing, loadErr := store.LoadNote(id)
			if loadErr == nil && existing.Title == record.Title && existing.Body == record.Body && existing.GoalID == record.GoalID && strings.Join(existing.SourceRefs, "\x00") == strings.Join(refs, "\x00") {
				return id, nil
			}
		}
		return created.ID, err
	case "skill":
		store, err := knowledge.Open(stateDir)
		if err != nil {
			return "", err
		}
		created, err := store.AddSkill(record.SkillID, record.Description, record.Body)
		if os.IsExist(err) {
			existing, loadErr := store.LoadSkill(record.SkillID)
			if loadErr == nil && existing.Description == record.Description && existing.Body == record.Body {
				return record.SkillID, nil
			}
		}
		return created.ID, err
	case "plan":
		store, err := goal.Open(stateDir)
		if err != nil {
			return "", err
		}
		plan, err := store.ApplyReviewedPlanChange(record.GoalID, record.PlanChange, record.ID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s@%d", record.GoalID, plan.Revision), nil
	case "schedule":
		goals, err := goal.Open(stateDir)
		if err != nil {
			return "", err
		}
		current, err := goals.Load(record.GoalID)
		if err != nil {
			return "", err
		}
		if current.Status != goal.Active {
			return "", errors.New("resume the goal before accepting its schedule")
		}
		store, err := jobs.Open(stateDir)
		if err != nil {
			return "", err
		}
		id := "job-" + record.ID
		if existing, err := store.Load(id); err == nil {
			if existing.GoalID == current.ID && existing.Schedule == record.Cron && existing.Timezone == record.Timezone && existing.Objective == record.Task && existing.ChangeOnly == record.ChangeOnly && existing.Mode == action.Inspect {
				return id, nil
			}
			return "", errors.New("suggested schedule ID already exists with different settings")
		} else if !os.IsNotExist(err) {
			return "", err
		}
		definition, err := store.Save(jobs.Definition{ID: id, Name: current.Title, GoalID: current.ID, Enabled: true, Schedule: record.Cron, Timezone: record.Timezone, Missed: "run_once", SourcePath: current.SourcePath, RefreshSnapshot: true, ChangeOnly: record.ChangeOnly, Objective: record.Task, Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 24, CreatedAt: record.CreatedAt})
		return definition.ID, err
	default:
		return "", errors.New("unsupported suggestion kind")
	}
}
