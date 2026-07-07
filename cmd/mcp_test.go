package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPServeListsTools(t *testing.T) {
	isolateEnv(t)
	out := executeRoot(t, []string{"mcp", "serve"}, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`+"\n")
	for _, want := range []string{`"name":"gather"`, `"name":"compress"`, `"name":"digest"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %s:\n%s", want, out)
		}
	}
}

func TestMCPServeDigestProtocolFraming(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc target() {}\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cwd, err := json.Marshal(dir)
	if err != nil {
		t.Fatalf("marshal cwd: %v", err)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"dev"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"digest","arguments":{"cwd":` + string(cwd) + `,"instruction":"inspect target"}}}`,
		"",
	}, "\n")
	stdout, stderr, err := executeRootErr(t, []string{"mcp", "serve", "--disable-compress"}, input)
	if err != nil {
		t.Fatalf("mcp serve: %v\nstderr:\n%s\nstdout:\n%s", err, stderr, stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr not protocol-safe empty:\n%s", stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("response lines = %d\n%s", len(lines), stdout)
	}
	for _, line := range lines {
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("stdout line is not JSON-RPC: %v\n%s", err, line)
		}
		if resp["jsonrpc"] != "2.0" {
			t.Fatalf("jsonrpc = %#v in %s", resp["jsonrpc"], line)
		}
	}
	digest := decodeMCPResponse(t, lines[2])
	result := digest["result"].(map[string]any)
	structured := result["structuredContent"].(map[string]any)
	if structured["stage"] != "compress" || structured["digest"] == nil {
		t.Fatalf("digest result = %#v", structured)
	}
}

func decodeMCPResponse(t *testing.T, line string) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != nil {
		t.Fatalf("response error: %#v", resp["error"])
	}
	return resp
}
