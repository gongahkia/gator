package workapp

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

// localConversationTask projects the actual execution owner, including the
// otherwise idle period in which a proposed plan is waiting for human review.
func (m Model) localConversationTask(request worksession.Request) (workrun.ConversationTask, bool) {
	task := workrun.ConversationTask{ID: request.ID, Title: boundedConversation(request.Text, 256)}
	if m.running && request.ID == m.activeRequest {
		task.State, task.ControlID = "running", request.ID
		if m.operation != nil {
			task.ControlID = m.operation.RunID
		}
		if m.interaction != nil {
			task.State = "waiting for review"
		}
		if m.cancelling {
			task.State = "stopping"
		}
		task.CanGuide = m.bridgeGuidanceID() != "" && m.interaction == nil && !m.cancelling
		task.CanStop = m.cancel != nil && !m.cancelling
		if request.Plan != nil && m.operation != nil && task.CanStop && m.config.RevisePlan != nil && (request.Plan.Phase == "dispatching" || request.Plan.DrainingRun == m.operation.RunID) {
			task.CanRevisePlan = !m.controlBusy && !m.followupSaving && !m.receiptBusy && m.requestsIssue == ""
		}
		if request.Plan == nil && !m.cancelling && m.operation != nil && m.interaction != nil && len(m.pendingInteractions) == 0 {
			digest, err := workrun.InteractionDigest(*m.interaction)
			if err == nil && m.operation.CanCorrectAction(m.interaction.ID, digest) {
				task.CanCorrectAction, task.ReviewControlID = true, m.bridgeApprovalToken()
			}
		}
		return task, true
	}
	p := request.Plan
	if p == nil || request.State != "queued" && !(request.State == "interrupted" && p.Phase == "interrupted" && p.Content == nil) {
		return task, false
	}
	switch p.Phase {
	case "revising":
		if request.ID != m.planRevising || !m.running {
			return task, false
		}
		task.State, task.CanStop = "preparing a revised plan", m.cancel != nil && !m.cancelling
		task.CanRevisePlan = m.config.RevisePlan != nil && task.CanStop && !m.controlBusy && !m.followupSaving && !m.receiptBusy && m.requestsIssue == ""
	case "preparing":
		if request.ID != m.planPreparing || !m.running {
			return task, false // Stored preparation is not evidence of a live owner.
		}
		task.State = "preparing a plan"
		task.CanStop = m.cancel != nil && !m.cancelling
		task.CanRevisePlan = m.config.RevisePlan != nil && m.planRevising == "" && task.CanStop && !m.controlBusy && !m.followupSaving && !m.receiptBusy && m.requestsIssue == ""
	case "interrupted":
		task.State, task.CanStop = "plan preparation interrupted", true
		task.CanRevisePlan = m.config.RevisePlan != nil && !m.running && !m.controlBusy && !m.followupSaving && !m.receiptBusy && m.requestsIssue == "" && (!m.dialogueHost || m.dialogueCanStart)
	case "review", "approved":
		task.State = "plan awaiting review"
		if p.Phase == "approved" {
			task.State = "reviewed plan awaiting dispatch"
		}
		task.CanStop = true
		task.CanRevisePlan = m.config.RevisePlan != nil && !m.running && !m.controlBusy && !m.followupSaving && !m.receiptBusy && m.requestsIssue == "" && (!m.dialogueHost || m.dialogueCanStart)
	default:
		return task, false
	}
	if p.Content != nil {
		task.Summary = boundedConversation(p.Content.Outcome, 2048)
	}
	task.PreparationControlID = initialPreparationControl(request)
	// Revision, digest and phase must all still match after tool-free routing.
	task.ControlID = fmt.Sprintf("plan-%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s:%s", request.ID, p.Revision, p.Digest, p.Phase))))
	return task, true
}

const actionCorrectionAccepted = "I withdrew that pending proposal and passed your correction to the same task. Any replacement still needs a fresh exact review; this correction did not send or approve anything."

func (m *Model) correctPendingAction(token, instruction string) error {
	if m.operation == nil || m.interaction == nil || !m.running || m.cancelling || token == "" || token != m.bridgeApprovalToken() {
		return errors.New("the pending action changed; inspect its current review or receipt")
	}
	digest, err := workrun.InteractionDigest(*m.interaction)
	if err != nil {
		return err
	}
	if err := m.operation.CorrectAction(m.interaction.ID, digest, instruction); err != nil {
		return err
	}
	m.interaction = nil
	m.status = "Previous proposal withdrawn. Preparing a correction within the same task; fresh review is still required."
	return nil
}

// Only the automatic transition to the first unapproved proposal can inherit
// its preparation correction identity. Edits, approvals, dispatch and later
// revisions continue to require their exact current control epoch.
func initialPreparationControl(request worksession.Request) string {
	p := request.Plan
	if request.State != "queued" || p == nil || p.Phase != "review" || p.Revision != 1 || p.Content == nil || p.Edit != nil || p.Approval != nil {
		return ""
	}
	return fmt.Sprintf("plan-%x", sha256.Sum256([]byte(fmt.Sprintf("%s:0::preparing", request.ID))))
}

func (m Model) ownsConversationTask(id string) bool {
	for _, request := range m.requests.Requests {
		if request.ID == id {
			_, ok := m.localConversationTask(request)
			return ok
		}
	}
	return false
}

// stopConversationTask cancels an undispatched plan through its canonical CAS
// path. A different running task, original inputs and approvals are untouched.
func (m *Model) stopConversationTask(id, controlID string) (bool, error) {
	if !m.retainsRequests() {
		return false, errors.New("the selected task owner is unavailable")
	}
	journal, err := m.config.SessionStore.LoadRequests(m.conversation)
	if err != nil {
		return false, err
	}
	for _, request := range journal.Requests {
		if request.ID != id {
			continue
		}
		task, ok := m.localConversationTask(request)
		if !ok || !task.CanStop || controlID == "" || task.ControlID != controlID {
			return false, errors.New("the selected task or plan changed; no different task was stopped")
		}
		if request.Plan != nil && (request.State == "queued" || request.State == "interrupted") {
			journal, err = m.config.SessionStore.CancelTaskPlan(m.conversation, id, request.Plan.Revision, request.Plan.Digest)
			if err != nil {
				return false, err
			}
			m.requests = journal
			if id == m.planPreparing || id == m.planRevising {
				m.interruptRun()
			}
			m.status = "Plan cancelled. No implementation was started."
			return true, nil
		}
		m.interruptRun()
		return false, nil
	}
	return false, errors.New("the selected task no longer exists; no different task was stopped")
}
