package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/workapp"
)

// currentWorkModelStatus resolves only display-safe readiness metadata. It
// never returns credential values or reads credential stores owned by another
// application.
func currentWorkModelStatus(settingsStore config.Store, stateDir string) (workapp.ModelStatus, error) {
	settings, err := settingsStore.Load()
	if err != nil {
		return workapp.ModelStatus{}, err
	}
	status := workapp.ModelStatus{
		Provider: strings.TrimSpace(settings.Defaults.Provider),
		Model:    strings.TrimSpace(settings.Defaults.Model),
	}
	if status.Provider == "" {
		status.Access = "not configured"
		return status, nil
	}

	if custom, found := configuredCustomProvider(settings, status.Provider); found {
		if status.Model == "" {
			status.Model = custom.DefaultModel
		}
		status.Access = customProviderAccess(custom, status.Model)
		status.Processing = workModelProcessing(status.Provider, custom.BaseURL)
		if !customProviderSupportsModel(custom, status.Model) {
			status.Issue = "The selected model is no longer configured for this provider. Choose a model in Models."
		} else if custom.APIKeyEnv != "" && strings.TrimSpace(os.Getenv(custom.APIKeyEnv)) == "" {
			status.Issue = "This model requires " + custom.APIKeyEnv + " in Gator's environment. Set it and reconnect this chat, or choose another model."
		}
		if custom.ID == localmodel.ProviderID {
			if _, err := localmodel.NewClientFromChatCompletionsURL(custom.BaseURL); err != nil {
				status.Issue = "The managed local model requires a local Ollama endpoint. Repair it in Models."
			}
		}
		return status, nil
	}
	provider, err := model.ParseProvider(status.Provider)
	if err != nil {
		return status, err
	}
	status.Model = model.EffectiveModel(provider, status.Model)
	status.Processing = workModelProcessing(status.Provider, settings.ProviderEndpoint(status.Provider))
	credentials, err := auth.New(stateDir)
	if err != nil {
		return status, err
	}
	credential, found, err := credentials.Read(gatorCredentialStoreKey(provider))
	if err != nil {
		return status, err
	}
	configured := func() (workapp.ModelStatus, error) {
		if status.Issue == "" {
			// Construct the same adapter configuration as execution, without
			// extensions or inference/network calls. Credentials alone do not
			// satisfy provider-specific endpoint/deployment requirements.
			if _, err := model.New(model.Config{Provider: provider, Model: status.Model, BaseURL: settings.ProviderEndpoint(status.Provider), Credentials: &credentials, ProviderOptions: settings.OptionsForProvider(status.Provider)}); err != nil {
				status.Issue = "Model configuration is incomplete. Open Models to inspect its endpoint, credentials and provider options."
			}
		}
		return status, nil
	}
	if found {
		kind := strings.ToLower(credentialKindLabel(credential))
		if credential.Expired(time.Now()) {
			status.Access = "stored " + kind + " expired"
			status.Issue = "The selected model's stored credential has expired. Reconnect it in Models."
		} else if credential.IsAPIKey() {
			status.Access = "authenticated with Gator " + kind
		} else {
			status.Access = "stored non-API credential is not supported; configure an API key"
			status.Issue = "The selected model requires an API key. Configure it in Models."
		}
		return configured()
	}
	if sources := remainingCredentialSources(provider); len(sources) > 0 {
		status.Access = "authenticated via " + strings.Join(sources, ", ")
		return configured()
	}
	status.Access = "not authenticated; requires " + model.CredentialHint(provider)
	status.Issue = "The selected model requires " + model.CredentialHint(provider) + ". Connect it in Models or choose another model."
	return status, nil
}

func workModelProcessing(provider, endpoint string) workapp.ModelProcessing {
	processing := workapp.ModelProcessing{Kind: "remote", Destination: provider, Description: "Selected task context leaves this computer for the model service. Hosted providers may charge under their API plan. Connected tools contact their own services."}
	if strings.TrimSpace(endpoint) == "" {
		return processing
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return workapp.ModelProcessing{Kind: "unknown", Description: "The processing destination could not be read. Inspect this provider in Models before sending."}
	}
	// Only a display-safe hostname/port is projected, never URL credentials,
	// paths, queries, provider options or API keys. Do not resolve DNS here.
	processing.Destination = parsed.Hostname()
	if parsed.Port() != "" {
		processing.Destination = net.JoinHostPort(parsed.Hostname(), parsed.Port())
	}
	ip := net.ParseIP(parsed.Hostname())
	if strings.EqualFold(parsed.Hostname(), "localhost") || ip != nil && ip.IsLoopback() {
		processing.Kind = "local_endpoint"
		processing.Description = "Requests go to an endpoint on this computer. A custom endpoint may forward them elsewhere; inspect its configuration. Connected tools can still contact their services."
		if provider == localmodel.ProviderID {
			processing.Kind = "local"
			processing.Description = "The Ollama chat model processes requests on this computer. Downloads and connected tools can still contact their services."
		}
	}
	return processing
}

func customProviderAccess(provider config.CustomProvider, selectedModel string) string {
	if provider.ID == localmodel.ProviderID {
		if selectedModel != "" && customProviderSupportsModel(provider, selectedModel) {
			return "local model configured"
		}
		return "local model selection is incomplete"
	}
	if provider.APIKeyEnv == "" {
		return "custom endpoint configured (keyless)"
	}
	if strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) != "" {
		return "authenticated via " + provider.APIKeyEnv
	}
	return fmt.Sprintf("custom endpoint configured; %s is not set", provider.APIKeyEnv)
}
