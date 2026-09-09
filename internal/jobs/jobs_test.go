package jobs

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
)

func TestStoreAndScheduleDue(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(Definition{Name: "Daily brief", Enabled: true, Schedule: "0 9 * * *", Timezone: "Asia/Singapore", SourcePath: "/source", Objective: "Write brief", Mode: action.Draft, Contract: artifact.DefaultContract("brief.md")})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 9, 1, 0, 10, 0, time.UTC)
	due, ok, err := Due(definition, now)
	if err != nil || !ok || !due.Equal(now.Truncate(time.Minute)) {
		t.Fatalf("Due = %v %v %v", due, ok, err)
	}
	if _, claimed, err := store.Claim(definition.ID, due); err != nil || !claimed {
		t.Fatalf("Claim = %v %v", claimed, err)
	}
	loaded, _ := store.Load(definition.ID)
	if _, ok, _ := Due(loaded, now); ok {
		t.Fatal("job was due twice in one minute")
	}
}

func TestJobsCannotApproveActions(t *testing.T) {
	contract := artifact.DefaultContract("brief.md")
	contract.ExternalActions = action.Approve
	definition := Definition{Version: 1, ID: "job-test", Name: "unsafe", Schedule: "* * * * *", Timezone: "UTC", SourcePath: "/source", Objective: "send", Mode: action.Draft, Contract: contract, MaxSteps: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := definition.Validate(); err == nil {
		t.Fatal("approved scheduled action accepted")
	}
}
