package workapp

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/actionlog"
)

func recoveryFixture(owner, id string) actionlog.Entry {
	return actionlog.Entry{ID: id, ConversationID: owner, RunID: "run-one", StartedAt: time.Now(), Action: action.Record{Proposal: action.Proposal{ID: "action-one", ConnectorID: "google", Operation: "gmail_send", CorrelationID: id, Preview: `{"to":"one@example.com","subject":"Hello","body":"private"}`}, Status: action.Unknown, Error: "Original outcome unconfirmed"}}
}
func TestActionRecoveryScopesOwnerAndDoesNotDuplicateSealedReceipts(t *testing.T) {
	entries := []actionlog.Entry{recoveryFixture("chat-one", "send-one"), recoveryFixture("chat-other", "foreign"), recoveryFixture("chat-one", "send-two")}
	var readErr error
	m := New(Config{ReadActionRecoveries: func(string) ([]actionlog.Entry, error) { return entries, readErr }})
	m.conversation = "chat-one"
	sealed := projectRecoveredAction(entries[0])
	m.messages = []message{{bundle: &BundleSummary{RunID: "run-one", Actions: []ActionReceipt{sealed}}}}
	m.refreshActionRecoveries()
	state := m.BridgeState()
	if len(state.ActionRecoveries) != 1 || state.ActionRecoveries[0].ID != "send-two" || state.ActionRecoveries[0].Receipt.Recipient != "one@example.com" {
		t.Fatal("owner or manifest deduplication wrong", state.ActionRecoveries)
	}
	readErr = errors.New("private details in another chat")
	m.refreshActionRecoveries()
	if len(m.actionRecoveries) != 1 || m.actionRecoveryIssue == "" {
		t.Fatal("transient error lost saved same-chat receipt")
	}
	m.conversation = "chat-other"
	m.refreshActionRecoveries()
	if len(m.actionRecoveries) != 0 || m.actionRecoveryIssue == "" {
		t.Fatal("cached request crossed chat boundary")
	}
	entries = nil
	readErr = nil
	m.refreshActionRecoveries()
	if len(m.actionRecoveries) != 0 || m.actionRecoveryIssue != "" {
		t.Fatal("removed receipt retained after refresh")
	}
}
func TestActionRecoveryKeepsNewestBoundedReceiptsAndDistinctDuplicateAttempts(t *testing.T) {
	var entries []actionlog.Entry
	for i := 0; i < 40; i++ {
		entries = append(entries, recoveryFixture("chat", fmt.Sprint(i)))
	}
	m := New(Config{ReadActionRecoveries: func(string) ([]actionlog.Entry, error) { return entries, nil }})
	m.conversation = "chat"
	m.refreshActionRecoveries()
	if len(m.actionRecoveries) != 32 || !m.actionRecoveryTruncated || m.actionRecoveries[0].ID != "39" {
		t.Fatal("recovery list not bounded to newest")
	}
	// Identical non-email proposals still represent separate approved calls.
	entries = entries[:2]
	for i := range entries {
		entries[i].Action.Proposal.Operation = "publish"
		entries[i].Action.Proposal.CorrelationID = ""
	}
	m.messages = []message{{bundle: &BundleSummary{RunID: "run-one", Actions: []ActionReceipt{projectRecoveredAction(entries[0])}}}}
	m.refreshActionRecoveries()
	if len(m.actionRecoveries) != 1 {
		t.Fatal("one sealed request hid two actual dispatches")
	}
	entries[1].Action.Status = action.Executed
	entries[1].Action.Error = ""
	entries[1].Action.ReceiptID = "provider-receipt"
	m.refreshActionRecoveries()
	if len(m.actionRecoveries) != 1 || m.actionRecoveries[0].Receipt.ReceiptID != "provider-receipt" {
		t.Fatal("newer provider evidence was hidden by an uncertain sealed outcome")
	}
}
