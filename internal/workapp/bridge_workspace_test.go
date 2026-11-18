package workapp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/workspace"
)

func workspaceModel(source string) Model {
	m := New(Config{BridgeMode: true, ManagedSessions: true, ResolveSource: func(path string) (string, error) { r, e := workspace.Open(path); return r.Path(), e }})
	m.source = source
	m.conversation = "original"
	m.input = "Unsent original"
	m.running = true
	return m
}
func resolveNative(t *testing.T, m Model, id, path string) Model {
	t.Helper()
	next, cmd := m.ResolveNativeWorkspace(NativeWorkspaceAction{RequestID: id, Path: path})
	if cmd == nil {
		t.Fatal("resolution not started")
	}
	next, _ = next.Update(cmd())
	return next.(Model)
}
func TestNativeWorkspaceCanonicalDataAndOwnerIsolation(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Unicode café 日本 ; source")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(folder, link); err != nil {
		t.Fatal(err)
	}
	canonical, _ := workspace.Open(folder)
	m := resolveNative(t, workspaceModel(root), "one", link)
	r := m.nativeWorkspaceResult
	if r.Error != "" || r.Source != canonical.Path() || !r.New || r.ConversationID != "original" || r.RequestID != "one" {
		t.Fatal(r)
	}
	if m.source != root || m.conversation != "original" || !m.running || m.input != "Unsent original" || m.sessionRequest != nil {
		t.Fatal("resolution mutated original authority")
	}
	m.source = canonical.Path()
	m = resolveNative(t, m, "same", link)
	if m.nativeWorkspaceResult.New {
		t.Fatal("same workspace requested a new owner")
	}
}
func TestNativeWorkspaceRejectsMissingFilesControlPathsAndInaccessibleFolders(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	_ = os.WriteFile(file, []byte("private"), 0600)
	private := filepath.Join(root, "denied")
	_ = os.Mkdir(private, 0000)
	defer os.Chmod(private, 0700)
	cases := []string{filepath.Join(root, "missing"), file, "relative", root + "\n/source", root + "\x00", private}
	for i, path := range cases {
		if path == private && os.Geteuid() == 0 {
			continue
		}
		m := resolveNative(t, workspaceModel(root), string(rune('a'+i)), path)
		if m.nativeWorkspaceResult.Error == "" || m.nativeWorkspaceResult.Source != "" {
			t.Fatalf("accepted %q: %+v", path, m.nativeWorkspaceResult)
		}
	}
}
func TestNativeWorkspaceResultIdentityAndDuplicates(t *testing.T) {
	root := t.TempDir()
	m := workspaceModel(root)
	next, cmd := m.ResolveNativeWorkspace(NativeWorkspaceAction{RequestID: "one", Path: root})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("no pending resolution")
	}
	next, second := m.ResolveNativeWorkspace(NativeWorkspaceAction{RequestID: "two", Path: root})
	if second != nil {
		t.Fatal("overlapping resolution accepted")
	}
	m = next.(Model)
	next, _ = m.finishNativeWorkspace(NativeWorkspaceResult{RequestID: "wrong", Source: root})
	m = next.(Model)
	if m.nativeWorkspaceResult != nil {
		t.Fatal("stale result accepted")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	next, again := m.ResolveNativeWorkspace(NativeWorkspaceAction{RequestID: "one", Path: root + "/different"})
	if again != nil {
		t.Fatal("completed identity replayed")
	}
	m = next.(Model)
	if m.nativeWorkspaceResult.RequestID != "one" {
		t.Fatal("result replaced")
	}
	next, cmd = m.ResolveNativeWorkspace(NativeWorkspaceAction{RequestID: "three", Path: root})
	m = next.(Model)
	m.conversation = "replacement"
	next, _ = m.Update(cmd())
	if next.(Model).nativeWorkspaceResult.Error == "" {
		t.Fatal("changed owner result accepted")
	}
}

func TestNativeWorkspaceLongExistingPathAndNoMissingDirectoryCreation(t *testing.T) {
	root := t.TempDir()
	long := root
	for i := 0; i < 35; i++ {
		long = filepath.Join(long, "long-project-component")
	}
	if err := os.MkdirAll(long, 0700); err != nil {
		t.Fatal(err)
	}
	m := resolveNative(t, workspaceModel(root), "long", long)
	canonical, _ := workspace.Open(long)
	if m.nativeWorkspaceResult.Error != "" || m.nativeWorkspaceResult.Source != canonical.Path() {
		t.Fatal(m.nativeWorkspaceResult)
	}
	missing := filepath.Join(root, "must-not-create")
	m = resolveNative(t, m, "missing", missing)
	if m.nativeWorkspaceResult.Error == "" {
		t.Fatal("missing accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("selection created a directory", err)
	}
}

func TestNativeWorkspaceRejectsDirectoryWithoutTraversalPermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires ordinary user permissions")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "list-only")
	if err := os.Mkdir(directory, 0400); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(directory, 0700)
	m := resolveNative(t, workspaceModel(root), "no-traversal", directory)
	if m.nativeWorkspaceResult.Error == "" {
		t.Fatal("directory without traversal permission was accepted")
	}
}

func TestNativeWorkspaceSameDirectoryRetainsGoSourceUnicodeSpelling(t *testing.T) {
	root := t.TempDir()
	composed := filepath.Join(root, "Folder caf\u00e9")
	decomposed := filepath.Join(root, "Folder cafe\u0301")
	if err := os.Mkdir(composed, 0700); err != nil {
		t.Fatal(err)
	}
	a, err := os.Stat(composed)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(decomposed)
	if os.IsNotExist(err) {
		t.Skip("filesystem does not resolve both Unicode spellings to the same directory")
	}
	if err != nil || !os.SameFile(a, b) {
		t.Fatal("unexpected fixture identity", err)
	}
	m := resolveNative(t, workspaceModel(decomposed), "same-unicode", composed)
	r := m.nativeWorkspaceResult
	if r.Error != "" || r.New || r.Source != decomposed {
		t.Fatalf("same directory must retain exact Go source spelling: %+v", r)
	}
}
