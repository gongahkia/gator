func bedrockConfig(config Config) (chatcompletions.Config, error) {
	apiKey, err := bearerOrAPIKey(config, AmazonBedrock, "AWS_BEARER_TOKEN_BEDROCK")
	if err != nil {
		return chatcompletions.Config{}, err
	}
	if strings.TrimSpace(config.Model) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--model is required for provider %q", AmazonBedrock)
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if strings.TrimSpace(apiKey) != "" {
		if baseURL == "" {
			region := providerOption(config, "region", "AWS_REGION")
			if region == "" {
				region = "us-east-1"
			}
			baseURL = "https://bedrock-mantle." + region + ".api.aws/v1/chat/completions"
		}
		return chatcompletions.Config{
			APIKey:       apiKey,
			APIKeyEnv:    "AWS_BEARER_TOKEN_BEDROCK",
			BaseURL:      baseURL,
			Model:        config.Model,
			ProviderName: "Amazon Bedrock",
			Client:       config.Client,
		}, nil
	}
	signer, err := bedrock.LoadProfile(
		context.Background(),
		config.Client,
		providerOption(config, "region", "AWS_REGION"),
		providerOption(config, "profile", "AWS_PROFILE"),
	)
	if err != nil {
		return chatcompletions.Config{}, fmt.Errorf("load standard AWS credentials: %w", err)
	}
	if baseURL == "" {
		baseURL = "https://bedrock-runtime." + signer.Region() + ".amazonaws.com/v1/chat/completions"
	}
	return chatcompletions.Config{
		APIKeyEnv:     "AWS_BEARER_TOKEN_BEDROCK or standard AWS credentials",
		BaseURL:       baseURL,
		Model:         config.Model,
		ProviderName:  "Amazon Bedrock",
		RequestSigner: signer.SignRequest,
		Client:        config.Client,
	}, nil
}

