package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/workrun"
)

func hasSourceEdits(c WorkCase) bool {
	for _, followup := range c.Followups {
		if len(followup.SourceEdits) > 0 {
			return true
		}
	}
	return false
}

func copyWorkFixture(source, destination string) error {
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	count, total := 0, int64(0)
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.Mkdir(target, 0700)
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("longitudinal fixture contains a non-regular file")
		}
		count++
		total += info.Size()
		if count > 1000 || total > 16*1024*1024 {
			return errors.New("longitudinal fixture exceeds copy limits")
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, err = io.Copy(output, input)
		inputErr := input.Close()
		closeErr := output.Close()
		if err != nil {
			return err
		}
		if inputErr != nil {
			return inputErr
		}
		return closeErr
	})
}

func seedWorkGoal(state, source string, seed WorkGoalSeed) (string, error) {
	store, err := goal.Open(state)
	if err != nil {
		return "", err
	}
	record, err := store.Add(seed.Title, seed.Context, source)
	if err != nil {
		return "", err
	}
	plan, err := store.LoadPlan(record.ID)
	if err != nil {
		return "", err
	}
	if seed.Outcome != "" {
		plan, err = store.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "set_outcome", ExpectedRevision: plan.Revision, Outcome: seed.Outcome})
		if err != nil {
			return "", err
		}
	}
	for _, title := range seed.Steps {
		plan, err = store.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "add_step", ExpectedRevision: plan.Revision, Title: title})
		if err != nil {
			return "", err
		}
	}
	memories, err := memory.Open(state)
	if err != nil {
		return "", err
	}
	for _, fact := range seed.Memory {
		if _, err := memories.Add("fact", fact, record.ID, "benchmark user seed", time.Time{}); err != nil {
			return "", err
		}
	}
	return record.ID, nil
}

func applyEvalGoalContext(state, goalID string, request *workrun.Request) error {
	store, err := goal.Open(state)
	if err != nil {
		return err
	}
	record, err := store.Load(goalID)
	if err != nil {
		return err
	}
	plan, err := store.LoadPlan(goalID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	memories, err := memory.Open(state)
	if err != nil {
		return err
	}
	items, err := memories.List(goalID, false)
	if err != nil {
		return err
	}
	request.GoalID = goalID
	request.GoalContext = fmt.Sprintf("Goal: %s\nContext: %s", record.Title, record.Context)
	request.GoalPlanContext = string(data)
	request.MemoryContext = memory.Render(items)
	return nil
}

func applyWorkFollowupChanges(state, source, goalID string, followup WorkFollowup) error {
	for _, edit := range followup.SourceEdits {
		path := filepath.Join(source, edit.Path)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("followup edit %s requires an existing regular fixture file", edit.Path)
		}
		if err := os.WriteFile(path, []byte(edit.Content), 0600); err != nil {
			return err
		}
	}
	if goalID == "" && (followup.GoalContext != "" || len(followup.PlanChanges) > 0) {
		return errors.New("followup goal changes require a seeded goal")
	}
	if goalID != "" {
		store, err := goal.Open(state)
		if err != nil {
			return err
		}
		if strings.TrimSpace(followup.GoalContext) != "" {
			if _, err := store.SetContext(goalID, followup.GoalContext); err != nil {
				return err
			}
		}
		for _, change := range followup.PlanChanges {
			if _, err := store.ApplyPlanChange(goalID, change); err != nil {
				return err
			}
		}
	}
	return nil
}
