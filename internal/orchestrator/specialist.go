package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

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
	"list_agents":       {},
	"steer_agent":       {},
	"delegate_agents":   {},
	"start_agent":       {},
	"inspect_agent":     {},
	"await_agent":       {},
	"cancel_agent":      {},
	"answer_agent":      {},
	"complete_code":     {},
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
			inspected := []string{}
			missing := []string{}
			seen := map[string]bool{}
			coverage := map[string]*readCoverage{}
			result, err := (agent.Runner{Model: agent.WithBudget(model, usage), Tools: tools, Now: now}).Run(ctx, agent.RunOptions{
				Task: invocation.Task, System: system, MaxSteps: maxSteps, OnEvent: invocation.OnEvent, Steering: invocation.Steering,
				AfterTool: func(_ context.Context, call agent.ToolCall, result agent.ToolResult, err error) error {
					if call.Name != "read_file" {
						return nil
					}
					if err != nil {
						if errors.Is(err, os.ErrNotExist) && len(missing) < 64 {
							var args struct {
								Path string `json:"path"`
							}
							if json.Unmarshal(call.Arguments, &args) == nil && strings.HasPrefix(path.Clean(args.Path), "candidate/") {
								missing = append(missing, path.Clean(args.Path))
							}
						}
						return nil
					}
					var read struct {
						OK     bool `json:"ok"`
						Result struct {
							Path  string `json:"path"`
							Start *int   `json:"start_offset"`
							End   *int   `json:"end_offset"`
							Total *int   `json:"total_bytes"`
						} `json:"result"`
					}
					if json.Unmarshal([]byte(result.Content), &read) == nil && read.OK && read.Result.Path != "" {
						r := read.Result
						if !seen[r.Path] && len(inspected) < 64 {
							seen[r.Path] = true
							inspected = append(inspected, r.Path)
						}
						if r.Start != nil && r.End != nil && r.Total != nil && (coverage[r.Path] != nil || len(coverage) < 64) {
							if coverage[r.Path] == nil {
								coverage[r.Path] = &readCoverage{total: *r.Total}
							}
							coverage[r.Path].add(*r.Start, *r.End, *r.Total)
						}
					}

					return nil
				},
			})
			fully := []string{}
			for _, name := range inspected {
				if item := coverage[name]; item != nil && item.complete() {
					fully = append(fully, name)
				}
			}
			return Result{FullyInspectedPaths: fully, InspectedPaths: inspected, MissingPaths: missing, Summary: result.FinalText, Steps: result.Steps, Usage: usage.Usage()}, err
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

// readCoverage is host-observed byte coverage, not a model claim. Bounded
// intervals support paged reads and reject incomplete final lines.
type readCoverage struct {
	total   int
	ranges  [][2]int
	invalid bool
}

func (c *readCoverage) add(start, end, total int) {
	if total != c.total || start < 0 || end < start || end > total || len(c.ranges) >= 4096 {
		c.invalid = true
		return
	}
	c.ranges = append(c.ranges, [2]int{start, end})
}
func (c *readCoverage) complete() bool {
	if c.invalid || len(c.ranges) == 0 {
		return false
	}
	sort.Slice(c.ranges, func(i, j int) bool { return c.ranges[i][0] < c.ranges[j][0] })
	end := 0
	for _, r := range c.ranges {
		if r[0] > end {
			return false
		}
		if r[1] > end {
			end = r[1]
		}
	}
	return end == c.total
}
