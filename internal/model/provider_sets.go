package model

var allProviders = map[Provider]struct{}{
	OpenAI: {}, AzureOpenAI: {}, AzureOpenAIResponses: {}, Anthropic: {}, Gemini: {}, Mistral: {}, XAI: {}, Groq: {}, OpenRouter: {}, Together: {}, Fireworks: {}, DeepSeek: {}, Cerebras: {}, NVIDIA: {}, HuggingFace: {}, MoonshotAI: {}, ZAI: {}, ZAICodingCN: {}, MiniMax: {}, MiniMaxCN: {}, Baseten: {}, VercelAIGateway: {}, AntLing: {}, Xiaomi: {}, MoonshotAICN: {}, CloudflareWorkers: {}, CloudflareGateway: {}, QwenTokenPlan: {}, QwenTokenPlanCN: {}, QwenTokenPlanIndividual: {}, XiaomiTokenPlanCN: {}, XiaomiTokenPlanAMS: {}, XiaomiTokenPlanSGP: {}, OpenAICompatible: {}, KimiCoding: {}, Radius: {}, OpenCode: {}, OpenCodeGo: {},
}

var directProviders = map[Provider]struct{}{
	OpenAI: {}, AzureOpenAI: {}, AzureOpenAIResponses: {}, Anthropic: {}, Gemini: {}, Mistral: {}, XAI: {}, Groq: {}, OpenRouter: {}, Together: {}, Fireworks: {}, DeepSeek: {}, Cerebras: {}, NVIDIA: {}, HuggingFace: {}, MoonshotAI: {}, ZAI: {}, ZAICodingCN: {}, MiniMax: {}, MiniMaxCN: {}, Baseten: {}, VercelAIGateway: {}, AntLing: {}, Xiaomi: {}, MoonshotAICN: {}, CloudflareWorkers: {}, CloudflareGateway: {}, QwenTokenPlan: {}, QwenTokenPlanCN: {}, QwenTokenPlanIndividual: {}, XiaomiTokenPlanCN: {}, XiaomiTokenPlanAMS: {}, XiaomiTokenPlanSGP: {}, OpenAICompatible: {}, KimiCoding: {}, Radius: {}, OpenCode: {}, OpenCodeGo: {},
}
