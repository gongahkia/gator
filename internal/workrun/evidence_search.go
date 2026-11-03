package workrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
)

type evidenceSearchTool struct{ catalog *evidenceCatalog }

func (t evidenceSearchTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "search_evidence", Description: "Find an exact, case-sensitive literal term in an already retained evidence ID, without new network access. Returns bounded UTF-8-safe raw-byte contexts and exact offsets from the fetched source. For large HTML this locates a term before read_evidence pagination; it does not render HTML. Returns first and last matches with an omitted count when necessary. Only retained bytes are searched, which may be a partial upstream page. A match establishes source integrity, not semantic support.", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["id","query"],"properties":{"id":{"type":"string","maxLength":256},"query":{"type":"string","minLength":1,"maxLength":512},"offset_bytes":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":8},"context_bytes":{"type":"integer","minimum":0,"maximum":1024}}}`)}
}
func (t evidenceSearchTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}
	if len(raw) > 8192 {
		return agent.ToolResult{}, errors.New("evidence search arguments exceed their limit")
	}
	var input struct {
		ID      string `json:"id"`
		Query   string `json:"query"`
		Offset  int    `json:"offset_bytes"`
		Limit   int    `json:"limit"`
		Context *int   `json:"context_bytes"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		return agent.ToolResult{}, err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return agent.ToolResult{}, errors.New("evidence search requires one argument object")
	}
	if input.Limit == 0 {
		input.Limit = 4
	}
	contextBytes := 400
	if input.Context != nil {
		contextBytes = *input.Context
	}
	if len(input.ID) > 256 || input.Query == "" || len(input.Query) > 512 || !utf8.ValidString(input.Query) || strings.ContainsRune(input.Query, 0) || input.Limit < 1 || input.Limit > 8 || contextBytes < 0 || contextBytes > 1024 {
		return agent.ToolResult{}, errors.New("invalid bounded evidence search")
	}
	entry, data, err := t.catalog.read(input.ID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if !utf8.Valid(data) || bytes.ContainsRune(data, 0) || input.Offset < 0 || input.Offset > len(data) || input.Offset < len(data) && !utf8.RuneStart(data[input.Offset]) {
		return agent.ToolResult{}, errors.New("evidence search requires text and a valid UTF-8 offset")
	}
	firstCount := (input.Limit + 1) / 2
	lastCount := input.Limit - firstCount
	first, last := []int{}, []int{}
	count := 0
	for offset := input.Offset; offset < len(data); {
		n := bytes.Index(data[offset:], []byte(input.Query))
		if n < 0 {
			break
		}
		position := offset + n
		if len(first) < firstCount {
			first = append(first, position)
		} else if lastCount > 0 {
			if len(last) < lastCount {
				last = append(last, position)
			} else {
				last[(count-firstCount)%lastCount] = position
			}
		}
		count++
		offset = position + len(input.Query)
	}
	sort.Ints(last)
	positions := append(first, last...)
	matches := []map[string]any{}
	for _, position := range positions {
		start, end := max(0, position-contextBytes), min(len(data), position+len(input.Query)+contextBytes)
		for start > 0 && !utf8.RuneStart(data[start]) {
			start--
		}
		for end < len(data) && !utf8.RuneStart(data[end]) {
			end++
		}
		matches = append(matches, map[string]any{"start_offset_bytes": start, "end_offset_bytes": end, "match_offset_bytes": position, "text": string(data[start:end])})
	}
	retained, err := t.catalog.retainEntry(entry, data)
	if err != nil {
		return agent.ToolResult{}, err
	}
	content, err := json.Marshal(map[string]any{"evidence": retained, "matches": matches, "total_matches": count, "matches_omitted": count - len(matches), "retained_bytes": len(data), "note": "Exact contexts from retained bytes only. HTML remains unrendered; the upstream page may have been truncated. Use offsets with read_evidence for adjacent text."})
	return agent.ToolResult{Content: string(content)}, err
}
