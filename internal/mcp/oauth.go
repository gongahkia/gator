package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	oauthMetadataLimit = 64 * 1024
	oauthRefreshWindow = 5 * time.Minute
)

const (
	extraResource = "mcp_resource"
	extraTokenURL = "mcp_token_url"
	extraClientID = "mcp_client_id"
)

// OAuthLoginOptions provides testable transport and pre-registered-client
// seams for an explicit local MCP login. A ClientID is public metadata, never
// a client secret.
type OAuthLoginOptions struct {
	HTTPClient *http.Client
	ClientID   string
	// RedirectURL is an exact loopback redirect URI already registered to a
	// pre-registered public client. It is required when that client does not
	// permit an ephemeral port through Dynamic Client Registration.
	RedirectURL string
}

// OAuthLogin owns a one-time browser authorization callback. It keeps PKCE
// state only in memory until Complete stores the returned credential.
type OAuthLogin struct {
	attempt       auth.BrowserAttempt
	callback      *auth.Callback
	credentials   auth.Store
	credentialKey string
	resource      string
	tokenURL      string
	clientID      string
}

// HTTPAuthorizationStatus is safe to display in a local status UI. It never
// includes a token, client ID, authorization URL, or endpoint URL.
type HTTPAuthorizationStatus struct {
	Server        string
	Authenticated bool
	Expired       bool
}