func azureResponsesURL(configured string) (string, error) {
	baseURL := strings.TrimSpace(configured)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AZURE_OPENAI_BASE_URL"))
	}
	if baseURL == "" {
		resource := strings.TrimSpace(os.Getenv("AZURE_OPENAI_RESOURCE_NAME"))
		if resource == "" {
			return "", errors.New("--base-url, AZURE_OPENAI_BASE_URL, or AZURE_OPENAI_RESOURCE_NAME is required for provider \"azure-openai-responses\"")
		}
		baseURL = "https://" + resource + ".openai.azure.com"
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return "", fmt.Errorf("invalid Azure OpenAI Responses base URL %q", baseURL)
	}
	path := strings.TrimRight(endpoint.Path, "/")
	switch {
	case strings.HasSuffix(path, "/responses"):
	case strings.HasSuffix(path, "/openai/v1"):
		path += "/responses"
	default:
		path += "/openai/v1/responses"
	}
	endpoint.Path = path
	query := endpoint.Query()
	if query.Get("api-version") == "" {
		apiVersion := strings.TrimSpace(os.Getenv("AZURE_OPENAI_API_VERSION"))
		if apiVersion == "" {
			apiVersion = "v1"
		}
		query.Set("api-version", apiVersion)
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

type azureResponsesAuth struct {
	value  string
	source string
	header string
	prefix string
}

// azureResponsesCredential resolves API-key authentication before an ambient
// Entra token. A Gator-owned provider credential retains the normal explicit,
// stored, then environment precedence used by the rest of the CLI.
func azureResponsesCredential(config Config) (azureResponsesAuth, error) {
	if value := strings.TrimSpace(config.APIKey); value != "" {
		return azureResponsesAuth{value: value, source: "AZURE_OPENAI_API_KEY", header: "api-key"}, nil
	}
	if config.Credentials != nil {
		credential, found, err := config.Credentials.Read(string(AzureOpenAIResponses))
		if err != nil {
			return azureResponsesAuth{}, fmt.Errorf("read Gator credential for %q: %w", AzureOpenAIResponses, err)
		}
		if found {
			switch {
			case credential.IsAPIKey():
				return azureResponsesAuth{value: credential.Key, source: "Gator Azure OpenAI Responses API-key credential", header: "api-key"}, nil
			case credential.IsBearerToken():
				if credential.Expired(time.Now()) {
					return azureResponsesAuth{}, errors.New("Gator Azure OpenAI Responses bearer token expired; replace it with 'gator login azure-openai-responses --bearer-token' or set AZURE_OPENAI_AUTH_TOKEN")
				}
				return azureResponsesAuth{value: credential.Access, source: "Gator Azure OpenAI Responses bearer-token credential", header: "Authorization", prefix: "Bearer "}, nil
			}
		}
	}
	if value := strings.TrimSpace(os.Getenv("AZURE_OPENAI_API_KEY")); value != "" {
		return azureResponsesAuth{value: value, source: "AZURE_OPENAI_API_KEY", header: "api-key"}, nil
	}
	if value := strings.TrimSpace(os.Getenv("AZURE_OPENAI_AUTH_TOKEN")); value != "" {
		return azureResponsesAuth{value: value, source: "AZURE_OPENAI_AUTH_TOKEN", header: "Authorization", prefix: "Bearer "}, nil
	}
	return azureResponsesAuth{}, errors.New("AZURE_OPENAI_API_KEY or AZURE_OPENAI_AUTH_TOKEN is required")
}

func miniMaxMessagesURL(configured string, china bool) string {
	baseURL := strings.TrimRight(strings.TrimSpace(configured), "/")
	if baseURL == "" {
		if china {
			baseURL = "https://api.minimaxi.com/anthropic"
		} else {
			baseURL = "https://api.minimax.io/anthropic"
		}
	}
	if strings.HasSuffix(baseURL, "/v1/messages") {
		return baseURL
	}
	return baseURL + "/v1/messages"
}

// openCodeBackend uses the checked-in OpenCode model catalog rather than
// guessing a protocol from a model-family prefix. Gator never launches
// OpenCode or reads its credential store.
func openCodeBackend(provider Provider, config Config) (Backend, error) {
	apiKey, err := key(config, provider, "OPENCODE_API_KEY")
	if err != nil {
		return Backend{}, err
	}
	if err := requireKey(apiKey, "OPENCODE_API_KEY"); err != nil {
		return Backend{}, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://opencode.ai/zen"
		if provider == OpenCodeGo {
			baseURL += "/go"
		}
	}
	protocol, found := openCodeModelProtocol(provider, config.Model)
	if !found {
		return Backend{}, fmt.Errorf("model %q is not in the embedded %s catalog version %s; choose a documented OpenCode model or update Gator's catalog", config.Model, openCodeProviderName(provider), openCodeCatalogVersion)
	}
	switch protocol {
	case openCodeResponses:
		return Backend{Provider: provider, Model: openai.Responses{
			APIKey:    apiKey,
			APIKeyEnv: "OPENCODE_API_KEY",
			Model:     config.Model,
			BaseURL:   openCodeEndpoint(baseURL, "/responses"),
			Client:    config.Client,
		}}, nil
	case openCodeAnthropic:
		return Backend{Provider: provider, Model: anthropic.Messages{
			APIKey:  apiKey,
			Model:   config.Model,
			BaseURL: openCodeEndpoint(baseURL, "/messages"),
			Client:  config.Client,
		}}, nil
	case openCodeGemini:
		return Backend{Provider: provider, Model: gemini.GenerateContent{
			APIKey:  apiKey,
			Model:   config.Model,
			BaseURL: openCodeAPIVersionBase(baseURL),
			Client:  config.Client,
		}}, nil
	case openCodeChatCompletions:
		providerName := "OpenCode Zen"
		if provider == OpenCodeGo {
			providerName = "OpenCode Go"
		}
		return Backend{Provider: provider, Model: chatcompletions.Model{Config: chatcompletions.Config{
			APIKey:       apiKey,
			APIKeyEnv:    "OPENCODE_API_KEY",
			BaseURL:      openCodeEndpoint(baseURL, "/chat/completions"),
			Model:        config.Model,
			ProviderName: providerName,
			Client:       config.Client,
		}}}, nil
	default:
		return Backend{}, fmt.Errorf("model %q has an unsupported %s protocol in catalog version %s", config.Model, openCodeProviderName(provider), openCodeCatalogVersion)
	}
}

func openCodeEndpoint(baseURL, suffix string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, suffix) {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + suffix
	}
	return baseURL + "/v1" + suffix
}

func openCodeAPIVersionBase(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL
	}
	return baseURL + "/v1"
}

