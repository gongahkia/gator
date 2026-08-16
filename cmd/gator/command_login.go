package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model"
)

func login(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	apiKey := flags.String("api-key", "", "API key to store (visible to the current process)")
	fromEnvironment := flags.String("from-env", "", "environment variable containing the API key")
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
	key := strings.TrimSpace(*apiKey)
	source := "--api-key"
	if environment := strings.TrimSpace(*fromEnvironment); environment != "" {
		key = strings.TrimSpace(os.Getenv(environment))
		source = environment
	} else if key == "" {
		environment := model.CredentialHint(provider)
		key = strings.TrimSpace(os.Getenv(environment))
		source = environment
	}
	if key == "" {
		return fmt.Errorf("no API key found; set %s or pass --from-env NAME", model.CredentialHint(provider))
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
