package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/workspace"
	"os"
	"path/filepath"
	"sync"
)

type codeIntegration struct {
	mu       sync.Mutex
	request  Request
	work     workspace.Work
	patches  map[string]string
	accepted map[string]string
	records  []patch.Candidate
	next     int
}

func (c *codeIntegration) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "integrate_code", Description: "Select retained code patches, detect conflicts, and verify the combined frozen candidate. Only a verified candidate can seed another Code assignment. Failed attempts remain evidence.", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["patches"],"properties":{"patches":{"type":"array","minItems":1,"maxItems":16,"items":{"type":"string"}}}}`)}
}
func (c *codeIntegration) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var input struct {
		Patches []string `json:"patches"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return agent.ToolResult{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[string]bool{}
	var payloads [][]byte
	for _, name := range input.Patches {
		if seen[name] {
			return agent.ToolResult{}, errors.New("patch selected twice")
		}
		seen[name] = true
		expected := c.patches[name]
		if expected == "" {
			expected = c.accepted[name]
		}
		if expected == "" {
			return agent.ToolResult{}, fmt.Errorf("patch %q has no successful retained Code evidence", name)
		}
		data, err := c.work.Output.ReadRegularFile(name, 16*1024*1024)
		if err != nil {
			return agent.ToolResult{}, err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != expected {
			return agent.ToolResult{}, errors.New("selected patch changed after creation")
		}
		payloads = append(payloads, data)
	}
	c.next++
	directory := filepath.Join(c.work.Scratch.Path(), fmt.Sprintf("integration-%03d", c.next))
	candidate, runErr := patch.Integrate(ctx, c.work.Source.Path(), directory, payloads, input.Patches, c.request.Code.Verification, c.request.Code.Sandbox)
	if candidate.ID == "" {
		candidate.ID = fmt.Sprintf("failed-%03d", c.next)
	}
	name := "code/" + c.request.RunID + "-" + candidate.ID + ".patch"
	candidate.PatchPath = name
	if len(candidate.Patch) > 0 {
		if err := artifact.WriteBinary(c.work.Output, c.request.Contract, name, candidate.Patch); err != nil {
			return agent.ToolResult{}, err
		}
	}
	if candidate.Status == "verified" {
		c.accepted[name] = candidate.SHA256
	}
	c.records = append(c.records, candidate)
	data, err := json.Marshal(candidate)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(c.request.StateDir, "gator", "candidates", c.request.RunID), 0700); err != nil {
		return agent.ToolResult{}, err
	}
	if err := os.WriteFile(filepath.Join(c.request.StateDir, "gator", "candidates", c.request.RunID, candidate.ID+".json"), data, 0600); err != nil {
		return agent.ToolResult{}, err
	}
	report := struct {
		Candidate patch.Candidate `json:"candidate"`
		Patch     string          `json:"patch"`
		Error     string          `json:"error,omitempty"`
	}{Candidate: candidate, Patch: name}
	if runErr != nil {
		report.Error = runErr.Error()
	}
	encoded, err := json.Marshal(report)
	return agent.ToolResult{Content: string(encoded)}, err
}
func (c *codeIntegration) baseline(name string) ([]byte, error) {
	if name == "" {
		return nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	expected := c.accepted[name]
	if expected == "" {
		return nil, errors.New("baseline must select a verified candidate")
	}
	data, err := c.work.Output.ReadRegularFile(name, 16*1024*1024)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expected {
		return nil, errors.New("baseline candidate changed")
	}
	return data, nil
}
