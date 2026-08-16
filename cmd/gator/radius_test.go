package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRadiusDeviceLoginStoresRefreshableCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		switch request.URL.Path {
		case "/v1/oauth/device":
			if request.PostForm.Get("client_id") != "gator-client" || request.PostForm.Get("scope") != radiusOAuthScope {
				t.Fatalf("device request form = %#v", request.PostForm)
			}
			_, _ = fmt.Fprint(writer, `{"device_code":"device","user_code":"CODE","verification_uri":"https://radius.example.test/code","expires_in":600}`)
		case "/v1/oauth/token":
			if request.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" || request.PostForm.Get("device_code") != "device" || request.PostForm.Get("client_id") != "gator-client" {
				t.Fatalf("token request form = %#v", request.PostForm)
			}
			_, _ = fmt.Fprint(writer, `{"access_token":"radius-access","refresh_token":"radius-refresh","expires_in":3600}`)
		default:
			t.Fatalf("path = %q", request.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_RADIUS_OAUTH_CLIENT_ID", "gator-client")
	t.Setenv("GATOR_RADIUS_GATEWAY", server.URL)
	login, err := beginRadiusDeviceLogin()
	if err != nil {
		t.Fatalf("begin Radius device login: %v", err)
	}
	login.flow.WaitBeforePoll = false
	login.device.PollInterval = time.Millisecond
	if err := login.Complete(context.Background()); err != nil {
		t.Fatalf("complete Radius device login: %v", err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	credential, found, err := credentials.Read("radius")
	if err != nil || !found || credential.Access != "radius-access" || credential.Refresh != "radius-refresh" || credential.Expired(time.Now()) {
		t.Fatalf("Radius credential = %#v, found=%v, error=%v", credential, found, err)
	}
}
