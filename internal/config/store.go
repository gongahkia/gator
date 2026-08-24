// Package config stores non-secret Gator settings.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gongahkia/gator/internal/hooks"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/mcp"
	"github.com/gongahkia/gator/internal/sandbox"
)

const version = 2

// Settings is the single user-owned configuration document. Credentials do
// not belong here; they remain in Gator's private auth store.
type Settings struct {
	Version  int      `json:"version"`
	Defaults Defaults `json:"defaults"`
	Theme    string   `json:"theme,omitempty"`
	// ProviderEndpoints stores explicit non-secret endpoint overrides by
	// provider ID. Credentials remain exclusively in Gator's auth store.
	ProviderEndpoints map[string]string `json:"provider_endpoints,omitempty"`
	// ProviderOptions stores provider-specific non-secret settings such as an
	// Azure resource, Cloudflare account ID, or Vertex project and location.
	// Secrets never belong here; they remain exclusively in auth.json.
	ProviderOptions map[string]map[string]string `json:"provider_options,omitempty"`
	Extensions      []Extension                  `json:"extensions,omitempty"`
	CustomProviders []CustomProvider             `json:"custom_providers,omitempty"`
	// ModelAliases changes only a model's local display label. Its key is the
	// stable provider:model identity; Gator always sends the real model ID.
	ModelAliases    map[string]string `json:"model_aliases,omitempty"`
	ExtensionTrusts []ExtensionTrust  `json:"extension_trusts,omitempty"`
	HookTrusts      []hooks.Trust     `json:"hook_trusts,omitempty"`
	LSPTrusts       []lsp.Trust       `json:"lsp_trusts,omitempty"`
	MCPTrusts       []mcp.Trust       `json:"mcp_trusts,omitempty"`
	Execution       sandbox.Policy    `json:"execution"`
}

// Defaults applies when an interactive session or scripted run does not name
// a provider or model explicitly.
type Defaults struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// Extension records whether a globally installed extension is available to
// native runs. Extension files live in Gator's data directory, not config.json.
type Extension struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// ExtensionTrust pins the full content of a project's executable extension
// bundle. A repository path alone is never sufficient authority to execute
// project-controlled code.
type ExtensionTrust struct {
	Repository string `json:"repository"`
	Hash       string `json:"hash"`
}

// CustomProvider persists one OpenAI-compatible Chat Completions endpoint.
// APIKeyEnv identifies a process environment variable; config.json never
// contains the key itself. An empty APIKeyEnv is the intended keyless route for
// local servers such as Ollama, LM Studio, and vLLM when configured that way.
type CustomProvider struct {
	ID           string   `json:"id"`
	BaseURL      string   `json:"base_url"`
	APIKeyEnv    string   `json:"api_key_env,omitempty"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"default_model"`
}

// ModelAliasKey identifies a model display alias. Provider IDs cannot contain
// colons, so this is unambiguous even when a model ID itself contains one.
func ModelAliasKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + ":" + strings.TrimSpace(model)
}

var customProviderIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var environmentNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)
var providerOptionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Store owns config.json below one configuration root.
type Store struct {
	path string
}

// Default returns usable settings without requiring a file on disk.
func Default() Settings {
	return Settings{Version: version, Execution: sandbox.DefaultPolicy()}
}

// ProviderEndpoint returns a persisted non-secret endpoint override for one
// built-in provider. Callers may still supply an explicit one-run override.
func (s Settings) ProviderEndpoint(provider string) string {
	return strings.TrimSpace(s.ProviderEndpoints[strings.ToLower(strings.TrimSpace(provider))])
}

// OptionsForProvider returns a copy of persisted non-secret provider options.
// Callers can safely specialize it for one run without mutating Settings.
func (s Settings) OptionsForProvider(provider string) map[string]string {
	configured := s.ProviderOptions[strings.ToLower(strings.TrimSpace(provider))]
	if len(configured) == 0 {
		return nil
	}
	options := make(map[string]string, len(configured))
	for name, value := range configured {
		options[name] = strings.TrimSpace(value)
	}
	return options
}

