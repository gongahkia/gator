package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fixtureCall struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func fixtureReply(w http.ResponseWriter, id int64, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}
func fixtureError(w http.ResponseWriter, id int64, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": "PRIVATE_BACKEND_SECRET", "data": data}})
}
func modernDiscovery() any {
	return map[string]any{"resultType": "complete", "supportedVersions": []string{modernVersion}, "capabilities": map[string]any{"tools": map[string]any{}}, "ttlMs": 0, "cacheScope": "private"}
}
func fixtureTool(name string) toolDescription {
	return toolDescription{Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func mcpFixture(t *testing.T, modern bool, handle func(http.ResponseWriter, *http.Request, fixtureCall) bool) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call fixtureCall
		if json.NewDecoder(r.Body).Decode(&call) != nil {
			t.Error("bad request")
			w.WriteHeader(400)
			return
		}
		if handle != nil && handle(w, r, call) {
			return
		}
		switch call.Method {
		case "server/discover":
			if modern {
				fixtureReply(w, call.ID, modernDiscovery())
			} else {
				fixtureError(w, call.ID, -32601, nil)
			}
		case "initialize":
			fixtureReply(w, call.ID, map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}})
		case "notifications/initialized":
			w.WriteHeader(202)
		case "tools/list":
			result := map[string]any{"tools": []toolDescription{fixtureTool("lookup")}}
			if modern {
				result["resultType"] = "complete"
			}
			fixtureReply(w, call.ID, result)
		case "tools/call":
			result := map[string]any{"content": []any{map[string]any{"type": "text", "text": "fixture result"}}}
			if modern {
				result["resultType"] = "complete"
			}
			fixtureReply(w, call.ID, result)
		default:
			fixtureError(w, call.ID, -32601, nil)
		}
	}))
	t.Cleanup(s.Close)
	return s
}
func TestDiscoveryPaginatesAndInvokesMappedToolOnLastPage(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(fmt.Sprint(modern), func(t *testing.T) {
			var cursors []string
			calls := 0
			s := mcpFixture(t, modern, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
				if c.Method == "tools/list" {
					var p struct {
						Cursor string `json:"cursor"`
					}
					json.Unmarshal(c.Params, &p)
					cursors = append(cursors, p.Cursor)
					result := map[string]any{"resultType": "complete"}
					switch p.Cursor {
					case "":
						result["tools"] = []toolDescription{fixtureTool("first")}
						result["nextCursor"] = "opaque+/=cursor"
					case "opaque+/=cursor":
						result["tools"] = []toolDescription{}
						result["nextCursor"] = "last"
					case "last":
						result["tools"] = []toolDescription{fixtureTool("lookup")}
					default:
						t.Error("cursor changed")
					}
					fixtureReply(w, c.ID, result)
					return true
				}
				if c.Method == "tools/call" {
					calls++
				}
				return false
			})
			raw, err := InvokeRemoteTool(context.Background(), s.URL, "", "mapped", "lookup", json.RawMessage(`{}`))
			if err != nil || !strings.Contains(string(raw), "fixture result") {
				t.Fatalf("call: %s %v", raw, err)
			}
			if calls != 1 || strings.Join(cursors, ",") != ",opaque+/=cursor,last" {
				t.Fatalf("calls/cursors: %d %v", calls, cursors)
			}
		})
	}
}
func TestDiscoveryRejectsPartialOrUnboundedCatalogs(t *testing.T) {
	for _, kind := range []string{"repeated-cursor", "duplicate-tool", "alias-collision", "too-many-pages", "too-many-tools", "too-many-bytes", "long-cursor", "invalid-schema", "missing-tools"} {
		t.Run(kind, func(t *testing.T) {
			page := 0
			s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
				if c.Method != "tools/list" {
					return false
				}
				page++
				result := map[string]any{"resultType": "complete", "tools": []toolDescription{fixtureTool(fmt.Sprintf("tool%d", page))}}
				switch kind {
				case "repeated-cursor":
					result["nextCursor"] = "again"
				case "duplicate-tool":
					result["tools"] = []toolDescription{fixtureTool("same"), fixtureTool("same")}
				case "alias-collision":
					result["tools"] = []toolDescription{fixtureTool("same-a"), fixtureTool("same_a")}
				case "too-many-pages":
					result["nextCursor"] = fmt.Sprint(page)
				case "too-many-tools":
					tools := make([]toolDescription, 257)
					for i := range tools {
						tools[i] = fixtureTool(fmt.Sprintf("tool%d", i))
					}
					result["tools"] = tools
				case "too-many-bytes":
					tool := fixtureTool(fmt.Sprintf("tool%d", page))
					tool.Description = strings.Repeat("x", 70*1024)
					result["tools"] = []toolDescription{tool}
					result["nextCursor"] = fmt.Sprint(page)
				case "long-cursor":
					result["nextCursor"] = strings.Repeat("x", maxCursor+1)
				case "invalid-schema":
					result["tools"] = []toolDescription{{Name: "lookup", InputSchema: json.RawMessage(`null`)}}
				case "missing-tools":
					delete(result, "tools")
				}
				fixtureReply(w, c.ID, result)
				return true
			})
			c := newHTTPClient(s.URL)
			defer c.Close()
			tools, err := listTools(context.Background(), c)
			if err == nil || tools != nil {
				t.Fatalf("partial catalog enabled: %d tools, %v", len(tools), err)
			}
		})
	}
}
func TestSSEMatchesResponseBeforeEOFThroughCommentsNotificationsAndMultilineData(t *testing.T) {
	s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
		if c.Method != "tools/call" {
			return false
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprint(w, ": keep alive\r\nid: ignored\r\ndata:\r\n\r\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\r\n\r\n")
		fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%d,\r\ndata: \"result\":{\"resultType\":\"complete\",\"content\":[]}}\r\n\r\n", c.ID)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		return true
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	raw, err := InvokeRemoteTool(ctx, s.URL, "", "mapped", "lookup", json.RawMessage(`{}`))
	if err != nil || !strings.Contains(string(raw), "complete") {
		t.Fatalf("matching response: %s %v", raw, err)
	}
}
func TestSSERejectsMissingMalformedOrUnboundedResults(t *testing.T) {
	for _, kind := range []string{"wrong-id", "malformed", "oversized-line", "oversized-stream", "many-events", "server-request", "rpc-error"} {
		t.Run(kind, func(t *testing.T) {
			s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
				if c.Method != "tools/call" {
					return false
				}
				w.Header().Set("Content-Type", "text/event-stream")
				switch kind {
				case "wrong-id":
					fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":999,\"result\":{}}\n\n")
				case "malformed":
					fmt.Fprint(w, "data: invalid\n\n")
				case "oversized-line":
					fmt.Fprint(w, "data: "+strings.Repeat("x", maxWire+1)+"\n\n")
				case "oversized-stream":
					for i := 0; i < 300; i++ {
						fmt.Fprint(w, ":"+strings.Repeat("x", 4096)+"\n")
					}
				case "many-events":
					for i := 0; i < 257; i++ {
						fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n")
					}
				case "server-request":
					fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"sampling/createMessage\",\"id\":7}\n\n")
				case "rpc-error":
					fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%d,\"error\":{\"code\":-32001,\"message\":\"PRIVATE_BACKEND_SECRET\"}}\n\n", c.ID)
				}
				return true
			})
			_, err := InvokeRemoteTool(context.Background(), s.URL, "", "mapped", "lookup", json.RawMessage(`{}`))
			if err == nil || strings.Contains(err.Error(), "PRIVATE_BACKEND_SECRET") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}
