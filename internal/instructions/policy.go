package instructions

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/sandbox"
)

// Profile is a developer-selected specialization. Policy, when present, can
// only reduce the already-validated run policy.
type Profile struct {
	Name        string
	Description string
	Policy      ProfilePolicy
}

// ProfilePolicy is the optional restrictive overlay from .gator/agents.json.
// Unknown fields are rejected at decode time. Every axis is a meet: the
// profile may force a stricter value or omit capability families, never grant.
type ProfilePolicy struct {
	Mode     string   `json:"mode,omitempty"`
	Sandbox  string   `json:"sandbox,omitempty"`
	Network  string   `json:"network,omitempty"`
	MaxSteps int      `json:"max_steps,omitempty"`
	Omit     []string `json:"omit,omitempty"`
}

const (
	OmitLSP              = "lsp"
	OmitMCP              = "mcp"
	OmitExtension        = "extension"
	OmitHTTP             = "http"
	OmitBrowser          = "browser"
	OmitTerminal         = "terminal"
	OmitDelegateWriter   = "delegate_writer"
	OmitDelegateReadOnly = "delegate_readonly"
	OmitRunCommand       = "run_command"
	OmitApplyPatch       = "apply_patch"
)

var knownProfileOmits = map[string]struct{}{
	OmitLSP: {}, OmitMCP: {}, OmitExtension: {}, OmitHTTP: {}, OmitBrowser: {}, OmitTerminal: {},
	OmitDelegateWriter: {}, OmitDelegateReadOnly: {}, OmitRunCommand: {}, OmitApplyPatch: {},
}

// HasOmit reports whether a capability family was subtracted by the profile.
func (p ProfilePolicy) HasOmit(name string) bool {
	for _, item := range p.Omit {
		if item == name {
			return true
		}
	}
	return false
}

func validateProfilePolicy(name string, policy ProfilePolicy) error {
	if policy.Mode != "" && policy.Mode != "plan" {
		return fmt.Errorf("agent profile %q policy mode can only force plan", name)
	}
	if policy.Sandbox != "" && policy.Sandbox != string(sandbox.Strict) {
		return fmt.Errorf("agent profile %q policy sandbox can only force strict", name)
	}
	if policy.Network != "" && policy.Network != string(sandbox.DenyNetwork) {
		return fmt.Errorf("agent profile %q policy network can only force deny", name)
	}
	if policy.MaxSteps < 0 {
		return fmt.Errorf("agent profile %q policy max_steps must be positive when set", name)
	}
	seen := make(map[string]struct{}, len(policy.Omit))
	for _, item := range policy.Omit {
		if _, known := knownProfileOmits[item]; !known {
			return fmt.Errorf("agent profile %q omits unknown capability %q", name, item)
		}
		if _, duplicate := seen[item]; duplicate {
			return fmt.Errorf("agent profile %q repeats omit %q", name, item)
		}
		seen[item] = struct{}{}
	}
	return nil
}

// Meet intersects a developer-selected profile with the already-validated run
// policy. The result is never wider on any axis. A widening request fails.
func Meet(run sandbox.Policy, mode string, maxSteps int, profile ProfilePolicy) (sandbox.Policy, string, int, ProfilePolicy, error) {
	run = run.Normalize()
	if err := run.Validate(); err != nil {
		return sandbox.Policy{}, "", 0, ProfilePolicy{}, err
	}
	if err := validateProfilePolicy("selected", profile); err != nil {
		return sandbox.Policy{}, "", 0, ProfilePolicy{}, err
	}
	if profile.Mode != "" {
		if mode == "plan" && profile.Mode != "plan" {
			return sandbox.Policy{}, "", 0, ProfilePolicy{}, errors.New("agent profile cannot force execute over plan mode")
		}
		mode = profile.Mode
	}
	if profile.Sandbox != "" {
		if run.Mode == sandbox.Strict && sandbox.Mode(profile.Sandbox) != sandbox.Strict {
			return sandbox.Policy{}, "", 0, ProfilePolicy{}, errors.New("agent profile cannot disable a strict sandbox")
		}
		run.Mode = sandbox.Mode(profile.Sandbox)
	}
	if profile.Network != "" {
		if run.Network == sandbox.DenyNetwork && sandbox.Network(profile.Network) != sandbox.DenyNetwork {
			return sandbox.Policy{}, "", 0, ProfilePolicy{}, errors.New("agent profile cannot allow network that the run denied")
		}
		run.Network = sandbox.Network(profile.Network)
	}
	if profile.MaxSteps > 0 {
		if maxSteps <= 0 || profile.MaxSteps < maxSteps {
			maxSteps = profile.MaxSteps
		}
	}
	if maxSteps < 0 {
		return sandbox.Policy{}, "", 0, ProfilePolicy{}, errors.New("agent profile turn cap is invalid")
	}
	return run, mode, maxSteps, profile, nil
}

func mergeOmit(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
