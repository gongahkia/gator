package engine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/runtime"
	"github.com/gongahkia/norbot/internal/store"
)

func TestCreateChangeRunIntegration(t *testing.T) {
	databaseURL := os.Getenv("NORBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set NORBOT_TEST_DATABASE_URL to run Postgres integration coverage")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	parentID := "change-parent-" + time.Now().UTC().Format("20060102150405.000000000")
	graph := domain.DefaultGraph()
	parent := domain.Run{ID: parentID, AppID: "app-" + parentID, Prompt: "create app", Profile: domain.ProfileFrontend, DeploymentTarget: domain.DeploymentDocker, MaxFixes: 2, Stage: domain.StageDeployer, Status: domain.StatusCompleted, WorkspaceStatus: "ready", Providers: map[domain.Stage]string{domain.StagePlanner: "test", domain.StageBuilder: "test", domain.StageVerifier: "test", domain.StageDeployer: "local-deployer"}, Graph: graph, Architecture: domain.DefaultArchitecture(domain.ProfileFrontend, graph), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := st.CreateRun(ctx, parent); err != nil {
		t.Fatal(err)
	}
	workspace := runtime.Workspace{ArtifactsDir: root}
	if _, err := workspace.WriteArtifact(parent.ID, "generated-app/frontend/package.json", []byte(`{"name":"app"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.WriteArtifact(parent.ID, "generated-app/frontend/src/main.jsx", []byte("export default null")); err != nil {
		t.Fatal(err)
	}
	service := New(st, config.Config{ArtifactsDir: root}, nil)
	child, err := service.CreateChangeRun(ctx, parent.ID, "add search", false)
	if err != nil {
		t.Fatal(err)
	}
	if child.AppID != parent.AppID || child.ParentRunID != parent.ID || child.BaseSnapshotDigest == "" {
		t.Fatalf("child=%#v", child)
	}
	if child.Stage != domain.StageBuilder || child.WorkspaceStatus != "provisioning" {
		t.Fatalf("child stage/status=%s/%s", child.Stage, child.WorkspaceStatus)
	}
	architectural, err := service.CreateChangeRun(ctx, parent.ID, "replace data model", true)
	if err != nil {
		t.Fatal(err)
	}
	if architectural.Stage != domain.StagePlanner || architectural.AppID != parent.AppID {
		t.Fatalf("architecture child=%#v", architectural)
	}
}
