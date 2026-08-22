package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const oauthResponseLimit = 64 * 1024

// BrowserFlow describes an OAuth 2.0 authorization-code flow using PKCE. A
// caller supplies its own client ID: this package never reuses a vendor CLI or
// another agent's application identity.
type BrowserFlow struct {
	ClientID           string
	AuthorizationURL   string
	TokenURL           string
	RedirectURL        string
	Scopes             []string
	AuthorizeParams    map[string]string
	TokenParams        map[string]string
	TokenRequestJSON   bool
	TokenIncludesState bool
	AllowMissingState  bool
	// AllowMissingExpiry accepts OAuth token responses without expires_in. It
	// is needed for standards-compliant authorization servers that issue an
	// opaque token with no client-visible expiry.
	AllowMissingExpiry bool
	// RequireBearerToken rejects a token response whose declared token type is
	// not Bearer. HTTP resource clients use this to avoid accepting a token for
	// an incompatible authorization scheme.
	RequireBearerToken bool
	HTTPClient         *http.Client
}

// BrowserAttempt carries the one-time state and verifier for a browser login.
// It is intentionally short-lived and must not be persisted.
type BrowserAttempt struct {
	flow     BrowserFlow
	state    string
	verifier string
}

// State returns the one-time OAuth state for custom authorization endpoints.
func (a BrowserAttempt) State() string {
	return a.state
}

// CodeVerifier returns the short-lived PKCE verifier for a custom token
// exchange. Callers must never persist or log it.
func (a BrowserAttempt) CodeVerifier() string {
	return a.verifier
}

// RedirectURL returns the validated loopback callback URL for this attempt.
func (a BrowserAttempt) RedirectURL() string {
	return a.flow.RedirectURL
}

// WithClientID returns the same short-lived authorization attempt with a
// public client ID selected after a loopback callback has been reserved (for
// example through OAuth Dynamic Client Registration). The state and PKCE
// verifier remain unchanged and are never persisted.
func (a BrowserAttempt) WithClientID(clientID string) (BrowserAttempt, error) {
	if strings.TrimSpace(clientID) == "" {
		return BrowserAttempt{}, errors.New("OAuth client ID is required")
	}
	a.flow.ClientID = clientID
	return a, nil
}

// BeginBrowserFlow validates a public loopback callback and creates the PKCE
// values needed to present the authorization URL.
func BeginBrowserFlow(flow BrowserFlow) (BrowserAttempt, error) {
	if strings.TrimSpace(flow.ClientID) == "" || strings.TrimSpace(flow.AuthorizationURL) == "" || strings.TrimSpace(flow.TokenURL) == "" {
		return BrowserAttempt{}, errors.New("OAuth client ID, authorization URL, and token URL are required")
	}
	if err := validateLoopbackURL(flow.RedirectURL); err != nil {
		return BrowserAttempt{}, err
	}
	if _, err := absoluteURL(flow.AuthorizationURL); err != nil {
		return BrowserAttempt{}, fmt.Errorf("invalid OAuth authorization URL: %w", err)
	}
	if _, err := absoluteURL(flow.TokenURL); err != nil {
		return BrowserAttempt{}, fmt.Errorf("invalid OAuth token URL: %w", err)
	}
	state, err := randomURLValue(24)
	if err != nil {
		return BrowserAttempt{}, fmt.Errorf("generate OAuth state: %w", err)
	}
	verifier, err := randomURLValue(48)
	if err != nil {
		return BrowserAttempt{}, fmt.Errorf("generate OAuth verifier: %w", err)
	}
	return BrowserAttempt{flow: flow, state: state, verifier: verifier}, nil
}

