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

const SchemaVersion = config.PolicySchemaVersion

var ErrDenied = errors.New("policy denied operation")

func Validate(cfg config.PolicyConfig) error {
	return config.ValidatePolicy(cfg)
}

func CheckEndpoint(cfg config.PolicyConfig, transport, baseURL string) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	if !allowsTransport(cfg.Provider.AllowedTransports, transport) {
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

func allowsTransport(allowed []string, transport string) bool {
	for _, candidate := range allowed {
		if strings.EqualFold(candidate, transport) {
			return true
		}
	}
	return false
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
