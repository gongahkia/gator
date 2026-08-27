package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/model"
)

const providerConfigUsage = `usage:
  gator provider list
  gator provider add ID --base-url URL --model MODEL [--model MODEL...] [--api-key-env NAME]
  gator provider discover ID [--apply]
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
	case "discover":
		return discoverCustomProvider(arguments[1:], store, settings, out)
	case "remove":
		if len(arguments) != 3 || arguments[2] != "--yes" {
			return errors.New("removing a provider removes its endpoint configuration; use: gator provider remove ID --yes")
		}
		if !hasCustomProvider(settings.CustomProviders, arguments[1]) {
			return fmt.Errorf("custom provider %q is not configured", arguments[1])
		}
		settings.CustomProviders = removeCustomProvider(settings.CustomProviders, arguments[1])
		clearDeletedCustomProviderReferences(&settings, arguments[1])
		if err := store.Save(settings); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Removed custom provider %q. Its API key environment variable was not changed.\n", arguments[1])
		return err
	default:
		return fmt.Errorf("unknown provider command %q\n%s", arguments[0], providerConfigUsage)
	}
}

func discoverCustomProvider(arguments []string, store config.Store, settings config.Settings, out io.Writer) error {
	if len(arguments) != 1 && (len(arguments) != 2 || arguments[1] != "--apply") {
		return errors.New(providerConfigUsage)
	}
	apply := len(arguments) == 2
	provider, found := findCustomProvider(settings.CustomProviders, arguments[0])
	if !found {
		return fmt.Errorf("custom provider %q is not configured", arguments[0])
	}
	models, err := discoverModels(provider)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("custom provider %q returned no model IDs", provider.ID)
	}
	if !apply {
		if _, err := fmt.Fprintf(out, "Discovered models for %s:\n%s\n\nRun 'gator provider discover %s --apply' to replace the configured catalog.\n", provider.ID, strings.Join(models, "\n"), provider.ID); err != nil {
			return err
		}
		return nil
	}
	provider.Models = models
	if !customProviderSupportsModel(provider, provider.DefaultModel) {
		provider.DefaultModel = models[0]
	}
	settings.CustomProviders = setCustomProvider(settings.CustomProviders, provider)
	if err := store.Save(settings); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Replaced the model catalog for %q (%d models; default %q).\n", provider.ID, len(models), provider.DefaultModel)
	return err
}

func discoverModels(provider config.CustomProvider) ([]string, error) {
	endpoint, err := modelsEndpoint(provider.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("discover %q models: %w", provider.ID, err)
	}
	context, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(context, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if provider.APIKeyEnv != "" {
		if key := strings.TrimSpace(os.Getenv(provider.APIKeyEnv)); key != "" {
			request.Header.Set("Authorization", "Bearer "+key)
		}
	}
	response, err := modelCatalogClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("request %q model catalog: %w", provider.ID, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("request %q model catalog: server returned HTTP %d", provider.ID, response.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2*1024*1024))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode %q model catalog: %w", provider.ID, err)
	}
	models := make([]string, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || len(id) > 512 || strings.ContainsAny(id, "\r\n") {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	sort.Strings(models)
	return models, nil
}

func modelCatalogClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			origin := via[0].URL
			if request.URL.Scheme != origin.Scheme || request.URL.Host != origin.Host {
				return fmt.Errorf("model catalog redirected off origin from %s to %s", origin.Host, request.URL.Host)
			}
			if len(via) >= 5 {
				return errors.New("model catalog followed too many redirects")
			}
			return nil
		},
	}
}

func modelsEndpoint(baseURL string) (string, error) {
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	const suffix = "/chat/completions"
	if !strings.HasSuffix(endpoint.Path, suffix) {
		return "", errors.New("base URL must end in /chat/completions to discover models")
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, suffix) + "/models"
	endpoint.RawQuery = ""
	return endpoint.String(), nil
}

func addCustomProvider(arguments []string, store config.Store, settings config.Settings, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(providerConfigUsage)
	}
	id := strings.TrimSpace(arguments[0])
	if err := reservedCustomProviderIDError(id); err != nil {
		return err
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

func reservedCustomProviderIDError(id string) error {
	id = strings.TrimSpace(id)
	if id == localmodel.ProviderID {
		return fmt.Errorf("custom provider ID %q is reserved for 'gator local use'", id)
	}
	if _, err := model.ParseProvider(id); err == nil {
		return fmt.Errorf("custom provider ID %q conflicts with Gator's built-in provider; choose another ID", id)
	}
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

func findCustomProvider(providers []config.CustomProvider, id string) (config.CustomProvider, bool) {
	for _, provider := range providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return config.CustomProvider{}, false
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
