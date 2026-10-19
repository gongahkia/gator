// Package pilot records explicit human judgments about real project follow-up
// work. Automated checks and unreviewed runs never become user-success scores.
package pilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workhistory"
)

type Review struct {
	Version     int       `json:"version"`
	WorkID      string    `json:"work_id"`
	GoalID      string    `json:"goal_id,omitempty"`
	Reviewer    string    `json:"reviewer"`
	Draft       string    `json:"draft"`
	Corrections int       `json:"corrections"`
	Followup    string    `json:"followup"`
	Recovery    string    `json:"recovery"`
	Notes       string    `json:"notes,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Store struct{ stateDir, root string }
type Report struct {
	Reviews                                                                  []Review `json:"reviews"`
	Usable, LightEdits, Rewrite, Failed                                      int
	Corrections                                                              int
	UsefulFollowups, UnhelpfulFollowups, RecoverySuccesses, RecoveryFailures int
	UniqueRuns, ModelRequests                                                int
	Tokens                                                                   int64
	ElapsedSeconds                                                           float64
	KnownCostUSD                                                             *float64
	UsageIncomplete                                                          bool
}

func Open(stateDir string) (Store, error) {
	root := filepath.Join(stateDir, "gator", "pilot")
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("pilot state directory required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return Store{}, err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return Store{}, err
	}
	return Store{stateDir, root}, nil
}
func validChoice(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
func validate(r Review) error {
	if r.WorkID == "" || len(r.WorkID) > 128 || filepath.Base(r.WorkID) != r.WorkID || r.Reviewer == "" || len(r.Reviewer) > 100 || strings.ContainsAny(r.WorkID+r.Reviewer, "\x00\r\n") || len(r.Notes) > 4096 || r.Corrections < 0 || r.Corrections > 1000 {
		return errors.New("invalid pilot review identity or correction count")
	}
	if !validChoice(r.Draft, "usable", "light-edits", "rewrite", "failed") || !validChoice(r.Followup, "unreviewed", "useful", "unhelpful") || !validChoice(r.Recovery, "unreviewed", "succeeded", "failed") {
		return errors.New("review draft usability, follow-up usefulness, and recovery using the supported choices")
	}
	return nil
}
func (s Store) Save(r Review) (Review, error) {
	if r.Followup == "" {
		r.Followup = "unreviewed"
	}
	if r.Recovery == "" {
		r.Recovery = "unreviewed"
	}
	if err := validate(r); err != nil {
		return Review{}, err
	}
	history, err := workhistory.Open(s.stateDir)
	if err != nil {
		return Review{}, err
	}
	work, err := history.Load(r.WorkID)
	if err != nil {
		return Review{}, err
	}
	if work.Status == workhistory.Running {
		return Review{}, errors.New("finish or stop this Work run before reviewing it")
	}
	if work.Decision != nil {
		r.GoalID = work.Decision.GoalID
	}
	r.Version, r.UpdatedAt = 1, time.Now().UTC()
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return Review{}, err
	}
	digest := sha256.Sum256([]byte(r.WorkID + "\x00" + r.Reviewer))
	file, err := os.CreateTemp(s.root, ".review-*")
	if err != nil {
		return Review{}, err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return Review{}, err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return Review{}, err
	}
	if err := file.Close(); err != nil {
		return Review{}, err
	}
	return r, os.Rename(file.Name(), filepath.Join(s.root, hex.EncodeToString(digest[:])+".json"))
}
func (s Store) Report(goalID string) (Report, error) {
	result := Report{}
	files, err := os.ReadDir(s.root)
	if err != nil {
		return result, err
	}
	history, err := workhistory.Open(s.stateDir)
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	knownCost := 0.0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.root, file.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return result, err
		}
		if !info.Mode().IsRegular() || info.Size() > 16*1024 {
			return result, errors.New("pilot review is not a bounded regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return result, err
		}
		var review Review
		if err := json.Unmarshal(data, &review); err != nil {
			return result, err
		}
		if err := validate(review); err != nil {
			return result, err
		}
		if goalID != "" && review.GoalID != goalID {
			continue
		}
		result.Reviews = append(result.Reviews, review)
		switch review.Draft {
		case "usable":
			result.Usable++
		case "light-edits":
			result.LightEdits++
		case "rewrite":
			result.Rewrite++
		case "failed":
			result.Failed++
		}
		result.Corrections += review.Corrections
		if review.Followup == "useful" {
			result.UsefulFollowups++
		}
		if review.Followup == "unhelpful" {
			result.UnhelpfulFollowups++
		}
		if review.Recovery == "succeeded" {
			result.RecoverySuccesses++
		}
		if review.Recovery == "failed" {
			result.RecoveryFailures++
		}
		if seen[review.WorkID] {
			continue
		}
		seen[review.WorkID] = true
		result.UniqueRuns++
		work, err := history.Load(review.WorkID)
		if err != nil {
			return result, err
		}
		result.ElapsedSeconds += work.FinishedAt.Sub(work.StartedAt).Seconds()
		if work.Evidence.ArtifactManifestPath == "" {
			result.UsageIncomplete = true
			continue
		}
		bundle, err := artifact.OpenBundle(filepath.Dir(work.Evidence.ArtifactManifestPath))
		if err != nil {
			result.UsageIncomplete = true
			continue
		}
		usage := bundle.Manifest.Usage
		result.ModelRequests += usage.ModelRequests
		result.Tokens += usage.InputTokens + usage.OutputTokens
		if !usage.Reported || usage.UnknownRequests > 0 {
			result.UsageIncomplete = true
		}
		if usage.KnownCost == nil {
			result.UsageIncomplete = true
		} else {
			knownCost += *usage.KnownCost
		}
	}
	sort.Slice(result.Reviews, func(i, j int) bool { return result.Reviews[i].UpdatedAt.After(result.Reviews[j].UpdatedAt) })
	if !result.UsageIncomplete && result.UniqueRuns > 0 {
		result.KnownCostUSD = &knownCost
	}
	return result, nil
}
