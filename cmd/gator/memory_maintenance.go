package main

import (
	"context"
	"os"
	"time"

	"github.com/gongahkia/gator/internal/learning"
)

// The existing supervisor supplies idle maintenance; no model, tool action or
// new always-on process is started. Re-read the current privacy setting on each
// tick, and retain progress so restarts do not repeat the first page forever.
func runDueMemoryMaintenance(ctx context.Context, stateDir string, now time.Time) (bool, error) {
	settings, err := loadSettings()
	if err != nil {
		return false, err
	}
	if !settings.Desktop.LearnsFromChats() {
		return false, nil
	}
	store, err := learning.Open(stateDir)
	if err != nil {
		return false, err
	}
	previous, err := store.MaintenanceStatus()
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	interval := time.Hour
	if previous.Truncated && previous.Error == "" {
		interval = time.Minute
	}
	if !previous.FinishedAt.IsZero() && !previous.FinishedAt.After(now.Add(24*time.Hour)) && now.Before(previous.FinishedAt.Add(interval)) {
		return false, nil
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err = store.Maintain(bounded)
	return true, err
}
