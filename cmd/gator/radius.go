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

const radiusOAuthScope = "gateway offline_access"

type radiusEndpoints struct {
	gatewayURL string
	clientID   string
}

type radiusDeviceLogin struct {
	flow        auth.DeviceFlow
	device      auth.DeviceAuthorization
	credentials auth.Store
}

func beginRadiusDeviceLogin() (*radiusDeviceLogin, error) {
	endpoints, err := configuredRadiusEndpoints()
	if err != nil {
		return nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return nil, err
	}
	flow := auth.DeviceFlow{
		ClientID:       endpoints.clientID,
		DeviceCodeURL:  endpoints.gatewayURL + "/v1/oauth/device",
		TokenURL:       endpoints.gatewayURL + "/v1/oauth/token",
		Scope:          radiusOAuthScope,
		WaitBeforePoll: true,
	}
	device, err := flow.Start(context.Background())
	if err != nil {
		return nil, err
	}
	return &radiusDeviceLogin{flow: flow, device: device, credentials: credentials}, nil
}

func (l *radiusDeviceLogin) URL() string {
	return l.device.VerificationURL + "\nCode: " + l.device.UserCode
}

func (l *radiusDeviceLogin) Complete(ctx context.Context) error {
	token, err := l.flow.PollToken(ctx, l.device)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token.Access) == "" || strings.TrimSpace(token.Refresh) == "" {
		return errors.New("Radius OAuth response was missing an access or refresh token")
	}
	if err := l.credentials.Put("radius", auth.Credential{
		Type:    "oauth",
		Access:  token.Access,
		Refresh: token.Refresh,
		Expires: tokenExpiry(time.Now(), token.Expires),
	}); err != nil {
		return fmt.Errorf("store Radius OAuth credential: %w", err)
	}
	return nil
}

func (l *radiusDeviceLogin) Cancel() {}

func configuredRadiusEndpoints() (radiusEndpoints, error) {
	clientID := strings.TrimSpace(os.Getenv("GATOR_RADIUS_OAUTH_CLIENT_ID"))
	if clientID == "" {
		return radiusEndpoints{}, errors.New("GATOR_RADIUS_OAUTH_CLIENT_ID is required: Gator will not impersonate another application's OAuth client")
	}
	gatewayURL := strings.TrimRight(strings.TrimSpace(os.Getenv("GATOR_RADIUS_GATEWAY")), "/")
	if gatewayURL == "" {
		gatewayURL = "https://radius.pi.dev"
	}
	parsed, err := absoluteURL(gatewayURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return radiusEndpoints{}, errors.New("GATOR_RADIUS_GATEWAY must be an http or https URL")
	}
	return radiusEndpoints{gatewayURL: strings.TrimRight(parsed.String(), "/"), clientID: clientID}, nil
}

func radiusOAuthRefreshFlow() (auth.BrowserFlow, error) {
	endpoints, err := configuredRadiusEndpoints()
	if err != nil {
		return auth.BrowserFlow{}, err
	}
	return auth.BrowserFlow{ClientID: endpoints.clientID, TokenURL: endpoints.gatewayURL + "/v1/oauth/token"}, nil
}
