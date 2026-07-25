// Package extension defines Norbot's versioned process-plugin customization seam.
package extension

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gongahkia/norbot/internal/config"
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

// ProcessRegistry accepts only digest-pinned local executables. Plugins speak one
// JSON-RPC 2.0 request/response over stdio for every invocation.
type ProcessRegistry struct { clients map[string]ProcessClient }
type ProcessClient struct { spec config.ProcessPlugin; seq *atomic.Int64 }
type rpcRequest struct { JSONRPC string `json:"jsonrpc"`; ID int64 `json:"id"`; Method string `json:"method"`; Params any `json:"params"` }
type rpcResponse struct { JSONRPC string `json:"jsonrpc"`; ID int64 `json:"id"`; Result json.RawMessage `json:"result"`; Error *rpcError `json:"error"` }
type rpcError struct { Code int `json:"code"`; Message string `json:"message"` }

func LoadProcessPlugins(specs []config.ProcessPlugin) (*ProcessRegistry, error) {
	registry := &ProcessRegistry{clients: make(map[string]ProcessClient, len(specs))}
	for _, spec := range specs {
		if err := verifyDigest(spec.Command, spec.SHA256); err != nil { return nil, fmt.Errorf("verify plugin %q: %w", spec.ID, err) }
		if _, exists := registry.clients[spec.ID]; exists { return nil, fmt.Errorf("duplicate process plugin %q", spec.ID) }
		registry.clients[spec.ID] = ProcessClient{spec: spec, seq: &atomic.Int64{}}
	}
	return registry, nil
}

func (r *ProcessRegistry) IDs() []string {
	ids := make([]string, 0, len(r.clients))
	for id := range r.clients { ids = append(ids, id) }
	return ids
}

func (r *ProcessRegistry) Call(ctx context.Context, pluginID, method string, params any, result any) error {
	client, ok := r.clients[pluginID]
	if !ok { return fmt.Errorf("unknown process plugin %q", pluginID) }
	return client.Call(ctx, method, params, result)
}

func (p ProcessClient) Call(ctx context.Context, method string, params any, result any) error {
	if !allowedMethod(p.spec.Methods, method) { return fmt.Errorf("plugin %q does not allow method %q", p.spec.ID, method) }
	if err := verifyDigest(p.spec.Command, p.spec.SHA256); err != nil { return fmt.Errorf("plugin digest changed: %w", err) }
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, p.spec.Command)
	stdin, err := command.StdinPipe(); if err != nil { return err }
	stdout, err := command.StdoutPipe(); if err != nil { return err }
	stderr, err := command.StderrPipe(); if err != nil { return err }
	if err := command.Start(); err != nil { return err }
	id := p.seq.Add(1)
	request, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); if err != nil { return err }
	if _, err := stdin.Write(append(request, '\n')); err != nil { return err }
	if err := stdin.Close(); err != nil { return err }
	line, err := bufio.NewReader(io.LimitReader(stdout, 1<<20)).ReadBytes('\n'); if err != nil { return fmt.Errorf("plugin response: %w", err) }
	if err := command.Wait(); err != nil {
		stderrBytes, _ := io.ReadAll(io.LimitReader(stderr, 8<<10))
		return fmt.Errorf("plugin exited: %w: %s", err, strings.TrimSpace(string(stderrBytes)))
	}
	var response rpcResponse
	if err := json.Unmarshal(line, &response); err != nil { return fmt.Errorf("decode plugin response: %w", err) }
	if response.JSONRPC != "2.0" || response.ID != id { return fmt.Errorf("invalid plugin JSON-RPC response") }
	if response.Error != nil { return fmt.Errorf("plugin error %d: %s", response.Error.Code, response.Error.Message) }
	if result != nil && len(response.Result) > 0 { return json.Unmarshal(response.Result, result) }
	return nil
}

func verifyDigest(path, expected string) error {
	file, err := os.Open(path); if err != nil { return err }
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil { return err }
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimPrefix(expected, "sha256:")) { return fmt.Errorf("expected sha256:%s, got sha256:%s", expected, actual) }
	return nil
}

func allowedMethod(methods []string, method string) bool {
	for _, allowed := range methods { if allowed == method { return true } }
	return false
}
