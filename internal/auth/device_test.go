package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDeviceFlowStartsAndPollsForAnApprovedToken(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		switch request.URL.Path {
		case "/device":
			if request.PostForm.Get("client_id") != "gator-client" || request.PostForm.Get("scope") != "read:user" {
				t.Fatalf("device form = %#v", request.PostForm)
			}
			_, _ = fmt.Fprint(writer, `{"device_code":"device","user_code":"ABCD-EFGH","verification_uri":"https://example.test/device","interval":1,"expires_in":60}`)
		case "/token":
			polls++
			if request.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" || request.PostForm.Get("device_code") != "device" {
				t.Fatalf("token form = %#v", request.PostForm)
			}
			if polls == 1 {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(writer, `{"error":"authorization_pending"}`)
				return
			}
			_, _ = fmt.Fprint(writer, `{"access_token":"github-token"}`)
		default:
			t.Fatalf("path = %q", request.URL.Path)
		}
	}))
	defer server.Close()
	flow := DeviceFlow{ClientID: "gator-client", DeviceCodeURL: server.URL + "/device", TokenURL: server.URL + "/token", Scope: "read:user", WaitBeforePoll: false}
	authorization, err := flow.Start(context.Background())
	if err != nil {
		t.Fatalf("start device flow: %v", err)
	}
	authorization.PollInterval = time.Millisecond
	token, err := flow.Poll(context.Background(), authorization)
	if err != nil {
		t.Fatalf("poll device flow: %v", err)
	}
	if token != "github-token" || polls != 2 {
		t.Fatalf("token = %q, polls = %d", token, polls)
	}
}

func TestDeviceFlowRejectsUnsafeVerificationURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(writer, `{"device_code":"device","user_code":"CODE","verification_uri":"file:///tmp/unsafe","expires_in":60}`)
	}))
	defer server.Close()
	if _, err := (DeviceFlow{ClientID: "client", DeviceCodeURL: server.URL, TokenURL: server.URL}).Start(context.Background()); err == nil {
		t.Fatal("unsafe verification URL was accepted")
	}
}

func TestDeviceFlowPrefersCompleteVerificationURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(writer, `{"device_code":"device","user_code":"CODE","verification_uri":"https://example.test/device","verification_uri_complete":"https://example.test/device?user_code=CODE","expires_in":60}`)
	}))
	defer server.Close()
	authorization, err := (DeviceFlow{ClientID: "client", DeviceCodeURL: server.URL, TokenURL: server.URL}).Start(context.Background())
	if err != nil {
		t.Fatalf("start device flow: %v", err)
	}
	if authorization.VerificationURL != "https://example.test/device?user_code=CODE" {
		t.Fatalf("verification URL = %q", authorization.VerificationURL)
	}
}
