package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// This fixture is the test executable itself, not an installed developer MCP
// service. It receives only private protocol data and never launches a shell.
func TestMCPChild(t *testing.T) {
	mode := os.Getenv("GATOR_TEST_MCP_CHILD")
	if mode == "" {
		return
	}
	if mode == "blocked-write" {
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), maxWire+1)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var call fixtureCall
		if json.Unmarshal(scanner.Bytes(), &call) != nil {
			os.Exit(2)
		}
		if call.Method == "notifications/initialized" || call.Method == "notifications/cancelled" {
			continue
		}
		if call.Method == "server/discover" && (mode == "legacy-silent" || mode == "legacy-late") {
			if mode == "legacy-silent" {
				continue
			}
			time.Sleep(probeTimeout + 50*time.Millisecond)
		}
		var result any
		switch call.Method {
		case "server/discover":
			if strings.HasPrefix(mode, "legacy") {
				_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "error": map[string]any{"code": -32602}})
				continue
			}
			result = modernDiscovery()
		case "initialize":
			result = map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "tools/list":
			result = map[string]any{"resultType": "complete", "tools": []toolDescription{fixtureTool("lookup")}}
		case "tools/call":
			switch mode {
			case "stalled-read":
				continue
			case "oversized":
				fmt.Fprintln(os.Stdout, strings.Repeat("x", maxWire+1))
				continue
			case "exit":
				os.Exit(0)
			}
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress"})
			result = map[string]any{"resultType": "complete", "content": []any{map[string]any{"type": "text", "text": "child result"}}}
		default:
			os.Exit(3)
		}
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result})
	}
	if mode == "ignore-eof" {
		time.Sleep(time.Hour)
	}
	os.Exit(0)
}
func childClient(t *testing.T, mode string) (*stdioClient, *atomic.Int32) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(context.Background(), exe, "-test.run=^TestMCPChild$")
	command.Env = append(os.Environ(), "GATOR_TEST_MCP_CHILD="+mode)
	cleanup := &atomic.Int32{}
	c, err := startStdio(context.Background(), command, func() { cleanup.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	client := c.(*stdioClient)
	t.Cleanup(func() { client.Close() })
	return client, cleanup
}
func TestStdioRealProcessModernAndLegacyDiscovery(t *testing.T) {
	for _, mode := range []string{"modern", "legacy", "legacy-silent", "legacy-late"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := childClient(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			tools, err := listTools(ctx, c)
			if err != nil || len(tools) != 1 {
				t.Fatalf("discovery: %v %v", tools, err)
			}
			raw, err := c.Call(ctx, "tools/call", map[string]any{"name": "lookup", "arguments": map[string]any{}})
			if err != nil || !strings.Contains(string(raw), "child result") {
				t.Fatalf("call: %s %v", raw, err)
			}
		})
	}
}
func TestStdioBlockedReadOrWriteCancelsAndNeverReplays(t *testing.T) {
	for _, mode := range []string{"stalled-read", "blocked-write"} {
		t.Run(mode, func(t *testing.T) {
			c, cleanup := childClient(t, mode)
			if mode != "blocked-write" {
				if _, err := listTools(context.Background(), c); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := c.Call(ctx, "tools/call", map[string]any{"name": "lookup", "arguments": map[string]any{"text": strings.Repeat("x", 100*1024)}})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("cancel: %v", err)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("cancellation blocked")
			}
			if _, err := c.Call(context.Background(), "tools/call", map[string]any{}); err == nil {
				t.Fatal("closed client restarted/replayed")
			}
			c.Close()
			if cleanup.Load() != 1 {
				t.Fatal("cleanup not exactly once")
			}
			select {
			case <-c.exited:
			case <-time.After(time.Second):
				t.Fatal("fixture subprocess survived cancellation")
			}
		})
	}
}
func TestStdioCloseReapsStubbornChildAndInterruptsWaitingCall(t *testing.T) {
	for _, mode := range []string{"ignore-eof", "stalled-read"} {
		t.Run(mode, func(t *testing.T) {
			c, cleanup := childClient(t, mode)
			if _, err := listTools(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			first := make(chan error, 1)
			if mode == "stalled-read" {
				go func() { _, err := c.Call(context.Background(), "tools/call", map[string]any{}); first <- err }()
				time.Sleep(20 * time.Millisecond)
			}
			start := time.Now()
			c.Close()
			c.Close()
			if time.Since(start) > 2*time.Second || cleanup.Load() != 1 {
				t.Fatal("unbounded/non-idempotent close")
			}
			if mode == "stalled-read" {
				select {
				case err := <-first:
					if err == nil {
						t.Fatal("closed call succeeded")
					}
				case <-time.After(time.Second):
					t.Fatal("read remained blocked")
				}
			}
			select {
			case <-c.exited:
			case <-time.After(time.Second):
				t.Fatal("stubborn subprocess was not reaped")
			}
		})
	}
}
func TestStdioRejectsOversizedMessagesAndUnexpectedExit(t *testing.T) {
	for _, mode := range []string{"oversized", "exit"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := childClient(t, mode)
			if _, err := listTools(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Call(context.Background(), "tools/call", map[string]any{}); err == nil {
				t.Fatal("bad response accepted")
			}
		})
	}
}
func TestStdioQueuedCancellationDoesNotWaitForInflightCall(t *testing.T) {
	c, _ := childClient(t, "stalled-read")
	if _, err := listTools(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := c.Call(context.Background(), "tools/call", map[string]any{}); result <- err }()
	// The gate's occupancy is inspected only to synchronize this failure fixture.
	deadline := time.Now().Add(time.Second)
	for len(c.gate) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := c.Call(ctx, "tools/call", map[string]any{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued cancellation: %v", err)
	}
	c.Close()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("close did not unblock first call")
	}
}
