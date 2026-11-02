package workactivity

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

func observeTool(name, result string) *Trace {
	t := New(time.Now())
	call := &agent.ToolCall{ID: "call", Name: name, Arguments: json.RawMessage(`{"argv":["private-argument"],"secret":"never-copy"}`)}
	t.Record(agent.Event{Kind: agent.EventToolCalled, ToolCall: call})
	t.Record(agent.Event{Kind: agent.EventToolFinished, ToolCall: call, ToolResult: result})
	return t
}
func TestObservedDetailsIgnoreArgumentsDiagnosticsAndFailedOrUnknownTools(t *testing.T) {
	cases := []struct{ name, result, kind string }{
		{"web_search", `{"ok":true,"result":{"results":[{"url":"https://EXAMPLE.com/private?secret=hidden-query","snippet":"never-copy"}]}}`, "searched"},
		{"web_fetch", `{"evidence":{"id":"evidence-one","locator":"https://example.com/private?secret=hidden-query"},"text":"never-copy"}`, "fetched"},
		{"code_apply_patch", `{"ok":true,"result":{"applied":true,"changed_paths":["src/main.go"]}}`, "changed_file"},
		{"code_stage", `{"candidate":{"id":"candidate-one","status":"verified","changed_paths":["src/main.go"]}}`, "candidate"},
		{"code_run_command", `{"ok":true,"result":{"exit_code":1,"argv":["private-argument"],"output":"never-copy"}}`, "command"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			trace := observeTool(c.name, c.result)
			if len(trace.Entries[0].Details) == 0 || trace.Entries[0].Details[0].Kind != c.kind {
				t.Fatal("missing observed details")
			}
			data, _ := json.Marshal(trace)
			for _, secret := range []string{"private-argument", "never-copy", "hidden-query", "/private?"} {
				if strings.Contains(string(data), secret) {
					t.Fatalf("leaked detail: %s", secret)
				}
			}
			if !trace.ValidDetails() {
				t.Fatal("own details failed validation")
			}
		})
	}
	for _, c := range []struct{ name, result string }{{"unknown", cases[0].result}, {"web_search", `{"ok":false,"result":{"results":[{"url":"https://example.com"}]}}`}, {"code_apply_patch", `{"ok":true,"result":{"applied":false,"changed_paths":["secret"]}}`}, {"read_file", `{"ok":true,"result":{"path":"/Users/private/secret"}}`}, {"read_file", `{"ok":true,"result":{"path":"../secret"}}`}} {
		if len(observeTool(c.name, c.result).Entries[0].Details) > 0 {
			t.Fatal("unobserved or unsafe metadata accepted")
		}
	}
}
func TestDetailBoundsCloneAndSidecarValidation(t *testing.T) {
	results := []string{}
	for i := 0; i < 50; i++ {
		results = append(results, fmt.Sprintf(`{"url":"https://source-%d.example/private"}`, i))
	}
	trace := observeTool("web_search", `{"ok":true,"result":{"results":[`+strings.Join(results, ",")+`]}}`)
	if len(trace.Entries[0].Details) != maxDetails || !trace.Entries[0].Partial {
		t.Fatal("details not bounded")
	}
	copy := trace.Clone()
	copy.Entries[0].Details[0].Domain = "different.example"
	if trace.Entries[0].Details[0].Domain == "different.example" {
		t.Fatal("clone aliases details")
	}
	copy.Entries[0].Details[0].Path = "/private"
	if copy.ValidDetails() {
		t.Fatal("invalid sidecar metadata accepted")
	}
	for i := 0; i < 160; i++ {
		trace.Entries = append(trace.Entries, Entry{Details: make([]Detail, 13)})
	}
	if trace.ValidDetails() {
		t.Fatal("oversized sidecar metadata accepted")
	}
	command := observeTool("code_run_command", `{"ok":true,"result":{"exit_code":0}}`)
	cloned := command.Clone()
	*cloned.Entries[0].Details[0].ExitCode = 12
	if *command.Entries[0].Details[0].ExitCode != 0 {
		t.Fatal("exit code aliases clone")
	}
}
func TestApprovalWaitingIsObservedAndNeverFinishesAsSuccess(t *testing.T) {
	trace := New(time.Now())
	call := &agent.ToolCall{ID: "command", Name: "code_run_command"}
	trace.Record(agent.Event{Kind: agent.EventToolCalled, ToolCall: call})
	trace.Record(agent.Event{Kind: agent.EventCommandApprovalRequested, Argv: []string{"private-command"}})
	if trace.Entries[0].State != "waiting" {
		t.Fatal("pending command approval not visible")
	}
	trace.Record(agent.Event{Kind: agent.EventCommandApprovalResolved})
	if trace.Entries[0].State != "running" {
		t.Fatal("resolved approval claimed completion")
	}
	trace.Record(agent.Event{Kind: agent.EventCommandApprovalRequested})
	trace.Finish(time.Now(), "stopped", "")
	if trace.Entries[0].State != "stopped" {
		t.Fatal("waiting command became successful")
	}
}

func TestLongRunBoundsObservedDetailBytes(t *testing.T) {
	trace := New(time.Now())
	for i := 0; i < 160; i++ {
		results := []string{}
		for j := 0; j < 12; j++ {
			results = append(results, fmt.Sprintf(`{"url":"https://%s-%d-%d.example/private?token=never-copy"}`, strings.Repeat("domain", 30), i, j))
		}
		call := &agent.ToolCall{ID: fmt.Sprint(i), Name: "web_search"}
		trace.Record(agent.Event{Kind: agent.EventToolFinished, ToolCall: call, ToolResult: `{"results":[` + strings.Join(results, ",") + `]}`})
	}
	partial := false
	for _, entry := range trace.Entries {
		partial = partial || entry.Partial
	}
	if !partial || !trace.ValidDetails() || trace.detailBytes > maxDetailBytes || len(trace.Entries) != 160 {
		t.Fatal("long run escaped display budget", trace.detailBytes)
	}
}