// ResolveDir resolves Gator's configuration root. An explicit override and
// GATOR_CONFIG_DIR are useful for tests and managed installations.
func ResolveDir(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return absoluteDirectory(override)
	}
	if configured := os.Getenv("GATOR_CONFIG_DIR"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	if configured := os.Getenv("XDG_CONFIG_HOME"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}

// New creates a settings store below configurationRoot.
func New(configurationRoot string) (Store, error) {
	if strings.TrimSpace(configurationRoot) == "" {
		return Store{}, errors.New("configuration directory is required")
	}
	root, err := absoluteDirectory(configurationRoot)
	if err != nil {
		return Store{}, err
	}
	return Store{path: filepath.Join(root, "gator", "config.json")}, nil
}

// DefaultStore opens the standard user settings location.
func DefaultStore() (Store, error) {
	directory, err := ResolveDir("")
	if err != nil {
		return Store{}, err
	}
	return New(directory)
}

// Path returns the settings path without reading its contents.
func (s Store) Path() string {
	return s.path
}

// Load reads settings. A missing file deliberately resolves to defaults so a
// new install is immediately usable with environment credentials.
func (s Store) Load() (Settings, error) {
	if strings.TrimSpace(s.path) == "" {
		return Settings{}, errors.New("configuration store is not initialized")
	}
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("stat configuration file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Settings{}, errors.New("configuration file is not a regular file")
	}
	if info.Size() > 1024*1024 {
		return Settings{}, errors.New("configuration file exceeds 1 MiB")
	}
	contents, err := os.ReadFile(s.path)
	if err != nil {
		return Settings{}, fmt.Errorf("read configuration file: %w", err)
	}
	settings := Default()
	if err := json.Unmarshal(contents, &settings); err != nil {
		return Settings{}, fmt.Errorf("decode configuration file: %w", err)
	}
	if err := validate(settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

// Save atomically publishes the entire settings document. Callers normally
// load, change one field, then save so the document stays understandable when
// inspected or managed outside Gator.
func (s Store) Save(settings Settings) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("configuration store is not initialized")
	}
	if settings.Version == 0 {
		settings.Version = version
	}
	if err := validate(settings); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create configuration directory: %w", err)
	}
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration file: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".config-*")
	if err != nil {
		return fmt.Errorf("create configuration file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set configuration file permissions: %w", err)
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close configuration file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("publish configuration file: %w", err)
	}
	return nil
}

