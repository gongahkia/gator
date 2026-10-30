package cliagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func rpcFixture(t *testing.T, pi bool) (*rpcWire, net.Conn, *bufio.Reader) {
	t.Helper()
	client, server := net.Pipe()
	w := newRPCWire(client, client, pi)
	t.Cleanup(func() { w.Close(); _ = server.Close() })
	return w, server, bufio.NewReader(server)
}
func readRPC(t *testing.T, r *bufio.Reader) map[string]any {
	t.Helper()
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Error(err)
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(line, &value); err != nil {
		t.Error(err)
	}
	return value
}
func TestRPCResponsesRemainCorrelatedAroundServerQuestions(t *testing.T) {
	w, server, reader := rpcFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	answers := make(chan string, 2)
	for _, name := range []string{"first", "second"} {
		go func(name string) {
			data, err := w.Call(ctx, name, nil)
			answers <- name + ":" + string(data) + fmt.Sprint(err)
		}(name)
	}
	one, two := readRPC(t, reader), readRPC(t, reader)
	fmt.Fprintln(server, `{"id":7,"method":"item/tool/requestUserInput","params":{"question":"Which file?"}}`)
	for _, request := range []map[string]any{two, one} {
		encoded, _ := json.Marshal(map[string]any{"id": request["id"], "result": request["method"]})
		fmt.Fprintln(server, string(encoded))
	}
	seen := map[string]bool{}
	for range 2 {
		select {
		case result := <-answers:
			seen[result] = true
		case <-ctx.Done():
			t.Fatal("response deadlock")
		}
	}
	if !seen[`first:"first"<nil>`] || !seen[`second:"second"<nil>`] {
		t.Fatal(seen)
	}
	select {
	case event := <-w.events:
		if !strings.Contains(string(event), `"id":7`) {
			t.Fatal(string(event))
		}
	case <-ctx.Done():
		t.Fatal("lost server request")
	}
	acknowledged := make(chan error, 1)
	go func() {
		acknowledged <- w.Reply(ctx, json.RawMessage(`7`), map[string]any{"answers": map[string]any{}})
	}()
	response := readRPC(t, reader)
	if response["id"] != float64(7) {
		t.Fatal(response)
	}
	if err := <-acknowledged; err != nil {
		t.Fatal(err)
	}
}
func TestRPCPiResponseAndUnicodeFraming(t *testing.T) {
	w, server, reader := rpcFixture(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := w.Call(ctx, "prompt", map[string]any{"message": "a\u2028b\u2029c"}); finished <- err }()
	request := readRPC(t, reader)
	if request["message"] != "a\u2028b\u2029c" || request["type"] != "prompt" {
		t.Fatal(request)
	}
	encoded, _ := json.Marshal(map[string]any{"id": request["id"], "type": "response", "success": false, "error": "fixture rejection"})
	fmt.Fprintln(server, string(encoded))
	if err := <-finished; err == nil || !strings.Contains(err.Error(), "fixture rejection") {
		t.Fatal(err)
	}
}
func TestRPCTimeoutClosesAmbiguousTransport(t *testing.T) {
	w, _, reader := rpcFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := w.Call(ctx, "turn/start", nil); finished <- err }()
	_ = readRPC(t, reader)
	if err := <-finished; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	select {
	case <-w.done:
	case <-time.After(time.Second):
		t.Fatal("ambiguous connection remained reusable")
	}
	if _, err := w.Call(context.Background(), "turn/start", nil); err == nil {
		t.Fatal("closed transport accepted another request")
	}
}
func TestRPCMalformedAndOversizedStreamsFailClosed(t *testing.T) {
	for _, line := range []string{"broken", strings.Repeat("x", 1024*1024+1)} {
		t.Run(fmt.Sprint(len(line)), func(t *testing.T) {
			w, server, _ := rpcFixture(t, false)
			go func() { fmt.Fprintln(server, line) }()
			select {
			case <-w.done:
				if w.err() == nil {
					t.Fatal("missing protocol failure")
				}
			case <-time.After(time.Second):
				t.Fatal("unbounded stream")
			}
		})
	}
}
func TestRPCBlockedWriteHonorsCancellation(t *testing.T) {
	w, _, _ := rpcFixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := w.Call(ctx, "turn/start", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
