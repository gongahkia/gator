package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/llm"
)

func TestLoadPrecedence(t *testing.T) {
	clearPawEnv(t)
	path := writeConfig(t, `
max_turns = 7
max_brain_tokens = 123
call_timeout = "3s"

[brain]
transport = "openai"
base_url = "https://file.example/v1"
api_key = "file-key"
provider = "file-provider"
model = "file-brain"

[drone]
transport = "ollama"
base_url = "http://file-drone"
provider = "file-drone-provider"
model = "file-drone"

[gather]
max_depth = 2
max_file_bytes = 99

[verify]
command = "go test ./pkg"
timeout = "10s"

[tls]
ca_file = "/file/ca.pem"
insecure_skip_verify = false
`)
	t.Setenv("PAW_BRAIN_MODEL", "env-brain")
	t.Setenv("PAW_BRAIN_PROVIDER", "env-provider")
	t.Setenv("PAW_MAX_TURNS", "9")
	t.Setenv("PAW_CALL_TIMEOUT", "5s")
	t.Setenv("PAW_GATHER_MAX_FILE_BYTES", "256")
	t.Setenv("PAW_VERIFY_TIMEOUT", "15s")
	t.Setenv("PAW_TLS_CA_FILE", "/env/ca.pem")
	t.Setenv("PAW_INSECURE_SKIP_TLS_VERIFY", "true")
	t.Setenv("PAW_POLICY_ALLOWED_TRANSPORTS", "ollama,openai")
	t.Setenv("PAW_POLICY_ALLOWED_BASE_URLS", "https://one.example/v1, https://two.example/v1")
	t.Setenv("PAW_POLICY_ALLOW_LOOPBACK", "false")
	t.Setenv("PAW_POLICY_AUTO_APPROVE", "true")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.BaseURL != "https://file.example/v1" || cfg.Brain.APIKey != "file-key" {
		t.Fatalf("brain file overrides failed: %#v", cfg.Brain)
	}
	if cfg.Brain.Model != "env-brain" {
		t.Fatalf("brain env override failed: %#v", cfg.Brain)
	}
	if cfg.Brain.Provider != "env-provider" {
		t.Fatalf("brain provider env override failed: %#v", cfg.Brain)
	}
	if cfg.Drone.BaseURL != "http://file-drone" || cfg.Drone.Provider != "file-drone-provider" || cfg.Drone.Model != "file-drone" {
		t.Fatalf("drone file overrides failed: %#v", cfg.Drone)
	}
	if cfg.MaxTurns != 9 || cfg.MaxBrainTokens != 123 || cfg.CallTimeout != 5*time.Second {
		t.Fatalf("top-level precedence failed: %#v", cfg)
	}
	if cfg.Gather.MaxDepth != 2 || cfg.Gather.MaxFileBytes != 256 {
		t.Fatalf("gather precedence failed: %#v", cfg.Gather)
	}
	if cfg.Verify.Command != "go test ./pkg" || cfg.Verify.Timeout != 15*time.Second {
		t.Fatalf("verify precedence failed: %#v", cfg.Verify)
	}
	if cfg.TLS.CAFile != "/env/ca.pem" || !cfg.TLS.InsecureSkipVerify {
		t.Fatalf("tls precedence failed: %#v", cfg.TLS)
	}
	if got, want := strings.Join(cfg.Policy.Provider.AllowedTransports, ","), "ollama,openai"; got != want || strings.Join(cfg.Policy.Provider.AllowedBaseURLs, ",") != "https://one.example/v1,https://two.example/v1" || cfg.Policy.Provider.AllowLoopback || !cfg.Policy.Approval.AutoApprove {
		t.Fatalf("policy env precedence failed: %#v", cfg.Policy)
	}
}

