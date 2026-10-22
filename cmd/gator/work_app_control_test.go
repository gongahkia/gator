package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workrun"
)

func appControllerForTest(t *testing.T) gatorAppController {
	t.Helper()
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return gatorAppController{stateDir: t.TempDir(), settings: store, request: &workrun.Request{}}
}

func TestGatorAppChatRebindsShortcutsAndVoicePreferences(t *testing.T) {
	c := appControllerForTest(t)
	for _, change := range []workrun.AppChange{
		{Setting: "shortcuts", Value: json.RawMessage(`{"action":"command_search","shortcut":"Mod+P"}`)},
		{Setting: "voice", Value: json.RawMessage(`{"spoken_replies":true,"language":"en-SG"}`)},
	} {
		mutation, err := c.Prepare(context.Background(), change)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mutation.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	saved, _ := c.settings.Load()
	if saved.Desktop.ResolvedShortcuts()["command_search"] != "Mod+P" || !saved.Desktop.Voice.SpokenReplies || saved.Desktop.Voice.Language != "en-SG" {
		t.Fatalf("preferences not saved: %#v", saved.Desktop)
	}
	conflict, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "shortcuts", Value: json.RawMessage(`{"action":"command_search","shortcut":"Mod+F"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conflict.Apply(context.Background()); err == nil {
		t.Fatal("conflicting keybind saved")
	}
	saved, _ = c.settings.Load()
	if saved.Desktop.ResolvedShortcuts()["command_search"] != "Mod+P" {
		t.Fatal("failed rebind altered the saved binding")
	}
}

func TestGatorAppModelAndThemeChangesStayWithinKnownSettings(t *testing.T) {
	c := appControllerForTest(t)
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "fixture", Model: "old"}
	settings.CustomProviders = []config.CustomProvider{{ID: "fixture", BaseURL: "http://127.0.0.1:11434/v1/chat/completions", Models: []string{"old", "new"}, DefaultModel: "old"}}
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	c.request.Provider = "fixture/old"
	change := workrun.AppChange{Setting: "model", Value: json.RawMessage(`{"provider":"fixture","model":"new"}`)}
	mutation, err := c.Prepare(context.Background(), change)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved, _ := c.settings.Load()
	if saved.Defaults.Model != "new" || c.request.Provider != "fixture/old" {
		t.Fatal("model selection did not affect only future requests")
	}
	change.Value = json.RawMessage(`{"provider":"fixture","model":"unconfigured"}`)
	if _, err := c.Prepare(context.Background(), change); err == nil {
		t.Fatal("unconfigured model accepted")
	}
	mutation, err = c.Prepare(context.Background(), workrun.AppChange{Setting: "theme", Value: json.RawMessage(`{"theme":"mono"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.settings.Update(func(s *config.Settings) error { s.Theme = "contrast"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.Apply(context.Background()); err == nil {
		t.Fatal("stale change overwrote a newer UI edit")
	}
}

func TestGatorAppRetrievalChangesRequireUploadReviewAndPreserveChat(t *testing.T) {
	c := appControllerForTest(t)
	before, _ := c.settings.Load()
	mutation, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "retrieval", Value: json.RawMessage(`{"provider":"gemini"}`)})
	if err != nil {
		t.Fatal(err)
	}
	preview, _ := json.Marshal(mutation.Preview)
	if !mutation.Review || !mutation.AfterRun || !strings.Contains(string(preview), "Uploads eligible saved memory") {
		t.Fatal("cloud retrieval did not disclose the saved text upload for review")
	}
	unchanged, _ := c.settings.Load()
	if unchanged.Embeddings != before.Embeddings {
		t.Fatal("preparing a review changed retrieval")
	}
	if _, err := mutation.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := c.settings.Load()
	if after.Embeddings.Provider != "gemini" || !after.Embeddings.CloudUploadApproved || after.Defaults != before.Defaults {
		t.Fatal("retrieval selection changed unrelated settings or lost upload consent")
	}
	if _, err := mutation.Apply(context.Background()); err == nil {
		t.Fatal("stale retrieval review was applied")
	}
	local, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "retrieval", Value: json.RawMessage(`{"provider":"ollama"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ = c.settings.Load()
	if after.Embeddings.CloudUploadApproved {
		t.Fatal("local retrieval kept the cloud upload opt-in")
	}
}

func TestGatorAppInspectionReadsUsageAndOmitsEndpointSecrets(t *testing.T) {
	c := appControllerForTest(t)
	settings := config.Default()
	settings.Defaults.Provider = "private-fixture"
	settings.CustomProviders = []config.CustomProvider{{ID: "private-fixture", BaseURL: "https://endpoint-must-stay-private.example/v1/chat/completions", APIKeyEnv: "PRIVATE_FIXTURE_KEY", Models: []string{"fixture-model"}, DefaultModel: "fixture-model"}}
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRIVATE_FIXTURE_KEY", "credential-must-stay-private")
	for _, topic := range []string{"settings", "models", "connections", "usage", "outputs"} {
		view, err := c.Inspect(context.Background(), workrun.AppQuery{Topic: topic})
		if err != nil {
			t.Fatalf("%s: %v", topic, err)
		}
		data, _ := json.Marshal(view)
		for _, secret := range []string{"endpoint-must-stay-private", "credential-must-stay-private"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("%s exposed %s", topic, secret)
			}
		}
		if topic == "usage" && view.(uisnapshot.Panel).Usage == nil {
			t.Fatal("usage inspection did not return the live usage projection")
		}
	}
	if _, err := c.Inspect(context.Background(), workrun.AppQuery{Topic: "arbitrary-file"}); err == nil {
		t.Fatal("unknown inspection accepted")
	}
}
