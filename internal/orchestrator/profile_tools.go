package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"

	"github.com/gongahkia/gator/internal/agent"
)

type CreateAgentRequest struct {
	Name                string `json:"name"`
	Base                string `json:"base,omitempty"`
	Description         string `json:"description,omitempty"`
	Instructions        string `json:"instructions,omitempty"`
	MaxSteps            int    `json:"max_steps,omitempty"`
	FromRetained        string `json:"from_retained,omitempty"`
	ReplaceDigest       string `json:"replace_digest,omitempty"`
	Save                bool   `json:"save,omitempty"`
	ExpectedSavedDigest string `json:"expected_saved_digest,omitempty"`
	UserQuote           string `json:"user_quote,omitempty"`
	Task                string `json:"task,omitempty"`
}

type CreateAgentResult struct {
	Status        string            `json:"status"`
	Profile       AgentProfile      `json:"profile"`
	Configuration RoleConfiguration `json:"configuration"`
	Task          *Task             `json:"task,omitempty"`
	StartError    string            `json:"start_error,omitempty"`
	Warning       string            `json:"warning,omitempty"`
}

func (s *Supervisor) removeTaskProfile(name string) error {
	store := s.taskProfiles()
	definitions, err := store.List()
	if err != nil {
		return err
	}
	for _, d := range definitions {
		if d.Name == name {
			return store.Delete(name, d.Digest())
		}
	}
	return nil
}

func (s *Supervisor) taskProfiles() ProfileStore {
	return ProfileStore{directory: filepath.Join(s.options.StatePath, "roles")}
}

func ReadTaskProfiles(directory string) ([]AgentDefinition, error) {
	return (ProfileStore{directory: filepath.Join(directory, "roles")}).List()
}

func (s *Supervisor) derive(definition AgentDefinition, saved bool) (Specialist, error) {
	base, found := s.bases[definition.Base]
	if !found {
		return Specialist{}, errors.New("base role is unavailable in this task; inspect list_agents")
	}
	if _, reserved := s.bases[definition.Name]; reserved {
		return Specialist{}, errors.New("custom specialist cannot replace a built-in role")
	}
	return deriveSpecialist(base, definition, saved)
}

func (s *Supervisor) resolveDefinition(request CreateAgentRequest) (AgentDefinition, error) {
	definition := AgentDefinition{Version: 1, Name: request.Name, Base: request.Base, Description: request.Description, Instructions: request.Instructions, MaxSteps: request.MaxSteps}
	if request.FromRetained != "" {
		prior, found := s.options.RetainedProfiles[request.FromRetained]
		if !found {
			return AgentDefinition{}, errors.New("retained specialist is not in this conversation's selected lineage")
		}
		if request.Base != "" || request.Description != "" || request.Instructions != "" || request.MaxSteps != 0 {
			return AgentDefinition{}, errors.New("reuse a retained definition first, then revise it explicitly")
		}
		definition = prior
		if request.Name != "" {
			definition.Name = request.Name
		}
	}
	return definition.Normalize(), nil
}