// AuthorizationURL returns the browser URL for this one-time attempt.
func (a BrowserAttempt) AuthorizationURL() string {
	authorize, _ := url.Parse(a.flow.AuthorizationURL)
	query := authorize.Query()
	query.Set("client_id", a.flow.ClientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", a.flow.RedirectURL)
	query.Set("code_challenge", pkceChallenge(a.verifier))
	query.Set("code_challenge_method", "S256")
	query.Set("state", a.state)
	if len(a.flow.Scopes) > 0 {
		query.Set("scope", strings.Join(a.flow.Scopes, " "))
	}
	for key, value := range a.flow.AuthorizeParams {
		query.Set(key, value)
	}
	authorize.RawQuery = query.Encode()
	return authorize.String()
}

// Callback serves exactly one loopback OAuth callback for an attempt.
type Callback struct {
	server   *http.Server
	listener net.Listener
	result   chan callbackResult
	once     sync.Once
}

type callbackResult struct {
	code string
	err  error
}

// StartCallback begins a local callback server before the browser is opened.
func (a BrowserAttempt) StartCallback() (*Callback, error) {
	callbackURL, err := url.Parse(a.flow.RedirectURL)
	if err != nil {
		return nil, fmt.Errorf("parse OAuth redirect URL: %w", err)
	}
	listener, err := net.Listen("tcp", callbackURL.Host)
	if err != nil {
		return nil, fmt.Errorf("listen for OAuth callback: %w", err)
	}
	return a.startCallback(listener, callbackURL)
}

// BeginEphemeralLoopbackFlow reserves a private 127.0.0.1 callback before the
// authorization URL is created. The actual redirect URI can therefore be sent
// to Dynamic Client Registration without a port-selection race.
func BeginEphemeralLoopbackFlow(flow BrowserFlow, callbackPath string) (BrowserAttempt, *Callback, error) {
	if strings.TrimSpace(callbackPath) == "" || !strings.HasPrefix(callbackPath, "/") || strings.Contains(callbackPath, "?") || strings.Contains(callbackPath, "#") {
		return BrowserAttempt{}, nil, errors.New("OAuth callback path must be an absolute path without query or fragment")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return BrowserAttempt{}, nil, fmt.Errorf("listen for OAuth callback: %w", err)
	}
	closeListener := true
	defer func() {
		if closeListener {
			_ = listener.Close()
		}
	}()
	flow.RedirectURL = "http://" + listener.Addr().String() + callbackPath
	attempt, err := BeginBrowserFlow(flow)
	if err != nil {
		return BrowserAttempt{}, nil, err
	}
	callbackURL, err := url.Parse(flow.RedirectURL)
	if err != nil {
		return BrowserAttempt{}, nil, fmt.Errorf("parse OAuth redirect URL: %w", err)
	}
	callback, err := attempt.startCallback(listener, callbackURL)
	if err != nil {
		return BrowserAttempt{}, nil, err
	}
	closeListener = false
	return attempt, callback, nil
}

func (a BrowserAttempt) startCallback(listener net.Listener, callbackURL *url.URL) (*Callback, error) {
	if listener == nil || callbackURL == nil {
		return nil, errors.New("OAuth callback listener is required")
	}
	callback := &Callback{listener: listener, result: make(chan callbackResult, 1)}
	callback.server = &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != callbackURL.Path {
			http.NotFound(writer, request)
			return
		}
		if message := strings.TrimSpace(request.URL.Query().Get("error")); message != "" {
			callback.finish(callbackResult{err: fmt.Errorf("OAuth authorization failed: %s", message)})
			writeCallbackPage(writer, http.StatusBadRequest, "Authentication did not complete. Return to Gator.")
			return
		}
		if !a.flow.AllowMissingState && request.URL.Query().Get("state") != a.state {
			callback.finish(callbackResult{err: errors.New("OAuth callback state mismatch")})
			writeCallbackPage(writer, http.StatusBadRequest, "Authentication state did not match. Return to Gator.")
			return
		}
		code := strings.TrimSpace(request.URL.Query().Get("code"))
		if code == "" {
			callback.finish(callbackResult{err: errors.New("OAuth callback did not include an authorization code")})
			writeCallbackPage(writer, http.StatusBadRequest, "Authentication returned no code. Return to Gator.")
			return
		}
		callback.finish(callbackResult{code: code})
		writeCallbackPage(writer, http.StatusOK, "Authentication completed. You can close this page and return to Gator.")
	})}
	go func() {
		_ = callback.server.Serve(listener)
	}()
	return callback, nil
}

// Wait returns the one callback result or stops when the caller cancels.
func (c *Callback) Wait(ctx context.Context) (string, error) {
	if c == nil {
		return "", errors.New("OAuth callback server is not initialized")
	}
	defer c.Close()
	select {
	case result := <-c.result:
		return result.code, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Close stops the callback server and releases its loopback port.
func (c *Callback) Close() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		_ = c.server.Close()
	})
}

func (c *Callback) finish(result callbackResult) {
	select {
	case c.result <- result:
	default:
	}
}

// Exchange converts the one-time authorization code into a stored OAuth
// credential. The token response is bounded and never included in errors.
func (a BrowserAttempt) Exchange(ctx context.Context, code string) (Credential, error) {
	if strings.TrimSpace(code) == "" {
		return Credential{}, errors.New("OAuth authorization code is required")
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {a.flow.ClientID},
		"code":          {code},
		"code_verifier": {a.verifier},
		"redirect_uri":  {a.flow.RedirectURL},
	}
	if a.flow.TokenIncludesState {
		form.Set("state", a.state)
	}
	return a.flow.exchangeToken(ctx, form, "authorization code")
}

