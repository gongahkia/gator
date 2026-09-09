package jobs

import (
	"os"
	"path/filepath"
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

func TestAttemptPersistsImmutableIntentEventsAndResult(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(Definition{Name: "Daily brief", Enabled: true, Schedule: "0 9 * * *", Timezone: "UTC", SourcePath: "/source", Objective: "Write brief", Mode: action.Draft, Contract: artifact.DefaultContract("brief.md")})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Begin(definition, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	running, err := store.History(definition.ID, 10)
	if err != nil || len(running) != 1 || running[0].Status != "running" {
		t.Fatalf("running history = %#v, %v", running, err)
	}
	attempt.Status = "completed"
	attempt.FinishedAt = attempt.StartedAt.Add(time.Second)
	if err := store.Record(attempt); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(store.Root(), "history", definition.ID, attempt.ID)
	for _, name := range []string{"intent.json", "events.jsonl", "result.json"} {
		if info, err := os.Stat(filepath.Join(directory, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("%s not persisted: %v", name, err)
		}
	}
	if err := store.Record(attempt); !os.IsExist(err) {
		t.Fatalf("second terminal publication error = %v, want exists", err)
	}
	history, err := store.History(definition.ID, 10)
	if err != nil || len(history) != 1 || history[0].DefinitionSHA256 == "" {
		t.Fatalf("History = %#v, %v", history, err)
	}
	if err := store.Remove(definition.ID); err != nil {
		t.Fatal(err)
	}
	history, err = store.History(definition.ID, 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("history after definition removal = %#v, %v", history, err)
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

func TestRunOnceCatchesScheduleMissedSinceCreation(t *testing.T) {
	created := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	definition := Definition{Version: 1, ID: "job-catchup", Name: "Catch up", Enabled: true, Schedule: "0 1 * * *", Timezone: "UTC", Missed: "run_once", SourcePath: "/source", Objective: "Write", Mode: action.Draft, Contract: artifact.DefaultContract("brief.md"), MaxSteps: 1, CreatedAt: created, UpdatedAt: created}
	due, ok, err := Due(definition, time.Date(2026, 9, 9, 3, 0, 0, 0, time.UTC))
	if err != nil || !ok || !due.Equal(time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("Due = %v, %v, %v", due, ok, err)
	}
}
