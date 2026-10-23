package workapp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func receiptModel(t *testing.T) (Model, ReceiptAction, *int, *ReceiptRequest) {
	t.Helper()
	calls, got := new(int), new(ReceiptRequest)
	m := New(Config{CheckReceipt: func(ctx context.Context, request ReceiptRequest) (ReceiptCheck, error) {
		*calls++
		*got = request
		if _, ok := ctx.Deadline(); !ok {
			t.Error("receipt read has no deadline")
		}
		return ReceiptCheck{Status: "found", ProviderID: "message-one"}, nil
	}})
	m.conversation = "work-chat"
	a := ReceiptAction{ConversationID: m.conversation, RunID: "work-one", ActionID: "action-" + strings.Repeat("a", 16), CorrelationID: strings.Repeat("b", 32), ManifestSHA256: strings.Repeat("c", 64), Action: "check_sent", RequestID: "receipt-1"}
	bundle := BundleSummary{Path: "owned-bundle", RunID: a.RunID, ManifestSHA256: a.ManifestSHA256, Actions: []ActionReceipt{
		{ID: a.ActionID, CorrelationID: strings.Repeat("d", 32), Operation: "gmail_send", Status: "unknown"},
		{ID: a.ActionID, CorrelationID: a.CorrelationID, Operation: "gmail_send", Status: "unknown"},
	}}
	m.messages = []message{{role: "Result", bundle: &bundle}}
	return m, a, calls, got
}

func TestReceiptInspectionBindsChatVersionAndExactSend(t *testing.T) {
	for _, mutate := range []func(*ReceiptAction){
		func(a *ReceiptAction) { a.ConversationID = "other-chat" },
		func(a *ReceiptAction) { a.RunID = "other-run" },
		func(a *ReceiptAction) { a.ManifestSHA256 = strings.Repeat("e", 64) },
		func(a *ReceiptAction) { a.ActionID = "action-other" },
		func(a *ReceiptAction) { a.CorrelationID = strings.Repeat("e", 32) },
		func(a *ReceiptAction) { a.Action = "resend" },
		func(a *ReceiptAction) { a.RequestID = "../request" },
	} {
		m, a, calls, _ := receiptModel(t)
		mutate(&a)
		next, cmd := m.ApplyReceiptAction(a)
		result := next.(Model).BridgeState().ReceiptResult
		if cmd != nil || *calls != 0 || result == nil || result.Error == "" || result.RequestID != a.RequestID {
			t.Fatal("unowned or stale receipt accepted", a, result)
		}
	}
	m, a, calls, got := receiptModel(t)
	next, cmd := m.ApplyReceiptAction(a)
	m = next.(Model)
	if cmd == nil || !m.BridgeState().ReceiptBusy || m.BridgeState().ReceiptResult != nil || *calls != 0 {
		t.Fatal("request acknowledged before inspection")
	}
	completion := cmd()
	if *calls != 1 || got.BundlePath != "owned-bundle" || got.CorrelationID != a.CorrelationID || got.ManifestSHA256 != a.ManifestSHA256 {
		t.Fatal("callback did not receive owned exact send", got)
	}
	next, _ = m.Update(completion)
	state := next.(Model).BridgeState()
	if state.ReceiptBusy || state.ReceiptResult.RequestID != a.RequestID || state.ReceiptResult.Check.ProviderID != "message-one" {
		t.Fatal("missing exact acknowledgement", state.ReceiptResult)
	}
}

func TestReceiptInspectionRejectsAmbiguityAndCompletedActions(t *testing.T) {
	for _, status := range []string{"executed", "failed", "pending", "denied", "unknown"} {
		m, a, _, _ := receiptModel(t)
		m.messages[0].bundle.Actions[1].Status = status
		if status == "unknown" {
			m.messages[0].bundle.Actions = append(m.messages[0].bundle.Actions, m.messages[0].bundle.Actions[1])
		}
		next, cmd := m.ApplyReceiptAction(a)
		if cmd != nil || next.(Model).receiptResult.Error == "" {
			t.Fatal("ineligible or ambiguous request accepted", status)
		}
	}
}

func TestReceiptInspectionSerializesAndIgnoresOtherCompletion(t *testing.T) {
	m, a, calls, _ := receiptModel(t)
	next, first := m.ApplyReceiptAction(a)
	m = next.(Model)
	a.RequestID = "receipt-2"
	next, second := m.ApplyReceiptAction(a)
	m = next.(Model)
	if second != nil || !m.receiptBusy || m.receiptResult.RequestID != a.RequestID || m.receiptResult.Error == "" {
		t.Fatal("parallel inspection was not explicitly rejected")
	}
	next, _ = m.Update(receiptChecked{m.conversation, ReceiptResult{RequestID: "other-request"}})
	m = next.(Model)
	if !m.receiptBusy {
		t.Fatal("foreign completion cleared pending request")
	}
	m.conversation = "other-chat"
	next, _ = m.Update(first())
	m = next.(Model)
	if *calls != 1 || m.receiptBusy || m.receiptResult.Check != nil {
		t.Fatal("old check crossed conversation boundary")
	}
}

func TestReceiptInspectionReportsReadFailureAndOwnsProjectionCopies(t *testing.T) {
	m, a, _, _ := receiptModel(t)
	m.config.CheckReceipt = func(context.Context, ReceiptRequest) (ReceiptCheck, error) {
		return ReceiptCheck{}, errors.New("read permission revoked")
	}
	next, cmd := m.ApplyReceiptAction(a)
	next, _ = next.Update(cmd())
	if got := next.(Model).BridgeState().ReceiptResult; got.Error != "read permission revoked" || got.Check != nil {
		t.Fatal("failure claimed confirmed receipt", got)
	}
	original := *m.messages[0].bundle
	original.Actions[0].Steps = []ReceiptStep{{Operation: "document_create", Status: "executed", ProviderResourceID: "doc-one"}}
	clone := cloneBundleSummary(original)
	clone.Actions[0].Status = "failed"
	clone.Actions[0].Steps[0].ProviderResourceID = "different"
	if original.Actions[0].Status != "unknown" || original.Actions[0].Steps[0].ProviderResourceID != "doc-one" {
		t.Fatal("receipt projection aliases retained state")
	}
}
