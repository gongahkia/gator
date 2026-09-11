package model

import (
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/model/chatcompletions"
)

func compatibleConfig(provider Provider, config Config) (chatcompletions.Config, error) {
	definition, ok := compatibleProviders[provider]
	if !ok {
		return chatcompletions.Config{}, fmt.Errorf("provider %q is not OpenAI-compatible", provider)
	}
	apiKey, err := key(config, provider, definition.apiKeyEnv)
	if err != nil {
		return chatcompletions.Config{}, err
	}
	if err := requireKey(apiKey, definition.apiKeyEnv); err != nil {
		return chatcompletions.Config{}, err
	}
	baseURL := config.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = definition.baseURL
	}
	if strings.TrimSpace(config.Model) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--model is required for provider %q", provider)
	}
	if strings.TrimSpace(baseURL) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--base-url or GATOR_BASE_URL is required for provider %q", provider)
	}
	return chatcompletions.Config{
		APIKey:              apiKey,
		APIKeyEnv:           definition.apiKeyEnv,
		BaseURL:             baseURL,
		Model:               config.Model,
		ProviderName:        definition.name,
		AuthorizationHeader: definition.authorizationHeader,
		AuthorizationPrefix: definition.authorizationPrefix,
		Client:              config.Client,
	}, nil
}

type compatibleProvider struct {
	name                string
	apiKeyEnv           string
	baseURL             string
	authorizationHeader string
	authorizationPrefix string
}

var compatibleProviders = map[Provider]compatibleProvider{
	AzureOpenAI:             {name: "Azure OpenAI API", apiKeyEnv: "AZURE_OPENAI_API_KEY", baseURL: "", authorizationHeader: "api-key"},
	Mistral:                 {name: "Mistral API", apiKeyEnv: "MISTRAL_API_KEY", baseURL: "https://api.mistral.ai/v1/chat/completions"},
	XAI:                     {name: "xAI API", apiKeyEnv: "XAI_API_KEY", baseURL: "https://api.x.ai/v1/chat/completions"},
	Groq:                    {name: "Groq API", apiKeyEnv: "GROQ_API_KEY", baseURL: "https://api.groq.com/openai/v1/chat/completions"},
	OpenRouter:              {name: "OpenRouter API", apiKeyEnv: "OPENROUTER_API_KEY", baseURL: "https://openrouter.ai/api/v1/chat/completions"},
	Together:                {name: "Together AI API", apiKeyEnv: "TOGETHER_API_KEY", baseURL: "https://api.together.xyz/v1/chat/completions"},
	Fireworks:               {name: "Fireworks AI API", apiKeyEnv: "FIREWORKS_API_KEY", baseURL: "https://api.fireworks.ai/inference/v1/chat/completions"},
	DeepSeek:                {name: "DeepSeek API", apiKeyEnv: "DEEPSEEK_API_KEY", baseURL: "https://api.deepseek.com/chat/completions"},
	Cerebras:                {name: "Cerebras Inference", apiKeyEnv: "CEREBRAS_API_KEY", baseURL: "https://api.cerebras.ai/v1/chat/completions"},
	NVIDIA:                  {name: "NVIDIA NIM", apiKeyEnv: "NVIDIA_API_KEY", baseURL: "https://integrate.api.nvidia.com/v1/chat/completions"},
	HuggingFace:             {name: "Hugging Face Inference Providers", apiKeyEnv: "HF_TOKEN", baseURL: "https://router.huggingface.co/v1/chat/completions"},
	MoonshotAI:              {name: "Moonshot AI Kimi API", apiKeyEnv: "MOONSHOT_API_KEY", baseURL: "https://api.moonshot.ai/v1/chat/completions"},
	ZAI:                     {name: "Z.AI GLM Coding Plan", apiKeyEnv: "ZAI_API_KEY", baseURL: "https://api.z.ai/api/coding/paas/v4/chat/completions"},
	ZAICodingCN:             {name: "Z.AI GLM Coding Plan (China)", apiKeyEnv: "ZAI_CODING_CN_API_KEY", baseURL: "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions"},
	Baseten:                 {name: "Baseten Inference", apiKeyEnv: "BASETEN_API_KEY", baseURL: "https://inference.baseten.co/v1/chat/completions", authorizationPrefix: "Api-Key "},
	VercelAIGateway:         {name: "Vercel AI Gateway", apiKeyEnv: "AI_GATEWAY_API_KEY", baseURL: "https://ai-gateway.vercel.sh/v1/chat/completions"},
	AntLing:                 {name: "Ant Ling API", apiKeyEnv: "ANT_LING_API_KEY", baseURL: "https://api.ant-ling.com/v1/chat/completions"},
	Xiaomi:                  {name: "Xiaomi MiMo API", apiKeyEnv: "MIMO_API_KEY", baseURL: "https://api.xiaomimimo.com/v1/chat/completions"},
	MoonshotAICN:            {name: "Moonshot AI Kimi API (China)", apiKeyEnv: "MOONSHOT_API_KEY", baseURL: "https://api.moonshot.cn/v1/chat/completions"},
	QwenTokenPlan:           {name: "Qwen Token Plan", apiKeyEnv: "QWEN_TOKEN_PLAN_API_KEY", baseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1/chat/completions"},
	QwenTokenPlanCN:         {name: "Qwen Token Plan (China)", apiKeyEnv: "QWEN_TOKEN_PLAN_CN_API_KEY", baseURL: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/chat/completions"},
	QwenTokenPlanIndividual: {name: "Qwen Token Plan (Individual)", apiKeyEnv: "QWEN_TOKEN_PLAN_API_KEY", baseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1/chat/completions"},
	XiaomiTokenPlanCN:       {name: "Xiaomi MiMo Token Plan (China)", apiKeyEnv: "MIMO_API_KEY", baseURL: "https://token-plan-cn.xiaomimimo.com/v1/chat/completions"},
	XiaomiTokenPlanAMS:      {name: "Xiaomi MiMo Token Plan (Amsterdam)", apiKeyEnv: "MIMO_API_KEY", baseURL: "https://token-plan-ams.xiaomimimo.com/v1/chat/completions"},
	XiaomiTokenPlanSGP:      {name: "Xiaomi MiMo Token Plan (Singapore)", apiKeyEnv: "MIMO_API_KEY", baseURL: "https://token-plan-sgp.xiaomimimo.com/v1/chat/completions"},
	OpenAICompatible:        {name: "OpenAI-compatible API", apiKeyEnv: "GATOR_COMPATIBLE_API_KEY", baseURL: ""},
}

// ParseProvider validates a user-visible provider name.
