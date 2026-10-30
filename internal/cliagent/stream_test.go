package cliagent

import (
	"strings"
	"testing"
)

func TestStreamsRequireTerminalSuccess(t *testing.T) {
	for _, test := range []struct {
		kind    string
		lines   []string
		summary string
	}{
		{"codex", []string{`{"type":"thread.started"}`, `{"type":"item.completed","item":{"type":"agent_message","text":"done"}}`, `{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}`}, "done"},
		{"claude", []string{`{"type":"assistant"}`, `{"type":"result","subtype":"success","result":"done","usage":{"input_tokens":10,"output_tokens":2}}`}, "done"},
		{"pi", []string{`{"type":"message_end","message":{"role":"user","content":"read notes"}}`, `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"stopReason":"stop","usage":{"input":10,"output":2}}}`, `{"type":"agent_end"}`, `{"type":"agent_settled","aborted":false}`}, "done"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			s := streamState{}
			for i, line := range test.lines {
				if err := s.accept(test.kind, []byte(line)); err != nil {
					t.Fatal(err)
				}
				if i < len(test.lines)-1 && s.complete {
					t.Fatal("premature completion")
				}
			}
			if !s.complete || s.failed || s.summary != test.summary || s.usage.InputTokens != 10 {
				t.Fatalf("state: %#v", s)
			}
		})
	}
	s := streamState{}
	if err := s.accept("claude", []byte(`{"type":"result","subtype":"error_max_turns","is_error":true}`)); err != nil || !s.failed {
		t.Fatal("false success")
	}
	if err := s.accept("codex", []byte("not json")); err == nil {
		t.Fatal("malformed stream accepted")
	}
}
func TestAdapterArgumentsDoNotBypassPermissions(t *testing.T) {
	for _, kind := range []string{"codex", "claude", "pi"} {
		c := Config{Kind: kind}
		args := strings.Join(c.Arguments(false), " ")
		if strings.Contains(args, "dangerously") || strings.Contains(args, "bypassPermissions") {
			t.Fatal(args)
		}
		if kind == "codex" && !strings.Contains(args, "read-only") {
			t.Fatal(args)
		}
		if kind == "pi" && strings.Contains(args, "bash") {
			t.Fatal(args)
		}
	}
}
