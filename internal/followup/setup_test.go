package followup

import (
	"testing"

	"github.com/gongahkia/gator/internal/action"
)

func TestReviewedDraftAllowsQuietChecksWithoutGrantingActions(t *testing.T) {
	draft, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := draft.Definition("goal-aaaaaaaaaaaaaaaaaaaaaaaa", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !definition.Contract.AllowOptionalArtifacts || definition.Contract.ExternalActions != action.Forbid {
		t.Fatal("optional draft changed external authority")
	}
	draft.Mode = action.Act
	if err := draft.Validate(); err == nil {
		t.Fatal("unattended act was permitted")
	}
	draft.Mode = action.Inspect
	definition, err = draft.Definition("goal-aaaaaaaaaaaaaaaaaaaaaaaa", nil)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Contract.AllowOptionalArtifacts {
		t.Fatal("inspect could create optional artifacts")
	}
	draft.Enabled = false
	definition, err = draft.Definition("goal-aaaaaaaaaaaaaaaaaaaaaaaa", nil)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Enabled {
		t.Fatal("editing a paused agreement resumed it")
	}
}