func validate(settings Settings) error {
	if settings.Version != version {
		return fmt.Errorf("unsupported configuration version %d", settings.Version)
	}
	if len(settings.Defaults.Provider) > 128 || len(settings.Defaults.Model) > 512 {
		return errors.New("configuration default exceeds its size limit")
	}
	if settings.Theme != "" && settings.Theme != "gator" && settings.Theme != "contrast" && settings.Theme != "mono" {
		return fmt.Errorf("unknown theme %q", settings.Theme)
	}
	if err := settings.Execution.Validate(); err != nil {
		return fmt.Errorf("invalid execution policy: %w", err)
	}
	if len(settings.Extensions) > 256 {
		return errors.New("configuration has too many extensions")
	}
	if len(settings.CustomProviders) > 128 {
		return errors.New("configuration has too many custom providers")
	}
	if len(settings.ModelAliases) > 512 {
		return errors.New("configuration has too many model aliases")
	}
	if len(settings.ProviderEndpoints) > 128 {
		return errors.New("configuration has too many provider endpoint overrides")
	}
	for provider, baseURL := range settings.ProviderEndpoints {
		if !customProviderIDPattern.MatchString(provider) {
			return fmt.Errorf("invalid provider endpoint ID %q", provider)
		}
		if len(baseURL) == 0 || len(baseURL) > 2048 {
			return fmt.Errorf("provider endpoint %q must be 1-2048 bytes", provider)
		}
		endpoint, err := url.Parse(baseURL)
		if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
			return fmt.Errorf("provider endpoint %q requires an absolute http(s) URL", provider)
		}
	}
	if len(settings.ProviderOptions) > 128 {
		return errors.New("configuration has too many provider option sets")
	}
	for provider, options := range settings.ProviderOptions {
		if !customProviderIDPattern.MatchString(provider) {
			return fmt.Errorf("invalid provider option ID %q", provider)
		}
		if len(options) == 0 || len(options) > 16 {
			return fmt.Errorf("provider option set %q must contain 1-16 values", provider)
		}
		for name, value := range options {
			if !providerOptionNamePattern.MatchString(name) {
				return fmt.Errorf("invalid provider option name %q", name)
			}
			if strings.TrimSpace(value) == "" || len(value) > 2048 || strings.ContainsAny(value, "\r\n") {
				return fmt.Errorf("provider option %q for %q is invalid", name, provider)
			}
		}
	}
	for key, value := range settings.ModelAliases {
		if strings.TrimSpace(key) == "" || len(key) > 1024 || strings.ContainsAny(key, "\r\n") {
			return errors.New("model alias key is invalid")
		}
		if strings.TrimSpace(value) == "" || len(value) > 128 || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("model alias %q is invalid", key)
		}
	}
	providers := make(map[string]struct{}, len(settings.CustomProviders))
	for _, provider := range settings.CustomProviders {
		if !customProviderIDPattern.MatchString(provider.ID) {
			return fmt.Errorf("invalid custom provider ID %q", provider.ID)
		}
		if _, exists := providers[provider.ID]; exists {
			return fmt.Errorf("custom provider %q is configured more than once", provider.ID)
		}
		providers[provider.ID] = struct{}{}
		if len(provider.BaseURL) == 0 || len(provider.BaseURL) > 2048 {
			return fmt.Errorf("custom provider %q requires a base URL no longer than 2048 bytes", provider.ID)
		}
		endpoint, err := url.Parse(provider.BaseURL)
		if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
			return fmt.Errorf("custom provider %q requires an absolute http(s) base URL", provider.ID)
		}
		if provider.APIKeyEnv != "" && !environmentNamePattern.MatchString(provider.APIKeyEnv) {
			return fmt.Errorf("custom provider %q has an invalid API key environment variable", provider.ID)
		}
		if len(provider.Models) == 0 || len(provider.Models) > 128 || len(provider.DefaultModel) == 0 || len(provider.DefaultModel) > 512 {
			return fmt.Errorf("custom provider %q requires 1-128 models and a default model", provider.ID)
		}
		models := make(map[string]struct{}, len(provider.Models))
		for _, name := range provider.Models {
			if strings.TrimSpace(name) == "" || len(name) > 512 || strings.ContainsAny(name, "\r\n") {
				return fmt.Errorf("custom provider %q has an invalid model", provider.ID)
			}
			if _, exists := models[name]; exists {
				return fmt.Errorf("custom provider %q lists model %q more than once", provider.ID, name)
			}
			models[name] = struct{}{}
		}
		if _, exists := models[provider.DefaultModel]; !exists {
			return fmt.Errorf("custom provider %q default model %q is not listed", provider.ID, provider.DefaultModel)
		}
	}
	enabled := make(map[string]struct{}, len(settings.Extensions))
	for _, extension := range settings.Extensions {
		id := strings.TrimSpace(extension.ID)
		if id == "" || len(id) > 128 {
			return errors.New("extension ID is required and must be at most 128 bytes")
		}
		if _, exists := enabled[id]; exists {
			return fmt.Errorf("extension %q is configured more than once", id)
		}
		enabled[id] = struct{}{}
	}
	if len(settings.ExtensionTrusts) > 256 {
		return errors.New("configuration has too many extension trust records")
	}
	trustedExtensions := make(map[string]struct{}, len(settings.ExtensionTrusts))
	for _, trust := range settings.ExtensionTrusts {
		if strings.TrimSpace(trust.Repository) == "" || len(trust.Repository) > 4*1024 || !validSHA256(trust.Hash) {
			return errors.New("extension trust record is invalid")
		}
		if _, exists := trustedExtensions[trust.Repository]; exists {
			return fmt.Errorf("repository %q has more than one extension trust record", trust.Repository)
		}
		trustedExtensions[trust.Repository] = struct{}{}
	}
	if len(settings.HookTrusts) > 256 {
		return errors.New("configuration has too many hook trust records")
	}
	trustedHooks := make(map[string]struct{}, len(settings.HookTrusts))
	for _, trust := range settings.HookTrusts {
		if strings.TrimSpace(trust.Repository) == "" || len(trust.Repository) > 4*1024 {
			return errors.New("hook trust repository path is invalid")
		}
		if !validSHA256(trust.Hash) {
			return fmt.Errorf("hook trust for %q has an invalid hash", trust.Repository)
		}
		if _, exists := trustedHooks[trust.Repository]; exists {
			return fmt.Errorf("repository %q has more than one hook trust record", trust.Repository)
		}
		trustedHooks[trust.Repository] = struct{}{}
	}
	if len(settings.MCPTrusts) > 256 {
		return errors.New("configuration has too many MCP trust records")
	}
	trustedMCP := make(map[string]struct{}, len(settings.MCPTrusts))
	for _, trust := range settings.MCPTrusts {
		if strings.TrimSpace(trust.Repository) == "" || len(trust.Repository) > 4*1024 || !validSHA256(trust.Hash) {
			return errors.New("MCP trust record is invalid")
		}
		if _, exists := trustedMCP[trust.Repository]; exists {
			return fmt.Errorf("repository %q has more than one MCP trust record", trust.Repository)
		}
		trustedMCP[trust.Repository] = struct{}{}
	}
	if len(settings.LSPTrusts) > 256 {
		return errors.New("configuration has too many LSP trust records")
	}
	trustedLSP := make(map[string]struct{}, len(settings.LSPTrusts))
	for _, trust := range settings.LSPTrusts {
		if strings.TrimSpace(trust.Repository) == "" || len(trust.Repository) > 4*1024 || !validSHA256(trust.Hash) {
			return errors.New("LSP trust record is invalid")
		}
		if _, exists := trustedLSP[trust.Repository]; exists {
			return fmt.Errorf("repository %q has more than one LSP trust record", trust.Repository)
		}
		trustedLSP[trust.Repository] = struct{}{}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func absoluteDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve configuration directory: %w", err)
	}
	return abs, nil
}