// Refresh exchanges a rotating OAuth refresh token. Providers that do not
// rotate the refresh token may omit it from their response; the previous token
// is retained in that case.
func (f BrowserFlow) Refresh(ctx context.Context, credential Credential) (Credential, error) {
	if !credential.IsOAuth() || strings.TrimSpace(credential.Refresh) == "" {
		return Credential{}, errors.New("OAuth credential has no refresh token")
	}
	if strings.TrimSpace(f.ClientID) == "" || strings.TrimSpace(f.TokenURL) == "" {
		return Credential{}, errors.New("OAuth client ID and token URL are required for refresh")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {f.ClientID},
		"refresh_token": {credential.Refresh},
	}
	for key, value := range f.TokenParams {
		form.Set(key, value)
	}
	refreshed, err := f.exchangeToken(ctx, form, "refresh token")
	if err != nil {
		return Credential{}, err
	}
	if refreshed.Refresh == "" {
		refreshed.Refresh = credential.Refresh
	}
	refreshed.Extra = cloneCredential(credential).Extra
	return refreshed, nil
}

func (f BrowserFlow) exchangeToken(ctx context.Context, form url.Values, action string) (Credential, error) {
	for key, value := range f.TokenParams {
		form.Set(key, value)
	}
	var body io.Reader = strings.NewReader(form.Encode())
	contentType := "application/x-www-form-urlencoded"
	if f.TokenRequestJSON {
		payload := make(map[string]string, len(form))
		for key, values := range form {
			if len(values) > 0 {
				payload[key] = values[len(values)-1]
			}
		}
		contents, err := json.Marshal(payload)
		if err != nil {
			return Credential{}, fmt.Errorf("encode OAuth %s request: %w", action, err)
		}
		body = bytes.NewReader(contents)
		contentType = "application/json"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, f.TokenURL, body)
	if err != nil {
		return Credential{}, fmt.Errorf("create OAuth %s request: %w", action, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", contentType)
	response, err := f.client().Do(request)
	if err != nil {
		return Credential{}, fmt.Errorf("exchange OAuth %s: %w", action, err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, oauthResponseLimit))
	if err != nil {
		return Credential{}, fmt.Errorf("read OAuth token response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Credential{}, fmt.Errorf("OAuth %s exchange returned HTTP %d", action, response.StatusCode)
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(contents, &token); err != nil {
		return Credential{}, fmt.Errorf("OAuth %s exchange returned invalid JSON", action)
	}
	if strings.TrimSpace(token.AccessToken) == "" || (token.ExpiresIn <= 0 && !f.AllowMissingExpiry) {
		return Credential{}, fmt.Errorf("OAuth %s exchange returned incomplete credentials", action)
	}
	if f.RequireBearerToken && token.TokenType != "" && !strings.EqualFold(token.TokenType, "bearer") {
		return Credential{}, fmt.Errorf("OAuth %s exchange returned unsupported token type %q", action, token.TokenType)
	}
	expires := int64(0)
	if token.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli()
	}
	return Credential{Type: oauthType, Access: token.AccessToken, Refresh: token.RefreshToken, Expires: expires}, nil
}

func (f BrowserFlow) client() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func randomURLValue(bytes int) (string, error) {
	contents := make([]byte, bytes)
	if _, err := rand.Read(contents); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(contents), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// PKCEChallenge derives an S256 challenge for a short-lived verifier. It is
// exported for OAuth providers whose authorization endpoint has a nonstandard
// request shape but still uses standard PKCE.
func PKCEChallenge(verifier string) string {
	return pkceChallenge(verifier)
}

func validateLoopbackURL(value string) error {
	parsed, err := absoluteURL(value)
	if err != nil {
		return fmt.Errorf("invalid OAuth redirect URL: %w", err)
	}
	if parsed.Scheme != "http" || parsed.User != nil || parsed.Port() == "" || parsed.Path == "" {
		return errors.New("OAuth redirect URL must be an http loopback URL with a port and path")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return errors.New("OAuth redirect URL must use a loopback host")
	}
	return nil
}

func absoluteURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("must be an absolute URL")
	}
	return parsed, nil
}

func writeCallbackPage(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, "<!doctype html><title>Gator authentication</title><p>"+message+"</p>")
}
