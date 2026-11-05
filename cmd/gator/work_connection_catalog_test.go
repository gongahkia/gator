package main

import (
	"context"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/connector"
)

func TestConnectionCatalogOffersServicesWithoutAnAdvancedCategory(t *testing.T) {
	root, _, _ := nativeConnectionFixture(t, "https://example.com")
	c := &connectionControls{stateDir: root}
	screen, err := c.screen("")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"Connect Google email": connector.KindGoogle, "Slack": connector.KindSlack, "Notion": connector.KindNotion, "Atlassian": connector.KindAtlassian, "JSON API": connector.KindHTTPJSON, "Personal Google": connector.KindGoogle}
	for _, a := range screen.Actions {
		if strings.Contains(strings.ToLower(a.Label), "advanced") || a.Action == "add-token" {
			t.Fatal("service discovery still requires a generic advanced flow")
		}
		if kind, found := want[a.Label]; found {
			if a.Icon != kind {
				t.Fatalf("%s has no service logo identity", a.Label)
			}
			delete(want, a.Label)
		}
	}
	if len(want) != 0 {
		t.Fatal("services missing from Connections", want)
	}
}

func TestServiceConnectionSetupPinsTheCorrectAPIWithoutSavingCredentials(t *testing.T) {
	root, _, credentials := nativeConnectionFixture(t, "https://example.com")
	c := &connectionControls{stateDir: root}
	for _, test := range []struct {
		action, id, name, kind, resource string
		args                             []string
	}{
		{"add-slack", "slack", "Team Slack", connector.KindSlack, "https://slack.com/api", []string{"slack", "Team Slack"}},
		{"add-notion", "notion", "Team Notion", connector.KindNotion, "https://api.notion.com/v1", []string{"notion", "Team Notion"}},
		{"add-atlassian", "atlassian", "Team Atlassian", connector.KindAtlassian, "https://api.atlassian.com/ex/jira/cloud-id", []string{"atlassian", "Team Atlassian", "https://api.atlassian.com/ex/jira/cloud-id"}},
		{"add-json", "metrics", "Metrics", connector.KindHTTPJSON, "https://example.com/metrics", []string{"metrics", "Metrics", "https://example.com/metrics"}},
	} {
		t.Run(test.id, func(t *testing.T) {
			if _, err := c.call(context.Background(), test.action, append(append([]string{}, test.args...), "unexpected-resource")); err == nil {
				t.Fatal("extra setup fields changed the preset API identity")
			}
			if _, err := c.call(context.Background(), test.action, test.args); err != nil {
				t.Fatal(err)
			}
			d, _, err := configuredConnector(test.id)
			if err != nil || d.Kind != test.kind || d.Resource != test.resource || d.Name != test.name {
				t.Fatalf("incorrect native service setup: %+v, %v", d, err)
			}
			if _, found, err := credentials.Read(d.CredentialRef()); err != nil || found {
				t.Fatal("saving a service claimed authorization before token setup", err)
			}
		})
	}
}
