package workapp

import (
	"context"
	"encoding/json"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/workrun"
)

func TestDialogueReactionPersistsWithoutModelWorkOrDuplicateAdmission(t *testing.T) {
	config, store := dialogueConfig(t)
	calls := 0
	config.RouteConversation = func(context.Context, workrun.ConversationRequest, ConversationInput) (workrun.ConversationDecision, error) {
		calls++
		return workrun.ConversationDecision{Kind: "reply", Text: "ordinary"}, nil
	}
	m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	before, _ := json.Marshal(m.requests)
	m, cmd := dialogueSubmit(t, m, "reaction-message-0001", "Thanks!")
	m = routeDialogue(t, m, cmd)
	turn := m.dialogueTurns[0]
	reaction := m.BridgeState().Dialogue[0].Reaction
	if calls != 0 || turn.State != "answered" || turn.Response != "You're welcome." || reaction == nil || reaction.TurnID != turn.ID || reaction.ConversationID != m.conversation || reaction.Revision != turn.Revision {
		t.Fatalf("unconfirmed or unbound reaction: %+v %+v calls=%d", turn, reaction, calls)
	}
	after, _ := json.Marshal(m.requests)
	if string(before) != string(after) || m.running || m.dialogueDispatch != nil || len(m.messages) != 0 {
		t.Fatal("reaction admitted Work or changed its journal")
	}
	m, cmd = dialogueSubmit(t, m, "reaction-message-0001", "Thanks!")
	if cmd != nil || len(m.dialogueTurns) != 1 || calls != 0 {
		t.Fatal("retry duplicated reaction")
	}
	config.StartConversationID = m.conversation
	reopened := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	if got := reopened.BridgeState().Dialogue[0]; got.Reaction == nil || *got.Reaction != *reaction || got.Response != turn.Response {
		t.Fatalf("lost durable outcome: %+v", got)
	}
	page, _, err := store.DialoguePage(m.conversation, 0, 1)
	if err != nil || len(page) != 1 || page[0].Version != 1 || page[0].Response == "" {
		t.Fatal("broke existing stored dialogue contract")
	}
	if _, err := store.UpdateDialogue(turn, turn.Revision); err == nil {
		t.Fatal("terminal reaction could be replayed")
	}
	other, _ := store.CreateChat(t.TempDir(), time.Now())
	config.StartConversationID = other.ID
	if len(New(config).BridgeState().Dialogue) != 0 {
		t.Fatal("reaction crossed conversations")
	}
}
func TestDialogueReactionExclusionsUseOrdinaryRouterAndKeepApprovalExact(t *testing.T) {
	for _, text := range []string{"Approved", "Okay, send it", "Thanks, now stop that task", "Haha, can you fix the bug?", "Thanks"} {
		t.Run(text, func(t *testing.T) {
			config, _ := dialogueConfig(t)
			calls := 0
			config.RouteConversation = func(context.Context, workrun.ConversationRequest, ConversationInput) (workrun.ConversationDecision, error) {
				calls++
				return workrun.ConversationDecision{Kind: "reply", Text: "Please use the exact review controls."}, nil
			}
			m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
			m.running = true
			m.interaction = &workrun.Interaction{ID: 23}
			token := m.bridgeApprovalToken()
			approval := m.interaction
			m, cmd := dialogueSubmit(t, m, "nonreaction-message-0001", text)
			m = routeDialogue(t, m, cmd)
			if calls != 1 || m.BridgeState().Dialogue[0].Reaction != nil || m.interaction != approval || m.bridgeApprovalToken() != token || !m.running || m.dialogueDispatch != nil {
				t.Fatal("ordinary routing/approval boundary changed")
			}
		})
	}
	for _, plan := range []bool{false, true} {
		config, _ := dialogueConfig(t)
		calls := 0
		config.RouteConversation = func(context.Context, workrun.ConversationRequest, ConversationInput) (workrun.ConversationDecision, error) {
			calls++
			return workrun.ConversationDecision{Kind: "reply", Text: "Inspect those inputs."}, nil
		}
		m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
		m.running = true
		m.options.PlanFirst = plan
		// Captured attachments decline the fast path even when the new text is thanks.
		m.options.Attachments = []string{"brief.txt"}
		m, cmd := dialogueSubmit(t, m, "attached-message-0001", "Thanks")
		m = routeDialogue(t, m, cmd)
		if calls != 1 || m.BridgeState().Dialogue[0].Reaction != nil {
			t.Fatal("attachment-bearing text became reaction-only")
		}
	}
}
func TestDialogueReactionRevalidatesReviewAndRetainedRevisionBeforeCompletion(t *testing.T) {
	config, store := dialogueConfig(t)
	m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	m, cmd := dialogueSubmit(t, m, "stale-reaction-0001", "Thanks")
	event := cmd()
	turn := m.dialogueTurns[0]
	turn.State = "failed"
	turn.Issue = "Another owner confirmed a failure."
	turn.UpdatedAt = time.Now().UTC()
	if _, err := store.UpdateDialogue(turn, turn.Revision); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(event)
	m = next.(Model)
	if m.BridgeState().Dialogue[0].Reaction != nil || m.dialogueTurns[0].State != "failed" {
		t.Fatal("stale completion overwrote canonical state")
	}
	m.dialogueIssue = ""
	m, cmd = dialogueSubmit(t, m, "changed-review-0001", "Appreciate it")
	event = cmd()
	m.interaction = &workrun.Interaction{ID: 24}
	next, _ = m.Update(event)
	m = next.(Model)
	if m.BridgeState().Dialogue[1].Reaction != nil || m.dialogueTurns[1].State != "failed" {
		t.Fatal("new review was hidden by a reaction")
	}
}
func TestDialogueReactionFailureAndUnconfirmedRoutesStayUnacknowledged(t *testing.T) {
	config, store := dialogueConfig(t)
	m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	m, cmd := dialogueSubmit(t, m, "unconfirmed-ack-0001", "Thanks")
	config.StartConversationID = m.conversation
	if New(config).BridgeState().Dialogue[0].Reaction != nil {
		t.Fatal("routing was represented as complete after restart")
	}
	event := cmd().(conversationRouted)
	event.Err = fmt.Errorf("inert route failure")
	next, _ := m.Update(event)
	m = next.(Model)
	if m.BridgeState().Dialogue[0].Reaction != nil || m.dialogueTurns[0].State != "failed" {
		t.Fatal("failed route manufactured an acknowledgement")
	}
	_ = store
}

