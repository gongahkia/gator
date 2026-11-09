package main

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/state"
)

type maintenanceNotice chan string

func (notice maintenanceNotice) Write(data []byte) (int, error) {
	notice <- string(data)
	return len(data), nil
}

func TestUIMaintenanceLeaseDefersBusyProfilesAndHoldsUntilHostFinishes(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", directory)
	t.Setenv("GATOR_UI_BACKEND", "1")
	release, err := state.Guard(directory, false)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := uiMaintenanceLease(bytes.NewReader(nil), &out); err != nil || out.String() != "{\"ready\":false}\n" {
		t.Fatal("busy profile was not deferred", out.String(), err)
	}
	release()
	input, host := io.Pipe()
	notice := make(maintenanceNotice, 1)
	done := make(chan error, 1)
	go func() { done <- uiMaintenanceLease(input, notice) }()
	select {
	case value := <-notice:
		if value != "{\"ready\":true}\n" {
			t.Fatal(value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("maintenance did not acquire ownership")
	}
	if release, err := state.Guard(directory, true); !errors.Is(err, state.ErrBusy) {
		if release != nil {
			release()
		}
		t.Fatal("lease ended before scan/prune finished", err)
	}
	host.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("maintenance did not release at pipe close")
	}
	release, err = state.Guard(directory, true)
	if err != nil {
		t.Fatal("profile still held after cleanup", err)
	}
	release()
}
