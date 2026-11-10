package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/worksession"
)

// TaskPlanReference is supplied by the foreground owner after durable claim.
// A renderer/model cannot supply an approved plan object or execution grant.
type TaskPlanReference struct {
	RequestID string
	Revision  uint64
	Digest    string
}

func loadReviewedTaskPlan(request Request) (*worksession.TaskPlan, error) {
	if request.TaskPlan == nil {
		return nil, nil
	}
	store, err := worksession.Open(request.StateDir)
	if err != nil {
		return nil, err
	}
	j, err := store.LoadRequests(request.ConversationID)
	if err != nil {
		return nil, err
	}
	for _, r := range j.Requests {
		if r.ID != request.TaskPlan.RequestID {
			continue
		}
		if r.Plan == nil || r.State != "running" || r.Plan.Phase != "dispatching" || r.Plan.Revision != request.TaskPlan.Revision || r.Plan.Digest != request.TaskPlan.Digest || r.Text != request.Objective {
			return nil, worksession.ErrPlanChanged
		}
		if err := store.CheckPlanDispatch(request.ConversationID, r); err != nil {
			return nil, err
		}
		c, err := store.Load(request.ConversationID)
		if err != nil {
			return nil, err
		}
		if request.SourcePath != "" && request.SourcePath != "." {
			source, err := filepath.Abs(request.SourcePath)
			if err != nil || source != c.SourcePath {
				return nil, worksession.ErrPlanChanged
			}
		}
		var frozen struct {
			ConnectorIDs []string
			WebOrigins   []string
		}
		if json.Unmarshal(r.Options, &frozen) != nil || !slices.Equal(request.ConnectorIDs, r.Plan.Content.Sources) || !slices.Equal(request.WebOrigins, frozen.WebOrigins) {
			return nil, worksession.ErrPlanChanged
		}
		if r.Plan.Budget.MaxSteps > 0 && request.MaxSteps > r.Plan.Budget.MaxSteps || r.Plan.Budget.CodeMaxSteps > 0 && request.Code.MaxSteps > r.Plan.Budget.CodeMaxSteps {
			return nil, errors.New("execution limits exceed the reviewed plan")
		}
		return r.Plan.Clone(), nil
	}
	return nil, worksession.ErrPlanChanged
}

func planToolEffects(tool agent.Tool, call agent.ToolCall) ([]string, error) {
	if _, native := tool.(evidenceSearchTool); native {
		return []string{"read"}, nil
	}
	if _, native := tool.(webAccessTool); native {
		return []string{"read"}, nil
	}
	if effects, known, err := orchestrator.ToolEffects(tool, call); known {
		return effects, err
	}
	if _, capability, ok := tools.ConnectorOperationCapability(tool); ok {
		if action.RequiresFreshApproval(capability) {
			return []string{"connected_write"}, nil
		}
		return []string{"read"}, nil
	}
	name := call.Name
	switch name {
	case "read_file", "list_files", "search_files", "list_input_files", "extract_document", "inspect_table", "web_search", "http_fetch", "web_fetch", "list_evidence", "read_evidence", "check_claims", "artifact_status", "knowledge_read", "knowledge_search", "chat_read", "chat_search", "skill_list", "skill_read", "local_agenda", "gator_app_inspect":
		return []string{"read"}, nil
	case "write_artifact", "write_json_artifact", "write_table_artifact", "work_write_document", "work_write_workbook", "work_write_presentation", "reconcile_tables":
		return []string{"draft"}, nil
	case "memory_remember", "memory_forget", "local_plan", "suggest_change", "gator_app_update":
		return []string{"settings"}, nil
	case "code_use_integration":
		return nil, errors.New("this dynamic coding integration is not supported by plan-first scope checking")
	case "code_open", "code_read_file", "code_list_files", "code_search_files", "code_apply_patch", "code_run_command", "code_git_status", "code_git_diff", "code_stage", "code_terminal_start", "code_terminal_read", "code_terminal_write", "code_terminal_stop", "code_terminal_list", "code_terminal_detach", "complete_code", "integrate_code":
		return []string{"read", "code", "checks"}, nil
	case "browser_tabs", "browser_snapshot", "browser_screenshot":
		return []string{"read"}, nil
	case "browser_navigate", "browser_click", "browser_fill", "browser_select", "browser_press", "browser_download", "browser_upload":
		return []string{"browser"}, nil
	case "delegate_agents":
		if len(call.Arguments) > 128*1024 {
			return nil, errors.New("delegation scope exceeds its bound")
		}
		var args struct {
			Tasks []struct {
				Agent string `json:"agent"`
			} `json:"tasks"`
		}
		if json.Unmarshal(call.Arguments, &args) != nil || len(args.Tasks) == 0 || len(args.Tasks) > 24 {
			return nil, errors.New("delegation scope is unavailable")
		}
		effects := []string{"read"}
		for _, task := range args.Tasks {
			switch task.Agent {
			case "code", "cli_codex", "cli_claude", "cli_pi":
				effects = []string{"read", "code", "checks"}
			case "source_researcher", "artifact_reviewer", "claim_verifier", "connected_researcher", "spreadsheet_analyst", "goal_reviewer":
			default:
				return nil, errors.New("this specialist scope is not supported by plan-first")
			}
		}
		return effects, nil
	}
	return nil, errors.New("this operation is not supported by plan-first scope checking")
}

