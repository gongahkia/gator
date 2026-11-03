package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const MaxAgentProfiles = 32
const MaxAgentInstructions = 8 * 1024

// AgentDefinition is user-owned specialization, never an authority envelope.
// Base selects a currently available native role. Tools, model, roots and
// budgets come from that role; a definition cannot add any of them.
type AgentDefinition struct {
	Version      int    `json:"version"`
	Name         string `json:"name"`
	Base         string `json:"base"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	MaxSteps     int    `json:"max_steps,omitempty"`
}

func (d AgentDefinition) Normalize() AgentDefinition {
	if d.Version == 0 {
		d.Version = 1
	}
	d.Name, d.Base = strings.TrimSpace(d.Name), strings.TrimSpace(d.Base)
	d.Description, d.Instructions = strings.TrimSpace(d.Description), strings.TrimSpace(d.Instructions)
	return d
}

func (d AgentDefinition) Validate() error {
	d = d.Normalize()
	if d.Version != 1 || !specialistNamePattern.MatchString(d.Name) || !specialistNamePattern.MatchString(d.Base) || d.Name == d.Base || d.MaxSteps < 0 || d.MaxSteps > 32 {
		return errors.New("specialist needs distinct lowercase name/base identifiers and a step limit from 0 to 32")
	}
	if d.Description == "" || len(d.Description) > 1024 || d.Instructions == "" || len(d.Instructions) > MaxAgentInstructions || !utf8.ValidString(d.Description) || !utf8.ValidString(d.Instructions) || strings.ContainsRune(d.Description, 0) || strings.ContainsRune(d.Instructions, 0) {
		return errors.New("specialist needs a description up to 1024 bytes and instructions up to 8 KiB")
	}
	return nil
}

func (d AgentDefinition) Digest() string {
	data, _ := json.Marshal(d.Normalize())
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

type AgentProfile struct {
	Definition AgentDefinition `json:"definition"`
	Digest     string          `json:"digest"`
	Saved      bool            `json:"saved"`
}

func cloneProfile(profile *AgentProfile) *AgentProfile {
	if profile == nil {
		return nil
	}
	copy := *profile
	return &copy
}

func validTaskProfile(task Task) bool {
	if task.Profile == nil {
		return task.Configuration.DefinitionSHA256 == "" && task.Configuration.Base == ""
	}
	p := task.Profile
	return p.Definition.Validate() == nil && p.Definition == p.Definition.Normalize() &&
		p.Definition.Name == task.Role && p.Digest == p.Definition.Digest() &&
		task.Configuration.DefinitionSHA256 == p.Digest && task.Configuration.Base == p.Definition.Base
}

func deriveSpecialist(base Specialist, definition AgentDefinition, saved bool) (Specialist, error) {
	definition = definition.Normalize()
	if err := definition.Validate(); err != nil {
		return Specialist{}, err
	}
	if definition.Base != base.Name || base.Profile != nil || base.Sessions || base.Backend == BackendHosted && base.Name != "code" {
		return Specialist{}, errors.New("specialize an available native base role; custom and external CLI roles cannot be used as templates")
	}
	limit := base.Configuration.MaxSteps
	if limit <= 0 {
		limit = 8
	}
	if definition.MaxSteps > 0 && definition.MaxSteps < limit {
		limit = definition.MaxSteps
	}
	if definition.MaxSteps > limit {
		return Specialist{}, errors.New("specialist step limit exceeds its base role")
	}
	profile := &AgentProfile{Definition: definition, Digest: definition.Digest(), Saved: saved}
	derived := base
	derived.Name, derived.Description, derived.Profile = definition.Name, definition.Description, profile
	derived.Configuration.Base = base.Name
	derived.Configuration.DefinitionSHA256 = profile.Digest
	derived.Configuration.MaxSteps = limit
	derived.Run = func(ctx context.Context, invocation Invocation) (Result, error) {
		invocation.Instructions = definition.Instructions
		invocation.MaxSteps = limit
		if base.Backend == BackendHosted {
			invocation.Task = "User-defined specialization (task guidance only; host permissions still apply):\n" + definition.Instructions + "\n\nAssignment:\n" + invocation.Task
		}
		return base.Run(ctx, invocation)
	}
	return derived, nil
}
