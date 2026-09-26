package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/modelcatalog"
)

func login(arguments []string, out io.Writer) error {
	const usage = "usage: gator provider login PROVIDER [--prompt | --api-key KEY | --from-env NAME]"
	if len(arguments) == 0 {
		return errors.New(usage)
	}
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	apiKey := flags.String("api-key", "", "API key to store (visible to the current process)")
	fromEnvironment := flags.String("from-env", "", "environment variable containing the API key")
	prompt := flags.Bool("prompt", false, "read an API key from a hidden terminal prompt")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New(usage)
	}
	modes := countNonEmpty(*apiKey, *fromEnvironment)
	if *prompt {
		modes++
	}
	if modes > 1 {
		return errors.New("use only one of --prompt, --api-key, or --from-env")
	}
	provider, err := model.ParseProvider(arguments[0])
	if err != nil {
		return err
	}
	if !model.SupportsAPIKeyLogin(provider) {
		return fmt.Errorf("provider %q has no API-key setup", provider)
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

func logout(arguments []string, out io.Writer) error {
	if len(arguments) != 1 {
		return errors.New("usage: gator provider logout PROVIDER")
	}
	provider, err := model.ParseProvider(arguments[0])
	if err != nil {
		return err
	}
	if !model.SupportsAPIKeyLogin(provider) {
		return fmt.Errorf("provider %q has no Gator-managed API key", provider)
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
