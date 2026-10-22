package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func followupControllerForTest(t *testing.T) gatorAppController {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("GATOR_CONFIG_DIR", configDir)
	settings, err := config.New(configDir)
	if err != nil {
		t.Fatal(err)
	}
	state, source := t.TempDir(), t.TempDir()
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	sessions, _ := worksession.Open(state)
	chat, err := sessions.Create("Venue planning", source, "snap-test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return gatorAppController{stateDir: state, settings: settings, request: &workrun.Request{ForegroundApp: true, SourcePath: source, ConversationID: chat.ID}}
}

func followupToolChange(c gatorAppController, value map[string]any) workrun.AppChange {
	raw, _ := json.Marshal(value)
	return workrun.AppChange{ConversationID: c.request.ConversationID, Setting: "followup", Value: raw}
}

func createFollowupValue() map[string]any {
	return map[string]any{"action": "create", "title": "Venue replies", "outcome": "Track venue availability", "alert_when": "A price or availability changes", "interval_minutes": 15, "ends_at": time.Now().Add(48 * time.Hour).Format(time.RFC3339), "mode": "inspect"}
}

func TestChatFollowupReviewPersistsOwnedAgreementAndGuardsEdits(t *testing.T) {
	c := followupControllerForTest(t)
	ctx := context.Background()
	m, err := c.Prepare(ctx, followupToolChange(c, createFollowupValue()))
	if err != nil {
		t.Fatal(err)
	}
	if !m.Review || m.AfterRun {
		t.Fatal("creation skipped foreground agreement review")
	}
	if _, ok := m.Preview.(workrun.AppAgreement); !ok {
		t.Fatal("no structured agreement")
	}
	store, _ := jobs.Open(c.stateDir)
	before, _ := store.List()
	if len(before) != 0 {
		t.Fatal("prepare activated a check")
	}
	result, err := m.Apply(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt := result.(map[string]any)
	if receipt["saved"] != true || receipt["supervisor_running"] != false {
		t.Fatalf("offline save overclaimed: %+v", receipt)
	}
	definitions, _ := store.List()
	if len(definitions) != 1 {
		t.Fatal("no durable check")
	}
	d := definitions[0]
	if d.OriginConversationID != c.request.ConversationID || d.SourcePath != c.request.SourcePath || d.Mode != action.Inspect || d.EndsAt.IsZero() {
		t.Fatalf("agreement changed: %+v", d)
	}
	// Reapplying the prepared create must not produce a second definition.
	if _, err := m.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	definitions, _ = store.List()
	if len(definitions) != 1 {
		t.Fatal("repeated save duplicated check")
	}
	digest, _ := d.PolicyDigest()
	edit := map[string]any{"action": "edit", "id": d.ID, "expected_digest": digest, "alert_when": "Only a price changes"}
	pending, err := c.Prepare(ctx, followupToolChange(c, edit))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Claim(d.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := pending.Apply(ctx); err != nil {
		t.Fatal("scheduler bookkeeping invalidated review", err)
	}
	d, _ = store.Load(d.ID)
	if d.AlertWhen != "Only a price changes" || d.LastScheduledAt.IsZero() {
		t.Fatal("edit lost agreement or scheduler position")
	}
	if _, err := pending.Apply(ctx); err == nil {
		t.Fatal("stale edit accepted")
	}
	digest, _ = d.PolicyDigest()
	pause, err := c.Prepare(ctx, followupToolChange(c, map[string]any{"action": "pause", "id": d.ID, "expected_digest": digest}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pause.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	d, _ = store.Load(d.ID)
	if d.Enabled {
		t.Fatal("pause not saved")
	}
	digest, _ = d.PolicyDigest()
	resume, err := c.Prepare(ctx, followupToolChange(c, map[string]any{"action": "resume", "id": d.ID, "expected_digest": digest}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resume.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	d, _ = store.Load(d.ID)
	if !d.Enabled {
		t.Fatal("resume not saved")
	}
	sessions, _ := worksession.Open(c.stateDir)
	other, _ := sessions.Create("Another chat", c.request.SourcePath, "snap-other", time.Now())
	attack := followupToolChange(c, edit)
	attack.ConversationID = other.ID
	if _, err := c.Prepare(ctx, attack); err == nil {
		t.Fatal("cross-chat edit accepted")
	}
	if _, err := c.Inspect(ctx, workrun.AppQuery{Topic: "followups", ID: d.ID, ConversationID: other.ID}); err == nil {
		t.Fatal("cross-chat inspection accepted")
	}
}

func TestChatFollowupRejectsUnselectedSourcesExpiryAndForgedScope(t *testing.T) {
	c := followupControllerForTest(t)
	for _, mutate := range []func(map[string]any){
		func(v map[string]any) { v["source_path"] = "/another-workspace" },
		func(v map[string]any) { v["origin_conversation_id"] = "chat-foreign" },
		func(v map[string]any) { v["connector_ids"] = []string{"unselected"} },
		func(v map[string]any) { v["ends_at"] = "Friday" },
		func(v map[string]any) { v["ends_at"] = time.Now().Add(-time.Hour).Format(time.RFC3339) },
		func(v map[string]any) { v["mode"] = "act" },
		func(v map[string]any) { delete(v, "ends_at") },
	} {
		v := createFollowupValue()
		mutate(v)
		if _, err := c.Prepare(context.Background(), followupToolChange(c, v)); err == nil {
			t.Fatalf("accepted invalid agreement: %+v", v)
		}
	}
}

func TestChatFollowupRetainsSaveWhenBackgroundStartFails(t *testing.T) {
	c := followupControllerForTest(t)
	calls := 0
	c.startSupervisor = func(string) error { calls++; return errors.New("fixture startup failure") }
	v := createFollowupValue()
	v["start_local"] = true
	m, err := c.Prepare(context.Background(), followupToolChange(c, v))
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Apply(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := result.(map[string]any)
	if calls != 1 || r["saved"] != true || r["issue"] == "" || r["supervisor_running"] != false {
		t.Fatalf("partial activation misreported: %+v", r)
	}
}

func TestChatFollowupRechecksSelectedSourceAndCanPauseWithoutAccess(t *testing.T) {
	c := followupControllerForTest(t)
	settings := config.Default()
	settings.Connectors = []connector.Descriptor{{Version: 1, ID: "venue", Name: "Venue", Kind: connector.KindHTTPJSON, Resource: "https://example.com/venue", Authentication: connector.AuthNone}}
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	c.request.ConnectorIDs = []string{"venue"}
	v := createFollowupValue()
	v["connector_ids"] = []string{"venue"}
	m, err := c.Prepare(context.Background(), followupToolChange(c, v))
	if err != nil {
		t.Fatal(err)
	}
	settings.Connectors[0].Resource = "https://example.com/another-venue"
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background()); err == nil {
		t.Fatal("replacement source escaped review")
	}
	store, _ := jobs.Open(c.stateDir)
	definitions, _ := store.List()
	if len(definitions) != 0 {
		t.Fatal("stale source review wrote a check")
	}
	m, err = c.Prepare(context.Background(), followupToolChange(c, v))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	definitions, _ = store.List()
	d := definitions[0]
	digest, _ := d.PolicyDigest()
	settings.Connectors = nil
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	m, err = c.Prepare(context.Background(), followupToolChange(c, map[string]any{"action": "pause", "id": d.ID, "expected_digest": digest}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	d, _ = store.Load(d.ID)
	if d.Enabled {
		t.Fatal("could not pause a disconnected source")
	}
}
