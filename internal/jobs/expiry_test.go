package jobs

import (
	"encoding/json"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"strings"
	"testing"
	"time"
)

func TestExpiryStopsCatchupClaimsAndManualRuns(t *testing.T) {
	store, _ := Open(t.TempDir())
	now := time.Now().UTC()
	definition, err := store.Save(Definition{Name: "Timed check", Enabled: true, Schedule: "* * * * *", SourcePath: t.TempDir(), Objective: "Check changes", Mode: action.Inspect, Contract: artifact.InspectionContract(), EndsAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := definition.PolicyDigest()
	if _, due, err := Due(definition, now); err != nil || !due {
		t.Fatalf("before expiry: %v %v", due, err)
	}
	for _, at := range []time.Time{definition.EndsAt, definition.EndsAt.Add(time.Hour)} {
		if _, due, err := Due(definition, at); err != nil || due {
			t.Fatalf("catchup past expiry: %v %v", due, err)
		}
	}
	definition.EndsAt = now.Add(-time.Second)
	definition, err = store.Save(definition)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := definition.PolicyDigest()
	if before == after {
		t.Fatal("expiry not bound to review")
	}
	if _, _, ok, err := store.ClaimBegin(definition.ID, now.Add(-time.Minute)); err != nil || ok {
		t.Fatalf("expired stale claim: %v %v", ok, err)
	}
	if _, ok, err := store.Claim(definition.ID, now); err != nil || ok {
		t.Fatalf("expired claim: %v %v", ok, err)
	}
	if _, err := store.Begin(definition, now); err == nil {
		t.Fatal("manual run bypassed expiry")
	}
	if _, err := store.SetEnabled(definition.ID, true); err == nil {
		t.Fatal("resume bypassed expiry")
	}
	if _, err := store.SetEnabled(definition.ID, false); err != nil {
		t.Fatal(err)
	}
	definition.EndsAt = time.Time{}
	data, _ := json.Marshal(definition)
	if strings.Contains(string(data), "ends_at") {
		t.Fatal("unset expiry changes legacy policy digests")
	}
}
