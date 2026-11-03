// Package websearch resolves user-selected search without mixing credentials
// with display state, model configuration or project files.
package websearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/usage"
)

type Provider struct{ ID, Label, Environment, Documentation string }

func Providers() []Provider {
	return []Provider{
		{"tinyfish", "TinyFish", "TINYFISH_API_KEY", "https://docs.tinyfish.ai/search-api/reference"},
		{"exa", "Exa", "EXA_API_KEY", "https://exa.ai/docs/reference/search"},
		{"tavily", "Tavily", "TAVILY_API_KEY", "https://docs.tavily.com/documentation/api-reference/endpoint/search"},
		{"brave", "Brave", "BRAVE_SEARCH_API_KEY", "https://api-dashboard.search.brave.com/documentation"},
	}
}
func Find(id string) (Provider, bool) {
	for _, p := range Providers() {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}
func CredentialRef(provider string) string { return "web-search-" + provider }

type Resolved struct {
	Options  tools.HTTPFetchOptions `json:"-"`
	Provider string
	Source   string
	Ready    bool
	Issue    string
	key      string
}

// Fingerprint stays host-side: it binds an asynchronous access test to the
// exact effective credential without publishing a credential hash to the UI.
func (r Resolved) Fingerprint() string {
	d := sha256.Sum256([]byte(r.Provider + "\x00" + r.Source + "\x00" + r.key))
	return hex.EncodeToString(d[:])
}

func Environment(getenv func(string) string) tools.HTTPFetchOptions {
	return tools.HTTPFetchOptions{SearchProvider: strings.TrimSpace(getenv("GATOR_WEB_SEARCH_PROVIDER")), TinyfishSearchAPIKey: strings.TrimSpace(getenv("TINYFISH_API_KEY")), ExaSearchAPIKey: strings.TrimSpace(getenv("EXA_API_KEY")), TavilySearchAPIKey: strings.TrimSpace(getenv("TAVILY_API_KEY")), BraveSearchAPIKey: strings.TrimSpace(getenv("BRAVE_SEARCH_API_KEY"))}
}

func selectedOptions(provider, key string) tools.HTTPFetchOptions {
	o := tools.HTTPFetchOptions{SearchProvider: provider}
	switch provider {
	case "tinyfish":
		o.TinyfishSearchAPIKey = key
	case "exa":
		o.ExaSearchAPIKey = key
	case "tavily":
		o.TavilySearchAPIKey = key
	case "brave":
		o.BraveSearchAPIKey = key
	}
	return o
}

// Resolve honors an app choice before environment auto-selection. A saved key
// wins for its selected service; a missing key can use only that same service's
// environment credential. Errors never choose another service or data destination.
func Resolve(selection config.SearchSettings, credentials auth.Store, getenv func(string) string) Resolved {
	r := Resolved{}
	if selection.Validate() != nil {
		r.Issue = "Search settings need attention."
		return r
	}
	if selection.Provider == "off" {
		r.Options.SearchDisabled = true
		r.Source = "disabled"
		r.Issue = "Web search is turned off."
		return r
	}
	if selection.Provider == "" || selection.Provider == "environment" {
		r.Options = Environment(getenv)
		r.Source = "launch environment"
		provider, err := r.Options.SearchStatus()
		r.Provider = provider
		if err != nil {
			r.Issue = "Search environment is incomplete or ambiguous. Choose a service in Search settings."
			return r
		}
		if provider == "" {
			r.Issue = "No search service is configured."
			return r
		}
		p, _ := Find(provider)
		r.key = strings.TrimSpace(getenv(p.Environment))
		r.Ready = r.key != ""
		return r
	}
	r.Provider = selection.Provider
	p, ok := Find(r.Provider)
	if !ok {
		r.Issue = "Unsupported search service."
		return r
	}
	credential, found, err := credentials.Read(CredentialRef(r.Provider))
	if err != nil {
		r.Issue = "The saved search credential could not be read. Review Search settings."
		r.Options = selectedOptions(r.Provider, "")
		return r
	}
	if found {
		if !credential.IsAPIKey() {
			r.Issue = "The saved search credential is not an API key. Replace it in Search settings."
			r.Options = selectedOptions(r.Provider, "")
			return r
		}
		r.key = strings.TrimSpace(credential.Key)
		r.Source = "saved on this computer"
	} else {
		r.key = strings.TrimSpace(getenv(p.Environment))
		r.Source = "launch environment"
	}
	r.Options = selectedOptions(r.Provider, r.key)
	r.Ready = r.key != ""
	if !r.Ready {
		r.Issue = "Add an API key for " + p.Label + " in Search settings."
	}
	return r
}

// Meter applies the same cross-process pause/request/concurrency policy to
// search and search tests. No token estimate or provider price is invented.
func Meter(stateDir string) func(context.Context, string) (func(bool) error, error) {
	return func(ctx context.Context, provider string) (func(bool) error, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		store, err := usage.Open(stateDir)
		if err != nil {
			return nil, err
		}
		reservation, err := store.Reserve(time.Now(), "search", provider+"/search", 0)
		if err != nil {
			return nil, err
		}
		return func(failed bool) error { return reservation.Finish(0, true, failed, nil) }, nil
	}
}
