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

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model"
)

func login(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	apiKey := flags.String("api-key", "", "API key to store (visible to the current process)")
	fromEnvironment := flags.String("from-env", "", "environment variable containing the API key")
	subscription := flags.Bool("subscription", false, "use the provider subscription OAuth flow")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 1 {
		return errors.New("usage: gator login PROVIDER [--api-key KEY | --from-env NAME]")
	}
	if strings.TrimSpace(*apiKey) != "" && strings.TrimSpace(*fromEnvironment) != "" {
		return errors.New("use either --api-key or --from-env, not both")
	}
	provider, err := model.ParseProvider(flags.Arg(0))
	if err != nil {
		return err
	}
	if !model.SupportsDirect(provider) {
		return fmt.Errorf("provider %q has no direct Gator API integration", provider)
	}
	if model.RequiresOAuthLogin(provider) || *subscription {
		if strings.TrimSpace(*apiKey) != "" || strings.TrimSpace(*fromEnvironment) != "" {
			return fmt.Errorf("provider %q uses subscription OAuth; do not pass an API key", provider)
		}
		if !model.SupportsOAuthLogin(provider) {
			return fmt.Errorf("provider %q has no supported Gator OAuth flow", provider)
		}
		return loginOAuth(provider, out)
	}
	if !model.SupportsAPIKeyLogin(provider) {
		return fmt.Errorf("provider %q does not support Gator API-key login", provider)
	}
	key := strings.TrimSpace(*apiKey)
	source := "--api-key"
	if environment := strings.TrimSpace(*fromEnvironment); environment != "" {
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
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	if err := credentials.Delete(string(provider)); err != nil {
		return fmt.Errorf("remove Gator credential: %w", err)
	}
	_, err = fmt.Fprintf(out, "Removed the Gator credential for %s.\n", provider)
	return err
}
