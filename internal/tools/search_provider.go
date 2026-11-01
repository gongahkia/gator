package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// SearchStatus reports host configuration without returning credential values.
func (o HTTPFetchOptions) SearchStatus() (string, error) {
	p, _, err := o.searchConfiguration()
	return p, err
}

func (o HTTPFetchOptions) searchConfiguration() (string, string, error) {
	keys := map[string]string{"brave": strings.TrimSpace(o.BraveSearchAPIKey), "tinyfish": strings.TrimSpace(o.TinyfishSearchAPIKey), "exa": strings.TrimSpace(o.ExaSearchAPIKey), "tavily": strings.TrimSpace(o.TavilySearchAPIKey)}
	p := strings.ToLower(strings.TrimSpace(o.SearchProvider))
	if p == "" {
		for name, key := range keys {
			if key != "" {
				if p != "" {
					return "", "", errors.New("multiple web search keys configured; select GATOR_WEB_SEARCH_PROVIDER")
				}
				p = name
			}
		}
		if p == "" {
			return "", "", nil
		}
	}
	key, ok := keys[p]
	if !ok {
		return "", "", errors.New("unsupported GATOR_WEB_SEARCH_PROVIDER; choose tinyfish, exa, tavily or brave")
	}
	if key == "" {
		return "", "", errors.New("selected web search provider has no configured key")
	}
	return p, key, nil
}

func (t WebSearch) searchRequest(ctx context.Context, query string, count int) (*http.Request, error) {
	p, key, err := t.Options.searchConfiguration()
	if err != nil {
		return nil, err
	}
	endpoint, method, header, auth := "", http.MethodGet, "", key
	var payload any
	switch p {
	case "brave":
		endpoint, header = braveWebSearchEndpoint, "X-Subscription-Token"
	case "tinyfish":
		endpoint, header = "https://api.search.tinyfish.ai", "X-API-Key"
	case "exa":
		endpoint, method, header = "https://api.exa.ai/search", http.MethodPost, "X-Api-Key"
		payload = map[string]any{"query": query, "type": "auto", "numResults": count}
	case "tavily":
		endpoint, method, header, auth = "https://api.tavily.com/search", http.MethodPost, "Authorization", "Bearer "+key
		payload = map[string]any{"query": query, "max_results": count, "search_depth": "basic", "auto_parameters": false, "include_answer": false, "include_raw_content": false, "include_images": false}
	default:
		return nil, errors.New("web search is not configured")
	}
	u, _, err := normalizeHTTPURL(endpoint)
	if err != nil {
		return nil, err
	}
	var body []byte
	if method == http.MethodGet {
		v := u.Query()
		if p == "brave" {
			v.Set("q", query)
			v.Set("count", strconv.Itoa(count))
		} else {
			v.Set("query", query)
		}
		u.RawQuery = v.Encode()
	} else {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set(header, auth)
	if method == http.MethodPost {
		r.Header.Set("Content-Type", "application/json")
	}
	return r, nil
}

func parseSearchResults(provider string, body []byte, count int) ([]webSearchResult, error) {
	type item struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
		Age         string `json:"age"`
		Snippet     string `json:"snippet"`
		Content     string `json:"content"`
		Published   string `json:"publishedDate"`
		Date        string `json:"date"`
	}
	var response struct {
		Web struct {
			Results []item `json:"results"`
		} `json:"web"`
		Results []item `json:"results"`
	}
	if json.Unmarshal(body, &response) != nil {
		return nil, errors.New("web search returned invalid JSON")
	}
	items := response.Results
	if provider == "brave" {
		items = response.Web.Results
	}
	// Empty/malformed objects must not masquerade as a successful quiet search.
	if items == nil {
		return nil, errors.New("web search response is missing results")
	}
	result := make([]webSearchResult, 0, min(count, len(items)))
	for _, i := range items {
		if strings.TrimSpace(i.URL) == "" {
			continue
		}
		desc, age := i.Description, i.Age
		switch provider {
		case "tinyfish":
			desc, age = i.Snippet, i.Date
		case "exa":
			age = i.Published
		case "tavily":
			desc = i.Content
		}
		result = append(result, webSearchResult{Title: boundedWebSearchText(i.Title), URL: boundedWebSearchText(i.URL), Description: boundedWebSearchText(desc), Age: boundedWebSearchText(age)})
		if len(result) == count {
			break
		}
	}
	return result, nil
}
