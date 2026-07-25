// Package extension defines Norbot's versioned in-process customization seam.
package extension

import (
	"context"
	"fmt"

	"github.com/gongahkia/norbot/internal/domain"
)

const APIVersion = "v1"

type ProviderAdapter interface {
	ID() string
	APIVersion() string
	Supports(domain.Stage) bool
	Invoke(context.Context, Request) (Response, error)
}

type Tool interface {
	ID() string
	APIVersion() string
	Spec() ToolSpec
}

type Profile interface {
	ID() string
	APIVersion() string
	Generate(context.Context, GenerateRequest) (GenerateResult, error)
}

type Request struct {
	RunID  string
	Stage  domain.Stage
	Prompt string
}
type Response struct {
	Text     string
	Metadata map[string]any
}
type ToolSpec struct {
	Kind             string
	ApprovalRequired bool
}
type GenerateRequest struct {
	RunID   string
	Profile domain.Profile
}
type GenerateResult struct{ Files []string }

type Registry struct {
	Providers map[string]ProviderAdapter
	Tools     map[string]Tool
	Profiles  map[string]Profile
}

func NewRegistry() *Registry {
	return &Registry{Providers: map[string]ProviderAdapter{}, Tools: map[string]Tool{}, Profiles: map[string]Profile{}}
}
func (r *Registry) RegisterProvider(adapter ProviderAdapter) error {
	if adapter.APIVersion() != APIVersion {
		return fmt.Errorf("provider %q targets %s, need %s", adapter.ID(), adapter.APIVersion(), APIVersion)
	}
	if _, exists := r.Providers[adapter.ID()]; exists {
		return fmt.Errorf("duplicate provider %q", adapter.ID())
	}
	r.Providers[adapter.ID()] = adapter
	return nil
}
func (r *Registry) RegisterTool(tool Tool) error {
	if tool.APIVersion() != APIVersion {
		return fmt.Errorf("tool %q targets %s, need %s", tool.ID(), tool.APIVersion(), APIVersion)
	}
	if _, exists := r.Tools[tool.ID()]; exists {
		return fmt.Errorf("duplicate tool %q", tool.ID())
	}
	r.Tools[tool.ID()] = tool
	return nil
}
func (r *Registry) RegisterProfile(profile Profile) error {
	if profile.APIVersion() != APIVersion {
		return fmt.Errorf("profile %q targets %s, need %s", profile.ID(), profile.APIVersion(), APIVersion)
	}
	if _, exists := r.Profiles[profile.ID()]; exists {
		return fmt.Errorf("duplicate profile %q", profile.ID())
	}
	r.Profiles[profile.ID()] = profile
	return nil
}
