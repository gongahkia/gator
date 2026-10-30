package cliagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaudeNativeConversationQuestionsPermissionsAndInterrupt(t *testing.T) {
	client, server := net.Pipe()
	wire := newProtocolWire(client, client, false, true)
	t.Cleanup(func() { wire.Close(); server.Close() })
	reader := bufio.NewReader(server)
	thread := nativeUUID()
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		reply := func(request map[string]any, result any) {
			data, _ := json.Marshal(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": request["request_id"], "response": result}})
			fmt.Fprintln(server, string(data))
		}
		initialize := readRPC(t, reader)
		if initialize["type"] != "control_request" || initialize["request"].(map[string]any)["subtype"] != "initialize" {
			t.Error("missing Claude initialization")
		}
		reply(initialize, map[string]any{})
		user := readRPC(t, reader)
		if user["type"] != "user" || user["session_id"] != thread || user["message"].(map[string]any)["content"] != "Draft report" {
			t.Error("incorrect user frame")
		}
		fmt.Fprintln(server, `{"type":"control_request","request_id":"permission-one","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"publish"}}}`)
		denial := readRPC(t, reader)
		if denial["response"].(map[string]any)["response"].(map[string]any)["behavior"] != "deny" {
			t.Error("permission automatically granted")
		}
		fmt.Fprintln(server, `{"type":"control_request","request_id":"question-one","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{"questions":[{"question":"Which format?","header":"Format","multiSelect":false,"options":[{"label":"Markdown"}]}]}}}`)
		answer := readRPC(t, reader)
		response := answer["response"].(map[string]any)
		updated := response["response"].(map[string]any)["updatedInput"].(map[string]any)
		if response["request_id"] != "question-one" || updated["answers"].(map[string]any)["Which format?"] != "Markdown" || len(updated["questions"].([]any)) != 1 {
			t.Error("Claude answer lost original question mapping")
		}
		fmt.Fprintf(server, "{\"type\":\"result\",\"uuid\":\"result-one\",\"session_id\":%q,\"subtype\":\"success\",\"result\":\"Draft ready\"}\n", thread)
		next := readRPC(t, reader)
		if next["session_id"] != thread {
			t.Error("follow-up lost native conversation")
		}
		// A duplicate previous completion cannot finish this new turn.
		fmt.Fprintf(server, "{\"type\":\"result\",\"uuid\":\"result-one\",\"session_id\":%q,\"subtype\":\"success\",\"result\":\"stale\"}\n", thread)
		interrupt := readRPC(t, reader)
		if interrupt["request"].(map[string]any)["subtype"] != "interrupt" {
			t.Error("missing native interrupt")
		}
		reply(interrupt, map[string]any{})
		fmt.Fprintf(server, "{\"type\":\"result\",\"uuid\":\"result-two\",\"session_id\":%q,\"subtype\":\"error_during_execution\",\"is_error\":true,\"errors\":[\"interrupted\"]}\n", thread)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := newNativeClient(ctx, "claude", wire, Config{}, "/fixture", thread)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, "Draft report"); err != nil {
		t.Fatal(err)
	}
	s := waitNative(t, c, "awaiting_input")
	if len(s.Questions) != 1 || s.Questions[0].ID != "question-1" || s.QuestionID == "" {
		t.Fatal(s)
	}
	if err := c.Answer(ctx, map[string][]string{"question-1": {"Markdown"}}); err != nil {
		t.Fatal(err)
	}
	s, err = c.Await(ctx)
	if err != nil || s.Status != "completed" || s.Summary != "Draft ready" {
		t.Fatal(s, err)
	}
	if err := c.Send(ctx, "Revise draft"); err != nil {
		t.Fatal(err)
	}
	if err := c.Steer(ctx, "new direction"); err == nil {
		t.Fatal("advertised unsupported steering")
	}
	if err := c.Interrupt(ctx); err != nil {
		t.Fatal(err)
	}
	s, err = c.Await(ctx)
	if err != nil || s.Status != "cancelled" {
		t.Fatal(s, err)
	}
	<-serverDone
}

func TestClaudeNativeRejectsWrongSessionAndCancelledQuestion(t *testing.T) {
	c := &nativeClient{kind: "claude", state: NativeState{ThreadID: nativeUUID(), Status: "running"}}
	if err := c.event([]byte(`{"type":"result","uuid":"one","session_id":"other","subtype":"success"}`)); err == nil {
		t.Fatal("accepted different session")
	}
	c.state.Status, c.state.QuestionID = "awaiting_input", "host-nonce"
	c.questionID = json.RawMessage(`"pending"`)
	if err := c.event([]byte(`{"type":"control_cancel_request","request_id":"pending"}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.Answer(context.Background(), map[string][]string{}); err == nil {
		t.Fatal("answered cancelled request")
	}
}

func TestClaudeProcessResumesRetainedPrivateSession(t *testing.T) {
	r := fixtureRequest(t, "")
	script, err := os.ReadFile("testdata/claude_rpc.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.Config.Executable, script, 0700); err != nil {
		t.Fatal(err)
	}
	r.Config.Kind, r.Edit = "claude", true
	request := SessionRequest{Request: r, Store: filepath.Join(r.Scratch, "sessions"), Owner: "conversation", Capture: "source"}
	for _, wanted := range []string{"after-1", "after-2"} {
		value, err := RunSession(context.Background(), request)
		if err != nil || !strings.Contains(string(value.Patch), "+"+wanted) {
			t.Fatal(value, err)
		}
		if request.SessionID != "" && value.SessionID != request.SessionID {
			t.Fatal("changed Gator session")
		}
		request.SessionID = value.SessionID
	}
	args := strings.Join(nativeArguments(r.Config, true, "/private/home"), " ")
	for _, expected := range []string{"--bare", "--input-format stream-json", "--permission-prompt-tool stdio", "AskUserQuestion", "--setting-sources="} {
		if !strings.Contains(args, expected) {
			t.Fatal("missing native boundary flag", expected)
		}
	}
	if strings.Contains(args, "--no-session-persistence") || strings.Contains(args, "--dangerously-skip-permissions") {
		t.Fatal(args)
	}
}
