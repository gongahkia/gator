# Model providers

Norbot calls only the configured provider IDs. Select each stage provider in **Runs → New run**; an existing run keeps its provider IDs permanently.

Credentials are environment-variable references in `config.json`. Do not put a credential value in configuration, prompts, skills, traces, artifacts, or generated apps.

## Native adapters

| kind | Authentication | Required fields |
| --- | --- | --- |
| `openai_responses` | `credential_env` bearer key | `model`, `base_url`, `credential_env` |
| `azure_openai_responses` | `credential_env` in `api-key` header | Azure deployment `model`, Azure `/openai/v1` `base_url`, `credential_env` |
| `anthropic_messages` | `credential_env` bearer key | `model`, `base_url`, `credential_env` |
| `gemini_generate_content` | `credential_env` query key | `model`, `base_url`, `credential_env` |
| `vertex_ai_generate_content` | Google Application Default Credentials | `model`, `base_url`, `project`, `region` |
| `cohere_v2_chat` | `credential_env` bearer key | `model`, `base_url`, `credential_env` |
| `ollama_chat` | none | `model`, `base_url` |
| `aws_bedrock_converse` | AWS default credential chain | `model`, `region` |

### Azure OpenAI

Microsoft Foundry's v1 Responses endpoint is `{endpoint}/openai/v1/responses` and accepts the API key in `api-key`. `model` is the Azure deployment name, not necessarily the underlying model name.

```json
{
  "id": "azure-openai",
  "kind": "azure_openai_responses",
  "model": "<azure-deployment-name>",
  "base_url": "https://<resource>.openai.azure.com/openai/v1",
  "credential_env": "AZURE_OPENAI_KEY",
  "stages": ["planner", "builder", "verifier"],
  "budget": {"max_concurrent": 2, "requests_per_minute": 60}
}
```

Set `AZURE_OPENAI_KEY` in `.env`. `AZURE_OPENAI_API_KEY` also works if that exact name is used in `credential_env`.

### Cohere

```json
{
  "id": "cohere",
  "kind": "cohere_v2_chat",
  "model": "<cohere-chat-model>",
  "base_url": "https://api.cohere.com",
  "credential_env": "COHERE_API_KEY",
  "stages": ["planner", "builder", "verifier"],
  "budget": {"max_concurrent": 2, "requests_per_minute": 40}
}
```

### Ollama

Norbot calls Ollama's native non-streaming `/api/chat` endpoint. In Docker Desktop on macOS, use `host.docker.internal`, not `localhost`.

```json
{
  "id": "ollama",
  "kind": "ollama_chat",
  "model": "qwen3-coder",
  "base_url": "http://host.docker.internal:11434",
  "stages": ["planner", "builder", "verifier"],
  "budget": {"max_concurrent": 1, "requests_per_minute": 20}
}
```

Pull the selected model before creating a run. This adapter has no API-key field and fails closed when the endpoint is unreachable.

### Amazon Bedrock

Norbot uses the AWS SDK v2 `Converse` API. It supports standard AWS credential resolution, including `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`/`AWS_SESSION_TOKEN`, profile credentials, ECS/EC2 roles, and Kubernetes workload identity. The identity needs `bedrock:InvokeModel` for the chosen model or inference profile.

```json
{
  "id": "bedrock",
  "kind": "aws_bedrock_converse",
  "model": "amazon.nova-lite-v1:0",
  "region": "us-east-1",
  "stages": ["planner", "builder", "verifier"],
  "budget": {"max_concurrent": 2, "requests_per_minute": 30}
}
```

For Bedrock's short-lived API keys instead of IAM, configure its documented OpenAI-compatible `bedrock-mantle.<region>.api.aws/v1` endpoint as `openai_compatible` with `credential_env: "AWS_BEARER_TOKEN_BEDROCK"`.

### Vertex AI

Norbot uses the Vertex AI `generateContent` REST API with Application Default Credentials and the Cloud Platform OAuth scope. For Docker, mount the service-account JSON read-only and set `GOOGLE_APPLICATION_CREDENTIALS` to its in-container path; workload identity is preferred on Kubernetes.

