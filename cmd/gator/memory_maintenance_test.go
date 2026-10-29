package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/learning"
)

func TestIdleMemoryMaintenanceScheduleRespectsCurrentOptOut(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	settingsStore, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := settingsStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	off := false
	settings.Desktop.LearnFromChats = &off
	if err := settingsStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	store, _ := learning.Open(state)
	for i := 0; i < 3; i++ {
		_, err := store.RecordObservation(learning.ObservationInput{WorkID: fmt.Sprintf("work-idle-%d", i), Signal: learning.VerificationFailed, EvidenceRefs: []string{fmt.Sprintf("gator/work-history/work-idle-%d.json", i)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if ran, err := runDueMemoryMaintenance(context.Background(), state, now); err != nil || ran {
		t.Fatal("opt-out ignored", ran, err)
	}
	if _, err := store.MaintenanceStatus(); !os.IsNotExist(err) {
		t.Fatal("opt-out wrote maintenance state", err)
	}
	on := true
	settings.Desktop.LearnFromChats = &on
	if err := settingsStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	if ran, err := runDueMemoryMaintenance(context.Background(), state, now); err != nil || !ran {
		t.Fatal(ran, err)
	}
	proposals, err := store.List()
	if err != nil || len(proposals) != 1 || proposals[0].Status != learning.Candidate {
		t.Fatal("idle evidence not proposed", proposals, err)
	}
	if ran, err := runDueMemoryMaintenance(context.Background(), state, now.Add(time.Minute)); err != nil || ran {
		t.Fatal("ran before due", ran, err)
	}
	if ran, err := runDueMemoryMaintenance(context.Background(), state, now.Add(2*time.Hour)); err != nil || !ran {
		t.Fatal("idle pass did not repeat", ran, err)
	}
	settings.Desktop.LearnFromChats = &off
	if err := settingsStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	if ran, err := runDueMemoryMaintenance(context.Background(), state, now.Add(3*time.Hour)); err != nil || ran {
		t.Fatal("did not reload privacy setting", ran, err)
	}
}
