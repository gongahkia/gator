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
		return task, true
	}
	p := request.Plan
	if p == nil || request.State != "queued" {
		return task, false
	}
	switch p.Phase {
	case "preparing":
		if request.ID != m.planPreparing || !m.running {
			return task, false // Stored preparation is not evidence of a live owner.
		}
		task.State = "preparing a plan"
		task.CanStop = m.cancel != nil && !m.cancelling
	case "review", "approved":
		task.State = "plan awaiting review"
		if p.Phase == "approved" {
			task.State = "reviewed plan awaiting dispatch"
		}
		task.CanStop = true
	default:
		return task, false
	}
	if p.Content != nil {
		task.Summary = boundedConversation(p.Content.Outcome, 2048)
	}
	// Revision, digest and phase must all still match after tool-free routing.
	task.ControlID = fmt.Sprintf("plan-%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s:%s", request.ID, p.Revision, p.Digest, p.Phase))))
	return task, true
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
		if request.Plan != nil && request.State == "queued" {
			journal, err = m.config.SessionStore.CancelTaskPlan(m.conversation, id, request.Plan.Revision, request.Plan.Digest)
			if err != nil {
				return false, err
			}
			m.requests = journal
			if id == m.planPreparing {
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
