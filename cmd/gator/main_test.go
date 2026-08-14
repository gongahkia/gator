package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"help"}, &output); err != nil {
		t.Fatalf("run help: %v", err)
	}
	if !strings.Contains(output.String(), "Usage:") {
		t.Fatalf("help output = %q, want usage", output.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"ship"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run error = %v, want unknown-command error", err)
	}
}
