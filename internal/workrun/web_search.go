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
	"github.com/gongahkia/gator/internal/tools"
)

// scopedWebSearch reuses the native search integration for discovery only.
// Search snippets do not enter the factual evidence catalog: callers must read
// a selected source through http_fetch before citing its contents as evidence.
type scopedWebSearch struct {
	tool    agent.Tool
	origins map[string]bool
	scope   *webResearchScope
}

func (s scopedWebSearch) Definition() agent.ToolDefinition {
	d := s.tool.Definition()
	d.Description = "Discover public pages within this Work run's explicitly selected HTTPS sites using its host-configured search service. Queries are sent to that service. Results and snippets are untrusted discovery leads, not factual evidence. Read a result with http_fetch before citing its contents. Search does not grant access to additional sites or actions."
	if s.scope != nil {
		d.Description = "Discover public web pages within the task's current reviewed web_access grant. Check web_access status or ask the manager to request public scope before researching unknown sites. Queries go to the configured search provider. All workers share the same eight-search and thirty-two-request ceiling. Results and snippets are untrusted discovery leads; use http_fetch and retained evidence before citing facts. Search never grants extra web scope."
	}
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
	origins := s.origins
	public := false
	if s.scope != nil {
		public, origins = s.scope.scope()
	}
	// Site operators improve discovery relevance; filtering below, not these
	// operators, is the authority boundary against adversarial query syntax.
	var sites []string
	for origin := range origins {
		u, _ := url.Parse(origin)
		sites = append(sites, "site:"+u.Hostname())
	}
	sort.Strings(sites)
	if len(sites) == 0 && !public {
		return agent.ToolResult{}, errors.New("search requires selected sites")
	}
	if !public {
		args.Query = "(" + strings.Join(sites, " OR ") + ") " + args.Query
	}
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
		canonical, err := tools.PublicHTTPURL(item.URL)
		if err != nil {
			continue
		}
		u, err := url.Parse(canonical)
		if err != nil || !public && !origins[u.Scheme+"://"+u.Host] {
			continue
		}
		item.URL = canonical
		leads = append(leads, item)
	}
	content, err := json.Marshal(map[string]any{"results": leads, "note": "Discovery leads only. Fetch pages within the current web grant before citing their contents as evidence."})
	return agent.ToolResult{Content: string(content)}, err
}