func (s *Supervisor) Create(ctx context.Context, request CreateAgentRequest) (CreateAgentResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.options.CustomAgents || s.closed {
		return CreateAgentResult{}, errors.New("custom specialists are unavailable")
	}
	if err := ctx.Err(); err != nil {
		return CreateAgentResult{}, err
	}
	if err := s.ctx.Err(); err != nil {
		return CreateAgentResult{}, err
	}
	definition, err := s.resolveDefinition(request)
	if err != nil {
		return CreateAgentResult{}, err
	}
	role, err := s.derive(definition, request.Save)
	if err != nil {
		return CreateAgentResult{}, err
	}
	prior, exists := s.registry[definition.Name]
	if exists && (prior.Profile == nil || prior.Profile.Digest != request.ReplaceDigest) || !exists && request.ReplaceDigest != "" {
		return CreateAgentResult{}, ErrAgentProfileChanged
	}
	count := 0
	for _, registered := range s.registry {
		if registered.Profile != nil {
			count++
		}
	}
	if !exists && count >= MaxAgentProfiles {
		return CreateAgentResult{}, errors.New("task specialist definition limit reached")
	}
	if request.Save {
		if s.options.SaveProfile == nil {
			return CreateAgentResult{}, errors.New("saving specialists requires an explicit request in the foreground app")
		}
		if err := s.options.SaveProfile(ctx, definition, request.ExpectedSavedDigest, request.UserQuote); err != nil {
			return CreateAgentResult{}, err
		}
	} else {
		if request.ExpectedSavedDigest != "" || request.UserQuote != "" {
			return CreateAgentResult{}, errors.New("saved-library fields require save:true")
		}
		store := s.taskProfiles()
		profiles, err := store.List()
		if err != nil {
			return CreateAgentResult{}, err
		}
		expected := ""
		for _, profile := range profiles {
			if profile.Name == definition.Name {
				expected = profile.Digest()
			}
		}
		if err := store.PutContext(ctx, definition, expected); err != nil {
			return CreateAgentResult{}, err
		}
	}
	s.registry[definition.Name] = role
	delete(s.unavailableProfiles, definition.Name)
	result := CreateAgentResult{Status: "created", Profile: *cloneProfile(role.Profile), Configuration: role.Configuration}
	if request.Save {
		if err := s.removeTaskProfile(definition.Name); err != nil {
			result.Warning = "Saved, but an older task-local definition could not be removed from this task's history."
		}
	}
	if request.Task != "" {
		task, err := s.startLocked(StartRequest{Agent: definition.Name, Task: request.Task})
		if err != nil {
			result.StartError = err.Error()
		} else {
			result.Task = &task
			result.Status = "started"
		}
	}
	return result, nil
}

// Caller holds s.mu. Retained profiles are inert until explicitly selected.
func (s *Supervisor) retainedProfileIndex() []map[string]any {
	keys := make([]string, 0, len(s.options.RetainedProfiles))
	for key := range s.options.RetainedProfiles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 64 {
		keys = keys[len(keys)-64:]
	}
	result := []map[string]any{}
	for _, key := range keys {
		d := s.options.RetainedProfiles[key]
		result = append(result, map[string]any{"id": key, "name": d.Name, "base": d.Base, "description": d.Description, "digest": d.Digest()})
	}
	return result
}

type profileTool struct {
	s    *Supervisor
	name string
}

func (t profileTool) Definition() agent.ToolDefinition {
	if t.name == "create_agent" {
		return agent.ToolDefinition{Name: t.name, Description: "Create a named specialist on the fly from an available native base in list_agents. Give it focused instructions; it inherits the base's model, tools and shared budget and cannot expand permissions or recursively delegate. Optional task starts it asynchronously immediately. Default lifetime is this task; save:true makes it reusable only when the user explicitly asks, with an exact user_quote from the current user request. Use replace_digest to revise an existing task definition and expected_saved_digest for a saved-library overwrite. from_retained explicitly reuses a definition ID returned by list_agents.", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["name"],"properties":{"name":{"type":"string","maxLength":64},"base":{"type":"string","maxLength":64},"description":{"type":"string","maxLength":1024},"instructions":{"type":"string","maxLength":8192},"max_steps":{"type":"integer","minimum":0,"maximum":32},"from_retained":{"type":"string","maxLength":256},"replace_digest":{"type":"string","maxLength":64},"save":{"type":"boolean"},"expected_saved_digest":{"type":"string","maxLength":64},"user_quote":{"type":"string","maxLength":4096},"task":{"type":"string","maxLength":8192}}}`)}
	}
	return agent.ToolDefinition{Name: t.name, Description: "Inspect, save or delete reusable specialist profiles. list shows descriptions/revisions; inspect shows the exact definition. save copies the current task specialist only after the user explicitly asks; provide its definition_digest, the expected_saved_digest (empty for a new saved name), and an exact user_quote from the current request. delete requires the saved revision and user quote; it prevents new starts but does not cancel running workers or erase history. Use cancel_agent for running tasks.", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["action"],"properties":{"action":{"type":"string","enum":["list","inspect","save","delete"]},"name":{"type":"string","maxLength":64},"definition_digest":{"type":"string","maxLength":64},"expected_saved_digest":{"type":"string","maxLength":64},"user_quote":{"type":"string","maxLength":4096}}}`)}
}

