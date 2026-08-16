package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestKimiCodingDeviceLoginStoresRefreshableCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		switch request.URL.Path {
		case "/api/oauth/device_authorization":
			if request.PostForm.Get("client_id") != "gator-client" {
				t.Fatalf("device request form = %#v", request.PostForm)
			}
			_, _ = fmt.Fprint(writer, `{"device_code":"device","user_code":"CODE","verification_uri":"https://auth.example.test/code","verification_uri_complete":"https://auth.example.test/code?user_code=CODE","expires_in":600}`)
		case "/api/oauth/token":
			if request.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" || request.PostForm.Get("device_code") != "device" || request.PostForm.Get("client_id") != "gator-client" {
				t.Fatalf("token request form = %#v", request.PostForm)
			}
			_, _ = fmt.Fprint(writer, `{"access_token":"kimi-access","refresh_token":"kimi-refresh","expires_in":3600}`)
		default:
			t.Fatalf("path = %q", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_KIMI_CODE_OAUTH_CLIENT_ID", "gator-client")
	t.Setenv("GATOR_KIMI_CODE_OAUTH_URL", server.URL)
	login, err := beginKimiCodingDeviceLogin()
	if err != nil {
		t.Fatalf("begin Kimi Code device login: %v", err)
	}
	if got := login.URL(); got != "https://auth.example.test/code?user_code=CODE\nCode: CODE" {
		t.Fatalf("device login URL = %q", got)
	}
	login.flow.WaitBeforePoll = false
	login.device.PollInterval = time.Millisecond
	if err := login.Complete(context.Background()); err != nil {
		t.Fatalf("complete Kimi Code device login: %v", err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	credential, found, err := credentials.Read("kimi-coding")
	if err != nil || !found || credential.Access != "kimi-access" || credential.Refresh != "kimi-refresh" || credential.Expired(time.Now()) {
		t.Fatalf("Kimi Code credential = %#v, found=%v, error=%v", credential, found, err)
	}
}
