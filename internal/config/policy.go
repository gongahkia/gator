package config

import (
	"fmt"
	"net/url"
	"strings"
)

type PolicyValidationError struct {
	Path   string
	Reason string
}

func (e *PolicyValidationError) Error() string {
	return e.Path + ": " + e.Reason
}

func ValidatePolicy(cfg PolicyConfig) error {
	if cfg.Version != PolicySchemaVersion {
		return invalidPolicy("policy.version", "unsupported schema %q; want %q", cfg.Version, PolicySchemaVersion)
	}
	if len(cfg.Provider.AllowedTransports) == 0 {
		return invalidPolicy("policy.provider.allowed_transports", "is required")
	}
	for i, transport := range cfg.Provider.AllowedTransports {
		if !supportedPolicyTransport(transport) {
			return invalidPolicy(fmt.Sprintf("policy.provider.allowed_transports[%d]", i), "unsupported transport %q", transport)
		}
	}
	for i, rawURL := range cfg.Provider.AllowedBaseURLs {
		if err := validatePolicyURL(rawURL); err != nil {
			return invalidPolicy(fmt.Sprintf("policy.provider.allowed_base_urls[%d]", i), "%v", err)
		}
	}
	if err := validateGitPolicy(cfg.Git); err != nil {
		return err
	}
	if err := positivePolicy("policy.risk.max_files", cfg.Risk.MaxFiles); err != nil {
		return err
	}
	if err := positivePolicy("policy.risk.max_lines", cfg.Risk.MaxLines); err != nil {
		return err
	}
	if err := positivePolicy("policy.risk.max_tokens", cfg.Risk.MaxTokens); err != nil {
		return err
	}
	if err := positivePolicy("policy.risk.max_commands", cfg.Risk.MaxCommands); err != nil {
		return err
	}
	if err := positivePolicy("policy.egress.max_files", cfg.Egress.MaxFiles); err != nil {
		return err
	}
	return positivePolicy("policy.egress.max_bytes", cfg.Egress.MaxBytes)
}

func invalidPolicy(path, format string, args ...any) error {
	return &PolicyValidationError{Path: path, Reason: fmt.Sprintf(format, args...)}
}

func supportedPolicyTransport(transport string) bool {
	switch strings.ToLower(transport) {
	case "openai", "anthropic", "ollama", "codex-cli", "gemini-cli", "claude-cli", "opencode-cli", "aider-cli", "goose-cli", "qwen-cli", "cursor-cli":
		return true
	default:
		return false
	}
}

func validatePolicyURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("must be an absolute http or https URL without credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("must not contain query or fragment")
	}
	return nil
}

func validateGitPolicy(git GitPolicy) error {
	if git.AllowPush && len(git.AllowedRemotes) == 0 {
		return invalidPolicy("policy.git.allowed_remotes", "is required when allow_push is true")
	}
	seen := make(map[string]struct{}, len(git.AllowedRemotes))
	for i, remote := range git.AllowedRemotes {
		path := fmt.Sprintf("policy.git.allowed_remotes[%d]", i)
		if strings.TrimSpace(remote) == "" {
			return invalidPolicy(path, "must not be empty")
		}
		if remote != strings.TrimSpace(remote) {
			return invalidPolicy(path, "must not have leading or trailing whitespace")
		}
		if _, ok := seen[remote]; ok {
			return invalidPolicy(path, "duplicates %q", remote)
		}
		seen[remote] = struct{}{}
	}
	return nil
}

func positivePolicy(path string, value int) error {
	if value <= 0 {
		return invalidPolicy(path, "must be greater than zero")
	}
	return nil
}