func TestDialogueReactionOlderPageRetainsExactOutcomes(t *testing.T) {
	config, _ := dialogueConfig(t)
	calls := 0
	config.RouteConversation = func(context.Context, workrun.ConversationRequest, ConversationInput) (workrun.ConversationDecision, error) {
		calls++
		return workrun.ConversationDecision{Kind: "reply", Text: "ordinary"}, nil
	}
	m := New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	var first string
	for i := 0; i < 104; i++ {
		var cmd tea.Cmd
		m, cmd = dialogueSubmit(t, m, fmt.Sprintf("older-reaction-%04d", i), "Thanks")
		m = routeDialogue(t, m, cmd)
		if i == 0 {
			first = m.dialogueTurns[0].ID
		}
	}
	if len(m.dialogueTurns) != 100 || !m.dialogueMore {
		t.Fatal("fixture did not page")
	}
	config.StartConversationID = m.conversation
	m = New(config).ConfigureDialogueHost(DialogueHost{CanStart: true})
	m = m.LoadOlderDialogue(m.dialogueTurns[0].Sequence)
	if len(m.dialogueTurns) != 104 || m.dialogueTurns[0].ID != first || m.BridgeState().Dialogue[0].Reaction == nil || calls != 0 {
		t.Fatal("older page lost reaction or replayed inference")
	}
}
