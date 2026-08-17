package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
)

const providerConfigUsage = `usage:
  gator provider list
  gator provider add ID --base-url URL --model MODEL [--model MODEL...] [--api-key-env NAME]
  gator provider remove ID --yes`

// providerCommand persists custom/local Chat Completions endpoints. It is
// deliberately separate from provider credentials: keys remain environment
// variables and are never written to config.json.
func providerCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(providerConfigUsage)
	}
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	settings, err := store.Load()
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "list":
		if len(arguments) != 1 {
			return errors.New(providerConfigUsage)
		}
		if len(settings.CustomProviders) == 0 {
			_, err := fmt.Fprintln(out, "No custom providers configured.")
			return err
		}
		for _, provider := range settings.CustomProviders {
			credential := "no API key"
			if provider.APIKeyEnv != "" {
				credential = "API key from " + provider.APIKeyEnv
			}
			if _, err := fmt.Fprintf(out, "%s  %s  default:%s  models:%s  %s\n", provider.ID, provider.BaseURL, provider.DefaultModel, strings.Join(provider.Models, ","), credential); err != nil {
				return err
			}
		}
		return nil
	case "add":
		return addCustomProvider(arguments[1:], store, settings, out)
	case "remove":
		if len(arguments) != 3 || arguments[2] != "--yes" {
			return errors.New("removing a provider removes its endpoint configuration; use: gator provider remove ID --yes")
		}
		if !hasCustomProvider(settings.CustomProviders, arguments[1]) {
			return fmt.Errorf("custom provider %q is not configured", arguments[1])
		}
		settings.CustomProviders = removeCustomProvider(settings.CustomProviders, arguments[1])
		if err := store.Save(settings); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Removed custom provider %q.\n", arguments[1])
		return err
	default:
		return fmt.Errorf("unknown provider command %q\n%s", arguments[0], providerConfigUsage)
	}
}

func addCustomProvider(arguments []string, store config.Store, settings config.Settings, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(providerConfigUsage)
	}
	id := strings.TrimSpace(arguments[0])
	if _, err := model.ParseProvider(id); err == nil {
		return fmt.Errorf("custom provider ID %q conflicts with Gator's built-in provider; choose another ID", id)
	}
	flags := flag.NewFlagSet("provider add", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	baseURL := flags.String("base-url", "", "full OpenAI Chat Completions endpoint")
	apiKeyEnv := flags.String("api-key-env", "", "environment variable containing the API key")
	var models stringList
	flags.Var(&models, "model", "allowed model ID; repeat to add more")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if len(flags.Args()) != 0 || len(models) == 0 || strings.TrimSpace(*baseURL) == "" {
		return errors.New(providerConfigUsage)
	}
	provider := config.CustomProvider{ID: id, BaseURL: strings.TrimSpace(*baseURL), APIKeyEnv: strings.TrimSpace(*apiKeyEnv), Models: models, DefaultModel: models[0]}
	settings.CustomProviders = setCustomProvider(settings.CustomProviders, provider)
	if err := store.Save(settings); err != nil {
		return err
	}
	credential := "without an API key"
	if provider.APIKeyEnv != "" {
		credential = "using " + provider.APIKeyEnv
	}
	_, err := fmt.Fprintf(out, "Saved custom provider %q with default model %q %s. Use --provider %s or set it as Gator's default provider.\n", provider.ID, provider.DefaultModel, credential, provider.ID)
	return err
}

type stringList []string

func (v *stringList) String() string { return strings.Join(*v, ",") }

func (v *stringList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return errors.New("model ID is required and cannot contain a newline")
	}
	*v = append(*v, value)
	return nil
}

func hasCustomProvider(providers []config.CustomProvider, id string) bool {
	for _, provider := range providers {
		if provider.ID == id {
			return true
		}
	}
	return false
}

func setCustomProvider(providers []config.CustomProvider, candidate config.CustomProvider) []config.CustomProvider {
	for index := range providers {
		if providers[index].ID == candidate.ID {
			providers[index] = candidate
			return providers
		}
	}
	providers = append(providers, candidate)
	for index := 1; index < len(providers); index++ {
		for previous := index; previous > 0 && providers[previous].ID < providers[previous-1].ID; previous-- {
			providers[previous], providers[previous-1] = providers[previous-1], providers[previous]
		}
	}
	return providers
}

func removeCustomProvider(providers []config.CustomProvider, id string) []config.CustomProvider {
	result := providers[:0]
	for _, provider := range providers {
		if provider.ID != id {
			result = append(result, provider)
		}
	}
	return result
}
