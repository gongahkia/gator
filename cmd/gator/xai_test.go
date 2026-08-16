package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestXAIDeviceLoginStoresRefreshableOAuthCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		switch request.URL.Path {
		case "/oauth2/device/code":
			if request.PostForm.Get("client_id") != "gator-client" || request.PostForm.Get("referrer") != "gator" || request.PostForm.Get("scope") != xaiOAuthScope {
				t.Fatalf("device request form = %#v", request.PostForm)
			}
			_, _ = fmt.Fprint(writer, `{"device_code":"device","user_code":"CODE","verification_uri":"https://auth.example.test/device","expires_in":600}`)
		case "/oauth2/token":
			if request.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" || request.PostForm.Get("device_code") != "device" {
				t.Fatalf("token request form = %#v", request.PostForm)
			}
			_, _ = fmt.Fprint(writer, `{"access_token":"xai-access","refresh_token":"xai-refresh","expires_in":3600}`)
		default:
			t.Fatalf("path = %q", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_XAI_OAUTH_CLIENT_ID", "gator-client")
	t.Setenv("GATOR_XAI_AUTH_URL", server.URL)
	login, err := beginXAIDeviceLogin()
	if err != nil {
		t.Fatalf("begin xAI device login: %v", err)
	}
	login.flow.WaitBeforePoll = false
	login.device.PollInterval = time.Millisecond
	if err := login.Complete(context.Background()); err != nil {
		t.Fatalf("complete xAI device login: %v", err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	credential, found, err := credentials.Read("xai")
	if err != nil || !found || credential.Access != "xai-access" || credential.Refresh != "xai-refresh" || credential.Expired(time.Now()) {
		t.Fatalf("xAI credential = %#v, found=%v, error=%v", credential, found, err)
	}
}
