package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/testrepo"
)

func TestNewIDUsesEntropyAndTime(t *testing.T) {
	id, err := NewID(time.Date(2026, 7, 21, 12, 30, 45, 123, time.UTC), bytes.NewReader(bytes.Repeat([]byte{0xab}, 10)))
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if id != "session-20260721T123045.000000123Z-abababababababababab" {
		t.Fatalf("id = %q", id)
	}
}

func TestCreateNewRetriesIDCollision(t *testing.T) {
	cwd := t.TempDir()
	now := time.Date(2026, 7, 21, 12, 30, 45, 0, time.UTC)
	first := bytes.Repeat([]byte{0xab}, 10)
	second := bytes.Repeat([]byte{0xcd}, 10)
	id, err := NewID(now, bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewManifest(id, cwd, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Create(cwd, manifest); err != nil {
		t.Fatal(err)
	}
	store, created, err := CreateNew(cwd, now, bytes.NewReader(append(first, second...)))
	if err != nil {
		t.Fatal(err)
	}
	if store == nil || created.ID == id {
		t.Fatalf("created = %#v", created)
	}
}

func TestCreateNewFailsAfterRepeatedIDCollisions(t *testing.T) {
	cwd := t.TempDir()
	now := time.Date(2026, 7, 21, 12, 30, 45, 0, time.UTC)
	entropy := bytes.Repeat([]byte{0xab}, 10)
	id, err := NewID(now, bytes.NewReader(entropy))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewManifest(id, cwd, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Create(cwd, manifest); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateNew(cwd, now, bytes.NewReader(bytes.Repeat(entropy, maxIDAttempts))); !errors.Is(err, ErrSessionIDCollision) {
		t.Fatalf("create new error = %v", err)
	}
}

func TestStorePersistsManifestAndEvents(t *testing.T) {
	cwd := t.TempDir()
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	manifest, err := NewManifest("session-test-01", cwd, now)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	store, err := Create(cwd, manifest)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.LoadManifest()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ID != manifest.ID || got.Cwd != manifest.Cwd || got.Status != StatusActive {
		t.Fatalf("manifest = %#v", got)
	}
	event, err := store.Append(Event{Type: "session.started", At: now, Data: json.RawMessage(`{"source":"test"}`)})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if event.Sequence != 1 || event.SessionID != manifest.ID {
		t.Fatalf("event = %#v", event)
	}
	events, err := store.Events()
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Type != "session.started" {
		t.Fatalf("events = %#v", events)
	}
	info, err := os.Stat(filepath.Join(store.Dir(), "manifest.json"))
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest mode = %o", info.Mode().Perm())
	}
}

func TestAppendScrubsSecretEventData(t *testing.T) {
	cwd := t.TempDir()
	manifest, err := NewManifest("session-scrub-event", cwd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Create(cwd, manifest)
	if err != nil {
		t.Fatal(err)
	}
	secret := "sk-abcdefghijklmnopqrstuvwxyz123456"
	event, err := store.Append(Event{Type: "session.note", Data: json.RawMessage(`{"value":"` + secret + `"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(event.Data), secret) {
		t.Fatalf("event data leaked secret: %s", event.Data)
	}
	data, err := os.ReadFile(filepath.Join(store.Dir(), "events.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("session event file leaked secret: %s", data)
	}
}

func TestNewManifestRecordsNonGitWorkspaceCapabilities(t *testing.T) {
	cwd := t.TempDir()
	manifest, err := NewManifest("session-nongit-capabilities", cwd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Workspace.IsGit || manifest.Workspace.GitRoot != "" || !manifest.Workspace.WritableHint {
		t.Fatalf("workspace = %#v", manifest.Workspace)
	}
}

func TestNewManifestRecordsGitWorkspaceCapabilities(t *testing.T) {
	repo := testrepo.NewGit(t)
	manifest, err := NewManifest("session-git-capabilities", repo.Root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Workspace.IsGit || manifest.Workspace.GitRoot != manifest.Workspace.Root || !manifest.Workspace.WritableHint {
		t.Fatalf("workspace = %#v", manifest.Workspace)
	}
}

func TestManifestRejectsInconsistentWorkspaceCapabilities(t *testing.T) {
	cwd := t.TempDir()
	manifest, err := NewManifest("session-capability-validation", cwd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	manifest.Workspace.GitRoot = manifest.Workspace.Root
	if err := validateManifest(manifest); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("non-git validation error = %v", err)
	}
	manifest.Workspace.IsGit = true
	manifest.Workspace.GitRoot = ""
	if err := validateManifest(manifest); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("git validation error = %v", err)
	}
}

func TestAppendAssignsContiguousSequencesUnderConcurrency(t *testing.T) {
	cwd := t.TempDir()
	manifest, err := NewManifest("session-journal-concurrent", cwd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Create(cwd, manifest)
	if err != nil {
		t.Fatal(err)
	}
	const writers = 32
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Append(Event{Type: "session.updated"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != writers {
		t.Fatalf("events = %d, want %d", len(events), writers)
	}
	for i, event := range events {
		if event.Sequence != i+1 || event.SessionID != manifest.ID {
			t.Fatalf("event %d = %#v", i, event)
		}
	}
}

func TestEventsRejectWrongSessionID(t *testing.T) {
	cwd := t.TempDir()
	manifest, err := NewManifest("session-journal-binding", cwd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Create(cwd, manifest)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"schema_version":"paw.session/2","session_id":"session-other","sequence":1,"at":"2026-07-21T00:00:00Z","type":"session.started"}` + "\n")
	if err := os.WriteFile(filepath.Join(store.Dir(), "events.ndjson"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Events(); err == nil {
		t.Fatal("expected session binding error")
	}
}

func TestSaveManifestRejectsInvalidWithoutReplacingExisting(t *testing.T) {
	cwd := t.TempDir()
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	manifest, err := NewManifest("session-atomic-01", cwd, now)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Create(cwd, manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Dir(), "manifest.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveManifest(Manifest{}); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("save invalid manifest error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("invalid manifest replaced persisted manifest")
	}
}

func TestSaveManifestReplacesExistingSnapshot(t *testing.T) {
	cwd := t.TempDir()
	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	manifest, err := NewManifest("session-atomic-02", cwd, now)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Create(cwd, manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Status = StatusStopped
	manifest.UpdatedAt = now.Add(time.Minute)
	if err := store.SaveManifest(manifest); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusStopped || !got.UpdatedAt.Equal(manifest.UpdatedAt) {
		t.Fatalf("manifest = %#v", got)
	}
	entries, err := os.ReadDir(store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "manifest.json" {
		t.Fatalf("session entries = %#v", entries)
	}
}

func TestLoadManifestAcceptsV2Fixture(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("testdata", "manifest-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := (&Store{dir: dir}).LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != SchemaVersion || manifest.ID != "session-fixture-v2" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestLoadManifestRejectsLegacyFixtureWithoutRewrite(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("testdata", "manifest-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Store{dir: dir}).LoadManifest(); !errors.Is(err, ErrMigrationRequired) {
		t.Fatalf("load legacy manifest error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, data) {
		t.Fatal("legacy manifest was rewritten")
	}
}

func TestCreateRejectsTraversalID(t *testing.T) {
	cwd := t.TempDir()
	manifest, err := NewManifest("session-../escape", cwd, time.Now())
	if err == nil || !errors.Is(err, ErrInvalidSessionID) || manifest.ID != "" {
		t.Fatalf("manifest = %#v err = %v", manifest, err)
	}
	manifest, err = NewManifest("session-..", cwd, time.Now())
	if err == nil || !errors.Is(err, ErrInvalidSessionID) || manifest.ID != "" {
		t.Fatalf("manifest = %#v err = %v", manifest, err)
	}
}

func TestCreateRejectsManifestForOtherWorkspace(t *testing.T) {
	manifest, err := NewManifest("session-other-workspace", t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Create(t.TempDir(), manifest); err == nil {
		t.Fatal("expected workspace mismatch")
	}
}

func TestSessionRootIsRepoLocal(t *testing.T) {
	cwd := t.TempDir()
	root, err := SessionRoot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonical, ".paw", "sessions")
	if root != want {
		t.Fatalf("session root = %q, want %q", root, want)
	}
}

func TestSessionRootRejectsEscapingPawSymlink(t *testing.T) {
	cwd := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(cwd, ".paw")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := SessionRoot(cwd); err == nil {
		t.Fatal("expected symlink containment error")
	}
}

func TestEventsRejectCorruptSequence(t *testing.T) {
	cwd := t.TempDir()
	manifest, err := NewManifest("session-corrupt-01", cwd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Create(cwd, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir(), "events.ndjson"), []byte(`{"schema_version":"paw.session/2","sequence":2}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Events(); err == nil {
		t.Fatal("expected corrupt event error")
	}
}

func TestListOrdersNewestSessionFirst(t *testing.T) {
	cwd := t.TempDir()
	older, err := NewManifest("session-list-older", cwd, time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	newer, err := NewManifest("session-list-newer", cwd, time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Create(cwd, older); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(cwd, newer); err != nil {
		t.Fatal(err)
	}
	manifests, err := List(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 2 || manifests[0].ID != newer.ID || manifests[1].ID != older.ID {
		t.Fatalf("manifests = %#v", manifests)
	}
}

func TestListReturnsEmptySliceForNewWorkspace(t *testing.T) {
	manifests, err := List(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if manifests == nil || len(manifests) != 0 {
		t.Fatalf("manifests = %#v", manifests)
	}
}
