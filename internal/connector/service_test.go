package connector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
)

func TestGoogleActionEvidenceNamesAffectedResource(t *testing.T) {
	for _, test := range []struct {
		operation string
		payload   string
		path      string
		method    string
		response  string
		resource  string
	}{
		{"docs_create", `{"title":"Brief"}`, "/v1/documents", http.MethodPost, `{"documentId":"doc-1"}`, "doc-1"},
		{"docs_update", `{"resource_id":"doc-1","requests":[]}`, "/v1/documents/doc-1:batchUpdate", http.MethodPost, `{"documentId":"doc-1","replies":[]}`, "doc-1"},
		{"sheets_create", `{"properties":{"title":"Tracker"}}`, "/v4/spreadsheets", http.MethodPost, `{"spreadsheetId":"sheet-1"}`, "sheet-1"},
		{"sheets_values_update", `{"resource_id":"sheet-1/Sheet1!A1","values":[["done"]]}`, "/v4/spreadsheets/sheet-1/values/Sheet1!A1", http.MethodPut, `{"spreadsheetId":"sheet-1","updatedRange":"Sheet1!A1","updatedCells":1}`, "sheet-1"},
		{"sheets_batch_update", `{"resource_id":"sheet-1","requests":[]}`, "/v4/spreadsheets/sheet-1:batchUpdate", http.MethodPost, `{"spreadsheetId":"sheet-1","replies":[]}`, "sheet-1"},
	} {
		t.Run(test.operation, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls++
				if request.URL.Path != test.path || request.Method != test.method {
					t.Errorf("request = %s %s, want %s %s", request.Method, request.URL.Path, test.method, test.path)
				}
				if test.operation == "sheets_values_update" {
					body, _ := io.ReadAll(request.Body)
					if request.URL.Query().Get("valueInputOption") != "RAW" || strings.Contains(string(body), "valueInputOption") {
						t.Errorf("Sheet update omitted required query option or sent it in the body: %s %s", request.URL, body)
					}
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.response))
			}))
			defer server.Close()
			descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
			registry, err := NewRegistry([]Descriptor{descriptor})
			if err != nil {
				t.Fatal(err)
			}
			runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
			prepared, err := runtime.PrepareAction(descriptor.ID, test.operation, actionInput([]byte(test.payload)))
			if err != nil || calls != 0 {
				t.Fatalf("prepare = %v, calls=%d", err, calls)
			}
			if test.operation == "sheets_values_update" && !strings.Contains(prepared.Proposal.Preview, `"valueInputOption":"RAW"`) {
				t.Fatalf("approval omitted value interpretation: %s", prepared.Proposal.Preview)
			}
			evidence, err := runtime.ExecutePreparedWithEvidence(context.Background(), action.Act, prepared)
			wantDigest := sha256.Sum256([]byte(test.response))
			if err != nil || calls != 1 || evidence.ProviderResourceID != test.resource || evidence.ResponseSHA256 != hex.EncodeToString(wantDigest[:]) || evidence.ReceiptID != "" {
				t.Fatalf("evidence=%#v, err=%v, calls=%d", evidence, err, calls)
			}
		})
	}
}

func TestGoogleSheetsValueOptionIsBoundToSingleAndBatchApproval(t *testing.T) {
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: "https://www.googleapis.com", Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry}
	payload := []byte(`{"resource_id":"sheet-1/Sheet1!A1","values":[["=2+2"]],"valueInputOption":"USER_ENTERED"}`)
	prepared, err := runtime.PrepareAction(descriptor.ID, "sheets_values_update", actionInput(payload))
	if err != nil || !strings.Contains(prepared.Proposal.Target, "valueInputOption=USER_ENTERED") || !strings.Contains(prepared.Proposal.Preview, `"valueInputOption":"USER_ENTERED"`) || strings.Contains(string(prepared.requestPayload), "valueInputOption") {
		t.Fatalf("single update option = %#v, %v", prepared, err)
	}
	batch, err := runtime.PrepareAction(descriptor.ID, "batch", actionInput([]byte(`{"actions":[{"operation":"sheets_values_update","payload":{"resource_id":"sheet-1/Sheet1!A1","values":[["done"]]}}]}`)))
	if err != nil || len(batch.googleSteps) != 1 || !strings.Contains(batch.googleSteps[0].target, "valueInputOption=RAW") || !strings.Contains(batch.Proposal.Preview, `"valueInputOption":"RAW"`) || strings.Contains(string(batch.googleSteps[0].payload), "valueInputOption") {
		t.Fatalf("batch update option = %#v, %v", batch, err)
	}
	for _, invalid := range []string{
		`{"resource_id":"sheet-1/Sheet1!A1","values":[["done"]],"valueInputOption":"BAD"}`,
		`{"resource_id":"sheet-1/Sheet1!A1","values":[]}`,
		`{"resource_id":"sheet-1/Sheet1!A1","values":[[]]}`,
		`{"resource_id":"sheet-1/Sheet1!A1","values":[[null]]}`,
		`{"resource_id":"sheet-1/Sheet1!A1","values":[[{}]]}`,
		`{"resource_id":"sheet-1/Sheet1!A1","values":[["done"]],"value_input_option":"RAW"}`,
		`{"resource_id":"sheet-1/Sheet1!A1","range":"Sheet1!B2","values":[["done"]]}`,
		`{"resource_id":"sheet-1/Sheet1!A1","majorDimension":"BAD","values":[["done"]]}`,
		`{"resource_id":"sheet-1/Sheet1!A1"}`,
	} {
		if _, err := runtime.PrepareAction(descriptor.ID, "sheets_values_update", actionInput([]byte(invalid))); err == nil {
			t.Fatalf("invalid Sheet update prepared: %s", invalid)
		}
	}
	if _, err := runtime.PrepareAction(descriptor.ID, "batch", actionInput([]byte(`{"actions":[{"operation":"sheets_values_update","payload":{"resource_id":"sheet-1/Sheet1!A1","values":[[null]]}}]}`))); err == nil {
		t.Fatal("batch prepared a Sheet write whose only cell would be skipped")
	}
}

