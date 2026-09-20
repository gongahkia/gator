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

const providerOnboardingUsage = `usage:
  gator provider openai
  gator provider anthropic
  gator provider gemini
  gator provider codex [--device]
  gator provider copilot [--host URL]
  gator provider kimi
  gator provider xai
  gator provider claude
  gator provider openrouter
  gator provider radius

Provider onboarding starts the closest supported sign-in route. It never stores or
translates a vendor CLI credential into Gator. Claude, Radius, and native xAI
runs use API credentials held by Gator; their provider account subscription
flows remain separate.`

// onboardProvider is the provider-first onboarding path. Native Gator API
// credentials and product-owned OAuth live under `gator provider login`, while
// this path uses an installed vendor CLI wherever that is the public,
// no-client-registration route.
func onboardProvider(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(providerOnboardingUsage)
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
			return errors.New("usage: gator provider xai")
		}
		if _, err := fmt.Fprintln(out, "Starting OpenCode's xAI provider login. Choose the browser or headless Grok subscription method, or an API key. Gator does not store or translate the credential."); err != nil {
			return err
		}
		return delegate([]string{"opencode", "login", "--provider", "xai"}, out)
	case "claude", "anthropic":
		if len(remaining) != 0 {
			return errors.New("usage: gator provider claude")
		}
		if _, err := fmt.Fprintln(out, "Claude Code delegation uses an Anthropic API key, not Claude.ai subscription OAuth."); err != nil {
			return err
		}
		if err := onboardAPIKeyProvider("anthropic", out); err != nil {
			return fmt.Errorf("onboard Claude: %w", err)
		}
		return nil
	case "openrouter":
		if len(remaining) != 0 {
			return errors.New("usage: gator provider openrouter")
		}
		return login([]string{"openrouter", "--subscription"}, out)
	case "radius":
		if len(remaining) != 0 {
			return errors.New("usage: gator provider radius")
		}
		if _, err := fmt.Fprintln(out, "Radius account OAuth requires a Gator-registered client. Connecting with a Radius API key instead."); err != nil {
			return err
		}
		if err := onboardAPIKeyProvider("radius", out); err != nil {
			return fmt.Errorf("onboard Radius: %w", err)
		}
		return nil
	default:
		provider, err := model.ParseProvider(target)
		if err == nil && model.SupportsAPIKeyLogin(provider) && model.APIKeyEnvironment(provider) != "" {
			if len(remaining) != 0 {
				return fmt.Errorf("usage: gator provider %s", target)
			}
			return onboardAPIKeyProvider(string(provider), out)
		}
		return fmt.Errorf("unknown provider %q\n\n%s", target, providerOnboardingUsage)
	}
}

func onboardAPIKeyProvider(providerName string, out io.Writer) error {
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
		return fmt.Errorf("no API key found; set %s or run 'gator provider %s' in an interactive terminal", environment, provider)
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