// HTTPAuthorizationStatuses reports whether each configured Streamable HTTP
// server has a private credential bound to its current exact resource. It does
// not contact any remote service.
func HTTPAuthorizationStatuses(repository string, credentials auth.Store, now time.Time) ([]HTTPAuthorizationStatus, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return nil, err
	}
	document, _, err := loadManifest(root)
	if err != nil {
		return nil, err
	}
	statuses := make([]HTTPAuthorizationStatus, 0, len(document.Servers))
	for _, specification := range document.Servers {
		if specification.Transport != "streamable_http" {
			continue
		}
		status := HTTPAuthorizationStatus{Server: specification.Name}
		resource, err := canonicalResource(specification.URL)
		if err != nil || strings.TrimSpace(credentials.Path()) == "" {
			statuses = append(statuses, status)
			continue
		}
		key, err := OAuthCredentialKey(resource)
		if err != nil {
			statuses = append(statuses, status)
			continue
		}
		credential, found, err := credentials.Read(key)
		if err == nil && found && credential.IsOAuth() && credential.Extra[extraResource] == resource && strings.TrimSpace(credential.Extra[extraClientID]) != "" && strings.TrimSpace(credential.Extra[extraTokenURL]) != "" {
			status.Expired = credential.Expired(now)
			status.Authenticated = !status.Expired
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// BeginOAuthLogin discovers the authorization server for one trusted
// Streamable HTTP server and starts a loopback PKCE flow. It deliberately
// refuses untrusted project configuration before sending a browser URL or
// storing a token.
func BeginOAuthLogin(ctx context.Context, repository, serverName, trustedHash string, credentials auth.Store, options OAuthLoginOptions) (*OAuthLogin, error) {
	if strings.TrimSpace(credentials.Path()) == "" {
		return nil, errors.New("MCP OAuth credential store is not initialized")
	}
	root, err := workspace.Open(repository)
	if err != nil {
		return nil, err
	}
	document, digest, err := loadManifest(root)
	if err != nil {
		return nil, err
	}
	if digest == "" || trustedHash != digest {
		return nil, errors.New("project MCP configuration is not trusted; review it and run 'gator mcp trust' before authenticating a remote server")
	}
	specification, found := findHTTPServer(document, serverName)
	if !found {
		return nil, fmt.Errorf("trusted Streamable HTTP MCP server %q is not configured", serverName)
	}
	resource, err := canonicalResource(specification.URL)
	if err != nil {
		return nil, err
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	client = oauthHTTPClient(client)
	protected, challengeScopes, err := discoverProtectedResource(ctx, client, resource)
	if err != nil {
		return nil, fmt.Errorf("discover MCP authorization server: %w", err)
	}
	authorization, err := discoverAuthorizationServer(ctx, client, protected.AuthorizationServers[0])
	if err != nil {
		return nil, fmt.Errorf("discover OAuth authorization server: %w", err)
	}
	if !authorization.supportsS256() {
		return nil, errors.New("MCP authorization server does not advertise PKCE S256 support")
	}
	scopes := challengeScopes
	if len(scopes) == 0 {
		scopes = append([]string(nil), protected.ScopesSupported...)
	}
	clientID := strings.TrimSpace(options.ClientID)
	if clientID != "" && strings.TrimSpace(options.RedirectURL) == "" {
		return nil, errors.New("a pre-registered MCP OAuth client requires its exact loopback redirect URI; retry with 'gator mcp login --client-id CLIENT_ID --redirect-url LOOPBACK_URL " + serverName + "'")
	}
	flow := auth.BrowserFlow{
		ClientID:           clientID,
		AuthorizationURL:   authorization.AuthorizationEndpoint,
		TokenURL:           authorization.TokenEndpoint,
		Scopes:             scopes,
		AuthorizeParams:    map[string]string{"resource": resource},
		TokenParams:        map[string]string{"resource": resource},
		AllowMissingExpiry: true,
		RequireBearerToken: true,
		HTTPClient:         client,
	}
	var attempt auth.BrowserAttempt
	var callback *auth.Callback
	if strings.TrimSpace(options.RedirectURL) != "" {
		if clientID == "" {
			if authorization.RegistrationEndpoint == "" {
				return nil, errors.New("MCP authorization server has no dynamic registration endpoint; register a public OAuth client and retry with 'gator mcp login --client-id CLIENT_ID --redirect-url LOOPBACK_URL " + serverName + "'")
			}
			clientID, err = registerPublicClient(ctx, client, authorization.RegistrationEndpoint, options.RedirectURL)
			if err != nil {
				return nil, fmt.Errorf("register public MCP OAuth client: %w", err)
			}
		}
		flow.ClientID = clientID
		flow.RedirectURL = options.RedirectURL
		attempt, err = auth.BeginBrowserFlow(flow)
		if err != nil {
			return nil, fmt.Errorf("start MCP OAuth flow: %w", err)
		}
		callback, err = attempt.StartCallback()
		if err != nil {
			return nil, fmt.Errorf("start MCP OAuth callback: %w", err)
		}
	} else {
		flow.ClientID = "gator-pending-registration"
		attempt, callback, err = auth.BeginEphemeralLoopbackFlow(flow, "/gator/mcp/oauth/callback")
		if err != nil {
			return nil, fmt.Errorf("start MCP OAuth callback: %w", err)
		}
		if clientID == "" {
			if authorization.RegistrationEndpoint == "" {
				callback.Close()
				return nil, errors.New("MCP authorization server has no dynamic registration endpoint; register a public OAuth client and retry with 'gator mcp login --client-id CLIENT_ID --redirect-url LOOPBACK_URL " + serverName + "'")
			}
			clientID, err = registerPublicClient(ctx, client, authorization.RegistrationEndpoint, attempt.RedirectURL())
			if err != nil {
				callback.Close()
				return nil, fmt.Errorf("register public MCP OAuth client: %w", err)
			}
		}
		attempt, err = attempt.WithClientID(clientID)
		if err != nil {
			callback.Close()
			return nil, err
		}
	}
	key, err := OAuthCredentialKey(resource)
	if err != nil {
		return nil, err
	}
	return &OAuthLogin{attempt: attempt, callback: callback, credentials: credentials, credentialKey: key, resource: resource, tokenURL: authorization.TokenEndpoint, clientID: clientID}, nil
}

// URL returns the browser URL for this one-time MCP authorization flow.
func (l *OAuthLogin) URL() string {
	if l == nil {
		return ""
	}
	return l.attempt.AuthorizationURL()
}

// RedirectURL returns the exact loopback callback registered for this flow.
func (l *OAuthLogin) RedirectURL() string {
	if l == nil {
		return ""
	}
	return l.attempt.RedirectURL()
}

// Complete waits for the loopback callback, exchanges the authorization code,
// and atomically stores a token bound to this exact MCP resource.
func (l *OAuthLogin) Complete(ctx context.Context) error {
	if l == nil || l.callback == nil {
		return errors.New("MCP OAuth login is not initialized")
	}
	defer l.callback.Close()
	code, err := l.callback.Wait(ctx)
	if err != nil {
		return err
	}
	credential, err := l.attempt.Exchange(ctx, code)
	if err != nil {
		return err
	}
	credential.Extra = map[string]string{extraResource: l.resource, extraTokenURL: l.tokenURL, extraClientID: l.clientID}
	if err := l.credentials.Put(l.credentialKey, credential); err != nil {
		return err
	}
	return nil
}

// Cancel releases the local callback without storing a credential.
func (l *OAuthLogin) Cancel() {
	if l != nil && l.callback != nil {
		l.callback.Close()
	}
}

// OAuthCredentialKey returns the private-store key for an exact canonical MCP
// resource. It intentionally contains no server name or host, which avoids
// leaking a configured remote endpoint through the credential-store index.
func OAuthCredentialKey(endpoint string) (string, error) {
	resource, err := canonicalResource(endpoint)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(resource))
	return "mcp-" + hex.EncodeToString(digest[:16]), nil
}

// OAuthCredentialKeyForServer resolves the current Streamable HTTP server
// entry to its private credential-store key. It performs no network request
// and is safe to use for an explicit logout operation.
func OAuthCredentialKeyForServer(repository, serverName string) (string, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return "", err
	}
	document, _, err := loadManifest(root)
	if err != nil {
		return "", err
	}
	specification, found := findHTTPServer(document, serverName)
	if !found {
		return "", fmt.Errorf("Streamable HTTP MCP server %q is not configured", serverName)
	}
	return OAuthCredentialKey(specification.URL)
}

func findHTTPServer(document manifest, name string) (server, bool) {
	for _, specification := range document.Servers {
		if specification.Name == name && specification.Transport == "streamable_http" {
			return specification, true
		}
	}
	return server{}, false
}

func accessToken(ctx context.Context, credentials auth.Store, endpoint string, now time.Time) (string, bool, error) {
	if strings.TrimSpace(credentials.Path()) == "" {
		return "", false, nil
	}
	resource, err := canonicalResource(endpoint)
	if err != nil {
		return "", false, err
	}
	key, err := OAuthCredentialKey(resource)
	if err != nil {
		return "", false, err
	}
	credential, found, err := credentials.Read(key)
	if err != nil || !found {
		return "", false, err
	}
	if !credential.IsOAuth() || credential.Extra[extraResource] != resource || strings.TrimSpace(credential.Extra[extraClientID]) == "" || strings.TrimSpace(credential.Extra[extraTokenURL]) == "" {
		return "", true, errors.New("stored MCP OAuth credential has incompatible resource metadata; run 'gator mcp logout SERVER' then 'gator mcp login SERVER'")
	}
	if credential.Expires == 0 || credential.Expires > now.Add(oauthRefreshWindow).UnixMilli() {
		return credential.Access, true, nil
	}
	if strings.TrimSpace(credential.Refresh) == "" {
		return "", true, errors.New("stored MCP OAuth credential has expired and cannot be refreshed; run 'gator mcp login SERVER'")
	}
	flow := auth.BrowserFlow{
		ClientID:           credential.Extra[extraClientID],
		TokenURL:           credential.Extra[extraTokenURL],
		TokenParams:        map[string]string{"resource": resource},
		AllowMissingExpiry: true,
		RequireBearerToken: true,
	}
	refreshed, err := flow.Refresh(ctx, credential)
	if err != nil {
		return "", true, fmt.Errorf("refresh stored MCP OAuth credential: %w", err)
	}
	if err := credentials.Put(key, refreshed); err != nil {
		return "", true, fmt.Errorf("store refreshed MCP OAuth credential: %w", err)
	}
	return refreshed.Access, true, nil
}

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

func discoverProtectedResource(ctx context.Context, client *http.Client, resource string) (protectedResourceMetadata, []string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, resource, nil)
	if err != nil {
		return protectedResourceMetadata{}, nil, err
	}
	request.Header.Set("MCP-Protocol-Version", protocolVersion)
	response, err := client.Do(request)
	if err != nil {
		return protectedResourceMetadata{}, nil, err
	}
	metadataURL, scopes := bearerChallenge(response.Header.Values("WWW-Authenticate"))
	_ = response.Body.Close()
	if metadataURL != "" {
		metadata, err := fetchProtectedMetadata(ctx, client, metadataURL)
		return metadata, scopes, err
	}
	for _, candidate := range protectedMetadataURLs(resource) {
		metadata, err := fetchProtectedMetadata(ctx, client, candidate)
		if err == nil {
			return metadata, scopes, nil
		}
		if !errors.Is(err, errMetadataUnavailable) {
			return protectedResourceMetadata{}, nil, err
		}
	}
	return protectedResourceMetadata{}, nil, errors.New("MCP server did not provide OAuth protected-resource metadata")
}

