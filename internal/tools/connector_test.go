package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	surface, err := ConnectorTools(connector.Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client()}, action.Draft, []string{"metrics"}, func(source connector.Provenance) {
		sources = append(sources, source)
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
	if _, err := ConnectorTools(runtime, action.Inspect, []string{"missing"}, nil); err == nil {
		t.Fatal("unknown connector was selected")
	}
	descriptor := connector.Descriptor{Version: connector.DescriptorVersion, ID: "source", Name: "Source", Kind: connector.KindHTTPJSON, Resource: "https://example.com/data", Authentication: connector.AuthNone}
	registry, err = connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConnectorTools(connector.Runtime{Registry: registry}, action.Inspect, []string{"source", "source"}, nil); err == nil {
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
	surface, err := ConnectorTools(connector.Runtime{Registry: registry, HTTPClient: server.Client()}, action.Inspect, []string{"source"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := surface[0].Execute(ctx, []byte(`{}`)); err == nil {
		t.Fatal("canceled connector call succeeded")
	}
}