func (t profileTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if len(raw) == 0 || len(raw) > 128*1024 {
		return agent.ToolResult{}, errors.New("specialist request exceeds its input bound")
	}
	if t.name == "create_agent" {
		var request CreateAgentRequest
		if err := decodeTask(raw, &request); err != nil {
			return agent.ToolResult{}, err
		}
		result, err := t.s.Create(ctx, request)
		if err != nil {
			return agent.ToolResult{}, err
		}
		data, err := json.Marshal(result)
		return agent.ToolResult{Content: string(data)}, err
	}
	var input struct {
		Action              string `json:"action"`
		Name                string `json:"name"`
		DefinitionDigest    string `json:"definition_digest"`
		ExpectedSavedDigest string `json:"expected_saved_digest"`
		UserQuote           string `json:"user_quote"`
	}
	if err := decodeTask(raw, &input); err != nil {
		return agent.ToolResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if t.s.closed {
		return agent.ToolResult{}, errors.New("supervisor closed")
	}
	var result any
	switch input.Action {
	case "list", "inspect":
		if t.s.options.ListProfiles == nil {
			return agent.ToolResult{}, errors.New("saved specialists are unavailable in this context")
		}
		profiles, err := t.s.options.ListProfiles()
		if err != nil {
			return agent.ToolResult{}, err
		}
		if input.Action == "inspect" {
			if role, ok := t.s.registry[input.Name]; ok && role.Profile != nil {
				result = cloneProfile(role.Profile)
			} else {
				for _, d := range profiles {
					if d.Name == input.Name {
						result = AgentProfile{Definition: d, Digest: d.Digest(), Saved: true}
						break
					}
				}
				if result == nil {
					return agent.ToolResult{}, errors.New("unknown specialist profile")
				}
			}
		} else {
			items := []map[string]any{}
			for _, d := range profiles {
				current, exists := t.s.registry[d.Name]
				available := exists && current.Profile != nil && current.Profile.Digest == d.Digest()
				items = append(items, map[string]any{"name": d.Name, "base": d.Base, "description": d.Description, "digest": d.Digest(), "available_in_this_task": available})
			}
			result = map[string]any{"profiles": items}
		}
	case "save":
		role, ok := t.s.registry[input.Name]
		if !ok || role.Profile == nil || role.Profile.Digest != input.DefinitionDigest {
			return agent.ToolResult{}, ErrAgentProfileChanged
		}
		if t.s.options.SaveProfile == nil {
			return agent.ToolResult{}, errors.New("saving specialists requires an explicit request in the foreground app")
		}
		if err := t.s.options.SaveProfile(ctx, role.Profile.Definition, input.ExpectedSavedDigest, input.UserQuote); err != nil {
			return agent.ToolResult{}, err
		}
		role.Profile = cloneProfile(role.Profile)
		role.Profile.Saved = true
		t.s.registry[input.Name] = role
		result = map[string]any{"status": "saved", "name": input.Name, "digest": role.Profile.Digest}
		if err := t.s.removeTaskProfile(input.Name); err != nil {
			result.(map[string]any)["warning"] = "Saved, but an older task-local definition could not be removed from this task's history."
		}
	case "delete":
		if t.s.options.DeleteProfile == nil {
			return agent.ToolResult{}, errors.New("deleting specialists requires an explicit request in the foreground app")
		}
		if err := t.s.options.DeleteProfile(ctx, input.Name, input.ExpectedSavedDigest, input.UserQuote); err != nil {
			return agent.ToolResult{}, err
		}
		if role, exists := t.s.registry[input.Name]; exists && role.Profile != nil {
			delete(t.s.registry, input.Name)
		}
		result = map[string]any{"status": "deleted", "name": input.Name, "note": "Running tasks keep their captured definition. Retained task history is unchanged."}
		if err := t.s.removeTaskProfile(input.Name); err != nil {
			result.(map[string]any)["warning"] = "Saved profile deleted, but a task-local history copy could not be removed."
		}
	default:
		return agent.ToolResult{}, errors.New("unknown specialist profile action")
	}
	data, err := json.Marshal(result)
	return agent.ToolResult{Content: string(data)}, err
}
