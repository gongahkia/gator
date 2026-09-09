package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gongahkia/gator/internal/action"
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
