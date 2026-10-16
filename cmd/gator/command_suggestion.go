package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/goal"
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
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(record)
	case "accept", "reject":
		record, err := store.Resolve(args[1], args[0], func(record suggestion.Record) (string, error) {
			return applySuggestion(stateDir, record)
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s %s. Result: %s\n", strings.Title(record.Status), record.ID, record.ResultID)
		return err
	default:
		return errors.New(suggestionUsage)
	}
}

func suggestionLabel(record suggestion.Record) string {
	if record.Title != "" {
		return record.Title
	}
	if record.SkillID != "" {
		return record.SkillID
	}
	return record.PlanChange.Kind
}

func applySuggestion(stateDir string, record suggestion.Record) (string, error) {
	switch record.Kind {
	case "note":
		store, err := knowledge.Open(stateDir)
		if err != nil {
			return "", err
		}
		id := "note-" + strings.TrimPrefix(record.ID, "suggestion-")
		refs := append(append([]string(nil), record.SourceRefs...), "work:"+record.WorkID)
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
		plan, err := store.ApplyPlanChange(record.GoalID, record.PlanChange)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s@%d", record.GoalID, plan.Revision), nil
	default:
		return "", errors.New("unsupported suggestion kind")
	}
}
