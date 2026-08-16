package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
)

const (
	copilotAPIVersion = "2026-06-01"
	copilotUserAgent  = "GitHubCopilotChat/0.35.0"
)

type copilotEndpoints struct {
	githubBaseURL string
	tokenURL      string
	apiURL        string
	clientID      string
}

type copilotDeviceLogin struct {
	flow        auth.DeviceFlow
	device      auth.DeviceAuthorization
	endpoints   copilotEndpoints
	credentials auth.Store
}

func beginCopilotDeviceLogin() (*copilotDeviceLogin, error) {
	endpoints, err := configuredCopilotEndpoints()
	if err != nil {
		return nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return nil, err
	}
	flow := auth.DeviceFlow{
		ClientID:       endpoints.clientID,
		DeviceCodeURL:  strings.TrimRight(endpoints.githubBaseURL, "/") + "/login/device/code",
		TokenURL:       strings.TrimRight(endpoints.githubBaseURL, "/") + "/login/oauth/access_token",
		Scope:          "read:user",
		WaitBeforePoll: true,
		Headers:        copilotProtocolHeaders(),
	}
	device, err := flow.Start(context.Background())
	if err != nil {
		return nil, err
	}
	return &copilotDeviceLogin{flow: flow, device: device, endpoints: endpoints, credentials: credentials}, nil
}

func (l *copilotDeviceLogin) URL() string {
	return l.device.VerificationURL + "\nCode: " + l.device.UserCode
}

func (l *copilotDeviceLogin) Complete(ctx context.Context) error {
	githubToken, err := l.flow.Poll(ctx, l.device)
	if err != nil {
		return err
	}
	credential, err := exchangeCopilotToken(ctx, l.endpoints, githubToken)
	if err != nil {
		return err
	}
	if err := l.credentials.Put("copilot", credential); err != nil {
		return fmt.Errorf("store Copilot OAuth credential: %w", err)
	}
	return nil
}

func (l *copilotDeviceLogin) Cancel() {}

func configuredCopilotEndpoints() (copilotEndpoints, error) {
	clientID := strings.TrimSpace(os.Getenv("GATOR_COPILOT_OAUTH_CLIENT_ID"))
	if clientID == "" {
		return copilotEndpoints{}, errors.New("GATOR_COPILOT_OAUTH_CLIENT_ID is required: Gator will not impersonate another application's OAuth client")
	}
	githubBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("GATOR_COPILOT_GITHUB_URL")), "/")
	if githubBaseURL == "" {
		githubBaseURL = "https://github.com"
	}
	githubURL, err := absoluteURL(githubBaseURL)
	if err != nil || (githubURL.Scheme != "https" && githubURL.Scheme != "http") {
		return copilotEndpoints{}, errors.New("GATOR_COPILOT_GITHUB_URL must be an http or https URL")
	}
	if githubURL.Path != "" && githubURL.Path != "/" {
		return copilotEndpoints{}, errors.New("GATOR_COPILOT_GITHUB_URL must not include a path")
	}
	tokenURL := strings.TrimSpace(os.Getenv("GATOR_COPILOT_TOKEN_URL"))
	apiURL := strings.TrimRight(strings.TrimSpace(os.Getenv("GATOR_COPILOT_API_URL")), "/")
	if tokenURL == "" {
		if githubURL.Hostname() == "github.com" {
			tokenURL = "https://api.github.com/copilot_internal/v2/token"
		} else {
			tokenURL = "https://api." + githubURL.Hostname() + "/copilot_internal/v2/token"
		}
	}
	if _, err := absoluteURL(tokenURL); err != nil {
		return copilotEndpoints{}, fmt.Errorf("invalid GATOR_COPILOT_TOKEN_URL: %w", err)
	}
	if apiURL == "" {
		if githubURL.Hostname() == "github.com" {
			apiURL = "https://api.individual.githubcopilot.com"
		} else {
			apiURL = "https://copilot-api." + githubURL.Hostname()
		}
	}
	if _, err := absoluteURL(apiURL); err != nil {
		return copilotEndpoints{}, fmt.Errorf("invalid GATOR_COPILOT_API_URL: %w", err)
	}
	return copilotEndpoints{githubBaseURL: githubURL.String(), tokenURL: tokenURL, apiURL: apiURL, clientID: clientID}, nil
}

