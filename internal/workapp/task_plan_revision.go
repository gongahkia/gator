package workapp

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worksession"
)

type taskPlanRevised struct {
	ID, TurnID, Digest string
	Revision           uint64
	Content            worksession.PlanContent
	Err                error
}

func (m Model) startTaskPlanRevision(id, controlID, parent, turnID, instruction string) (Model, tea.Cmd, error) {
	if !m.retainsRequests() || m.config.RevisePlan == nil || m.running || m.controlBusy || m.followupSaving || m.receiptBusy || m.requestsIssue != "" {
		return m, nil, errors.New("this owner is not ready to revise its retained plan")
	}
	if m.dialogueHost && !m.dialogueCanStart {
		return m, nil, errors.New("three tasks are working; wait for one to finish before revising this plan")
	}
	release := func() {}
	if m.config.BeforeLocalCommand != nil {
		var err error
		release, err = m.config.BeforeLocalCommand()
		if err != nil {
			return m, nil, err
		}
	}
	defer release()
	lease, err := m.config.SessionStore.AcquireExecution(m.conversation)
	if err != nil {
		return m, nil, err
	}
	journal, err := m.config.SessionStore.LoadRequests(m.conversation)
	if err != nil {
		lease.Close()
		return m, nil, err
	}
	var original worksession.Request
	for _, candidate := range journal.Requests {
		if candidate.ID == id {
			original = candidate
			break
		}
	}
	task, ok := m.localConversationTask(original)
	if !ok || !task.CanRevisePlan || task.ControlID == "" || task.ControlID != controlID {
		lease.Close()
		return m, nil, worksession.ErrPlanChanged
	}
	journal, request, claimed, err := m.config.SessionStore.BeginTaskPlanEdit(m.conversation, id, original.Plan.Revision, original.Plan.Digest, parent, turnID, instruction)
	if err != nil {
		lease.Close()
		return m, nil, err
	}
	m.requests = journal
	if !claimed {
		lease.Close()
		return m, nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel, m.running, m.cancelling = cancel, true, false
	m.planRevising, m.planEditTurn = id, turnID
	m.operation, m.live, m.activeRequest = nil, nil, ""
	m.home = false
	m.runStarted = time.Now()
	m.loadingRun++
	m.status = "Preparing a revised plan for your review…"
	revise := m.config.RevisePlan
	command := func() tea.Msg {
		defer lease.Close()
		defer cancel()
		content, err := revise(ctx, request)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return taskPlanRevised{ID: id, TurnID: turnID, Revision: request.Plan.Revision, Digest: request.Plan.Digest, Content: content, Err: err}
	}
	if m.config.Live {
		return m, tea.Batch(command, m.nextLoadingTick()), nil
	}
	return m, command, nil
}

func (m Model) finishTaskPlanRevision(value taskPlanRevised) (tea.Model, tea.Cmd) {
	if value.ID != m.planRevising || value.TurnID != m.planEditTurn {
		return m, nil
	}
	m.planRevising, m.planEditTurn = "", ""
	m.running, m.cancelling, m.cancel = false, false, nil
	m.loadingRun++
	issue := ""
	var content *worksession.PlanContent
	if value.Err != nil {
		issue = boundedConversation(value.Err.Error(), 3500)
	} else {
		content = &value.Content
	}
	journal, err := m.config.SessionStore.FinishTaskPlanEdit(m.conversation, value.ID, value.Revision, value.Digest, value.TurnID, content, issue)
	if err != nil && content != nil {
		// A changed binding or invalid proposal is a failed edit, never reason to
		// execute or silently replace the previous plan with another task.
		journal, err = m.config.SessionStore.FinishTaskPlanEdit(m.conversation, value.ID, value.Revision, value.Digest, value.TurnID, nil, boundedConversation(err.Error(), 3500))
	}
	if err != nil {
		journal, err = m.config.SessionStore.LoadRequests(m.conversation)
		if err != nil {
			m.requestsIssue = "Could not confirm the revised plan: " + err.Error()
			return m, nil
		}
	}
	m.requests = journal
	for _, r := range journal.Requests {
		if r.ID != value.ID || r.Plan == nil || r.Plan.Edit == nil || r.Plan.Edit.TurnID != value.TurnID {
			continue
		}
		m.publishPlanEditOutcome(r.ID)
		if r.Plan.Edit.State == "applied" {
			m.status = "Revised plan saved. Review it before starting."
		} else {
			m.status = "Plan revision ended without implementation. Inspect the retained plan."
		}
		return m, nil
	}
	m.bridgeNotice = "The plan changed while its revision was prepared. Inspect the original task; no implementation was started."
	return m, nil
}

func (m *Model) publishPlanEditOutcome(id string) {
	turn, err := m.config.SessionStore.PublishPlanEditResult(m.conversation, id)
	if err != nil {
		m.bridgeNotice = "The retained plan's revision receipt could not be confirmed. Inspect its current state before retrying: " + err.Error()
		return
	}
	if turn.ConversationID == m.conversation {
		m.rememberDialogue(turn)
	}
}

func (m *Model) refreshPlanEditMessages() {
	if !m.retainsRequests() {
		return
	}
	for _, turn := range m.dialogueTurns {
		if turn.State != "editing_plan" {
			continue
		}
		fresh, err := m.config.SessionStore.ReadDialogue(m.conversation, turn.ID)
		if err == nil && fresh.Revision > turn.Revision {
			m.rememberDialogue(fresh)
		}
	}
}
