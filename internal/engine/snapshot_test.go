package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/runtime"
)

func TestCaptureAppSnapshotIsContentAddressedAndImmutable(t *testing.T) {
	root := t.TempDir()
	workspace := runtime.Workspace{ArtifactsDir: root}
	run := domain.Run{ID: "approved", AppID: "app-1"}
	if _, err := workspace.WriteArtifact(run.ID, "generated-app/frontend/src/main.jsx", []byte("first")); err != nil {
		t.Fatal(err)
	}
	service := &Service{config: config.Config{ArtifactsDir: root}}
	snapshot, err := service.captureAppSnapshot(run, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Digest == "" || snapshot.FileCount != 1 || snapshot.AppID != run.AppID {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if _, err := workspace.WriteArtifact(run.ID, "generated-app/frontend/src/main.jsx", []byte("changed")); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(snapshot.Path, "generated-app", "frontend", "src", "main.jsx"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "first" {
		t.Fatalf("snapshot mutated: %q", contents)
	}
	files, err := snapshotGeneratedApp(snapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	if actual := digestFiles(files); actual != snapshot.Digest {
		t.Fatalf("digest=%s want=%s", actual, snapshot.Digest)
	}
}

func TestApplyRevisionStagesThenReplacesGeneratedApp(t *testing.T) {
	root := t.TempDir()
	workspace := runtime.Workspace{ArtifactsDir: root}
	run := domain.Run{ID: "approval"}
	baseline := map[string]string{"generated-app/frontend/src/main.jsx": "before", "generated-app/frontend/package.json": "{}"}
	for path, content := range baseline {
		if _, err := workspace.WriteArtifact(run.ID, path, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{"generated-app/frontend/src/main.jsx": "after"}
	revision := domain.Revision{RunID: run.ID, State: "proposed", BaselineDigest: digestFiles(baseline), PatchDigest: digestStringMap(files), Files: files}
	if err := applyRevision(workspace, run, revision); err != nil {
		t.Fatal(err)
	}
	current, err := snapshotGeneratedApp(workspace.RunPath(run.ID))
	if err != nil {
		t.Fatal(err)
	}
	if current["generated-app/frontend/src/main.jsx"] != "after" || current["generated-app/frontend/package.json"] != "{}" {
		t.Fatalf("current=%#v", current)
	}
	entries, err := os.ReadDir(workspace.RunPath(run.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "generated-app" {
			t.Fatalf("approval staging residue: %s", entry.Name())
		}
	}
}
