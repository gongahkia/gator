package model

import (
	"net/http"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/auth"
)

// Provider identifies a direct model backend or a retained legacy session
// provider that now fails without a fallback.
type Provider string

const (
	OpenAI                  Provider = "openai"
	AzureOpenAI             Provider = "azure-openai"
	AzureOpenAIResponses    Provider = "azure-openai-responses"
	Anthropic               Provider = "anthropic"
	Gemini                  Provider = "gemini"
	Mistral                 Provider = "mistral"
	XAI                     Provider = "xai"
	Groq                    Provider = "groq"
	OpenRouter              Provider = "openrouter"
	Together                Provider = "together"
	Fireworks               Provider = "fireworks"
	DeepSeek                Provider = "deepseek"
	Cerebras                Provider = "cerebras"
	NVIDIA                  Provider = "nvidia"
	HuggingFace             Provider = "huggingface"
	MoonshotAI              Provider = "moonshotai"
	ZAI                     Provider = "zai"
	ZAICodingCN             Provider = "zai-coding-cn"
	MiniMax                 Provider = "minimax"
	MiniMaxCN               Provider = "minimax-cn"
	Baseten                 Provider = "baseten"
	VercelAIGateway         Provider = "vercel-ai-gateway"
	AntLing                 Provider = "ant-ling"
	Xiaomi                  Provider = "xiaomi"
	MoonshotAICN            Provider = "moonshotai-cn"
	CloudflareWorkers       Provider = "cloudflare-workers-ai"
	CloudflareGateway       Provider = "cloudflare-ai-gateway"
	AmazonBedrock           Provider = "amazon-bedrock"
	GoogleVertex            Provider = "google-vertex"
	QwenTokenPlan           Provider = "qwen-token-plan"
	QwenTokenPlanCN         Provider = "qwen-token-plan-cn"
	QwenTokenPlanIndividual Provider = "qwen-token-plan-individual"
	XiaomiTokenPlanCN       Provider = "xiaomi-token-plan-cn"
	XiaomiTokenPlanAMS      Provider = "xiaomi-token-plan-ams"
	XiaomiTokenPlanSGP      Provider = "xiaomi-token-plan-sgp"
	OpenAICompatible        Provider = "openai-compatible"
	Codex                   Provider = "codex"
	Claude                  Provider = "claude"
	Copilot                 Provider = "copilot"
	KimiCoding              Provider = "kimi-coding"
	Radius                  Provider = "radius"
	OpenCode                Provider = "opencode"
	OpenCodeGo              Provider = "opencode-go"
	Cursor                  Provider = "cursor"
)

// Config selects one provider. APIKey is optional only because the factory can
// obtain the provider's documented environment variable when it is unset.
type Config struct {
	Provider                  Provider
	Model                     string
	BaseURL                   string
	APIKey                    string
	Credentials               *auth.Store
	Client                    *http.Client
	CloudflareAccountID       string
	CloudflareGatewayID       string
	CloudflareGatewayProtocol string
}

// Backend contains a direct model adapter. The agent and tool loop remain in
// Gator for every supported provider.
type Backend struct {
	Provider Provider
	Model    agent.Model
}
