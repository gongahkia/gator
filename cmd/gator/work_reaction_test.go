package main

import (
	"context"
	"testing"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestQuickReactionUsesScopeValidationBeforeBypassingModelInitialization(t *testing.T) {
	for _, provider := range []string{"ollama", "openai"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
			state := t.TempDir()
			t.Setenv("GATOR_STATE_DIR", state)
			input, err := captureConversationAdmission(workapp.ConversationInput{Version: 1, RunOptions: workapp.RunOptions{Mode: "inspect", ModelProvider: provider, ModelName: "fixture-not-installed"}})
			if err != nil {
				t.Fatal(err)
			}
			route := routeInteractiveConversation(state)
			decision, err := route(context.Background(), workrun.ConversationRequest{Text: "Thanks!"}, input)
			if err != nil || decision.Reaction != "appreciation" || decision.Text == "" {
				t.Fatal("ack required model initialization or inference", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err = route(ctx, workrun.ConversationRequest{Text: "Thanks!"}, input); err == nil {
				t.Fatal("cancelled route acknowledged")
			}
			store, err := config.DefaultStore()
			if err != nil {
				t.Fatal(err)
			}
			settings, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if settings.ProviderEndpoints == nil {
				settings.ProviderEndpoints = map[string]string{}
			}
			settings.ProviderEndpoints["openai"] = "https://example.invalid/changed"
			if err = store.Save(settings); err != nil {
				t.Fatal(err)
			}
			if d, err := route(context.Background(), workrun.ConversationRequest{Text: "Thanks!"}, input); err == nil || d.Reaction != "" {
				t.Fatal("reaction bypassed changed scope validation")
			}
		})
	}
}
