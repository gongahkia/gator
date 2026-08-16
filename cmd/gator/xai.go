package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
)

const xaiOAuthScope = "openid profile email offline_access grok-cli:access api:access"

type xaiEndpoints struct {
	authBaseURL string
	clientID    string
}

type xaiDeviceLogin struct {
	flow        auth.DeviceFlow
	device      auth.DeviceAuthorization
	credentials auth.Store
}

func beginXAIDeviceLogin() (*xaiDeviceLogin, error) {
	endpoints, err := configuredXAIEndpoints()
	if err != nil {
		return nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return nil, err
	}
	flow := auth.DeviceFlow{
		ClientID:       endpoints.clientID,
		DeviceCodeURL:  endpoints.authBaseURL + "/oauth2/device/code",
		TokenURL:       endpoints.authBaseURL + "/oauth2/token",
		Scope:          xaiOAuthScope,
		DeviceParams:   map[string]string{"referrer": "gator"},
		WaitBeforePoll: true,
	}
	device, err := flow.Start(context.Background())
	if err != nil {
		return nil, err
	}
	return &xaiDeviceLogin{flow: flow, device: device, credentials: credentials}, nil
}

func (l *xaiDeviceLogin) URL() string {
	return l.device.VerificationURL + "\nCode: " + l.device.UserCode
}

func (l *xaiDeviceLogin) Complete(ctx context.Context) error {
	token, err := l.flow.PollToken(ctx, l.device)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token.Access) == "" || strings.TrimSpace(token.Refresh) == "" {
		return errors.New("xAI OAuth response was missing an access or refresh token")
	}
	credential := auth.Credential{Type: "oauth", Access: token.Access, Refresh: token.Refresh, Expires: tokenExpiry(time.Now(), token.Expires)}
	if err := l.credentials.Put("xai", credential); err != nil {
		return fmt.Errorf("store xAI OAuth credential: %w", err)
	}
	return nil
}

func (l *xaiDeviceLogin) Cancel() {}

func configuredXAIEndpoints() (xaiEndpoints, error) {
	clientID := strings.TrimSpace(os.Getenv("GATOR_XAI_OAUTH_CLIENT_ID"))
	if clientID == "" {
		return xaiEndpoints{}, errors.New("GATOR_XAI_OAUTH_CLIENT_ID is required: Gator will not impersonate another application's OAuth client")
	}
	authBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("GATOR_XAI_AUTH_URL")), "/")
	if authBaseURL == "" {
		authBaseURL = "https://auth.x.ai"
	}
	parsed, err := absoluteURL(authBaseURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return xaiEndpoints{}, errors.New("GATOR_XAI_AUTH_URL must be an http or https URL")
	}
	return xaiEndpoints{authBaseURL: strings.TrimRight(parsed.String(), "/"), clientID: clientID}, nil
}

func xaiOAuthRefreshFlow() (auth.BrowserFlow, error) {
	endpoints, err := configuredXAIEndpoints()
	if err != nil {
		return auth.BrowserFlow{}, err
	}
	return auth.BrowserFlow{ClientID: endpoints.clientID, TokenURL: endpoints.authBaseURL + "/oauth2/token"}, nil
}
