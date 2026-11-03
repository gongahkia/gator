package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/orchestrator"
)

func TestAgentProfilesFollowKnowledgeBackupAndDeletion(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	profiles, _ := orchestrator.NewProfileStore(root)
	d := orchestrator.AgentDefinition{Version: 1, Name: "research", Base: "source_researcher", Description: "Research facts", Instructions: "Cite primary evidence."}
	if err := profiles.Put(d, ""); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "agent-profiles/.profiles-orphan.tmp", "partial")
	writeFixture(t, root, "tasks/old-run/roles/profiles.json", `{"version":1,"profiles":[]}`)
	writeFixture(t, root, "tasks/old-run/roles/.profiles.lock", "")
	store, _ := Open(root)
	backup := filepath.Join(t.TempDir(), "profiles.tar.gz")
	manifest, err := store.Export(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"gator/agent-profiles/profiles.json", "gator/tasks/old-run/roles/profiles.json"} {
		if _, ok := manifest.Files[path]; !ok {
			t.Fatal("profile missing from backup", path)
		}
	}
	for _, path := range []string{"gator/agent-profiles/.profiles.lock", "gator/agent-profiles/.profiles-orphan.tmp", "gator/tasks/old-run/roles/.profiles.lock"} {
		if _, ok := manifest.Files[path]; ok {
			t.Fatal("runtime profile file exported", path)
		}
	}
	if _, err := VerifyBackup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	plan, err := store.PreviewDelete(ctx, "knowledge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	if definitions, err := profiles.List(); err != nil || len(definitions) != 0 {
		t.Fatal("knowledge deletion left saved definitions", definitions, err)
	}
	if _, err := os.Stat(filepath.Join(root, "gator/tasks/old-run/roles/profiles.json")); err != nil {
		t.Fatal("knowledge deletion erased task history", err)
	}
}
