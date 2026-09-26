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
  gator provider PROVIDER

Provider onboarding stores a direct provider API key. Use gator provider list
to inspect custom OpenAI-compatible endpoints, or /model in the TUI to choose
Cloud API key or Local model.`

// onboardProvider is the provider-first API-key setup path. Gator does not
// authenticate through subscription or external coding-agent harnesses.
func onboardProvider(arguments []string, out io.Writer) error {
	if len(arguments) != 1 {
		return errors.New(providerOnboardingUsage)
	}
	provider, err := model.ParseProvider(strings.ToLower(strings.TrimSpace(arguments[0])))
	if err != nil || !model.SupportsAPIKeyLogin(provider) {
		return fmt.Errorf("unknown API-key provider %q\n\n%s", arguments[0], providerOnboardingUsage)
	}
	return onboardAPIKeyProvider(string(provider), out)
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