```yaml
# docker-compose.vertex.yml
services:
  norbot:
    environment:
      GOOGLE_APPLICATION_CREDENTIALS: /run/secrets/norbot-gcp.json
    volumes:
      - ${NORBOT_GOOGLE_APPLICATION_CREDENTIALS_HOST}:/run/secrets/norbot-gcp.json:ro
```

Set `NORBOT_GOOGLE_APPLICATION_CREDENTIALS_HOST` to the absolute host path, then start with `docker compose -f docker-compose.yml -f docker-compose.vertex.yml up --build`.

```json
{
  "id": "vertex-ai",
  "kind": "vertex_ai_generate_content",
  "model": "gemini-2.5-flash",
  "base_url": "https://us-central1-aiplatform.googleapis.com",
  "project": "<google-cloud-project-id>",
  "region": "us-central1",
  "stages": ["planner", "builder", "verifier"],
  "budget": {"max_concurrent": 2, "requests_per_minute": 60}
}
```

## OpenAI-compatible adapters

`openai_compatible` calls `POST {base_url}/chat/completions` with OpenAI's chat-completions payload and optional bearer authentication. It is usable for a private compatible endpoint with no `credential_env`.

```json
{
  "id": "provider-id",
  "kind": "openai_compatible",
  "model": "<provider-model-id>",
  "base_url": "https://provider.example/v1",
  "credential_env": "PROVIDER_API_KEY",
  "stages": ["planner", "builder", "verifier"],
  "budget": {"max_concurrent": 2, "requests_per_minute": 60}
}
```

Verified compatible presets:

| Provider | `base_url` | credential environment variable |
| --- | --- | --- |
| Groq | `https://api.groq.com/openai/v1` | `GROQ_API_KEY` |
| Mistral | `https://api.mistral.ai/v1` | `MISTRAL_API_KEY` |
| xAI | `https://api.x.ai/v1` | `XAI_API_KEY` |
| OpenRouter | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| Together AI | `https://api.together.xyz/v1` | `TOGETHER_API_KEY` |
| Fireworks AI | `https://api.fireworks.ai/inference/v1` | `FIREWORKS_API_KEY` |
| Cerebras | `https://api.cerebras.ai/v1` | `CEREBRAS_API_KEY` |
| Hugging Face Inference Providers | `https://router.huggingface.co/v1` | `HF_TOKEN` |
| LM Studio / TGI / private server | its OpenAI-compatible `/v1` URL | omit `credential_env` if unauthenticated |

Norbot does not forward provider-specific optional headers, provider-hosted tools, or server-side web search. Every inference request is a single Norbot-controlled text request; agent tools remain subject to Norbot's separate operator policy.

## Validation

After restarting Norbot, open **Health** to confirm the configured credential and endpoint state. Then create a low-cost planner-only test run with that provider selected and inspect **Unified trace** for provider ID, model, usage, HTTP error, and trace ID. This repository has adapter contract tests but does not contain a live credential for any third-party provider.

## Sources

- [Azure OpenAI Responses API](https://learn.microsoft.com/en-us/rest/api/microsoft-foundry/azureopenai/responses)
- [Amazon Bedrock APIs](https://docs.aws.amazon.com/bedrock/latest/userguide/apis.html)
- [Cohere Chat v2](https://docs.cohere.com/docs/chat-api)
- [Ollama Chat API](https://docs.ollama.com/api/chat)
- [Vertex AI authentication](https://cloud.google.com/vertex-ai/generative-ai/docs/authentication)
- [Groq API](https://console.groq.com/docs/api-reference)
- [Mistral Chat API](https://docs.mistral.ai/api)
- [xAI Chat API](https://docs.x.ai/developers/rest-api-reference/inference/chat)
- [OpenRouter Quickstart](https://openrouter.ai/docs/quickstart)
- [Together AI authentication](https://docs.together.ai/docs/api-keys-authentication)
- [Fireworks OpenAI compatibility](https://docs.fireworks.ai/tools-sdks/openai-compatibility)
- [Cerebras Chat Completions](https://inference-docs.cerebras.ai/api-reference/chat-completions)
- [Hugging Face Inference Providers](https://huggingface.co/docs/inference-providers/en/index)
