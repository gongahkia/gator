package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestGatorAppTerminalPreferencesUseCanonicalSettingsAndRejectStaleChanges(t *testing.T) {
	c := appControllerForTest(t)
	if _, err := c.settings.ImportTerminalPreferences(config.DefaultTerminalPreferences()); err != nil {
		t.Fatal(err)
	}
	change := workrun.AppChange{Setting: "terminal", Value: json.RawMessage(`{"theme":"light","sidebarCollapsed":false}`)}
	prepared, err := c.Prepare(context.Background(), change)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Review || prepared.AfterRun || prepared.AfterReply || prepared.Summary == "" {
		t.Fatal("display-only change should report its actual immediate result")
	}
	if _, err := prepared.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	settings, _ := c.settings.Load()
	if settings.Theme != "light" || settings.TerminalPreferences().SidebarCollapsed {
		t.Fatal("chat updated a different preference store")
	}
	if _, err := prepared.Apply(context.Background()); err == nil {
		t.Fatal("stale prepared change executed again")
	}
	reset, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "theme", Value: json.RawMessage(`{"theme":"terminal"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reset.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	settings, _ = c.settings.Load()
	if settings.Theme != "terminal" || settings.TerminalPreferences().SidebarCollapsed {
		t.Fatal("theme alias lost layout or failed to reset native colors")
	}
	for _, value := range []string{`{}`, `{"theme":"bad"}`, `{"sidebarCollapsed":"yes"}`, `{"api_key":"secret"}`, `{"workDetailsSections":{"outputs":false}}`} {
		if _, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "terminal", Value: json.RawMessage(value)}); err == nil {
			t.Fatal("unsupported terminal mutation accepted", value)
		}
	}
}

func TestGatorAppTerminalChangesCannotClaimSuccessBeforeImportRecovery(t *testing.T) {
	c := appControllerForTest(t)
	c.request.ForegroundApp = true
	if _, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "theme", Value: json.RawMessage(`{"theme":"light"}`)}); err == nil {
		t.Fatal("unimported terminal preferences reported a supported change")
	}
}
