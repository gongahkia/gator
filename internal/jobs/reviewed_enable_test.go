package jobs

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
)

func TestReviewedEnableRequiresCurrentAgreementAndRetainsExpiry(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d, err := store.Save(Definition{Name: "Daily brief", Enabled: true, Schedule: "0 9 * * *", Timezone: "UTC", SourcePath: t.TempDir(), Objective: "Write brief", Mode: action.Draft, Contract: artifact.DefaultContract("brief.md")})
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := d.PolicyDigest()
	if _, err := store.SetEnabledReviewed(d.ID, false, ""); err == nil {
		t.Fatal("missing review accepted")
	}
	paused, err := store.SetEnabledReviewed(d.ID, false, digest)
	if err != nil || paused.Enabled {
		t.Fatalf("pause = %+v, %v", paused, err)
	}
	if _, err := store.SetEnabledReviewed(d.ID, true, digest); err == nil {
		t.Fatal("pre-pause review resumed changed policy")
	}
	current, _ := paused.PolicyDigest()
	resumed, err := store.SetEnabledReviewed(d.ID, true, current)
	if err != nil || !resumed.Enabled {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	current, _ = resumed.PolicyDigest()
	resumed.Objective = "New objective"
	if _, err := store.SaveReviewed(resumed, current); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetEnabledReviewed(d.ID, false, current); err == nil {
		t.Fatal("stale agreement changed current check")
	}
	d, err = store.Load(d.ID)
	if err != nil || !d.Enabled || d.Objective != "New objective" {
		t.Fatalf("stale review overwrote check: %+v, %v", d, err)
	}
	d.EndsAt = time.Now().Add(-time.Hour)
	d.Enabled = false
	d, err = store.Save(d)
	if err != nil {
		t.Fatal(err)
	}
	current, _ = d.PolicyDigest()
	if _, err := store.SetEnabledReviewed(d.ID, true, current); err == nil {
		t.Fatal("expired agreement resumed")
	}
}
