package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
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

func TestJobCommandBindsFollowupsToAnExistingChatAndItsSource(t *testing.T) {
	stateDir, source := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	sessions, err := worksession.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := sessions.CreateChat(source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	arguments := []string{"add", "chat check", "--schedule", "* * * * *", "--mode", "inspect", "--conversation", chat.ID}
	if err := jobCommand(append(arguments, "--", "Inspect this chat's folder"), &output); err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := store.List()
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions = %+v, %v", definitions, err)
	}
	definition := definitions[0]
	if definition.OriginConversationID != chat.ID || definition.SourcePath != chat.SourcePath || !strings.Contains(output.String(), chat.ID) {
		t.Fatalf("lost explicit chat/source: %+v, %s", definition, output.String())
	}
	entry, err := recordJobInbox(stateDir, definition, jobs.Attempt{ID: "attempt-cli", Conversation: "execution-chat", Status: "completed", Summary: "Completed check"})
	if err != nil || entry.OriginConversationID != chat.ID || entry.ConversationID != "execution-chat" {
		t.Fatalf("update routing = %+v, %v", entry, err)
	}
	for _, args := range [][]string{
		append(append([]string{}, arguments...), "--source", t.TempDir(), "--", "Wrong source"),
		{"add", "missing", "--schedule", "* * * * *", "--conversation", "work-missing", "--", "Missing chat"},
	} {
		if err := jobCommand(args, &bytes.Buffer{}); err == nil {
			t.Fatal("invalid chat/source was accepted")
		}
	}
	definitions, err = store.List()
	if err != nil || len(definitions) != 1 {
		t.Fatalf("rejected command created a job: %+v, %v", definitions, err)
	}
	if err := jobCommand([]string{"edit", definition.ID, "--source", t.TempDir()}, &bytes.Buffer{}); err == nil {
		t.Fatal("source edit detached a chat-linked check")
	}
	if err := jobCommand([]string{"edit", definition.ID, "--name", "renamed check"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Load(definition.ID)
	if err != nil || updated.OriginConversationID != chat.ID || updated.SourcePath != chat.SourcePath {
		t.Fatalf("edit lost routing: %+v, %v", updated, err)
	}
}

func TestJobEditRecapturesExplicitFrozenSource(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	definition, err := store.Save(jobs.Definition{Name: "frozen", Enabled: true, Schedule: "* * * * *", SourcePath: source, Objective: "Inspect", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	original := definition.SnapshotID
	if err := os.WriteFile(filepath.Join(source, "new.txt"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := jobCommand([]string{"edit", definition.ID, "--name", "renamed"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.Load(definition.ID)
	if err != nil || unchanged.SnapshotID != original {
		t.Fatalf("unrelated edit changed capture: %+v, %v", unchanged, err)
	}
	for _, selected := range []string{source, t.TempDir()} {
		if err := jobCommand([]string{"edit", definition.ID, "--source", selected}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		canonicalSelected, err := filepath.EvalSymlinks(selected)
		if err != nil {
			t.Fatal(err)
		}
		updated, err := store.Load(definition.ID)
		if err != nil || updated.SnapshotID == original || updated.SourcePath != selected || updated.Project == nil || updated.Project.Origin != canonicalSelected {
			t.Fatalf("explicit source edit did not recapture source/configuration: %+v, %v", updated, err)
		}
		original = updated.SnapshotID
	}
}

func TestJobWorkUsesCanonicalWorkHistory(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	definition := jobs.Definition{SourcePath: source, Objective: "Inspect scheduled work", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 1}
	service := workrun.Service{Executor: workrun.Executor{Model: &workScriptedModel{turns: []agent.Turn{{Text: "Inspected."}}}, StateDir: state}}
	result, err := runJobProcessWith(context.Background(), definition, state, func(ctx context.Context, _, _, _ string, request workrun.Request) (workrun.Outcome, error) {
		return service.Execute(ctx, request)
	}, "job-work-history")
	if err != nil {
		t.Fatal(err)
	}
	store, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load("job-work-history")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != workhistory.Completed || record.ConversationID != result.ConversationID || record.RevisionID != result.RevisionID || record.Evidence.ArtifactManifestPath != result.ManifestPath {
		t.Fatalf("job Work history = %#v; result = %#v", record, result)
	}
}

func TestScheduledInboxIsQuietForRepeatedResult(t *testing.T) {
	state := t.TempDir()
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(jobs.Definition{Name: "daily brief", Enabled: true, Schedule: "0 9 * * *", SourcePath: t.TempDir(), Objective: "Check the brief", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	complete := func(summary, digest, status string) string {
		t.Helper()
		attempt, err := store.Begin(definition, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		attempt.Status, attempt.Summary, attempt.ResultSHA256 = status, summary, digest
		attempt.NotificationPolicySHA256 = jobNotificationPolicyHash(definition)
		attempt.FinishedAt = time.Now().UTC()
		if status == "failed" {
			attempt.Error = "provider unavailable"
		}
		if err := store.Record(attempt); err != nil {
			t.Fatal(err)
		}
		entry, err := recordJobInbox(state, definition, attempt)
		if err != nil {
			t.Fatal(err)
		}
		return entry.ID
	}
	if id := complete("No change in the brief", "same", "completed"); id == "" {
		t.Fatal("first result was suppressed")
	}
	if id := complete("No change in the brief", "same", "completed"); id != "" {
		t.Fatal("identical result created a new inbox item")
	}
	if id := complete("The deadline moved", "changed", "completed"); id == "" {
		t.Fatal("changed result was suppressed")
	}
	if id := complete("", "", "failed"); id == "" {
		t.Fatal("failure was suppressed")
	}
	if id := complete("The deadline moved", "changed", "completed"); id == "" {
		t.Fatal("recovery after failure was suppressed")
	}
	mailbox, err := inbox.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := mailbox.List(false, 20)
	if err != nil || len(entries) != 4 || entries[0].Summary != "The deadline moved" {
		t.Fatalf("entries = %#v, %v", entries, err)
	}
}

func TestJobResultDigestIncludesArtifactContent(t *testing.T) {
	a := digestJobResult("No change", []artifact.File{{Path: "brief.md", SHA256: "one"}})
	b := digestJobResult("No change", []artifact.File{{Path: "brief.md", SHA256: "two"}})
	if a == b || summarizeJobResult("\n The deadline moved.\nMore detail", nil) != "The deadline moved." {
		t.Fatal("result digest or summary missed material output")
	}
}
