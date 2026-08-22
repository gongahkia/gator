package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/worktree"
)

func TestExecutorCompletesReviewableFeatureRun(t *testing.T) {
	repository := featureRepository(t)
	patch := "diff --git a/greeting.go b/greeting.go\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/greeting.go\n" +
		"@@ -0,0 +1,5 @@\n" +
		"+package feature\n" +
		"+\n" +
		"+func Greeting(name string) string {\n" +
		"+\treturn \"Hello, \" + name\n" +
		"+}\n" +
		"diff --git a/greeting_test.go b/greeting_test.go\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/greeting_test.go\n" +
		"@@ -0,0 +1,9 @@\n" +
		"+package feature\n" +
		"+\n" +
		"+import \"testing\"\n" +
		"+\n" +
		"+func TestGreeting(t *testing.T) {\n" +
		"+\tif got := Greeting(\"Ada\"); got != \"Hello, Ada\" {\n" +
		"+\t\tt.Fatalf(\"Greeting() = %q\", got)\n" +
		"+\t}\n" +
		"+}\n"
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "patch", Name: "apply_patch", Arguments: objectArguments(t, struct {
			Patch string `json:"patch"`
		}{Patch: patch})}}},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "test", Name: "run_command", Arguments: json.RawMessage(`{"argv":["go","test","./..."]}`)}}},
		{Text: "Added Greeting with its focused test. `go test ./...` passed; the patch is ready for review."},
	}}
	executor := Executor{
		Model: model,
		Now:   func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) },
	}

	outcome, err := executor.Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Add a Greeting feature with a focused test",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "run_feature_001",
		MaxSteps:       8,
		Verification:   [][]string{{"go", "test", "./..."}},
		StateDir:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("execute feature run: %v", err)
	}
	if outcome.Result.FinalText == "" || outcome.Result.Steps != 5 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if !strings.Contains(model.requests[0].System, "Gator, a careful coding agent") {
		t.Fatalf("system prompt was not sent: %q", model.requests[0].System)
	}
	if _, err := os.Stat(filepath.Join(outcome.Worktree.Path, "greeting.go")); err != nil {
		t.Fatalf("feature file missing from worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outcome.StatePath, "events.jsonl")); err != nil {
		t.Fatalf("run journal missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outcome.Worktree.Path, "greeting_test.go")); err != nil {
		t.Fatalf("feature test missing from worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, "greeting.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active checkout changed, stat error = %v", err)
	}
	if !containsEvent(outcome.Events, agent.EventRunFinished) {
		t.Fatalf("events did not contain completed run: %#v", outcome.Events)
	}
}

func TestExecutorRetainsWorktreeWhenEvidenceIsMissing(t *testing.T) {
	repository := featureRepository(t)
	model := &scriptedModel{turns: []agent.Turn{{Text: "Done."}}}
	executor := Executor{Model: model}

	outcome, err := executor.Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Add a feature",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "run_missing_evidence",
		MaxSteps:       2,
		StateDir:       t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected model call") {
		t.Fatalf("execute error = %v", err)
	}
	if outcome.Worktree.Path == "" {
		t.Fatal("failed run did not retain worktree details")
	}
	if !containsEvent(outcome.Events, agent.EventCompletionBlocked) {
		t.Fatalf("events did not record blocked completion: %#v", outcome.Events)
	}
}

func TestExecutorRunsReadOnlyPlanTurnAndSavesThread(t *testing.T) {
	repository := featureRepository(t)
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{Text: "Plan: inspect the feature package, change the relevant implementation, then add a focused test."},
	}}
	stateDirectory := t.TempDir()
	outcome, err := (Executor{Model: model}).Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Plan a focused feature",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "plan-thread-001",
		MaxSteps:       4,
		StateDir:       stateDirectory,
		Mode:           PlanMode,
	})
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	if outcome.ThreadID != "plan-thread-001" || outcome.Result.FinalText == "" {
		t.Fatalf("plan outcome = %#v", outcome)
	}
	if !strings.Contains(model.requests[0].System, "enforced Plan mode") {
		t.Fatalf("plan system prompt = %q", model.requests[0].System)
	}
	delegationAvailable := false
	for _, tool := range model.requests[0].Tools {
		if tool.Name == "apply_patch" || tool.Name == "run_command" {
			t.Fatalf("plan tool surface included %q", tool.Name)
		}
		if tool.Name == "delegate_readonly" {
			delegationAvailable = true
		}
	}
	if !delegationAvailable {
		t.Fatal("plan tool surface omitted read-only delegation")
	}
	if hasTool(model.requests[0].Tools, "delegate_writer") {
		t.Fatalf("plan tool surface included writer delegation: %#v", model.requests[0].Tools)
	}
	current, err := journal.LoadSession(outcome.StatePath)
	if err != nil {
		t.Fatalf("load plan session: %v", err)
	}
	if current.Mode != PlanMode.String() || current.ThreadID != outcome.ThreadID {
		t.Fatalf("plan session = %#v", current)
	}
	thread, err := journal.LoadThread(stateDirectory, repository, outcome.ThreadID)
	if err != nil {
		t.Fatalf("load saved thread: %v", err)
	}
	if thread.HeadStatePath != outcome.StatePath || thread.TurnCount != 1 {
		t.Fatalf("saved thread = %#v", thread)
	}
}

