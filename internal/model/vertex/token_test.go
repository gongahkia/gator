package vertex

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSourceUsesExplicitAccessToken(t *testing.T) {
	token, err := (Source{AccessToken: "access-token"}).Token(context.Background())
	if err != nil || token != "access-token" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
}

func TestSourceRefreshesAuthorizedUserADC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s", request.Method)
		}
		contents, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(contents))
		if err != nil || values.Get("grant_type") != "refresh_token" || values.Get("refresh_token") != "refresh-token" {
			t.Fatalf("token request = %q, err = %v", contents, err)
		}
		_, _ = io.WriteString(writer, `{"access_token":"new-access-token","expires_in":3600}`)
	}))
	defer server.Close()
	path := writeCredentials(t, map[string]string{
		"type":          "authorized_user",
		"client_id":     "client-id",
		"client_secret": "client-secret",
		"refresh_token": "refresh-token",
		"token_uri":     server.URL,
	})
	token, err := (Source{CredentialsPath: path, Client: server.Client()}).Token(context.Background())
	if err != nil || token != "new-access-token" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
}

func TestSourceRefreshesServiceAccountADC(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		contents, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		values, err := url.ParseQuery(string(contents))
		if err != nil || values.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Fatalf("token request = %q, err = %v", contents, err)
		}
		parts := strings.Split(values.Get("assertion"), ".")
		if len(parts) != 3 {
			t.Fatalf("assertion = %q", values.Get("assertion"))
		}
		claims, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil || !strings.Contains(string(claims), "service@example.test") {
			t.Fatalf("claims = %q, err = %v", claims, err)
		}
		_, _ = io.WriteString(writer, `{"access_token":"service-access-token"}`)
	}))
	defer server.Close()
	path := writeCredentials(t, map[string]string{
		"type":         "service_account",
		"client_email": "service@example.test",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":    server.URL,
	})
	token, err := (Source{CredentialsPath: path, Client: server.Client(), Now: func() time.Time { return time.Unix(1000, 0) }}).Token(context.Background())
	if err != nil || token != "service-access-token" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
}

func writeCredentials(t *testing.T, values map[string]string) string {
	t.Helper()
	contents, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	return path
}