func exchangeCopilotToken(ctx context.Context, endpoints copilotEndpoints, githubToken string) (auth.Credential, error) {
	if strings.TrimSpace(githubToken) == "" {
		return auth.Credential{}, errors.New("GitHub OAuth response has no access token")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoints.tokenURL, nil)
	if err != nil {
		return auth.Credential{}, fmt.Errorf("create Copilot token request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+githubToken)
	for name, values := range copilotProtocolHeaders() {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return auth.Credential{}, fmt.Errorf("request Copilot inference token: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return auth.Credential{}, fmt.Errorf("read Copilot inference token: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return auth.Credential{}, fmt.Errorf("Copilot inference token request returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(contents, &payload); err != nil || strings.TrimSpace(payload.Token) == "" || payload.ExpiresAt <= 0 {
		return auth.Credential{}, errors.New("Copilot inference token response was incomplete")
	}
	apiURL := copilotAPIURL(payload.Token, endpoints.apiURL)
	models, err := fetchCopilotModels(ctx, apiURL, payload.Token)
	if err != nil {
		return auth.Credential{}, err
	}
	expires := time.Unix(payload.ExpiresAt, 0).Add(-5 * time.Minute).UnixMilli()
	return auth.Credential{Type: "oauth", Access: payload.Token, Refresh: githubToken, Expires: expires, Extra: map[string]string{
		"base_url":            apiURL,
		"available_model_ids": strings.Join(models, ","),
	}}, nil
}

func refreshCopilotCredential(ctx context.Context, credential auth.Credential) (auth.Credential, error) {
	endpoints, err := configuredCopilotEndpoints()
	if err != nil {
		return auth.Credential{}, err
	}
	if configuredURL := strings.TrimSpace(credential.Extra["base_url"]); configuredURL != "" {
		endpoints.apiURL = configuredURL
	}
	return exchangeCopilotToken(ctx, endpoints, credential.Refresh)
}

func copilotAPIURL(token, fallback string) string {
	for _, part := range strings.Split(token, ";") {
		key, value, found := strings.Cut(part, "=")
		if !found || key != "proxy-ep" || strings.TrimSpace(value) == "" {
			continue
		}
		host := strings.TrimSpace(value)
		if strings.HasPrefix(host, "proxy.") {
			host = "api." + strings.TrimPrefix(host, "proxy.")
		}
		candidate, err := absoluteURL("https://" + host)
		if err == nil && candidate.Path == "" {
			return candidate.String()
		}
	}
	return strings.TrimRight(fallback, "/")
}

func fetchCopilotModels(ctx context.Context, apiURL, token string) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiURL, "/")+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create Copilot models request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-GitHub-Api-Version", copilotAPIVersion)
	for name, values := range copilotProtocolHeaders() {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Copilot model catalog: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 256*1024))
	if err != nil {
		return nil, fmt.Errorf("read Copilot model catalog: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Copilot model catalog returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID                 string `json:"id"`
			ModelPickerEnabled bool   `json:"model_picker_enabled"`
			Capabilities       struct {
				Supports struct {
					ToolCalls *bool `json:"tool_calls"`
				} `json:"supports"`
			} `json:"capabilities"`
			Policy struct {
				State string `json:"state"`
			} `json:"policy"`
		} `json:"data"`
	}
	if err := json.Unmarshal(contents, &payload); err != nil {
		return nil, errors.New("Copilot model catalog returned invalid JSON")
	}
	models := make([]string, 0, len(payload.Data))
	policyEnabled := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) == "" || item.Capabilities.Supports.ToolCalls != nil && !*item.Capabilities.Supports.ToolCalls || item.Policy.State == "disabled" {
			continue
		}
		if item.ModelPickerEnabled {
			models = append(models, item.ID)
		}
		if item.Policy.State == "enabled" {
			policyEnabled = append(policyEnabled, item.ID)
		}
	}
	if len(models) == 0 && strings.TrimRight(apiURL, "/") == "https://api.individual.githubcopilot.com" {
		models = policyEnabled
	}
	sort.Strings(models)
	return models, nil
}

func copilotProtocolHeaders() http.Header {
	return http.Header{
		"User-Agent":             []string{copilotUserAgent},
		"Editor-Version":         []string{"vscode/1.107.0"},
		"Editor-Plugin-Version":  []string{"copilot-chat/0.35.0"},
		"Copilot-Integration-Id": []string{"vscode-chat"},
	}
}

func absoluteURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("must be an absolute URL")
	}
	return parsed, nil
}
