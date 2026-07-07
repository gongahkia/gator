package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
)

func TestToolsListIncludesContextToolSchemas(t *testing.T) {
	out := serveLines(t, NewServer(fakeRunner{}), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	resp := decodeResponse(t, out)
	if resp.Error != nil {
		t.Fatalf("error = %#v", resp.Error)
	}
	tools := responseTools(t, resp)
	for _, name := range []string{"gather", "compress", "digest"} {
		if tools[name].Name == "" {
			t.Fatalf("missing tool %s in %#v", name, tools)
		}
	}
	digest := tools["digest"].InputSchema
	if digest["type"] != "object" || digest["additionalProperties"] != false {
		t.Fatalf("digest schema = %#v", digest)
	}
	if !schemaRequires(digest, "cwd") || !schemaRequires(digest, "instruction") {
		t.Fatalf("digest required = %#v", digest["required"])
	}
}

func TestDigestCallReturnsEnvelopeStructuredContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc target() {}\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg := config.Defaults()
	cfg.Gather = config.GatherConfig{MaxDepth: 2, MaxFileBytes: 4096}
	args := `{"cwd":` + quoteJSON(t, dir) + `,"instruction":"inspect target"}`
	req := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"digest","arguments":` + args + `}}`

	out := serveLines(t, NewServer(StageRunner{Config: cfg, DisableCompress: true}), req)
	resp := decodeResponse(t, out)
	if resp.Error != nil {
		t.Fatalf("error = %#v", resp.Error)
	}
	env := responseEnvelope(t, resp)
	if env.SchemaVersion != envelope.SchemaVersion || env.Stage != "compress" || env.Digest == nil {
		t.Fatalf("unexpected envelope = %#v", env)
	}
	if env.Raw == nil || len(env.Raw.Units) == 0 {
		t.Fatalf("missing raw context: %#v", env.Raw)
	}
	if env.Digest.Summary == "" {
		t.Fatalf("missing digest summary")
	}
}

func TestInvalidDigestInputReturnsStructuredError(t *testing.T) {
	out := serveLines(t, NewServer(fakeRunner{}), `{"jsonrpc":"2.0","id":"bad","method":"tools/call","params":{"name":"digest","arguments":{"cwd":"/tmp"}}}`)
	resp := decodeResponse(t, out)
	if resp.Error == nil || resp.Error.Code != codeInvalidParams || !strings.Contains(resp.Error.Message, "missing instruction") {
		t.Fatalf("error = %#v", resp.Error)
	}
}

func serveLines(t *testing.T, server *Server, lines ...string) string {
	t.Helper()
	var in bytes.Buffer
	for _, line := range lines {
		in.WriteString(line)
		in.WriteByte('\n')
	}
	var out bytes.Buffer
	if err := server.Serve(context.Background(), &in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	return out.String()
}

func decodeResponse(t *testing.T, out string) rpcResponse {
	t.Helper()
	var resp rpcResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("decode response: %v\n%s", err, out)
	}
	return resp
}

func responseTools(t *testing.T, resp rpcResponse) map[string]Tool {
	t.Helper()
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode tools: %v", err)
	}
	tools := map[string]Tool{}
	for _, tool := range result.Tools {
		tools[tool.Name] = tool
	}
	return tools
}

func responseEnvelope(t *testing.T, resp rpcResponse) *envelope.Envelope {
	t.Helper()
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var result struct {
		StructuredContent envelope.Envelope `json:"structuredContent"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return &result.StructuredContent
}

func schemaRequires(schema map[string]any, key string) bool {
	items, _ := schema["required"].([]any)
	for _, item := range items {
		if item == key {
			return true
		}
	}
	return false
}

func quoteJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	return string(data)
}

type fakeRunner struct{}

func (fakeRunner) Gather(context.Context, string, string) (*envelope.Envelope, error) {
	return envelope.NewEnvelope("task", "fake", "/tmp"), nil
}

func (fakeRunner) Compress(_ context.Context, env *envelope.Envelope) (*envelope.Envelope, error) {
	out := *env
	out.Stage = "compress"
	return &out, nil
}

func (fakeRunner) Digest(context.Context, string, string) (*envelope.Envelope, error) {
	env := envelope.NewEnvelope("task", "fake", "/tmp")
	env.Stage = "compress"
	env.Digest = &envelope.ContextDigest{Summary: "fake"}
	return env, nil
}