func TestExecutorExposesModelInvocableReadOnlyDelegation(t *testing.T) {
	repository := featureRepository(t)
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "delegate", Name: "delegate_readonly", Arguments: json.RawMessage(`{"tasks":[{"task":"Inspect the feature package and report relevant tests."}]}`)}}},
		{Text: "evidence: feature.go and feature_test.go are the relevant files."},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "The read-only scout found the relevant implementation and tests; the worktree is ready for review."},
	}}
	outcome, err := (Executor{Model: model}).Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Inspect the feature package",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "delegated-scout-001",
		MaxSteps:       6,
		StateDir:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("execute with delegated scout: %v", err)
	}
	if !containsEvent(outcome.Events, agent.EventSubagent) {
		t.Fatalf("delegated scout events were not retained: %#v", outcome.Events)
	}
	if len(model.requests) != 5 {
		t.Fatalf("model requests = %d, want parent/scout/parent sequence", len(model.requests))
	}
	if !hasTool(model.requests[0].Tools, "delegate_readonly") {
		t.Fatalf("primary tool surface omitted delegation: %#v", model.requests[0].Tools)
	}
	for _, tool := range model.requests[1].Tools {
		if tool.Name == "apply_patch" || tool.Name == "run_command" || tool.Name == "delegate_readonly" {
			t.Fatalf("scout received forbidden tool %q", tool.Name)
		}
	}
	if got := model.requests[2].Messages[len(model.requests[2].Messages)-1]; got.Role != agent.RoleTool || got.ToolName != "delegate_readonly" || !strings.Contains(got.Content, "untrusted evidence") {
		t.Fatalf("parent did not receive bounded untrusted scout report: %#v", got)
	}
}

func TestExecutorExposesSandboxedPersistentTerminalOnlyInExecuteMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the PTY dependency reports unsupported on Windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	repository := featureRepository(t)
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "terminal", Name: "terminal_start", Arguments: objectArguments(t, struct {
			Argv []string `json:"argv"`
		}{Argv: []string{sh, "-lc", "sleep 30"}})}}},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "The long-lived task was started and the worktree is ready for review."},
	}}
	outcome, err := (Executor{Model: model}).Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Start a local development task",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "terminal-task-001",
		MaxSteps:       5,
		StateDir:       t.TempDir(),
		Approve: func(context.Context, []string) (tools.CommandDecision, error) {
			return tools.CommandAllowOnce, nil
		},
	})
	if err != nil {
		t.Fatalf("execute terminal task: %v", err)
	}
	if !hasTool(model.requests[0].Tools, "terminal_start") || !hasTool(model.requests[0].Tools, "terminal_read") {
		t.Fatalf("execute tool surface omitted persistent terminal: %#v", model.requests[0].Tools)
	}
	if !containsEvent(outcome.Events, agent.EventTerminal) {
		t.Fatalf("terminal cleanup was not visible in the event stream: %#v", outcome.Events)
	}
	planModel := &scriptedModel{turns: []agent.Turn{{Text: "Plan only."}}}
	_, err = (Executor{Model: planModel}).Execute(context.Background(), Request{
		RepositoryPath: repository, Task: "Plan terminal work", Provider: "test", Model: "test-model", RunID: "terminal-plan-001", MaxSteps: 1, StateDir: t.TempDir(), Mode: PlanMode,
	})
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	if hasTool(planModel.requests[0].Tools, "terminal_start") || hasTool(planModel.requests[0].Tools, "terminal_write") {
		t.Fatalf("plan tool surface included persistent terminal: %#v", planModel.requests[0].Tools)
	}
}

