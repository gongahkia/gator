package jobs

import (
	"sort"
	"time"
)

// WatchHealth derives one incident from immutable attempts. Its identity is the
// first unsuccessful attempt, so retries and process restarts cannot create a
// new incident for every failed poll. A successful check closes the incident;
// it does not reconstruct changes during the unsuccessful interval.
type WatchHealth struct {
	First     Attempt
	Attempts  int
	Recovered bool
}

func unsuccessfulCheck(status string) bool {
	return status == "failed" || status == "interrupted" || status == "limited"
}

func ReduceWatchHealth(history []Attempt, current Attempt) WatchHealth {
	return reduceWatchHealth(history, current, true)
}

func ConsecutiveUnsuccessfulChecks(history []Attempt, current Attempt) int {
	if !unsuccessfulCheck(current.Status) {
		return 0
	}
	return reduceWatchHealth(history, current, false).Attempts
}

func reduceWatchHealth(history []Attempt, current Attempt, separateLimits bool) WatchHealth {
	// Shared admission holds are already exposed by chat health and Usage.
	// They never create inbox incidents or recovery alerts, including when the
	// user deliberately pauses requests. Keep counting them for coverage health.
	sharedHold := func(a Attempt) bool { return a.Status == "limited" && a.LimitReason == "shared" }
	if separateLimits && sharedHold(current) {
		return WatchHealth{}
	}
	if current.ID == "" || current.StartedAt.IsZero() || (!unsuccessfulCheck(current.Status) && current.Status != "completed" && current.Status != "unchanged") {
		return WatchHealth{}
	}
	items := []Attempt{current}
	for _, previous := range history {
		if previous.ID == current.ID || previous.JobID != current.JobID || previous.StartedAt.After(current.StartedAt) {
			continue
		}
		if previous.StartedAt.Equal(current.StartedAt) && previous.ID > current.ID {
			continue
		}
		items = append(items, previous)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
	h := WatchHealth{Recovered: !unsuccessfulCheck(current.Status)}
	for _, item := range items {
		if separateLimits && sharedHold(item) {
			continue
		}
		if item.Status == "running" || item.ID == current.ID && h.Recovered {
			continue
		}
		if item.NotificationPolicySHA256 != "" && current.NotificationPolicySHA256 != "" && item.NotificationPolicySHA256 != current.NotificationPolicySHA256 {
			break
		}
		if !unsuccessfulCheck(item.Status) {
			break
		}
		// A request limit becoming an execution failure (or the reverse) is a
		// different issue, so it receives its own first notice. Repeated polls
		// within the same phase still share an incident.
		if separateLimits && !h.First.StartedAt.IsZero() && (item.Status == "limited") != (h.First.Status == "limited") {
			break
		}
		h.First, h.Attempts = item, h.Attempts+1
	}
	return h
}

// LatestCoverageGap reports an observed supervisor heartbeat gap that overlaps
// this check's lifetime. It is an availability observation, not source coverage.
func LatestCoverageGap(a Availability, created time.Time) CoverageGap {
	for i := len(a.Gaps) - 1; i >= 0; i-- {
		gap := a.Gaps[i]
		if gap.To.After(created) {
			if gap.From.Before(created) {
				gap.From = created
			}
			return gap
		}
	}
	return CoverageGap{}
}
