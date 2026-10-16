package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotesAndSkillsPersistWithScopeAndCorrection(t *testing.T) {
	state := t.TempDir()
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	goalA := "goal-0123456789abcdef01234567"
	goalB := "goal-aaaaaaaaaaaaaaaaaaaaaaaa"
	global, err := store.AddNote("Meeting template", "Agenda, decisions, and owners", "")
	if err != nil {
		t.Fatal(err)
	}
	local, err := store.AddNote("Launch decisions", "Use the blue deck on Monday", goalA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddNote("Private", "Keep this for another goal", goalB); err != nil {
		t.Fatal(err)
	}
	skill, err := store.AddSkill("daily-brief", "When preparing a morning brief", "## Steps\n1. Check dated sources.\n2. List changes.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSkill("daily-brief", "duplicate", "body"); err == nil {
		t.Fatal("duplicate skill accepted")
	}
	reopened, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := reopened.ListNotes(goalA)
	if err != nil || len(notes) != 2 {
		t.Fatalf("goal notes = %#v, %v", notes, err)
	}
	for _, n := range notes {
		if n.GoalID == goalB {
			t.Fatalf("cross-goal note leaked: %#v", n)
		}
	}
	matched, err := reopened.SearchNotes("blue deck", goalA, 8)
	if err != nil || len(matched) != 1 || matched[0].ID != local.ID {
		t.Fatalf("search = %#v, %v", matched, err)
	}
	matched, err = reopened.SearchNotes("blue deck", goalB, 8)
	if err != nil || len(matched) != 0 {
		t.Fatalf("cross-goal search = %#v, %v", matched, err)
	}
	if _, err := reopened.UpdateNote(local.ID, "Launch decisions", "Use the green deck on Tuesday"); err != nil {
		t.Fatal(err)
	}
	matched, err = reopened.SearchNotes("blue deck", goalA, 8)
	if err != nil || len(matched) != 0 {
		t.Fatalf("stale note search = %#v, %v", matched, err)
	}
	if _, err := reopened.SetSkillEnabled(skill.ID, false); err != nil {
		t.Fatal(err)
	}
	skills, err := reopened.ListSkills(true)
	if err != nil || len(skills) != 0 {
		t.Fatalf("disabled skills = %#v, %v", skills, err)
	}
	path, err := reopened.NotePath(global.ID)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("note permissions = %v, %v", info, err)
	}
	if err := reopened.RemoveNote(local.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.LoadNote(local.ID); !os.IsNotExist(err) {
		t.Fatalf("removed note = %v", err)
	}
}

func TestKnowledgeRejectsTraversalSymlinksAndOversizedBodies(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSkill("../escape", "description", "body"); err == nil {
		t.Fatal("skill path traversal accepted")
	}
	if _, err := store.AddNote("large", strings.Repeat("x", maxNoteBytes+1), ""); err == nil {
		t.Fatal("oversized note accepted")
	}
	d, err := store.AddNote("safe", "private text", "")
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.NotePath(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadNote(d.ID); err == nil || !strings.Contains(err.Error(), "bounded regular") {
		t.Fatalf("symlink accepted: %v", err)
	}
}

func TestKnowledgeRootRejectsSymlinkAndCreatesMissingState(t *testing.T) {
	state := filepath.Join(t.TempDir(), "nested", "state")
	if _, err := Open(state); err != nil {
		t.Fatalf("create missing state directory: %v", err)
	}
	linkedState := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(linkedState, "gator")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(linkedState); err == nil {
		t.Fatal("symlinked Gator state root accepted")
	}
}