var errMetadataUnavailable = errors.New("OAuth metadata is unavailable")

func fetchProtectedMetadata(ctx context.Context, client *http.Client, endpoint string) (protectedResourceMetadata, error) {
	contents, err := fetchJSON(ctx, client, endpoint)
	if err != nil {
		return protectedResourceMetadata{}, err
	}
	var metadata protectedResourceMetadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return protectedResourceMetadata{}, errors.New("protected-resource metadata is not valid JSON")
	}
	if len(metadata.AuthorizationServers) == 0 {
		return protectedResourceMetadata{}, errors.New("protected-resource metadata has no authorization_servers")
	}
	for _, issuer := range metadata.AuthorizationServers {
		if _, err := canonicalOAuthURL(issuer); err != nil {
			return protectedResourceMetadata{}, fmt.Errorf("protected-resource metadata has invalid authorization server: %w", err)
		}
	}
	if err := validateScopes(metadata.ScopesSupported); err != nil {
		return protectedResourceMetadata{}, fmt.Errorf("protected-resource metadata scopes_supported: %w", err)
	}
	return metadata, nil
}

type authorizationServerMetadata struct {
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported"`
}

func (m authorizationServerMetadata) supportsS256() bool {
	for _, method := range m.CodeChallengeMethodsSupported {
		if method == "S256" {
			return true
		}
	}
	return false
}

