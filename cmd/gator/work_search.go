package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/websearch"
	"github.com/gongahkia/gator/internal/workapp"
)

const searchTestQuery = "Gator web search connectivity check"

type searchReview struct {
	selection   config.SearchSettings
	credentials map[string]*auth.Credential
	environment map[string]string
	fingerprint string
	at          time.Time
}
type searchCheck struct {
	fingerprint string
	at          time.Time
	results     int
	issue       string
}
type searchControls struct {
	stateDir string
	getenv   func(string) string
	http     tools.HTTPFetchOptions // Host test transport only; never renderer input.
	mu       sync.Mutex
	reviews  map[string]searchReview
	check    *searchCheck
}

func (c *searchControls) environment(key string) string {
	if c.getenv != nil {
		return c.getenv(key)
	}
	return os.Getenv(key)
}
func searchCredential(store auth.Store, provider string) (*auth.Credential, error) {
	v, found, err := store.Read(websearch.CredentialRef(provider))
	if err != nil || !found {
		return nil, err
	}
	return &v, nil
}

func (c *searchControls) call(ctx context.Context, operation string, args []string) (workapp.ControlScreen, error) {
	if err := ctx.Err(); err != nil {
		return workapp.ControlScreen{}, err
	}
	release, err := state.Guard(c.stateDir, false)
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	defer release()
	settingsStore, err := config.DefaultStore()
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	credentials, err := auth.New(c.stateDir)
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	if operation == "status" {
		if len(args) != 0 {
			return workapp.ControlScreen{}, errors.New("refresh Search settings")
		}
		return c.screen(settingsStore, credentials, "")
	}
	if len(args) < 1 {
		return workapp.ControlScreen{}, errors.New("refresh Search settings before changing them")
	}
	c.mu.Lock()
	prior, found := c.reviews[args[0]]
	c.mu.Unlock()
	if !found || time.Since(prior.at) > 10*time.Minute {
		return workapp.ControlScreen{}, errors.New("Search settings changed; refresh and try again")
	}
	selection := prior.selection
	provider := ""
	var replacement *auth.Credential
	changeCredential := false
	switch operation {
	case "configure":
		if len(args) != 3 {
			return workapp.ControlScreen{}, errors.New("choose a search service and provide its key")
		}
		provider = strings.TrimSpace(args[1])
		p, ok := websearch.Find(provider)
		if !ok {
			return workapp.ControlScreen{}, errors.New("choose a supported search service")
		}
		key := strings.TrimSpace(args[2])
		if key != "" {
			if err := validSearchKey(key); err != nil {
				return workapp.ControlScreen{}, err
			}
			replacement = &auth.Credential{Type: "api_key", Key: key}
			changeCredential = true
		} else if prior.credentials[provider] == nil && strings.TrimSpace(prior.environment[provider]) == "" {
			return workapp.ControlScreen{}, fmt.Errorf("enter an API key for %s", p.Label)
		}
		selection.Provider = provider
	case "off":
		if len(args) != 1 {
			return workapp.ControlScreen{}, errors.New("refresh Search settings")
		}
		selection.Provider = "off"
	case "environment":
		if len(args) != 1 {
			return workapp.ControlScreen{}, errors.New("refresh Search settings")
		}
		selection.Provider = "environment"
	case "remove-key":
		if len(args) != 2 {
			return workapp.ControlScreen{}, errors.New("choose a saved search key to remove")
		}
		provider = args[1]
		if _, ok := websearch.Find(provider); !ok || prior.credentials[provider] == nil {
			return workapp.ControlScreen{}, errors.New("that saved search key is no longer available")
		}
		changeCredential = true
		if selection.Provider == provider {
			selection.Provider = "off"
		}
	case "test":
		if len(args) != 1 {
			return workapp.ControlScreen{}, errors.New("refresh Search settings")
		}
		return c.test(ctx, settingsStore, credentials, prior, args[0])
	default:
		return workapp.ControlScreen{}, errors.New("unsupported Search settings action")
	}
	current, err := settingsStore.Load()
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	if current.Search != prior.selection {
		return workapp.ControlScreen{}, errors.New("Search settings changed; refresh and try again")
	}
	if provider != "" {
		now, err := searchCredential(credentials, provider)
		if err != nil {
			return workapp.ControlScreen{}, errors.New("saved search credentials are unavailable")
		}
		if !reflect.DeepEqual(now, prior.credentials[provider]) {
			return workapp.ControlScreen{}, errors.New("the search key changed; refresh and try again")
		}
		if operation == "configure" && !changeCredential && now == nil && c.environment(environmentForSearch(provider)) != prior.environment[provider] {
			return workapp.ControlScreen{}, errors.New("the search environment changed; refresh and try again")
		}
	}
	c.mu.Lock()
	delete(c.reviews, args[0])
	c.mu.Unlock()
	if changeCredential {
		if err := credentials.ReplaceReviewed(websearch.CredentialRef(provider), prior.credentials[provider], replacement); err != nil {
			return workapp.ControlScreen{}, errors.New("the search credential could not be changed; refresh and try again")
		}
	}
	err = settingsStore.Update(func(settings *config.Settings) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if settings.Search != prior.selection {
			return errors.New("Search settings changed; refresh and try again")
		}
		settings.Search = selection
		return nil
	})
	if err != nil {
		if changeCredential {
			if rollback := credentials.ReplaceReviewed(websearch.CredentialRef(provider), replacement, prior.credentials[provider]); rollback != nil {
				return workapp.ControlScreen{}, errors.New("the credential changed but Search settings could not be saved; refresh before continuing")
			}
		}
		return workapp.ControlScreen{}, err
	}
	notice := "Search settings saved for newly started tasks. Active tasks keep their captured service."
	if operation == "remove-key" {
		notice = "Saved key removed from this computer. This does not revoke it at the service or remove an environment key."
	}
	return c.screen(settingsStore, credentials, notice)
}

