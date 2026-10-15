package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

func TestGenerateContentPreservesCandidateDataForToolReplay(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("x-goog-api-key") != "test-key" || !strings.HasSuffix(request.URL.Path, "/models/gemini-test:generateContent") {
			t.Fatalf("request = %#v", request)
		}
		var body struct {
			Contents []struct {
				Role  string            `json:"role"`
				Parts []json.RawMessage `json:"parts"`
			} `json:"contents"`
			Tools      []json.RawMessage `json:"tools"`
			ToolConfig *struct {
				FunctionCallingConfig struct {
					Mode string `json:"mode"`
				} `json:"functionCallingConfig"`
			} `json:"toolConfig"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if requests == 1 {
			if len(body.Contents) != 1 || len(body.Tools) != 1 || body.ToolConfig == nil || body.ToolConfig.FunctionCallingConfig.Mode != "VALIDATED" {
				t.Fatalf("first request = %#v", body)
			}
			_, _ = io.WriteString(writer, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"read_file","args":{"path":"README.md"}},"thoughtSignature":"opaque-signature"}]}}]}`)
			return
		}
		if len(body.Contents) != 3 || body.Contents[1].Role != "model" || body.Contents[2].Role != "user" || body.ToolConfig != nil {
			t.Fatalf("replayed contents = %#v", body.Contents)
		}
		if !strings.Contains(string(body.Contents[1].Parts[0]), "opaque-signature") || !strings.Contains(string(body.Contents[2].Parts[0]), "functionResponse") {
			t.Fatalf("provider data was not faithfully replayed: %#v", body.Contents)
		}
		_, _ = io.WriteString(writer, `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]}}]}`)
	}))
	defer server.Close()

	model := GenerateContent{APIKey: "test-key", Model: "gemini-test", BaseURL: server.URL, Client: server.Client()}
	first, err := model.Complete(context.Background(), agent.TurnRequest{
		System:   "be precise",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "read the readme"}},
		Tools:    []agent.ToolDefinition{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("first complete: %v", err)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "call_1" || !strings.Contains(string(first.ProviderData), "opaque-signature") {
		t.Fatalf("first turn = %#v", first)
	}
	second, err := model.Complete(context.Background(), agent.TurnRequest{
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "read the readme"},
			{Role: agent.RoleAgent, ToolCalls: first.ToolCalls, ProviderData: first.ProviderData},
			{Role: agent.RoleTool, ToolCallID: "call_1", ToolName: "read_file", Content: `{"ok":true}`},
		},
	})
	if err != nil {
		t.Fatalf("second complete: %v", err)
	}
	if second.Text != "done" {
		t.Fatalf("second turn = %#v", second)
	}
}

func TestRetryDelayUsesHeaderThenProviderHint(t *testing.T) {
	if got := retryDelay(http.Header{"Retry-After": []string{"2.5"}}, []byte("Please retry in 26.282367284s.")); got != 2500*time.Millisecond {
		t.Fatalf("header retry delay = %s", got)
	}
	if got := retryDelay(nil, []byte(`{"error":{"message":"Please retry in 26.282367284s."}}`)); got < 26*time.Second || got > 27*time.Second {
		t.Fatalf("body retry delay = %s", got)
	}
	if got := retryDelay(nil, []byte(`{"error":{"message":"Please retry in 8h5m27.187517353s."}}`)); got < 8*time.Hour || got > 9*time.Hour {
		t.Fatalf("daily quota retry delay = %s", got)
	}
}

func TestGenerateContentCarriesRetryDelayOnRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "2.5")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, `{"error":{"message":"Please retry in 30s."}}`)
	}))
	defer server.Close()
	model := GenerateContent{APIKey: "test-key", Model: "gemini-test", BaseURL: server.URL, Client: server.Client()}
	_, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}}})
	var retry agent.RetryAfterError
	if !agent.IsTransient(err) || !errors.As(err, &retry) || retry.Delay != 2500*time.Millisecond {
		t.Fatalf("rate limit error = %v, retry = %#v", err, retry)
	}
}

func TestDecodeResponseClassifiesEmptyGeminiTurnsByFinishReason(t *testing.T) {
	for _, tc := range []struct {
		reason    string
		transient bool
		malformed bool
	}{
		{"STOP", true, false},
		{"OTHER", true, false},
		{"MALFORMED_FUNCTION_CALL", true, true},
		{"MAX_TOKENS", false, false},
		{"SAFETY", false, false},
	} {
		payload := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"thought":true}]},"finishReason":"` + tc.reason + `"}]}`)
		_, err := decodeResponse(payload)
		if err == nil || agent.IsTransient(err) != tc.transient || agent.IsMalformedToolCall(err) != tc.malformed || !strings.Contains(err.Error(), tc.reason) {
			t.Errorf("finish reason %s: %v", tc.reason, err)
		}
	}
	_, err := decodeResponse([]byte(`{"promptFeedback":{"blockReason":"SAFETY"}}`))
	if err == nil || agent.IsTransient(err) || !strings.Contains(err.Error(), "SAFETY") {
		t.Fatalf("blocked prompt = %v", err)
	}
	_, err = decodeResponse([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"write_artifact","args":{"path":"partial.md"}}}]},"finishReason":"MALFORMED_FUNCTION_CALL"}]}`))
	if err == nil || !agent.IsMalformedToolCall(err) {
		t.Fatalf("malformed candidate with a partial call was accepted: %v", err)
	}
}

func TestGenerateContentEncodesImageInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Contents []struct {
				Parts []json.RawMessage `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || len(body.Contents) != 1 || len(body.Contents[0].Parts) != 2 || !strings.Contains(string(body.Contents[0].Parts[1]), "inlineData") {
			t.Fatalf("image request = %#v, err = %v", body, err)
		}
		_, _ = io.WriteString(writer, `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]}}]}`)
	}))
	defer server.Close()
	model := GenerateContent{APIKey: "test-key", Model: "gemini-test", BaseURL: server.URL, Client: server.Client()}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect", Images: []agent.Image{{Name: "screen.png", MediaType: "image/png", Data: []byte("png")}}}}}); err != nil {
		t.Fatalf("complete image input: %v", err)
	}
}

func TestGenerateContentEncodesPDFAttachment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Contents []struct {
				Parts []json.RawMessage `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || len(body.Contents) != 1 || len(body.Contents[0].Parts) != 2 || !strings.Contains(string(body.Contents[0].Parts[1]), `"mimeType":"application/pdf"`) || !strings.Contains(string(body.Contents[0].Parts[1]), `"data":"cGRm"`) {
			t.Fatalf("PDF request = %#v, err = %v", body, err)
		}
		_, _ = io.WriteString(writer, `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]}}]}`)
	}))
	defer server.Close()
	model := GenerateContent{APIKey: "test-key", Model: "gemini-test", BaseURL: server.URL, Client: server.Client()}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect", Attachments: []agent.Attachment{{Name: "report.pdf", MediaType: "application/pdf", Data: []byte("pdf")}}}}}); err != nil {
		t.Fatalf("complete PDF attachment: %v", err)
	}
}
