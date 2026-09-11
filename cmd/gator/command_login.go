package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/modelcatalog"
)

func login(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("usage: gator login PROVIDER [--prompt | --api-key KEY | --from-env NAME | --bearer-token TOKEN | --bearer-token-from-env NAME]")
	}
	providerName := arguments[0]
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	apiKey := flags.String("api-key", "", "API key to store (visible to the current process)")
	bearerToken := flags.String("bearer-token", "", "bearer token to store (visible to the current process)")
	bearerTokenFromEnvironment := flags.String("bearer-token-from-env", "", "environment variable containing a bearer token")
	fromEnvironment := flags.String("from-env", "", "environment variable containing the API key")
	prompt := flags.Bool("prompt", false, "read an API key from a hidden terminal prompt")
	subscription := flags.Bool("subscription", false, "use the provider subscription OAuth flow")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("usage: gator login PROVIDER [--prompt | --api-key KEY | --from-env NAME | --bearer-token TOKEN | --bearer-token-from-env NAME]")
	}
	credentialModes := countNonEmpty(*apiKey, *bearerToken, *bearerTokenFromEnvironment, *fromEnvironment)
	if *prompt {
		credentialModes++
	}
	if credentialModes > 1 {
		return errors.New("use only one of --prompt, --api-key, --from-env, --bearer-token, or --bearer-token-from-env")
	}
	provider, err := model.ParseProvider(providerName)
	if err != nil {
		return err
	}
	if provider == model.Claude {
		return errors.New("Claude.ai subscription OAuth is not a supported Gator login. Use 'gator connect claude' to store an Anthropic API key, then run with --provider anthropic, or use 'gator delegate claude run ...'")
	}
	if !model.SupportsDirect(provider) {
		return fmt.Errorf("provider %q has no direct Gator API integration", provider)
	}
	if model.RequiresOAuthLogin(provider) || *subscription {
		if credentialModes != 0 {
			return fmt.Errorf("provider %q uses subscription OAuth; do not pass an API key, bearer token, environment source, or --prompt", provider)
		}
		if !model.SupportsOAuthLogin(provider) {
			return fmt.Errorf("provider %q has no supported Gator OAuth flow", provider)
		}
		return loginOAuth(provider, out)
	}
	if !model.SupportsAPIKeyLogin(provider) {
		return fmt.Errorf("provider %q uses %s; gator login does not store that credential", provider, model.CredentialHint(provider))
	}
	if token, source := bearerTokenValue(*bearerToken, *bearerTokenFromEnvironment); token != "" {
		if provider != model.AzureOpenAIResponses {
			return fmt.Errorf("provider %q does not support a Gator-managed bearer token", provider)
		}
		credentials, err := gatorCredentials()
		if err != nil {
			return err
		}
		if err := credentials.Put(string(provider), auth.Credential{Type: "bearer_token", Access: token}); err != nil {
			return fmt.Errorf("store Gator credential: %w", err)
		}
		_, err = fmt.Fprintf(out, "Stored a Gator bearer token for %s from %s. It is not refreshed; replace it before it expires.\n", provider, source)
		return err
	}
	key := strings.TrimSpace(*apiKey)
	source := "--api-key"
	if *prompt {
		if !term.IsTerminal(os.Stdin.Fd()) {
			return errors.New("--prompt requires an interactive terminal")
		}
		if _, err := fmt.Fprintf(out, "Enter the %s API key (input hidden): ", provider); err != nil {
			return err
		}
		secret, err := term.ReadPassword(os.Stdin.Fd())
		_, newlineErr := fmt.Fprintln(out)
		if err != nil {
			return fmt.Errorf("read %s API key: %w", provider, err)
		}
		key = strings.TrimSpace(string(secret))
		clear(secret)
		if newlineErr != nil {
			return newlineErr
		}
		if key == "" {
			return errors.New("API key cannot be empty")
		}
		source = "interactive prompt"
	} else if environment := strings.TrimSpace(*fromEnvironment); environment != "" {
		key = strings.TrimSpace(os.Getenv(environment))
		source = environment
	} else if key == "" {
		environment := model.APIKeyEnvironment(provider)
		key = strings.TrimSpace(os.Getenv(environment))
		source = environment
	}
	if key == "" {
		return fmt.Errorf("no API key found; set %s or pass --from-env NAME", model.APIKeyEnvironment(provider))
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	if err := credentials.Put(string(provider), auth.Credential{Type: "api_key", Key: key}); err != nil {
		return fmt.Errorf("store Gator credential: %w", err)
	}
	_, err = fmt.Fprintf(out, "Stored a Gator credential for %s from %s.\n", provider, source)
	return err
}

