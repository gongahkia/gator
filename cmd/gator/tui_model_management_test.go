package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/modelcatalog"
)

func TestTUIModelManagementRemovesClaudeCredentialFromAnthropicStore(t *testing.T) {
	root := t.TempDir()
	settings, err := config.New(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(root, "state")
	credentials, err := auth.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put("anthropic", auth.Credential{Type: "api_key", Key: "secret-anthropic"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	backend := newTUIModelManagementBackend(settings, stateDir)
	result, err := backend.RemoveCredential("claude")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Removed || result.StoreKey != "anthropic" || result.Kind != "API key" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.RemainingSources) != 1 || result.RemainingSources[0] != "ANTHROPIC_API_KEY" {
		t.Fatalf("remaining = %#v", result.RemainingSources)
	}
	if _, found, err := credentials.Read("anthropic"); err != nil || found {
		t.Fatalf("anthropic credential remains found=%v err=%v", found, err)
	}
	notice := describeCredentialRemoval(result)
	if strings.Contains(notice, "env-key") || strings.Contains(notice, "secret-anthropic") {
		t.Fatalf("removal notice leaked a secret: %q", notice)
	}
	if !strings.Contains(notice, "ANTHROPIC_API_KEY") {
		t.Fatalf("removal notice omitted remaining source: %q", notice)
	}
}

func TestTUIModelManagementCredentialRemovalIsIdempotent(t *testing.T) {
	root := t.TempDir()
	settings, err := config.New(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	backend := newTUIModelManagementBackend(settings, filepath.Join(root, "state"))
	result, err := backend.RemoveCredential("openai")
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed {
		t.Fatalf("missing credential reported removed: %#v", result)
	}
}

func TestTUIModelManagementSavesAndRemovesCustomProviderWithoutKeys(t *testing.T) {
	root := t.TempDir()
	settingsStore, err := config.New(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "team-gateway", Model: "coding-large"}
	settings.ModelAliases = map[string]string{"team-gateway:coding-large": "Large"}
	if err := settingsStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	backend := newTUIModelManagementBackend(settingsStore, filepath.Join(root, "state"))
	providers, err := backend.SaveCustomProvider(modelcatalog.CustomProviderSetup{
		ID: "team-gateway", BaseURL: "https://models.example.com/v1/chat/completions",
		APIKeyEnv: "TEAM_GATEWAY_API_KEY", Models: []string{"coding-large", "coding-small"}, DefaultModel: "coding-large",
	})
	if err != nil || len(providers) != 1 || providers[0].APIKeyEnv != "TEAM_GATEWAY_API_KEY" {
		t.Fatalf("save = %#v, err=%v", providers, err)
	}
	contents, err := os.ReadFile(settingsStore.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "sk-") || strings.Contains(string(contents), "TEAM_GATEWAY_API_KEY_VALUE") {
		t.Fatalf("config.json contained unexpected secret material: %s", contents)
	}
	if _, err := backend.SaveCustomProvider(modelcatalog.CustomProviderSetup{ID: "openai", BaseURL: "https://example.com/v1/chat/completions", Models: []string{"x"}}); err == nil {
		t.Fatal("built-in provider ID was accepted")
	}
	if _, err := backend.SaveCustomProvider(modelcatalog.CustomProviderSetup{ID: "gator-local", BaseURL: "http://127.0.0.1:11434/v1/chat/completions", Models: []string{"x"}}); err == nil {
		t.Fatal("gator-local was accepted")
	}
	providers, err = backend.RemoveCustomProvider("team-gateway")
	if err != nil || len(providers) != 0 {
		t.Fatalf("remove = %#v, err=%v", providers, err)
	}
	loaded, err := settingsStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Defaults.Provider != "" || loaded.Defaults.Model != "" || len(loaded.ModelAliases) != 0 {
		t.Fatalf("stale references remain: %#v", loaded)
	}
}

func TestTUIModelManagementDiscoveryPreviewAndApply(t *testing.T) {
	root := t.TempDir()
	settingsStore, err := config.New(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"data":[{"id":"alpha"},{"id":"beta"}]}`))
	}))
	defer server.Close()
	settings := config.Default()
	settings.CustomProviders = []config.CustomProvider{{
		ID: "team-gateway", BaseURL: server.URL + "/v1/chat/completions", Models: []string{"old"}, DefaultModel: "old",
	}}
	if err := settingsStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	backend := newTUIModelManagementBackend(settingsStore, filepath.Join(root, "state"))
	preview, err := backend.DiscoverCustomProvider("team-gateway")
	if err != nil || strings.Join(preview.Models, ",") != "alpha,beta" || preview.DefaultModel != "alpha" {
		t.Fatalf("preview = %#v, err=%v", preview, err)
	}
	loaded, err := settingsStore.Load()
	if err != nil || strings.Join(loaded.CustomProviders[0].Models, ",") != "old" {
		t.Fatalf("preview mutated config: %#v, err=%v", loaded.CustomProviders, err)
	}
	providers, err := backend.ApplyCustomProviderDiscovery("team-gateway", preview.Models)
	if err != nil || strings.Join(providers[0].Models, ",") != "alpha,beta" {
		t.Fatalf("apply = %#v, err=%v", providers, err)
	}
}

func TestModelCatalogClientRejectsOffOriginRedirect(t *testing.T) {
	external := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Fatal("off-origin catalog request was followed")
	}))
	defer external.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, external.URL+"/v1/models", http.StatusFound)
	}))
	defer origin.Close()
	_, err := discoverModels(config.CustomProvider{ID: "team-gateway", BaseURL: origin.URL + "/v1/chat/completions"})
	if err == nil || !strings.Contains(err.Error(), "off origin") {
		t.Fatalf("redirect error = %v", err)
	}
}

func TestLogoutClaudeRemovesAnthropicKey(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put("anthropic", auth.Credential{Type: "api_key", Key: "claude-secret"}); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := logout([]string{"claude"}, &output); err != nil {
		t.Fatal(err)
	}
	if _, found, err := credentials.Read("anthropic"); err != nil || found {
		t.Fatalf("anthropic key remains found=%v err=%v", found, err)
	}
	if strings.Contains(output.String(), "claude-secret") {
		t.Fatalf("logout leaked secret: %q", output.String())
	}
	if !strings.Contains(output.String(), "Removed the Gator API key for claude") {
		t.Fatalf("logout output = %q", output.String())
	}
}

func TestProviderRemoveClearsDefaults(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output strings.Builder
	if err := providerCommand([]string{"add", "team-gateway", "--base-url", "https://models.example.com/v1/chat/completions", "--model", "coding-large", "--api-key-env", "TEAM_GATEWAY_API_KEY"}, &output); err != nil {
		t.Fatal(err)
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	settings.Defaults = config.Defaults{Provider: "team-gateway", Model: "coding-large"}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := providerCommand([]string{"remove", "team-gateway", "--yes"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "environment variable was not changed") {
		t.Fatalf("remove output = %q", output.String())
	}
	settings, err = store.Load()
	if err != nil || settings.Defaults.Provider != "" || len(settings.CustomProviders) != 0 {
		t.Fatalf("settings after remove = %#v, err=%v", settings, err)
	}
}

func TestStoredCredentialStatusOmitsSecrets(t *testing.T) {
	status := storedCredentialStatus("openai", auth.Credential{Type: "oauth", Access: "access", Refresh: "refresh", Expires: time.Now().Add(time.Hour).UnixMilli()}, time.Now())
	if status.Kind != "OAuth credential" || !status.Present || status.Expired {
		t.Fatalf("status = %#v", status)
	}
}
