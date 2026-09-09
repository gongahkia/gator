package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestJobCommandCreatesDurableInspectSchedule(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	if err := jobCommand([]string{"add", "daily", "--schedule", "0 9 * * *", "--timezone", "Asia/Singapore", "--mode", "inspect", "--", "Summarize the folder"}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := jobCommand([]string{"list"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "daily") || !strings.Contains(output.String(), "Asia/Singapore") {
		t.Fatalf("list = %q", output.String())
	}
}

func TestJobCommandRejectsUnattendedApproval(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	err := jobCommand([]string{"add", "unsafe", "--schedule", "* * * * *", "--actions", "approve", "--", "Send this"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("scheduled approval was accepted")
	}
}
