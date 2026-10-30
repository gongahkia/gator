package cliagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func waitNative(t *testing.T, c *nativeClient, want string) NativeState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := c.State()
		if s.Status == want {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("wanted %s: %+v", want, c.State())
	return NativeState{}
}
func TestCodexNativeFollowupSteeringAndQuestion(t *testing.T) {
	wire, server, reader := rpcFixture(t, false)
	serverErrors := make(chan string, 1)
	go func() {
		reply := func(request map[string]any, result any) {
			data, _ := json.Marshal(map[string]any{"id": request["id"], "result": result})
			fmt.Fprintln(server, string(data))
		}
		init := readRPC(t, reader)
		reply(init, map[string]any{})
		_ = readRPC(t, reader)
		start := readRPC(t, reader)
		if start["method"] != "thread/start" {
			serverErrors <- "missing thread/start"
			return
		}
		reply(start, map[string]any{"thread": map[string]any{"id": "thread-one"}})
		turn := readRPC(t, reader)
		reply(turn, map[string]any{"turn": map[string]any{"id": "turn-one"}})
		steer := readRPC(t, reader)
		params := steer["params"].(map[string]any)
		if steer["method"] != "turn/steer" || params["expectedTurnId"] != "turn-one" {
			serverErrors <- "bad steering"
			return
		}
		reply(steer, map[string]any{"turnId": "turn-one"})
		fmt.Fprintln(server, `{"id":42,"method":"item/tool/requestUserInput","params":{"threadId":"thread-one","turnId":"turn-one","questions":[{"id":"format","question":"Which format?","options":[{"label":"Markdown"}]}]}}`)
		answer := readRPC(t, reader)
		if answer["id"] != float64(42) {
			serverErrors <- "bad question reply"
			return
		}
		fmt.Fprintln(server, `{"method":"item/completed","params":{"threadId":"thread-one","item":{"type":"agentMessage","text":"Draft ready"}}}`)
		fmt.Fprintln(server, `{"method":"turn/completed","params":{"threadId":"thread-one","turn":{"id":"turn-one","status":"completed"}}}`)
		next := readRPC(t, reader)
		if next["params"].(map[string]any)["threadId"] != "thread-one" {
			serverErrors <- "follow-up lost thread"
			return
		}
		reply(next, map[string]any{"turn": map[string]any{"id": "turn-two"}})
		interrupt := readRPC(t, reader)
		if interrupt["method"] != "turn/interrupt" {
			serverErrors <- "missing interrupt"
			return
		}
		reply(interrupt, map[string]any{})
		fmt.Fprintln(server, `{"method":"turn/completed","params":{"threadId":"thread-one","turn":{"id":"turn-two","status":"interrupted"}}}`)
		serverErrors <- ""
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := newNativeClient(ctx, "codex", wire, Config{}, "/fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Send(ctx, "Draft report"); err != nil {
		t.Fatal(err)
	}
	if err := client.Steer(ctx, "Focus on evidence"); err != nil {
		t.Fatal(err)
	}
	state := waitNative(t, client, "awaiting_input")
	if len(state.Questions) != 1 {
		t.Fatal(state)
	}
	if err := client.Answer(ctx, map[string][]string{"wrong": {"Markdown"}}); err == nil {
		t.Fatal("accepted wrong answer target")
	}
	if err := client.Answer(ctx, map[string][]string{"format": {"Markdown"}}); err != nil {
		t.Fatal(err)
	}
	state, err = client.Await(ctx)
	if err != nil || state.Status != "completed" || state.Summary != "Draft ready" {
		t.Fatal(state, err)
	}
	if err := client.Send(ctx, "Revise the same draft"); err != nil {
		t.Fatal(err)
	}
	if err := client.Interrupt(ctx); err != nil {
		t.Fatal(err)
	}
	state, err = client.Await(ctx)
	if err != nil || state.Status != "cancelled" {
		t.Fatal(state, err)
	}
	if problem := <-serverErrors; problem != "" {
		t.Fatal(problem)
	}
}
func TestPiNativeSettledIsRequiredAndAbortClearsQueuedWork(t *testing.T) {
	wire, server, reader := rpcFixture(t, true)
	go func() {
		reply := func(r map[string]any, data any) {
			encoded, _ := json.Marshal(map[string]any{"id": r["id"], "type": "response", "success": true, "data": data})
			fmt.Fprintln(server, string(encoded))
		}
		reply(readRPC(t, reader), map[string]any{"sessionId": "pi-one"})
		reply(readRPC(t, reader), nil)
		fmt.Fprintln(server, `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"working"}],"stopReason":"stop"}}`)
		fmt.Fprintln(server, `{"type":"agent_end"}`)
		r := readRPC(t, reader)
		if r["type"] != "clear_queue" {
			t.Error("abort failed to clear queue")
		}
		reply(r, nil)
		r = readRPC(t, reader)
		if r["type"] != "abort" {
			t.Error("expected abort")
		}
		reply(r, nil)
		fmt.Fprintln(server, `{"type":"agent_settled","aborted":true}`)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, err := newNativeClient(ctx, "pi", wire, Config{}, "/fixture", "pi-one")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, "Inspect"); err != nil {
		t.Fatal(err)
	}
	if c.State().Status == "completed" {
		t.Fatal("premature completion")
	}
	if err := c.Interrupt(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := c.Await(ctx)
	if err != nil || state.Status != "cancelled" {
		t.Fatal(state, err)
	}
}
func TestNativeRejectsOtherThreadAndSecretQuestions(t *testing.T) {
	for _, payload := range []string{`{"method":"turn/completed","params":{"threadId":"other","turn":{"status":"completed"}}}`, `{"id":1,"method":"item/tool/requestUserInput","params":{"questions":[{"id":"password","question":"Secret?","isSecret":true}]}}`} {
		c := &nativeClient{kind: "codex", state: NativeState{ThreadID: "mine", Status: "running"}}
		if err := c.event([]byte(payload)); err == nil {
			t.Fatal("accepted invalid event")
		}
	}
}

func TestNativeIgnoresRetiredTurnsAndRejectsOtherActiveTurn(t *testing.T) {
	c := &nativeClient{kind: "codex", state: NativeState{ThreadID: "thread", TurnID: "current", Status: "running"}, retiredTurns: map[string]bool{"old": true}}
	if err := c.event([]byte(`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"old","status":"completed"}}}`)); err != nil {
		t.Fatal(err)
	}
	if c.State().Status != "running" {
		t.Fatal("old completion finished new work")
	}
	if err := c.event([]byte(`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"unrelated","status":"completed"}}}`)); err == nil {
		t.Fatal("unrelated turn accepted")
	}
}
func TestNativeArgumentsPreserveSessionsAndLimitDiscovery(t *testing.T) {
	for _, kind := range []string{"codex", "pi"} {
		args := strings.Join(nativeArguments(Config{Kind: kind, Provider: "openai"}, false, "/private/session"), " ")
		if strings.Contains(args, "--ephemeral") || strings.Contains(args, "--no-session") {
			t.Fatal(args)
		}
		if kind == "pi" && !strings.Contains(args, "--no-approve") {
			t.Fatal(args)
		}
		if kind == "codex" && !strings.Contains(args, "--disable multi_agent") {
			t.Fatal(args)
		}
	}
}
