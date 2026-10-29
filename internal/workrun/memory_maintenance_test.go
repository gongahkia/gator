package workrun

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/learning"
)

func TestWorkFinalizationProposesRepeatedFailuresOnlyWhenEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			state, source := t.TempDir(), t.TempDir()
			for i := 0; i < 3; i++ {
				model := &scriptedModel{turns: []agent.Turn{{Text: "Done without the required artifact."}}}
				_, err := (Executor{Model: model, StateDir: state}).Execute(context.Background(), Request{SourcePath: source, Objective: "Write a report", RunID: fmt.Sprintf("work-maintain-%d", i), Contract: artifact.DefaultContract("report.md"), MaxSteps: 1, MaintainMemory: enabled, ForegroundApp: i == 0})
				if err == nil {
					t.Fatal("fixture should retain failed verification")
				}
				if len(model.requests) != 1 {
					t.Fatal("maintenance made extra model requests")
				}
			}
			store, err := learning.Open(state)
			if err != nil {
				t.Fatal(err)
			}
			records, err := store.List()
			if err != nil {
				t.Fatal(err)
			}
			if !enabled {
				if len(records) != 0 {
					t.Fatal("disabled maintenance created a lesson")
				}
				if _, err := store.MaintenanceStatus(); !os.IsNotExist(err) {
					t.Fatal("disabled maintenance ran", err)
				}
				return
			}
			if len(records) != 1 || records[0].Status != learning.Candidate || len(records[0].Provenance.WorkIDs) != 3 || records[0].Scope.Kind != learning.Project || records[0].Scope.Value != source {
				t.Fatal("missing scoped independent proposal", records)
			}
			if active, err := store.Projection(learning.Context{Project: source}); err != nil || len(active) != 0 {
				t.Fatal("unapproved proposal used", active, err)
			}
			if _, err := store.MaintenanceStatus(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
