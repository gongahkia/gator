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
	"github.com/gongahkia/gator/internal/googlework"
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

func TestGoogleAgendaAndTaskListExposeManualFrontendFilters(t *testing.T) {
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: "https://www.googleapis.com", Authentication: AuthNone}
	includeCompleted := false
	includeHidden := false
	includeDeleted := true
	endpoint, _, _, err := serviceRead(descriptor, "tasks_list", serviceInput{ResourceID: "list", IncludeCompleted: &includeCompleted, IncludeHidden: &includeHidden, IncludeDeleted: &includeDeleted})
	if err != nil || !strings.Contains(endpoint, "showCompleted=false") || !strings.Contains(endpoint, "showHidden=false") || !strings.Contains(endpoint, "showDeleted=true") {
		t.Fatalf("task list endpoint=%q err=%v", endpoint, err)
	}
	singleEvents := true
	endpoint, _, _, err = serviceRead(descriptor, "events_list", serviceInput{ResourceID: "calendar", TimeMin: "2026-09-18T00:00:00Z", TimeMax: "2026-09-19T00:00:00Z", SingleEvents: &singleEvents})
	if err != nil || !strings.Contains(endpoint, "timeMin=2026-09-18T00%3A00%3A00Z") || !strings.Contains(endpoint, "singleEvents=true") {
		t.Fatalf("agenda endpoint=%q err=%v", endpoint, err)
	}
	endpoint, _, _, err = serviceRead(descriptor, "calendar_colors", serviceInput{})
	if err != nil || endpoint != "https://www.googleapis.com/calendar/v3/colors" {
		t.Fatalf("calendar colors endpoint=%q err=%v", endpoint, err)
	}
	endpoint, _, _, err = serviceRead(descriptor, "drive_get", serviceInput{ResourceID: "file", Fields: "id,name,owners"})
	if err != nil || !strings.Contains(endpoint, "fields=id%2Cname%2Cowners") {
		t.Fatalf("Drive metadata endpoint=%q err=%v", endpoint, err)
	}
}