func discoverAuthorizationServer(ctx context.Context, client *http.Client, issuer string) (authorizationServerMetadata, error) {
	var unavailable error
	for _, endpoint := range authorizationMetadataURLs(issuer) {
		contents, err := fetchJSON(ctx, client, endpoint)
		if err != nil {
			if errors.Is(err, errMetadataUnavailable) {
				unavailable = err
				continue
			}
			return authorizationServerMetadata{}, err
		}
		var metadata authorizationServerMetadata
		if err := json.Unmarshal(contents, &metadata); err != nil {
			return authorizationServerMetadata{}, errors.New("authorization-server metadata is not valid JSON")
		}
		if _, err := canonicalOAuthURL(metadata.AuthorizationEndpoint); err != nil {
			return authorizationServerMetadata{}, fmt.Errorf("authorization-server metadata has invalid authorization_endpoint: %w", err)
		}
		if _, err := canonicalOAuthURL(metadata.TokenEndpoint); err != nil {
			return authorizationServerMetadata{}, fmt.Errorf("authorization-server metadata has invalid token_endpoint: %w", err)
		}
		if metadata.RegistrationEndpoint != "" {
			if _, err := canonicalOAuthURL(metadata.RegistrationEndpoint); err != nil {
				return authorizationServerMetadata{}, fmt.Errorf("authorization-server metadata has invalid registration_endpoint: %w", err)
			}
		}
		return metadata, nil
	}
	if unavailable != nil {
		return authorizationServerMetadata{}, errors.New("authorization server did not expose OAuth or OpenID Connect discovery metadata")
	}
	return authorizationServerMetadata{}, errors.New("authorization server metadata is unavailable")
}

func registerPublicClient(ctx context.Context, client *http.Client, endpoint, redirectURL string) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"client_name":                "Gator",
		"redirect_uris":              []string{redirectURL},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, oauthMetadataLimit+1))
	if err != nil {
		return "", err
	}
	if len(contents) > oauthMetadataLimit {
		return "", errors.New("OAuth dynamic-registration response exceeds 64 KiB")
	}
	if response.StatusCode/100 != 2 {
		return "", fmt.Errorf("OAuth dynamic registration returned HTTP %d", response.StatusCode)
	}
	var result struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(contents, &result); err != nil || strings.TrimSpace(result.ClientID) == "" || len(result.ClientID) > 2048 {
		return "", errors.New("OAuth dynamic registration returned no usable public client ID")
	}
	return result.ClientID, nil
}

