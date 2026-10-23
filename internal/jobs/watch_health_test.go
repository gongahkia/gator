package jobs

import (
	"testing"
	"time"
)

func TestWatchHealthSeparatesConsecutiveFailuresRecoveryAndChangedPolicy(t *testing.T) {
	now := time.Now().UTC()
	makeAttempt := func(id, status string, offset int) Attempt {
		return Attempt{ID: id, JobID: "job-watch", Status: status, StartedAt: now.Add(time.Duration(offset) * time.Minute), NotificationPolicySHA256: "policy"}
	}
	first := makeAttempt("attempt-a", "failed", 0)
	second := makeAttempt("attempt-b", "interrupted", 1)
	running := makeAttempt("attempt-c", "running", 2)
	recovery := makeAttempt("attempt-d", "unchanged", 3)
	history := []Attempt{recovery, running, second, first}
	h := ReduceWatchHealth(history, second)
	if h.First.ID != first.ID || h.Attempts != 2 || h.Recovered {
		t.Fatalf("repeated failure lost incident: %+v", h)
	}
	h = ReduceWatchHealth(history, recovery)
	if h.First.ID != first.ID || h.Attempts != 2 || !h.Recovered {
		t.Fatalf("quiet recovery lost incident: %+v", h)
	}
	third := makeAttempt("attempt-e", "failed", 4)
	h = ReduceWatchHealth(history, third)
	if h.First.ID != third.ID || h.Attempts != 1 {
		t.Fatal("new incident was merged across success", h)
	}
	second.NotificationPolicySHA256 = "new-policy"
	if h = ReduceWatchHealth(history, second); h.Attempts != 1 {
		t.Fatal("incident crossed changed agreement", h)
	}
	// Equal timestamps have deterministic ordering, regardless of filesystem enumeration.
	second.NotificationPolicySHA256 = "policy"
	second.StartedAt = first.StartedAt
	if h = ReduceWatchHealth([]Attempt{second, first}, second); h.First.ID != first.ID {
		t.Fatal("unstable incident ordering", h)
	}
	limited := makeAttempt("attempt-limit", "limited", 5)
	if h = ReduceWatchHealth(append(history, third), limited); h.First.ID != limited.ID || h.Attempts != 1 {
		t.Fatal("changed limit/failure phase was suppressed", h)
	}
	if count := ConsecutiveUnsuccessfulChecks(append(history, third), limited); count != 2 {
		t.Fatal("phase change lost unsuccessful check count", count)
	}
}

func TestCoverageGapsAreObservedAndBoundToCheckLifetime(t *testing.T) {
	now := time.Now()
	a := Availability{Gaps: []CoverageGap{{From: now.Add(-time.Hour), To: now}}}
	created := now.Add(-30 * time.Minute)
	if gap := LatestCoverageGap(a, created); !gap.From.Equal(created) || !gap.To.Equal(now) {
		t.Fatal("gap predates agreement", gap)
	}
	if gap := LatestCoverageGap(a, now.Add(time.Minute)); !gap.From.IsZero() {
		t.Fatal("unrelated old gap projected", gap)
	}
}
