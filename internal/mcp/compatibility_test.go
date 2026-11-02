package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProtocolErasUseExactMetadataAndNeverDowngradeToolCalls(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(fmt.Sprint(modern), func(t *testing.T) {
			var methods []string
			s := mcpFixture(t, modern, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
				methods = append(methods, c.Method)
				var params map[string]json.RawMessage
				json.Unmarshal(c.Params, &params)
				if c.Method == "server/discover" || modern {
					var meta map[string]json.RawMessage
					json.Unmarshal(params["_meta"], &meta)
					if string(meta["io.modelcontextprotocol/protocolVersion"]) != `"2026-07-28"` || string(meta["io.modelcontextprotocol/clientCapabilities"]) != "{}" || r.Header.Get("Mcp-Method") != c.Method || r.Header.Get("MCP-Protocol-Version") != modernVersion {
						t.Error("modern request metadata missing/mismatched")
					}
					if r.Header.Get("Mcp-Session-Id") != "" {
						t.Error("modern call used legacy session")
					}
				} else if params["_meta"] != nil || r.Header.Get("MCP-Protocol-Version") != protocolVersion {
					t.Error("legacy metadata mismatched")
				}
				if c.Method == "tools/call" && modern && r.Header.Get("Mcp-Name") != "lookup" {
					t.Error("missing native tool header")
				}
				return false
			})
			_, err := InvokeRemoteTool(context.Background(), s.URL, "", "mapped", "lookup", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			want := "server/discover,tools/list,tools/call"
			if !modern {
				want = "server/discover,initialize,notifications/initialized,tools/list,tools/call"
			}
			if strings.Join(methods, ",") != want {
				t.Fatalf("lifecycle %v", methods)
			}
		})
	}
}
func TestProtocolMismatchAndInvalidCapabilitiesNeverEnableTools(t *testing.T) {
	for _, kind := range []string{"future", "recognized-modern-error", "bad-discovery", "no-tools", "old-unsupported-version", "unknown-error-data", "no-legacy-tools"} {
		t.Run(kind, func(t *testing.T) {
			lists := 0
			s := mcpFixture(t, false, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
				if c.Method == "tools/list" {
					lists++
				}
				if c.Method == "server/discover" {
					switch kind {
					case "future":
						fixtureError(w, c.ID, -32022, map[string]any{"supported": []string{"2099-01-01"}})
					case "recognized-modern-error":
						fixtureError(w, c.ID, -32021, nil)
					case "unknown-error-data":
						fixtureError(w, c.ID, -32022, "invalid")
					case "bad-discovery":
						fixtureReply(w, c.ID, map[string]any{"resultType": "complete"})
					case "no-tools":
						fixtureReply(w, c.ID, map[string]any{"resultType": "complete", "supportedVersions": []string{modernVersion}, "capabilities": map[string]any{}})
					default:
						return false
					}
					return true
				}
				if c.Method == "initialize" {
					version := protocolVersion
					caps := map[string]any{"tools": map[string]any{}}
					if kind == "old-unsupported-version" {
						version = "2024-11-05"
					}
					if kind == "no-legacy-tools" {
						delete(caps, "tools")
					}
					fixtureReply(w, c.ID, map[string]any{"protocolVersion": version, "capabilities": caps})
					return true
				}
				return false
			})
			c := newHTTPClient(s.URL)
			defer c.Close()
			tools, err := listTools(context.Background(), c)
			if err == nil || tools != nil || lists != 0 || strings.Contains(err.Error(), "PRIVATE_BACKEND_SECRET") {
				t.Fatalf("unsupported server enabled: %v %v lists=%d", tools, err, lists)
			}
		})
	}
}
func TestRecognizedVersionErrorSelectsOnlyAdvertisedSupportedLegacy(t *testing.T) {
	s := mcpFixture(t, false, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
		if c.Method == "server/discover" {
			w.WriteHeader(400)
			fixtureError(w, c.ID, -32022, map[string]any{"supported": []string{protocolVersion}})
			return true
		}
		return false
	})
	_, err := InvokeRemoteTool(context.Background(), s.URL, "", "mapped", "lookup", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
}
func TestModernToolHeaderAnnotationsAreBoundToExactArguments(t *testing.T) {
	s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
		if c.Method == "tools/list" {
			tool := fixtureTool("Lookup.v2")
			tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"scope":{"type":"object","properties":{"region":{"type":"string","x-mcp-header":"Region"}}},"count":{"type":"integer","x-mcp-header":"Count"},"flag":{"type":"boolean","x-mcp-header":"Flag"},"optional":{"type":"string","x-mcp-header":"Optional"}}}`)
			fixtureReply(w, c.ID, map[string]any{"resultType": "complete", "tools": []toolDescription{tool}})
			return true
		}
		if c.Method == "tools/call" {
			if r.Header.Get("Mcp-Name") != "Lookup.v2" || r.Header.Get("Mcp-Param-Region") != encodeHeader("line1\n世界") || r.Header.Get("Mcp-Param-Count") != "42" || r.Header.Get("Mcp-Param-Flag") != "false" || r.Header.Get("Mcp-Param-Optional") != "" {
				t.Error("incorrect header projection")
			}
		}
		return false
	})
	_, err := InvokeRemoteTool(context.Background(), s.URL, "", "mapped", "Lookup.v2", json.RawMessage(`{"scope":{"region":"line1\n世界"},"count":42,"flag":false,"optional":null}`))
	if err != nil {
		t.Fatal(err)
	}
}
func TestInvalidHeaderAnnotationsAndTypesFailBeforeDispatch(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object","properties":{"a":{"type":"string","x-mcp-header":"Bad\r\nX"}}}`,
		`{"type":"object","properties":{"a":{"type":"string","x-mcp-header":"Same"},"b":{"type":"string","x-mcp-header":"same"}}}`,
		`{"type":"object","properties":{"a":{"type":"number","x-mcp-header":"Float"}}}`,
		`{"type":"object","oneOf":[{"properties":{"a":{"type":"string","x-mcp-header":"Nested"}}}]}`,
	} {
		t.Run(schema, func(t *testing.T) {
			calls := 0
			s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
				if c.Method == "tools/list" {
					tool := fixtureTool("lookup")
					tool.InputSchema = json.RawMessage(schema)
					fixtureReply(w, c.ID, map[string]any{"resultType": "complete", "tools": []toolDescription{tool}})
					return true
				}
				if c.Method == "tools/call" {
					calls++
				}
				return false
			})
			_, err := InvokeRemoteTool(context.Background(), s.URL, "", "mapped", "lookup", json.RawMessage(`{}`))
			if err == nil || calls != 0 {
				t.Fatalf("invalid headers dispatched: %v %d", err, calls)
			}
		})
	}
	for _, args := range []string{`{"count":1.5}`, `{"count":9007199254740992}`, `{"count":9007199254740991.1}`, `{"count":1e99999999}`, `{"count":"42"}`} {
		if err := applyHeaders(http.Header{}, []headerBinding{{Name: "Count", Type: "integer", Path: []string{"count"}}}, json.RawMessage(args)); err == nil {
			t.Fatalf("bad integer accepted: %s", args)
		}
	}
}
func TestRPCDoesNotConfuseFractionalIDsMalformedResultsOrPrivateErrors(t *testing.T) {
	for _, raw := range []string{`{"jsonrpc":"2.0","id":1.5,"result":{}}`, `{"jsonrpc":"1.0","id":1,"result":{}}`, `{"jsonrpc":"2.0","id":1,"result":null}`, `{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-1}}`, `{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"PRIVATE_BACKEND_SECRET"}}`} {
		_, err := parseRPCResponse([]byte(raw), 1)
		if err == nil || strings.Contains(err.Error(), "PRIVATE_BACKEND_SECRET") {
			t.Fatalf("unsafe RPC %s: %v", raw, err)
		}
	}
}
func TestToolResultErrorIsNotSuccessfulAgentActivity(t *testing.T) {
	tool := Tool{server: loadedServer{name: "fixture", client: &errorResultClient{}}, specification: fixtureTool("lookup"), approve: func(context.Context, string, string) error { return nil }}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err == nil || strings.Contains(err.Error(), "PRIVATE_BACKEND_SECRET") {
		t.Fatalf("tool error lost or leaked: %v", err)
	}
}

