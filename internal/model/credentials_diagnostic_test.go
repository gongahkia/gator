package model

import (
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model/openai"
	"testing"
)

func TestCredentialDiagnosticSourcePreservesExistingPriority(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "environment-key")
	store, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("openai", auth.Credential{Type: "api_key", Key: "stored-key"}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		config      Config
		key, source string
	}{
		{Config{Provider: OpenAI}, "environment-key", "environment"},
		{Config{Provider: OpenAI, Credentials: &store}, "stored-key", "gator-store"},
		{Config{Provider: OpenAI, Credentials: &store, APIKey: "explicit-key"}, "explicit-key", "explicit"},
	} {
		backend, err := New(test.config)
		if err != nil {
			t.Fatal(err)
		}
		adapter := backend.Model.(openai.Responses)
		if adapter.APIKey != test.key || adapter.CredentialSource != test.source {
			t.Fatalf("unexpected credential source: %s", adapter.CredentialSource)
		}
	}
}
