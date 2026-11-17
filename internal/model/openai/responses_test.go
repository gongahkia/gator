package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/telemetry"
)

func TestResponsesCompleteConvertsToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "test-model" || body.Store || len(body.Tools) != 1 || body.Tools[0].Name != "read_file" {
			t.Fatalf("request = %#v", body)
		}
		if body.Tools[0].Strict == nil || *body.Tools[0].Strict {
			t.Fatal("tool must explicitly send strict:false to preserve optional fields")
		}
		_, _ = io.WriteString(writer, `{"output":[{"id":"fc_item","type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"README.md\"}"}]}`)
	}))
	defer server.Close()

	model := Responses{APIKey: "test-key", Model: "test-model", BaseURL: server.URL, Client: server.Client()}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{
		System:   "be precise",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "read the readme"}},
		Tools:    []agent.ToolDefinition{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", turn.ToolCalls)
	}
	call := turn.ToolCalls[0]
	if call.ID != "call_1" || call.ProviderID != "fc_item" || call.Name != "read_file" || string(call.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("tool call = %#v", call)
	}
}

func TestResponsesUsesStructuredComputerToolAndContinuation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !body.Store || body.PreviousResponseID != "" || len(body.Tools) != 1 || body.Tools[0].Type != "computer" || body.Tools[0].Name != "" || len(body.Input) != 1 {
			t.Fatalf("computer request = %#v", body)
		}
		_, _ = io.WriteString(writer, `{"id":"resp-first","output":[{"id":"computer-item","type":"computer_call","call_id":"computer-call","actions":[{"type":"click","button":"left","x":20,"y":30}]}]}`)
	}))
	defer server.Close()
	model := Responses{APIKey: "test-key", BaseURL: server.URL, Client: server.Client()}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "use the approved app"}},
		Tools:    []agent.ToolDefinition{{Name: "computer_action", Parameters: json.RawMessage(`{"type":"object"}`)}},
		Computer: &agent.ComputerUse{Environment: "computer", DisplayWidth: 1440, DisplayHeight: 900, RetainState: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Kind != agent.ToolCallComputer || turn.ToolCalls[0].Name != "computer_action" || string(turn.ToolCalls[0].Arguments) != `{"actions":[{"type":"click","button":"left","x":20,"y":30}]}` || string(turn.ProviderData) != `{"response_id":"resp-first"}` {
		t.Fatalf("computer turn = %#v", turn)
	}
}

func TestResponsesReplaysComputerCallWithScreenshotOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.PreviousResponseID != "resp_abc" || len(body.Input) != 1 || body.Input[0].Type != "computer_call_output" {
			t.Fatalf("computer replay = %#v", body.Input)
		}
		output, ok := body.Input[0].Output.(map[string]any)
		if !ok || output["type"] != "computer_screenshot" || output["detail"] != "original" || !strings.HasPrefix(output["image_url"].(string), "data:image/png;base64,") {
			t.Fatalf("computer output = %#v", body.Input[0].Output)
		}
		_, _ = io.WriteString(writer, `{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`)
	}))
	defer server.Close()
	model := Responses{APIKey: "test-key", BaseURL: server.URL, Client: server.Client()}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{
		{Role: agent.RoleUser, Content: "inspect"},
		{Role: agent.RoleAgent, ProviderData: json.RawMessage(`{"response_id":"resp_abc"}`), ToolCalls: []agent.ToolCall{{ID: "computer-call", ProviderID: "computer-item", Kind: agent.ToolCallComputer, Name: "computer_action", Arguments: json.RawMessage(`{"actions":[{"type":"screenshot"}]}`)}}},
		{Role: agent.RoleTool, ToolCallID: "computer-call", ToolName: "computer_action", Images: []agent.Image{{Name: "window.png", MediaType: "image/png", Data: []byte("png")}}},
	}, Computer: &agent.ComputerUse{RetainState: true}})
	if err != nil || turn.Text != "done" {
		t.Fatalf("complete = %#v, %v", turn, err)
	}
}

