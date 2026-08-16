package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
)

type openRouterLogin struct {
	attempt     auth.BrowserAttempt
	callback    *auth.Callback
	keysURL     string
	credentials auth.Store
}

func beginOpenRouterLogin() (*openRouterLogin, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("GATOR_OPENROUTER_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://openrouter.ai"
	}
	parsed, err := absoluteURL(baseURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("GATOR_OPENROUTER_BASE_URL must be an http or https URL")
	}
	redirectURL := strings.TrimSpace(os.Getenv("GATOR_OPENROUTER_OAUTH_REDIRECT_URL"))
	if redirectURL == "" {
		redirectURL = "http://127.0.0.1:1455/oauth/callback"
	}
	// OpenRouter does not use a registered client ID in this authorization
	// shape. The placeholder is never placed in its authorization request.
	attempt, err := auth.BeginBrowserFlow(auth.BrowserFlow{
		ClientID:          "gator-openrouter-local",
		AuthorizationURL:  parsed.String() + "/auth",
		TokenURL:          parsed.String() + "/api/v1/auth/keys",
		RedirectURL:       redirectURL,
		AllowMissingState: true,
	})
	if err != nil {
		return nil, err
	}
	callback, err := attempt.StartCallback()
	if err != nil {
		return nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		callback.Close()
		return nil, err
	}
	return &openRouterLogin{attempt: attempt, callback: callback, keysURL: parsed.String() + "/api/v1/auth/keys", credentials: credentials}, nil
}

func (l *openRouterLogin) URL() string {
	authorize, _ := absoluteURL(strings.TrimSuffix(l.keysURL, "/api/v1/auth/keys") + "/auth")
	query := authorize.Query()
	query.Set("callback_url", l.attempt.RedirectURL())
	query.Set("code_challenge", openRouterCodeChallenge(l.attempt.CodeVerifier()))
	query.Set("code_challenge_method", "S256")
	authorize.RawQuery = query.Encode()
	return authorize.String()
}

func (l *openRouterLogin) Complete(ctx context.Context) error {
	defer l.callback.Close()
	code, err := l.callback.Wait(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{
		"code":                  code,
		"code_verifier":         l.attempt.CodeVerifier(),
		"code_challenge_method": "S256",
	})
	if err != nil {
		return fmt.Errorf("encode OpenRouter key exchange: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, l.keysURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create OpenRouter key exchange: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("exchange OpenRouter authorization code: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("read OpenRouter key exchange: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("OpenRouter key exchange returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(contents, &result); err != nil || strings.TrimSpace(result.Key) == "" {
		return errors.New("OpenRouter key exchange returned no API key")
	}
	if err := l.credentials.Put("openrouter", auth.Credential{Type: "api_key", Key: result.Key}); err != nil {
		return fmt.Errorf("store OpenRouter API key: %w", err)
	}
	return nil
}

func (l *openRouterLogin) Cancel() {
	if l != nil && l.callback != nil {
		l.callback.Close()
	}
}

func openRouterCodeChallenge(verifier string) string {
	// BrowserAttempt creates the PKCE challenge internally for the standard
	// flow. Reconstructing it here avoids exposing token exchange internals.
	return auth.PKCEChallenge(verifier)
}
