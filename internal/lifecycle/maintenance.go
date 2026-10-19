package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/gongahkia/gator/internal/state"
)

type Sweep struct {
	Day, PolicyDigest string
	At                time.Time
	Removed           int
}

// Maintain applies only an explicitly saved retention policy. It is called at
// startup and from the local supervisor; busy data is left for a later sweep.
func Maintain(ctx context.Context, stateDir string, now time.Time) (Sweep, error) {
	s, err := Open(stateDir)
	if err != nil {
		return Sweep{}, err
	}
	p, err := s.Policy()
	if err != nil {
		return Sweep{}, err
	}
	if p.HistoryDays == 0 && p.KnowledgeDays == 0 && p.SnapshotDays == 0 {
		return Sweep{}, nil
	}
	release, err := state.Guard(stateDir, true)
	if err != nil {
		return Sweep{}, err
	}
	defer release()
	p, err = s.Policy()
	if err != nil {
		return Sweep{}, err
	}
	if p.HistoryDays == 0 && p.KnowledgeDays == 0 && p.SnapshotDays == 0 {
		return Sweep{}, nil
	}
	data, _ := json.Marshal(p)
	digest := sha256.Sum256(data)
	policyDigest := hex.EncodeToString(digest[:])
	day := now.UTC().Format("2006-01-02")
	var last Sweep
	if data, err := os.ReadFile(filepath.Join(s.control, "last-sweep.json")); err == nil {
		_ = json.Unmarshal(data, &last)
	}
	if last.Day == day && last.PolicyDigest == policyDigest {
		return last, nil
	}
	plan, err := s.selectRetention(ctx, now, p)
	if err != nil {
		return Sweep{}, err
	}
	plan, err = s.savePlan(plan)
	if err != nil {
		return Sweep{}, err
	}
	removed, err := s.applyLocked(ctx, plan.ID)
	if err != nil {
		return Sweep{}, err
	}
	result := Sweep{Day: day, PolicyDigest: policyDigest, At: now.UTC(), Removed: removed}
	return result, privateJSON(filepath.Join(s.control, "last-sweep.json"), result)
}