type errorResultClient struct{}

func (*errorResultClient) Call(context.Context, string, any) (json.RawMessage, error) {
	return json.RawMessage(`{"isError":true,"content":[{"text":"PRIVATE_BACKEND_SECRET"}]}`), nil
}
func (*errorResultClient) Close() error { return nil }

func TestUntrustedOrChangedProjectNeverInvokesServer(t *testing.T) {
	var requests atomic.Int32
	s := mcpFixture(t, true, func(http.ResponseWriter, *http.Request, fixtureCall) bool { requests.Add(1); return false })
	repository := oauthRepository(t, s.URL)
	digest, err := BundleHash(repository)
	if err != nil {
		t.Fatal(err)
	}
	for _, trust := range []string{"", strings.Repeat("0", 64)} {
		set, err := Load(context.Background(), repository, trust)
		if err != nil || set.Trusted() || len(set.Tools(nil)) != 0 {
			t.Fatalf("untrusted setup: %v %v", set, err)
		}
		set.Close()
	}
	if requests.Load() != 0 {
		t.Fatal("untrusted config contacted a server")
	}
	// A display-only manifest edit changes the existing trust digest too.
	path := filepath.Join(repository, ".gator", "mcp.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	set, err := Load(context.Background(), repository, digest)
	if err != nil || set.Trusted() {
		t.Fatalf("stale trust: %v", err)
	}
	set.Close()
	if requests.Load() != 0 {
		t.Fatal("stale trust contacted a server")
	}
}
func TestModernErrorWithoutIDNeverFallsBackToLegacy(t *testing.T) {
	var initialized int
	s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
		if c.Method == "initialize" {
			initialized++
		}
		if c.Method == "server/discover" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			fmt.Fprint(w, `{"jsonrpc":"2.0","error":{"code":-32022,"data":{"supported":["2099-01-01"]}}}`)
			return true
		}
		return false
	})
	c := newHTTPClient(s.URL)
	defer c.Close()
	_, err := listTools(context.Background(), c)
	if err == nil || initialized != 0 {
		t.Fatalf("modern server incorrectly downgraded: %v %d", err, initialized)
	}
}
func TestClosedHTTPClientCannotDispatchAgain(t *testing.T) {
	var requests atomic.Int32
	s := mcpFixture(t, true, func(http.ResponseWriter, *http.Request, fixtureCall) bool { requests.Add(1); return false })
	c := newHTTPClient(s.URL)
	c.Close()
	for i := 0; i < 100; i++ {
		if _, err := c.Call(context.Background(), "tools/call", map[string]any{}); err == nil {
			t.Fatal("closed client accepted call")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("closed client dispatched")
	}
}

func TestInvalidArgumentsCannotStartRemoteSetupOrDispatch(t *testing.T) {
	var requests atomic.Int32
	s := mcpFixture(t, true, func(http.ResponseWriter, *http.Request, fixtureCall) bool { requests.Add(1); return false })
	for _, args := range []string{`null`, `[]`, `"text"`, `invalid`} {
		if _, err := InvokeRemoteTool(context.Background(), s.URL, "", "mapped", "lookup", json.RawMessage(args)); err == nil {
			t.Fatal("invalid call accepted")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("invalid arguments contacted server")
	}
}

func TestHeaderIntegersPreserveExactJSONSchemaSemantics(t *testing.T) {
	for _, value := range []string{"42", "42.0", "4.2e1"} {
		headers := http.Header{}
		if err := applyHeaders(headers, []headerBinding{{Name: "Count", Type: "integer", Path: []string{"count"}}}, json.RawMessage(`{"count":`+value+`}`)); err != nil || headers.Get("Mcp-Param-Count") != "42" {
			t.Fatalf("exact integer %s: %v", value, err)
		}
	}
}
