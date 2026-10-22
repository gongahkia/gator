package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/config"
)

func TestComposerModelsKeepAliasesSeparateFromIdentityAndHidePrivateConfiguration(t *testing.T) {
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "fixture", Model: "model-one"}
	settings.CustomProviders = []config.CustomProvider{
		{ID: "fixture", BaseURL: "http://localhost:1234/private-endpoint", Models: []string{"model-one", "model-two"}, DefaultModel: "model-one"},
		{ID: "locked", BaseURL: "http://localhost:1234/another-private-endpoint", APIKeyEnv: "GATOR_COMPOSER_TEST_KEY", Models: []string{"locked-model"}, DefaultModel: "locked-model"},
	}
	settings.ModelAliases = map[string]string{config.ModelAliasKey("fixture", "model-two"): "Friendly model"}
	t.Setenv("GATOR_COMPOSER_TEST_KEY", "")
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	choices, err := workComposerModels(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seenAlias, seenLocked := false, false
	for _, choice := range choices {
		if choice.Provider == "fixture" && choice.Model == "model-two" {
			seenAlias = choice.Label == "Friendly model" && choice.Unavailable == ""
		}
		if choice.Provider == "locked" {
			seenLocked = choice.Unavailable != ""
		}
	}
	if !seenAlias || !seenLocked {
		t.Fatal("incorrect configured model metadata")
	}
	encoded, _ := json.Marshal(choices)
	if strings.Contains(string(encoded), "private-endpoint") || strings.Contains(string(encoded), "GATOR_COMPOSER_TEST_KEY") {
		t.Fatal("picker exposed private configuration")
	}
}
