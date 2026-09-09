package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/gongahkia/gator/internal/model"
)

const connectUsage = `Usage:
  gator connect openai
  gator connect anthropic
  gator connect gemini
  gator connect codex [--device]
  gator connect copilot [--host URL]
  gator connect kimi
  gator connect xai
  gator connect claude
  gator connect openrouter
  gator connect radius

Connect starts the closest supported sign-in route. It never stores or
translates a vendor CLI credential into Gator. Claude, Radius, and native xAI
runs use API credentials held by Gator; their provider account subscription
flows remain separate.`

// connect is the short, provider-first onboarding path. Native `gator login`
// remains available for direct Gator API providers and product-owned OAuth
// clients, while this command uses an installed vendor CLI wherever that is
// the public, no-client-registration route.
func connect(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(connectUsage)
	}
	target := strings.ToLower(strings.TrimSpace(arguments[0]))
	remaining := arguments[1:]
	switch target {
	case "codex":
		return delegate(append([]string{"codex", "login"}, remaining...), out)
	case "copilot":
		return delegate(append([]string{"copilot", "login"}, remaining...), out)
	case "kimi", "kimi-coding":
		return delegate(append([]string{"kimi", "login"}, remaining...), out)
	case "xai", "grok":
		if len(remaining) != 0 {
			return errors.New("usage: gator connect xai")
		}
		if _, err := fmt.Fprintln(out, "Starting OpenCode's xAI provider login. Choose the browser or headless Grok subscription method, or an API key. Gator does not store or translate the credential."); err != nil {
			return err
		}
		return delegate([]string{"opencode", "login", "--provider", "xai"}, out)
	case "claude", "anthropic":
		if len(remaining) != 0 {
			return errors.New("usage: gator connect claude")
		}
		if _, err := fmt.Fprintln(out, "Claude Code delegation uses an Anthropic API key, not Claude.ai subscription OAuth."); err != nil {
			return err
		}
		if err := connectAPIKeyProvider("anthropic", out); err != nil {
			return fmt.Errorf("connect Claude: %w", err)
		}
		return nil
	case "openrouter":
		if len(remaining) != 0 {
			return errors.New("usage: gator connect openrouter")
		}
		return login([]string{"openrouter", "--subscription"}, out)
	case "radius":
		if len(remaining) != 0 {
			return errors.New("usage: gator connect radius")
		}
		if _, err := fmt.Fprintln(out, "Radius account OAuth requires a Gator-registered client. Connecting with a Radius API key instead."); err != nil {
			return err
		}
		if err := connectAPIKeyProvider("radius", out); err != nil {
			return fmt.Errorf("connect Radius: %w", err)
		}
		return nil
	default:
		provider, err := model.ParseProvider(target)
		if err == nil && model.SupportsAPIKeyLogin(provider) && model.APIKeyEnvironment(provider) != "" {
			if len(remaining) != 0 {
				return fmt.Errorf("usage: gator connect %s", target)
			}
			return connectAPIKeyProvider(string(provider), out)
		}
		return fmt.Errorf("unknown connection target %q\n\n%s", target, connectUsage)
	}
}

func connectAPIKeyProvider(providerName string, out io.Writer) error {
	provider, err := model.ParseProvider(providerName)
	if err != nil {
		return err
	}
	if !model.SupportsAPIKeyLogin(provider) {
		return fmt.Errorf("provider %q does not support API-key setup", provider)
	}
	environment := model.APIKeyEnvironment(provider)
	if strings.TrimSpace(os.Getenv(environment)) != "" {
		return login([]string{string(provider), "--from-env", environment}, out)
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("no API key found; set %s or run 'gator connect %s' in an interactive terminal", environment, provider)
	}
	if _, err := fmt.Fprintf(out, "Enter the %s API key (input hidden): ", provider); err != nil {
		return err
	}
	secret, err := term.ReadPassword(os.Stdin.Fd())
	_, newlineErr := fmt.Fprintln(out)
	if err != nil {
		return fmt.Errorf("read %s API key: %w", provider, err)
	}
	key := strings.TrimSpace(string(secret))
	clear(secret)
	if newlineErr != nil {
		return newlineErr
	}
	if key == "" {
		return errors.New("API key cannot be empty")
	}
	return login([]string{string(provider), "--api-key", key}, out)
}
