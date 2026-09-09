package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
)

func TestSlackAdapterUsesPinnedServiceEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/auth.test" {
			t.Errorf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: 1, ID: "slack-team", Name: "Slack", Kind: KindSlack, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Runtime{Registry: registry, HTTPClient: server.Client()}).Invoke(context.Background(), action.Inspect, descriptor.ID, "whoami", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Provenance.Operation != "whoami" || result.Provenance.Resource != server.URL+"/auth.test" {
		t.Fatalf("provenance = %#v", result.Provenance)
	}
}

func TestGoogleAdapterUsesServiceSpecificHosts(t *testing.T) {
	descriptor := Descriptor{Version: 1, ID: "google", Name: "Google", Kind: KindGoogle, Resource: "https://www.googleapis.com", Authentication: AuthBearer}
	endpoint, _, _, err := serviceRead(descriptor, "docs_get", serviceInput{ResourceID: "doc"})
	if err != nil || endpoint != "https://docs.googleapis.com/v1/documents/doc" {
		t.Fatalf("Docs endpoint = %q, %v", endpoint, err)
	}
	endpoint, _, _, err = serviceRead(descriptor, "sheets_get", serviceInput{ResourceID: "sheet"})
	if err != nil || endpoint != "https://sheets.googleapis.com/v4/spreadsheets/sheet" {
		t.Fatalf("Sheets endpoint = %q, %v", endpoint, err)
	}
}

func TestSlackHTTP200ErrorIsNotTreatedAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":false,"error":"not_authed"}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: 1, ID: "slack-team", Name: "Slack", Kind: KindSlack, Resource: server.URL, Authentication: AuthNone}
	registry, _ := NewRegistry([]Descriptor{descriptor})
	if _, err := (Runtime{Registry: registry, HTTPClient: server.Client()}).Invoke(context.Background(), action.Inspect, descriptor.ID, "whoami", json.RawMessage(`{}`)); err == nil {
		t.Fatal("Slack ok=false response was accepted")
	}
}

func TestNotionSearchOmitsUnsetPagination(t *testing.T) {
	descriptor := Descriptor{Version: 1, ID: "notion", Name: "Notion", Kind: KindNotion, Resource: "https://api.notion.com/v1", Authentication: AuthBearer}
	_, method, body, err := serviceRead(descriptor, "search", serviceInput{})
	if err != nil || method != http.MethodPost || !bytes.Equal(body, []byte(`{}`)) {
		t.Fatalf("Notion search = %s %s, %v", method, body, err)
	}
}

func TestOAuthConnectorRefreshesResourceBoundCredential(t *testing.T) {
	var sawAccess string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/token":
			_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "fresh", "refresh_token": "next", "expires_in": 3600, "token_type": "Bearer"})
		case "/auth.test":
			sawAccess = request.Header.Get("Authorization")
			_, _ = writer.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	descriptor := Descriptor{
		Version: 1, ID: "slack-oauth", Name: "Slack", Kind: KindSlack, Resource: server.URL, Authentication: AuthOAuth,
		OAuthClientID: "public-client", OAuthAuthorizeURL: server.URL + "/authorize", OAuthTokenURL: server.URL + "/token", OAuthRedirectURL: "http://127.0.0.1:1457/oauth/callback", OAuthScopes: "search:read",
	}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(descriptor.CredentialRef(), auth.Credential{Type: "oauth", Access: "expired", Refresh: "refresh", Expires: time.Now().Add(-time.Minute).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client()}).Invoke(context.Background(), action.Inspect, descriptor.ID, "whoami", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if sawAccess != "Bearer fresh" {
		t.Fatalf("authorization = %q", sawAccess)
	}
}

func TestPermissionSetOnlyNarrowsConnectorAuthority(t *testing.T) {
	permissions := PermissionSet{{ConnectorID: "slack-team", Write: PermissionDraft}, {ConnectorID: "slack-team", Operation: "post_message", Write: PermissionAsk}, {ConnectorID: "slack-team", Operation: "search", Read: PermissionDeny}}
	if got := permissions.Resolve("slack-team", "post_message", action.Publish); got != PermissionDraft {
		t.Fatalf("write permission = %q", got)
	}
	if got := permissions.Resolve("slack-team", "search", action.ConnectedRead); got != PermissionDeny {
		t.Fatalf("read permission = %q", got)
	}
}

func TestRemoteMCPRequiresTypedMapping(t *testing.T) {
	descriptor := Descriptor{Version: 1, ID: "linear", Name: "Linear", Kind: KindRemoteMCP, Resource: "https://mcp.example.com", Authentication: AuthBearer, SearchTool: "search_issues", ReadTool: "get_issue", ActionTool: "create_issue"}
	if err := descriptor.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(descriptor.Operations()) != 3 {
		t.Fatalf("operations = %#v", descriptor.Operations())
	}
}
