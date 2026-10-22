package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func writeFixture(t *testing.T, root, path, text string) {
	t.Helper()
	p := filepath.Join(root, "gator", path)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRetentionReviewRejectsConcurrentPolicyChanges(t *testing.T) {
	s, _ := Open(t.TempDir())
	if err := s.SavePolicy(Policy{HistoryDays: 30}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveReviewedPolicy(Policy{HistoryDays: 7}, Policy{}); err == nil {
		t.Fatal("stale retention policy overwrote a newer edit")
	}
	p, err := s.Policy()
	if err != nil || p.HistoryDays != 30 {
		t.Fatal("policy changed after rejection", err)
	}
}

func TestBackupRejectsOversizedFilesAndCorruptArchive(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	writeFixture(t, root, "memory/large.json", "")
	f, err := os.OpenFile(filepath.Join(root, "gator/memory/large.json"), os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate((1 << 30) + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	destination := filepath.Join(t.TempDir(), "backup.tar.gz")
	if _, err = s.Export(context.Background(), destination); err == nil {
		t.Fatal("created an unverifiable oversized backup")
	}
	writeFixture(t, root, "memory/large.json", `{"text":"saved"}`)
	if _, err = s.Export(context.Background(), destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-8] ^= 1 // Corrupt gzip's CRC without changing the tar contents.
	if err = os.WriteFile(destination, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyBackup(context.Background(), destination); err == nil {
		t.Fatal("corrupt backup verified")
	}
}
func TestReviewRejectsChangedContentAndPreservesOriginals(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	writeFixture(t, root, "memory/item.json", `{"text":"first"}`)
	source := filepath.Join(t.TempDir(), "original.md")
	if err := os.WriteFile(source, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := s.PreviewDelete(context.Background(), "knowledge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "gator/memory/item.json")); err != nil {
		t.Fatal("preview deleted data")
	}
	info, _ := os.Stat(filepath.Join(root, "gator/memory/item.json"))
	writeFixture(t, root, "memory/item.json", `{"text":"other"}`)
	_ = os.Chtimes(filepath.Join(root, "gator/memory/item.json"), info.ModTime(), info.ModTime())
	if _, err = s.Apply(context.Background(), p.ID); err == nil {
		t.Fatal("changed content was deleted using a stale review")
	}
	p, err = s.PreviewDelete(context.Background(), "knowledge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != "keep" {
		t.Fatal("original source was changed")
	}
}
func TestMaintenanceRefusesActiveWorkAndDoesNotFollowLinks(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	writeFixture(t, root, "memory/item.json", `{}`)
	release, err := state.Guard(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PreviewDelete(context.Background(), "all"); err == nil {
		t.Fatal("maintenance raced an active writer")
	}
	release()
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(root, "gator/memory/link")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PreviewDelete(context.Background(), "all"); err == nil {
		t.Fatal("maintenance accepted a source symlink")
	}
}

func TestDataRootSymlinkCannotCreateControlFilesOutsideState(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "gator")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root); err == nil {
		t.Fatal("accepted a symbolic private root")
	}
	if _, err := os.Stat(filepath.Join(outside, "data-control")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created control data outside the private root")
	}
}
func TestPrivateBackupVerifiesAndExcludesCredentials(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	writeFixture(t, root, "memory/item.json", `{"text":"private preference"}`)
	writeFixture(t, root, "auth.json", `{"key":"secret-do-not-export"}`)
	writeFixture(t, root, "jobs/supervisor.token", "runtime-secret")
	output := filepath.Join(t.TempDir(), "backup.tar.gz")
	m, err := s.Export(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 1 || m.Files["gator/auth.json"] != "" || m.Files["gator/jobs/supervisor.token"] != "" {
		t.Fatalf("unexpected archive scope: %#v", m.Files)
	}
	verified, err := VerifyBackup(context.Background(), output)
	if err != nil || len(verified.Files) != 1 {
		t.Fatalf("verify: %v", err)
	}
	info, _ := os.Stat(output)
	if info.Mode().Perm() != 0600 {
		t.Fatal("backup is not private")
	}
	if _, err = s.Export(context.Background(), output); err == nil {
		t.Fatal("backup overwrote an existing file")
	}
	p, err := s.PreviewDelete(context.Background(), "all")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "gator/auth.json")); err != nil {
		t.Fatal("content deletion removed credentials")
	}
	if _, err = VerifyBackup(context.Background(), output); err != nil {
		t.Fatal("content deletion removed the external backup")
	}
	if _, err = s.PreviewDelete(context.Background(), "knowledge"); err != nil {
		t.Fatal("deletion left the open data store unusable", err)
	}
}
func TestSnapshotCleanupPreservesReferencedSharedContent(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "project.md"), []byte("source truth"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	one, err := snapshot.Create(source, root, snapshot.Options{Now: func() time.Time { return now.Add(-40 * 24 * time.Hour) }})
	if err != nil {
		t.Fatal(err)
	}
	two, err := snapshot.Create(source, root, snapshot.Options{Now: func() time.Time { return now.Add(-39 * 24 * time.Hour) }})
	if err != nil {
		t.Fatal(err)
	}
	history, _ := workhistory.Open(root)
	if _, err = history.Start(workhistory.Start{ID: "work-protected", Objective: "Retain evidence", Mode: action.Inspect, ExternalActions: action.Forbid, StartedAt: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err = history.Finish("work-protected", workhistory.Finish{Succeeded: true, SnapshotID: one.ID, FinishedAt: now, VerificationStatus: workhistory.VerificationUnavailable}); err != nil {
		t.Fatal(err)
	}
	s, _ := Open(root)
	p, err := s.PreviewDelete(context.Background(), "snapshots")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range p.Items {
		if strings.Contains(item.Path, one.ID) || strings.Contains(item.Path, "trees/"+one.TreeID) || strings.Contains(item.Path, "blobs/") {
			t.Fatalf("selected shared evidence: %s", item.Path)
		}
	}
	if _, err = s.Apply(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = snapshot.Open(root, one.ID); err != nil {
		t.Fatal("referenced snapshot removed", err)
	}
	if one.ID != two.ID {
		if _, err = snapshot.Open(root, two.ID); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("unused snapshot retained", err)
		}
	}
}
func TestRetentionDefaultsKeepDataAndProtectOngoingGoals(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	s, _ := Open(root)
	memories, _ := memory.Open(root)
	global, _ := memories.Add("preference", "Old global preference", "", "user", time.Time{})
	goals, _ := goal.Open(root)
	g, err := goals.Add("Ongoing project", "", source)
	if err != nil {
		t.Fatal(err)
	}
	active, _ := memories.Add("preference", "Current goal preference", g.ID, "user", time.Time{})
	now := time.Now().UTC()
	for _, m := range []memory.Record{global, active} {
		m.UpdatedAt = now.Add(-40 * 24 * time.Hour)
		m.CreatedAt = m.UpdatedAt
		data, _ := json.Marshal(m)
		writeFixture(t, root, "memory/"+m.ID+".json", string(data))
	}
	if _, err := Maintain(context.Background(), root, now); err != nil {
		t.Fatal(err)
	}
	if _, err = memories.Load(global.ID); err != nil {
		t.Fatal("default retention could not load knowledge", err)
	}
	if err = s.SavePolicy(Policy{KnowledgeDays: 30}); err != nil {
		t.Fatal(err)
	}
	result, err := Maintain(context.Background(), root, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed == 0 {
		t.Fatal("saved retention policy did not apply")
	}
	if _, err = memories.Load(global.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expired global knowledge retained", err)
	}
	if _, err = memories.Load(active.ID); err != nil {
		t.Fatal("ongoing goal knowledge removed", err)
	}
}

func TestFileVersionsFollowBackupAndExplicitHistoryDeletion(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"files/identity/file.json", "files/identity/version.bin", "files/identity/draft.json", "files/alias/link.json", "files/identity/saving.lock", "files/identity/interrupted.tmp"} {
		writeFixture(t, root, path, "retained file content")
	}
	output := filepath.Join(t.TempDir(), "backup.tar.gz")
	manifest, err := s.Export(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 4 {
		t.Fatalf("backup should retain versions and drafts, excluding transient locks: %#v", manifest.Files)
	}
	if _, err := VerifyBackup(context.Background(), output); err != nil {
		t.Fatal(err)
	}
	plan, err := s.PreviewDelete(context.Background(), "history")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), plan.ID); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "gator/files")); err != nil || len(entries) != 0 {
		t.Fatal("history deletion retained private working files", err)
	}
}

func TestRetentionPreservesSelectedUnsentChatAndBackupMemorySuppression(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	sessions, _ := worksession.Open(root)
	old := time.Now().Add(-60 * 24 * time.Hour)
	active, _ := sessions.CreateChat(t.TempDir(), old)
	expired, _ := sessions.CreateChat(t.TempDir(), old)
	if err := sessions.SetActive(active.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePolicy(Policy{HistoryDays: 30}); err != nil {
		t.Fatal(err)
	}
	if _, err := Maintain(context.Background(), root, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Load(active.ID); err != nil {
		t.Fatal("unsent active chat lost", err)
	}
	if _, err := sessions.Load(expired.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expired unselected chat retained", err)
	}
	memories, _ := memory.Open(root)
	record, err := memories.RememberChat(memory.ChatInput{Subject: "profile.name", Kind: "fact", Quote: "My name is Sam.", ConversationID: active.ID, RunID: "run-first"})
	if err != nil {
		t.Fatal(err)
	}
	if err = memories.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	manifest, err := s.Export(context.Background(), filepath.Join(t.TempDir(), "backup.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for file := range manifest.Files {
		if strings.Contains(file, ".write-lock") {
			t.Fatal("transient lock in backup")
		}
		found = found || strings.Contains(file, ".forgotten/")
	}
	if !found {
		t.Fatal("backup lost forgotten-subject suppression")
	}
}
