package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/suggestion"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestSuggestionShowChecksRetainedReferencesAndSourceChanges(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	work := createCLIWorkBundle(t, stateDir, "suggestion-evidence")
	retained, err := workspace.OpenWork(work.Path)
	if err != nil {
		t.Fatal(err)
	}
	contents := []byte("Venue A capacity is 80.\n")
	if err := os.WriteFile(filepath.Join(retained.Source.Path(), "venue.md"), contents, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	hash := hex.EncodeToString(digest[:])
	root, err := workspace.Open(work.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteRegularFileAtomic("evidence/"+hash+".txt", contents, 512*1024); err != nil {
		t.Fatal(err)
	}
	bundle, err := artifact.OpenBundle(work.Path)
	if err != nil {
		t.Fatal(err)
	}
	manifest := bundle.Manifest
	manifest.Evidence = []artifact.Evidence{{ID: "evidence-venue", Locator: "source/venue.md", SHA256: hash, SnapshotPath: "evidence/" + hash + ".txt", RetrievedAt: time.Now().UTC()}}
	if err := artifact.WriteManifest(work.ManifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	queue, err := suggestion.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := queue.Create(suggestion.Record{Kind: "note", Title: "Venue", Body: "Capacity is 80", WorkID: work.ID, SourceRefs: []string{"source/venue.md:1", "source/venue.md:99"}})
	if err != nil {
		t.Fatal(err)
	}
	check := func() suggestionEvidenceCheck {
		var output bytes.Buffer
		if err := suggestionCommand([]string{"show", record.ID}, &output); err != nil {
			t.Fatal(err)
		}
		var displayed struct {
			ID                 string                  `json:"id"`
			ReferenceIntegrity suggestionEvidenceCheck `json:"reference_integrity"`
		}
		if err := json.Unmarshal(output.Bytes(), &displayed); err != nil || displayed.ID != record.ID {
			t.Fatalf("suggestion show = %s, %v", output.String(), err)
		}
		return displayed.ReferenceIntegrity
	}
	first := check()
	if first.Status != "unverified" || len(first.Verified) != 1 || first.Verified[0] != "source/venue.md:1" || len(first.Unverified) != 1 || first.Unverified[0] != "source/venue.md:99" {
		t.Fatalf("initial source check = %#v", first)
	}
	if err := os.WriteFile(filepath.Join(retained.Source.Path(), "venue.md"), []byte("Venue A capacity is 100.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	after := check()
	if after.Status != "unverified" || len(after.Verified) != 0 || len(after.Unverified) != 2 {
		t.Fatalf("changed source remained verified: %#v", after)
	}
	if err := os.WriteFile(filepath.Join(work.Output.Path(), "report.md"), []byte("tampered\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if tampered := check(); tampered.Status != "work_unavailable" || len(tampered.Verified) != 0 {
		t.Fatalf("tampered Work bundle backed a suggestion: %#v", tampered)
	}
}

func TestAcceptedChatGoalProposalCreatesPlanAndReadOnlyFollowUpOnce(t *testing.T) {
	state := t.TempDir()
	source := t.TempDir()
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATOR_STATE_DIR", state)
	queue, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := queue.Create(suggestion.Record{
		Kind: "goal", Title: "Plan the offsite", Body: "Compare venues and decide by Friday.",
		GoalSourcePath: source, GoalOutcome: "Choose a venue and confirm the budget",
		GoalSteps: []string{"Confirm capacity", "Compare prices"}, WorkID: "work-chat-1",
		Cron: "0 9 * * 1-5", Timezone: "Asia/Singapore", Task: "Review the offsite plan and changed sources",
	})
	if err != nil {
		t.Fatal(err)
	}
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if before, err := goals.List(); err != nil || len(before) != 0 {
		t.Fatalf("pending proposal created a goal: %#v, %v", before, err)
	}
	jobStore, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if before, err := jobStore.List(); err != nil || len(before) != 0 {
		t.Fatalf("pending proposal created a schedule: %#v, %v", before, err)
	}
	firstID, err := applySuggestion(state, proposal)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := suggestionCommand([]string{"accept", proposal.ID}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "gator goal chat "+firstID) {
		t.Fatalf("missing continuation command: %s", output.String())
	}
	accepted, err := queue.Load(proposal.ID)
	if err != nil || accepted.Status != "accepted" || accepted.ResultID != firstID {
		t.Fatalf("accepted proposal = %#v, %v", accepted, err)
	}
	created, err := goals.Load(firstID)
	if err != nil || created.Title != proposal.Title || created.Context != proposal.Body || created.SourcePath != source {
		t.Fatalf("created goal = %#v, %v", created, err)
	}
	plan, err := goals.LoadPlan(firstID)
	if err != nil || plan.Outcome != proposal.GoalOutcome || len(plan.Steps) != 2 || plan.Steps[0].Title != "Confirm capacity" || plan.Steps[0].Status != goal.StepPending {
		t.Fatalf("initial plan = %#v, %v", plan, err)
	}
	if jobsFound, err := jobStore.List(); err != nil || len(jobsFound) != 1 || jobsFound[0].GoalID != firstID || jobsFound[0].Mode != action.Inspect || jobsFound[0].Contract.ExternalActions != action.Forbid {
		t.Fatalf("read-only follow-up = %#v, %v", jobsFound, err)
	}
	if again, err := goals.List(); err != nil || len(again) != 1 {
		t.Fatalf("repeated apply created another goal: %#v, %v", again, err)
	}
}

func TestReviewSuggestionsActivatesOnlyAcceptedChanges(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	queue, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	note, err := queue.Create(suggestion.Record{Kind: "note", Title: "Venue", Body: "Capacity is 90", WorkID: "work-1"})
	if err != nil {
		t.Fatal(err)
	}
	knowledgeStore, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if notes, err := knowledgeStore.ListNotes(""); err != nil || len(notes) != 0 {
		t.Fatalf("pending note became active: %#v, %v", notes, err)
	}
	var output bytes.Buffer
	if err := suggestionCommand([]string{"list"}, &output); err != nil || !strings.Contains(output.String(), note.ID) {
		t.Fatalf("pending list = %q, %v", output.String(), err)
	}
	if err := suggestionCommand([]string{"accept", note.ID}, &output); err != nil {
		t.Fatal(err)
	}
	accepted, err := queue.Load(note.ID)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("accepted = %#v, %v", accepted, err)
	}
	if saved, err := knowledgeStore.LoadNote(accepted.ResultID); err != nil || saved.Body != note.Body || len(saved.SourceRefs) != 1 || saved.SourceRefs[0] != "work:work-1" {
		t.Fatalf("saved note = %#v, %v", saved, err)
	}
	if err := suggestionCommand([]string{"accept", note.ID}, &output); err == nil {
		t.Fatal("accepted suggestion applied twice")
	}
	skill, err := queue.Create(suggestion.Record{Kind: "skill", SkillID: "daily-brief", Description: "Use for a daily briefing", Body: "Read calendar, then email.", WorkID: "work-2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := suggestionCommand([]string{"reject", skill.ID}, &output); err != nil {
		t.Fatal(err)
	}
	if _, err := knowledgeStore.LoadSkill("daily-brief"); err == nil {
		t.Fatal("rejected skill became active")
	}
}

func TestAcceptedPlanSuggestionChecksCurrentRevision(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := queue.Create(suggestion.Record{Kind: "plan", GoalID: record.ID, WorkID: "work-1", PlanChange: goal.PlanChange{Kind: "add_step", ExpectedRevision: 0, Title: "Confirm capacity"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "set_outcome", ExpectedRevision: 0, Outcome: "Choose a venue"}); err != nil {
		t.Fatal(err)
	}
	if err := suggestionCommand([]string{"accept", proposal.ID}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "expected revision") {
		t.Fatalf("stale plan suggestion applied: %v", err)
	}
	loaded, err := queue.Load(proposal.ID)
	if err != nil || loaded.Status != "pending" {
		t.Fatalf("stale proposal status = %#v, %v", loaded, err)
	}
}

func TestAcceptedScheduleSuggestionCreatesInspectOnlyGoalJob(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Daily project check", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := queue.Create(suggestion.Record{Kind: "schedule", GoalID: record.ID, WorkID: "work-1", Cron: "0 9 * * 1-5", Timezone: "Asia/Singapore", Task: "Check changed blockers", ChangeOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	jobStore, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if before, err := jobStore.List(); err != nil || len(before) != 0 {
		t.Fatalf("pending schedule created job: %#v, %v", before, err)
	}
	firstID, err := applySuggestion(state, proposal)
	if err != nil || firstID == "" {
		t.Fatalf("first schedule application = %q, %v", firstID, err)
	}
	if err := suggestionCommand([]string{"accept", proposal.ID}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	accepted, err := queue.Load(proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := jobStore.Load(accepted.ResultID)
	if err != nil || job.GoalID != record.ID || !job.ChangeOnly || job.Mode != "inspect" || job.Contract.ExternalActions != "forbid" {
		t.Fatalf("accepted job = %#v, %v", job, err)
	}
	all, err := jobStore.List()
	if err != nil || len(all) != 1 || accepted.ResultID != firstID {
		t.Fatalf("schedule retry created a duplicate: %#v, %v", all, err)
	}
}
