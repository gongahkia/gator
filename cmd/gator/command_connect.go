package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

const connectUsage = `Usage:
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
		if err := login([]string{"anthropic"}, out); err != nil {
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
		if err := login([]string{"radius"}, out); err != nil {
			return fmt.Errorf("connect Radius: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown connection target %q\n\n%s", target, connectUsage)
	}
}
