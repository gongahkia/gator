package core

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

type Provider interface {
	ID() string
	Status(context.Context) ProviderStatus
	Start(context.Context, Workspace, string, io.Reader, io.Writer, io.Writer) (*exec.Cmd, error)
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	registry := &Registry{providers: map[string]Provider{}}
	for _, provider := range providers {
		registry.providers[provider.ID()] = provider
	}
	return registry
}

func DefaultRegistry() *Registry {
	return NewRegistry(
		commandProvider{id: "codex", executable: "codex", argv: func(prompt string) []string { return []string{"exec", prompt} }, capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "pi", executable: "pi", capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "aider", executable: "aider", capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "amp", executable: "amp", capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "cline", executable: "cline", capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "copilot", executable: "copilot", capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "cursor", executable: "cursor-agent", capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "gemini", executable: "gemini", capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "goose", executable: "goose", capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "kimi", executable: "kimi", capabilities: ProviderCapabilities{Terminal: true}},
		probeOnlyProvider{id: "vibe", executable: "vibe", capabilities: ProviderCapabilities{Terminal: true}},
	)
}

func (r *Registry) Statuses(ctx context.Context) []ProviderStatus {
	statuses := make([]ProviderStatus, 0, len(r.providers))
	values := make(chan ProviderStatus, len(r.providers))
	var group sync.WaitGroup
	for _, provider := range r.providers {
		group.Add(1)
		go func(value Provider) {
			defer group.Done()
			values <- value.Status(ctx)
		}(provider)
	}
	group.Wait()
	close(values)
	for value := range values {
		statuses = append(statuses, value)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].ID < statuses[j].ID })
	return statuses
}

func (r *Registry) Provider(id string) (Provider, bool) {
	provider, ok := r.providers[id]
	return provider, ok
}

type probeOnlyProvider struct {
	id           string
	executable   string
	capabilities ProviderCapabilities
}

func (p probeOnlyProvider) ID() string { return p.id }

func (p probeOnlyProvider) Status(ctx context.Context) ProviderStatus {
	path, err := exec.LookPath(p.executable)
	if err != nil {
		return ProviderStatus{ID: p.id, Executable: p.executable, Capabilities: p.capabilities, Reason: "executable is unavailable"}
	}
	return ProviderStatus{ID: p.id, Discovered: true, Executable: path, Version: probeVersion(ctx, path), Capabilities: p.capabilities, Reason: "discovered; standalone execution adapter is not implemented"}
}

func (p probeOnlyProvider) Start(context.Context, Workspace, string, io.Reader, io.Writer, io.Writer) (*exec.Cmd, error) {
	return nil, fmt.Errorf("provider %s has no standalone execution adapter", p.id)
}

type commandProvider struct {
	id           string
	executable   string
	argv         func(string) []string
	capabilities ProviderCapabilities
}

func (p commandProvider) ID() string { return p.id }

func (p commandProvider) Status(ctx context.Context) ProviderStatus {
	path, err := exec.LookPath(p.executable)
	if err != nil {
		return ProviderStatus{ID: p.id, Executable: p.executable, Capabilities: p.capabilities, Reason: "executable is unavailable"}
	}
	return ProviderStatus{ID: p.id, Available: true, Discovered: true, Executable: path, Version: probeVersion(ctx, path), Capabilities: p.capabilities}
}

func (p commandProvider) Start(ctx context.Context, workspace Workspace, prompt string, input io.Reader, output, errors io.Writer) (*exec.Cmd, error) {
	path, err := exec.LookPath(p.executable)
	if err != nil {
		return nil, fmt.Errorf("provider %s executable: %w", p.id, err)
	}
	args := p.argv(prompt)
	command := exec.CommandContext(ctx, path, args...)
	command.Dir, command.Stdin, command.Stdout, command.Stderr = workspace.Root, input, output, errors
	command.WaitDelay = 5 * time.Second
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return command.Process.Kill()
	}
	return command, nil
}

func probeVersion(ctx context.Context, executable string) string {
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probe, executable, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
