package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/extension"
	"github.com/gongahkia/norbot/internal/runtime"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type Request struct {
	RunID  string
	Stage  domain.Stage
	Prompt string
}

type Result struct {
	Text      string         `json:"text"`
	Provider  string         `json:"provider"`
	Model     string         `json:"model"`
	Metadata  map[string]any `json:"metadata"`
	RateLimit RateLimit      `json:"rate_limit"`
	Usage     TokenUsage     `json:"usage"`
}

type TokenUsage struct {
	InputTokens  *int `json:"input_tokens,omitempty"`
	OutputTokens *int `json:"output_tokens,omitempty"`
	CachedTokens *int `json:"cached_tokens,omitempty"`
}

type RateLimit struct {
	RemainingRequests *int       `json:"remaining_requests,omitempty"`
	ResetAt           *time.Time `json:"reset_at,omitempty"`
}

type Invoker struct {
	HTTPClient        *http.Client
	Workspace         runtime.WorkspaceBackend
	Extensions        *extension.Registry
	Bedrock           bedrockConverseClient
	GoogleTokenSource oauth2.TokenSource
}

type bedrockConverseClient interface {
	Converse(context.Context, *bedrockruntime.ConverseInput, ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

func (i Invoker) Invoke(ctx context.Context, provider config.Provider, request Request) (Result, error) {
	if provider.Kind == "plugin" {
		if i.Extensions == nil {
			return Result{}, fmt.Errorf("process plugin registry is not configured")
		}
		adapter, ok := i.Extensions.Providers[provider.ID]
		if !ok || !adapter.Supports(request.Stage) {
			return Result{}, fmt.Errorf("plugin provider %q does not support %s", provider.ID, request.Stage)
		}
		response, err := adapter.Invoke(ctx, extension.Request{RunID: request.RunID, Stage: request.Stage, Prompt: request.Prompt})
		if err != nil {
			return Result{}, err
		}
		return Result{Text: response.Text, Provider: provider.ID, Model: "process-plugin", Metadata: response.Metadata}, nil
	}
	if provider.Kind == "cli" {
		output, err := i.Workspace.RunCLI(ctx, request.RunID, request.Stage, provider.Image, provider.Network, provider.Command, request.Prompt, provider.CredentialEnv, provider.KubernetesSecret, provider.KubernetesSecretKey)
		if err != nil {
			return Result{}, err
		}
		return Result{Text: output, Provider: provider.ID, Model: strings.Join(provider.Command, " "), Metadata: map[string]any{"kind": "cli"}}, nil
	}
	key := ""
	if provider.RequiresCredentialEnv() {
		key = strings.TrimSpace(os.Getenv(provider.CredentialEnv))
		if key == "" {
			return Result{}, fmt.Errorf("credential env %s is not configured", provider.CredentialEnv)
		}
	}
	switch provider.Kind {
	case "openai_responses":
		return i.openAIResponses(ctx, provider, key, request)
	case "azure_openai_responses":
		return i.azureOpenAIResponses(ctx, provider, key, request)
	case "openai_compatible":
		return i.openAICompatible(ctx, provider, key, request)
	case "anthropic_messages":
		return i.anthropic(ctx, provider, key, request)
	case "gemini_generate_content":
		return i.gemini(ctx, provider, key, request)
	case "vertex_ai_generate_content":
		return i.vertexAI(ctx, provider, request)
	case "cohere_v2_chat":
		return i.cohere(ctx, provider, key, request)
	case "ollama_chat":
		return i.ollama(ctx, provider, request)
	case "aws_bedrock_converse":
		return i.bedrock(ctx, provider, request)
	default:
		return Result{}, fmt.Errorf("unsupported provider kind %q", provider.Kind)
	}
}

func (i Invoker) openAIResponses(ctx context.Context, p config.Provider, key string, request Request) (Result, error) {
	body := map[string]any{"model": p.Model, "input": request.Prompt}
	payload, rateLimit, err := i.postJSON(ctx, joinURL(p.BaseURL, "/responses"), body, map[string]string{"Authorization": "Bearer " + key})
	if err != nil {
		return Result{}, err
	}
	text := stringField(payload, "output_text")
	if text == "" {
		for _, output := range objectSlice(payload["output"]) {
			for _, content := range objectSlice(output["content"]) {
				if candidate := stringField(content, "text"); candidate != "" {
					text += candidate
				}
			}
		}
	}
	return result(p, text, payload, rateLimit), nil
}

func (i Invoker) azureOpenAIResponses(ctx context.Context, p config.Provider, key string, request Request) (Result, error) {
	body := map[string]any{"model": p.Model, "input": request.Prompt}
	payload, rateLimit, err := i.postJSON(ctx, joinURL(p.BaseURL, "/responses"), body, map[string]string{"api-key": key})
	if err != nil {
		return Result{}, err
	}
	text := stringField(payload, "output_text")
	if text == "" {
		for _, output := range objectSlice(payload["output"]) {
			for _, content := range objectSlice(output["content"]) {
				text += stringField(content, "text")
			}
		}
	}
	return result(p, text, payload, rateLimit), nil
}

func (i Invoker) openAICompatible(ctx context.Context, p config.Provider, key string, request Request) (Result, error) {
	body := map[string]any{"model": p.Model, "messages": []map[string]string{{"role": "user", "content": request.Prompt}}}
	headers := map[string]string{}
	if key != "" {
		headers["Authorization"] = "Bearer " + key
	}
	payload, rateLimit, err := i.postJSON(ctx, joinURL(p.BaseURL, "/chat/completions"), body, headers)
	if err != nil {
		return Result{}, err
	}
	text := ""
	for _, choice := range objectSlice(payload["choices"]) {
		text += stringField(asObject(choice["message"]), "content")
	}
	return result(p, text, payload, rateLimit), nil
}

func (i Invoker) anthropic(ctx context.Context, p config.Provider, key string, request Request) (Result, error) {
	body := map[string]any{"model": p.Model, "max_tokens": 4096, "messages": []map[string]string{{"role": "user", "content": request.Prompt}}}
	payload, rateLimit, err := i.postJSON(ctx, joinURL(p.BaseURL, "/v1/messages"), body, map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"})
	if err != nil {
		return Result{}, err
	}
	text := ""
	for _, content := range objectSlice(payload["content"]) {
		text += stringField(content, "text")
	}
	return result(p, text, payload, rateLimit), nil
}

func (i Invoker) gemini(ctx context.Context, p config.Provider, key string, request Request) (Result, error) {
	body := map[string]any{"contents": []map[string]any{{"parts": []map[string]string{{"text": request.Prompt}}}}}
	payload, rateLimit, err := i.postJSON(ctx, joinURL(p.BaseURL, "/models/"+p.Model+":generateContent?key="+key), body, nil)
	if err != nil {
		return Result{}, err
	}
	text := ""
	for _, candidate := range objectSlice(payload["candidates"]) {
		for _, part := range objectSlice(asObject(candidate["content"])["parts"]) {
			text += stringField(part, "text")
		}
	}
	if usage := asObject(payload["usageMetadata"]); usage != nil {
		payload["usage"] = map[string]any{"prompt_tokens": usage["promptTokenCount"], "completion_tokens": usage["candidatesTokenCount"], "cached_content_token_count": usage["cachedContentTokenCount"]}
	}
	return result(p, text, payload, rateLimit), nil
}

func (i Invoker) vertexAI(ctx context.Context, p config.Provider, request Request) (Result, error) {
	tokenSource := i.GoogleTokenSource
	if tokenSource == nil {
		var err error
		tokenSource, err = google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return Result{}, fmt.Errorf("load Vertex AI credentials: %w", err)
		}
	}
	token, err := tokenSource.Token()
	if err != nil {
		return Result{}, fmt.Errorf("load Vertex AI access token: %w", err)
	}
	body := map[string]any{"contents": []map[string]any{{"parts": []map[string]string{{"text": request.Prompt}}}}}
	endpoint := joinURL(p.BaseURL, "/v1/projects/"+url.PathEscape(p.Project)+"/locations/"+url.PathEscape(p.Region)+"/publishers/google/models/"+url.PathEscape(p.Model)+":generateContent")
	payload, rateLimit, err := i.postJSON(ctx, endpoint, body, map[string]string{"Authorization": "Bearer " + token.AccessToken})
	if err != nil {
		return Result{}, err
	}
	text := ""
	for _, candidate := range objectSlice(payload["candidates"]) {
		for _, part := range objectSlice(asObject(candidate["content"])["parts"]) {
			text += stringField(part, "text")
		}
	}
	if usage := asObject(payload["usageMetadata"]); usage != nil {
		payload["usage"] = map[string]any{"prompt_tokens": usage["promptTokenCount"], "completion_tokens": usage["candidatesTokenCount"], "cached_content_token_count": usage["cachedContentTokenCount"]}
	}
	return result(p, text, payload, rateLimit), nil
}

func (i Invoker) cohere(ctx context.Context, p config.Provider, key string, request Request) (Result, error) {
	body := map[string]any{"model": p.Model, "messages": []map[string]string{{"role": "user", "content": request.Prompt}}}
	payload, rateLimit, err := i.postJSON(ctx, joinURL(p.BaseURL, "/v2/chat"), body, map[string]string{"Authorization": "Bearer " + key})
	if err != nil {
		return Result{}, err
	}
	text := ""
	for _, content := range objectSlice(asObject(payload["message"])["content"]) {
		text += stringField(content, "text")
	}
	return result(p, text, payload, rateLimit), nil
}

func (i Invoker) ollama(ctx context.Context, p config.Provider, request Request) (Result, error) {
	body := map[string]any{"model": p.Model, "messages": []map[string]string{{"role": "user", "content": request.Prompt}}, "stream": false}
	payload, rateLimit, err := i.postJSON(ctx, joinURL(p.BaseURL, "/api/chat"), body, nil)
	if err != nil {
		return Result{}, err
	}
	return result(p, stringField(asObject(payload["message"]), "content"), payload, rateLimit), nil
}

func (i Invoker) bedrock(ctx context.Context, p config.Provider, request Request) (Result, error) {
	client := i.Bedrock
	if client == nil {
		loaded, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(p.Region))
		if err != nil {
			return Result{}, fmt.Errorf("load AWS Bedrock configuration: %w", err)
		}
		client = bedrockruntime.NewFromConfig(loaded)
	}
	output, err := client.Converse(ctx, &bedrockruntime.ConverseInput{ModelId: aws.String(p.Model), Messages: []bedrocktypes.Message{{Role: bedrocktypes.ConversationRoleUser, Content: []bedrocktypes.ContentBlock{&bedrocktypes.ContentBlockMemberText{Value: request.Prompt}}}}, InferenceConfig: &bedrocktypes.InferenceConfiguration{MaxTokens: aws.Int32(4096)}})
	if err != nil {
		return Result{}, err
	}
	text := ""
	if message, ok := output.Output.(*bedrocktypes.ConverseOutputMemberMessage); ok {
		for _, content := range message.Value.Content {
			if block, ok := content.(*bedrocktypes.ContentBlockMemberText); ok {
				text += block.Value
			}
		}
	}
	metadata := map[string]any{"kind": p.Kind, "stop_reason": string(output.StopReason)}
	usage := TokenUsage{}
	if output.Usage != nil {
		usage.InputTokens = int32ToInt(output.Usage.InputTokens)
		usage.OutputTokens = int32ToInt(output.Usage.OutputTokens)
		usage.CachedTokens = int32ToInt(output.Usage.CacheReadInputTokens)
	}
	return Result{Text: text, Provider: p.ID, Model: p.Model, Metadata: metadata, Usage: usage}, nil
}

func (i Invoker) postJSON(ctx context.Context, url string, body any, headers map[string]string) (map[string]any, RateLimit, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, RateLimit{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, RateLimit{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := i.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, RateLimit{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, RateLimit{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, RateLimit{}, fmt.Errorf("provider status %d: %s", response.StatusCode, tail(string(raw), 1000))
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, RateLimit{}, fmt.Errorf("decode provider response: %w", err)
	}
	return payload, rateLimitFromHeaders(response.Header), nil
}

func result(p config.Provider, text string, payload map[string]any, rateLimit RateLimit) Result {
	metadata := map[string]any{"kind": p.Kind, "response_id": stringField(payload, "id")}
	if rateLimit.RemainingRequests != nil {
		metadata["rate_limit_remaining_requests"] = *rateLimit.RemainingRequests
	}
	if rateLimit.ResetAt != nil {
		metadata["rate_limit_reset_at"] = rateLimit.ResetAt.UTC().Format(time.RFC3339)
	}
	usage := usageFromPayload(payload)
	return Result{Text: text, Provider: p.ID, Model: p.Model, Metadata: metadata, RateLimit: rateLimit, Usage: usage}
}

func usageFromPayload(payload map[string]any) TokenUsage {
	raw := asObject(payload["usage"])
	if raw == nil {
		raw = payload
	}
	lookup := func(object map[string]any, keys ...string) *int {
		for _, key := range keys {
			value, ok := object[key]
			if !ok {
				continue
			}
			switch v := value.(type) {
			case float64:
				if v >= 0 {
					n := int(v)
					return &n
				}
			case json.Number:
				if n, err := v.Int64(); err == nil && n >= 0 {
					value := int(n)
					return &value
				}
			}
		}
		return nil
	}
	input := lookup(raw, "input_tokens", "inputTokens", "prompt_tokens", "prompt_eval_count")
	output := lookup(raw, "output_tokens", "outputTokens", "completion_tokens", "candidates_token_count", "eval_count")
	cached := lookup(raw, "cache_read_input_tokens", "cacheReadInputTokens", "cached_content_token_count")
	for _, nestedKey := range []string{"tokens", "billed_units"} {
		nested := asObject(raw[nestedKey])
		if nested == nil {
			continue
		}
		if input == nil {
			input = lookup(nested, "input_tokens", "inputTokens", "prompt_tokens")
		}
		if output == nil {
			output = lookup(nested, "output_tokens", "outputTokens", "completion_tokens")
		}
	}
	if details := asObject(raw["input_tokens_details"]); details != nil && cached == nil {
		if value, ok := details["cached_tokens"].(float64); ok && value >= 0 {
			n := int(value)
			cached = &n
		}
	}
	return TokenUsage{InputTokens: input, OutputTokens: output, CachedTokens: cached}
}

func int32ToInt(value *int32) *int {
	if value == nil || *value < 0 {
		return nil
	}
	converted := int(*value)
	return &converted
}

func rateLimitFromHeaders(headers http.Header) RateLimit {
	result := RateLimit{}
	for _, key := range []string{"x-ratelimit-remaining-requests", "anthropic-ratelimit-requests-remaining", "x-goog-ratelimit-remaining-requests"} {
		if value, err := strconv.Atoi(strings.TrimSpace(headers.Get(key))); err == nil && value >= 0 {
			result.RemainingRequests = &value
			break
		}
	}
	for _, key := range []string{"x-ratelimit-reset-requests", "anthropic-ratelimit-requests-reset", "x-goog-ratelimit-reset"} {
		value := strings.TrimSpace(headers.Get(key))
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			result.ResetAt = &parsed
			break
		}
		if duration, err := time.ParseDuration(value); err == nil {
			reset := time.Now().UTC().Add(duration)
			result.ResetAt = &reset
			break
		}
	}
	return result
}
func joinURL(base, path string) string { return strings.TrimRight(base, "/") + path }
func stringField(object map[string]any, key string) string {
	if value, ok := object[key].(string); ok {
		return value
	}
	return ""
}
func asObject(value any) map[string]any { object, _ := value.(map[string]any); return object }
func objectSlice(value any) []map[string]any {
	raw, _ := value.([]any)
	values := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if object := asObject(item); object != nil {
			values = append(values, object)
		}
	}
	return values
}
func tail(value string, length int) string {
	if len(value) <= length {
		return value
	}
	return value[len(value)-length:]
}
