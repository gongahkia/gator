package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
)

// scopedWebSearch reuses the native search integration for discovery only.
// Search snippets do not enter the factual evidence catalog: callers must read
// a selected source through http_fetch before citing its contents as evidence.
type scopedWebSearch struct {
	tool    agent.Tool
	origins map[string]bool
}

func (s scopedWebSearch) Definition() agent.ToolDefinition {
	d := s.tool.Definition()
	d.Description = "Discover public pages within this Work run's explicitly selected HTTPS sites using the configured Brave Search service. Queries are sent to that service. Results and snippets are untrusted discovery leads, not factual evidence. Read a result with http_fetch before citing its contents. Search does not grant access to additional sites or actions."
	return d
}

func (s scopedWebSearch) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var args struct {
		Query string `json:"query"`
		Count int    `json:"count,omitempty"`
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(&args); err != nil {
		return agent.ToolResult{}, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return agent.ToolResult{}, errors.New("search requires one JSON argument object")
	}
	if strings.TrimSpace(args.Query) == "" {
		return agent.ToolResult{}, errors.New("search query is required")
	}
	// Site operators improve discovery relevance; filtering below, not these
	// operators, is the authority boundary against adversarial query syntax.
	var sites []string
	for origin := range s.origins {
		u, _ := url.Parse(origin)
		sites = append(sites, "site:"+u.Hostname())
	}
	sort.Strings(sites)
	if len(sites) == 0 {
		return agent.ToolResult{}, errors.New("search requires selected sites")
	}
	args.Query = "(" + strings.Join(sites, " OR ") + ") " + args.Query
	payload, err := json.Marshal(args)
	if err != nil {
		return agent.ToolResult{}, err
	}
	result, err := s.tool.Execute(ctx, payload)
	if err != nil {
		return result, err
	}
	type lead struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
		Age         string `json:"age,omitempty"`
	}
	var response struct {
		Result struct {
			Results []lead `json:"results"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
		return agent.ToolResult{}, err
	}
	leads := []lead{}
	for _, item := range response.Result.Results {
		u, err := url.Parse(item.URL)
		if err != nil || u.User != nil || !s.origins[u.Scheme+"://"+u.Host] {
			continue
		}
		leads = append(leads, item)
	}
	content, err := json.Marshal(map[string]any{"results": leads, "note": "Discovery leads only. Fetch selected pages before citing their contents as evidence."})
	return agent.ToolResult{Content: string(content)}, err
}
