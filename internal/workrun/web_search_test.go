package workrun

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/tools"
)

type searchFixture struct {
	calls int
	query string
}

func (f *searchFixture) Do(r *http.Request) (*http.Response, error) {
	f.calls++
	if r.URL.Host == "api.search.brave.com" {
		f.query = r.URL.Query().Get("q")
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"web":{"results":[{"title":"Primary","url":"https://research.invalid/venue","description":"unverified snippet"},{"title":"Other","url":"https://other.invalid/secret","description":"hidden"},{"title":"Credentials","url":"https://user:password@research.invalid/no"},{"title":"Insecure","url":"http://research.invalid/no"},{"title":"Wrong port","url":"https://research.invalid:444/no"}]}}`)), Request: r}, nil
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: io.NopCloser(strings.NewReader("Verified source content.")), Request: r}, nil
}

func TestWorkSearchScopesDiscoveryAndRequiresPageEvidence(t *testing.T) {
	f := &searchFixture{}
	step := 0
	m := researchModel(func(r agent.TurnRequest) (agent.Turn, error) {
		step++
		switch step {
		case 1:
			return researchCall("web_search", map[string]any{"query": "deadline OR site:other.invalid", "count": 5}), nil
		case 2:
			result := r.Messages[len(r.Messages)-1].Content
			if strings.Contains(result, "hidden") || strings.Contains(result, "password") || strings.Contains(result, "other.invalid") || strings.Contains(result, ":444") {
				t.Fatal("unselected result disclosed", result)
			}
			var response struct {
				Results []json.RawMessage `json:"results"`
			}
			if json.Unmarshal([]byte(result), &response) != nil || len(response.Results) != 1 {
				t.Fatal("selected discovery missing", result)
			}
			return researchCall("http_fetch", map[string]string{"url": "https://research.invalid/venue"}), nil
		default:
			return agent.Turn{Text: "Review source before action."}, nil
		}
	})
	s := Service{Executor: Executor{Model: m, StateDir: t.TempDir(), HTTP: tools.HTTPFetchOptions{Client: f, Resolver: researchResolver{}, BraveSearchAPIKey: "fixture-secret"}}}
	o, err := s.Execute(context.Background(), Request{SourcePath: t.TempDir(), Objective: "Research venue deadline", Mode: action.Inspect, Contract: artifact.InspectionContract(), WebOrigins: []string{"https://research.invalid"}})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != 2 || !strings.HasPrefix(f.query, "(site:research.invalid)") {
		t.Fatal("native integration not scoped", f.calls, f.query)
	}
	if len(o.Manifest.Evidence) != 1 || o.Manifest.Evidence[0].Locator != "https://research.invalid/venue" {
		t.Fatal("snippets treated as factual evidence", o.Manifest.Evidence)
	}
}

func TestWorkSearchRequiresSitesAndHostCredential(t *testing.T) {
	for _, options := range []tools.HTTPFetchOptions{{}, {BraveSearchAPIKey: "fixture-secret"}} {
		c := &evidenceCatalog{}
		if surface, err := c.web(nil, options); err != nil || len(surface) != 0 {
			t.Fatal("search without selected sites", surface, err)
		}
		surface, err := c.web([]string{"https://research.invalid"}, options)
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		if options.BraveSearchAPIKey != "" {
			want = 2
		}
		if len(surface) != want {
			t.Fatal("search credential boundary", len(surface), want)
		}
	}
	f := &searchFixture{}
	c := &evidenceCatalog{}
	surface, _ := c.web([]string{"https://research.invalid"}, tools.HTTPFetchOptions{Client: f, Resolver: researchResolver{}, BraveSearchAPIKey: "fixture-secret"})
	for _, raw := range []string{`{"query":""}`, `{"query":"x","actor":"trusted"}`, `{"query":"x"} {}`, `{"query":"x","count":11}`} {
		if _, err := surface[1].Execute(context.Background(), []byte(raw)); err == nil {
			t.Fatal("invalid search accepted", raw)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid request reached network")
	}
}