func TestDefaultsAreLocalFirst(t *testing.T) {
	cfg := Defaults()
	if cfg.Brain.Transport != "ollama" || cfg.Brain.BaseURL != "http://localhost:11434" || cfg.Brain.Model != "gpt-oss:20b" {
		t.Fatalf("brain defaults = %#v", cfg.Brain)
	}
	if cfg.Drone.Transport != "ollama" || cfg.Drone.BaseURL != "http://localhost:11434" || cfg.Drone.Model != "qwen3:8b" {
		t.Fatalf("drone defaults = %#v", cfg.Drone)
	}
	if cfg.OllamaAutoPull {
		t.Fatal("ollama auto-pull defaulted true")
	}
	if cfg.TLS.CAFile != "" || cfg.TLS.InsecureSkipVerify {
		t.Fatalf("tls defaults = %#v", cfg.TLS)
	}
	if cfg.Verify.Timeout != 2*time.Minute {
		t.Fatalf("verify defaults = %#v", cfg.Verify)
	}
	if cfg.Policy.Version != PolicySchemaVersion || !cfg.Policy.Provider.AllowLoopback || !cfg.Policy.Egress.BlockSecrets || cfg.Policy.Approval.AutoApprove {
		t.Fatalf("policy defaults = %#v", cfg.Policy)
	}
}

func TestLoadPolicyV1Fixture(t *testing.T) {
	clearPawEnv(t)
	setTestHome(t)
	cfg, err := Load(filepath.Join("testdata", "policy-v1.toml"))
	if err != nil {
		t.Fatal(err)
	}
	policy := cfg.Policy
	if policy.Version != PolicySchemaVersion || strings.Join(policy.Provider.AllowedTransports, ",") != "ollama,openai" || policy.Command.Allow[0] != "go test ./..." || policy.Command.Deny[0] != "rm -rf" || !policy.Command.RequireApproval || policy.Risk.MaxFiles != 4 || policy.Risk.MaxLines != 120 || policy.Risk.MaxTokens != 4096 || policy.Risk.MaxCommands != 3 || !policy.Git.AllowCommit || !policy.Git.AllowPush || strings.Join(policy.Git.AllowedRemotes, ",") != "origin" || policy.Egress.MaxBytes != 4096 || policy.Approval.AutoApprove {
		t.Fatalf("policy = %#v", policy)
	}
}

func TestLoadRejectsPushWithoutAllowlistedRemote(t *testing.T) {
	clearPawEnv(t)
	setTestHome(t)
	path := writeConfig(t, `
[policy.git]
allow_push = true
`)
	_, err := Load(path)
	var diagnostic *PolicyValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Path != "policy.git.allowed_remotes" {
		t.Fatalf("push allowlist error = %v", err)
	}
}

