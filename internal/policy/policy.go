package policy

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/gongahkia/paw/internal/config"
)

const SchemaVersion = "paw.policy/1"

var ErrDenied = errors.New("policy denied operation")

func Validate(cfg config.PolicyConfig) error {
	if cfg.Version != SchemaVersion {
		return fmt.Errorf("policy version %q: want %q", cfg.Version, SchemaVersion)
	}
	if len(cfg.Provider.AllowedTransports) == 0 {
		return errors.New("policy.provider.allowed_transports is required")
	}
	for _, transport := range cfg.Provider.AllowedTransports {
		if !supportedTransport(transport) {
			return fmt.Errorf("policy.provider.allowed_transports: unsupported transport %q", transport)
		}
	}
	for _, rawURL := range cfg.Provider.AllowedBaseURLs {
		if _, err := normalizedURL(rawURL); err != nil {
			return fmt.Errorf("policy.provider.allowed_base_urls: %w", err)
		}
	}
	if err := positive("policy.risk.max_files", cfg.Risk.MaxFiles); err != nil {
		return err
	}
	if err := positive("policy.risk.max_lines", cfg.Risk.MaxLines); err != nil {
		return err
	}
	if err := positive("policy.risk.max_tokens", cfg.Risk.MaxTokens); err != nil {
		return err
	}
	if err := positive("policy.risk.max_commands", cfg.Risk.MaxCommands); err != nil {
		return err
	}
	if err := positive("policy.egress.max_files", cfg.Egress.MaxFiles); err != nil {
		return err
	}
	return positive("policy.egress.max_bytes", cfg.Egress.MaxBytes)
}

func CheckEndpoint(cfg config.PolicyConfig, transport, baseURL string) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	if !slices.Contains(cfg.Provider.AllowedTransports, strings.ToLower(transport)) {
		return fmt.Errorf("%w: transport %q", ErrDenied, transport)
	}
	if isCLITransport(transport) {
		return nil
	}
	normalized, err := normalizedURL(baseURL)
	if err != nil {
		return fmt.Errorf("%w: endpoint %q: %v", ErrDenied, baseURL, err)
	}
	if cfg.Provider.AllowLoopback && isLoopback(normalized) {
		return nil
	}
	for _, allowed := range cfg.Provider.AllowedBaseURLs {
		candidate, err := normalizedURL(allowed)
		if err == nil && candidate == normalized {
			return nil
		}
	}
	return fmt.Errorf("%w: endpoint %q is not allowlisted", ErrDenied, normalized)
}

func CheckGitRemote(cfg config.PolicyConfig, remote string) error {
	if !cfg.Git.AllowPush {
		return fmt.Errorf("%w: git push is disabled", ErrDenied)
	}
	if !slices.Contains(cfg.Git.AllowedRemotes, remote) {
		return fmt.Errorf("%w: git remote %q", ErrDenied, remote)
	}
	return nil
}

func supportedTransport(transport string) bool {
	switch strings.ToLower(transport) {
	case "openai", "anthropic", "ollama", "codex-cli", "gemini-cli", "claude-cli", "opencode-cli", "aider-cli", "goose-cli", "qwen-cli", "cursor-cli":
		return true
	default:
		return false
	}
}

func isCLITransport(transport string) bool {
	switch strings.ToLower(transport) {
	case "codex-cli", "gemini-cli", "claude-cli", "opencode-cli", "aider-cli", "goose-cli", "qwen-cli", "cursor-cli":
		return true
	default:
		return false
	}
}

func normalizedURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("must be an absolute http or https URL without credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("must not contain query or fragment")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

func isLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func positive(name string, value int) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than zero", name)
	}
	return nil
}
