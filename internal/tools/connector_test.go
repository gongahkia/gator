package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/connector"
)

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
