package workhome

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/suggestion"
	"github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workhistory"
)

func findSection(t *testing.T, view Overview, id string) OverviewSection {
	t.Helper()
	for _, s := range view.Sections {
		if s.ID == id {
			return s
		}
	}
	t.Fatal("missing section", id)
	return OverviewSection{}
}
func hasOverviewItem(section OverviewSection, kind, target string) bool {
	for _, item := range section.Items {
		if item.Kind == kind && (target == "" || item.Target == target) {
			return true
		}
	}
	return false
}
func TestOverviewUnifiesChangesReviewsPlansAndBlockersWithoutMutatingThem(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	now := time.Now()
	gs, _ := goal.Open(root)
	g, err := gs.Add("Offsite", "Choose a venue", source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := gs.InitializePlan(g.ID, "Choose a venue", []string{"Resolve capacity", "Compare quotes"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = gs.ApplyPlanChange(g.ID, goal.PlanChange{Kind: "update_step", StepID: plan.Steps[0].ID, Status: goal.StepBlocked, ExpectedRevision: plan.Revision})
	if err != nil {
		t.Fatal(err)
	}
	_, err = gs.ApplyPlanChange(g.ID, goal.PlanChange{Kind: "set_next_review", NextReviewAt: now.Add(-time.Hour), ExpectedRevision: plan.Revision})
	if err != nil {
		t.Fatal(err)
	}
	is, _ := inbox.Open(root)
	entry, err := is.Add(inbox.Entry{Kind: "job", Title: "Deadline changed", Summary: "A decision is due tomorrow.", Status: "completed", GoalID: g.ID, CreatedAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	proposals, _ := suggestion.Open(root)
	proposal, err := proposals.Create(suggestion.Record{Kind: "note", Title: "Updated deadline", Body: "Review tomorrow", GoalID: g.ID, WorkID: "work-proposal"})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := jobs.Open(root)
	definition, err := store.Save(jobs.Definition{Name: "Project check", Enabled: true, Schedule: "0 9 * * *", Timezone: "UTC", Missed: "skip", SourcePath: source, RefreshSnapshot: true, Objective: "Check project changes", Mode: action.Inspect, Contract: artifact.InspectionContract(), GoalID: g.ID})
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := usage.Open(root)
	if err = ledger.SavePolicy(usage.Policy{Paused: true}); err != nil {
		t.Fatal(err)
	}
	view, err := ReadOverview(context.Background(), root, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Sections) != 4 {
		t.Fatal("the four everyday questions are not visible")
	}
	if !hasOverviewItem(findSection(t, view, "changed"), "inbox", entry.ID) {
		t.Fatal("new result missing")
	}
	needs := findSection(t, view, "needs")
	if !hasOverviewItem(needs, "suggestion", proposal.ID) || !hasOverviewItem(needs, "inbox", entry.ID) || !hasOverviewItem(needs, "plan", g.ID) {
		t.Fatal("reviews were not brought together")
	}
	if !hasOverviewItem(findSection(t, view, "next"), "plan", g.ID) {
		t.Fatal("next step missing")
	}
	blocked := findSection(t, view, "blocked")
	if !hasOverviewItem(blocked, "plan", g.ID) || !hasOverviewItem(blocked, "recovery", "") || !hasOverviewItem(blocked, "usage", "") {
		t.Fatal("blockers were hidden")
	}
	// A schedule's calendar time becomes visible only with a live supervisor.
	online, err := ReadOverview(context.Background(), root, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if !hasOverviewItem(findSection(t, online, "next"), "check", definition.ID) {
		t.Fatal("next scheduled check missing")
	}
	if err = MarkOverviewSeen(root, view.At); err != nil {
		t.Fatal(err)
	}
	reviewed, err := ReadOverview(context.Background(), root, false, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if findSection(t, reviewed, "changed").Total != 0 {
		t.Fatal("reviewed changes did not clear")
	}
	if !hasOverviewItem(findSection(t, reviewed, "needs"), "inbox", entry.ID) {
		t.Fatal("reviewing changes silently marked a result read")
	}
	saved, _ := proposals.Load(proposal.ID)
	if saved.Status != "pending" {
		t.Fatal("reading overview accepted a proposal")
	}
	current, _ := store.Load(definition.ID)
	if !current.Enabled {
		t.Fatal("reading overview changed a check")
	}
}
func TestOverviewShowsStoppedWorkAndCorruptStoresAsBlockers(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	history, _ := workhistory.Open(root)
	if _, err := history.Start(workhistory.Start{ID: "interrupted", Objective: "Interrupted project review", Mode: action.Inspect, ExternalActions: action.Forbid, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "gator", "suggestions"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gator", "suggestions", "suggestion-broken.json"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	view, err := ReadOverview(context.Background(), root, false, now)
	if err != nil {
		t.Fatal(err)
	}
	blocked := findSection(t, view, "blocked")
	if !hasOverviewItem(blocked, "work", "interrupted") {
		t.Fatal("stopped run was hidden")
	}
	found := false
	for _, item := range blocked.Items {
		if strings.Contains(item.Title, "Could not read proposed changes") {
			found = true
		}
	}
	if !found {
		t.Fatal("unreadable state was presented as healthy")
	}
}
func TestOverviewKeepsDeterministicBoundedListsAndCancellation(t *testing.T) {
	root := t.TempDir()
	store, _ := inbox.Open(root)
	now := time.Now()
	for i := 0; i < 55; i++ {
		if _, err := store.Add(inbox.Entry{Kind: "job", Title: "Result", Summary: "Retained update", Status: "completed", CreatedAt: now.Add(-time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	view, err := ReadOverview(context.Background(), root, true, now)
	if err != nil {
		t.Fatal(err)
	}
	changed := findSection(t, view, "changed")
	if changed.Total != 55 || len(changed.Items) != 50 {
		t.Fatal("bounded results hid the total")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = ReadOverview(ctx, root, true, now); err != context.Canceled {
		t.Fatal("cancellation was ignored", err)
	}
}
