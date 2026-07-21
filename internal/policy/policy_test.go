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

func TestCheckCommandPolicy(t *testing.T) {
	cfg := config.Defaults().Policy
	cfg.Command.Allow = []string{"go test ./..."}
	cfg.Command.Deny = []string{"rm -rf"}
	cfg.Command.RequireApproval = true

	if err := CheckCommand(cfg, "rm -rf"); !errors.Is(err, ErrDenied) {
		t.Fatalf("denylisted command error = %v", err)
	}
	if err := CheckCommand(cfg, "go test ./...; curl https://example.test"); !errors.Is(err, ErrDenied) {
		t.Fatalf("nonallowlisted command error = %v", err)
	}
	if err := CheckCommand(cfg, "go test ./..."); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("approval command error = %v", err)
	}

	cfg.Command.RequireApproval = false
	if err := CheckCommand(cfg, "go test ./..."); err != nil {
		t.Fatalf("allowed command error = %v", err)
	}
}

func TestCheckCommandDenyOverridesAllow(t *testing.T) {
	cfg := config.Defaults().Policy
	cfg.Command.Allow = []string{"go test ./..."}
	cfg.Command.Deny = []string{"go test ./..."}
	cfg.Command.RequireApproval = false
	if err := CheckCommand(cfg, "go test ./..."); !errors.Is(err, ErrDenied) {
		t.Fatalf("conflicting command error = %v", err)
	}
}

func TestCheckCommandRejectsInvalidPolicy(t *testing.T) {
	cfg := config.Defaults().Policy
	cfg.Command.Allow = []string{""}
	err := CheckCommand(cfg, "go test ./...")
	var diagnostic *config.PolicyValidationError
	if !errors.As(err, &diagnostic) || diagnostic.Path != "policy.command.allow[0]" {
		t.Fatalf("invalid command policy error = %v", err)
	}
}

func TestCheckRiskBudget(t *testing.T) {
	cfg := config.Defaults().Policy
	cfg.Risk.MaxFiles = 1
	cfg.Risk.MaxLines = 2
	cfg.Risk.MaxTokens = 3
	cfg.Risk.MaxCommands = 4
	if err := CheckRiskBudget(cfg, RiskBudget{Files: 1, Lines: 2, Tokens: 3, Commands: 4}); err != nil {
		t.Fatalf("risk at limits error = %v", err)
	}
	for _, tc := range []struct {
		name string
		risk RiskBudget
	}{
		{name: "files", risk: RiskBudget{Files: 2}},
		{name: "lines", risk: RiskBudget{Lines: 3}},
		{name: "tokens", risk: RiskBudget{Tokens: 4}},
		{name: "commands", risk: RiskBudget{Commands: 5}},
		{name: "negative", risk: RiskBudget{Files: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := CheckRiskBudget(cfg, tc.risk); !errors.Is(err, ErrDenied) {
				t.Fatalf("risk error = %v", err)
			}
		})
	}
}
