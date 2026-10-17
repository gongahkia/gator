package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestGoalConnectorReadinessShowsRecoveryAndRevokedRead(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	if err := connectorCommandWithIO([]string{"add", "metrics", "--url", "https://example.com/metrics.json", "--auth", "bearer"}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	definition := jobs.Definition{ConnectorIDs: []string{"metrics"}, WatchSources: []jobs.WatchSource{{ConnectorID: "metrics", Operation: "fetch"}}}
	if got := strings.Join(goalConnectorReadiness(definition), "\n"); !strings.Contains(got, "login required; run gator provider connector login metrics") {
		t.Fatalf("missing recovery instruction: %s", got)
	}
	descriptor, credentials, err := configuredConnector("metrics")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := auth.NewBearerToken("private-token", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	if err := connectorCommandWithIO([]string{"permission", "metrics", "fetch", "read", "deny"}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(goalConnectorReadiness(definition), "\n"); !strings.Contains(got, "authenticated; read permission missing for fetch") || strings.Contains(got, "private-token") {
		t.Fatalf("revoked read not visible or token leaked: %s", got)
	}
}
