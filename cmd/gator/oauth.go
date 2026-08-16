package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model"
)

func oauthFlow(provider model.Provider) (auth.BrowserFlow, error) {
	switch provider {
	case model.Codex:
		return configuredOAuthFlow(
			"GATOR_CODEX_OAUTH_CLIENT_ID",
			"GATOR_CODEX_OAUTH_REDIRECT_URL",
			"http://127.0.0.1:1455/auth/callback",
			"https://auth.openai.com/oauth/authorize",
			"https://auth.openai.com/oauth/token",
			[]string{"openid", "profile", "email", "offline_access"},
		)
	case model.Claude:
		return configuredOAuthFlow(
			"GATOR_CLAUDE_OAUTH_CLIENT_ID",
			"GATOR_CLAUDE_OAUTH_REDIRECT_URL",
			"http://127.0.0.1:53692/callback",
			"https://claude.ai/oauth/authorize",
			"https://platform.claude.com/v1/oauth/token",
			[]string{"org:create_api_key", "user:profile", "user:inference", "user:sessions:claude_code", "user:mcp_servers", "user:file_upload"},
		)
	default:
		return auth.BrowserFlow{}, fmt.Errorf("provider %q has no browser OAuth flow", provider)
	}
}

func configuredOAuthFlow(clientIDEnv, redirectEnv, defaultRedirect, authorizeURL, tokenURL string, scopes []string) (auth.BrowserFlow, error) {
	clientID := strings.TrimSpace(os.Getenv(clientIDEnv))
	if clientID == "" {
		return auth.BrowserFlow{}, fmt.Errorf("%s is required: Gator will not impersonate another application's OAuth client", clientIDEnv)
	}
	redirectURL := strings.TrimSpace(os.Getenv(redirectEnv))
	if redirectURL == "" {
		redirectURL = defaultRedirect
	}
	return auth.BrowserFlow{ClientID: clientID, AuthorizationURL: authorizeURL, TokenURL: tokenURL, RedirectURL: redirectURL, Scopes: scopes}, nil
}
