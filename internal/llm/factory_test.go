package llm

import (
	"context"
	"testing"

	"github.com/gongahkia/paw/internal/llm/faketest"
)

func TestBrainClientDefaultsToOllama(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOllama("local", "ok")
	t.Setenv("PAW_BRAIN_BASE_URL", srv.URL)
	t.Setenv("PAW_BRAIN_MODEL", "local-brain")

	client, err := NewBrainClient(FactoryConfig{})
	if err != nil {
		t.Fatalf("brain client: %v", err)
	}
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "local"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if srv.LastRequest().Path != "/api/chat" {
		t.Fatalf("path = %q", srv.LastRequest().Path)
	}
}

func TestBrainOpenAIAllowsLocalNoKey(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOpenAI("local", "ok")
	t.Setenv("PAW_BRAIN_TRANSPORT", "openai")
	t.Setenv("PAW_BRAIN_BASE_URL", srv.URL)
	t.Setenv("PAW_BRAIN_MODEL", "local-openai")

	client, err := NewBrainClient(FactoryConfig{})
	if err != nil {
		t.Fatalf("brain client: %v", err)
	}
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "local"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if got := srv.LastRequest().Header.Get("Authorization"); got != "" {
		t.Fatalf("authorization = %q", got)
	}
}
