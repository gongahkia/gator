package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestGoogleDriveOperationsUsePinnedEndpoints(t *testing.T) {
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: "https://www.googleapis.com", Authentication: AuthNone}
	endpoint, _, _, err := serviceRead(descriptor, "drive_get", serviceInput{ResourceID: "file", Fields: "id,name,owners"})
	if err != nil || !strings.Contains(endpoint, "fields=id%2Cname%2Cowners") {
		t.Fatalf("Drive metadata endpoint=%q err=%v", endpoint, err)
	}
}

func TestGoogleDailyContextReadsUsePinnedHostsAndBoundedWindows(t *testing.T) {
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: "https://www.googleapis.com", Authentication: AuthNone}
	calendar, method, _, err := serviceRead(descriptor, "calendar_events", serviceInput{TimeMin: "2026-10-16T00:00:00+08:00", TimeMax: "2026-10-17T00:00:00+08:00", Limit: 20})
	if err != nil || method != "GET" || !strings.HasPrefix(calendar, "https://www.googleapis.com/calendar/v3/calendars/primary/events?") || !strings.Contains(calendar, "singleEvents=true") {
		t.Fatalf("calendar endpoint = %q, %q, %v", calendar, method, err)
	}
	if _, _, _, err := serviceRead(descriptor, "calendar_events", serviceInput{TimeMin: "2026-10-17T00:00:00Z", TimeMax: "2026-10-16T00:00:00Z"}); err == nil {
		t.Fatal("reversed calendar window accepted")
	}
	mail, _, _, err := serviceRead(descriptor, "gmail_list", serviceInput{Query: "is:unread", Limit: 10})
	if err != nil || !strings.HasPrefix(mail, "https://gmail.googleapis.com/gmail/v1/users/me/messages?") || !strings.Contains(mail, "maxResults=10") {
		t.Fatalf("Gmail list endpoint = %q, %v", mail, err)
	}
	message, _, _, err := serviceRead(descriptor, "gmail_get", serviceInput{ResourceID: "abc123"})
	if err != nil || message != "https://gmail.googleapis.com/gmail/v1/users/me/messages/abc123?format=full" {
		t.Fatalf("Gmail message endpoint = %q, %v", message, err)
	}
	if _, _, _, err := serviceRead(descriptor, "gmail_get", serviceInput{}); err == nil {
		t.Fatal("empty Gmail message ID accepted")
	}
}

func TestGoogleDailyReadsRetainEndpointProvenance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/calendar/v3/calendars/primary/events":
			if request.URL.Query().Get("timeMin") == "" || request.URL.Query().Get("singleEvents") != "true" {
				t.Errorf("calendar query = %v", request.URL.Query())
			}
			_, _ = writer.Write([]byte(`{"items":[{"id":"meeting-1","summary":"Review"}]}`))
		case "/gmail/v1/users/me/messages":
			_, _ = writer.Write([]byte(`{"messages":[{"id":"mail-1"}]}`))
		case "/gmail/v1/users/me/messages/mail-1":
			_, _ = writer.Write([]byte(`{"id":"mail-1","snippet":"Decision needed","payload":{"headers":[{"name":"Subject","value":"Venue"}]}}`))
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
	for _, input := range []struct{ operation, args, marker string }{
		{"calendar_events", `{"time_min":"2026-10-16T00:00:00+08:00","time_max":"2026-10-17T00:00:00+08:00"}`, "meeting-1"},
		{"gmail_list", `{"query":"is:unread"}`, "mail-1"},
		{"gmail_get", `{"resource_id":"mail-1"}`, "Decision needed"},
	} {
		result, err := runtime.Invoke(context.Background(), action.Inspect, descriptor.ID, input.operation, json.RawMessage(input.args))
		if err != nil || !bytes.Contains(result.Data, []byte(input.marker)) || result.Provenance.Operation != input.operation || result.Provenance.SHA256 == "" {
			t.Fatalf("%s = %#v, %v", input.operation, result, err)
		}
	}
}

func TestGoogleBatchPinsCanonicalPayloadAndDoesNotSendDuringPreparation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/v1/documents" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"documentId":"doc-1","title":"Prepare release"}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
	prepared, err := runtime.PrepareAction(descriptor.ID, "batch", json.RawMessage(`{"payload":{"actions":[{"payload":{"title":"Prepare release"},"operation":"docs_create"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if requests != 0 || !bytes.Contains([]byte(prepared.Proposal.Preview), []byte(`"operation":"docs_create"`)) {
		t.Fatalf("prepared request count=%d proposal=%#v", requests, prepared.Proposal)
	}
	if err := runtime.ExecutePrepared(context.Background(), action.Act, prepared); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestGoogleBatchRejectsGmailSendBeforeApprovalOrNetwork(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	input := json.RawMessage(`{"payload":{"actions":[{"operation":"gmail_send","payload":{"from":"me@example.com","to":"you@example.com","subject":"Hello","body":"Hi"}}]}}`)
	_, err = (Runtime{Registry: registry, HTTPClient: server.Client()}).PrepareAction(descriptor.ID, "batch", input)
	if err == nil || !strings.Contains(err.Error(), "not an allowed Docs or Sheets mutation") || requests != 0 {
		t.Fatalf("Gmail batch preparation = %v, requests=%d", err, requests)
	}
}

func TestPreparedGoogleActionRejectsAChangedApprovalEnvelope(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"documentId":"doc-1"}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := (Runtime{Registry: registry, HTTPClient: server.Client()}).PrepareAction(descriptor.ID, "docs_create", json.RawMessage(`{"payload":{"title":"Prepare release"}}`))
	if err != nil {
		t.Fatal(err)
	}
	prepared.Proposal.Target = server.URL + "/different"
	if err := (Runtime{Registry: registry, HTTPClient: server.Client()}).ExecutePrepared(context.Background(), action.Act, prepared); err == nil || requests != 0 {
		t.Fatalf("tampered action error=%v requests=%d", err, requests)
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
