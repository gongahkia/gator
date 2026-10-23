package workapp

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/actionlog"
)

type RecoveredAction struct {
	ID      string        `json:"id"`
	RunID   string        `json:"run_id"`
	At      string        `json:"at"`
	Live    bool          `json:"live"`
	Receipt ActionReceipt `json:"receipt"`
}

func receiptIdentity(run string, r ActionReceipt) string {
	// Presentation-only fields and cropped error/target text do not identify an
	// execution. Preserve distinct attempts and distinct provider receipts.
	steps := append([]ReceiptStep(nil), r.Steps...)
	for i := range steps {
		steps[i].Target = ""
	}
	data, _ := json.Marshal(struct {
		Run, ID, Correlation, Status, ProviderReceipt, Resource string
		Steps                                                   []ReceiptStep
	}{run, r.ID, r.CorrelationID, r.Status, r.ReceiptID, r.ProviderResourceID, steps})
	return string(data)
}
func (m *Model) refreshActionRecoveries() {
	owner := m.conversation
	if owner == "" || m.config.ReadActionRecoveries == nil {
		m.actionRecoveries = nil
		m.actionRecoveryIssue = ""
		m.actionRecoveryOwner = owner
		m.actionRecoveryTruncated = false
		return
	}
	entries, err := m.config.ReadActionRecoveries(owner)
	if err != nil {
		if m.actionRecoveryOwner != owner {
			m.actionRecoveries = nil
			m.actionRecoveryTruncated = false
		}
		m.actionRecoveryOwner = owner
		m.actionRecoveryIssue = "Saved request receipts are unavailable. Verify connected services before retrying interrupted work."
		return
	}
	m.actionRecoveryOwner = owner
	m.actionRecoveryIssue = ""
	m.actionRecoveries = nil
	m.actionRecoveryTruncated = false
	sealed := map[string]int{}
	for _, message := range m.messages {
		if message.bundle != nil {
			for _, r := range message.bundle.Actions {
				sealed[receiptIdentity(message.bundle.RunID, r)]++
			}
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.ConversationID != owner {
			continue
		}
		receipt := projectRecoveredAction(entry)
		key := receiptIdentity(entry.RunID, receipt)
		if sealed[key] > 0 {
			sealed[key]--
			continue
		}
		if len(m.actionRecoveries) == 32 {
			m.actionRecoveryTruncated = true
			continue
		}
		m.actionRecoveries = append(m.actionRecoveries, RecoveredAction{ID: entry.ID, RunID: entry.RunID, At: entry.StartedAt.Format(time.RFC3339), Live: entry.Live, Receipt: receipt})
	}
}
func projectRecoveredAction(entry actionlog.Entry) ActionReceipt {
	r := entry.Action
	item := ActionReceipt{ID: r.Proposal.ID, ConnectorID: r.Proposal.ConnectorID, Operation: r.Proposal.Operation, Status: string(r.Status), CorrelationID: r.Proposal.CorrelationID, ReceiptID: r.ReceiptID, ProviderResourceID: r.ProviderResourceID}
	// The journal keeps the full approved preview privately. Only the useful
	// email envelope, bounded outcome text and batch receipts enter chat state.
	if item.Operation == "gmail_send" {
		var fields struct{ To, Subject string }
		if json.Unmarshal([]byte(r.Proposal.Preview), &fields) == nil {
			item.Recipient = cropReceiptText(fields.To, 254)
			item.Subject = cropReceiptText(fields.Subject, 200)
		}
	}
	item.Error = cropReceiptText(r.Error, 240)
	for _, step := range r.BatchSteps {
		item.Steps = append(item.Steps, ReceiptStep{Operation: step.Operation, Status: string(step.Status), ProviderResourceID: step.ProviderResourceID})
	}
	return item
}

func cropReceiptText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}
