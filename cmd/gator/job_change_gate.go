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

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
)

// localJobInputHash is intentionally limited to selected local material. Jobs
// with web or connector reads cannot use the change gate because those inputs
// may change without a local file changing.
func localJobInputHash(definition jobs.Definition, stateDir string) (string, error) {
	if !definition.ChangeOnly || !definition.RefreshSnapshot || len(definition.ConnectorIDs) > 0 || len(definition.WebOrigins) > 0 {
		return "", errors.New("change gate requires a local-only refreshed job")
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
		Objective string
		Goal      goal.Record
	}{definition.Objective, currentGoal}); err != nil {
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
