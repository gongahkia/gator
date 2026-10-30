// Package cliagent adapts explicit user-configured coding CLIs to Gator's
// specialist supervisor. Each invocation owns a private copy and OS sandbox.
package cliagent

import (
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/sandbox"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	Kind           string   `json:"kind"`
	Executable     string   `json:"executable"`
	BaseURL        string   `json:"base_url,omitempty"`
	Model          string   `json:"model,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	APIKeyEnv      string   `json:"api_key_env"`
	ReadOnlyRoots  []string `json:"runtime_roots,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	AllowEdits     bool     `json:"allow_edits,omitempty"`
}

func (c Config) Role() string { return "cli_" + c.Kind }
func (c Config) Validate() error {
	if c.Kind != "codex" && c.Kind != "claude" && c.Kind != "pi" {
		return errors.New("CLI agent must be codex, claude or pi")
	}
	if !filepath.IsAbs(c.Executable) || strings.ContainsAny(c.Executable, "\x00\r\n") {
		return errors.New("CLI executable must be an absolute path")
	}
	if c.TimeoutSeconds < 10 || c.TimeoutSeconds > 1800 {
		return errors.New("CLI timeout must be 10 through 1800 seconds")
	}
	if !regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`).MatchString(c.APIKeyEnv) || !strings.HasSuffix(c.APIKeyEnv, "_API_KEY") {
		return errors.New("select an API key environment variable ending in _API_KEY; never store its value")
	}
	for _, v := range []string{c.Model, c.Provider} {
		if len(v) > 256 || strings.ContainsAny(v, "\x00\r\n") {
			return errors.New("invalid CLI model or provider")
		}
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(c.BaseURL, "\x00\r\n") {
			return errors.New("CLI base URL must be HTTPS without credentials, query or fragment")
		}
	}
	if c.Provider != "" && c.Kind != "pi" {
		return errors.New("provider selection is only available for Pi")
	}
	if len(c.ReadOnlyRoots) > 8 {
		return errors.New("too many CLI runtime roots")
	}
	return (sandbox.Policy{ReadOnlyRoots: c.ReadOnlyRoots}).Validate()
}
func ValidateAll(configs []Config) error {
	if len(configs) > 3 {
		return errors.New("at most three CLI adapters may be enabled")
	}
	seen := map[string]bool{}
	for _, c := range configs {
		if err := c.Validate(); err != nil {
			return err
		}
		if seen[c.Kind] {
			return fmt.Errorf("duplicate CLI adapter %s", c.Kind)
		}
		seen[c.Kind] = true
	}
	return nil
}
func (c Config) Arguments(edit bool) []string {
	switch c.Kind {
	case "codex":
		// Gator establishes the strict OS sandbox before executing this argv.
		// Nested Seatbelt cannot initialize on macOS. The child's sandbox mode
		// does not relax Gator's enforced read/write envelope.
		args := []string{"exec", "--json", "--skip-git-repo-check", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--sandbox", "danger-full-access", "-c", `approval_policy="never"`, "--disable", "multi_agent", "--color", "never"}
		if c.BaseURL != "" {
			args = append(args, "-c", `model_provider="gator"`, "-c", `model_providers.gator.name="Gator selected endpoint"`, "-c", "model_providers.gator.base_url="+strconv.Quote(c.BaseURL), "-c", `model_providers.gator.env_key="CODEX_API_KEY"`, "-c", `model_providers.gator.wire_api="responses"`)
		}
		if c.Model != "" {
			args = append(args, "--model", c.Model)
		}
		return append(args, "-")
	case "claude":
		tools := "Read,Glob,Grep"
		if edit {
			tools += ",Edit,Write"
		}
		args := []string{"--bare", "--print", "--output-format", "stream-json", "--verbose", "--tools", tools, "--allowedTools", tools, "--max-turns", "16", "--no-session-persistence"}
		if c.Model != "" {
			args = append(args, "--model", c.Model)
		}
		return args
	default:
		selected := "read,grep,find,ls"
		if edit {
			selected += ",write,edit"
		}
		args := []string{"--print", "--mode", "json", "--no-session", "--no-extensions", "--no-skills", "--no-prompt-templates", "--no-context-files", "--no-approve", "--offline", "--tools", selected}
		if c.Model != "" {
			args = append(args, "--model", c.Model)
		}
		provider := c.Provider
		if provider == "" {
			provider = "openai"
		}
		args = append(args, "--provider", provider)
		return args
	}
}
func (c Config) CredentialTarget() string {
	if c.Kind == "codex" {
		return "CODEX_API_KEY"
	}
	if c.Kind == "claude" {
		return "ANTHROPIC_API_KEY"
	}
	return c.APIKeyEnv
}
