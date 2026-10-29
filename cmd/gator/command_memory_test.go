package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/personalization"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestPersonalSummaryCLIAndTerminalResultFollowCorrections(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", dir)
	store, _ := memory.Open(dir)
	record, err := store.Add("preference", "Use concise answers.", "", "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := memoryCommand([]string{"summary", "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var summary personalization.Summary
	if err := json.Unmarshal(output.Bytes(), &summary); err != nil || len(summary.Entries) != 1 || summary.Entries[0].ID != record.ID {
		t.Fatal("CLI summary", output.String(), err)
	}
	if _, err := store.Correct(record.ID, "Use detailed reviews.", "user"); err != nil {
		t.Fatal(err)
	}
	panel, err := workMemoryResult(dir, "", []string{"summary"})
	if err != nil || panel == nil || strings.Contains(panel.Summary, "concise answers") || !strings.Contains(panel.Summary, "detailed reviews") {
		t.Fatal("stale terminal summary", panel, err)
	}
	if err := store.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := memoryCommand([]string{"summary"}, &output); err != nil || strings.Contains(output.String(), record.ID) || !strings.Contains(output.String(), "gator memory correct ID TEXT") {
		t.Fatal("stale summary or missing correction route", output.String(), err)
	}
}

func TestMemoryCLIAndPromptProjection(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	var output bytes.Buffer
	if err := run([]string{"goal", "add", "Plan launch"}, &output); err != nil {
		t.Fatal(err)
	}
	goalID := strings.Fields(output.String())[0]
	output.Reset()
	if err := run([]string{"memory", "add", "--goal", goalID, "--kind", "decision", "--source", "planning call", "Present the launch deck on Monday"}, &output); err != nil {
		t.Fatal(err)
	}
	memoryID := strings.Fields(output.String())[0]
	request := workrun.Request{GoalID: goalID, Objective: "What day should the launch deck be presented?"}
	if err := applyMemoryContext(state, &request); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request.MemoryContext, memoryID) || !strings.Contains(request.MemoryContext, "Monday") || !strings.Contains(request.MemoryContext, "selected by lexical match: deck, launch (goal)") {
		t.Fatalf("projection = %q", request.MemoryContext)
	}
	output.Reset()
	if err := run([]string{"memory", "search", "--goal", goalID, "launch deck"}, &output); err != nil || !strings.Contains(output.String(), "[lexical match: deck, launch; scope: goal]") {
		t.Fatalf("memory search explanation = %q, %v", output.String(), err)
	}
	output.Reset()
	if err := run([]string{"memory", "correct", memoryID, "Present the launch deck on Tuesday"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := applyMemoryContext(state, &request); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(request.MemoryContext, "Monday") || !strings.Contains(request.MemoryContext, "Tuesday") {
		t.Fatalf("corrected projection = %q", request.MemoryContext)
	}
	output.Reset()
	if err := run([]string{"memory", "forget", memoryID}, &output); err != nil {
		t.Fatal(err)
	}
	if err := applyMemoryContext(state, &request); err != nil || request.MemoryContext != "" {
		t.Fatalf("forgotten projection = %q, %v", request.MemoryContext, err)
	}
}

func TestMemoryCLIFileSourceRequiresReviewAfterChange(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	path := filepath.Join(t.TempDir(), "brief.txt")
	if err := os.WriteFile(path, []byte("Daily brief at 9am"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"memory", "add", "--kind", "preference", "--source-file", path, "Daily brief at 9am"}, &output); err != nil {
		t.Fatal(err)
	}
	id := strings.Fields(output.String())[0]
	request := workrun.Request{Objective: "When is the daily brief?"}
	if err := applyMemoryContext(state, &request); err != nil || !strings.Contains(request.MemoryContext, id) {
		t.Fatalf("initial memory = %q, %v", request.MemoryContext, err)
	}
	if err := os.WriteFile(path, []byte("Daily brief at 8am"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := applyMemoryContext(state, &request); err != nil || request.MemoryContext != "" {
		t.Fatalf("changed source remained in prompt: %q, %v", request.MemoryContext, err)
	}
	output.Reset()
	if err := run([]string{"memory", "show", id}, &output); err != nil || !strings.Contains(output.String(), "Source status: changed") {
		t.Fatalf("memory status = %q, %v", output.String(), err)
	}
	if err := run([]string{"memory", "correct", id, "--source-file", path, "Daily brief at 8am"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := applyMemoryContext(state, &request); err != nil || !strings.Contains(request.MemoryContext, "8am") {
		t.Fatalf("reviewed memory = %q, %v", request.MemoryContext, err)
	}
}

func TestWorkStartsWithScopedRelevantNoteLeads(t *testing.T) {
	state := t.TempDir()
	store, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	goalA := "goal-0123456789abcdef01234567"
	goalB := "goal-aaaaaaaaaaaaaaaaaaaaaaaa"
	note, err := store.AddNoteWithRefs("Offsite planning", "The venue capacity is 90 people.\nConfirm booking by Friday.", goalA, []string{"source/venue.md:4"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddNote("Other team's venue", "Venue capacity is 20.", goalB); err != nil {
		t.Fatal(err)
	}
	budget, err := store.AddNote("Offsite budget", "The approved budget is 4000 dollars.", goalA)
	if err != nil {
		t.Fatal(err)
	}
	request := workrun.Request{GoalID: goalA, Objective: "What is the venue capacity?"}
	if err := applyNoteContext(state, &request); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request.NoteContext, note.ID) || !strings.Contains(request.NoteContext, "capacity is 90") || !strings.Contains(request.NoteContext, "source/venue.md:4") || !strings.Contains(request.NoteContext, "goal, lexical match: capacity, venue") || strings.Contains(request.NoteContext, "capacity is 20") {
		t.Fatalf("note projection = %q", request.NoteContext)
	}
	request.Objective = "What is the budget?"
	request.GoalContext = "Plan the venue capacity for the offsite"
	if err := applyNoteContext(state, &request); err != nil || !strings.Contains(request.NoteContext, budget.ID) || strings.Contains(request.NoteContext, note.ID) {
		t.Fatalf("current request lost to broad goal context = %q, %v", request.NoteContext, err)
	}
	request.Objective = "Continue"
	request.GoalContext = "Check venue capacity before booking"
	if err := applyNoteContext(state, &request); err != nil || !strings.Contains(request.NoteContext, note.ID) {
		t.Fatalf("goal-context fallback = %q, %v", request.NoteContext, err)
	}
	request.Objective = "What is the venue capacity?"
	request.GoalContext = ""
	request.GoalID = goalB
	if err := applyNoteContext(state, &request); err != nil || strings.Contains(request.NoteContext, note.ID) || !strings.Contains(request.NoteContext, "capacity is 20") {
		t.Fatalf("cross-goal note projection = %q, %v", request.NoteContext, err)
	}
	if _, err := store.UpdateNote(note.ID, "Offsite planning", "Venue capacity is 85 after correction."); err != nil {
		t.Fatal(err)
	}
	request.GoalID = goalA
	if err := applyNoteContext(state, &request); err != nil || strings.Contains(request.NoteContext, "capacity is 90") || !strings.Contains(request.NoteContext, "capacity is 85") {
		t.Fatalf("corrected note projection = %q, %v", request.NoteContext, err)
	}
	if err := store.RemoveNote(note.ID); err != nil {
		t.Fatal(err)
	}
	if err := applyNoteContext(state, &request); err != nil || request.NoteContext != "" {
		t.Fatalf("removed note projection = %q, %v", request.NoteContext, err)
	}
}

func TestChatProfileCapsuleIsForegroundOnlyAndHonorsExclusions(t *testing.T) {
	state := t.TempDir()
	store, err := memory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.RememberChat(memory.ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I prefer concise bullet points.", ConversationID: "work-profile", RunID: "run-profile"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ foreground, excluded, want bool }{{true, false, true}, {true, true, false}, {false, false, false}} {
		request := workrun.Request{Objective: "Suggest an outdoor activity.", ForegroundApp: test.foreground}
		if test.excluded {
			request.ExcludedKnowledgeIDs = []string{record.ID}
		}
		if err := applyMemoryContext(state, &request); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(request.MemoryContext, record.Text) != test.want {
			t.Fatal("profile recall crossed foreground/exclusion boundary", request.MemoryContext, test)
		}
		if len(request.ContextRefs) != map[bool]int{true: 1, false: 0}[test.want] {
			t.Fatal("supplied refs differ from actual capsule")
		}
	}
}