func TestValidatePolicyRejectsInvalidGitRemoteEntries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		remotes []string
		path    string
	}{
		{name: "empty", remotes: []string{""}, path: "policy.git.allowed_remotes[0]"},
		{name: "whitespace", remotes: []string{" origin"}, path: "policy.git.allowed_remotes[0]"},
		{name: "duplicate", remotes: []string{"origin", "origin"}, path: "policy.git.allowed_remotes[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Defaults().Policy
			cfg.Git.AllowedRemotes = tc.remotes
			err := ValidatePolicy(cfg)
			var diagnostic *PolicyValidationError
			if !errors.As(err, &diagnostic) || diagnostic.Path != tc.path {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
}

func TestLoadPolicyPrecedence(t *testing.T) {
	clearPawEnv(t)
	userConfig := setTestHome(t)
	writeConfigAt(t, userConfig, `
[policy.risk]
max_files = 2
`)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	writeConfigAt(t, filepath.Join(repo, ".paw", "config.toml"), `
[policy.provider]
allowed_transports = ["openai"]

[policy.risk]
max_files = 3
max_lines = 300
`)
	explicit := writeConfig(t, `
[policy.risk]
max_files = 4
`)
	chdir(t, repo)
	t.Setenv("PAW_POLICY_ALLOWED_TRANSPORTS", "anthropic")
	t.Setenv("PAW_POLICY_AUTO_APPROVE", "true")

	cfg, err := Load(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Policy.Version != PolicySchemaVersion || strings.Join(cfg.Policy.Provider.AllowedTransports, ",") != "anthropic" || cfg.Policy.Risk.MaxFiles != 4 || cfg.Policy.Risk.MaxLines != 300 || !cfg.Policy.Approval.AutoApprove {
		t.Fatalf("policy precedence = %#v", cfg.Policy)
	}
}

func TestPolicyRepoDiscoveryUsesNearestConfig(t *testing.T) {
	clearPawEnv(t)
	setTestHome(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	writeConfigAt(t, filepath.Join(repo, ".paw", "config.toml"), `
[policy.risk]
max_commands = 3
`)
	nested := filepath.Join(repo, "services", "api")
	mkdir(t, nested)
	writeConfigAt(t, filepath.Join(repo, "services", ".paw", "config.toml"), `
[policy.risk]
max_commands = 2
`)
	chdir(t, nested)

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Policy.Risk.MaxCommands != 2 {
		t.Fatalf("nearest policy max commands = %d", cfg.Policy.Risk.MaxCommands)
	}
}

func TestLoadRejectsPolicyWithPathDiagnostic(t *testing.T) {
	clearPawEnv(t)
	setTestHome(t)
	path := writeConfig(t, `
[policy.risk]
max_commands = 0
`)
	_, err := Load(path)
	var diagnostic *PolicyValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Path != "policy.risk.max_commands" || diagnostic.Reason != "must be greater than zero" {
		t.Fatalf("policy diagnostic = %#v err=%v", diagnostic, err)
	}
}

func TestLoadOllamaAutoPullOptIn(t *testing.T) {
	clearPawEnv(t)
	path := writeConfig(t, `ollama_auto_pull = true`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.OllamaAutoPull {
		t.Fatalf("file opt-in not loaded: %#v", cfg)
	}
	t.Setenv("PAW_OLLAMA_AUTO_PULL", "false")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("load env: %v", err)
	}
	if cfg.OllamaAutoPull {
		t.Fatalf("env opt-out not loaded: %#v", cfg)
	}
}

func TestLoadRepoConfig(t *testing.T) {
	clearPawEnv(t)
	setTestHome(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	writeConfigAt(t, filepath.Join(repo, ".paw", "config.toml"), `
[brain]
model = "repo-brain"
`)
	work := filepath.Join(repo, "cmd", "paw")
	mkdir(t, work)
	chdir(t, work)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.Model != "repo-brain" {
		t.Fatalf("brain model = %q", cfg.Brain.Model)
	}
}

func TestLoadRepoOverridesUserAndEnvOverridesRepo(t *testing.T) {
	clearPawEnv(t)
	userConfig := setTestHome(t)
	writeConfigAt(t, userConfig, `
[brain]
model = "user-brain"
`)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	writeConfigAt(t, filepath.Join(repo, ".paw", "config.toml"), `
[brain]
model = "repo-brain"
`)
	chdir(t, repo)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.Model != "repo-brain" {
		t.Fatalf("repo override failed: %#v", cfg.Brain)
	}

	t.Setenv("PAW_BRAIN_MODEL", "env-brain")
	cfg, err = Load("")
	if err != nil {
		t.Fatalf("load env: %v", err)
	}
	if cfg.Brain.Model != "env-brain" {
		t.Fatalf("env override failed: %#v", cfg.Brain)
	}
}

func TestLoadExplicitConfigOverridesRepo(t *testing.T) {
	clearPawEnv(t)
	setTestHome(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	writeConfigAt(t, filepath.Join(repo, ".paw", "config.toml"), `
[brain]
model = "repo-brain"
`)
	explicit := writeConfig(t, `
[brain]
model = "explicit-brain"
`)
	chdir(t, repo)

	cfg, err := Load(explicit)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.Model != "explicit-brain" {
		t.Fatalf("explicit override failed: %#v", cfg.Brain)
	}
}

func TestRepoConfigNearestWins(t *testing.T) {
	clearPawEnv(t)
	setTestHome(t)
	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	writeConfigAt(t, filepath.Join(repo, ".paw", "config.toml"), `
[brain]
model = "root-brain"
`)
	nested := filepath.Join(repo, "services", "api")
	mkdir(t, nested)
	writeConfigAt(t, filepath.Join(repo, "services", ".paw", "config.toml"), `
[brain]
model = "nearest-brain"
`)
	chdir(t, nested)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.Model != "nearest-brain" {
		t.Fatalf("nearest config not used: %#v", cfg.Brain)
	}
}

func TestRepoConfigStopsAtGitRoot(t *testing.T) {
	clearPawEnv(t)
	setTestHome(t)
	parent := t.TempDir()
	writeConfigAt(t, filepath.Join(parent, ".paw", "config.toml"), `
[brain]
model = "parent-brain"
`)
	repo := filepath.Join(parent, "repo")
	work := filepath.Join(repo, "sub")
	mkdir(t, filepath.Join(repo, ".git"))
	mkdir(t, work)
	chdir(t, work)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.Model == "parent-brain" {
		t.Fatalf("walk crossed git root: %#v", cfg.Brain)
	}
}

func TestRepoConfigStopsAtHome(t *testing.T) {
	clearPawEnv(t)
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	mkdir(t, home)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	writeConfigAt(t, filepath.Join(tmp, ".paw", "config.toml"), `
[brain]
model = "above-home-brain"
`)
	work := filepath.Join(home, "work", "repo")
	mkdir(t, work)
	chdir(t, work)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.Model == "above-home-brain" {
		t.Fatalf("walk crossed home: %#v", cfg.Brain)
	}
}

func TestLoadFailsFastOnMissingBrainKey(t *testing.T) {
	clearPawEnv(t)
	path := writeConfig(t, `
[brain]
transport = "openai"
base_url = "https://brain.example/v1"
model = "glm-test"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.APIKey != "" {
		t.Fatalf("brain api key = %q", cfg.Brain.APIKey)
	}
	client, err := llm.NewBrainClient(llm.FactoryConfig{
		Brain: llm.EndpointConfig{
			Transport: cfg.Brain.Transport,
			BaseURL:   cfg.Brain.BaseURL,
			APIKey:    cfg.Brain.APIKey,
			Model:     cfg.Brain.Model,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "PAW_BRAIN_API_KEY") {
		t.Fatalf("expected missing key factory error, got client=%T err=%v", client, err)
	}
}

func TestLoadAllowsAnthropicBrainTransport(t *testing.T) {
	clearPawEnv(t)
	path := writeConfig(t, `
[brain]
transport = "anthropic"
base_url = "https://api.deepseek.com/anthropic"
api_key = "deepseek-key"
model = "deepseek-v4-pro"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.Transport != "anthropic" || cfg.Brain.BaseURL != "https://api.deepseek.com/anthropic" {
		t.Fatalf("brain = %#v", cfg.Brain)
	}
	if cfg.Brain.APIKey != "deepseek-key" || cfg.Brain.Model != "deepseek-v4-pro" {
		t.Fatalf("brain = %#v", cfg.Brain)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	clearPawEnv(t)
	setTestHome(t)
	cfg := Defaults()
	cfg.Brain.Transport = "openai"
	cfg.Brain.BaseURL = "https://api.example/v1"
	cfg.Brain.APIKey = "brain-key"
	cfg.Brain.Model = "brain-model"
	cfg.Drone.Model = "drone-model"
	cfg.MaxTurns = 17
	cfg.CallTimeout = 4 * time.Second
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Brain.Model != "brain-model" || got.Drone.Model != "drone-model" || got.MaxTurns != 17 || got.CallTimeout != 4*time.Second {
		t.Fatalf("round trip = %#v", got)
	}
}

func clearPawEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PAW_BRAIN_TRANSPORT",
		"PAW_BRAIN_BASE_URL",
		"PAW_BRAIN_API_KEY",
		"PAW_BRAIN_PROVIDER",
		"PAW_BRAIN_MODEL",
		"PAW_DRONE_TRANSPORT",
		"PAW_DRONE_BASE_URL",
		"PAW_DRONE_API_KEY",
		"PAW_DRONE_PROVIDER",
		"PAW_DRONE_MODEL",
		"PAW_MAX_TURNS",
		"PAW_MAX_BRAIN_TOKENS",
		"PAW_CALL_TIMEOUT",
		"PAW_OLLAMA_AUTO_PULL",
		"PAW_TLS_CA_FILE",
		"PAW_INSECURE_SKIP_TLS_VERIFY",
		"PAW_GATHER_MAX_DEPTH",
		"PAW_GATHER_MAX_FILE_BYTES",
		"PAW_VERIFY_CMD",
		"PAW_VERIFY_TIMEOUT",
		"PAW_POLICY_ALLOWED_TRANSPORTS",
		"PAW_POLICY_ALLOWED_BASE_URLS",
		"PAW_POLICY_ALLOW_LOOPBACK",
		"PAW_POLICY_BLOCK_SECRETS",
		"PAW_POLICY_AUTO_APPROVE",
	} {
		t.Setenv(key, "")
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := t.TempDir() + "/config.toml"
	writeConfigAt(t, path, body)
	return path
}

func writeConfigAt(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return DefaultPath()
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func chdir(t *testing.T, path string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
