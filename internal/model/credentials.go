func key(config Config, provider Provider, environment string) (string, error) {
	if config.APIKey != "" {
		return config.APIKey, nil
	}
	if config.Credentials != nil {
		credential, found, err := config.Credentials.Read(string(provider))
		if err != nil {
			return "", fmt.Errorf("read Gator credential for %q: %w", provider, err)
		}
		if found && credential.IsAPIKey() {
			return credential.Key, nil
		}
		if found && credential.IsOAuth() && (provider == XAI || provider == Radius) {
			if credential.Expired(time.Now()) {
				return "", fmt.Errorf("Gator OAuth credential for %q expired; run 'gator login %s --subscription'", provider, provider)
			}
			return credential.Access, nil
		}
	}
	return os.Getenv(environment), nil
}

// bearerOrAPIKey resolves a Gator-owned bearer token in addition to the
// normal API-key chain. Bedrock accepts an AWS bearer token as an alternative
// to its standard signed AWS credential chain.
func bearerOrAPIKey(config Config, provider Provider, environment string) (string, error) {
	if value := strings.TrimSpace(config.APIKey); value != "" {
		return value, nil
	}
	if config.Credentials != nil {
		credential, found, err := config.Credentials.Read(string(provider))
		if err != nil {
			return "", fmt.Errorf("read Gator credential for %q: %w", provider, err)
		}
		if found {
			switch {
			case credential.IsAPIKey():
				return credential.Key, nil
			case credential.IsBearerToken():
				if credential.Expired(time.Now()) {
					return "", fmt.Errorf("Gator bearer token for %q is expired; replace it in /model", provider)
				}
				return credential.Access, nil
			}
		}
	}
	return strings.TrimSpace(os.Getenv(environment)), nil
}

func vertexCredentialSource(config Config) (vertex.Source, error) {
	source := vertex.FromEnvironment()
	if path := providerOption(config, "credentials_path", "GOOGLE_APPLICATION_CREDENTIALS"); path != "" {
		source.CredentialsPath = path
	}
	if config.Credentials == nil {
		return source, nil
	}
	credential, found, err := config.Credentials.Read(string(GoogleVertex))
	if err != nil {
		return vertex.Source{}, fmt.Errorf("read Gator credential for %q: %w", GoogleVertex, err)
	}
	if !found {
		return source, nil
	}
	if !credential.IsBearerToken() {
		return source, nil
	}
	if credential.Expired(time.Now()) {
		return vertex.Source{}, errors.New("Gator Google Vertex bearer token expired; replace it in /model or use Application Default Credentials")
	}
	source.AccessToken = credential.Access
	return source, nil
}

func providerOption(config Config, name, environment string) string {
	if value := strings.TrimSpace(config.ProviderOptions[name]); value != "" {
		return value
	}
	if environment == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(environment))
}

func kimiCredential(config Config) (string, bool, error) {
	if config.APIKey != "" {
		return config.APIKey, true, nil
	}
	if config.Credentials != nil {
		credential, found, err := config.Credentials.Read(string(KimiCoding))
		if err != nil {
			return "", false, fmt.Errorf("read Gator credential for %q: %w", KimiCoding, err)
		}
		if found && credential.IsOAuth() {
			if credential.Expired(time.Now()) {
				return "", false, fmt.Errorf("Gator OAuth credential for %q expired; run 'gator login %s --subscription'", KimiCoding, KimiCoding)
			}
			return credential.Access, true, nil
		}
		if found && credential.IsAPIKey() {
			return credential.Key, true, nil
		}
	}
	key := os.Getenv("KIMI_API_KEY")
	if strings.TrimSpace(key) == "" {
		return "", false, errors.New("KIMI_API_KEY or a Gator Kimi Code OAuth credential is required")
	}
	return key, true, nil
}

func oauthCredential(config Config, provider Provider) (auth.Credential, error) {
	if config.Credentials == nil {
		return auth.Credential{}, fmt.Errorf("Gator OAuth credential for %q is required; run 'gator login %s'", provider, provider)
	}
	credential, found, err := config.Credentials.Read(string(provider))
	if err != nil {
		return auth.Credential{}, fmt.Errorf("read Gator credential for %q: %w", provider, err)
	}
	if !found || !credential.IsOAuth() {
		return auth.Credential{}, fmt.Errorf("Gator OAuth credential for %q is required; run 'gator login %s'", provider, provider)
	}
	if credential.Expired(time.Now()) {
		return auth.Credential{}, fmt.Errorf("Gator OAuth credential for %q expired; run 'gator login %s'", provider, provider)
	}
	return credential, nil
}

func codexResponsesURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "https://chatgpt.com/backend-api/codex/responses"
	}
	if strings.HasSuffix(baseURL, "/codex/responses") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/codex") {
		return baseURL + "/responses"
	}
	return baseURL + "/codex/responses"
}

func kimiMessagesURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "https://api.kimi.com/coding/v1/messages"
	}
	if strings.HasSuffix(baseURL, "/messages") {
		return baseURL
	}
	return baseURL + "/v1/messages"
}

func codexAccountID(credential auth.Credential) (string, error) {
	if accountID := strings.TrimSpace(credential.Extra["chatgpt_account_id"]); accountID != "" {
		return accountID, nil
	}
	parts := strings.Split(credential.Access, ".")
	if len(parts) != 3 {
		return "", errors.New("Codex OAuth credential has no ChatGPT account identifier")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("decode Codex OAuth credential account identifier")
	}
	var claims struct {
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || strings.TrimSpace(claims.Auth.AccountID) == "" {
		return "", errors.New("Codex OAuth credential has no ChatGPT account identifier")
	}
	return claims.Auth.AccountID, nil
}

func copilotChatURL(override, credentialURL string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(override), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(credentialURL), "/")
	}
	if baseURL == "" {
		baseURL = "https://api.individual.githubcopilot.com"
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func copilotRequestHeaders(turn agent.TurnRequest) http.Header {
	initiator := "user"
	if len(turn.Messages) > 0 && turn.Messages[len(turn.Messages)-1].Role != agent.RoleUser {
		initiator = "agent"
	}
	return http.Header{
		"X-Initiator":   []string{initiator},
		"Openai-Intent": []string{"conversation-edits"},
	}
}

func requireKey(value, environment string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", environment)
	}
	return nil
}

// allProviders includes retained legacy provider names so loading a previous
// session produces a clear no-fallback error instead of treating its metadata
// as malformed. Names exposes only providers that can currently execute.