func TestWriterDelegationReturnsOnlyChildDeltaForExplicitParentReview(t *testing.T) {
	repository := featureRepository(t)
	parent, err := worktree.Create(context.Background(), repository, "writer-parent-001")
	if err != nil {
		t.Fatalf("create parent worktree: %v", err)
	}
	parentPatch := "diff --git a/parent.txt b/parent.txt\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/parent.txt\n" +
		"@@ -0,0 +1 @@\n" +
		"+parent snapshot\n"
	if _, err := (tools.ApplyPatch{Root: parent.Root}).Execute(context.Background(), objectArguments(t, struct {
		Patch string `json:"patch"`
	}{Patch: parentPatch})); err != nil {
		t.Fatalf("apply parent snapshot: %v", err)
	}
	writerPatch := "diff --git a/writer.txt b/writer.txt\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/writer.txt\n" +
		"@@ -0,0 +1 @@\n" +
		"+writer delta\n"
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "patch", Name: "apply_patch", Arguments: objectArguments(t, struct {
			Patch string `json:"patch"`
		}{Patch: writerPatch})}}},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "Implemented the independent writer change and inspected its delta."},
	}}
	var events []agent.Event
	writer := newWriterTool(Executor{Model: model}, parent, Request{
		RepositoryPath: repository,
		Task:           "Implement both parent and writer work.",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "writer-parent-001",
		ThreadID:       "writer-parent-001",
		MaxSteps:       6,
		BaseCommit:     parent.BaseCommit,
		StateDir:       t.TempDir(),
		Mode:           ExecuteMode,
	}, tools.NewCommandMemory(nil), fixedScoutClock(), func(event agent.Event) {
		events = append(events, event)
	})
	result, err := writer.Execute(context.Background(), json.RawMessage(`{"task":"Add writer.txt with the delegated behavior."}`))
	if err != nil {
		t.Fatalf("delegate writer: %v", err)
	}
	var payload struct {
		OK     bool                  `json:"ok"`
		Writer delegatedWriterReport `json:"writer"`
		Notice string                `json:"notice"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode writer result: %v\n%s", err, result.Content)
	}
	if !payload.OK || !payload.Writer.Completed || !payload.Writer.PatchAvailable || !payload.Writer.ReviewRequired || payload.Writer.RunID == "" {
		t.Fatalf("writer result = %#v", payload)
	}
	if strings.Contains(payload.Writer.Patch, "parent.txt") || !strings.Contains(payload.Writer.Patch, "writer.txt") {
		t.Fatalf("writer handoff must contain only the child delta: %q", payload.Writer.Patch)
	}
	if !strings.Contains(payload.Notice, "never auto-merges") || len(events) != 2 || events[0].Kind != agent.EventSubagent || !strings.Contains(events[0].Text, "starting isolated writer") {
		t.Fatalf("writer delegation events = %#v; notice = %q", events, payload.Notice)
	}
	if len(model.requests) != 4 || hasTool(model.requests[0].Tools, "delegate_writer") {
		t.Fatalf("writer child model requests = %#v", model.requests)
	}
	if _, err := (tools.ApplyPatch{Root: parent.Root}).Execute(context.Background(), objectArguments(t, struct {
		Patch string `json:"patch"`
	}{Patch: payload.Writer.Patch})); err != nil {
		t.Fatalf("explicitly apply writer delta to parent: %v", err)
	}
	if contents, err := os.ReadFile(filepath.Join(parent.Path, "parent.txt")); err != nil || string(contents) != "parent snapshot\n" {
		t.Fatalf("parent snapshot changed: %q, %v", contents, err)
	}
	if contents, err := os.ReadFile(filepath.Join(parent.Path, "writer.txt")); err != nil || string(contents) != "writer delta\n" {
		t.Fatalf("writer delta was not explicitly transferable: %q, %v", contents, err)
	}
}

func TestExecutorPersistsDocumentAttachmentsForResume(t *testing.T) {
	repository := featureRepository(t)
	stateDirectory := t.TempDir()
	model := &scriptedModel{turns: []agent.Turn{{Text: "Plan ready."}}}
	outcome, err := (Executor{Model: model}).Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Review the attached report",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "attachment-session-001",
		MaxSteps:       2,
		StateDir:       stateDirectory,
		Mode:           PlanMode,
		Attachments:    []agent.Attachment{{Name: "report.pdf", MediaType: "application/pdf", Data: []byte("pdf")}},
	})
	if err != nil {
		t.Fatalf("execute plan: %v", err)
	}
	if len(model.requests) != 1 || len(model.requests[0].Messages) != 1 || len(model.requests[0].Messages[0].Attachments) != 1 {
		t.Fatalf("model requests = %#v", model.requests)
	}
	session, err := journal.LoadSession(outcome.StatePath)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if len(session.Messages) < 1 || len(session.Messages[0].Attachments) != 0 {
		t.Fatalf("persisted session attachments = %#v", session.Messages)
	}
	if len(session.AttachmentManifest) != 1 || session.AttachmentManifest[0].Name != "report.pdf" || session.AttachmentManifest[0].MediaType != "application/pdf" || session.AttachmentManifest[0].Bytes != len("pdf") || session.AttachmentManifest[0].SHA256 == "" {
		t.Fatalf("session attachment manifest = %#v", session.AttachmentManifest)
	}
}

func TestExecutorResumeAdvancesThread(t *testing.T) {
	repository := featureRepository(t)
	stateDirectory := t.TempDir()
	firstModel := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "The worktree is ready for review."},
	}}
	first, err := (Executor{Model: firstModel}).Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Inspect the fixture",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "thread-resume-001",
		MaxSteps:       4,
		StateDir:       stateDirectory,
	})
	if err != nil {
		t.Fatalf("execute first turn: %v", err)
	}
	previous, err := journal.LoadSession(first.StatePath)
	if err != nil {
		t.Fatalf("load first session: %v", err)
	}
	secondModel := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "The follow-up is ready for review."},
	}}
	second, err := (Executor{Model: secondModel}).Resume(context.Background(), previous, first.StatePath, "Review the previous result.", Request{StateDir: stateDirectory})
	if err != nil {
		t.Fatalf("resume thread: %v", err)
	}
	if second.ThreadID != first.ThreadID || second.Worktree.Path != first.Worktree.Path {
		t.Fatalf("resumed outcome = %#v", second)
	}
	thread, err := journal.LoadThread(stateDirectory, repository, first.ThreadID)
	if err != nil {
		t.Fatalf("load resumed thread: %v", err)
	}
	if thread.TurnCount != 2 || thread.HeadStatePath != second.StatePath {
		t.Fatalf("resumed thread = %#v", thread)
	}
}

func TestExecutorForkRestoresSnapshotIntoNewWorktree(t *testing.T) {
	repository := featureRepository(t)
	stateDirectory := t.TempDir()
	patch := "diff --git a/branch.txt b/branch.txt\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/branch.txt\n" +
		"@@ -0,0 +1 @@\n" +
		"+original branch state\n"
	firstModel := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "patch", Name: "apply_patch", Arguments: objectArguments(t, struct {
			Patch string `json:"patch"`
		}{Patch: patch})}}},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "The original branch is ready."},
	}}
	first, err := (Executor{Model: firstModel}).Execute(context.Background(), Request{
		RepositoryPath: repository, Task: "Create branch state", Provider: "test", Model: "test-model",
		RunID: "fork-source-001", MaxSteps: 6, StateDir: stateDirectory,
	})
	if err != nil {
		t.Fatalf("execute source branch: %v", err)
	}
	if snapshot, err := journal.LoadSnapshot(first.StatePath); err != nil || !strings.Contains(string(snapshot), "branch.txt") {
		t.Fatalf("source snapshot = %q, %v", snapshot, err)
	}
	previous, err := journal.LoadSession(first.StatePath)
	if err != nil {
		t.Fatalf("load source session: %v", err)
	}
	forkModel := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "The fork is ready."},
	}}
	fork, err := (Executor{Model: forkModel}).Fork(context.Background(), previous, first.StatePath, "Try an alternate implementation.", Request{StateDir: stateDirectory})
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if fork.ThreadID == first.ThreadID || fork.Worktree.Path == first.Worktree.Path {
		t.Fatalf("fork reused source branch: %#v", fork)
	}
	if contents, err := os.ReadFile(filepath.Join(fork.Worktree.Path, "branch.txt")); err != nil || string(contents) != "original branch state\n" {
		t.Fatalf("fork did not restore snapshot: %q, %v", contents, err)
	}
	forkSession, err := journal.LoadSession(fork.StatePath)
	if err != nil {
		t.Fatalf("load fork session: %v", err)
	}
	if forkSession.ForkedFromStatePath != first.StatePath || forkSession.ParentStatePath != "" {
		t.Fatalf("fork session = %#v", forkSession)
	}
	thread, err := journal.LoadThread(stateDirectory, repository, fork.ThreadID)
	if err != nil {
		t.Fatalf("load fork thread: %v", err)
	}
	if thread.ForkedFrom != first.StatePath {
		t.Fatalf("fork thread = %#v", thread)
	}
}

func TestValidateVerification(t *testing.T) {
	if err := validateVerification([][]string{{}}); err == nil {
		t.Fatal("empty verification command was accepted")
	}
	if err := validateVerification([][]string{{"go", "test", "./..."}}); err != nil {
		t.Fatalf("valid verification command: %v", err)
	}
}

func TestCompletionCheckRequiresVerifierEvenAfterOtherCommands(t *testing.T) {
	check := completionCheck([][]string{{"go", "test", "./..."}})
	messages := []agent.Message{
		{Role: agent.RoleUser, Content: "task"},
		{Role: agent.RoleTool, ToolName: "git_status", Content: `{"ok":true}`},
		{Role: agent.RoleTool, ToolName: "git_diff", Content: `{"ok":true}`},
		{Role: agent.RoleTool, ToolName: "run_command", Content: `{"ok":true,"result":{"argv":["echo","hi"],"exit_code":0}}`},
	}
	err := check(messages)
	if err == nil || !strings.Contains(err.Error(), "go test ./...") {
		t.Fatalf("completion check = %v, want missing verifier", err)
	}
	messages = append(messages, agent.Message{Role: agent.RoleTool, ToolName: "run_command", Content: `{"ok":true,"result":{"argv":["go","test","./..."],"exit_code":0}}`})
	if err := check(messages); err != nil {
		t.Fatalf("completion check after verifier: %v", err)
	}
}

func TestExecutorLoadsRootAgentInstructions(t *testing.T) {
	repository := featureRepository(t)
	writeFile(t, repository, "AGENTS.md", "Always name the feature tests clearly.\n")
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "No changes were needed."},
	}}

	_, err := (Executor{Model: model}).Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Inspect the fixture",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "run_instructions_001",
		MaxSteps:       4,
		StateDir:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(model.requests[0].System, "Always name the feature tests clearly.") {
		t.Fatalf("system prompt did not include AGENTS.md: %q", model.requests[0].System)
	}
}

func TestExecutorResumesWithFreshEvidence(t *testing.T) {
	repository := featureRepository(t)
	retained, err := worktree.Create(context.Background(), repository, "run_original_001")
	if err != nil {
		t.Fatalf("create retained worktree: %v", err)
	}
	previous := journal.Session{
		Version:      2,
		Repository:   repository,
		WorktreePath: retained.Path,
		Provider:     "test",
		Model:        "test-model",
		Task:         "Add a feature",
		MaxSteps:     4,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "Add a feature"},
			{Role: agent.RoleTool, ToolName: "git_status", ToolCallID: "old-status", Content: `{"ok":true}`},
			{Role: agent.RoleTool, ToolName: "git_diff", ToolCallID: "old-diff", Content: `{"ok":true}`},
		},
	}
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "The retained patch is ready for review."},
	}}
	stateDirectory := t.TempDir()
	executor := Executor{Model: model}

	outcome, err := executor.Resume(context.Background(), previous, "/state/original", "Review the existing patch and finish.", Request{StateDir: stateDirectory})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if outcome.Worktree.Path != retained.Path || outcome.Result.FinalText == "" {
		t.Fatalf("resume outcome = %#v", outcome)
	}
	if got := model.requests[0].Messages[len(model.requests[0].Messages)-1]; got.Role != agent.RoleUser || !strings.Contains(got.Content, "Review the existing patch") {
		t.Fatalf("resume history = %#v", model.requests[0].Messages)
	}
	current, err := journal.LoadSession(outcome.StatePath)
	if err != nil {
		t.Fatalf("load resumed session: %v", err)
	}
	if current.ParentStatePath != "/state/original" {
		t.Fatalf("resumed session parent = %q", current.ParentStatePath)
	}
}

type scriptedModel struct {
	turns    []agent.Turn
	requests []agent.TurnRequest
}

func (m *scriptedModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.requests = append(m.requests, request)
	if len(m.turns) == 0 {
		return agent.Turn{}, errors.New("unexpected model call")
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]
	return turn, nil
}

func objectArguments(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode arguments: %v", err)
	}
	return encoded
}

func containsEvent(events []agent.Event, kind agent.EventKind) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func hasTool(definitions []agent.ToolDefinition, name string) bool {
	for _, definition := range definitions {
		if definition.Name == name {
			return true
		}
	}
	return false
}

func featureRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "feature-repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init", "--quiet")
	writeFile(t, repository, "go.mod", "module example.com/feature\n\ngo 1.25.0\n")
	writeFile(t, repository, "README.md", "# Feature Fixture\n")
	runGit(t, repository, "add", "go.mod", "README.md")
	runGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "fixture")
	return repository
}

func writeFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}
