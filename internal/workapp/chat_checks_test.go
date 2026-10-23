package workapp

import (
	"errors"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workhome"
)

func TestChatCheckHealthKeepsOriginAndReportsStaleCoverage(t *testing.T) {
	m := New(Config{})
	m.conversation = "origin"
	now := time.Now().UTC().Truncate(time.Second)
	checks := []workhome.Check{{ID: "job-one", Title: "Project", State: "offline", LastSuccess: now, Unsuccessful: 3, CoverageGap: jobs.CoverageGap{From: now.Add(-time.Hour), To: now}, Definition: jobs.Definition{OriginConversationID: "origin"}}, {ID: "job-other", Title: "Private other", Definition: jobs.Definition{OriginConversationID: "other"}}}
	next, _ := m.Update(checksRefreshed{Checks: checks})
	m = next.(Model)
	p := m.BridgeState().ChatChecks
	if len(p) != 1 || p[0].ID != "job-one" || p[0].NextDue != "" || p[0].LastSuccess == "" || p[0].GapFrom == "" || p[0].Unsuccessful != 3 {
		t.Fatal("untruthful health projection", p)
	}
	next, _ = m.Update(checksRefreshed{Err: errors.New("unavailable")})
	m = next.(Model)
	if state := m.BridgeState(); len(state.ChatChecks) != 1 || state.ChatChecksIssue == "" {
		t.Fatal("failed refresh lost last known state")
	}
	m.conversation = "new-chat"
	if len(m.BridgeState().ChatChecks) != 0 || m.BridgeState().ChatChecksIssue != "" {
		t.Fatal("health leaked into another chat")
	}
	m.config.Jobs = append(m.config.Jobs, jobs.Definition{OriginConversationID: "new-chat"})
	if m.BridgeState().ChatChecksIssue == "" {
		t.Fatal("known check with no loaded health was silently omitted")
	}
}
