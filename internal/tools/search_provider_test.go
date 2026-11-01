package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/sandbox"
)

type publicResolver struct{}

func (publicResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

type providerSearchHTTP struct {
	t        *testing.T
	provider string
	calls    int
	status   int
	body     string
}

func (f *providerSearchHTTP) Do(r *http.Request) (*http.Response, error) {
	f.calls++
	if strings.Contains(r.URL.String(), "fixture-secret") {
		f.t.Fatal("credential in URL")
	}
	var payload map[string]any
	if r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		if strings.Contains(string(data), "fixture-secret") {
			f.t.Fatal("credential in body")
		}
		if r.Method == http.MethodPost && json.Unmarshal(data, &payload) != nil {
			f.t.Fatal("invalid request payload")
		}
	}
	var wantHost, header string
	switch f.provider {
	case "brave":
		wantHost, header = "api.search.brave.com", "X-Subscription-Token"
		if r.Method != "GET" || r.URL.Query().Get("q") != "project deadline" {
			f.t.Fatal("Brave request shape")
		}
	case "tinyfish":
		wantHost, header = "api.search.tinyfish.ai", "X-API-Key"
		if r.Method != "GET" || r.URL.Query().Get("query") != "project deadline" {
			f.t.Fatal("TinyFish request shape")
		}
	case "exa":
		wantHost, header = "api.exa.ai", "X-Api-Key"
		if r.Method != "POST" || payload["query"] != "project deadline" || payload["numResults"] != float64(2) || payload["type"] != "auto" || payload["contents"] != nil {
			f.t.Fatal("Exa search unexpectedly requested extra content")
		}
	case "tavily":
		wantHost, header = "api.tavily.com", "Authorization"
		if r.Method != "POST" || payload["query"] != "project deadline" || payload["max_results"] != float64(2) || payload["search_depth"] != "basic" || payload["auto_parameters"] != false || payload["include_answer"] != false || payload["include_raw_content"] != false {
			f.t.Fatal("Tavily enabled unexpected paid/answer mode")
		}
	}
	wantKey := "fixture-secret"
	if header == "Authorization" {
		wantKey = "Bearer " + wantKey
	}
	if r.URL.Host != wantHost || r.Header.Get(header) != wantKey {
		f.t.Fatal("provider host/auth mismatch")
	}
	for _, h := range []string{"X-Subscription-Token", "X-API-Key", "Authorization"} {
		if h != header && !strings.EqualFold(h, header) && r.Header.Get(h) != "" {
			f.t.Fatal("cross-provider credential leakage")
		}
	}
	status := f.status
	if status == 0 {
		status = 200
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(f.body)), Request: r}, nil
}

func TestSearchProvidersUseFixedContractsAndNormalizeResults(t *testing.T) {
	for _, provider := range []string{"brave", "tinyfish", "exa", "tavily"} {
		t.Run(provider, func(t *testing.T) {
			body := `{"results":[{"title":"one","url":"https://public.test/one","snippet":"Snippet","content":"Content","publishedDate":"2026-11-01","date":"2026-11-01"},{"title":"two","url":"https://public.test/two"},{"title":"three","url":"https://public.test/three"}],"token":"must-not-forward"}`
			if provider == "brave" {
				body = `{"web":{"results":[{"title":"one","url":"https://public.test/one","description":"Snippet"},{"title":"two","url":"https://public.test/two"},{"title":"three","url":"https://public.test/three"}]}}`
			}
			client := &providerSearchHTTP{t: t, provider: provider, body: body}
			o := HTTPFetchOptions{SearchProvider: provider, BraveSearchAPIKey: "unused-brave", TinyfishSearchAPIKey: "unused-tinyfish", ExaSearchAPIKey: "unused-exa", TavilySearchAPIKey: "unused-tavily", Resolver: publicResolver{}, Client: client}
			switch provider {
			case "brave":
				o.BraveSearchAPIKey = "fixture-secret"
			case "tinyfish":
				o.TinyfishSearchAPIKey = "fixture-secret"
			case "exa":
				o.ExaSearchAPIKey = "fixture-secret"
			case "tavily":
				o.TavilySearchAPIKey = "fixture-secret"
			}
			allowed := 0
			surface := HTTPTools(CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}, Approve: func(context.Context, []string) (CommandDecision, error) { allowed++; return CommandAllowOnce, nil }}, o)
			result, err := surface[1].Execute(context.Background(), []byte(`{"query":"project deadline","count":2}`))
			if err != nil {
				t.Fatal(err)
			}
			if client.calls != 1 || allowed != 1 || strings.Contains(result.Content, "fixture-secret") || strings.Contains(result.Content, "must-not-forward") {
				t.Fatal("unexpected dispatch or disclosure")
			}
			var envelope struct {
				Result webSearchToolResult `json:"result"`
			}
			if json.Unmarshal([]byte(result.Content), &envelope) != nil || len(envelope.Result.Results) != 2 {
				t.Fatal("bounded normalized results missing")
			}
			if provider == "tinyfish" && envelope.Result.Results[0].Description != "Snippet" {
				t.Fatal("TinyFish snippet lost")
			}
			if provider == "tavily" && envelope.Result.Results[0].Description != "Content" {
				t.Fatal("Tavily snippet lost")
			}
			for _, status := range []int{401, 402, 429, 503} {
				client.status = status
				client.body = `{"error":"fixture-secret private backend content"}`
				_, err := surface[1].Execute(context.Background(), []byte(`{"query":"project deadline","count":2}`))
				if err == nil || strings.Contains(err.Error(), "fixture-secret") {
					t.Fatal("provider failure leaked or passed")
				}
			}
		})
	}
}

func TestSearchSelectionFailsClosedWithoutFallback(t *testing.T) {
	for _, o := range []HTTPFetchOptions{{SearchProvider: "unknown", BraveSearchAPIKey: "secret"}, {SearchProvider: "exa", BraveSearchAPIKey: "secret"}, {BraveSearchAPIKey: "secret", TinyfishSearchAPIKey: "secret"}} {
		if _, err := o.SearchStatus(); err == nil {
			t.Fatal("invalid/ambiguous provider configuration accepted")
		}
		surface := HTTPTools(CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}, Approve: func(context.Context, []string) (CommandDecision, error) {
			t.Fatal("bad configuration reached approval")
			return CommandDeny, nil
		}}, o)
		if _, err := surface[1].Execute(context.Background(), []byte(`{"query":"project deadline"}`)); err == nil {
			t.Fatal("bad configuration dispatched")
		}
	}
	if p, e := (HTTPFetchOptions{TinyfishSearchAPIKey: "only"}).SearchStatus(); e != nil || p != "tinyfish" {
		t.Fatal("single provider auto-selection failed")
	}
	for _, provider := range []string{"brave", "tinyfish", "exa", "tavily"} {
		if _, err := parseSearchResults(provider, []byte(`{"error":"private"}`), 2); err == nil {
			t.Fatal("missing results masked failure")
		}
	}
}

type secretSearchFailure struct{}

func (secretSearchFailure) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("private fixture-secret")
}
func TestSearchTransportErrorsDoNotExposeProviderSecrets(t *testing.T) {
	surface := HTTPTools(CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}, Approve: func(context.Context, []string) (CommandDecision, error) { return CommandAllowOnce, nil }}, HTTPFetchOptions{TinyfishSearchAPIKey: "fixture-secret", Resolver: publicResolver{}, Client: secretSearchFailure{}})
	if _, err := surface[1].Execute(context.Background(), []byte(`{"query":"project deadline"}`)); err == nil || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatal("sensitive transport error")
	}
}
