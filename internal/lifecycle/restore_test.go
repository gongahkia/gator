package lifecycle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

type restoreModel struct{}

func TestRestorePreservesForgottenConceptAndSourceChatSuppression(t *testing.T) {
	root := t.TempDir()
	memories, err := memory.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	input := memory.ChatInput{Subject: "profile.home_city", Kind: "fact", Quote: "I live in Harbor City.", ConversationID: "work-source", RunID: "run-source"}
	record, err := memories.RememberChat(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := memories.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "backup.tar.gz")
	if _, err := store.Export(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	plan, err := store.PreviewRestore(context.Background(), archive, filepath.Join(t.TempDir(), "restored"), nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.Restore(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := memory.Open(receipt.Plan.Target)
	if err != nil {
		t.Fatal(err)
	}
	input.Subject = "profile.city"
	if _, err := restored.RememberChat(input); err == nil {
		t.Fatal("restore lost canonical forget marker")
	}
	if blocked, err := restored.HistorySuppressed(input.ConversationID, ""); err != nil || !blocked {
		t.Fatal("restore lost source-history marker", err)
	}
}

func (restoreModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	return agent.Turn{Text: "The decision is due Friday. Original paths in this transcript are historical."}, nil
}

type restoreDraftModel struct{ calls int }

func (m *restoreDraftModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	if m.calls == 1 {
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "write-draft", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"decision.md","content":"# Decision\nChoose by Friday.\n"}`)}}}, nil
	}
	return agent.Turn{Text: "Prepared the decision draft for review."}, nil
}
func restoreFixture(t *testing.T) (Store, string, string, workrun.Outcome, string) {
	t.Helper()
	root, source := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "notes.md"), []byte("Choose by Friday."), 0600); err != nil {
		t.Fatal(err)
	}
	outcome, err := (workrun.Executor{StateDir: root, Model: &restoreDraftModel{}}).Execute(context.Background(), workrun.Request{RunID: "restore-completed", SourcePath: source, Objective: "Draft the decision summary", Mode: action.Draft, Contract: artifact.DefaultContract("decision.md")})
	if err != nil {
		t.Fatal(err)
	}
	goals, _ := goal.Open(root)
	g, err := goals.Add("Restored project", "Keep the original decision", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = goals.AttachConversation(g.ID, outcome.ConversationID); err != nil {
		t.Fatal(err)
	}
	_, _ = goals.InitializePlan(g.ID, "Make the decision", []string{"Review options"})
	memories, _ := memory.Open(root)
	if _, err = memories.Add("preference", "Use a short summary", g.ID, "user", time.Time{}); err != nil {
		t.Fatal(err)
	}
	notes, _ := knowledge.Open(root)
	if _, err = notes.AddNote("Decision", "Choose by Friday", g.ID); err != nil {
		t.Fatal(err)
	}
	store, _ := jobs.Open(root)
	if _, err = store.Save(jobs.Definition{Name: "Restored check", Enabled: true, Schedule: "0 9 * * *", Timezone: "UTC", Missed: "skip", SourcePath: source, RefreshSnapshot: true, Objective: "Inspect the decision", Mode: action.Inspect, Contract: artifact.InspectionContract(), GoalID: g.ID}); err != nil {
		t.Fatal(err)
	}
	history, _ := workhistory.Open(root)
	if _, err = history.Start(workhistory.Start{ID: "restore-interrupted", Objective: "Interrupted Work", Mode: action.Inspect, ExternalActions: action.Forbid, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "backup.tar.gz")
	if _, err = s.Export(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	return s, archive, source, outcome, g.ID
}
func TestRestoreMigratesUsableHistoryAndKnowledgeIntoFreshProfile(t *testing.T) {
	s, archive, source, outcome, goalID := restoreFixture(t)
	destination := filepath.Join(t.TempDir(), "fresh-profile")
	newSource := t.TempDir()
	newSource, _ = filepath.EvalSymlinks(newSource)
	if err := os.WriteFile(filepath.Join(newSource, "notes.md"), []byte("Choose by Monday."), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := s.PreviewRestore(context.Background(), archive, destination, []SourceMap{{source, newSource}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preview wrote to target")
	}
	if plan.PausedChecks != 1 || plan.InterruptedRuns != 1 {
		t.Fatalf("missing recovery disclosure: %#v", plan)
	}
	receipt, err := s.Restore(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	destination = receipt.Plan.Target
	goals, _ := goal.Open(destination)
	g, err := goals.Load(goalID)
	if err != nil || g.SourcePath != newSource || g.ConversationID != outcome.ConversationID {
		t.Fatalf("goal did not migrate: %#v %v", g, err)
	}
	sessions, _ := worksession.Open(destination)
	revision, err := sessions.LoadRevision(outcome.ConversationID, outcome.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(revision.BundlePath, destination+string(filepath.Separator)) {
		t.Fatal("bundle path still uses original state")
	}
	bundle, err := artifact.OpenBundle(revision.BundlePath)
	if err != nil {
		t.Fatal("migrated bundle cannot be verified", err)
	}
	original, err := artifact.OpenBundle(outcome.Work.Path)
	if err != nil {
		t.Fatal(err)
	}
	oldManifest, _ := json.Marshal(original.Manifest)
	newManifest, _ := json.Marshal(bundle.Manifest)
	if string(oldManifest) != string(newManifest) {
		t.Fatal("migration changed sealed artifact evidence")
	}
	data, err := bundle.Output.ReadRegularFile("decision.md", 1024)
	if err != nil || !strings.Contains(string(data), "Choose by Friday.") {
		t.Fatal("draft artifact content was lost", err)
	}
	if revision.Replay != nil && len(revision.Replay.Configuration) > 0 {
		t.Fatal("restored replay reactivated old execution configuration")
	}
	if revision.FinalText == "" {
		t.Fatal("transcript was lost")
	}
	history, _ := workhistory.Open(destination)
	record, err := history.Load("restore-completed")
	if err != nil || !strings.HasPrefix(record.Evidence.ArtifactManifestPath, destination) {
		t.Fatal("history evidence was not rebased", err)
	}
	interrupted, err := history.Load("restore-interrupted")
	if err != nil || interrupted.Status != workhistory.Failed || interrupted.LiveStatus == "active" {
		t.Fatal("restored interrupted work appears live", err)
	}
	jobsStore, _ := jobs.Open(destination)
	definitions, err := jobsStore.List()
	if err != nil || len(definitions) != 1 || definitions[0].Enabled || definitions[0].SourcePath != newSource {
		t.Fatalf("restored check unsafe: %#v %v", definitions, err)
	}
	ledger, _ := usage.Open(destination)
	policy, err := ledger.Policy()
	if err != nil || !policy.Paused {
		t.Fatal("new profile was not paused")
	}
	memories, _ := memory.Open(destination)
	saved, err := memories.List(goalID, false)
	if err != nil || len(saved) != 1 {
		t.Fatal("saved memory was lost", err)
	}
	// Continue the restored conversation with a new snapshot and local fake model.
	resumed, err := (workrun.Executor{StateDir: destination, Model: restoreModel{}}).Execute(context.Background(), workrun.Request{RunID: "after-restore", SourcePath: newSource, ConversationID: g.ConversationID, Objective: "Continue with current sources", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil || resumed.RevisionID == outcome.RevisionID {
		t.Fatal("migrated conversation cannot continue", err)
	}
	if resumed.SnapshotID == outcome.SnapshotID {
		t.Fatal("migration reused stale source content")
	}
	latest, _ := sessions.Load(g.ConversationID)
	if latest.SourceRefreshRequired {
		t.Fatal("fresh capture did not clear migration refresh requirement")
	}
	restoredStore, err := Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	nextArchive := filepath.Join(t.TempDir(), "next-generation.tar.gz")
	if _, err = restoredStore.Export(context.Background(), nextArchive); err != nil {
		t.Fatal("re-export restored profile", err)
	}
	nextPlan, err := restoredStore.PreviewRestore(context.Background(), nextArchive, filepath.Join(t.TempDir(), "second-profile"), nil)
	if err != nil {
		t.Fatal("preview second migration", err)
	}
	if _, err = restoredStore.Restore(context.Background(), nextPlan.ID); err != nil {
		t.Fatal("second migration", err)
	}
	if _, err = s.Restore(context.Background(), plan.ID); err == nil {
		t.Fatal("restore overwrote an existing profile")
	}
	if _, err = VerifyBackup(context.Background(), archive); err != nil {
		t.Fatal("restore changed archive", err)
	}
	if _, err = os.Stat(filepath.Join(source, "notes.md")); err != nil {
		t.Fatal("restore changed original source")
	}
}
func TestRestoreRejectsChangedArchiveOccupiedTargetsAndCancellation(t *testing.T) {
	s, archive, _, _, _ := restoreFixture(t)
	destination := filepath.Join(t.TempDir(), "new-profile")
	plan, err := s.PreviewRestore(context.Background(), archive, destination, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Restore(context.Background(), plan.ID); err == nil {
		t.Fatal("replaced an existing empty directory")
	}
	if err = os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(archive, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("changed"))
	file.Close()
	if _, err = s.Restore(context.Background(), plan.ID); err == nil {
		t.Fatal("changed archive accepted")
	}
	if _, err = os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed restore published target")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.PreviewRestore(ctx, archive, destination, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled restore continued", err)
	}
}
func hostileBackup(t *testing.T, entries map[string][]byte, extra *tar.Header) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "hostile.tar.gz")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	manifest := BackupManifest{Version: 1, StateDir: "/old-state", CreatedAt: time.Now(), Files: map[string]string{}}
	for name, data := range entries {
		if err = tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(data)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err = tw.Write(data); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		manifest.Files[name] = hex.EncodeToString(sum[:])
	}
	if extra != nil {
		if err = tw.WriteHeader(extra); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := json.Marshal(manifest)
	_ = tw.WriteHeader(&tar.Header{Name: "gator-backup.json", Size: int64(len(data)), Mode: 0600, Typeflag: tar.TypeReg})
	_, _ = tw.Write(data)
	tw.Close()
	gz.Close()
	file.Close()
	return filename
}
func TestRestoreRejectsTraversalLinksAndRuntimeCapabilities(t *testing.T) {
	s, _ := Open(t.TempDir())
	for _, header := range []*tar.Header{
		{Name: "../escaped", Typeflag: tar.TypeReg},
		{Name: "gator/memory/link", Typeflag: tar.TypeSymlink, Linkname: "/tmp"},
		{Name: "gator/memory/link", Typeflag: tar.TypeLink, Linkname: "gator/auth.json"},
		{Name: "gator/auth.json", Typeflag: tar.TypeReg},
		{Name: "gator/work-control/access.json", Typeflag: tar.TypeReg},
	} {
		t.Run(header.Name+strconv.Itoa(int(header.Typeflag)), func(t *testing.T) {
			archive := hostileBackup(t, map[string][]byte{"gator/memory/one.json": []byte("{}")}, header)
			target := filepath.Join(t.TempDir(), "fresh")
			if _, err := s.PreviewRestore(context.Background(), archive, target, nil); err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unsafe restore created a profile")
			}
		})
	}
}

func TestEmptyBackupCanBecomeAFreshProfile(t *testing.T) {
	s, _ := Open(t.TempDir())
	archive := filepath.Join(t.TempDir(), "empty.tar.gz")
	if _, err := s.Export(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	p, err := s.PreviewRestore(context.Background(), archive, filepath.Join(t.TempDir(), "fresh-empty"), nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Restore(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Files != 0 {
		t.Fatal("fixture is not an empty backup")
	}
	if _, err = os.Stat(filepath.Join(receipt.Plan.Target, "gator", "usage", "policy.json")); err != nil {
		t.Fatal("empty profile was not initialized")
	}
}

func TestProfilePublicationCannotReplaceAnExistingEmptyDirectory(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parentPath, "staged", "profile"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentPath, "staged", "profile", "content"), []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(parentPath, "destination"), 0700); err != nil {
		t.Fatal(err)
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err = publishProfile(parent, "staged/profile", "destination"); err == nil {
		t.Fatal("publication replaced an existing directory after preflight")
	}
	entries, err := os.ReadDir(filepath.Join(parentPath, "destination"))
	if err != nil || len(entries) != 0 {
		t.Fatal("publication changed the existing destination", err)
	}
}

func TestRestorePreservesUnsentChatWithoutSyntheticRevisions(t *testing.T) {
	ctx := context.Background()
	root, source := t.TempDir(), t.TempDir()
	// Restore canonicalizes existing source paths. macOS may return a /var
	// temporary path whose real directory is under /private/var.
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := worksession.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := sessions.CreateChat(source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "unsent.tar.gz")
	if _, err = store.Export(ctx, archive); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	plan, err := store.PreviewRestore(ctx, archive, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preview created destination")
	}
	if _, err = store.Restore(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := worksession.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.Load(chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.HeadRevision != "" || got.SourcePath != source {
		t.Fatalf("unsent identity changed: %#v", got)
	}
	revisions, err := restored.Revisions(chat.ID)
	if err != nil || len(revisions) != 0 {
		t.Fatalf("synthetic revisions: %#v %v", revisions, err)
	}
	original, err := sessions.Load(chat.ID)
	if err != nil || original.HeadRevision != "" {
		t.Fatalf("original changed: %#v %v", original, err)
	}
}

func TestRestoreStillRejectsClaimedMissingRevision(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	sessions, err := worksession.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := sessions.CreateChat(source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	chat.HeadRevision = "rev-missing"
	bytes, err := json.Marshal(chat)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "gator", "conversations", chat.ID, "conversation.json"), bytes, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "missing.tar.gz")
	if _, err = store.Export(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if _, err = store.PreviewRestore(context.Background(), archive, target, nil); err == nil {
		t.Fatal("accepted a claimed missing revision")
	}
	if _, err = os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid preview created destination")
	}
}
