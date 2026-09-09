package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/eval"
	"github.com/gongahkia/gator/internal/extension"
	"github.com/gongahkia/gator/internal/hooks"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/mcp"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestWorkCodeResolvesCapturedIntegrationsUsingOriginalTrust(t *testing.T) {
	source, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var initialized atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		if call.Method == "initialize" {
			initialized.Add(1)
		}
		result := map[string]any{}
		if call.Method == "tools/list" {
			result["tools"] = []any{map[string]any{"name": "lookup", "description": "fixture lookup", "inputSchema": map[string]any{"type": "object"}}}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result})
	}))
	defer server.Close()
	files := map[string]string{
		"a.txt":                  "base\n",
		".gator/agents.json":     `{"version":1,"profiles":[{"name":"worker","instructions":"amber profile"}]}`,
		".gator/rules.json":      `{"version":1,"rules":[{"id":"scoped","paths":["a.txt"],"file":".gator/scoped.md"}]}`,
		".gator/scoped.md":       "amber scoped",
		".gator/hooks.json":      `{"version":1,"hooks":[{"name":"guard","event":"pre_tool_use","tool":"apply_patch","command":[".gator/guard"]}]}`,
		".gator/guard":           "#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' '{\"decision\":\"allow\"}'\n",
		".gator/lsp.json":        `{"version":1,"servers":[{"name":"fixture","command":[".gator/language-server"],"language":"go"}]}`,
		".gator/language-server": "#!/bin/sh\nexit 0\n",
		".gator/mcp.json":        `{"version":1,"servers":[{"name":"fixture","transport":"streamable_http","url":"` + server.URL + `"}]}`,
		".gator/extensions/guide/gator-extension.json": `{"version":1,"id":"guide","name":"Guide","prompts":["guide.md"]}`,
		".gator/extensions/guide/guide.md":             "amber extension",
	}
	for name, text := range files {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0600)
		if strings.HasPrefix(text, "#!/bin/sh") {
			mode = 0700
		}
		if err := os.WriteFile(path, []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	hookHash, err := hooks.BundleHash(source)
	if err != nil {
		t.Fatal(err)
	}
	mcpHash, err := mcp.BundleHash(source)
	if err != nil {
		t.Fatal(err)
	}
	lspHash, err := lsp.BundleHash(source)
	if err != nil {
		t.Fatal(err)
	}
	extensionHash, err := extension.BundleHash(source)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.ExtensionTrusts = []config.ExtensionTrust{{Repository: source, Hash: extensionHash}}
	store, err := extension.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	codeScript := &eval.ScriptedModel{Turns: []agent.Turn{depthCall("apply_patch", map[string]string{"patch": "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-base\n+changed\n"}), depthCall("git_status", map[string]any{}), depthCall("git_diff", map[string]any{}), depthCall("run_command", map[string]any{"argv": []string{"git", "diff", "--check"}}), {Text: "Verified"}}}
	backend := &nativeWorkBackend{provider: "scripted", model: "fixture", code: gatorrun.Executor{HookTrusts: []hooks.Trust{{Repository: source, Hash: hookHash}}, MCPTrusts: []mcp.Trust{{Repository: source, Hash: mcpHash}}, LSPTrusts: []lsp.Trust{{Repository: source, Hash: lspHash}}, Extensions: extension.NewResolver(store, settings), Model: depthModel(func(ctx context.Context, r agent.TurnRequest) (agent.Turn, error) {
		for _, text := range []string{"amber profile", "amber scoped", "amber extension"} {
			if !strings.Contains(r.System, text) {
				t.Errorf("missing frozen instruction %q", text)
			}
		}
		if strings.Contains(r.System, "mutable replacement") {
			t.Error("live configuration leaked")
		}
		tools := map[string]bool{}
		for _, tool := range r.Tools {
			tools[tool.Name] = true
		}
		if !tools["mcp_fixture_lookup"] || !tools["lsp_fixture_diagnostics"] {
			t.Errorf("trusted integration tool resolution failed: %v", tools)
		}
		return codeScript.Complete(ctx, r)
	})}}
	state := t.TempDir()
	manager := &eval.ScriptedModel{Turns: []agent.Turn{depthCall("delegate_agents", map[string]any{"tasks": []any{map[string]string{"agent": "code", "task": "Change a.txt"}}}), depthCall("write_artifact", map[string]string{"path": "report.md", "content": "Prepared a frozen-configuration patch"}), {Text: "Prepared"}}}
	outcome, err := (workrun.Service{Executor: workrun.Executor{Model: manager, Code: backend.codeDelegate(state), StateDir: state}}).Execute(context.Background(), workrun.Request{RunID: "configuration", SourcePath: source, Objective: "Prepare a patch", RequireCode: true, Contract: artifact.DefaultContract("report.md"), Code: workrun.CodePolicy{Profile: "worker", Scopes: []string{"a.txt"}, Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}, Capabilities: []string{"hooks", "mcp", "lsp", "extension"}}, OnSnapshot: func(snapshot.Manifest) {
		for name := range files {
			if strings.HasPrefix(name, ".gator/") {
				if err := os.WriteFile(filepath.Join(source, name), []byte("mutable replacement"), 0600); err != nil {
					t.Error(err)
				}
			}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Manifest.Subagents) != 1 || outcome.Manifest.Subagents[0].Status != "completed" || initialized.Load() != 1 {
		t.Fatalf("Code evidence: %+v; initialized=%d", outcome.Manifest.Subagents, initialized.Load())
	}
	hookSeen := false
	for _, event := range outcome.Events {
		hookSeen = hookSeen || event.Kind == agent.EventHook && strings.Contains(event.Text, "guard")
	}
	if !hookSeen {
		t.Fatal("captured trusted hook did not execute")
	}
}
