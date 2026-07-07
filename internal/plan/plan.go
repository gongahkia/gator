package plan

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gongahkia/paw/internal/budget"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/schema"
)

const maxPlanOutputTokens = 1024

type Plan struct {
	Client        llm.Client
	UseRawContext bool
}

func New(client llm.Client) *Plan {
	return &Plan{Client: client}
}

func (p *Plan) Name() string {
	return "plan"
}

func (p *Plan) Run(ctx context.Context, in *envelope.Envelope) (*envelope.Envelope, error) {
	out := *in
	out.Stage = p.Name()
	if in.Done {
		out.Plan = &envelope.Plan{Done: true, Reasoning: "already done"}
		return &out, nil
	}
	if p.Client == nil {
		return nil, fmt.Errorf("plan client is nil")
	}
	rawSchema := schema.Raw("plan")
	resp, err := p.Client.Chat(ctx, llm.ChatRequest{
		Messages:    planMessages(in, p.UseRawContext),
		Temperature: 0,
		MaxTokens:   maxPlanOutputTokens,
		JSONSchema:  rawSchema,
	})
	if err != nil {
		return nil, err
	}
	budget.AddBrain(&out.Budget, resp.Usage.InputTokens, resp.Usage.OutputTokens)
	budget.AddBrainCache(&out.Budget, resp.Usage.CacheCreationInputTokens, resp.Usage.CacheReadInputTokens)
	var next envelope.Plan
	if err := json.Unmarshal([]byte(resp.Content), &next); err != nil {
		return nil, err
	}
	if err := validatePlan(next); err != nil {
		return nil, err
	}
	out.Plan = &next
	out.Done = next.Done
	return &out, nil
}

func validatePlan(plan envelope.Plan) error {
	raw, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	if err := schema.ValidatePlan(raw); err != nil {
		return err
	}
	if plan.Done {
		return nil
	}
	if plan.NextAction == nil {
		return fmt.Errorf("plan next_action is required when done is false")
	}
	switch plan.NextAction.Kind {
	case "run_command":
		if plan.NextAction.Command == "" {
			return fmt.Errorf("run_command requires command")
		}
	case "edit_file":
		if plan.NextAction.TargetPath == "" {
			return fmt.Errorf("edit_file requires target_path")
		}
	}
	return nil
}
