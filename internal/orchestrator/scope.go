package orchestrator

import (
	"encoding/json"
	"errors"

	"github.com/gongahkia/gator/internal/agent"
)

// ToolEffects resolves the actual registered template for plan scope checks.
// A custom name must never disguise a coding worker as a read-only operation.
func ToolEffects(tool agent.Tool, call agent.ToolCall) ([]string, bool, error) {
	var supervisor *Supervisor
	switch t := tool.(type) {
	case taskTool:
		supervisor = t.s
	case profileTool:
		supervisor = t.s
	case *delegateTool:
		supervisor = t.supervisor
	default:
		return nil, false, nil
	}
	if supervisor == nil {
		return nil, false, nil
	}
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	read := []string{"read"}
	if len(call.Arguments) == 0 {
		return read, true, nil
	} // Advertise; validate real calls below.
	roleEffects := func(name string) ([]string, error) {
		role, exists := supervisor.registry[name]
		if !exists {
			return nil, errors.New("unknown specialist scope")
		}
		if role.Backend == BackendHosted {
			return []string{"read", "code", "checks"}, nil
		}
		return read, nil
	}
	switch call.Name {
	case "create_agent":
		var input CreateAgentRequest
		if err := decodeTask(call.Arguments, &input); err != nil {
			return nil, true, err
		}
		definition, err := supervisor.resolveDefinition(input)
		if err != nil {
			return nil, true, err
		}
		result := read
		if input.Task != "" {
			result, err = roleEffects(definition.Base)
		}
		if input.Save {
			result = append(append([]string{}, result...), "settings")
		}
		return result, true, err
	case "agent_profiles":
		var input struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal(call.Arguments, &input); err != nil {
			return nil, true, err
		}
		if input.Action == "save" || input.Action == "delete" {
			return []string{"settings"}, true, nil
		}
		return read, true, nil
	case "start_agent":
		var input StartRequest
		if err := decodeTask(call.Arguments, &input); err != nil {
			return nil, true, err
		}
		effects, err := roleEffects(input.Agent)
		return effects, true, err
	case "delegate_agents":
		var input struct {
			Tasks []struct {
				Agent string `json:"agent"`
			} `json:"tasks"`
		}
		if err := json.Unmarshal(call.Arguments, &input); err != nil {
			return nil, true, err
		}
		result := read
		for _, task := range input.Tasks {
			effects, err := roleEffects(task.Agent)
			if err != nil {
				return nil, true, err
			}
			if len(effects) > len(result) {
				result = effects
			}
		}
		return result, true, nil
	case "steer_agent", "answer_agent":
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(call.Arguments, &input); err != nil {
			return nil, true, err
		}
		if execution := supervisor.tasks[input.ID]; execution != nil {
			if execution.role.Backend == BackendHosted {
				return []string{"read", "code", "checks"}, true, nil
			}
		}
		return read, true, nil
	default:
		return read, true, nil
	}
}
