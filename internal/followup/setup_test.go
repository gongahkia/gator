package followup

import (
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
)

func TestEndDateIsExplicitAndRetainedInReview(t *testing.T) {
	d, _ := New(t.TempDir(), "")
	d.EndsAt = "Friday"
	if err := d.Validate(); err == nil {
		t.Fatal("ambiguous date accepted")
	}
	d.EndsAt = "2030-10-25T17:00:00+08:00"
	definition, err := d.Definition("goal-aaaaaaaaaaaaaaaaaaaaaaaa", nil)
	if err != nil || definition.EndsAt.IsZero() {
		t.Fatalf("lost expiry: %v", err)
	}
	if !strings.Contains(strings.Join(d.ReviewLines(), "\n"), "+08:00") {
		t.Fatal("timezone missing from review")
	}
}

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

func TestCalendarWindowIsRetiredButLocalFollowupWorks(t *testing.T) {
	draft, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	draft.CalendarDays = 14
	if err := draft.Validate(); err == nil || !strings.Contains(err.Error(), "support was removed") {
		t.Fatalf("calendar draft = %v", err)
	}
	draft.CalendarDays = 0
	if err := draft.Validate(); err != nil {
		t.Fatalf("local-only draft = %v", err)
	}
}
