package bedrock

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultResolvesStandardCredentialSources(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*testing.T) (Signer, string)
	}{
		{
			name: "static environment",
			run: func(t *testing.T) (Signer, string) {
				clearCredentialEnvironment(t)
				t.Setenv("AWS_ACCESS_KEY_ID", "STATICKEY")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "static-secret")
				t.Setenv("AWS_REGION", "us-west-2")
				signer, err := LoadDefault(context.Background(), nil)
				if err != nil {
					t.Fatalf("load static credentials: %v", err)
				}
				if signer.Region() != "us-west-2" {
					t.Fatalf("region = %q", signer.Region())
				}
				return signer, "STATICKEY"
			},
		},
		{
			name: "selected shared profile",
			run: func(t *testing.T) (Signer, string) {
				clearCredentialEnvironment(t)
				directory := t.TempDir()
				credentialsPath := filepath.Join(directory, "credentials")
				if err := os.WriteFile(credentialsPath, []byte("[gator]\naws_access_key_id = PROFILEKEY\naws_secret_access_key = profile-secret\n"), 0o600); err != nil {
					t.Fatalf("write credentials: %v", err)
				}
				t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
				t.Setenv("AWS_CONFIG_FILE", filepath.Join(directory, "config"))
				t.Setenv("AWS_PROFILE", "gator")
				t.Setenv("AWS_REGION", "us-east-2")
				signer, err := LoadDefault(context.Background(), nil)
				if err != nil {
					t.Fatalf("load profile credentials: %v", err)
				}
				return signer, "PROFILEKEY"
			},
		},
		{
			name: "ECS task endpoint",
			run: func(t *testing.T) (Signer, string) {
				clearCredentialEnvironment(t)
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path != "/credentials" {
						t.Fatalf("ECS path = %q", request.URL.Path)
					}
					_, _ = io.WriteString(writer, `{"AccessKeyId":"ECSKEY","SecretAccessKey":"ecs-secret","Token":"ecs-session","Expiration":"2030-01-01T00:00:00Z"}`)
				}))
				t.Cleanup(server.Close)
				t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", server.URL+"/credentials")
				t.Setenv("AWS_REGION", "us-east-1")
				signer, err := LoadDefault(context.Background(), server.Client())
				if err != nil {
					t.Fatalf("load ECS credentials: %v", err)
				}
				return signer, "ECSKEY"
			},
		},
		{
			name: "web identity",
			run: func(t *testing.T) (Signer, string) {
				clearCredentialEnvironment(t)
				directory := t.TempDir()
				tokenPath := filepath.Join(directory, "identity-token")
				if err := os.WriteFile(tokenPath, []byte("identity-token"), 0o600); err != nil {
					t.Fatalf("write web identity token: %v", err)
				}
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					contents, err := io.ReadAll(request.Body)
					if request.Method != http.MethodPost || err != nil || !strings.Contains(string(contents), "AssumeRoleWithWebIdentity") || !strings.Contains(string(contents), "identity-token") {
						t.Fatalf("STS request = %s %q, err = %v", request.Method, contents, err)
					}
					writer.Header().Set("Content-Type", "text/xml")
					_, _ = io.WriteString(writer, `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleWithWebIdentityResult><Credentials><AccessKeyId>WEBKEY</AccessKeyId><SecretAccessKey>web-secret</SecretAccessKey><SessionToken>web-session</SessionToken><Expiration>2030-01-01T00:00:00Z</Expiration></Credentials></AssumeRoleWithWebIdentityResult></AssumeRoleWithWebIdentityResponse>`)
				}))
				t.Cleanup(server.Close)
				t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", tokenPath)
				t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/gator")
				t.Setenv("AWS_ROLE_SESSION_NAME", "gator-test")
				t.Setenv("AWS_ENDPOINT_URL_STS", server.URL)
				t.Setenv("AWS_REGION", "us-east-1")
				signer, err := LoadDefault(context.Background(), server.Client())
				if err != nil {
					t.Fatalf("load web identity credentials: %v", err)
				}
				return signer, "WEBKEY"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			signer, accessKeyID := test.run(t)
			request, err := http.NewRequest(http.MethodPost, "https://bedrock-runtime.us-east-1.amazonaws.com/v1/chat/completions", strings.NewReader(`{"model":"test"}`))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			if err := signer.SignRequest(context.Background(), request, []byte(`{"model":"test"}`)); err != nil {
				t.Fatalf("sign request: %v", err)
			}
			if got := request.Header.Get("Authorization"); !strings.Contains(got, "Credential="+accessKeyID+"/") || strings.Contains(got, "Bearer") {
				t.Fatalf("authorization = %q", got)
			}
		})
	}
}

func TestAmbientSourceDoesNotExposeCredentialValues(t *testing.T) {
	clearCredentialEnvironment(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "do-not-expose")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	source, available := AmbientSource()
	if !available || source != "AWS static credentials in environment" || strings.Contains(source, "do-not-expose") {
		t.Fatalf("ambient source = %q, available=%v", source, available)
	}
}

func clearCredentialEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN", "AWS_ROLE_SESSION_NAME",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_AUTHORIZATION_TOKEN", "AWS_ENDPOINT_URL_STS",
		"AWS_PROFILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_CONFIG_FILE", "AWS_REGION", "AWS_DEFAULT_REGION",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}
