package workapp

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worksession"
)

type RequestRecoveryView struct {
	TaskID   string                      `json:"task_id"`
	Token    string                      `json:"token"`
	Text     string                      `json:"text"`
	Provider string                      `json:"provider"`
	Model    string                      `json:"model"`
	Recovery worksession.RequestRecovery `json:"recovery"`
}
type RequestRecoveryAction struct {
	ConversationID string `json:"conversation_id"`
	TaskID         string `json:"task_id"`
	Token          string `json:"token"`
	Action         string `json:"action"`
	RequestID      string `json:"request_id"`
}
type RequestRecoveryResult struct {
	RequestID string `json:"request_id"`
	Accepted  bool   `json:"accepted,omitempty"`
	Error     string `json:"error,omitempty"`
}

func recoveryToken(conversation string, r worksession.Request) string {
	data, _ := json.Marshal([]any{conversation, r})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
func (m Model) requestRecoveryViews() []RequestRecoveryView {
	result := []RequestRecoveryView{}
	for i := len(m.requests.Requests) - 1; i >= 0 && len(result) < 16; i-- {
		r := m.requests.Requests[i]
		if r.State != "failed" || r.Recovery == nil {
			continue
		}
		var options RunOptions
		if json.Unmarshal(r.Options, &options) != nil {
			continue
		}
		result = append(result, RequestRecoveryView{TaskID: r.ID, Token: recoveryToken(m.conversation, r), Text: boundedConversation(r.Text, 1024), Provider: options.ModelProvider, Model: options.ModelName, Recovery: *r.Recovery})
	}
	return result
}
func (m Model) ApplyRequestRecovery(action RequestRecoveryAction) (tea.Model, tea.Cmd) {
	m.requestRecoveryResult = &RequestRecoveryResult{RequestID: action.RequestID}
	fail := func(err error) (tea.Model, tea.Cmd) { m.requestRecoveryResult.Error = err.Error(); return m, nil }
	if action.ConversationID != m.conversation || !branchIdentity.MatchString(action.RequestID) || len(action.RequestID) > 64 || len(action.TaskID) != 32 || !branchIdentity.MatchString(action.TaskID) || !m.retainsRequests() || m.background || m.running || m.dialogueRouting != "" || len(m.dialogueQueue) > 0 || m.dialogueHost && !m.dialogueCanStart || m.section != "" || m.launcher || m.models != nil || m.controlBusy || m.followupSaving || m.recallBusy || m.receiptBusy || m.pendingBundleAction != nil || m.bridgeExternal != nil {
		return fail(errors.New("return to the idle original task owner before reviewing recovery"))
	}
	release := func() {}
	if m.config.BeforeLocalCommand != nil {
		var err error
		release, err = m.config.BeforeLocalCommand()
		if err != nil {
			return fail(err)
		}
	}
	defer release()
	journal, err := m.config.SessionStore.LoadRequests(m.conversation)
	if err != nil {
		return fail(err)
	}
	var original worksession.Request
	for _, r := range journal.Requests {
		if r.ID == action.TaskID {
			original = r
			break
		}
	}
	if original.ID == "" || original.State != "failed" || original.Recovery == nil || action.Token != recoveryToken(m.conversation, original) {
		return fail(errors.New("the original recovery outcome changed; inspect its current task"))
	}
	if action.Action == "discard" {
		for i := range journal.Requests {
			if journal.Requests[i].ID == original.ID {
				journal.Requests[i].State = "discarded"
				journal.Requests[i].UpdatedAt = time.Now().UTC()
			}
		}
		if err := m.saveRequests(journal); err != nil {
			return fail(err)
		}
		m.requestRecoveryResult.Accepted = true
		return m, nil
	}
	if action.Action != "prepare_retry" || !original.Recovery.RetrySafe || m.config.RevisePlan == nil {
		return fail(errors.New("inspect original work and action receipts before any additional execution"))
	}
	var frozen RunOptions
	if json.Unmarshal(original.Options, &frozen) != nil {
		return fail(errors.New("original request settings are unavailable"))
	}
	m.refreshModelStatus()
	if issue := m.composerModelIssue(); issue != "" {
		return fail(errors.New(issue))
	}
	if frozen.ModelProvider != "" && (m.modelStatus.Provider != frozen.ModelProvider || m.modelStatus.Model != frozen.ModelName) {
		return fail(errors.New("select the original provider and model after repair; changed processing needs its own scope review"))
	}
	input := ConversationInput{Version: 1, Source: m.source, RunOptions: frozen}
	if err := m.validateDialogueInputs(input); err != nil {
		return fail(err)
	}
	if m.config.CaptureConversationInput != nil {
		input, err = m.config.CaptureConversationInput(input)
		if err != nil {
			return fail(err)
		}
	}
	if original.Recovery.ScopeDigest == "" || input.ScopeDigest != original.Recovery.ScopeDigest {
		return fail(errors.New("the original accounts, permissions or processing configuration changed; review scope before preparing more work"))
	}
	lease, err := m.config.SessionStore.AcquireExecution(m.conversation)
	if err != nil {
		return fail(err)
	}
	raw, _ := json.Marshal(input)
	text := "Prepare a reviewed retry of the original request after provider setup repair. Preserve its inputs, scope and remaining budget."
	turn, err := m.config.SessionStore.AdmitDialogue(m.conversation, "request-recovery-"+action.RequestID, text, raw, time.Now())
	if err != nil {
		lease.Close()
		return fail(err)
	}
	for _, phase := range []string{"routing", "ready", "editing_plan"} {
		turn.State = phase
		turn.UpdatedAt = time.Now().UTC()
		if phase == "ready" {
			turn.Decision, _ = json.Marshal(map[string]string{"kind": "revise_plan", "target": original.ID, "instruction": text})
		}
		if phase == "editing_plan" {
			turn.Response = "Preparing the original task's retry for fresh review…"
		}
		turn, err = m.config.SessionStore.UpdateDialogue(turn, turn.Revision)
		if err != nil {
			lease.Close()
			return fail(err)
		}
	}
	m.rememberDialogue(turn)
	retained, request, err := m.config.SessionStore.PrepareFailedRequestReview(m.conversation, original.ID, m.conversation, turn.ID, text)
	if err != nil {
		lease.Close()
		turn.State, turn.Response, turn.Issue = "failed", "", boundedConversation(err.Error(), 3500)
		turn.UpdatedAt = time.Now().UTC()
		if saved, saveErr := m.config.SessionStore.UpdateDialogue(turn, turn.Revision); saveErr == nil {
			m.rememberDialogue(saved)
		}
		return fail(err)
	}
	m.requests = retained
	next, command, err := m.launchTaskPlanRevision(request, lease)
	if err != nil {
		lease.Close()
		return fail(err)
	}
	next.requestRecoveryResult = &RequestRecoveryResult{RequestID: action.RequestID, Accepted: true}
	return next, command
}