func TestGoogleActionMissingOrMismatchedResourceIsUncertain(t *testing.T) {
	for _, response := range []string{`{}`, `{"documentId":"other"}`, `{"documentId":"bad/id"}`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(response))
			}))
			defer server.Close()
			descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
			registry, err := NewRegistry([]Descriptor{descriptor})
			if err != nil {
				t.Fatal(err)
			}
			runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
			prepared, err := runtime.PrepareAction(descriptor.ID, "docs_update", actionInput([]byte(`{"resource_id":"doc-1","requests":[]}`)))
			if err != nil {
				t.Fatal(err)
			}
			_, err = runtime.ExecutePreparedWithEvidence(context.Background(), action.Act, prepared)
			var uncertain interface{ OutcomeUncertain() bool }
			if err == nil || !errors.As(err, &uncertain) || !uncertain.OutcomeUncertain() {
				t.Fatalf("response %s yielded %v; want uncertain outcome", response, err)
			}
		})
	}
}

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
	evidence, err := runtime.ExecutePreparedWithEvidence(context.Background(), action.Act, prepared)
	if err != nil || requests != 1 || len(evidence.BatchSteps) != 1 || evidence.BatchSteps[0].Status != action.Executed || evidence.BatchSteps[0].ProviderResourceID != "doc-1" || evidence.BatchSteps[0].ResponseSHA256 == "" {
		t.Fatalf("batch evidence=%#v, err=%v, requests=%d", evidence, err, requests)
	}
}

func TestGoogleBatchRetainsCompletedStepsWhenLaterCredentialDisappears(t *testing.T) {
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	var descriptor Descriptor
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Authorization") != "Bearer test-token" || requests != 1 {
			t.Errorf("unexpected batch request %d", requests)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"documentId":"doc-1"}`))
		if err := credentials.Delete(descriptor.CredentialRef()); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	descriptor = Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthBearer}
	credential, err := auth.NewBearerToken("test-token", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client()}
	prepared, err := runtime.PrepareAction(descriptor.ID, "batch", json.RawMessage(`{"payload":{"actions":[{"operation":"docs_create","payload":{"title":"First"}},{"operation":"docs_create","payload":{"title":"Second"}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := runtime.ExecutePreparedWithEvidence(context.Background(), action.Act, prepared)
	var uncertain interface{ OutcomeUncertain() bool }
	if err == nil || !errors.As(err, &uncertain) || !uncertain.OutcomeUncertain() || requests != 1 || len(evidence.BatchSteps) != 2 || evidence.BatchSteps[0].Status != action.Executed || evidence.BatchSteps[0].ProviderResourceID != "doc-1" || evidence.BatchSteps[1].Status != action.Failed {
		t.Fatalf("partial batch evidence=%#v, err=%v, requests=%d", evidence, err, requests)
	}
}

func TestGoogleBatchRetainsUnknownStepAfterSuccessResponseWithoutID(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = writer.Write([]byte(`{"documentId":"doc-1"}`))
		} else {
			_, _ = writer.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
	prepared, err := runtime.PrepareAction(descriptor.ID, "batch", json.RawMessage(`{"payload":{"actions":[{"operation":"docs_create","payload":{"title":"First"}},{"operation":"docs_create","payload":{"title":"Second"}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := runtime.ExecutePreparedWithEvidence(context.Background(), action.Act, prepared)
	var uncertain interface{ OutcomeUncertain() bool }
	if err == nil || !errors.As(err, &uncertain) || !uncertain.OutcomeUncertain() || requests != 2 || len(evidence.BatchSteps) != 2 || evidence.BatchSteps[0].Status != action.Executed || evidence.BatchSteps[1].Status != action.Unknown || evidence.BatchSteps[1].ResponseSHA256 == "" {
		t.Fatalf("unknown batch evidence=%#v, err=%v, requests=%d", evidence, err, requests)
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

func TestRetiredGoogleCalendarCannotSendRequestsEvenWithLegacyGrant(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(500) }))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthOAuth, OAuthClientID: "client", OAuthAuthorizeURL: server.URL + "/authorize", OAuthTokenURL: server.URL + "/token", OAuthRedirectURL: "http://127.0.0.1:12345/oauth/callback", OAuthScopes: "https://www.googleapis.com/auth/calendar.events.readonly"}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
	_, err = runtime.Invoke(context.Background(), action.Inspect, "google", "calendar_events", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "support was removed") || requests != 0 {
		t.Fatalf("retired read: requests=%d, err=%v", requests, err)
	}
	if _, _, _, err := serviceRead(descriptor, "calendar_events", serviceInput{}); err == nil {
		t.Fatal("retired endpoint still routable")
	}
}