func countNonEmpty(values ...string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func bearerTokenValue(token, environment string) (string, string) {
	if token = strings.TrimSpace(token); token != "" {
		return token, "--bearer-token"
	}
	environment = strings.TrimSpace(environment)
	if environment == "" {
		return "", ""
	}
	return strings.TrimSpace(os.Getenv(environment)), environment
}

func loginOAuth(provider model.Provider, out io.Writer) error {
	if provider == model.Copilot {
		return loginCopilot(out)
	}
	if provider == model.XAI {
		return loginXAI(out)
	}
	if provider == model.OpenRouter {
		return loginOpenRouter(out)
	}
	if provider == model.KimiCoding {
		return loginKimiCoding(out)
	}
	if provider == model.Radius {
		return loginRadius(out)
	}
	flow, err := oauthFlow(provider)
	if err != nil {
		return err
	}
	attempt, err := auth.BeginBrowserFlow(flow)
	if err != nil {
		return fmt.Errorf("start %s OAuth login: %w", provider, err)
	}
	callback, err := attempt.StartCallback()
	if err != nil {
		return fmt.Errorf("start %s OAuth callback: %w", provider, err)
	}
	defer callback.Close()
	if _, err := fmt.Fprintf(out, "Open this URL to authenticate Gator with %s:\n%s\n\nWaiting for the local callback...\n", provider, attempt.AuthorizationURL()); err != nil {
		return err
	}
	context, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	code, err := callback.Wait(context)
	if err != nil {
		return fmt.Errorf("complete %s OAuth login: %w", provider, err)
	}
	credential, err := attempt.Exchange(context, code)
	if err != nil {
		return fmt.Errorf("exchange %s OAuth credential: %w", provider, err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	if err := credentials.Put(string(provider), credential); err != nil {
		return fmt.Errorf("store %s OAuth credential: %w", provider, err)
	}
	_, err = fmt.Fprintf(out, "Stored a Gator OAuth credential for %s.\n", provider)
	return err
}

func loginOpenRouter(out io.Writer) error {
	login, err := beginOpenRouterLogin()
	if err != nil {
		return fmt.Errorf("start OpenRouter OAuth login: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Open this URL to authenticate Gator with OpenRouter:\n%s\n\nWaiting for the local callback...\n", login.URL()); err != nil {
		return err
	}
	context, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := login.Complete(context); err != nil {
		return fmt.Errorf("complete OpenRouter OAuth login: %w", err)
	}
	_, err = fmt.Fprintln(out, "Stored a user-controlled Gator API key for openrouter.")
	return err
}

func loginXAI(out io.Writer) error {
	login, err := beginXAIDeviceLogin()
	if err != nil {
		return fmt.Errorf("start xAI device login: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Open this URL and enter the displayed code to authenticate Gator with xAI:\n%s\n\nWaiting for device authorization...\n", login.URL()); err != nil {
		return err
	}
	context, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := login.Complete(context); err != nil {
		return fmt.Errorf("complete xAI device login: %w", err)
	}
	_, err = fmt.Fprintln(out, "Stored a Gator OAuth credential for xai.")
	return err
}

func loginKimiCoding(out io.Writer) error {
	login, err := beginKimiCodingDeviceLogin()
	if err != nil {
		return fmt.Errorf("start Kimi Code device login: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Open this URL and enter the displayed code to authenticate Gator with Kimi Code:\n%s\n\nWaiting for device authorization...\n", login.URL()); err != nil {
		return err
	}
	context, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := login.Complete(context); err != nil {
		return fmt.Errorf("complete Kimi Code device login: %w", err)
	}
	_, err = fmt.Fprintln(out, "Stored a Gator OAuth credential for kimi-coding.")
	return err
}

func loginRadius(out io.Writer) error {
	login, err := beginRadiusDeviceLogin()
	if err != nil {
		return fmt.Errorf("start Radius device login: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Open this URL and enter the displayed code to authenticate Gator with Radius:\n%s\n\nWaiting for device authorization...\n", login.URL()); err != nil {
		return err
	}
	context, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := login.Complete(context); err != nil {
		return fmt.Errorf("complete Radius device login: %w", err)
	}
	_, err = fmt.Fprintln(out, "Stored a Gator OAuth credential for radius.")
	return err
}

func loginCopilot(out io.Writer) error {
	login, err := beginCopilotDeviceLogin()
	if err != nil {
		return fmt.Errorf("start Copilot device login: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Open this URL and enter the displayed code to authenticate Gator with Copilot:\n%s\n\nWaiting for device authorization...\n", login.URL()); err != nil {
		return err
	}
	context, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := login.Complete(context); err != nil {
		return fmt.Errorf("complete Copilot device login: %w", err)
	}
	_, err = fmt.Fprintln(out, "Stored a Gator OAuth credential for copilot.")
	return err
}

func logout(arguments []string, out io.Writer) error {
	if len(arguments) != 1 {
		return errors.New("usage: gator logout PROVIDER")
	}
	provider, err := model.ParseProvider(arguments[0])
	if err != nil {
		return err
	}
	if !model.SupportsDirect(provider) {
		return fmt.Errorf("provider %q has no Gator-managed cloud credential", provider)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	storeKey := gatorCredentialStoreKey(provider)
	existing, found, err := credentials.Read(storeKey)
	if err != nil {
		return err
	}
	if found {
		if err := credentials.Delete(storeKey); err != nil {
			return fmt.Errorf("remove Gator credential: %w", err)
		}
	}
	result := modelcatalog.CredentialRemovalResult{
		Provider:         string(provider),
		StoreKey:         storeKey,
		Removed:          found,
		RemainingSources: remainingCredentialSources(provider),
	}
	if found {
		result.Kind = credentialKindLabel(existing)
	}
	_, err = fmt.Fprintln(out, describeCredentialRemoval(result))
	return err
}