func vertexConfig(config Config) (chatcompletions.Config, error) {
	project := providerOption(config, "project", "GOOGLE_CLOUD_PROJECT")
	if project == "" {
		project = strings.TrimSpace(os.Getenv("GCLOUD_PROJECT"))
	}
	if project == "" {
		return chatcompletions.Config{}, errors.New("GOOGLE_CLOUD_PROJECT or GCLOUD_PROJECT is required")
	}
	location := providerOption(config, "location", "GOOGLE_CLOUD_LOCATION")
	if location == "" {
		return chatcompletions.Config{}, errors.New("GOOGLE_CLOUD_LOCATION is required")
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = "https://" + location + "-aiplatform.googleapis.com/v1/projects/" + project + "/locations/" + location + "/endpoints/openapi/chat/completions"
	}
	if strings.TrimSpace(config.Model) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--model is required for provider %q", GoogleVertex)
	}
	credentials, err := vertexCredentialSource(config)
	if err != nil {
		return chatcompletions.Config{}, err
	}
	return chatcompletions.Config{
		APIKeySource: credentials.Token,
		APIKeyEnv:    "GATOR_VERTEX_ACCESS_TOKEN or Google Application Default Credentials",
		BaseURL:      baseURL,
		Model:        config.Model,
		ProviderName: "Google Vertex AI",
		Client:       config.Client,
	}, nil
}

func cloudflareWorkersConfig(config Config) (chatcompletions.Config, error) {
	apiKey, err := key(config, CloudflareWorkers, "CLOUDFLARE_API_TOKEN")
	if err != nil {
		return chatcompletions.Config{}, err
	}
	if err := requireKey(apiKey, "CLOUDFLARE_API_TOKEN"); err != nil {
		return chatcompletions.Config{}, err
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		accountID := providerOption(config, "account_id", "CLOUDFLARE_ACCOUNT_ID")
		if accountID == "" {
			return chatcompletions.Config{}, errors.New("CLOUDFLARE_ACCOUNT_ID is required")
		}
		baseURL = "https://api.cloudflare.com/client/v4/accounts/" + accountID + "/ai/v1/chat/completions"
	}
	if strings.TrimSpace(config.Model) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--model is required for provider %q", CloudflareWorkers)
	}
	return chatcompletions.Config{
		APIKey:       apiKey,
		APIKeyEnv:    "CLOUDFLARE_API_TOKEN",
		BaseURL:      baseURL,
		Model:        config.Model,
		ProviderName: "Cloudflare Workers AI",
		Client:       config.Client,
	}, nil
}

type cloudflareGatewayProtocol string

const (
	cloudflareGatewayOpenAIResponses          cloudflareGatewayProtocol = "openai-responses"
	cloudflareGatewayAnthropicMessages        cloudflareGatewayProtocol = "anthropic-messages"
	cloudflareGatewayWorkersAIChatCompletions cloudflareGatewayProtocol = "workers-ai-chat-completions"
)

type cloudflareGatewayConfig struct {
	apiKey   string
	model    string
	baseURL  string
	gateway  string
	protocol cloudflareGatewayProtocol
}

// cloudflareGatewayBackend uses Cloudflare's account REST API, where each
// native request schema has an explicit endpoint. The caller must select the
// schema rather than relying on an ambiguous model-name heuristic.
func cloudflareGatewayBackend(config Config) (Backend, error) {
	configured, err := resolveCloudflareGatewayConfig(config)
	if err != nil {
		return Backend{}, err
	}
	headers := http.Header{"Cf-Aig-Gateway-Id": []string{configured.gateway}}
	switch configured.protocol {
	case cloudflareGatewayOpenAIResponses:
		return Backend{Provider: CloudflareGateway, Model: openai.Responses{
			APIKey:    configured.apiKey,
			APIKeyEnv: "CLOUDFLARE_API_TOKEN",
			Model:     configured.model,
			BaseURL:   cloudflareGatewayEndpoint(configured.baseURL, "/responses"),
			Headers:   headers,
			Client:    config.Client,
		}}, nil
	case cloudflareGatewayAnthropicMessages:
		return Backend{Provider: CloudflareGateway, Model: anthropic.Messages{
			APIKey:     configured.apiKey,
			Model:      configured.model,
			BaseURL:    cloudflareGatewayEndpoint(configured.baseURL, "/messages"),
			BearerAuth: true,
			Headers:    headers,
			Client:     config.Client,
		}}, nil
	case cloudflareGatewayWorkersAIChatCompletions:
		return Backend{Provider: CloudflareGateway, Model: chatcompletions.Model{Config: chatcompletions.Config{
			APIKey:       configured.apiKey,
			APIKeyEnv:    "CLOUDFLARE_API_TOKEN",
			BaseURL:      cloudflareGatewayEndpoint(configured.baseURL, "/chat/completions"),
			Model:        configured.model,
			ProviderName: "Cloudflare AI Gateway Workers AI",
			Headers:      headers,
			Client:       config.Client,
		}}}, nil
	default:
		return Backend{}, fmt.Errorf("unsupported Cloudflare AI Gateway protocol %q", configured.protocol)
	}
}

