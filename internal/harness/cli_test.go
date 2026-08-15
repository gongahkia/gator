package harness

import (
	"context"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

func TestCodexArgumentsUseWorkspaceWriteAndEphemeralSessions(t *testing.T) {
	cli := CLI{Provider: Codex}
	arguments, cleanup, err := cli.arguments(Request{Root: "/worktree", Task: "Add a feature", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}
	defer cleanup()
	joined := strings.Join(arguments, " ")
	for _, expected := range []string{"exec", "--ephemeral", "--json", "--sandbox workspace-write", "--approve-for-me", "-C /worktree", "--model gpt-test"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("arguments = %q, missing %q", joined, expected)
		}
	}
	if strings.Contains(joined, "dangerously-bypass") {
		t.Fatalf("arguments weakened Codex sandboxing: %q", joined)
	}
}

func TestCopilotArgumentsDenyGitPush(t *testing.T) {
	cli := CLI{Provider: Copilot}
	arguments, cleanup, err := cli.arguments(Request{Root: "/worktree", Task: "Add a feature"})
	if err != nil {
		t.Fatalf("arguments: %v", err)
	}
	defer cleanup()
	if !strings.Contains(strings.Join(arguments, " "), "--deny-tool=shell(git push)") {
		t.Fatalf("arguments do not deny git push: %#v", arguments)
	}
}

func TestCLIRunUsesVendorOutputWithoutPersistingCredentials(t *testing.T) {
	var events []agent.Event
	cli := CLI{
		Provider:    Claude,
		CommandPath: "claude",
		Execute: func(_ context.Context, command string, arguments []string, directory string, onLine func(string, bool)) (processResult, error) {
			if command != "claude" || directory != "/worktree" || !strings.Contains(strings.Join(arguments, " "), "--bare") {
				t.Fatalf("command = %q, arguments = %#v, directory = %q", command, arguments, directory)
			}
			line := `{"type":"result","result":"Implemented the feature."}`
			onLine(line, false)
			return processResult{Stdout: line + "\n"}, nil
		},
	}
	result, err := cli.Run(context.Background(), Request{Root: "/worktree", Task: "Add a feature", OnEvent: func(event agent.Event) { events = append(events, event) }})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.FinalText != "Implemented the feature." || result.Output == "" {
		t.Fatalf("result = %#v", result)
	}
	if len(events) < 2 || events[0].Kind != agent.EventHarnessStarted || events[len(events)-1].Kind != agent.EventHarnessFinished {
		t.Fatalf("events = %#v", events)
	}
}

func TestCLIRunReportsNonzeroExitWithBoundedOutput(t *testing.T) {
	cli := CLI{
		Provider:    Cursor,
		CommandPath: "cursor-agent",
		Execute: func(_ context.Context, _ string, _ []string, _ string, _ func(string, bool)) (processResult, error) {
			return processResult{Stderr: "auth failed", Code: 17}, nil
		},
	}
	_, err := cli.Run(context.Background(), Request{Root: "/worktree", Task: "Add a feature"})
	if err == nil || !strings.Contains(err.Error(), "status 17") || !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("run error = %v", err)
	}
}

func TestStreamedTextExtractsCursorAssistantDelta(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"Implemented "},{"type":"text","text":"the change."}]}}`
	if got := streamedText(Cursor, line, false); got != "Implemented the change." {
		t.Fatalf("streamed text = %q", got)
	}
}
