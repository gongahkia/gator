package workapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
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
	preparing := m.running && id == m.planPreparing && m.planRevising == "" && m.cancel != nil && !m.cancelling
	revising := m.running && id == m.planRevising && m.cancel != nil && !m.cancelling
	executing := m.running && id == m.activeRequest && m.operation != nil && m.cancel != nil && !m.cancelling
	if !m.retainsRequests() || m.config.RevisePlan == nil || m.running && !preparing && !revising && !executing || m.controlBusy || m.followupSaving || m.receiptBusy || m.requestsIssue != "" {
		return m, nil, errors.New("this owner is not ready to revise its retained plan")
	}
	if !preparing && !revising && !executing && m.dialogueHost && !m.dialogueCanStart {
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
	var lease *os.File
	if !preparing && !revising && !executing {
		var err error
		lease, err = m.config.SessionStore.AcquireExecution(m.conversation)
		if err != nil {
			return m, nil, err
		}
	}
	closeLease := func() {
		if lease != nil {
			lease.Close()
		}
	}
	journal, err := m.config.SessionStore.LoadRequests(m.conversation)
	if err != nil {
		closeLease()
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
	if !ok || !task.CanRevisePlan || task.ControlID == "" || task.ControlID != controlID && (controlID == "" || task.PreparationControlID != controlID) {
		closeLease()
		return m, nil, worksession.ErrPlanChanged
	}
	correction, err := m.config.SessionStore.ReadDialogue(parent, turnID)
	if err != nil {
		closeLease()
		return m, nil, err
	}
	var input ConversationInput
	if json.Unmarshal(correction.Options, &input) != nil {
		closeLease()
		return m, nil, errors.New("the correction's captured inputs are unavailable")
	}
	if len(input.RunOptions.Uploads)+len(input.RunOptions.Attachments) > 0 {
		if input.Source != m.source {
			closeLease()
			return m, nil, errors.New("the correction belongs to a different workspace")
		}
		var frozen RunOptions
		if json.Unmarshal(original.Options, &frozen) != nil {
			closeLease()
			return m, nil, errors.New("original task input scope is unavailable")
		}
		frozen.Uploads = append(frozen.Uploads, input.RunOptions.Uploads...)
		frozen.Attachments = append(frozen.Attachments, input.RunOptions.Attachments...)
		frozen.Uploads = uniquePlanInputs(frozen.Uploads)
		frozen.Attachments = uniquePlanInputs(frozen.Attachments)
		if err := m.validateDialogueInputs(ConversationInput{Source: m.source, RunOptions: frozen}); err != nil {
			closeLease()
			return m, nil, err
		}
	}
	var request worksession.Request
	var claimed bool
	if executing && original.Plan != nil && original.Plan.Phase == "dispatching" {
		journal, request, claimed, err = m.config.SessionStore.BeginRunningPlanEdit(m.conversation, id, original.Plan.Revision, original.Plan.Digest, parent, turnID, instruction, m.operation.RunID)
	} else {
		journal, request, claimed, err = m.config.SessionStore.BeginTaskPlanEdit(m.conversation, id, original.Plan.Revision, original.Plan.Digest, parent, turnID, instruction)
	}
	if err != nil {
		closeLease()
		return m, nil, err
	}
	m.requests = journal
	if !claimed {
		closeLease()
		return m, nil, nil
	}
	if err := m.config.SessionStore.PublishPriorPlanEdits(m.conversation, id); err != nil {
		m.bridgeNotice = "Correction retained, but an earlier correction receipt needs reconciliation: " + err.Error()
	}
	m.refreshPlanEditMessages()
	if executing {
		m.planRevising, m.planEditTurn = id, turnID
		m.cancel()
		m.status = "Correction retained. Stopping the reviewed run and accounting its results before replanning…"
		return m, nil, nil
	}
	if revising {
		// Keep the draining call's identity until its completion releases its
		// lease. The canonical plan already rejects that obsolete result.
		if m.planPreparing == id {
			// The original preparation has no edit-turn callback; its handoff
			// selects the latest retained edit directly.
			m.planEditTurn = turnID
		}
		m.cancel()
		m.status = "Correction saved. Waiting for the superseded proposal to stop…"
		return m, nil, nil
	}
	if preparing {
		// Retire the first proposal only after its correction is durable. Its
		// completion releases the original execution lease before the replacement
		// starts; an obsolete result can never become a reviewable plan.
		m.planRevising, m.planEditTurn = id, turnID
		m.cancel()
		m.status = "Applying your correction before presenting the plan…"
		return m, nil, nil
	}
	return m.launchTaskPlanRevision(request, lease)
}

func (m Model) launchTaskPlanRevision(request worksession.Request, lease *os.File) (Model, tea.Cmd, error) {
	id, turnID := request.ID, request.Plan.Edit.TurnID
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

func (m Model) continueInitialPlanRevision(request worksession.Request) (tea.Model, tea.Cmd) {
	fail := func(err error) (tea.Model, tea.Cmd) {
		return m.finishTaskPlanRevision(taskPlanRevised{ID: request.ID, TurnID: m.planEditTurn, Revision: request.Plan.Revision, Digest: request.Plan.Digest, Err: err})
	}
	lease, err := m.config.SessionStore.AcquireExecution(m.conversation)
	if err != nil {
		return fail(err)
	}
	if err := m.config.SessionStore.CheckPlanBinding(m.conversation, request); err != nil {
		lease.Close()
		return fail(err)
	}
	next, cmd, err := m.launchTaskPlanRevision(request, lease)
	if err != nil {
		lease.Close()
		return fail(err)
	}
	return next, cmd
}

func (m Model) finishTaskPlanRevision(value taskPlanRevised) (tea.Model, tea.Cmd) {
	if value.ID != m.planRevising || value.TurnID != m.planEditTurn {
		return m, nil
	}
	// A correction can supersede this tool-free call while it drains. Start
	// only the latest retained edit, after the old command released its lease.
	if journal, err := m.config.SessionStore.LoadRequests(m.conversation); err == nil {
		for _, r := range journal.Requests {
			if r.ID != value.ID || r.Plan == nil || r.Plan.Edit == nil || r.Plan.Edit.TurnID == value.TurnID {
				continue
			}
			m.requests = journal
			if r.State == "queued" && r.Plan.Phase == "revising" && r.Plan.Edit.State == "preparing" && !m.cancelling {
				m.planEditTurn = r.Plan.Edit.TurnID
				return m.continueInitialPlanRevision(r)
			}
			if m.cancelling && r.State == "queued" && r.Plan.Phase == "revising" {
				var err error
				m.requests, err = m.config.SessionStore.CancelTaskPlan(m.conversation, r.ID, r.Plan.Revision, r.Plan.Digest)
				if err != nil {
					m.requestsIssue = err.Error()
				}
			}
			m.planRevising, m.planEditTurn = "", ""
			m.running, m.cancelling, m.cancel = false, false, nil
			m.loadingRun++
			m.publishPlanEditOutcome(r.ID)
			m.status = "Plan revision stopped without implementation. Inspect its retained outcome."
			return m, nil
		}
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

func uniquePlanInputs(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
