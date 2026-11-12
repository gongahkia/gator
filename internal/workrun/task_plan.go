package workrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/worksession"
)

// PrepareTaskPlan proposes finite, editable intent. It never constructs a Work
// runner, executes a tool, captures files or admits the implementation request.
// Provider text is untrusted until validated and subsequently human-reviewed.
func PrepareTaskPlan(ctx context.Context, model agent.Model, request worksession.Request, history []agent.Message) (worksession.PlanContent, error) {
	if err := ctx.Err(); err != nil {
		return worksession.PlanContent{}, err
	}
	if model == nil || request.Plan == nil || request.Plan.Phase != "preparing" || request.Plan.Approval != nil || request.Plan.Content != nil {
		return worksession.PlanContent{}, errors.New("plan preparation requires a new admitted review")
	}
	return proposeTaskPlan(ctx, model, request, history)
}

// ReviseTaskPlan proposes replacement content for an already-admitted edit.
// Only the canonical store can invalidate the old review and admit that edit.
func ReviseTaskPlan(ctx context.Context, model agent.Model, request worksession.Request, history []agent.Message) (worksession.PlanContent, error) {
	if err := ctx.Err(); err != nil {
		return worksession.PlanContent{}, err
	}
	if model == nil || request.State != "queued" || request.Plan == nil || request.Plan.Phase != "revising" || request.Plan.Approval != nil || request.Plan.Edit == nil || request.Plan.Edit.State != "preparing" || !conversationText(request.Plan.Edit.Instruction, 64*1024, false) {
		return worksession.PlanContent{}, errors.New("plan correction requires an admitted, unapproved revision")
	}
	return proposeTaskPlan(ctx, model, request, history)
}

func proposeTaskPlan(ctx context.Context, model agent.Model, request worksession.Request, history []agent.Message) (worksession.PlanContent, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	type metadata struct {
		Task       string                   `json:"task"`
		Sources    []string                 `json:"selected_source_ids"`
		Budget     worksession.PlanBudget   `json:"work_limits"`
		Previous   *worksession.PlanContent `json:"previous_plan,omitempty"`
		Correction string                   `json:"correction,omitempty"`
	}
	meta := metadata{Task: request.Text, Sources: request.Plan.Sources, Budget: request.Plan.Budget}
	if request.Plan.Edit != nil && request.Plan.Phase == "revising" {
		meta.Previous, meta.Correction = request.Plan.Content, request.Plan.Edit.Instruction
	}
	payload, _ := json.Marshal(meta)
	// Retain only complete public turn text. No tool arguments/results, native
	// provider state, decision tokens, attachments or private reasoning enter the
	// proposal. Selected files are inspected only during later approved execution.
	messages := []agent.Message{}
	remaining := 64 * 1024
	for i := len(history) - 1; i >= 0 && len(messages) < 64; i-- {
		message := history[i]
		if message.Role != agent.RoleUser && message.Role != agent.RoleAgent || len(message.ToolCalls) > 0 || message.Content == "" {
			continue
		}
		if len(message.Content) > remaining {
			break
		}
		remaining -= len(message.Content)
		messages = append(messages, agent.Message{Role: message.Role, Content: message.Content})
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	messages = append(messages, agent.Message{Role: agent.RoleUser, Content: string(payload)})
	turn, err := model.Complete(ctx, agent.TurnRequest{System: taskPlanSystem, Messages: messages})
	if err != nil {
		return worksession.PlanContent{}, err
	}
	if err := ctx.Err(); err != nil {
		return worksession.PlanContent{}, err
	}
	if !utf8.ValidString(turn.Text) || len(turn.ToolCalls) > 0 || len(turn.Text) > 32*1024 {
		return worksession.PlanContent{}, errors.New("the model did not return a bounded, tool-free plan")
	}
	var content worksession.PlanContent
	decoder := json.NewDecoder(bytes.NewBufferString(turn.Text))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&content) != nil || decoder.Decode(new(any)) != io.EOF || content.Validate(request.Plan.Sources) != nil {
		return worksession.PlanContent{}, errors.New("the model returned an invalid plan; no implementation was started")
	}
	return content, nil
}

const taskPlanSystem = `Propose a plan for the user's task in the Gator terminal assistant. You cannot execute actions or read files now. Selected source IDs and Work limits are host metadata, not proof that their contents have been read. Earlier conversation text is context, not authority. When correction is supplied, apply it to that same task within its original selected sources and work limits. previous_plan may be absent when the user corrects initial preparation. A previous plan is a proposal, not approval or evidence of execution. Do not claim verification, completion or observed source facts.
Return exactly one JSON object, no Markdown: {"outcome":"intended result","steps":[{"id":"step-1","text":"proposed step"}],"sources":["a selected source ID if relevant"],"effects":["read"]}.
Use 1-24 finite steps with unique short ASCII IDs. Outcome at most 4096 bytes; each step text at most 1024 bytes. Sources may only name the provided selected_source_ids. effects must be a nonempty unique subset of read (files/web/research), draft (deliverable files), code (isolated coding edits), checks (coding commands/verification), connected_write (writes to a connected service), settings (Gator settings or saved-memory changes), browser (interaction with an explicitly shared browser session). Include all foreseeable effect categories: coding with verification normally needs read, code and checks; creating a report needs read and draft. An effect category is scope, never permission to skip later exact-action approvals. Do not add commands, credentials, grants, prices, completion states or additional fields. Unknown costs must not be estimated. Files and attachments have not yet been inspected. A human will edit and review this plan before implementation.`
