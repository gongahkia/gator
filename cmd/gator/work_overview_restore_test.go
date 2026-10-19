package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/lifecycle"
	"github.com/gongahkia/gator/internal/memory"
)

func TestOverviewControlLinksToExactUnreadResultWithoutReadingIt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	store, _ := inbox.Open(root)
	entry, err := store.Add(inbox.Entry{Kind: "job", Title: "Deadline changed", Summary: "Review the new deadline.", Status: "completed", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	screen, err := overviewControl(context.Background(), root, "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(screen.Actions) < 4 || screen.Actions[0].Label != "What changed" || screen.Actions[3].Label != "What is blocked" {
		t.Fatal("four questions missing")
	}
	screen, err = overviewControl(context.Background(), root, "section", []string{"needs"})
	if err != nil {
		t.Fatal(err)
	}
	var target []string
	for _, a := range screen.Actions {
		if strings.Contains(a.Label, "Deadline changed") {
			target = a.Args
		}
	}
	if len(target) != 2 {
		t.Fatal("result has no typed navigation target")
	}
	screen, err = overviewControl(context.Background(), root, "item", target)
	if err != nil {
		t.Fatal(err)
	}
	if len(screen.Actions) == 0 || screen.Actions[0].Command != "/inbox show "+entry.ID {
		t.Fatal("result opens a generic destination")
	}
	entries, err := store.List(false, 0)
	if err != nil || len(entries) != 1 || !entries[0].ReadAt.IsZero() {
		t.Fatal("overview read or changed the result")
	}
}
func TestDataRestoreCLIAndControlsRequireTheSavedFinalReview(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	memories, _ := memory.Open(root)
	if _, err := memories.Add("preference", "Use a short summary", "", "user", time.Time{}); err != nil {
		t.Fatal(err)
	}
	s, _ := lifecycle.Open(root)
	archive := filepath.Join(t.TempDir(), "backup.tar.gz")
	if _, err := s.Export(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "fresh-profile")
	var output bytes.Buffer
	if err := run([]string{"data", "restore", archive, "--to", destination, "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var plan lifecycle.RestorePlan
	if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("CLI preview restored data")
	}
	if err := run([]string{"data", "restore-apply", plan.ID}, &output); err == nil {
		t.Fatal("restore without explicit confirmation accepted")
	}
	output.Reset()
	if err := run([]string{"data", "restore-apply", plan.ID, "--yes"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "GATOR_CONFIG_DIR=") {
		t.Fatal("launch command lost the fresh profile configuration")
	}
	// Restoring from /data creates another profile and leaves this session bound
	// to its original state and conversation.
	second := filepath.Join(t.TempDir(), "second-profile")
	screen, err := dataControl(context.Background(), root, "restore-plan", []string{archive, second, ""})
	if err != nil {
		t.Fatal(err)
	}
	if screen.Panel.Kind != "confirmation" || len(screen.Actions) != 1 {
		t.Fatal("no final restore agreement")
	}
	if _, err = os.Stat(second); !os.IsNotExist(err) {
		t.Fatal("form review restored data")
	}
	screen, err = dataControl(context.Background(), root, "restore-apply", screen.Actions[0].Args)
	if err != nil {
		t.Fatal(err)
	}
	if screen.ResetConversation || screen.ClearGoal {
		t.Fatal("restore changed the active profile")
	}
	if _, err = os.Stat(filepath.Join(root, "gator", "memory")); err != nil {
		t.Fatal("source profile was removed")
	}
}
func TestProfileLaunchQuotesShellMetacharactersLiterally(t *testing.T) {
	command := profileLaunch("/tmp/project 'literal' $(do-not-run)")
	if !strings.Contains(command, "'\"'\"'") || strings.Contains(command, `GATOR_STATE_DIR="/tmp`) {
		t.Fatal("unsafe shell quoting", command)
	}
}
