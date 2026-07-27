package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"golang.org/x/oauth2"
)

type testBedrockClient struct{ input *bedrockruntime.ConverseInput }

func (c *testBedrockClient) Converse(_ context.Context, input *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	c.input = input
	return &bedrockruntime.ConverseOutput{Output: &bedrocktypes.ConverseOutputMemberMessage{Value: bedrocktypes.Message{Role: bedrocktypes.ConversationRoleAssistant, Content: []bedrocktypes.ContentBlock{&bedrocktypes.ContentBlockMemberText{Value: "planned"}}}}, StopReason: bedrocktypes.StopReasonEndTurn, Usage: &bedrocktypes.TokenUsage{InputTokens: aws.Int32(11), OutputTokens: aws.Int32(7), CacheReadInputTokens: aws.Int32(3)}}, nil
}

func TestOpenAIResponsesAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing auth")
		}
		w.Header().Set("x-ratelimit-remaining-requests", "7")
		w.Header().Set("x-ratelimit-reset-requests", "1s")
		_, _ = io.WriteString(w, `{"id":"resp_1","output_text":"planned"}`)
	}))
	defer server.Close()
	t.Setenv("TEST_OPENAI", "test-key")
	invoker := Invoker{HTTPClient: server.Client()}
	result, err := invoker.Invoke(context.Background(), config.Provider{ID: "openai", Kind: "openai_responses", Model: "model", BaseURL: server.URL, CredentialEnv: "TEST_OPENAI"}, Request{RunID: "run", Stage: domain.StagePlanner, Prompt: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "planned" || result.Provider != "openai" {
		t.Fatalf("unexpected result %#v", result)
	}
	if result.RateLimit.RemainingRequests == nil || *result.RateLimit.RemainingRequests != 7 || result.RateLimit.ResetAt == nil {
		t.Fatalf("rate limit=%#v", result.RateLimit)
	}
	_ = os.Unsetenv("TEST_OPENAI")
}

func TestMissingCredentialFailsClosed(t *testing.T) {
	invoker := Invoker{}
	_, err := invoker.Invoke(context.Background(), config.Provider{ID: "openai", Kind: "openai_responses", Model: "model", BaseURL: "https://example.test", CredentialEnv: "MISSING_TEST_KEY"}, Request{})
	if err == nil {
		t.Fatal("expected credential failure")
	}
}

func TestAzureOpenAIResponsesAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" || r.Header.Get("api-key") != "test-key" {
			t.Fatalf("path=%s api-key=%q", r.URL.Path, r.Header.Get("api-key"))
		}
		_, _ = io.WriteString(w, `{"id":"resp_1","output_text":"planned"}`)
	}))
	defer server.Close()
	t.Setenv("TEST_AZURE_OPENAI", "test-key")
	result, err := (Invoker{HTTPClient: server.Client()}).Invoke(context.Background(), config.Provider{ID: "azure", Kind: "azure_openai_responses", Model: "deployment", BaseURL: server.URL, CredentialEnv: "TEST_AZURE_OPENAI"}, Request{Prompt: "plan"})
	if err != nil || result.Text != "planned" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestCohereAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/chat" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("path=%s authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"id":"chat_1","message":{"content":[{"type":"text","text":"planned"}]},"usage":{"tokens":{"input_tokens":11,"output_tokens":7}}}`)
	}))
	defer server.Close()
	t.Setenv("TEST_COHERE", "test-key")
	result, err := (Invoker{HTTPClient: server.Client()}).Invoke(context.Background(), config.Provider{ID: "cohere", Kind: "cohere_v2_chat", Model: "command", BaseURL: server.URL, CredentialEnv: "TEST_COHERE"}, Request{Prompt: "plan"})
	if err != nil || result.Text != "planned" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestOllamaAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" || r.Header.Get("Authorization") != "" {
			t.Fatalf("path=%s authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"model":"qwen","message":{"role":"assistant","content":"planned"},"prompt_eval_count":11,"eval_count":7}`)
	}))
	defer server.Close()
	result, err := (Invoker{HTTPClient: server.Client()}).Invoke(context.Background(), config.Provider{ID: "ollama", Kind: "ollama_chat", Model: "qwen", BaseURL: server.URL}, Request{Prompt: "plan"})
	if err != nil || result.Text != "planned" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestVertexAIAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects/project/locations/us-central1/publishers/google/models/gemini:generateContent" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("path=%s authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"planned"}]}}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7}}`)
	}))
	defer server.Close()
	result, err := (Invoker{HTTPClient: server.Client(), GoogleTokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})}).Invoke(context.Background(), config.Provider{ID: "vertex", Kind: "vertex_ai_generate_content", Model: "gemini", BaseURL: server.URL, Project: "project", Region: "us-central1"}, Request{Prompt: "plan"})
	if err != nil || result.Text != "planned" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestOpenAICompatibleAllowsLocalEndpointWithoutCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "" {
			t.Fatalf("path=%s authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"id":"chat_1","choices":[{"message":{"content":"planned"}}]}`)
	}))
	defer server.Close()
	result, err := (Invoker{HTTPClient: server.Client()}).Invoke(context.Background(), config.Provider{ID: "local", Kind: "openai_compatible", Model: "local", BaseURL: server.URL}, Request{Prompt: "plan"})
	if err != nil || result.Text != "planned" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestBedrockConverseAdapter(t *testing.T) {
	client := &testBedrockClient{}
	result, err := (Invoker{Bedrock: client}).Invoke(context.Background(), config.Provider{ID: "bedrock", Kind: "aws_bedrock_converse", Model: "amazon.nova-lite-v1:0", Region: "us-east-1"}, Request{Prompt: "plan"})
	if err != nil || result.Text != "planned" || result.Usage.InputTokens == nil || *result.Usage.InputTokens != 11 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if client.input == nil || aws.ToString(client.input.ModelId) != "amazon.nova-lite-v1:0" {
		t.Fatalf("input=%#v", client.input)
	}
}
