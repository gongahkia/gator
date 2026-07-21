package policy

import (
	"errors"
	"testing"

	"github.com/gongahkia/paw/internal/config"
)

func TestDefaultsAllowOnlyLoopbackOllama(t *testing.T) {
	cfg := config.Defaults().Policy
	if err := CheckEndpoint(cfg, "ollama", "http://localhost:11434/"); err != nil {
		t.Fatalf("loopback ollama denied: %v", err)
	}
	if err := CheckEndpoint(cfg, "openai", "https://api.example.test/v1"); !errors.Is(err, ErrDenied) {
		t.Fatalf("remote openai error = %v", err)
	}
}

func TestCheckEndpointRequiresExactAllowlistedBaseURL(t *testing.T) {
	cfg := config.Defaults().Policy
	cfg.Provider.AllowedTransports = append(cfg.Provider.AllowedTransports, "openai")
	cfg.Provider.AllowedBaseURLs = []string{"https://api.example.test/v1"}
	if err := CheckEndpoint(cfg, "openai", "https://api.example.test/v1/"); err != nil {
		t.Fatalf("allowlisted endpoint denied: %v", err)
	}
	if err := CheckEndpoint(cfg, "openai", "https://api.example.test/v2"); !errors.Is(err, ErrDenied) {
		t.Fatalf("unlisted endpoint error = %v", err)
	}
}

func TestCheckEndpointMatchesTransportCaseInsensitively(t *testing.T) {
	cfg := config.Defaults().Policy
	cfg.Provider.AllowedTransports = []string{"OpenAI"}
	cfg.Provider.AllowedBaseURLs = []string{"https://api.example.test/v1"}
	if err := CheckEndpoint(cfg, "openai", "https://api.example.test/v1"); err != nil {
		t.Fatalf("mixed-case allowlisted transport denied: %v", err)
	}
}

func TestValidateRejectsBadBudget(t *testing.T) {
	cfg := config.Defaults().Policy
	cfg.Risk.MaxFiles = 0
	err := Validate(cfg)
	var diagnostic *config.PolicyValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Path != "policy.risk.max_files" {
		t.Fatalf("budget validation error = %v", err)
	}
}

func TestValidateRejectsUnknownSchemaVersion(t *testing.T) {
	cfg := config.Defaults().Policy
	cfg.Version = "paw.policy/unknown"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected version validation error")
	}
}

func TestCheckGitRemoteRequiresPushAndRemote(t *testing.T) {
	cfg := config.Defaults().Policy
	if err := CheckGitRemote(cfg, "origin"); !errors.Is(err, ErrDenied) {
		t.Fatalf("disabled push error = %v", err)
	}
	cfg.Git.AllowPush = true
	cfg.Git.AllowedRemotes = []string{"origin"}
	if err := CheckGitRemote(cfg, "origin"); err != nil {
		t.Fatalf("allowlisted remote denied: %v", err)
	}
	if err := CheckGitRemote(cfg, "origin-mirror"); !errors.Is(err, ErrDenied) {
		t.Fatalf("nonallowlisted remote error = %v", err)
	}
}

func TestCheckGitRemoteRejectsInvalidAllowlist(t *testing.T) {
	cfg := config.Defaults().Policy
	cfg.Git.AllowPush = true
	err := CheckGitRemote(cfg, "origin")
	var diagnostic *config.PolicyValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Path != "policy.git.allowed_remotes" {
		t.Fatalf("invalid allowlist error = %v", err)
	}
}
