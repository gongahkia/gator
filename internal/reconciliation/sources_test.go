package reconciliation

import (
	"strconv"
	"testing"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/knowledge"
)

func TestConflictSourcesRequireCurrentScopedExactEvidence(t *testing.T) {
	state := t.TempDir()
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	g, err := goals.Add("Workshop", "", "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := goals.InitializePlan(g.ID, "Workshop on November 20", []string{"Prepare"})
	if err != nil {
		t.Fatal(err)
	}
	note, err := notes.AddNote("Workshop", "Confirmed date is November 27", g.ID)
	if err != nil {
		t.Fatal(err)
	}
	refs := []Source{{Kind: "note", ID: note.ID, Revision: note.Revision(), Quote: "November 27"}, {Kind: "plan", ID: g.ID, Revision: strconv.Itoa(plan.Revision), Quote: "November 20"}}
	if err := Check(goals, notes, refs, g.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := Check(goals, notes, refs, "", true, nil); err == nil {
		t.Fatal("scoped note leaked to main chat")
	}
	if err := Check(goals, notes, refs, g.ID, false, []string{note.ID}); err == nil {
		t.Fatal("excluded note accepted")
	}
	invalid := append([]Source(nil), refs...)
	invalid[0].Quote = "November 99"
	if err := Check(goals, notes, invalid, g.ID, false, nil); err == nil {
		t.Fatal("invented quote accepted")
	}
	if _, err := notes.UpdateNote(note.ID, note.Title, "Corrected date is November 20"); err != nil {
		t.Fatal(err)
	}
	if err := Check(goals, notes, refs, g.ID, false, nil); err == nil {
		t.Fatal("stale evidence accepted")
	}
}