func fetchJSON(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, oauthMetadataLimit+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > oauthMetadataLimit {
		return nil, errors.New("OAuth metadata response exceeds 64 KiB")
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed {
		return nil, errMetadataUnavailable
	}
	if response.StatusCode/100 != 2 {
		return nil, fmt.Errorf("OAuth metadata returned HTTP %d", response.StatusCode)
	}
	return contents, nil
}

// oauthHTTPClient keeps discovery, registration, and token posts on the exact
// validated endpoint. Following an unexpected redirect could disclose a public
// client registration body or authorization code to another origin.
func oauthHTTPClient(source *http.Client) *http.Client {
	clone := *source
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &clone
}

func protectedMetadataURLs(resource string) []string {
	parsed, err := url.Parse(resource)
	if err != nil {
		return nil
	}
	pathPart := strings.TrimPrefix(parsed.Path, "/")
	base := *parsed
	base.RawQuery = ""
	base.Fragment = ""
	base.Path = "/.well-known/oauth-protected-resource"
	base.RawPath = ""
	result := make([]string, 0, 2)
	if pathPart != "" {
		withPath := base
		withPath.Path += "/" + pathPart
		result = append(result, withPath.String())
	}
	result = append(result, base.String())
	return result
}

func authorizationMetadataURLs(issuer string) []string {
	parsed, err := url.Parse(issuer)
	if err != nil {
		return nil
	}
	issuerPath := strings.Trim(parsed.Path, "/")
	base := *parsed
	base.RawQuery = ""
	base.Fragment = ""
	base.RawPath = ""
	result := make([]string, 0, 3)
	for _, wellKnown := range []string{"oauth-authorization-server", "openid-configuration"} {
		candidate := base
		candidate.Path = "/.well-known/" + wellKnown
		if issuerPath != "" {
			candidate.Path += "/" + issuerPath
		}
		result = append(result, candidate.String())
	}
	if issuerPath != "" {
		candidate := base
		candidate.Path = path.Join("/"+issuerPath, ".well-known/openid-configuration")
		result = append(result, candidate.String())
	}
	return result
}

func canonicalResource(endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("MCP OAuth resource must be an absolute http(s) URL without credentials or fragment")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path == "/" && parsed.RawQuery == "" {
		parsed.Path = ""
	}
	parsed.RawPath = ""
	return parsed.String(), nil
}

func canonicalOAuthURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("must be an absolute URL without credentials or fragment")
	}
	local := parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && local) {
		return "", errors.New("must use HTTPS outside a loopback host")
	}
	return parsed.String(), nil
}

func validateScopes(scopes []string) error {
	if len(scopes) > 128 {
		return errors.New("contains too many scopes")
	}
	for _, scope := range scopes {
		if strings.TrimSpace(scope) == "" || len(scope) > 256 || strings.ContainsAny(scope, " \t\r\n") {
			return errors.New("contains an invalid scope")
		}
	}
	return nil
}

func bearerChallenge(values []string) (string, []string) {
	for _, value := range values {
		start := bearerSchemeOffset(value)
		if start < 0 {
			continue
		}
		parameters := parseChallengeParameters(strings.TrimSpace(value[start+len("Bearer"):]))
		metadata := parameters["resource_metadata"]
		if metadata == "" {
			continue
		}
		scopes := strings.Fields(parameters["scope"])
		if validateScopes(scopes) != nil {
			return "", nil
		}
		if _, err := canonicalOAuthURL(metadata); err != nil {
			return "", nil
		}
		return metadata, scopes
	}
	return "", nil
}

func bearerSchemeOffset(value string) int {
	lower := strings.ToLower(value)
	for start := 0; start < len(lower); {
		index := strings.Index(lower[start:], "bearer")
		if index < 0 {
			return -1
		}
		index += start
		before := index == 0 || lower[index-1] == ',' || lower[index-1] == ' ' || lower[index-1] == '\t'
		after := index+len("bearer") == len(lower) || lower[index+len("bearer")] == ' ' || lower[index+len("bearer")] == '\t'
		if before && after {
			return index
		}
		start = index + len("bearer")
	}
	return -1
}

func parseChallengeParameters(value string) map[string]string {
	result := make(map[string]string)
	for len(value) > 0 {
		value = strings.TrimLeft(value, " \t,")
		if value == "" {
			break
		}
		name, remainder, found := strings.Cut(value, "=")
		if !found {
			break
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			break
		}
		remainder = strings.TrimLeft(remainder, " \t")
		field := ""
		if strings.HasPrefix(remainder, "\"") {
			remainder = remainder[1:]
			var builder strings.Builder
			escaped := false
			index := 0
			for ; index < len(remainder); index++ {
				character := remainder[index]
				if escaped {
					builder.WriteByte(character)
					escaped = false
					continue
				}
				if character == '\\' {
					escaped = true
					continue
				}
				if character == '"' {
					index++
					break
				}
				builder.WriteByte(character)
			}
			field = builder.String()
			remainder = remainder[index:]
		} else {
			index := strings.IndexByte(remainder, ',')
			if index < 0 {
				field, remainder = strings.TrimSpace(remainder), ""
			} else {
				field, remainder = strings.TrimSpace(remainder[:index]), remainder[index:]
			}
		}
		result[name] = field
		value = remainder
	}
	return result
}