func TestResponsesComputerUseRequiresExplicitRetentionConsent(t *testing.T) {
	_, err := (Responses{}).requestBody(agent.TurnRequest{
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect"}},
		Computer: &agent.ComputerUse{},
	})
	if err == nil || !strings.Contains(err.Error(), "retention consent") {
		t.Fatalf("computer request error = %v", err)
	}
}

func TestResponsesUsesConfiguredAPIKeyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("api-key"); got != "azure-key" {
			t.Fatalf("api-key = %q", got)
		}
		if got := request.Header.Get("Authorization"); got != "" {
			t.Fatalf("unexpected authorization = %q", got)
		}
		_, _ = io.WriteString(writer, `{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`)
	}))
	defer server.Close()

	model := Responses{
		APIKey:              "azure-key",
		APIKeyEnv:           "AZURE_OPENAI_API_KEY",
		Model:               "deployment",
		BaseURL:             server.URL,
		AuthorizationHeader: "api-key",
		Client:              server.Client(),
	}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}})
	if err != nil || turn.Text != "done" {
		t.Fatalf("complete = %#v, %v", turn, err)
	}
}

func TestResponsesUsesConfiguredBearerAuthorizationHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer entra-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := request.Header.Get("api-key"); got != "" {
			t.Fatalf("unexpected api-key = %q", got)
		}
		_, _ = io.WriteString(writer, `{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`)
	}))
	defer server.Close()

	model := Responses{
		APIKey:              "entra-token",
		APIKeyEnv:           "AZURE_OPENAI_AUTH_TOKEN",
		Model:               "deployment",
		BaseURL:             server.URL,
		AuthorizationHeader: "Authorization",
		AuthorizationPrefix: "Bearer ",
		Client:              server.Client(),
	}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}})
	if err != nil || turn.Text != "done" {
		t.Fatalf("complete = %#v, %v", turn, err)
	}
}

func TestResponsesCompleteReplaysFunctionCallAndOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body.Input) != 3 {
			t.Fatalf("input = %#v", body.Input)
		}
		call, output := body.Input[1], body.Input[2]
		if call.Type != "function_call" || call.ID != "fc_item" || call.CallID != "call_1" || call.Arguments != `{"path":"README.md"}` {
			t.Fatalf("replayed call = %#v", call)
		}
		if output.Type != "function_call_output" || output.CallID != "call_1" || output.Output != `{"ok":true}` {
			t.Fatalf("replayed output = %#v", output)
		}
		_, _ = io.WriteString(writer, `{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`)
	}))
	defer server.Close()

	model := Responses{APIKey: "test-key", BaseURL: server.URL, Client: server.Client()}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{
		{Role: agent.RoleUser, Content: "read the readme"},
		{Role: agent.RoleAgent, ToolCalls: []agent.ToolCall{{ID: "call_1", ProviderID: "fc_item", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
		{Role: agent.RoleTool, ToolCallID: "call_1", ToolName: "read_file", Content: `{"ok":true}`},
	}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if turn.Text != "done" || len(turn.ToolCalls) != 0 {
		t.Fatalf("turn = %#v", turn)
	}
}

func TestResponsesEncodesImageInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		content, ok := body.Input[0].Content.([]any)
		if !ok || len(content) != 2 {
			t.Fatalf("image content = %#v", body.Input[0].Content)
		}
		_, _ = io.WriteString(writer, `{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`)
	}))
	defer server.Close()
	model := Responses{APIKey: "test-key", BaseURL: server.URL, Client: server.Client()}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect", Images: []agent.Image{{Name: "screen.png", MediaType: "image/png", Data: []byte("png")}}}}}); err != nil {
		t.Fatalf("complete image input: %v", err)
	}
}

