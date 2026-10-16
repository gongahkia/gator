package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/knowledge"
)

func TestNoteAndSkillCommandsCreatePrivatePersistentKnowledge(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	var out bytes.Buffer
	if err := run([]string{"goal", "add", "Plan the launch"}, &out); err != nil {
		t.Fatal(err)
	}
	goalID := strings.Fields(out.String())[0]
	out.Reset()
	if err := run([]string{"note", "add", "--title", "Launch meeting", "--goal", goalID, "--body", "Decision moved to Tuesday"}, &out); err != nil {
		t.Fatal(err)
	}
	noteID := strings.Fields(out.String())[0]
	out.Reset()
	if err := run([]string{"note", "search", "--goal", goalID, "Tuesday"}, &out); err != nil || !strings.Contains(out.String(), noteID) {
		t.Fatalf("note search = %q, %v", out.String(), err)
	}
	file := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(file, []byte("# Brief\nCheck dates and cite sources."), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"skill", "create", "daily-brief", "--description", "For morning summaries", "--file", file}, &out); err != nil {
		t.Fatal(err)
	}
	store, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	skill, err := store.LoadSkill("daily-brief")
	if err != nil || !strings.Contains(skill.Body, "Check dates") {
		t.Fatalf("saved skill = %#v, %v", skill, err)
	}
	path, err := store.SkillPath(skill.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(state, "gator", "knowledge", "skills")) {
		t.Fatalf("skill escaped knowledge root: %s", path)
	}
	out.Reset()
	if err := run([]string{"skill", "disable", "daily-brief"}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"skill", "list"}, &out); err != nil || strings.Contains(out.String(), "daily-brief") {
		t.Fatalf("disabled skill listed = %q, %v", out.String(), err)
	}
	if err := run([]string{"note", "remove", noteID, "--yes"}, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadNote(noteID); !os.IsNotExist(err) {
		t.Fatalf("note remained: %v", err)
	}
}

func TestTUIKnowledgeCommandsKeepGoalScopeAndText(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	var out bytes.Buffer
	if err := run([]string{"goal", "add", "Plan offsite"}, &out); err != nil {
		t.Fatal(err)
	}
	goalID := strings.Fields(out.String())[0]
	result, err := workTUINoteAction("add Venue decision | We chose the smaller room", goalID)
	if err != nil {
		t.Fatal(err)
	}
	noteID := strings.Fields(result)[0]
	store, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	note, err := store.LoadNote(noteID)
	if err != nil || note.GoalID != goalID || note.Title != "Venue decision" || note.Body != "We chose the smaller room" {
		t.Fatalf("TUI note = %#v, %v", note, err)
	}
	result, err = workTUISkillAction("create venue-review | When checking venues | Check capacity | Then accessibility")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "venue-review") {
		t.Fatalf("TUI skill result = %q", result)
	}
	skill, err := store.LoadSkill("venue-review")
	if err != nil || skill.Body != "Check capacity | Then accessibility" {
		t.Fatalf("TUI skill = %#v, %v", skill, err)
	}
	link := filepath.Join(t.TempDir(), "link.md")
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), link); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"skill", "create", "bad-import", "--description", "bad", "--file", link}, &out); err == nil {
		t.Fatal("symlinked skill import accepted")
	}
}
