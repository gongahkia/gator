package workapp

import (
	"context"
	"errors"
	"regexp"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ActionReceipt projects only retained action metadata. Payloads and credentials
// stay outside the chat projection, and provider acceptance is not delivery.
type ActionReceipt struct {
	ID, ConnectorID, Operation, Target, Status   string
	Recipient, Subject, Error                    string
	CorrelationID, ReceiptID, ProviderResourceID string
	Steps                                        []ReceiptStep
}
type ReceiptStep struct{ Operation, Target, Status, ProviderResourceID string }

type ReceiptAction struct {
	ConversationID string `json:"conversation_id"`
	RunID          string `json:"run_id"`
	ActionID       string `json:"action_id"`
	CorrelationID  string `json:"correlation_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Action         string `json:"action"`
	RequestID      string `json:"request_id"`
}
type ReceiptRequest struct {
	BundlePath, RunID, ManifestSHA256, CorrelationID, Action string
}
type ReceiptCheck struct {
	Status     string `json:"status"`
	ProviderID string `json:"provider_id,omitempty"`
	CheckedAt  string `json:"checked_at,omitempty"`
}
type ReceiptResult struct {
	RequestID string        `json:"request_id"`
	Check     *ReceiptCheck `json:"check,omitempty"`
	Error     string        `json:"error,omitempty"`
}
type receiptChecked struct {
	ConversationID string
	Result         ReceiptResult
}

var receiptNoncePattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)
var receiptDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (m Model) ApplyReceiptAction(a ReceiptAction) (tea.Model, tea.Cmd) {
	reject := func(err error) (tea.Model, tea.Cmd) {
		m.receiptResult = &ReceiptResult{RequestID: a.RequestID, Error: err.Error()}
		return m, nil
	}
	if m.receiptBusy {
		return reject(errors.New("Wait for the current receipt check to finish."))
	}
	if m.conversation == "" || a.ConversationID != m.conversation || !receiptNoncePattern.MatchString(a.RequestID) || !receiptDigestPattern.MatchString(a.ManifestSHA256) || a.Action != "inspect" && a.Action != "check_sent" || m.config.CheckReceipt == nil {
		return reject(errors.New("Return to this request’s conversation and reopen its receipt."))
	}
	var owned *BundleSummary
	for _, message := range m.messages {
		if message.bundle != nil && message.bundle.RunID == a.RunID && message.bundle.ManifestSHA256 == a.ManifestSHA256 {
			owned = message.bundle
			break
		}
	}
	if owned == nil {
		return reject(errors.New("This retained request changed or belongs to another chat."))
	}
	var selected *ActionReceipt
	for i := range owned.Actions {
		if owned.Actions[i].ID == a.ActionID && owned.Actions[i].CorrelationID == a.CorrelationID {
			if selected != nil {
				return reject(errors.New("The retained request identity is ambiguous."))
			}
			selected = &owned.Actions[i]
		}
	}
	if selected == nil || selected.Status != "unknown" || selected.Operation != "gmail_send" || selected.CorrelationID == "" {
		return reject(errors.New("Only an unconfirmed Gmail send can be checked in Sent."))
	}
	request := ReceiptRequest{BundlePath: owned.Path, RunID: owned.RunID, ManifestSHA256: owned.ManifestSHA256, CorrelationID: selected.CorrelationID, Action: a.Action}
	m.receiptBusy = true
	m.receiptPendingID = a.RequestID
	m.receiptResult = nil
	check, conversation := m.config.CheckReceipt, m.conversation
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		value, err := check(ctx, request)
		result := ReceiptResult{RequestID: a.RequestID}
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Check = &value
		}
		return receiptChecked{conversation, result}
	}
}