func resolveCloudflareGatewayConfig(config Config) (cloudflareGatewayConfig, error) {
	apiKey, err := key(config, CloudflareGateway, "CLOUDFLARE_API_TOKEN")
	if err != nil {
		return cloudflareGatewayConfig{}, err
	}
	if err := requireKey(apiKey, "CLOUDFLARE_API_TOKEN"); err != nil {
		return cloudflareGatewayConfig{}, err
	}
	if strings.TrimSpace(config.Model) == "" {
		return cloudflareGatewayConfig{}, fmt.Errorf("--model is required for provider %q", CloudflareGateway)
	}
	accountID := strings.TrimSpace(config.CloudflareAccountID)
	if accountID == "" {
		accountID = providerOption(config, "account_id", "")
	}
	if accountID == "" {
		accountID = strings.TrimSpace(os.Getenv("CLOUDFLARE_ACCOUNT_ID"))
	}
	if accountID == "" {
		return cloudflareGatewayConfig{}, errors.New("CLOUDFLARE_ACCOUNT_ID is required for provider \"cloudflare-ai-gateway\"")
	}
	gatewayID := strings.TrimSpace(config.CloudflareGatewayID)
	if gatewayID == "" {
		gatewayID = providerOption(config, "gateway_id", "")
	}
	if gatewayID == "" {
		gatewayID = strings.TrimSpace(os.Getenv("CLOUDFLARE_AI_GATEWAY_ID"))
	}
	if gatewayID == "" {
		return cloudflareGatewayConfig{}, errors.New("CLOUDFLARE_AI_GATEWAY_ID is required for provider \"cloudflare-ai-gateway\"")
	}
	protocolOption := config.CloudflareGatewayProtocol
	if strings.TrimSpace(protocolOption) == "" {
		protocolOption = providerOption(config, "gateway_protocol", "")
	}
	protocol, err := parseCloudflareGatewayProtocol(protocolOption)
	if err != nil {
		return cloudflareGatewayConfig{}, err
	}
	if err := validateCloudflareGatewayModel(protocol, config.Model); err != nil {
		return cloudflareGatewayConfig{}, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4/accounts/" + accountID + "/ai/v1"
	}
	return cloudflareGatewayConfig{apiKey: apiKey, model: config.Model, baseURL: baseURL, gateway: gatewayID, protocol: protocol}, nil
}

func parseCloudflareGatewayProtocol(configured string) (cloudflareGatewayProtocol, error) {
	if strings.TrimSpace(configured) == "" {
		configured = os.Getenv("GATOR_CLOUDFLARE_GATEWAY_PROTOCOL")
	}
	protocol := cloudflareGatewayProtocol(strings.TrimSpace(strings.ToLower(configured)))
	switch protocol {
	case cloudflareGatewayOpenAIResponses, cloudflareGatewayAnthropicMessages, cloudflareGatewayWorkersAIChatCompletions:
		return protocol, nil
	default:
		return "", errors.New("GATOR_CLOUDFLARE_GATEWAY_PROTOCOL is required and must be openai-responses, anthropic-messages, or workers-ai-chat-completions")
	}
}

func validateCloudflareGatewayModel(protocol cloudflareGatewayProtocol, model string) error {
	model = strings.TrimSpace(model)
	switch {
	case protocol == cloudflareGatewayOpenAIResponses && strings.HasPrefix(model, "openai/"):
		return nil
	case protocol == cloudflareGatewayAnthropicMessages && strings.HasPrefix(model, "anthropic/"):
		return nil
	case protocol == cloudflareGatewayWorkersAIChatCompletions && strings.HasPrefix(model, "@cf/"):
		return nil
	default:
		return fmt.Errorf("model %q is incompatible with Cloudflare AI Gateway protocol %q", model, protocol)
	}
}

func cloudflareGatewayEndpoint(baseURL, suffix string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, suffix) {
		return baseURL
	}
	return baseURL + suffix
}

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
