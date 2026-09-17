package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
)

// Backend identifies how a specialist executes. The manager only sees
// Specialist; backends are a host concern.
const (
	BackendLLM    Backend = "llm"
	BackendHosted Backend = "hosted"
)

// Backend is one specialist execution class in the closed registry.
type Backend string

var hostReservedTools = map[string]struct{}{
	"delegate_agents":   {},
	"start_agent":       {},
	"inspect_agent":     {},
	"await_agent":       {},
	"cancel_agent":      {},
	"delegate_readonly": {},
	"delegate_writer":   {},
	"delegate_writers":  {},
}

func (s Specialist) validate() error {
	if !specialistNamePattern.MatchString(s.Name) || s.Description == "" || len(s.Description) > 1024 || s.Run == nil {
		if s.Name == "" {
			return fmt.Errorf("specialist is invalid")
		}
		return fmt.Errorf("specialist %q is invalid", s.Name)
	}
	switch s.Backend {
	case BackendLLM, BackendHosted:
		return nil
	default:
		return fmt.Errorf("specialist %q backend must be %q or %q", s.Name, BackendLLM, BackendHosted)
	}
}

// LLMSpecialist adapts Gator's provider-neutral agent runner into a
// fresh-context specialist. It shares the same Result contract as hosted
// specialists; artifact and baseline fields stay empty unless a caller wraps Run.
func LLMSpecialist(name, description string, model agent.Model, tools []agent.Tool, system string, maxSteps int, now func() time.Time) Specialist {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	policyErr := hostPolicyTools(name, tools)
	return Specialist{
		Backend:     BackendLLM,
		Name:        name,
		Description: description,
		Run: func(ctx context.Context, invocation Invocation) (Result, error) {
			if policyErr != nil {
				return Result{}, policyErr
			}
			usage := &agent.Budget{Limits: agent.Limits{ModelRequests: 4096}}
			result, err := (agent.Runner{Model: agent.WithBudget(model, usage), Tools: tools, Now: now}).Run(ctx, agent.RunOptions{
				Task: invocation.Task, System: system, MaxSteps: maxSteps, OnEvent: invocation.OnEvent,
			})
			return Result{Summary: result.FinalText, Steps: result.Steps, Usage: usage.Usage()}, err
		},
	}
}

// HostedSpecialist registers an isolated host engine as a manager-visible
// specialist. Code is the current hosted backend: it returns the same Result
// fields as LLM specialists and fills artifact/baseline evidence when it has a patch.
func HostedSpecialist(name, description string, run func(context.Context, Invocation) (Result, error)) Specialist {
	return Specialist{
		Backend:     BackendHosted,
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		Run:         run,
	}
}

func hostPolicyTools(specialist string, tools []agent.Tool) error {
	for _, tool := range tools {
		if tool == nil {
			return fmt.Errorf("specialist %q includes a nil tool", specialist)
		}
		name := strings.TrimSpace(tool.Definition().Name)
		if _, reserved := hostReservedTools[name]; reserved {
			return fmt.Errorf("specialist %q cannot use host tool %q", specialist, name)
		}
		if strings.Contains(name, "publish") {
			return fmt.Errorf("specialist %q cannot publish", specialist)
		}
	}
	return nil
}
