package llm

import (
	"bytes"
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pawlog "github.com/gongahkia/paw/internal/log"
)

func TestTLSConfigForHTTPSLLMTransports(t *testing.T) {
	oldBackoffs := retryBackoffs
	retryBackoffs = nil
	t.Cleanup(func() { retryBackoffs = oldBackoffs })

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"openai ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
		case "/v1/messages":
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"anthropic ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caFile, certPEM, 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	tests := []struct {
		name    string
		brain   EndpointConfig
		content string
	}{
		{
			name: "openai",
			brain: EndpointConfig{
				Transport: "openai",
				BaseURL:   srv.URL,
				Model:     "gpt-test",
			},
			content: "openai ok",
		},
		{
			name: "anthropic",
			brain: EndpointConfig{
				Transport: "anthropic",
				BaseURL:   srv.URL,
				APIKey:    "test-key",
				Model:     "claude-test",
			},
			content: "anthropic ok",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/default verify fails", func(t *testing.T) {
			client, err := NewBrainClient(FactoryConfig{
				Brain:       tt.brain,
				CallTimeout: 2 * time.Second,
			})
			if err != nil {
				t.Fatalf("brain client: %v", err)
			}
			_, err = client.Chat(context.Background(), ChatRequest{
				Messages: []ChatMessage{{Role: "user", Content: "tls"}},
			})
			if err == nil || !strings.Contains(err.Error(), "certificate") {
				t.Fatalf("expected certificate error, got %v", err)
			}
		})

		t.Run(tt.name+"/ca file succeeds", func(t *testing.T) {
			resp := tlsChat(t, tt.brain, TLSConfig{CAFile: caFile})
			if resp.Content != tt.content {
				t.Fatalf("content = %q", resp.Content)
			}
		})

		t.Run(tt.name+"/skip verify succeeds and warns", func(t *testing.T) {
			var warnings bytes.Buffer
			logger, err := pawlog.New(pawlog.Config{Writer: &warnings})
			if err != nil {
				t.Fatalf("logger: %v", err)
			}

			resp := tlsChatContext(t, pawlog.With(context.Background(), logger), tt.brain, TLSConfig{InsecureSkipVerify: true})
			if resp.Content != tt.content {
				t.Fatalf("content = %q", resp.Content)
			}
			if !strings.Contains(warnings.String(), `msg="TLS verification disabled"`) {
				t.Fatalf("missing warning: %q", warnings.String())
			}
		})
	}
}

func tlsChat(t *testing.T, brain EndpointConfig, tlsConfig TLSConfig) *ChatResponse {
	t.Helper()
	return tlsChatContext(t, context.Background(), brain, tlsConfig)
}

func tlsChatContext(t *testing.T, ctx context.Context, brain EndpointConfig, tlsConfig TLSConfig) *ChatResponse {
	t.Helper()
	client, err := NewBrainClient(FactoryConfig{
		Brain:       brain,
		TLS:         tlsConfig,
		CallTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("brain client: %v", err)
	}
	resp, err := client.Chat(ctx, ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "tls"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	return resp
}
