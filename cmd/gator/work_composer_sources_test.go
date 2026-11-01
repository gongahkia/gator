package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestSourceCatalogUsesLocalStatusAndAccountBoundReadChecks(t *testing.T) {
	c := appControllerForTest(t)
	settings, _ := c.settings.Load()
	settings.Connectors = []connector.Descriptor{{Version: 1, ID: "reports", Name: "Reports", Kind: connector.KindHTTPJSON, Resource: "https://example.com/private-resource", Authentication: connector.AuthNone}, {Version: 1, ID: "missing", Name: "Missing", Kind: connector.KindHTTPJSON, Resource: "https://example.com/other", Authentication: connector.AuthBearer}}
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	controls := &connectionControls{stateDir: c.stateDir}
	choices, err := controls.composerSources(c.settings)
	if err != nil {
		t.Fatal(err)
	}
	if choices[0].Unavailable != "" || choices[1].Unavailable == "" || choices[0].Access != "Read access not checked" {
		t.Fatalf("incorrect local inventory: %#v", choices)
	}
	raw, _ := json.Marshal(choices)
	if strings.Contains(string(raw), "example.com") || strings.Contains(string(raw), choices[0].Binding) {
		t.Fatal("source projection leaked native configuration")
	}
	controls.recordSourceCheck("reports", choices[0].Binding, true)
	choices, _ = controls.composerSources(c.settings)
	if choices[0].CheckedAt == "" || !strings.Contains(choices[0].Access, "recently") {
		t.Fatal("explicit read check lost")
	}
	controls.checks["reports"] = sourceReadCheck{binding: choices[0].Binding, at: time.Now().Add(-6 * time.Minute), passed: true}
	choices, _ = controls.composerSources(c.settings)
	if !strings.Contains(choices[0].Access, "stale") {
		t.Fatal("old read check remained fresh")
	}
	settings.Connectors[0].Resource = "https://example.com/replaced"
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	choices, _ = controls.composerSources(c.settings)
	if choices[0].CheckedAt != "" {
		t.Fatal("replaced backend inherited read-check evidence")
	}
}
func TestSourceSelectionPersistsCASWithoutChangingAnotherConversation(t *testing.T) {
	c := appControllerForTest(t)
	settings, _ := c.settings.Load()
	settings.Connectors = []connector.Descriptor{{Version: 1, ID: "reports", Name: "Reports", Kind: connector.KindHTTPJSON, Resource: "https://example.com", Authentication: connector.AuthNone}}
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	sessions, _ := worksession.Open(c.stateDir)
	first, _ := sessions.Create("First", t.TempDir(), "snap-fixture", time.Now())
	second, _ := sessions.Create("Second", t.TempDir(), "snap-fixture", time.Now())
	if err := selectComposerSources(c.settings, c.stateDir, first.ID, nil, []string{"reports"}); err != nil {
		t.Fatal(err)
	}
	if err := selectComposerSources(c.settings, c.stateDir, first.ID, nil, []string{}); err == nil {
		t.Fatal("stale selection overwrote a newer preference")
	}
	p, err := sessions.LoadPreferences(second.ID)
	if err != nil || p.ConnectorIDs != nil {
		t.Fatal("another chat was changed")
	}
	if err := c.settings.Update(func(s *config.Settings) error { s.Connectors = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := selectComposerSources(c.settings, c.stateDir, first.ID, []string{"reports"}, []string{"reports"}); err == nil {
		t.Fatal("removed source was selected")
	}
}

func TestSourceCatalogExpiryAndRevocationInvalidateCheckedAccess(t *testing.T) {
	c := appControllerForTest(t)
	settings, _ := c.settings.Load()
	d := connector.Descriptor{Version: 1, ID: "private", Name: "Private", Kind: connector.KindHTTPJSON, Resource: "https://example.com", Authentication: connector.AuthBearer}
	settings.Connectors = []connector.Descriptor{d}
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(c.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.NewBearerToken("private-test-token", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(d.CredentialRef(), token); err != nil {
		t.Fatal(err)
	}
	controls := &connectionControls{stateDir: c.stateDir}
	choices, _ := controls.composerSources(c.settings)
	controls.recordSourceCheck(d.ID, choices[0].Binding, true)
	token.Expires = time.Now().Add(-time.Minute).UnixMilli()
	if err := credentials.Put(d.CredentialRef(), token); err != nil {
		t.Fatal(err)
	}
	choices, _ = controls.composerSources(c.settings)
	if choices[0].Unavailable == "" || choices[0].CheckedAt != "" {
		t.Fatal("expired credential retained access")
	}
	if err := credentials.Delete(d.CredentialRef()); err != nil {
		t.Fatal(err)
	}
	choices, _ = controls.composerSources(c.settings)
	if choices[0].Unavailable == "" || choices[0].CheckedAt != "" {
		t.Fatal("revoked credential retained access")
	}
}