func TestHTTPQueueCancellationCloseAndNoMutationReplay(t *testing.T) {
	entered := make(chan struct{})
	var once sync.Once
	var count atomic.Int32
	s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
		if c.Method != "tools/call" {
			return false
		}
		count.Add(1)
		once.Do(func() { close(entered) })
		<-r.Context().Done()
		return true
	})
	c := newHTTPClient(s.URL)
	if _, err := listTools(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "tools/call", map[string]any{"name": "lookup", "arguments": map[string]any{}})
		first <- err
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := c.Call(ctx, "tools/call", map[string]any{"name": "lookup", "arguments": map[string]any{}})
	if !errors.Is(err, context.DeadlineExceeded) || count.Load() != 1 {
		t.Fatalf("queued call dispatched: %v / %d", err, count.Load())
	}
	c.Close()
	select {
	case err := <-first:
		if err == nil {
			t.Fatal("closed call succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not interrupt HTTP read")
	}
	if count.Load() != 1 {
		t.Fatal("mutation replayed")
	}
}
func TestHTTPDoesNotRedirectCredentialsOrReplayLostReply(t *testing.T) {
	var redirected atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer other.Close()
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			var calls atomic.Int32
			s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
				if c.Method != "tools/call" {
					return false
				}
				calls.Add(1)
				if lost {
					connection, _, _ := w.(http.Hijacker).Hijack()
					connection.Close()
				} else {
					http.Redirect(w, r, other.URL, 307)
				}
				return true
			})
			_, err := InvokeRemoteTool(context.Background(), s.URL, "fixture-token", "mapped", "lookup", json.RawMessage(`{}`))
			if err == nil || calls.Load() != 1 || redirected.Load() != 0 || strings.Contains(err.Error(), "fixture-token") {
				t.Fatalf("call was unsafe: %v %d %d", err, calls.Load(), redirected.Load())
			}
		})
	}
}
func TestHTTPRejectsIncompleteToolResultWithoutReplay(t *testing.T) {
	for _, result := range []string{`{"resultType":"input_required","inputRequests":{}}`, `{"resultType":"future"}`, `{"content":[]}`, `{"resultType":"complete","isError":true,"content":[]}`} {
		t.Run(result, func(t *testing.T) {
			calls := 0
			s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
				if c.Method != "tools/call" {
					return false
				}
				calls++
				fixtureReply(w, c.ID, json.RawMessage(result))
				return true
			})
			_, err := InvokeRemoteTool(context.Background(), s.URL, "", "mapped", "lookup", json.RawMessage(`{}`))
			if err == nil || calls != 1 {
				t.Fatalf("incomplete tool result: %v %d", err, calls)
			}
		})
	}
}

func TestHTTPBodyDeadlineAndUTF8Bounds(t *testing.T) {
	for _, kind := range []string{"stalled-sse", "stalled-json", "large-result", "invalid-utf8"} {
		t.Run(kind, func(t *testing.T) {
			s := mcpFixture(t, true, func(w http.ResponseWriter, r *http.Request, c fixtureCall) bool {
				if c.Method != "tools/call" {
					return false
				}
				switch kind {
				case "stalled-sse":
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, ": keep alive\n\n")
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				case "stalled-json":
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"jsonrpc":`)
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				case "large-result":
					fixtureReply(w, c.ID, map[string]any{"resultType": "complete", "content": []any{map[string]any{"type": "text", "text": strings.Repeat("界", maxToolOutput/3+1)}}})
				case "invalid-utf8":
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, "{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"resultType\":\"complete\",\"text\":\"", c.ID)
					w.Write([]byte{0xff})
					fmt.Fprint(w, `"}}`)
				}
				return true
			})
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := InvokeRemoteTool(ctx, s.URL, "", "mapped", "lookup", json.RawMessage(`{}`))
			if err == nil || time.Since(start) > 2*time.Second {
				t.Fatalf("unbounded/invalid response: %v", err)
			}
		})
	}
}
