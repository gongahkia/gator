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
