package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultDevicePollInterval = 5 * time.Second

// DeviceFlow implements RFC 8628 device authorization for a caller-owned
// OAuth client. It only displays the verification URL and user code; opening a
// browser remains an explicit user action.
type DeviceFlow struct {
	ClientID       string
	DeviceCodeURL  string
	TokenURL       string
	Scope          string
	DeviceParams   map[string]string
	Headers        http.Header
	HTTPClient     *http.Client
	WaitBeforePoll bool
}

// DeviceAuthorization is the short-lived result displayed to the user.
type DeviceAuthorization struct {
	DeviceCode      string
	UserCode        string
	VerificationURL string
	PollInterval    time.Duration
	ExpiresAt       time.Time
}

// DeviceToken is the token payload returned after the device is approved.
type DeviceToken struct {
	Access  string
	Refresh string
	Expires time.Duration
}

// Start requests a device and user code from the provider.
func (f DeviceFlow) Start(ctx context.Context) (DeviceAuthorization, error) {
	if strings.TrimSpace(f.ClientID) == "" || strings.TrimSpace(f.DeviceCodeURL) == "" || strings.TrimSpace(f.TokenURL) == "" {
		return DeviceAuthorization{}, errors.New("OAuth client ID, device-code URL, and token URL are required")
	}
	form := url.Values{"client_id": {f.ClientID}}
	if strings.TrimSpace(f.Scope) != "" {
		form.Set("scope", f.Scope)
	}
	for key, value := range f.DeviceParams {
		form.Set(key, value)
	}
	response, err := f.post(ctx, f.DeviceCodeURL, form)
	if err != nil {
		return DeviceAuthorization{}, fmt.Errorf("request OAuth device code: %w", err)
	}
	var payload struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		Interval                int64  `json:"interval"`
		ExpiresIn               int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(response, &payload); err != nil {
		return DeviceAuthorization{}, errors.New("OAuth device code response returned invalid JSON")
	}
	if strings.TrimSpace(payload.DeviceCode) == "" || strings.TrimSpace(payload.UserCode) == "" || strings.TrimSpace(payload.VerificationURI) == "" || payload.ExpiresIn <= 0 {
		return DeviceAuthorization{}, errors.New("OAuth device code response was incomplete")
	}
	verificationURI := payload.VerificationURI
	if strings.TrimSpace(payload.VerificationURIComplete) != "" {
		verificationURI = payload.VerificationURIComplete
	}
	verification, err := absoluteURL(verificationURI)
	if err != nil || (verification.Scheme != "https" && verification.Scheme != "http") {
		return DeviceAuthorization{}, errors.New("OAuth device code response included an unsafe verification URL")
	}
	interval := time.Duration(payload.Interval) * time.Second
	if interval <= 0 {
		interval = defaultDevicePollInterval
	}
	return DeviceAuthorization{
		DeviceCode:      payload.DeviceCode,
		UserCode:        payload.UserCode,
		VerificationURL: verification.String(),
		PollInterval:    interval,
		ExpiresAt:       time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

// Poll waits for the user to approve the device then returns the OAuth access
// token. The token endpoint's response is bounded and never echoed in errors.
func (f DeviceFlow) Poll(ctx context.Context, authorization DeviceAuthorization) (string, error) {
	token, err := f.PollToken(ctx, authorization)
	if err != nil {
		return "", err
	}
	return token.Access, nil
}

// PollToken waits for device authorization and preserves a provider-issued
// refresh token and expiry for callers that need to persist them.
func (f DeviceFlow) PollToken(ctx context.Context, authorization DeviceAuthorization) (DeviceToken, error) {
	if strings.TrimSpace(authorization.DeviceCode) == "" || authorization.ExpiresAt.IsZero() {
		return DeviceToken{}, errors.New("OAuth device authorization is incomplete")
	}
	interval := authorization.PollInterval
	if interval <= 0 {
		interval = defaultDevicePollInterval
	}
	if f.WaitBeforePoll {
		if err := waitDevicePoll(ctx, interval); err != nil {
			return DeviceToken{}, err
		}
	}
	for {
		if time.Now().After(authorization.ExpiresAt) {
			return DeviceToken{}, errors.New("OAuth device authorization expired")
		}
		form := url.Values{
			"client_id":   {f.ClientID},
			"device_code": {authorization.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}
		contents, status, err := f.postStatus(ctx, f.TokenURL, form)
		if err != nil {
			return DeviceToken{}, fmt.Errorf("poll OAuth device authorization: %w", err)
		}
		var payload struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
			Error        string `json:"error"`
			Description  string `json:"error_description"`
			Interval     int64  `json:"interval"`
		}
		if err := json.Unmarshal(contents, &payload); err != nil {
			return DeviceToken{}, errors.New("OAuth device token response returned invalid JSON")
		}
		if status >= http.StatusOK && status < http.StatusMultipleChoices && strings.TrimSpace(payload.AccessToken) != "" {
			return DeviceToken{Access: payload.AccessToken, Refresh: payload.RefreshToken, Expires: time.Duration(payload.ExpiresIn) * time.Second}, nil
		}
		switch payload.Error {
		case "authorization_pending":
		case "slow_down":
			if payload.Interval > 0 {
				interval = time.Duration(payload.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case "access_denied", "authorization_denied":
			return DeviceToken{}, errors.New("OAuth device authorization was denied")
		case "expired_token":
			return DeviceToken{}, errors.New("OAuth device authorization expired")
		default:
			return DeviceToken{}, fmt.Errorf("OAuth device authorization failed (HTTP %d)", status)
		}
		if err := waitDevicePoll(ctx, interval); err != nil {
			return DeviceToken{}, err
		}
	}
}

func (f DeviceFlow) post(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	contents, status, err := f.postStatus(ctx, endpoint, form)
	if err != nil {
		return nil, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("OAuth server returned HTTP %d", status)
	}
	return contents, nil
}

func (f DeviceFlow) postStatus(ctx context.Context, endpoint string, form url.Values) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("create OAuth device request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for name, values := range f.Headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := f.client().Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, oauthResponseLimit))
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("read OAuth device response: %w", err)
	}
	return contents, response.StatusCode, nil
}

func (f DeviceFlow) client() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func waitDevicePoll(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
