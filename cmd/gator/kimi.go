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

type kimiEndpoints struct {
	authBaseURL string
	clientID    string
}

type kimiCodingDeviceLogin struct {
	flow        auth.DeviceFlow
	device      auth.DeviceAuthorization
	credentials auth.Store
}

func beginKimiCodingDeviceLogin() (*kimiCodingDeviceLogin, error) {
	endpoints, err := configuredKimiEndpoints()
	if err != nil {
		return nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return nil, err
	}
	flow := auth.DeviceFlow{
		ClientID:       endpoints.clientID,
		DeviceCodeURL:  endpoints.authBaseURL + "/api/oauth/device_authorization",
		TokenURL:       endpoints.authBaseURL + "/api/oauth/token",
		WaitBeforePoll: true,
	}
	device, err := flow.Start(context.Background())
	if err != nil {
		return nil, err
	}
	return &kimiCodingDeviceLogin{flow: flow, device: device, credentials: credentials}, nil
}

func (l *kimiCodingDeviceLogin) URL() string {
	return l.device.VerificationURL + "\nCode: " + l.device.UserCode
}

func (l *kimiCodingDeviceLogin) Complete(ctx context.Context) error {
	token, err := l.flow.PollToken(ctx, l.device)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token.Access) == "" || strings.TrimSpace(token.Refresh) == "" {
		return errors.New("Kimi Code OAuth response was missing an access or refresh token")
	}
	if err := l.credentials.Put("kimi-coding", auth.Credential{
		Type:    "oauth",
		Access:  token.Access,
		Refresh: token.Refresh,
		Expires: tokenExpiry(time.Now(), token.Expires),
	}); err != nil {
		return fmt.Errorf("store Kimi Code OAuth credential: %w", err)
	}
	return nil
}

func (l *kimiCodingDeviceLogin) Cancel() {}

func configuredKimiEndpoints() (kimiEndpoints, error) {
	clientID := strings.TrimSpace(os.Getenv("GATOR_KIMI_CODE_OAUTH_CLIENT_ID"))
	if clientID == "" {
		return kimiEndpoints{}, errors.New("GATOR_KIMI_CODE_OAUTH_CLIENT_ID is required: Gator will not impersonate another application's OAuth client")
	}
	authBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("GATOR_KIMI_CODE_OAUTH_URL")), "/")
	if authBaseURL == "" {
		authBaseURL = "https://auth.kimi.com"
	}
	parsed, err := absoluteURL(authBaseURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return kimiEndpoints{}, errors.New("GATOR_KIMI_CODE_OAUTH_URL must be an http or https URL")
	}
	return kimiEndpoints{authBaseURL: strings.TrimRight(parsed.String(), "/"), clientID: clientID}, nil
}

func kimiOAuthRefreshFlow() (auth.BrowserFlow, error) {
	endpoints, err := configuredKimiEndpoints()
	if err != nil {
		return auth.BrowserFlow{}, err
	}
	return auth.BrowserFlow{ClientID: endpoints.clientID, TokenURL: endpoints.authBaseURL + "/api/oauth/token"}, nil
}

func tokenExpiry(now time.Time, expires time.Duration) int64 {
	if expires <= 0 {
		expires = time.Hour
	}
	skew := 5 * time.Minute
	if expires <= skew {
		skew = 15 * time.Second
	}
	return now.Add(expires - skew).UnixMilli()
}
