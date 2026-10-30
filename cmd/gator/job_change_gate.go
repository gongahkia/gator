package main

import (
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

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
)

// localJobInputHash covers selected local material. A connected goal watch may
// compose this with a separately fetched remote digest; ordinary connector
// jobs cannot use this local-only result as their complete input gate.
func localJobInputHash(definition jobs.Definition, stateDir string) (string, error) {
	return localJobInputHashAt(definition, stateDir, time.Now())
}

func localJobInputHashAt(definition jobs.Definition, stateDir string, now time.Time) (string, error) {
	if !definition.ChangeOnly || !definition.RefreshSnapshot || len(definition.WebOrigins) > 0 || len(definition.ConnectorIDs) > 0 && len(definition.WatchSources) == 0 {
		return "", errors.New("change gate requires refreshed local input and bounded watch sources")
	}
	source := definition.SourcePath
	var currentGoal goal.Record
	if definition.GoalID != "" {
		goals, err := goal.Open(stateDir)
		if err != nil {
			return "", err
		}
		currentGoal, err = goals.Load(definition.GoalID)
		if err != nil {
			return "", err
		}
		if currentGoal.Status != goal.Active {
			return "", errors.New("goal is not active")
		}
		source = currentGoal.SourcePath
	}
	hash := sha256.New()
	write := func(value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = hash.Write(append(data, '\n'))
		return err
	}
	if err := write(struct {
		Objective   string
		GoalTitle   string
		GoalContext string
		GoalSource  string
		GoalStatus  goal.Status
	}{definition.Objective, currentGoal.Title, currentGoal.Context, currentGoal.SourcePath, currentGoal.Status}); err != nil {
		return "", err
	}
	if definition.GoalID != "" {
		goals, err := goal.Open(stateDir)
		if err != nil {
			return "", err
		}
		plan, err := goals.LoadPlan(definition.GoalID)
		if err != nil {
			return "", err
		}
		if err := write(plan); err != nil {
			return "", err
		}
		dependencies, err := goals.DependencyStates(plan)
		if err != nil {
			return "", err
		}
		if err := write(dependencies); err != nil {
			return "", err
		}
		if definition.GoalWatch {
			// Crossing a due time is a new input even when every file stays
			// identical. The boolean stays stable after the crossing, so it
			// triggers one review rather than a model call every minute.
			due := struct {
				Review  bool
				Steps   []string
				Started []string
				Ended   []string
			}{Review: !plan.NextReviewAt.IsZero() && !plan.NextReviewAt.After(now) && plan.LastReviewAt.Before(plan.NextReviewAt)}
			for _, step := range plan.Steps {
				if step.Status == goal.StepDone {
					continue
				}
				if !step.StartsAt.IsZero() && !step.StartsAt.After(now) {
					due.Started = append(due.Started, step.ID)
				}
				if !step.EndsAt.IsZero() && !step.EndsAt.After(now) {
					due.Ended = append(due.Ended, step.ID)
				}

				if step.Status != goal.StepDone && !step.DueAt.IsZero() && !step.DueAt.After(now) {
					due.Steps = append(due.Steps, step.ID)
				}
			}
			if err := write(due); err != nil {
				return "", err
			}
		}
	}
	notes, err := knowledge.Open(stateDir)
	if err != nil {
		return "", err
	}
	noteList, err := notes.ListNotes(definition.GoalID)
	if err != nil {
		return "", err
	}
	for _, item := range noteList {
		item, err = notes.LoadNote(item.ID)
		if err != nil {
			return "", err
		}
		if err := write(item); err != nil {
			return "", err
		}
		if _, err := io.WriteString(hash, item.Body+"\n"); err != nil {
			return "", err
		}
	}
	skills, err := notes.ListSkills(true)
	if err != nil {
		return "", err
	}
	for _, item := range skills {
		item, err = notes.LoadSkill(item.ID)
		if err != nil {
			return "", err
		}
		if err := write(item); err != nil {
			return "", err
		}
		if _, err := io.WriteString(hash, item.Body+"\n"); err != nil {
			return "", err
		}
	}
	memories, err := memory.Open(stateDir)
	if err != nil {
		return "", err
	}
	items, err := memories.List(definition.GoalID, false)
	if err != nil {
		return "", err
	}
	if err := write(items); err != nil {
		return "", err
	}
	count, total := 0, int64(0)
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != source && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("change-only source contains non-regular entry %s", path)
		}
		count++
		total += info.Size()
		if count > 10000 || info.Size() > 16*1024*1024 || total > 64*1024*1024 {
			return errors.New("change-only source exceeds 10,000 files, 16 MiB per file, or 64 MiB total")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(hash, filepath.ToSlash(relative)+"\x00"); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		opened, err := file.Stat()
		if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
			_ = file.Close()
			return errors.New("change-only source changed during read")
		}
		_, err = io.Copy(hash, file)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if err != nil {
		return "", err
	}
	return strings.ToLower(hex.EncodeToString(hash.Sum(nil))), nil
}
