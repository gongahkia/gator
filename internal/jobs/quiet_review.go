package jobs

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/workspace"
)

// QuietReview is a user's explicit assessment of one completed watch check
// that chose not to surface an alert. It lives beside the immutable attempt.
type QuietReview struct {
	Version   int       `json:"version"`
	Sequence  uint64    `json:"sequence,omitempty"`
	JobID     string    `json:"job_id"`
	AttemptID string    `json:"attempt_id"`
	Verdict   string    `json:"verdict"`
	Note      string    `json:"note,omitempty"`
	RatedAt   time.Time `json:"rated_at"`
}

type QuietAuditEntry struct {
	Attempt Attempt     `json:"attempt"`
	Review  QuietReview `json:"review"`
}

type QuietAudit struct {
	Total      int               `json:"total"`
	Missed     int               `json:"missed"`
	Correct    int               `json:"correct"`
	Unreviewed int               `json:"unreviewed"`
	Entries    []QuietAuditEntry `json:"entries"`
}

// ReviewQuietWatch labels a completed, model-evaluated quiet watch. Unchanged
// input skips are excluded: no model decision was made on those attempts.
func (s Store) ReviewQuietWatch(jobID, attemptID, verdict, note string, now time.Time) (QuietReview, error) {
	return s.reviewQuietWatch(jobID, attemptID, verdict, note, "", now)
}

