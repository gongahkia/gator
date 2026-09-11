package main

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
)

func TestCurrentWorkModelStatusReportsConfiguredLocalModel(t *testing.T) {
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: localmodel.ProviderID, Model: "qwen2.5-coder:7b"}
	settings.CustomProviders = []config.CustomProvider{{
		ID: localmodel.ProviderID, BaseURL: "http://127.0.0.1:11434/v1/chat/completions",
		Models: []string{"qwen2.5-coder:7b"}, DefaultModel: "qwen2.5-coder:7b",
	}}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	status, err := currentWorkModelStatus(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider != localmodel.ProviderID || status.Model != "qwen2.5-coder:7b" || status.Access != "local model configured" {
		t.Fatalf("local model status = %#v", status)
	}
}

func TestCurrentWorkModelStatusReportsProviderAuthentication(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "codex", Model: "gpt-5.6"}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	credentials, err := auth.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put("codex", auth.Credential{
		Type: "oauth", Access: "secret-access-token", Expires: time.Now().Add(time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	status, err := currentWorkModelStatus(store, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider != "codex" || status.Model != "gpt-5.6" || status.Access != "logged in (Gator OAuth credential)" {
		t.Fatalf("provider status = %#v", status)
	}
	if status.Access == "secret-access-token" {
		t.Fatal("provider status exposed credential material")
	}
}

func TestCurrentWorkModelStatusReportsAmbientAndMissingCredentials(t *testing.T) {
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "openai", Model: "gpt-5.6"}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OPENAI_API_KEY", "")
	status, err := currentWorkModelStatus(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if status.Access != "not authenticated; requires OPENAI_API_KEY" {
		t.Fatalf("missing credential status = %#v", status)
	}

	t.Setenv("OPENAI_API_KEY", "secret-key")
	status, err = currentWorkModelStatus(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if status.Access != "authenticated via OPENAI_API_KEY" {
		t.Fatalf("ambient credential status = %#v", status)
	}
}
