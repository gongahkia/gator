package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

type evidenceCatalog struct {
	mu      sync.Mutex
	work    workspace.Work
	entries map[string]artifact.Evidence
}

func newCatalog(work workspace.Work, source snapshot.Manifest) *evidenceCatalog {
	c := &evidenceCatalog{work: work, entries: map[string]artifact.Evidence{}}
	for _, entry := range source.Entries {
		if entry.Bytes <= 512*1024 {
			id := "local-" + entry.SHA256[:16] + "-" + fmt.Sprint(len(c.entries))
			c.entries[id] = artifact.Evidence{ID: id, Locator: "source/" + entry.Path, SHA256: entry.SHA256, RetrievedAt: source.CreatedAt}
		}
	}
	return c
}
func (c *evidenceCatalog) retain(locator string, data []byte, at time.Time) (artifact.Evidence, error) {
	if len(data) > 512*1024 {
		return artifact.Evidence{}, errors.New("evidence exceeds 512 KiB")
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	id := "evidence-" + digest[:24]
	entry := artifact.Evidence{ID: id, Locator: locator, SHA256: digest, RetrievedAt: at, SnapshotPath: "evidence/" + digest + ".txt"}
	if err := os.MkdirAll(filepath.Join(c.work.Path, "evidence"), 0700); err != nil {
		return entry, err
	}
	if err := os.WriteFile(filepath.Join(c.work.Path, entry.SnapshotPath), data, 0600); err != nil {
		return entry, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = entry
	return entry, nil
}
func (c *evidenceCatalog) list() []artifact.Evidence {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries := make([]artifact.Evidence, 0, len(c.entries))
	for _, entry := range c.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}
func (c *evidenceCatalog) read(id string) (artifact.Evidence, []byte, error) {
	c.mu.Lock()
	entry, ok := c.entries[id]
	c.mu.Unlock()
	if !ok {
		return entry, nil, errors.New("unknown selected evidence")
	}
	var data []byte
	var err error
	if entry.SnapshotPath != "" {
		root, openErr := workspace.Open(c.work.Path)
		if openErr != nil {
			return entry, nil, openErr
		}
		data, err = root.ReadRegularFile(entry.SnapshotPath, 512*1024)
	} else {
		data, err = c.work.Source.ReadRegularFile(strings.TrimPrefix(entry.Locator, "source/"), 512*1024)
	}
	if err != nil {
		return entry, nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != entry.SHA256 {
		return entry, nil, errors.New("evidence content changed")
	}
	return entry, data, nil
}

type evidenceTool struct {
	catalog *evidenceCatalog
	name    string
}

func (t evidenceTool) Definition() agent.ToolDefinition {
	schema := `{"type":"object","additionalProperties":false,"properties":{}}`
	if t.name == "read_evidence" {
		schema = `{"type":"object","additionalProperties":false,"required":["id"],"properties":{"id":{"type":"string"}}}`
	}
	if t.name == "check_claims" {
		schema = `{"type":"object","additionalProperties":false,"required":["claims"],"properties":{"claims":{"type":"array","maxItems":64,"items":{"type":"object","required":["claim","evidence_id","quote"],"properties":{"claim":{"type":"string"},"evidence_id":{"type":"string"},"quote":{"type":"string"}}}}}}`
	}
	return agent.ToolDefinition{Name: t.name, Description: "Inspect the selected-source evidence catalog or check exact claim quotations. A quotation match verifies citation integrity, not semantic entailment; report conflicting or unsupported claims separately.", Parameters: json.RawMessage(schema)}
}
func (t evidenceTool) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var value any
	switch t.name {
	case "list_evidence":
		value = t.catalog.list()
	case "read_evidence":
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return agent.ToolResult{}, err
		}
		entry, data, err := t.catalog.read(input.ID)
		if err != nil {
			return agent.ToolResult{}, err
		}
		retained, err := t.catalog.retain(entry.Locator, data, entry.RetrievedAt)
		if err != nil {
			return agent.ToolResult{}, err
		}
		value = map[string]any{"evidence": retained, "text": string(data)}
	case "check_claims":
		var input struct {
			Claims []struct {
				Claim string `json:"claim"`
				ID    string `json:"evidence_id"`
				Quote string `json:"quote"`
			} `json:"claims"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return agent.ToolResult{}, err
		}
		if len(input.Claims) > 64 {
			return agent.ToolResult{}, errors.New("too many claims")
		}
		checks := []map[string]any{}
		for _, claim := range input.Claims {
			_, data, err := t.catalog.read(claim.ID)
			matched := err == nil && len(claim.Quote) > 0 && strings.Contains(string(data), claim.Quote)
			checks = append(checks, map[string]any{"claim": claim.Claim, "evidence_id": claim.ID, "quote_match": matched, "semantic_entailment": "ungraded"})
		}
		value = checks
	}
	data, err := json.Marshal(value)
	return agent.ToolResult{Content: string(data)}, err
}
func (c *evidenceCatalog) tools() []agent.Tool {
	return []agent.Tool{evidenceTool{c, "list_evidence"}, evidenceTool{c, "read_evidence"}, evidenceTool{c, "check_claims"}}
}

type webEvidenceTool struct {
	tool    agent.Tool
	catalog *evidenceCatalog
}

func (t webEvidenceTool) Definition() agent.ToolDefinition { return t.tool.Definition() }
func (t webEvidenceTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	result, err := t.tool.Execute(ctx, raw)
	if err != nil {
		return result, err
	}
	var response struct {
		URL       string `json:"url"`
		Body      string `json:"body"`
		Status    int    `json:"status"`
		Truncated bool   `json:"truncated"`
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content), &envelope); err != nil {
		return agent.ToolResult{}, err
	}
	if err := json.Unmarshal(envelope.Result, &response); err != nil {
		return agent.ToolResult{}, err
	}
	if response.Status < 200 || response.Status >= 300 {
		return agent.ToolResult{}, fmt.Errorf("web evidence HTTP status %d", response.Status)
	}
	entry, err := t.catalog.retain(response.URL, []byte(response.Body), time.Now().UTC())
	if err != nil {
		return agent.ToolResult{}, err
	}
	data, err := json.Marshal(map[string]any{"evidence": entry, "text": response.Body, "truncated": response.Truncated})
	return agent.ToolResult{Content: string(data)}, err
}
func (c *evidenceCatalog) web(origins []string, options tools.HTTPFetchOptions) ([]agent.Tool, error) {
	if len(origins) == 0 {
		return nil, nil
	}
	allowed := map[string]bool{}
	for _, origin := range origins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return nil, errors.New("web selection must be an HTTPS origin")
		}
		allowed[parsed.Scheme+"://"+parsed.Host] = true
	}
	policy := tools.CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}, Approve: func(_ context.Context, argv []string) (tools.CommandDecision, error) {
		if len(argv) != 2 {
			return tools.CommandDeny, nil
		}
		parsed, err := url.Parse(argv[1])
		if err != nil {
			return tools.CommandDeny, err
		}
		if allowed[parsed.Scheme+"://"+parsed.Host] {
			return tools.CommandAllowOnce, nil
		}
		return tools.CommandDeny, errors.New("origin is not selected for this Work run")
	}}
	surface := tools.HTTPTools(policy, options)
	return []agent.Tool{webEvidenceTool{surface[0], c}}, nil
}
