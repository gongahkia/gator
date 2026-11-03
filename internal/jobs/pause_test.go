package jobs

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
)

func TestPausedCheckRejectsDueAndStaleClaims(t *testing.T) {
	state := t.TempDir()
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	d, err := store.Save(Definition{Name: "Project", Enabled: true, Schedule: "*/15 * * * *", Timezone: "UTC", Missed: "run_once", SourcePath: t.TempDir(), RefreshSnapshot: true, Objective: "Check changes", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	slot := d.CreatedAt.Truncate(time.Hour).Add(2 * time.Hour)
	if _, ready, err := Due(d, slot); err != nil || !ready {
		t.Fatalf("enabled slot not due: %t %v", ready, err)
	}
	paused, err := store.SetEnabled(d.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{slot, slot.Add(7 * time.Minute), slot.Add(24 * time.Hour)} {
		if due, ready, err := Due(paused, at); err != nil || ready || !due.IsZero() {
			t.Fatalf("paused check due at %s: %s %t %v", at, due, ready, err)
		}
	}
	// The supervisor may have observed an enabled slot immediately before pause.
	if _, claimed, err := store.Claim(d.ID, slot); err != nil || claimed {
		t.Fatalf("stale legacy claim admitted: %t %v", claimed, err)
	}
	if _, _, claimed, err := store.ClaimBegin(d.ID, slot); err != nil || claimed {
		t.Fatalf("stale durable claim admitted: %t %v", claimed, err)
	}
	store, err = Open(state)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := store.Load(d.ID)
	if err != nil || !retained.LastScheduledAt.IsZero() {
		t.Fatalf("paused claim advanced watermark: %+v %v", retained, err)
	}
	history, err := store.History(d.ID, 0)
	if err != nil || len(history) != 0 {
		t.Fatalf("paused claim created attempt: %+v %v", history, err)
	}
}

func TestCheckEvidenceDigestPreservesReviewGuards(t *testing.T) {
	d := Definition{Enabled: true, Objective: "Check changes", SourcePath: "project"}
	review, _ := d.PolicyDigest()
	evidence, _ := d.CheckPolicyDigest()
	if review != evidence {
		t.Fatal("existing enabled evidence digest changed")
	}
	d.Enabled = false
	pausedReview, _ := d.PolicyDigest()
	pausedEvidence, _ := d.CheckPolicyDigest()
	if review == pausedReview || evidence != pausedEvidence {
		t.Fatal("pause must invalidate reviews while preserving check evidence")
	}
	d.Objective = "Check another outcome"
	changed, _ := d.CheckPolicyDigest()
	if changed == evidence {
		t.Fatal("substantive edit reused old evidence")
	}
}
