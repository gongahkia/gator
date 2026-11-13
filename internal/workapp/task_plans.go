package workapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worksession"
)

type TaskPlanAction struct {
	ConversationID string                   `json:"conversation_id"`
	TaskID         string                   `json:"task_id"`
	Revision       uint64                   `json:"revision"`
	Digest         string                   `json:"digest"`
	Action         string                   `json:"action"`
	Content        *worksession.PlanContent `json:"content,omitempty"`
	RequestID      string                   `json:"request_id"`
}
type TaskPlanResult struct {
	Review    *TaskPlanReview `json:"review,omitempty"`
	RequestID string          `json:"request_id"`
	Error     string          `json:"error,omitempty"`
}
type taskPlanPrepared struct {
	ID      string
	Content worksession.PlanContent
	Err     error
}

func (m Model) prepareTaskPlan(prompt string, options RunOptions) (tea.Model, tea.Cmd) {
	fail := func(err error) (tea.Model, tea.Cmd) { m.admissionFailed(err); return m, nil }
	if !m.retainsRequests() || m.config.PreparePlan == nil {
		return fail(errors.New("plan preparation is unavailable"))
	}
	if m.requestsIssue != "" {
		return fail(errors.New(m.requestsIssue))
	}
	lease, err := m.config.SessionStore.AcquireExecution(m.conversation)
	if err != nil {
		return fail(err)
	}
	raw, err := json.Marshal(retainedRunOptions(options))
	if err != nil {
		lease.Close()
		return fail(err)
	}
	budget := worksession.PlanBudget{MaxSteps: options.MaxSteps, CodeMaxSteps: options.Code.MaxSteps}
	var journal worksession.RequestJournal
	var request worksession.Request
	consumed := m.nextRequest
	if consumed != "" {
		journal, request, err = m.config.SessionStore.BeginQueuedTaskPlan(m.conversation, consumed, options.ConnectorIDs, budget)
	} else {
		journal, request, err = m.config.SessionStore.AdmitPlanRequest(m.conversation, prompt, raw, m.submissionID, options.ConnectorIDs, budget, time.Now())
	}
	if err != nil {
		lease.Close()
		return fail(err)
	}
	if request.Plan.Phase != "preparing" || request.Plan.Content != nil {
		lease.Close()
		return fail(errors.New("this task already has a retained plan; review it rather than preparing again"))
	}
	m.requests = journal
	m.nextRequest = ""
	for i, q := range m.queue {
		if q.requestID == consumed {
			m.queue = append(append([]queuedRun(nil), m.queue[:i]...), m.queue[i+1:]...)
			break
		}
	}
	// The durable plan card contains the task and input names. It replaces this
	// transient user bubble until implementation creates the actual saved turn.
	if n := len(m.messages); n > 0 && m.messages[n-1].role == "You" && m.messages[n-1].text == prompt {
		m.messages = m.messages[:n-1]
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.cancelling = false
	m.operation = nil
	m.live = nil
	m.planPreparing = request.ID
	m.activeRequest = ""
	m.running = true
	m.home = false
	m.activity = nil
	m.runStarted = time.Now()
	m.loadingRun++
	m.status = "Preparing a plan for your review…"
	prepare := m.config.PreparePlan
	cmd := func() tea.Msg {
		defer lease.Close()
		defer cancel()
		content, err := prepare(ctx, request)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		lease.Close()
		return taskPlanPrepared{ID: request.ID, Content: content, Err: err}
	}
	if !m.config.Live {
		return m, cmd
	}
	return m, tea.Batch(cmd, m.nextLoadingTick())
}
func (m Model) finishTaskPlan(value taskPlanPrepared) (tea.Model, tea.Cmd) {
	if value.ID != m.planPreparing {
		return m, nil
	}
	stopRequested := m.cancelling
	m.planPreparing = ""
	m.running = false
	m.live = nil
	m.operation = nil
	m.cancel = nil
	m.loadingRun++
	m.cancelling = false
	j, err := m.config.SessionStore.LoadRequests(m.conversation)
	if err != nil {
		m.requestsIssue = "Could not reload the retained plan: " + err.Error()
		return m, nil
	}
	var r worksession.Request
	for _, candidate := range j.Requests {
		if candidate.ID == value.ID {
			r = candidate
			break
		}
	}
	if r.Plan == nil {
		m.bridgeNotice = "The retained plan is unavailable. No implementation was started."
		return m, nil
	}
	if m.planRevising == value.ID && m.planEditTurn != "" {
		if stopRequested && r.State == "queued" && r.Plan.Phase == "revising" && r.Plan.Edit != nil && r.Plan.Edit.TurnID == m.planEditTurn {
			saved, err := m.config.SessionStore.CancelTaskPlan(m.conversation, r.ID, r.Plan.Revision, r.Plan.Digest)
			m.planRevising, m.planEditTurn = "", ""
			if err != nil {
				m.requests = j
				m.bridgeNotice = "Could not confirm cancellation of this plan. No replacement was started; inspect its retained state: " + err.Error()
				return m, nil
			}
			m.requests = saved
			m.publishPlanEditOutcome(r.ID)
			m.status = "Plan cancelled before implementation."
			return m, nil
		}
		if r.State == "queued" && r.Plan.Phase == "revising" && r.Plan.Edit != nil && r.Plan.Edit.State == "preparing" && r.Plan.Edit.TurnID == m.planEditTurn {
			m.requests = j
			return m.continueInitialPlanRevision(r)
		}
		m.planRevising, m.planEditTurn = "", ""
		m.requests = j
		if r.Plan.Edit != nil {
			m.publishPlanEditOutcome(r.ID)
		}
		m.status = "Plan preparation ended without implementation. Inspect its retained outcome."
		return m, nil
	}
	interrupt := func() {
		fresh, loadErr := m.config.SessionStore.LoadRequests(m.conversation)
		if loadErr != nil {
			m.requestsIssue = loadErr.Error()
			return
		}
		for i := range fresh.Requests {
			r := &fresh.Requests[i]
			if r.ID == value.ID && r.State == "queued" && r.Plan != nil && r.Plan.Phase == "preparing" {
				r.State = "interrupted"
				r.Plan = r.Plan.Clone()
				r.Plan.Phase = "interrupted"
				r.Issue = "Plan preparation ended without a retained proposal. No implementation was started."
				r.UpdatedAt = time.Now().UTC()
			}
		}
		if saved, saveErr := m.config.SessionStore.SaveRequests(m.conversation, fresh); saveErr == nil {
			m.requests = saved
		} else {
			m.requestsIssue = saveErr.Error()
		}
	}
	if value.Err != nil {
		interrupt()
		m.status = "Plan preparation stopped. No implementation was started."
		return m, nil
	}
	saved, err := m.config.SessionStore.ReviseTaskPlan(m.conversation, r.ID, r.Plan.Revision, r.Plan.Digest, value.Content)
	if err != nil {
		interrupt()
		m.bridgeNotice = "The proposed plan could not be retained. No implementation was started: " + err.Error()
		return m, nil
	}
	m.requests = saved
	m.status = "Review the proposed plan before Gator starts."
	return m, nil
}

// All decisions refer to a durable task/revision/digest in this exact owner.
// The renderer supplies only reviewed content and finite intent, never grants.
func (m Model) ApplyTaskPlan(action TaskPlanAction) (tea.Model, tea.Cmd) {
	m.planResult = &TaskPlanResult{RequestID: action.RequestID}
	fail := func(err error) (tea.Model, tea.Cmd) { m.planResult.Error = err.Error(); return m, nil }
	if action.ConversationID != m.conversation || !branchIdentity.MatchString(action.ConversationID) || len(action.TaskID) != 32 || !branchIdentity.MatchString(action.TaskID) || !branchIdentity.MatchString(action.RequestID) || len(action.RequestID) > 64 {
		return fail(errors.New("invalid plan owner or identity"))
	}
	cancelPreparation := action.Action == "cancel" && (action.TaskID == m.planPreparing || action.TaskID == m.planRevising)
	if !m.retainsRequests() || m.background || m.section != "" || m.launcher || m.models != nil || m.controlBusy || m.followupSaving || m.recallBusy || m.bridgeExternal != nil || m.pendingBundleAction != nil || m.receiptBusy || m.running && action.Action != "inspect" && !cancelPreparation {
		return fail(errors.New("return to the idle conversation before changing this plan"))
	}
	j, err := m.config.SessionStore.LoadRequests(m.conversation)
	if err != nil {
		return fail(err)
	}
	var r worksession.Request
	for _, candidate := range j.Requests {
		if candidate.ID == action.TaskID {
			r = candidate
			break
		}
	}
	if r.Plan == nil || r.Plan.Revision != action.Revision || r.Plan.Digest != action.Digest {
		return fail(worksession.ErrPlanChanged)
	}
	if action.Action != "edit" && action.Content != nil {
		return fail(errors.New("only an edit can change plan content"))
	}
	switch action.Action {
	case "inspect":
		p := r.Plan.Clone()
		m.planResult.Review = &TaskPlanReview{ID: r.ID, Revision: p.Revision, Digest: p.Digest, Task: r.Text, State: r.State, Phase: p.Phase, Budget: p.Budget, Sources: append([]string{}, p.Sources...), Content: p.Content}
		if len(p.Attempts) > 0 {
			original := p.Budget
			m.planResult.Review.OriginalBudget = &original
			m.planResult.Review.Attempts = p.Clone().Attempts
			remaining, _, err := p.RemainingExecutionBudget()
			if err != nil {
				m.planResult.Review.Issue = err.Error()
			} else {
				m.planResult.Review.Budget = remaining
			}
		}
		if m.planResult.Review.Issue == "" {
			m.planResult.Review.Issue = r.Issue
		}
		if p.Edit != nil {
			m.planResult.Review.Correction, m.planResult.Review.EditState = p.Edit.Instruction, p.Edit.State
		}
		var frozen RunOptions
		if json.Unmarshal(r.Options, &frozen) == nil {
			m.planResult.Review.InputNames = uploadNames(frozen.Uploads)
		}
		return m, nil
	case "edit":
		if action.Content == nil {
			return fail(errors.New("review the edited plan content"))
		}
		j, err = m.config.SessionStore.ReviseTaskPlan(m.conversation, r.ID, action.Revision, action.Digest, *action.Content)
	case "cancel":
		j, err = m.config.SessionStore.CancelTaskPlan(m.conversation, r.ID, action.Revision, action.Digest)
	case "approve":
		var changed bool
		j, changed, err = m.config.SessionStore.ApproveTaskPlan(m.conversation, r.ID, action.Revision, action.Digest)
		if err != nil {
			return fail(err)
		}
		m.requests = j
		if !changed {
			return m, nil
		}
		return m.startReviewedPlan(r.ID)
	case "start":
		m.requests = j
		if r.State != "queued" || r.Plan.Phase != "approved" {
			return fail(errors.New("this task is not an undispatched reviewed plan"))
		}
		return m.startReviewedPlan(r.ID)
	default:
		return fail(errors.New("unsupported plan action"))
	}
	if err != nil {
		return fail(err)
	}
	m.requests = j
	if cancelPreparation {
		m.interruptRun()
	}
	m.status = "Plan review updated."
	return m, nil
}
func (m Model) startReviewedPlan(id string) (tea.Model, tea.Cmd) {
	for _, r := range m.requests.Requests {
		if r.ID != id {
			continue
		}
		var options RunOptions
		if json.Unmarshal(r.Options, &options) != nil {
			return m, nil
		}
		m.nextRequest = r.ID
		m.running = true
		m.home = false
		m.status = "Starting the reviewed task…"
		m.messages = append(m.messages, message{role: "You", text: r.Text, createdAt: time.Now().UTC(), replyToUpdateID: options.ReplyToUpdateID})
		next, cmd := m.startWork(m.source, m.conversation, r.Text, retainedRunOptions(options))
		updated := next.(Model)
		if !updated.running && updated.planResult != nil && updated.planResult.Error == "" {
			updated.planResult.Error = fmt.Sprint(updated.bridgeNotice)
		}
		return updated, cmd
	}
	return m, nil
}

type TaskPlanSummary struct {
	ID       string                 `json:"id"`
	Revision uint64                 `json:"revision"`
	Digest   string                 `json:"digest"`
	Task     string                 `json:"task"`
	Outcome  string                 `json:"outcome,omitempty"`
	Steps    []worksession.PlanStep `json:"steps,omitempty"`
	State    string                 `json:"state"`
	Phase    string                 `json:"phase"`
	WorkID   string                 `json:"work_id,omitempty"`
	At       time.Time              `json:"at"`
	Issue    string                 `json:"issue,omitempty"`
}
type TaskPlanReview struct {
	ID             string                    `json:"id"`
	Revision       uint64                    `json:"revision"`
	Digest         string                    `json:"digest"`
	Task           string                    `json:"task"`
	State          string                    `json:"state"`
	Phase          string                    `json:"phase"`
	Budget         worksession.PlanBudget    `json:"budget"`
	OriginalBudget *worksession.PlanBudget   `json:"original_budget,omitempty"`
	Attempts       []worksession.PlanAttempt `json:"attempts,omitempty"`
	Sources        []string                  `json:"sources"`
	Content        *worksession.PlanContent  `json:"content,omitempty"`
	InputNames     []string                  `json:"input_names,omitempty"`
	Issue          string                    `json:"issue,omitempty"`
	Correction     string                    `json:"correction,omitempty"`
	EditState      string                    `json:"edit_state,omitempty"`
}

func (m Model) taskPlanSummaries() []TaskPlanSummary {
	result := []TaskPlanSummary{}
	for _, r := range m.requests.Requests {
		if r.Plan == nil {
			continue
		}
		summary := TaskPlanSummary{ID: r.ID, Revision: r.Plan.Revision, Digest: r.Plan.Digest, Task: truncate(r.Text, 256), State: r.State, Phase: r.Plan.Phase, WorkID: r.WorkID, At: r.CreatedAt}
		summary.Issue = boundedConversation(r.Issue, 1024)
		if r.Plan.Content != nil {
			summary.Outcome = truncate(r.Plan.Content.Outcome, 512)
			for _, step := range r.Plan.Content.Steps {
				summary.Steps = append(summary.Steps, worksession.PlanStep{ID: step.ID, Text: truncate(step.Text, 256)})
			}
		}
		result = append(result, summary)
	}
	return result
}
