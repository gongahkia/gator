package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestStreamableHTTPInitializesListsAndCallsTools(t *testing.T) {
	var session string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("MCP-Protocol-Version") != protocolVersion {
			t.Errorf("protocol header = %q", request.Header.Get("MCP-Protocol-Version"))
		}
		if session != "" && request.Header.Get("Mcp-Session-Id") != session {
			t.Errorf("session header = %q, want %q", request.Header.Get("Mcp-Session-Id"), session)
		}
		var call struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Fatal(err)
		}
		if call.Method == "initialize" {
			session = "session-1"
			writer.Header().Set("Mcp-Session-Id", session)
		}
		result := any(map[string]any{})
		switch call.Method {
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "lookup", "description": "lookup data", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = writer.Write([]byte(`data: {"jsonrpc":"2.0","id":` + jsonNumber(call.ID) + `,"result":{"content":[{"type":"text","text":"ok"}]}}` + "\n\n"))
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result})
	}))
	defer server.Close()

	client := newHTTPClient(server.URL)
	tools, err := listTools(context.Background(), client)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "lookup" {
		t.Fatalf("tools = %#v", tools)
	}
	set := Set{trusted: true, servers: []loadedServer{{name: "demo", client: client, tools: tools}}}
	tool := set.Tools(func(context.Context, string, string) error { return nil })[0]
	result, err := tool.Execute(context.Background(), []byte(`{"query":"Ada"}`))
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !strings.Contains(result.Content, `"text":"ok"`) {
		t.Fatalf("tool result = %s", result.Content)
	}
}

func TestBundleHashPinsStdioExecutable(t *testing.T) {
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, ".gator", "mcp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gator", "mcp", "server"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":1,"servers":[{"name":"demo","transport":"stdio","command":[".gator/mcp/server"]}]}`
	if err := os.WriteFile(filepath.Join(repository, ".gator", "mcp.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := BundleHash(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gator", "mcp", "server"), []byte("#!/bin/sh\nprintf changed\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := BundleHash(repository)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("MCP bundle hash did not change after executable changed")
	}
}

func TestMCPToolRequiresApproval(t *testing.T) {
	tool := Tool{server: loadedServer{name: "demo", client: &testClient{}, tools: []toolDescription{{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}}}, specification: toolDescription{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("tool error = %v", err)
	}
}

type testClient struct{}

func (*testClient) Call(context.Context, string, any) (json.RawMessage, error) {
	return json.RawMessage(`{"content":[]}`), nil
}
func (*testClient) Close() error { return nil }

func jsonNumber(value int64) string { return strconv.FormatInt(value, 10) }
