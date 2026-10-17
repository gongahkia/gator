package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/connector"
)

func TestGoogleBatchToolKeepsPartialEvidenceInUnknownAction(t *testing.T) {
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	var descriptor connector.Descriptor
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"documentId":"doc-1"}`))
		if err := credentials.Delete(descriptor.CredentialRef()); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	descriptor = connector.Descriptor{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthBearer}
	credential, err := auth.NewBearerToken("test-token", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	var records []action.Record
	surface, err := ConnectorTools(connector.Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client()}, []string{descriptor.ID}, ConnectorPolicy{
		Mode: action.Act, ExternalActions: action.Approve,
		Approve:  func(context.Context, action.Proposal) (action.Decision, error) { return action.Allow, nil },
		OnAction: func(record action.Record) { records = append(records, record) },
	})
	if err != nil {
		t.Fatal(err)
	}
	var result string
	for _, tool := range surface {
		if tool.Definition().Name == "connector_google_batch" {
			result = executeTool(t, tool, `{"payload":{"actions":[{"operation":"docs_create","payload":{"title":"First"}},{"operation":"docs_create","payload":{"title":"Second"}}]}}`)
			break
		}
	}
	if requests != 1 || len(records) != 1 || records[0].Status != action.Unknown || len(records[0].BatchSteps) != 2 || records[0].BatchSteps[0].Status != action.Executed || records[0].BatchSteps[1].Status != action.Failed || records[0].Validate() != nil || !strings.Contains(result, `"batch_steps"`) || !strings.Contains(result, `"status":"unknown"`) {
		t.Fatalf("requests=%d records=%#v result=%s", requests, records, result)
	}
}

func TestGoogleDocumentActionRetainsProviderEvidenceAfterApproval(t *testing.T) {
	response := `{"documentId":"doc-1","replies":[]}`
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/v1/documents/doc-1:batchUpdate" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(response))
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	var records []action.Record
	surface, err := ConnectorTools(connector.Runtime{Registry: registry, HTTPClient: server.Client()}, []string{descriptor.ID}, ConnectorPolicy{
		Mode: action.Act, ExternalActions: action.Approve,
		Approve:  func(context.Context, action.Proposal) (action.Decision, error) { return action.Allow, nil },
		OnAction: func(record action.Record) { records = append(records, record) },
	})
	if err != nil {
		t.Fatal(err)
	}
	var result string
	for _, tool := range surface {
		if tool.Definition().Name == "connector_google_docs_update" {
			result = executeTool(t, tool, `{"payload":{"resource_id":"doc-1","requests":[]}}`)
			break
		}
	}
	digest := sha256.Sum256([]byte(response))
	if requests != 1 || len(records) != 1 || records[0].Status != action.Executed || records[0].ProviderResourceID != "doc-1" || records[0].ResponseSHA256 != hex.EncodeToString(digest[:]) || records[0].ReceiptID != "" || records[0].Validate() != nil || !strings.Contains(result, `"provider_resource_id":"doc-1"`) {
		t.Fatalf("requests=%d records=%#v result=%s", requests, records, result)
	}
}

func TestConnectorToolsExposeOnlySelectedAttributedSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"metric":7}`))
	}))
	defer server.Close()
	selected := connector.Descriptor{Version: connector.DescriptorVersion, ID: "metrics", Name: "Metrics", Kind: connector.KindHTTPJSON, Resource: server.URL, Authentication: connector.AuthNone}
	other := connector.Descriptor{Version: connector.DescriptorVersion, ID: "other", Name: "Other", Kind: connector.KindHTTPJSON, Resource: server.URL + "/other", Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{selected, other})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var sources []connector.Provenance
	surface, err := ConnectorTools(connector.Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client()}, []string{"metrics"}, ConnectorPolicy{
		Mode: action.Draft, ExternalActions: action.Forbid,
		OnSource: func(source connector.Provenance) { sources = append(sources, source) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(surface) != 1 || surface[0].Definition().Name != "connector_metrics_fetch" || !strings.Contains(surface[0].Definition().Description, "untrusted") {
		t.Fatalf("connector surface = %#v", surface)
	}
	result := executeTool(t, surface[0], `{}`)
	if !strings.Contains(result, `"metric":7`) || !strings.Contains(result, `"connector_id":"metrics"`) || len(sources) != 1 {
		t.Fatalf("tool result = %s, sources=%#v", result, sources)
	}
}

func TestConnectorToolsRejectUnknownOrDuplicateSelection(t *testing.T) {
	registry, err := connector.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	runtime := connector.Runtime{Registry: registry}
	if _, err := ConnectorTools(runtime, []string{"missing"}, ConnectorPolicy{Mode: action.Inspect, ExternalActions: action.Forbid}); err == nil {
		t.Fatal("unknown connector was selected")
	}
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "source", Name: "Source", Kind: connector.KindHTTPJSON, Resource: "https://example.com/data", Authentication: connector.AuthNone}
	registry, err = connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConnectorTools(connector.Runtime{Registry: registry}, []string{"source", "source"}, ConnectorPolicy{Mode: action.Inspect, ExternalActions: action.Forbid}); err == nil {
		t.Fatal("duplicate connector selection was accepted")
	}
}

func TestConnectorToolPropagatesContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "source", Name: "Source", Kind: connector.KindHTTPJSON, Resource: server.URL, Authentication: connector.AuthNone}
	registry, _ := connector.NewRegistry([]connector.Descriptor{descriptor})
	surface, err := ConnectorTools(connector.Runtime{Registry: registry, HTTPClient: server.Client()}, []string{"source"}, ConnectorPolicy{Mode: action.Inspect, ExternalActions: action.Forbid})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := surface[0].Execute(ctx, []byte(`{}`)); err == nil {
		t.Fatal("canceled connector call succeeded")
	}
}

func TestConnectorToolDraftsWebhookWithoutSending(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "release", Name: "Release", Kind: connector.KindHTTPWebhook, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	var records []action.Record
	surface, err := ConnectorTools(connector.Runtime{Registry: registry, HTTPClient: server.Client()}, []string{descriptor.ID}, ConnectorPolicy{
		Mode: action.Draft, ExternalActions: action.Propose,
		Approve: func(context.Context, action.Proposal) (action.Decision, error) {
			t.Fatal("draft requested approval")
			return action.Allow, nil
		},
		OnAction: func(record action.Record) { records = append(records, record) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(surface) != 1 || surface[0].Definition().Name != "connector_release_publish" {
		t.Fatalf("action surface = %#v", surface)
	}
	result := executeTool(t, surface[0], `{"payload":{"version":"v1"}}`)
	if requests.Load() != 0 || len(records) != 1 || records[0].Status != action.Pending || !strings.Contains(result, `"status":"pending"`) {
		t.Fatalf("requests=%d records=%#v result=%s", requests.Load(), records, result)
	}
}

func TestConnectorToolExecutesWebhookOnlyAfterFreshApproval(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "release", Name: "Release", Kind: connector.KindHTTPWebhook, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	approvals := 0
	var records []action.Record
	surface, err := ConnectorTools(connector.Runtime{Registry: registry, HTTPClient: server.Client()}, []string{descriptor.ID}, ConnectorPolicy{
		Mode: action.Act, ExternalActions: action.Approve,
		Approve: func(_ context.Context, proposal action.Proposal) (action.Decision, error) {
			approvals++
			if proposal.Target != server.URL || proposal.Preview != `{"version":"v1"}` {
				t.Fatalf("approval proposal = %#v", proposal)
			}
			return action.Allow, nil
		},
		OnAction: func(record action.Record) { records = append(records, record) },
	})
	if err != nil {
		t.Fatal(err)
	}
	result := executeTool(t, surface[0], `{"payload":{"version":"v1"}}`)
	if approvals != 1 || requests.Load() != 1 || len(records) != 1 || records[0].Status != action.Executed || !strings.Contains(result, `"status":"executed"`) {
		t.Fatalf("approvals=%d requests=%d records=%#v result=%s", approvals, requests.Load(), records, result)
	}
}

func TestGmailSendStaysDraftOrDeniedUntilFreshApproval(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"sent-1"}`))
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := connector.Runtime{Registry: registry, HTTPClient: server.Client()}
	arguments := `{"payload":{"from":"me@example.com","to":"venue@example.com","subject":"Venue update","body":"Can we use the new date?"}}`
	for _, scenario := range []struct {
		mode        action.Mode
		disposition action.Disposition
		decision    action.Decision
		status      string
		requests    int32
	}{
		{action.Draft, action.Propose, "", `"status":"pending"`, 0},
		{action.Act, action.Approve, action.Deny, `"status":"denied"`, 0},
		{action.Act, action.Approve, action.Allow, `"status":"executed"`, 1},
	} {
		surface, err := ConnectorTools(runtime, []string{descriptor.ID}, ConnectorPolicy{
			Mode: scenario.mode, ExternalActions: scenario.disposition,
			Approve: func(_ context.Context, proposal action.Proposal) (action.Decision, error) {
				if proposal.Target != server.URL+"/gmail/v1/users/me/messages/send" || !strings.Contains(proposal.Preview, "venue@example.com") {
					t.Fatalf("approval proposal = %#v", proposal)
				}
				return scenario.decision, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, tool := range surface {
			if tool.Definition().Name != "connector_google_gmail_send" {
				continue
			}
			found = true
			result := executeTool(t, tool, arguments)
			if !strings.Contains(result, scenario.status) || requests.Load() != scenario.requests {
				t.Fatalf("Gmail send result=%s, requests=%d", result, requests.Load())
			}
			if scenario.decision == action.Allow && !strings.Contains(result, `"receipt_id":"sent-1"`) {
				t.Fatalf("approved Gmail send lost provider receipt: %s", result)
			}
		}
		if !found {
			t.Fatal("Gmail send tool was not exposed")
		}
	}
	forbidden, err := ConnectorTools(runtime, []string{descriptor.ID}, ConnectorPolicy{Mode: action.Inspect, ExternalActions: action.Forbid})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range forbidden {
		if tool.Definition().Name == "connector_google_gmail_send" {
			t.Fatal("unattended inspect mode exposed Gmail send")
		}
	}
}

func TestConnectorToolsHideForbiddenActions(t *testing.T) {
	t.Parallel()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "release", Name: "Release", Kind: connector.KindHTTPWebhook, Resource: "https://example.com/hook", Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := ConnectorTools(connector.Runtime{Registry: registry}, []string{descriptor.ID}, ConnectorPolicy{Mode: action.Act, ExternalActions: action.Forbid})
	if err != nil || len(surface) != 0 {
		t.Fatalf("forbidden action surface = %#v, err = %v", surface, err)
	}
}

func TestConnectorReadAskRequiresAndUsesFreshApproval(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "source", Name: "Source", Kind: connector.KindHTTPJSON, Resource: server.URL, Authentication: connector.AuthNone}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	permissions := connector.PermissionSet{{ConnectorID: descriptor.ID, Operation: "fetch", Read: connector.PermissionAsk}}
	withoutApproval, err := ConnectorTools(connector.Runtime{Registry: registry}, []string{descriptor.ID}, ConnectorPolicy{Mode: action.Inspect, ExternalActions: action.Forbid, Permissions: permissions})
	if err != nil || len(withoutApproval) != 0 {
		t.Fatalf("asked read without approver = %#v, %v", withoutApproval, err)
	}
	approvals := 0
	withApproval, err := ConnectorTools(connector.Runtime{Registry: registry, HTTPClient: server.Client()}, []string{descriptor.ID}, ConnectorPolicy{
		Mode: action.Inspect, ExternalActions: action.Forbid, Permissions: permissions,
		ApproveRead: func(_ context.Context, connectorID, operation string, arguments json.RawMessage) (bool, error) {
			approvals++
			return connectorID == descriptor.ID && operation == "fetch" && string(arguments) == `{}`, nil
		},
	})
	if err != nil || len(withApproval) != 1 {
		t.Fatalf("asked read with approver = %#v, %v", withApproval, err)
	}
	if result := executeTool(t, withApproval[0], `{}`); approvals != 1 || !strings.Contains(result, `"ok":true`) {
		t.Fatalf("result = %s, approvals = %d", result, approvals)
	}
}