func TestGoogleEventResponsePinsRSVPOptionsAndETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPatch || request.URL.Path != "/calendar/v3/calendars/calendar/events/event" || request.URL.Query().Get("sendUpdates") != "all" || request.Header.Get("If-Match") != "etag-value" {
			t.Fatalf("request=%s %s?%s if-match=%q", request.Method, request.URL.Path, request.URL.RawQuery, request.Header.Get("If-Match"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		attendees, ok := body["attendees"].([]any)
		if !ok || len(attendees) != 1 {
			t.Fatalf("RSVP body=%#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"event","summary":"Planning"}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
	prepared, err := runtime.PrepareAction(descriptor.ID, "events_respond", json.RawMessage(`{"payload":{"resource_id":"calendar/event","response_status":"accepted","comment":"Count me in","send_updates":"all","etag":"etag-value"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ExecutePrepared(context.Background(), action.Act, prepared); err != nil {
		t.Fatal(err)
	}
}

func TestGoogleCalendarSubscriptionAndEventMoveUsePinnedEndpoints(t *testing.T) {
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: "https://www.googleapis.com", Authentication: AuthNone}
	target, body, err := serviceAction(descriptor, "calendars_subscribe", []byte(`{"resource_id":"team@example.test"}`))
	if err != nil || target != "https://www.googleapis.com/calendar/v3/users/me/calendarList" || string(body) != `{"id":"team@example.test"}` {
		t.Fatalf("calendar subscription target=%q body=%s err=%v", target, body, err)
	}
	target, body, err = serviceAction(descriptor, "events_move", []byte(`{"resource_id":"source/event","destination_calendar":"destination"}`))
	if err != nil || target != "https://www.googleapis.com/calendar/v3/calendars/source/events/event/move?destination=destination" || string(body) != `{}` {
		t.Fatalf("event move target=%q body=%s err=%v", target, body, err)
	}
}

func TestGoogleLocalSearchRequiresLiveConnectionBeforeReturningMirrorData(t *testing.T) {
	available := true
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/drive/v3/files":
			_, _ = writer.Write([]byte(`{"files":[{"id":"file-1","name":"Roadmap"}]}`))
		case "/oauth2/v3/userinfo":
			if !available {
				writer.WriteHeader(http.StatusServiceUnavailable)
				_, _ = writer.Write([]byte(`{"error":"offline"}`))
				return
			}
			_, _ = writer.Write([]byte(`{"email":"person@example.test"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client(), StateDir: t.TempDir()}
	if _, err := runtime.Invoke(context.Background(), action.Inspect, descriptor.ID, "drive_search", json.RawMessage(`{"query":"roadmap"}`)); err != nil {
		t.Fatal(err)
	}
	available = false
	if _, err := runtime.Invoke(context.Background(), action.Inspect, descriptor.ID, "local_search", json.RawMessage(`{"query":"roadmap","limit":10}`)); err == nil {
		t.Fatal("local mirror result was exposed after live health failed")
	}
	available = true
	result, err := runtime.Invoke(context.Background(), action.Inspect, descriptor.ID, "local_search", json.RawMessage(`{"query":"roadmap","limit":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result.Data, []byte(`"file-1"`)) || result.Provenance.Operation != "local_search" {
		t.Fatalf("local search result = %s, provenance=%#v", result.Data, result.Provenance)
	}
}

func TestGoogleBatchPinsCanonicalPayloadAndDoesNotSendDuringPreparation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/tasks/v1/lists/list/tasks" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"task-1","title":"Prepare release"}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client(), StateDir: t.TempDir()}
	prepared, err := runtime.PrepareAction(descriptor.ID, "batch", json.RawMessage(`{"payload":{"actions":[{"payload":{"title":"Prepare release","resource_id":"list"},"operation":"tasks_create"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if requests != 0 || !bytes.Contains([]byte(prepared.Proposal.Preview), []byte(`"operation":"tasks_create"`)) {
		t.Fatalf("prepared request count=%d proposal=%#v", requests, prepared.Proposal)
	}
	if err := runtime.ExecutePrepared(context.Background(), action.Act, prepared); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestPreparedGoogleActionRejectsAChangedApprovalEnvelope(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"task-1"}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := (Runtime{Registry: registry, HTTPClient: server.Client()}).PrepareAction(descriptor.ID, "tasks_create", json.RawMessage(`{"payload":{"resource_id":"list","title":"Prepare release"}}`))
	if err != nil {
		t.Fatal(err)
	}
	prepared.Proposal.Target = server.URL + "/different"
	if err := (Runtime{Registry: registry, HTTPClient: server.Client()}).ExecutePrepared(context.Background(), action.Act, prepared); err == nil || requests != 0 {
		t.Fatalf("tampered action error=%v requests=%d", err, requests)
	}
}

func TestGoogleQuickCaptureDoesNotCreateAThing(t *testing.T) {
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: "https://www.googleapis.com", Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Runtime{Registry: registry, Now: func() time.Time { return time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC) }}).Invoke(context.Background(), action.Inspect, descriptor.ID, "quick_capture", json.RawMessage(`{"text":"meeting Roadmap tomorrow at 9am"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result.Data, []byte(`"kind":"event"`)) || result.Provenance.Operation != "quick_capture" {
		t.Fatalf("result = %s, provenance=%#v", result.Data, result.Provenance)
	}
}

func TestGoogleTaskMetadataBuildsNotesBeforeAnyTaskWrite(t *testing.T) {
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: "https://www.googleapis.com", Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Runtime{Registry: registry}).Invoke(context.Background(), action.Inspect, descriptor.ID, "task_metadata_encode", json.RawMessage(`{"notes":"Launch plan","priority":"high","recurrence_rrule":"RRULE:FREQ=WEEKLY;INTERVAL=1","reminder_time":"09:00","reminder_zone":"Asia/Singapore"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result.Data, []byte(`"notes":"Launch plan\n\n[GATOR-TASK v1]`)) {
		t.Fatalf("metadata result = %s", result.Data)
	}
}

func TestGoogleRemindersAreLiveGatedAndDeliveredOnce(t *testing.T) {
	available := true
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/oauth2/v3/userinfo" {
			http.NotFound(writer, request)
			return
		}
		if !available {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte(`{"error":"offline"}`))
			return
		}
		_, _ = writer.Write([]byte(`{"email":"person@example.test"}`))
	}))
	defer server.Close()
	stateDir := t.TempDir()
	store, err := googlework.Open(stateDir, "google")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	if err := store.Mirror(context.Background(), "events_list", json.RawMessage(`{"items":[{"id":"event-1","summary":"Planning","start":{"dateTime":"2026-09-18T10:05:00Z"},"reminders":{"overrides":[{"method":"popup","minutes":5}]}}]}`), now); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client(), StateDir: stateDir, Now: func() time.Time { return now }}
	result, err := runtime.Invoke(context.Background(), action.Inspect, descriptor.ID, "reminders_due", json.RawMessage(`{"window_minutes":1}`))
	if err != nil || !bytes.Contains(result.Data, []byte(`"event-1"`)) {
		t.Fatalf("first reminder result=%s err=%v", result.Data, err)
	}
	result, err = runtime.Invoke(context.Background(), action.Inspect, descriptor.ID, "reminders_due", json.RawMessage(`{"window_minutes":1}`))
	if err != nil || bytes.Contains(result.Data, []byte(`"event-1"`)) {
		t.Fatalf("duplicate reminder result=%s err=%v", result.Data, err)
	}
	available = false
	if _, err := runtime.Invoke(context.Background(), action.Inspect, descriptor.ID, "reminders_due", json.RawMessage(`{"window_minutes":1}`)); err == nil {
		t.Fatal("cached reminder was exposed while the live Google health check failed")
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
