package workapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func dialogueConfig(t *testing.T) (Config, worksession.Store) {
	t.Helper()
	config, store := requestConfig(t, func(_, _, _ string, _ RunOptions) RunResult { return RunResult{FinalText: "A retained result"} })
	config.ManagedSessions = true
	config.RouteConversation = func(context.Context, workrun.ConversationRequest, ConversationInput) (workrun.ConversationDecision, error) {
		return workrun.ConversationDecision{Kind: "reply", Text: "You can keep talking."}, nil
	}
	config.BindConversationTask = func(parent, turn, source string, revision uint64) (worksession.DialogueTurn, worksession.Conversation, bool, error) {
		return store.BindDialogueTask(parent, turn, source, revision, time.Now())
	}
	config.ValidateUploads = func(string, []string, []string) error { return nil }
	return config, store
}
func dialogueSubmit(t *testing.T, m Model, id, text string, files ...string) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: id, Context: "conversation:" + m.conversation, Text: text, Files: files})
	m = next.(Model)
	if m.bridgeUploadResult.Error != "" {
		t.Fatal(m.bridgeUploadResult.Error)
	}
	return m, cmd
}
func routeDialogue(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("routing was not scheduled")
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

func TestDialogueOrdinaryComposerAdmissionRetainsInputsAndDoesNotWaitForWork(t *testing.T) {
	config, store := dialogueConfig(t)
	calls := 0
	config.RouteConversation = func(_ context.Context, request workrun.ConversationRequest, input ConversationInput) (workrun.ConversationDecision, error) {
		calls++
		if len(request.Recent) != 1 || request.Recent[0].Role != "user" || input.Source != "/captured" || input.RunOptions.MaxSteps != 12 {
			t.Fatalf("lost captured context: %+v %+v", request, input)
		}
		return workrun.ConversationDecision{Kind: "reply", Text: "You can keep talking."}, nil
	}
	m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	// The durable parent is authoritative; this test captures the same source.
	config.CurrentFolder = "/captured"
	chat, err := store.CreateChat(config.CurrentFolder, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	config.StartConversationID = chat.ID
	m = New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	m.running = true
	m.options.MaxSteps = 12
	m.messages = []message{{role: "You", text: "Compare venues", createdAt: time.Now()}}
	file := strings.Repeat("a", 32) + "/brief.txt"
	m.options.Attachments = []string{"notes.txt"}
	m, cmd := dialogueSubmit(t, m, "hello-message-0001", "Hello", file)
	if len(m.queue) != 0 || !m.running || cmd == nil || len(m.dialogueTurns) != 1 || m.dialogueTurns[0].State != "routing" {
		t.Fatal("message queued behind work or was not durably admitted")
	}
	turn, err := store.ReadDialogue(m.conversation, m.dialogueTurns[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var envelope ConversationInput
	if json.Unmarshal(turn.Options, &envelope) != nil || len(envelope.RunOptions.Uploads) != 1 || len(envelope.RunOptions.Attachments) != 1 || len(m.options.Attachments) != 0 {
		t.Fatal("captured input was lost or leaked into another submission")
	}
	m.options.MaxSteps = 24
	m = routeDialogue(t, m, cmd)
	if !m.running || len(m.queue) != 0 || m.dialogueTurns[0].Response != "You can keep talking." {
		t.Fatal("reply changed execution ownership")
	}
	m, retry := dialogueSubmit(t, m, "hello-message-0001", "Hello", file)
	if retry != nil || calls != 1 || len(m.dialogueTurns) != 1 {
		t.Fatal("acknowledgement retry routed or admitted twice")
	}
	next, _ := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "hello-message-0001", Context: "conversation:" + m.conversation, Text: "Different"})
	if next.(Model).bridgeUploadResult.Error == "" {
		t.Fatal("reused identity accepted different text")
	}
	config.StartConversationID = m.conversation
	restored := New(config)
	if restored.dialogueTurns[0].Response != "You can keep talking." || restored.dialogueRouting != "" || len(restored.dialogueQueue) != 0 {
		t.Fatal("restoration lost the answer or replayed routing")
	}
}

func TestDialogueTaskLaunchHasIndependentDurableAdmissionAndNoRetryReplay(t *testing.T) {
	config, store := dialogueConfig(t)
	config.RouteConversation = func(context.Context, workrun.ConversationRequest, ConversationInput) (workrun.ConversationDecision, error) {
		return workrun.ConversationDecision{Kind: "task"}, nil
	}
	m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	m.running = true
	m, cmd := dialogueSubmit(t, m, "independent-0001", "Read this brief", strings.Repeat("b", 32)+"/brief.txt")
	m = routeDialogue(t, m, cmd)
	if m.dialogueDispatch == nil || m.dialogueTurns[0].Response != "" {
		t.Fatal("missing dispatch or claimed work started before acceptance")
	}
	dispatch := *m.dialogueDispatch
	if dispatch.ChildID == m.conversation {
		t.Fatal("independent task reused parent owner")
	}
	childConfig := config
	childConfig.StartConversationID = dispatch.ChildID
	child := New(childConfig)
	if !child.background {
		t.Fatal("child remembered itself before being focused")
	}
	wrong, _ := child.BootstrapDialogue("wrong-parent", dispatch.TurnID)
	if wrong.(Model).dialogueBootstrapResult.Error == "" {
		t.Fatal("forged origin was accepted")
	}
	next, run := child.BootstrapDialogue(m.conversation, dispatch.TurnID)
	child = next.(Model)
	if child.dialogueBootstrapResult.Error != "" || run == nil || len(child.requests.Requests) != 1 {
		t.Fatal("child admission failed", child.dialogueBootstrapResult)
	}
	var options RunOptions
	if json.Unmarshal(child.requests.Requests[0].Options, &options) != nil || options.DialogueParent != m.conversation || options.DialogueTurn != dispatch.TurnID || len(options.Uploads) != 1 {
		t.Fatal("child lost origin/inputs")
	}
	if journal, err := store.LoadRequests(m.conversation); err != nil || len(journal.Requests) != 0 {
		t.Fatal("child changed parent work journal", err)
	}
	next, retry := child.BootstrapDialogue(m.conversation, dispatch.TurnID)
	child = next.(Model)
	if retry != nil || child.dialogueBootstrapResult.Error != "" {
		t.Fatal("duplicate bootstrap dispatched again")
	}
	next, _ = m.FinishDialogueDispatch(*child.dialogueBootstrapResult)
	m = next.(Model)
	if m.dialogueTurns[0].State != "delegated" || m.dialogueDispatch != nil {
		t.Fatal("accepted child was not acknowledged")
	}
	next, _ = child.Update(run())
	child = next.(Model)
	if state := child.dialogueTaskState(); state == nil || state.State != "completed" || state.Summary != "A retained result" {
		t.Fatalf("result not attached to original task: %+v", state)
	}
	restored := New(childConfig)
	next, retry = restored.BootstrapDialogue(m.conversation, dispatch.TurnID)
	if retry != nil || next.(Model).dialogueBootstrapResult.Error != "" {
		t.Fatal("restart replayed a completed task")
	}
}

func TestDialogueRevalidatesTargetsAndRetainsUnconfirmedMessagesWithoutReplay(t *testing.T) {
	config, _ := dialogueConfig(t)
	stateDir := t.TempDir()
	store, err := worksession.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	config.SessionStore = &store
	config.CreateConversation = func(source string) (worksession.Conversation, error) { return store.CreateChat(source, time.Now()) }
	config.LoadConversation = func(id string) (ConversationState, error) {
		c, err := store.Load(id)
		return ConversationState{Title: c.Title, SourcePath: c.SourcePath}, err
	}
	config.RouteConversation = func(_ context.Context, r workrun.ConversationRequest, _ ConversationInput) (workrun.ConversationDecision, error) {
		return workrun.ConversationDecision{Kind: "stop", Target: r.Tasks[0].ID}, nil
	}
	m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	release, err := m.admitRun("Original task", m.submissionOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	m.running = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.cancel = cancel
	m, cmd := dialogueSubmit(t, m, "stop-original-0001", "Stop that task")
	message := cmd()
	m.activeRequest = "another-task"
	next, _ := m.Update(message)
	m = next.(Model)
	if ctx.Err() != nil || m.dialogueTurns[0].State != "failed" {
		t.Fatal("changed target stopped unrelated work")
	}
	m, cmd = dialogueSubmit(t, m, "unconfirmed-message", "Hello again")
	if cmd == nil {
		t.Fatal("message not scheduled")
	}
	config.StartConversationID = m.conversation
	restored := New(config)
	if len(restored.dialogueQueue) != 0 || restored.dialogueDispatch != nil || restored.dialogueRouting != "" {
		t.Fatal("restored unconfirmed message was replayed")
	}
	// A damaged retained identity is an error, not permission for another Work admission.
	turn := m.dialogueTurns[len(m.dialogueTurns)-1]
	path := filepath.Join(stateDir, "gator", "conversations", m.conversation, "dialogue", fmt.Sprintf("%020d-%s.json", turn.Sequence, turn.ID))
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	next, _ = m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "unconfirmed-message", Context: "conversation:" + m.conversation, Text: "Hello again"})
	if next.(Model).bridgeUploadResult.Error == "" {
		t.Fatal("corrupt admission was silently retried")
	}
}

type waitingDialogueModel struct{}

func (waitingDialogueModel) Complete(ctx context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	<-ctx.Done()
	return agent.Turn{}, ctx.Err()
}

func TestDialogueRouteCancellationDoesNotCancelWork(t *testing.T) {
	config, _ := dialogueConfig(t)
	config.RouteConversation = func(ctx context.Context, _ workrun.ConversationRequest, _ ConversationInput) (workrun.ConversationDecision, error) {
		<-ctx.Done()
		return workrun.ConversationDecision{}, ctx.Err()
	}
	m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	m.running = true
	workCtx, workCancel := context.WithCancel(context.Background())
	defer workCancel()
	m.cancel = workCancel
	m, cmd := dialogueSubmit(t, m, "cancel-router-0001", "Hello")
	m.dialogueCancel()
	value := cmd().(conversationRouted)
	if !errors.Is(value.Err, context.Canceled) || workCtx.Err() != nil {
		t.Fatal("conversation routing cancellation leaked into execution")
	}
	// Exercise a real workrun operation: a routing failure cannot grant approval
	// or redirect the independent operation's control channel.
	service := workrun.Service{Executor: workrun.Executor{Model: waitingDialogueModel{}, StateDir: t.TempDir()}}
	operation := service.Start(context.Background(), workrun.Request{SourcePath: t.TempDir(), Objective: "Compare", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	defer func() {
		operation.Cancel()
		select {
		case <-operation.Done:
		case <-time.After(5 * time.Second):
			t.Error("operation did not finish cancellation")
		}
	}()
	m.operation = operation
	next, _ := m.Update(value)
	if next.(Model).operation != operation || next.(Model).dialogueTurns[0].State != "failed" {
		t.Fatal("failed response took ownership of Work")
	}
}
