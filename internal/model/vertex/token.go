// Package vertex resolves the credentials required by Vertex AI's
// OpenAI-compatible endpoint without starting another program.
package vertex

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// Source resolves a short-lived Google Cloud access token. AccessToken is an
// explicit escape hatch for environments that obtain the token themselves;
// otherwise Token reads Application Default Credentials (ADC).
type Source struct {
	AccessToken     string
	CredentialsPath string
	Client          *http.Client
	Now             func() time.Time
}

// FromEnvironment constructs the normal Vertex credential source. Gator uses
// GATOR_VERTEX_ACCESS_TOKEN only when the caller intentionally supplies a
// short-lived token; it otherwise follows Google's ADC file convention.
func FromEnvironment() Source {
	return Source{
		AccessToken:     strings.TrimSpace(os.Getenv("GATOR_VERTEX_ACCESS_TOKEN")),
		CredentialsPath: strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
	}
}

// Token obtains an OAuth access token suitable for Authorization: Bearer.
func (s Source) Token(ctx context.Context) (string, error) {
	if token := strings.TrimSpace(s.AccessToken); token != "" {
		return token, nil
	}
	credentials, err := s.credentials()
	if err != nil {
		return "", err
	}
	switch credentials.Type {
	case "authorized_user":
		return s.refreshAuthorizedUser(ctx, credentials)
	case "service_account":
		return s.refreshServiceAccount(ctx, credentials)
	default:
		return "", fmt.Errorf("unsupported Google Application Default Credentials type %q", credentials.Type)
	}
}

type credentials struct {
	Type         string `json:"type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	TokenURI     string `json:"token_uri"`
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
}

func (s Source) credentials() (credentials, error) {
	path := strings.TrimSpace(s.CredentialsPath)
	if path == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return credentials{}, fmt.Errorf("find Google Application Default Credentials: %w", err)
		}
		path = filepath.Join(configDir, "gcloud", "application_default_credentials.json")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return credentials{}, fmt.Errorf("read Google Application Default Credentials at %s: %w", path, err)
	}
	var parsed credentials
	if err := json.Unmarshal(contents, &parsed); err != nil {
		return credentials{}, fmt.Errorf("decode Google Application Default Credentials at %s: %w", path, err)
	}
	parsed.Type = strings.TrimSpace(parsed.Type)
	if parsed.TokenURI == "" {
		parsed.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return parsed, nil
}

func (s Source) refreshAuthorizedUser(ctx context.Context, credentials credentials) (string, error) {
	if credentials.ClientID == "" || credentials.ClientSecret == "" || credentials.RefreshToken == "" {
		return "", errors.New("authorized-user ADC requires client_id, client_secret, and refresh_token")
	}
	return s.tokenRequest(ctx, credentials.TokenURI, url.Values{
		"client_id":     []string{credentials.ClientID},
		"client_secret": []string{credentials.ClientSecret},
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{credentials.RefreshToken},
	})
}

func (s Source) refreshServiceAccount(ctx context.Context, credentials credentials) (string, error) {
	if credentials.ClientEmail == "" || credentials.PrivateKey == "" {
		return "", errors.New("service-account ADC requires client_email and private_key")
	}
	assertion, err := serviceAccountAssertion(credentials, s.now())
	if err != nil {
		return "", err
	}
	return s.tokenRequest(ctx, credentials.TokenURI, url.Values{
		"grant_type": []string{"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  []string{assertion},
	})
}

func (s Source) tokenRequest(ctx context.Context, endpoint string, values url.Values) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("create Google OAuth token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client().Do(request)
	if err != nil {
		return "", fmt.Errorf("request Google OAuth token: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read Google OAuth token response: %w", err)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(contents, &payload); err != nil {
		return "", fmt.Errorf("decode Google OAuth token response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if payload.Error != "" {
			return "", fmt.Errorf("Google OAuth token endpoint returned HTTP %d: %s", response.StatusCode, payload.Error)
		}
		return "", fmt.Errorf("Google OAuth token endpoint returned HTTP %d", response.StatusCode)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", errors.New("Google OAuth token response contained no access_token")
	}
	return payload.AccessToken, nil
}

func (s Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (s Source) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func serviceAccountAssertion(credentials credentials, now time.Time) (string, error) {
	block, _ := pem.Decode([]byte(credentials.PrivateKey))
	if block == nil {
		return "", errors.New("decode service-account private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if pkcs1, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes); parseErr == nil {
			key = pkcs1
		} else {
			return "", fmt.Errorf("parse service-account private key: %w", err)
		}
	}
	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("service-account private key is not RSA")
	}
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", fmt.Errorf("encode service-account JWT header: %w", err)
	}
	claims, err := json.Marshal(map[string]any{
		"iss":   credentials.ClientEmail,
		"scope": cloudPlatformScope,
		"aud":   credentials.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("encode service-account JWT claims: %w", err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign service-account JWT: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