func validSearchKey(key string) error {
	if key == "" || len(key) > 4096 {
		return errors.New("enter an API key up to 4096 characters")
	}
	for _, r := range key {
		if r < 33 || r > 126 {
			return errors.New("paste the API key only, without spaces, control characters or a Bearer prefix")
		}
	}
	return nil
}
func environmentForSearch(provider string) string {
	p, _ := websearch.Find(provider)
	return p.Environment
}

func (c *searchControls) test(ctx context.Context, settings config.Store, credentials auth.Store, prior searchReview, token string) (workapp.ControlScreen, error) {
	current, err := settings.Load()
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	resolved := websearch.Resolve(current.Search, credentials, c.environment)
	if current.Search != prior.selection || resolved.Fingerprint() != prior.fingerprint {
		return workapp.ControlScreen{}, errors.New("Search settings changed; refresh before testing")
	}
	if !resolved.Ready {
		return workapp.ControlScreen{}, errors.New("configure a search service and key before testing")
	}
	c.mu.Lock()
	delete(c.reviews, token)
	c.mu.Unlock()
	options := resolved.Options
	options.Client = c.http.Client
	options.Resolver = c.http.Resolver
	options.MaxRequests = 1
	options.BeginSearch = websearch.Meter(c.stateDir)
	surface := tools.HTTPTools(tools.CommandPolicy{Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}, Approve: func(context.Context, []string) (tools.CommandDecision, error) { return tools.CommandAllowOnce, nil }}, options)
	if len(surface) != 2 {
		return workapp.ControlScreen{}, errors.New("search service is unavailable")
	}
	testContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args, _ := json.Marshal(map[string]any{"query": searchTestQuery, "count": 1})
	result, searchErr := surface[1].Execute(testContext, args)
	check := searchCheck{fingerprint: resolved.Fingerprint(), at: time.Now()}
	if searchErr != nil {
		check.issue = safeSearchFailure(searchErr)
	} else {
		var response struct {
			Result struct {
				Results []json.RawMessage `json:"results"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
			check.issue = "The service response could not be read."
		} else {
			check.results = len(response.Result.Results)
		}
	}
	latest, err := settings.Load()
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	if latest.Search != prior.selection || websearch.Resolve(latest.Search, credentials, c.environment).Fingerprint() != prior.fingerprint {
		return workapp.ControlScreen{}, errors.New("Search settings changed during the test; its result was not applied")
	}
	if err := ctx.Err(); err != nil {
		return workapp.ControlScreen{}, err
	}
	c.mu.Lock()
	c.check = &check
	c.mu.Unlock()
	return c.screen(settings, credentials, "")
}

func safeSearchFailure(err error) string {
	text := err.Error()
	for _, status := range []string{"401", "403", "402", "429"} {
		if strings.Contains(text, "HTTP "+status) {
			return "Search test failed (HTTP " + status + "). Check the key, service access and account limits."
		}
	}
	if strings.Contains(text, "shared usage limit") {
		return "Search was not sent because shared requests are paused or limited. Open Usage & limits."
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "Search test was stopped or timed out."
	}
	return "Search test failed. Check your connection and service configuration, then try again."
}

func (c *searchControls) screen(settings config.Store, credentials auth.Store, notice string) (workapp.ControlScreen, error) {
	current, err := settings.Load()
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	resolved := websearch.Resolve(current.Search, credentials, c.environment)
	prior := searchReview{selection: current.Search, credentials: map[string]*auth.Credential{}, environment: map[string]string{}, fingerprint: resolved.Fingerprint(), at: time.Now()}
	providers, saved := []uisnapshot.Option{}, []uisnapshot.Option{}
	links := []uisnapshot.Link{}
	for _, p := range websearch.Providers() {
		credential, err := searchCredential(credentials, p.ID)
		if err != nil {
			return workapp.ControlScreen{}, errors.New("saved search credentials could not be read; open Recovery")
		}
		prior.credentials[p.ID] = credential
		prior.environment[p.ID] = c.environment(p.Environment)
		providers = append(providers, uisnapshot.Option{Value: p.ID, Label: p.Label})
		links = append(links, uisnapshot.Link{Label: p.Label + " API documentation", URL: p.Documentation})
		if credential != nil {
			saved = append(saved, uisnapshot.Option{Value: p.ID, Label: p.Label})
		}
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return workapp.ControlScreen{}, err
	}
	token := hex.EncodeToString(random[:])
	c.mu.Lock()
	if c.reviews == nil {
		c.reviews = map[string]searchReview{}
	}
	if len(c.reviews) >= 32 {
		c.reviews = map[string]searchReview{}
	}
	c.reviews[token] = prior
	check := c.check
	c.mu.Unlock()
	service := "No service selected"
	if p, ok := websearch.Find(resolved.Provider); ok {
		service = p.Label
	}
	if current.Search.Provider == "off" {
		service = "Web search off"
	}
	status := resolved.Issue
	if resolved.Ready {
		status = "Key available · service access has not been tested in this app session"
	}
	if check != nil && check.fingerprint == resolved.Fingerprint() {
		if check.issue != "" {
			status = check.issue
		} else {
			status = fmt.Sprintf("Last test succeeded · %d result(s) · %s", check.results, check.at.Local().Format("15:04:05"))
		}
	}
	mode := "App selection"
	if current.Search.Provider == "" || current.Search.Provider == "environment" {
		mode = "Launch environment"
	}
	if current.Search.Provider == "off" {
		mode = "Disabled in Gator"
	}
	screen := workapp.ControlScreen{Panel: uisnapshot.Panel{ActionIdentity: token, Kind: "search", Title: "Web search", Subtitle: "Discover new sources from your conversation.", Notice: notice, Summary: []uisnapshot.Field{{Label: "Service", Value: service}, {Label: "Status", Value: status}, {Label: "Configuration", Value: mode}, {Label: "Credential source", Value: resolved.Source}}, Links: links, Blocks: []uisnapshot.Block{{Title: "Scope and charges", Lines: []string{"New tasks use this service; active work keeps its captured settings. Reading a public page does not require a search key.", "Web access is reviewed in chat. Searches share the task's eight-search limit and the shared provider request limits.", "The test sends only the public query “" + searchTestQuery + "” to the selected service. It may incur a search charge; no project or conversation text is sent."}}}}}
	selected := resolved.Provider
	if _, ok := websearch.Find(selected); !ok {
		selected = "tinyfish"
	}
	screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "C", Label: "Choose search service", Action: "configure", Args: []string{token}, Description: "Choose the service that receives search queries. Enter its API key privately; leave the key empty to reuse that service's existing saved or environment key.", Fields: []workapp.ControlField{{Label: "Search service", Kind: "select", Value: selected, Options: providers}, {Label: "API key", Secret: true, Help: "API key only. Saved privately on this computer; never included in chat or settings previews."}}})
	if resolved.Ready {
		screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "T", Label: "Run test search", Action: "test", Args: []string{token}, Detail: "One fixed public query; may incur a service charge."})
	}
	if current.Search.Provider != "off" {
		screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "D", Label: "Turn off web search", Action: "off", Args: []string{token}, Detail: "Stops search in newly started tasks, including when environment keys exist. Page reading remains available."})
	}
	if current.Search.Provider != "" && current.Search.Provider != "environment" {
		screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "E", Label: "Use environment settings", Action: "environment", Args: []string{token}, Detail: "Use the search service/key configured by your launcher. Saved keys remain private and unused in this mode."})
	}
	if len(saved) > 0 {
		screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "K", Label: "Remove a saved search key", Action: "remove-key", Args: []string{token}, Description: "Remove the selected saved key from Gator. Removing the currently selected app service also turns off search for new tasks. This does not revoke the key at its service or remove an environment key.", Fields: []workapp.ControlField{{Label: "Saved key", Kind: "select", Value: saved[0].Value, Options: saved}}})
	}
	screen.Actions = append(screen.Actions, workapp.ControlAction{Key: "R", Label: "Refresh", Action: "status"})
	return screen, nil
}