func TestResponsesEncodesPDFAttachment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Input []struct {
				Content []struct {
					Type     string `json:"type"`
					FileData string `json:"file_data"`
					Filename string `json:"filename"`
				} `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || len(body.Input) != 1 || len(body.Input[0].Content) != 2 || body.Input[0].Content[1].Type != "input_file" || body.Input[0].Content[1].Filename != "report.pdf" || body.Input[0].Content[1].FileData != "cGRm" {
			t.Fatalf("PDF request = %#v, err = %v", body, err)
		}
		_, _ = io.WriteString(writer, `{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`)
	}))
	defer server.Close()
	model := Responses{APIKey: "test-key", BaseURL: server.URL, Client: server.Client()}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect", Attachments: []agent.Attachment{{Name: "report.pdf", MediaType: "application/pdf", Data: []byte("pdf")}}}}}); err != nil {
		t.Fatalf("complete PDF attachment: %v", err)
	}
}

func TestResponsesCompleteDescribesAPIErrorsWithoutKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, `{"error":{"message":"rate limit reached"}}`)
	}))
	defer server.Close()

	model := Responses{APIKey: "do-not-leak", BaseURL: server.URL, Client: server.Client()}
	_, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}})
	if err == nil || !strings.Contains(err.Error(), "HTTP 429: rate limit reached") || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("API error = %v", err)
	}
	if !agent.IsTransient(err) {
		t.Fatalf("429 was not marked transient: %v", err)
	}
}

func TestResponsesRequiresAPIKey(t *testing.T) {
	_, err := (Responses{}).Complete(context.Background(), agent.TurnRequest{})
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("missing-key error = %v", err)
	}
}

func TestResponsesCompleteStreamForwardsDeltasAndReturnsCompletedTurn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode stream request: %v", err)
		}
		if !body.Stream || request.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("stream request = %#v, accept = %q", body, request.Header.Get("Accept"))
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "event: response.output_text.delta\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello \"}\n\n")
		_, _ = io.WriteString(writer, "event: response.output_text.delta\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n")
		_, _ = io.WriteString(writer, "event: response.completed\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello world\"}]}]}}\n\n")
	}))
	defer server.Close()

	model := Responses{APIKey: "test-key", BaseURL: server.URL, Client: server.Client()}
	var deltas []string
	turn, err := model.CompleteStream(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}}, func(delta string) { deltas = append(deltas, delta) })
	if err != nil {
		t.Fatalf("complete stream: %v", err)
	}
	if strings.Join(deltas, "") != "hello world" || turn.Text != "hello world" {
		t.Fatalf("deltas = %#v, turn = %#v", deltas, turn)
	}
}

func TestProviderFailureSurvivesRunnerAndPrivateTraceWithoutRetryOrContent(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("x-request-id", "req_diagnostic")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":{"code":"invalid_api_key","type":"invalid_request_error","message":"sk-proj-DO_NOT_RETAIN PRIVATE_PROMPT"}}`)
			}))
			defer server.Close()
			adapter := Responses{APIKey: "sk-proj-DO_NOT_RETAIN", Model: "test-model", BaseURL: server.URL, Client: server.Client()}
			var backend agent.Model = adapter
			if !streaming {
				backend = completeOnly{adapter}
			}
			trace := telemetry.New("diagnostic-run", time.Now())
			_, err := (agent.Runner{Model: backend}).Run(context.Background(), agent.RunOptions{Task: "PRIVATE_PROMPT", MaxSteps: 1, OnEvent: trace.Record})
			var status agent.ProviderHTTPError
			if calls != 1 || !errors.As(err, &status) || status.Status != 401 || agent.IsTransient(err) {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
			errorText := err.Error()
			d := agent.ProviderDiagnosticFrom(err)
			if d == nil || d.Streaming != streaming || d.RequestID != "req_diagnostic" || d.Code != "invalid_api_key" || d.Model != "test-model" {
				t.Fatalf("diagnostic = %+v", d)
			}
			directory := t.TempDir()
			if err := trace.Finish(directory, ""); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "trace.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
				t.Fatalf("trace permissions: %v", err)
			}
			if !strings.Contains(string(data), `"request_id": "req_diagnostic"`) || !strings.Contains(string(data), `"code": "invalid_api_key"`) || strings.Contains(string(data)+errorText, "PRIVATE_PROMPT") || strings.Contains(string(data)+errorText, "DO_NOT_RETAIN") {
				t.Fatalf("unsafe or missing trace diagnostics: %s", data)
			}
		})
	}
}

type completeOnly struct{ model Responses }

func (m completeOnly) Complete(ctx context.Context, request agent.TurnRequest) (agent.Turn, error) {
	return m.model.Complete(ctx, request)
}
