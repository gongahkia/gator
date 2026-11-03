package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/tools"
	sharedusage "github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/websearch"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workrun"
)

type searchControlHTTP struct {
	calls  int
	status int
	run    func(*nethttp.Request)
}

func (h *searchControlHTTP) Do(r *nethttp.Request) (*nethttp.Response, error) {
	h.calls++
	if h.run != nil {
		h.run(r)
	}
	return &nethttp.Response{StatusCode: h.status, Header: nethttp.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"results":[{"title":"Connectivity example","url":"https://example.org"}]}`))}, nil
}

type searchControlResolver struct{}

func (searchControlResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

func searchControlFixture(t *testing.T) (*searchControls, config.Store, auth.Store, *searchControlHTTP, map[string]string) {
	t.Helper()
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	store, _ := config.DefaultStore()
	credentials, _ := auth.New(root)
	env := map[string]string{}
	http := &searchControlHTTP{status: 200}
	c := &searchControls{stateDir: root, getenv: func(k string) string { return env[k] }, http: tools.HTTPFetchOptions{Client: http, Resolver: searchControlResolver{}}}
	return c, store, credentials, http, env
}
func searchAction(t *testing.T, s workapp.ControlScreen, operation string) workapp.ControlAction {
	t.Helper()
	for _, a := range s.Actions {
		if a.Action == operation {
			return a
		}
	}
	t.Fatal("missing Search action", operation)
	return workapp.ControlAction{}
}
func configureSearch(t *testing.T, c *searchControls, provider, key string) workapp.ControlScreen {
	t.Helper()
	s, err := c.call(context.Background(), "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	a := searchAction(t, s, "configure")
	s, err = c.call(context.Background(), a.Action, append(a.Args, provider, key))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNativeSearchSetupTestRestartAndRemovalKeepSecretsPrivate(t *testing.T) {
	c, store, credentials, http, env := searchControlFixture(t)
	const key = "native-search-private-fixture-key"
	if err := credentials.Put("openai", auth.Credential{Type: "api_key", Key: "model-key-unchanged"}); err != nil {
		t.Fatal(err)
	}
	screen := configureSearch(t, c, "tinyfish", key)
	settings, err := store.Load()
	if err != nil || settings.Search.Provider != "tinyfish" {
		t.Fatal("selection not saved", err)
	}
	data, err := os.ReadFile(store.Path())
	if err != nil || strings.Contains(string(data), key) {
		t.Fatal("search key entered non-secret configuration", err)
	}
	credential, found, err := credentials.Read(websearch.CredentialRef("tinyfish"))
	if err != nil || !found || credential.Key != key {
		t.Fatal("private key not retained", err)
	}
	options := configuredWebSearch(settings, credentials, c.stateDir)
	if p, err := options.SearchStatus(); p != "tinyfish" || err != nil || options.TinyfishSearchAPIKey != key {
		t.Fatal("native work did not load app-selected service", p, err)
	}
	http.run = func(r *nethttp.Request) {
		if r.URL.Query().Get("query") != searchTestQuery || r.Header.Get("X-API-Key") != key {
			t.Error("test did not use the fixed query and private service header")
		}
	}
	test := searchAction(t, screen, "test")
	screen, err = c.call(context.Background(), test.Action, test.Args)
	if err != nil {
		t.Fatal(err)
	}
	projected, _ := json.Marshal(screen)
	if strings.Contains(string(projected), key) || !strings.Contains(string(projected), "Last test succeeded") {
		t.Fatal("search test result inaccurate or leaked key")
	}
	if http.calls != 1 {
		t.Fatal("setup made an unsolicited network request", http.calls)
	}
	ledger, _ := sharedusage.Open(c.stateDir)
	view, _ := ledger.Snapshot(time.Now())
	if view.Categories["search"] != 1 || view.Tokens != 0 {
		t.Fatal("search test bypassed shared accounting", view)
	}
	// Recreate the control process, preserving config/credentials but no stale
	// live access check. The next native Work still resolves the saved key.
	restarted := &searchControls{stateDir: c.stateDir, getenv: c.getenv, http: c.http}
	screen, err = restarted.call(context.Background(), "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	projected, _ = json.Marshal(screen)
	if strings.Contains(string(projected), "Last test succeeded") {
		t.Fatal("restart invented a current-session check")
	}
	remove := searchAction(t, screen, "remove-key")
	env["TINYFISH_API_KEY"] = "environment-search-key"
	_, err = restarted.call(context.Background(), remove.Action, append(remove.Args, "tinyfish"))
	if err != nil {
		t.Fatal(err)
	}
	settings, _ = store.Load()
	if settings.Search.Provider != "off" {
		t.Fatal("removal silently fell back to an environment key")
	}
	if _, found, err := credentials.Read(websearch.CredentialRef("tinyfish")); err != nil || found {
		t.Fatal("saved key still present", err)
	}
	if credential, found, err := credentials.Read("openai"); err != nil || !found || credential.Key != "model-key-unchanged" {
		t.Fatal("search changes touched a model credential", err)
	}
}

func TestSearchSetupRejectsStaleKeyAndProviderViews(t *testing.T) {
	c, store, credentials, _, _ := searchControlFixture(t)
	configureSearch(t, c, "tinyfish", "initial-key")
	old, err := c.call(context.Background(), "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	oldAction := searchAction(t, old, "configure")
	configureSearch(t, c, "tinyfish", "newer-key")
	if _, err := c.call(context.Background(), "configure", append(oldAction.Args, "tinyfish", "stale-key")); err == nil {
		t.Fatal("stale form overwrote a newer key")
	}
	credential, _, _ := credentials.Read(websearch.CredentialRef("tinyfish"))
	if credential.Key != "newer-key" {
		t.Fatal("newer key was lost")
	}
	old, _ = c.call(context.Background(), "status", nil)
	oldAction = searchAction(t, old, "configure")
	configureSearch(t, c, "brave", "brave-key")
	if _, err := c.call(context.Background(), "configure", append(oldAction.Args, "exa", "unrequested-key")); err == nil {
		t.Fatal("stale provider form changed new selection")
	}
	settings, _ := store.Load()
	if settings.Search.Provider != "brave" {
		t.Fatal("stale selection overwrote current provider")
	}
	if _, found, _ := credentials.Read(websearch.CredentialRef("exa")); found {
		t.Fatal("rejected provider left a private key behind")
	}
}

func TestSearchAccessFailureDoesNotLeakKeyOrAffectReadinessOfAnotherSelection(t *testing.T) {
	c, _, _, http, _ := searchControlFixture(t)
	screen := configureSearch(t, c, "tinyfish", "secret-do-not-display")
	http.status = 401
	a := searchAction(t, screen, "test")
	screen, err := c.call(context.Background(), a.Action, a.Args)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(screen)
	if !strings.Contains(string(encoded), "HTTP 401") || strings.Contains(string(encoded), "secret-do-not-display") {
		t.Fatal("unsafe or inaccurate authentication failure")
	}
	for _, bad := range []string{"key\nAuthorization: private", "Bearer wrong-key", strings.Repeat("k", 4097)} {
		screen, err = c.call(context.Background(), "status", nil)
		if err != nil {
			t.Fatal(err)
		}
		a = searchAction(t, screen, "configure")
		if _, err := c.call(context.Background(), a.Action, append(a.Args, "tinyfish", bad)); err == nil || strings.Contains(err.Error(), bad) {
			t.Fatal("invalid API key accepted or reflected")
		}
	}
}

func TestSearchTestChangedWhileNetworkIsRunningDoesNotPublishStaleHealth(t *testing.T) {
	c, _, _, http, _ := searchControlFixture(t)
	screen := configureSearch(t, c, "tinyfish", "first-key")
	started, release := make(chan struct{}), make(chan struct{})
	http.run = func(*nethttp.Request) { close(started); <-release }
	a := searchAction(t, screen, "test")
	done := make(chan error, 1)
	go func() { _, err := c.call(context.Background(), a.Action, a.Args); done <- err }()
	<-started
	configureSearch(t, c, "brave", "second-key")
	close(release)
	if err := <-done; err == nil || !strings.Contains(err.Error(), "changed during the test") {
		t.Fatal("stale check reported current health", err)
	}
}

func TestSearchKeyIsRestoredWhenSettingsPublicationFails(t *testing.T) {
	c, store, credentials, _, _ := searchControlFixture(t)
	screen := configureSearch(t, c, "tinyfish", "original-search-key")
	a := searchAction(t, screen, "configure")
	// Force a real publication-lock failure after credential CAS, without
	// relying on platform-specific permission behavior or mocking persistence.
	lock := filepath.Join(filepath.Dir(store.Path()), ".settings.lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := c.call(context.Background(), a.Action, append(a.Args, "tinyfish", "replacement-search-key"))
	if err == nil {
		t.Fatal("settings publication unexpectedly succeeded")
	}
	credential, found, readErr := credentials.Read(websearch.CredentialRef("tinyfish"))
	if readErr != nil || !found || credential.Key != "original-search-key" {
		t.Fatal("failed settings publication left replacement credentials", readErr)
	}
	settings, readErr := store.Load()
	if readErr != nil || settings.Search.Provider != "tinyfish" {
		t.Fatal("failed publication changed the saved selection", readErr)
	}
}

func TestDesktopWorkAssemblyCapturesNativeSearchSelectionPerTask(t *testing.T) {
	c, store, _, _, _ := searchControlFixture(t)
	configureSearch(t, c, "tinyfish", "native-assembly-search-key")
	if err := store.Update(func(s *config.Settings) error {
		s.Defaults = config.Defaults{Provider: "fixture", Model: "fixture"}
		s.CustomProviders = []config.CustomProvider{{ID: "fixture", BaseURL: "http://127.0.0.1:1/chat", Models: []string{"fixture"}, DefaultModel: "fixture"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	request := workrun.Request{SourcePath: t.TempDir(), Objective: "Inspect task search readiness", Mode: action.Inspect, Contract: artifact.InspectionContract(), ForegroundApp: true}
	first, err := configuredWorkService("fixture", "fixture", c.stateDir, &request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Executor.HTTP.TinyfishSearchAPIKey != "native-assembly-search-key" || first.Executor.HTTP.BeginSearch == nil {
		t.Fatal("desktop Work omitted search configuration or shared accounting")
	}
	if err := store.Update(func(s *config.Settings) error { s.Search.Provider = "off"; return nil }); err != nil {
		t.Fatal(err)
	}
	second, err := configuredWorkService("fixture", "fixture", c.stateDir, &request)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Executor.HTTP.SearchDisabled || first.Executor.HTTP.TinyfishSearchAPIKey != "native-assembly-search-key" {
		t.Fatal("settings change failed to apply to new tasks or changed an admitted task")
	}
}