// Only registered native tools are classified. Unknown model tool names still
// receive the runner's ordinary unavailable-tool response. A registered effect
// outside the review stops the whole run before this or later calls dispatch.
func taskPlanToolCheck(request Request, surface []agent.Tool) func(context.Context, agent.ToolCall) error {
	if request.reviewedPlan == nil {
		return nil
	}
	registered := map[string]agent.Tool{}
	for _, tool := range surface {
		registered[tool.Definition().Name] = tool
	}
	return func(ctx context.Context, call agent.ToolCall) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		tool, known := registered[call.Name]
		if !known {
			return nil
		}
		plan, err := loadReviewedTaskPlan(request)
		if err != nil {
			return err
		}
		if id, _, connected := tools.ConnectorOperationCapability(tool); connected && !slices.Contains(plan.Content.Sources, id) {
			return errors.New("the connected source is outside the reviewed plan")
		}
		effects, err := planToolEffects(tool, call)
		if err != nil {
			return err
		}
		for _, effect := range effects {
			if !slices.Contains(plan.Content.Effects, effect) {
				return fmt.Errorf("the reviewed plan does not include %s effects; stop and review a revised plan before continuing", effect)
			}
		}
		return nil
	}
}
func taskPlanPrompt(plan *worksession.TaskPlan) string {
	if plan == nil || plan.Content == nil {
		return ""
	}
	raw, _ := json.Marshal(plan.Content)
	text := "Human-reviewed task intent (revision and digest are host-bound). Follow this outcome and steps; stop and ask for a revised plan if the scope changes. These are proposed steps, not verified completion. Effect scope never grants command, connected-service, browser, external-terminal or artifact-application permission. Later exact-action reviews and verification remain required.\n" + strings.TrimSpace(string(raw))
	if plan.Edit != nil && plan.Edit.State == "applied" {
		correction, _ := json.Marshal(plan.Edit.Instruction)
		text += "\nRetained user correction incorporated before this plan's fresh review (JSON): " + string(correction) + "\nThe original task text is retained for provenance. Resolve conflicting original details using the current reviewed outcome and steps. This correction does not grant new inputs, source access, execution budget or external-action permission."
	}
	return text
}

// Advertise the supported subset; retain the full host registry in the runtime
// check so a fabricated call to a hidden native mutation still stops safely.
func filterTaskPlanTools(request Request, surface []agent.Tool) []agent.Tool {
	if request.reviewedPlan == nil {
		return surface
	}
	selected := []agent.Tool{}
	for _, tool := range surface {
		name := tool.Definition().Name
		if id, _, connected := tools.ConnectorOperationCapability(tool); connected && !slices.Contains(request.reviewedPlan.Content.Sources, id) {
			continue
		}
		effects, err := planToolEffects(tool, agent.ToolCall{Name: name})
		if name == "delegate_agents" {
			effects = []string{"read"}
			err = nil
		}
		if err != nil {
			continue
		}
		allowed := true
		for _, effect := range effects {
			allowed = allowed && slices.Contains(request.reviewedPlan.Content.Effects, effect)
		}
		if allowed {
			selected = append(selected, tool)
		}
	}
	return selected
}