// Revision binds feedback changes to the review displayed, including unrated
// checks. A newer person's assessment must not be silently overwritten.
func (r QuietReview) Revision() string {
	data, _ := json.Marshal(r)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (s Store) ReviewQuietWatchReviewed(jobID, attemptID, verdict, note, expected string, now time.Time) (QuietReview, error) {
	if expected == "" {
		return QuietReview{}, errors.New("review the current feedback before changing it")
	}
	return s.reviewQuietWatch(jobID, attemptID, verdict, note, expected, now)
}

func (s Store) reviewQuietWatch(jobID, attemptID, verdict, note, expected string, now time.Time) (QuietReview, error) {
	unlock, err := s.lockDefinitions()
	if err != nil {
		return QuietReview{}, err
	}
	defer unlock()
	if verdict != "missed" && verdict != "correct" && verdict != "clear" {
		return QuietReview{}, errors.New("quiet watch verdict must be missed, correct, or clear")
	}
	if now.IsZero() || len(note) > 512 || strings.ContainsRune(note, 0) ||
		(verdict == "missed" && strings.TrimSpace(note) == "") ||
		(verdict == "clear" && note != "") {
		return QuietReview{}, errors.New("quiet watch review needs a time and a bounded note for missed changes")
	}
	if _, err := s.quietWatchAttempt(jobID, attemptID); err != nil {
		return QuietReview{}, err
	}
	prior, err := s.loadQuietReview(jobID, attemptID)
	if err != nil {
		return QuietReview{}, err
	}
	if expected != "" && prior.Revision() != expected {
		return QuietReview{}, errors.New("feedback changed; refresh this check before reviewing it again")
	}
	if prior.Sequence == ^uint64(0) {
		return QuietReview{}, errors.New("quiet watch review revision is exhausted")
	}
	// A clear is a retained tombstone. Deleting the review would make an old
	// unrated revision valid again and let stale feedback overwrite this choice.
	if verdict == "clear" {
		verdict = ""
	}
	review := QuietReview{Version: 2, Sequence: prior.Sequence + 1, JobID: jobID, AttemptID: attemptID, Verdict: verdict, Note: note, RatedAt: now.UTC()}
	if err := writeJSON(s.quietReviewPath(jobID, attemptID), review); err != nil {
		return QuietReview{}, err
	}
	return review, nil
}

// QuietWatchAudit includes every retained completed quiet watch decision in
// its counts, even when entries are limited for display. Unreviewed is not a
// claim that an important change was missed.
func (s Store) QuietWatchAudit(jobID string, limit int) (QuietAudit, error) {
	if invalidQuietJobID(jobID) {
		return QuietAudit{}, errors.New("invalid job ID")
	}
	history, err := s.History(jobID, 0)
	if err != nil {
		return QuietAudit{}, err
	}
	hasWatch := false
	if current, err := s.Load(jobID); err == nil && current.GoalWatch {
		hasWatch = true
	}
	var audit QuietAudit
	for _, attempt := range history {
		if attempt.Status != "completed" {
			continue
		}
		if attempt.JobID != jobID {
			return QuietAudit{}, errors.New("job attempt identity does not match")
		}
		watch, err := s.isWatchAttempt(jobID, attempt.ID)
		if err != nil {
			return QuietAudit{}, err
		}
		if !watch {
			continue
		}
		hasWatch = true
		if attempt.WatchAttention {
			continue
		}
		review, err := s.loadQuietReview(jobID, attempt.ID)
		if err != nil {
			return QuietAudit{}, err
		}
		audit.Total++
		switch review.Verdict {
		case "missed":
			audit.Missed++
		case "correct":
			audit.Correct++
		default:
			audit.Unreviewed++
		}
		if limit <= 0 || len(audit.Entries) < limit {
			audit.Entries = append(audit.Entries, QuietAuditEntry{Attempt: attempt, Review: review})
		}
	}
	if !hasWatch {
		return QuietAudit{}, errors.New("job has no goal watch history")
	}
	return audit, nil
}

func (s Store) quietWatchAttempt(jobID, attemptID string) (Attempt, error) {
	if invalidQuietJobID(jobID) || invalidQuietAttemptID(attemptID) {
		return Attempt{}, errors.New("invalid job or attempt ID")
	}
	var attempt Attempt
	if err := s.readQuietState(filepath.Join("history", jobID, attemptID, "result.json"), &attempt); err != nil {
		return Attempt{}, err
	}
	if attempt.ID != attemptID || attempt.JobID != jobID || attempt.Status != "completed" || attempt.WatchAttention {
		return Attempt{}, errors.New("attempt is not a completed quiet watch decision")
	}
	watch, err := s.isWatchAttempt(jobID, attemptID)
	if err != nil {
		return Attempt{}, err
	}
	if !watch {
		return Attempt{}, errors.New("attempt is not a goal watch")
	}
	return attempt, nil
}

func (s Store) isWatchAttempt(jobID, attemptID string) (bool, error) {
	if invalidQuietJobID(jobID) || invalidQuietAttemptID(attemptID) {
		return false, errors.New("invalid job or attempt ID")
	}
	var original intent
	if err := s.readQuietState(filepath.Join("history", jobID, attemptID, "intent.json"), &original); err != nil {
		return false, err
	}
	if original.ExecutionID != attemptID || original.Definition.ID != jobID {
		return false, errors.New("job attempt intent identity does not match")
	}
	return original.Definition.GoalWatch, nil
}

func (s Store) loadQuietReview(jobID, attemptID string) (QuietReview, error) {
	var review QuietReview
	err := s.readQuietState(filepath.Join("history", jobID, attemptID, "quiet-review.json"), &review)
	if errors.Is(err, os.ErrNotExist) {
		return QuietReview{}, nil
	}
	if err != nil {
		return QuietReview{}, err
	}
	validVersion := review.Version == 1 && review.Sequence == 0 || review.Version == 2 && review.Sequence > 0
	validVerdict := review.Verdict == "missed" || review.Verdict == "correct" || review.Version == 2 && review.Verdict == "" && review.Note == ""
	if !validVersion || !validVerdict || review.JobID != jobID || review.AttemptID != attemptID || review.RatedAt.IsZero() ||
		len(review.Note) > 512 || strings.ContainsRune(review.Note, 0) ||
		(review.Verdict == "missed" && strings.TrimSpace(review.Note) == "") {
		return QuietReview{}, errors.New("quiet watch review is invalid")
	}
	return review, nil
}

func (s Store) readQuietState(path string, target any) error {
	root, err := workspace.Open(s.root)
	if err != nil {
		return err
	}
	data, err := root.ReadRegularFile(path, 1024*1024)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode job review state: %w", err)
	}
	return nil
}

func (s Store) quietReviewPath(jobID, attemptID string) string {
	return filepath.Join(s.root, "history", jobID, attemptID, "quiet-review.json")
}

func invalidQuietJobID(id string) bool { return invalidID(id) || !strings.HasPrefix(id, "job-") }
func invalidQuietAttemptID(id string) bool {
	return invalidID(id) || !strings.HasPrefix(id, "attempt-")
}
