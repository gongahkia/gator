package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/usage"
)

func TestAppSelectionSeparatesKeysAndOverridesEnvironment(t *testing.T) {
	store, _ := auth.New(t.TempDir())
	if err := store.Put(CredentialRef("tinyfish"), auth.Credential{Type: "api_key", Key: "saved-tinyfish-private"}); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"GATOR_WEB_SEARCH_PROVIDER": "brave", "BRAVE_SEARCH_API_KEY": "environment-brave-private", "TINYFISH_API_KEY": "environment-tinyfish-private"}
	getenv := func(k string) string { return env[k] }
	selected := Resolve(config.SearchSettings{Provider: "tinyfish"}, store, getenv)
	if !selected.Ready || selected.Provider != "tinyfish" || selected.Options.TinyfishSearchAPIKey != "saved-tinyfish-private" || selected.Options.BraveSearchAPIKey != "" {
		t.Fatal("app selection leaked/fell back to another key")
	}
	encoded, _ := json.Marshal(selected)
	if strings.Contains(string(encoded), "private") {
		t.Fatal("display projection included a credential")
	}
	for _, p := range Providers() {
		r := Resolve(config.SearchSettings{Provider: p.ID}, store, getenv)
		if !r.Ready && p.ID != "exa" && p.ID != "tavily" {
			t.Fatal("matching selected key missing", p.ID)
		}
		if p.ID == "exa" || p.ID == "tavily" {
			if r.Ready || r.Options.BraveSearchAPIKey != "" {
				t.Fatal("missing key selected a different vendor", p.ID)
			}
		}
		if err := (config.SearchSettings{Provider: p.ID}).Validate(); err != nil {
			t.Fatal("configuration/provider inventory drift", err)
		}
	}
	legacy := Resolve(config.SearchSettings{}, store, getenv)
	if !legacy.Ready || legacy.Provider != "brave" || legacy.Source != "launch environment" {
		t.Fatal("existing environment behavior changed")
	}
	disabled := Resolve(config.SearchSettings{Provider: "off"}, store, getenv)
	if p, err := disabled.Options.SearchStatus(); p != "" || err != nil || disabled.Ready || !disabled.Options.SearchDisabled {
		t.Fatal("app disable resurrected an environment key", p, err)
	}
	if len(tools.HTTPTools(tools.CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}}, disabled.Options)) != 1 {
		t.Fatal("disabled search tool remained available")
	}
	delete(env, "GATOR_WEB_SEARCH_PROVIDER")
	if r := Resolve(config.SearchSettings{}, store, getenv); r.Ready || r.Issue == "" {
		t.Fatal("ambiguous environment silently selected a provider")
	}
}

type searchResolver struct{}

func (searchResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

type searchHTTP struct {
	calls int
	fail  bool
}

func (h *searchHTTP) Do(r *http.Request) (*http.Response, error) {
	h.calls++
	if h.fail {
		return nil, errors.New("private fixture transport failure")
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"results":[]}`))}, nil
}

func TestSearchUsesSharedPauseLimitsAndSettlesSuccessAndFailure(t *testing.T) {
	root := t.TempDir()
	ledger, _ := usage.Open(root)
	prior, _ := ledger.Policy()
	next := prior
	next.Paused = true
	if err := ledger.SaveReviewedPolicy(next, prior); err != nil {
		t.Fatal(err)
	}
	http := &searchHTTP{}
	options := tools.HTTPFetchOptions{SearchProvider: "tinyfish", TinyfishSearchAPIKey: "fixture-private", Client: http, Resolver: searchResolver{}, BeginSearch: Meter(root)}
	surface := tools.HTTPTools(tools.CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}, Approve: func(context.Context, []string) (tools.CommandDecision, error) { return tools.CommandAllowOnce, nil }}, options)
	query := json.RawMessage(`{"query":"public connectivity test"}`)
	if _, err := surface[1].Execute(context.Background(), query); !errors.Is(err, usage.ErrLimit) || http.calls != 0 {
		t.Fatal("paused search reached its provider", http.calls, err)
	}
	prior = next
	next.Paused = false
	next.DailyRequests = 2
	if err := ledger.SaveReviewedPolicy(next, prior); err != nil {
		t.Fatal(err)
	}
	if _, err := surface[1].Execute(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	http.fail = true
	if _, err := surface[1].Execute(context.Background(), query); err == nil || strings.Contains(err.Error(), "private fixture") {
		t.Fatal("transport failure not safely retained", err)
	}
	if _, err := surface[1].Execute(context.Background(), query); !errors.Is(err, usage.ErrLimit) || http.calls != 2 {
		t.Fatal("shared daily limit did not stop search", err)
	}
	view, err := ledger.Snapshot(time.Now())
	if err != nil || view.Requests != 2 || view.Categories["search"] != 2 || view.Failed != 1 || view.ActiveRequests != 0 || view.Tokens != 0 || view.KnownCostUSD != nil {
		t.Fatal("search accounting was inaccurate", view, err)
	}
}
