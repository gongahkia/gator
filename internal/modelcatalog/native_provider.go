package modelcatalog

import (
	"errors"
	"strings"

	"github.com/gongahkia/gator/internal/model"
)

// NativeProviderAction is finite host input. APIKey is save-only transient data,
// never a display model, request journal, command argument or completion field.
type NativeProviderAction struct {
	RequestID  string               `json:"request_id"`
	Action     string               `json:"action"`
	Provider   string               `json:"provider,omitempty"`
	ReviewID   string               `json:"review_id,omitempty"`
	Input      *NativeProviderInput `json:"input,omitempty"`
	ReplaceKey bool                 `json:"replace_key,omitempty"`
	APIKey     string               `json:"api_key,omitempty"`
}
type NativeProviderInput struct {
	Model        string            `json:"model"`
	EndpointMode string            `json:"endpoint_mode"`
	Endpoint     string            `json:"endpoint"`
	APIVersion   string            `json:"api_version"`
	Options      map[string]string `json:"options,omitempty"`
}
type NativeProviderRow struct {
	IsDefault   bool              `json:"is_default"`
	ID          string            `json:"id"`
	Model       string            `json:"model"`
	Stored      bool              `json:"stored"`
	Ambient     bool              `json:"ambient"`
	Expired     bool              `json:"expired"`
	Kind        string            `json:"kind"`
	Configured  bool              `json:"configured"`
	Ready       bool              `json:"ready"`
	Issue       string            `json:"issue"`
	Destination string            `json:"destination"`
	Fields      []string          `json:"fields"`
	Options     map[string]string `json:"options,omitempty"`
	APIVersion  string            `json:"api_version"`
}
type NativeProviderReview struct {
	Options          map[string]string `json:"options,omitempty"`
	ID               string            `json:"id"`
	Provider         string            `json:"provider"`
	Operation        string            `json:"operation"`
	Model            string            `json:"model"`
	Destination      string            `json:"destination"`
	EndpointMode     string            `json:"endpoint_mode"`
	CredentialAction string            `json:"credential_action"`
	SetsDefaults     bool              `json:"sets_defaults"`
}
type NativeProviderResult struct {
	RequestID string                `json:"request_id"`
	Action    string                `json:"action"`
	Provider  string                `json:"provider,omitempty"`
	Rows      []NativeProviderRow   `json:"rows,omitempty"`
	Review    *NativeProviderReview `json:"review,omitempty"`
	Confirmed bool                  `json:"confirmed"`
	Changed   bool                  `json:"changed"`
	Conflict  bool                  `json:"conflict"`
	Error     string                `json:"error,omitempty"`
	Notice    string                `json:"notice,omitempty"`
}

func NativeCloudFields(provider model.Provider) []string {
	switch provider {
	case model.AzureOpenAI, model.AzureOpenAIResponses:
		return []string{"api_version"}
	case model.CloudflareWorkers:
		return []string{"account_id"}
	case model.CloudflareGateway:
		return []string{"account_id", "gateway_id", "gateway_protocol"}
	case model.Radius:
		return []string{"gateway"}
	default:
		return []string{}
	}
}

// NormalizeNativeCloudSetup reuses the terminal's canonical form validation,
// including Azure URL expansion and provider-specific field rules.
func NormalizeNativeCloudSetup(setup CloudModelSetup, version string) (CloudModelSetup, error) {
	provider, err := model.ParseProvider(setup.Provider)
	if err != nil || !model.SupportsAPIKeyLogin(provider) {
		return CloudModelSetup{}, errors.New("unsupported API-key provider")
	}
	allowed := NativeCloudFields(provider)
	for name := range setup.Options {
		found := false
		for _, field := range allowed {
			if name == field && field != "api_version" {
				found = true
			}
		}
		if !found {
			return CloudModelSetup{}, errors.New("unsupported provider option")
		}
	}
	if len(setup.Model) > 512 || strings.ContainsAny(setup.Model, "\x00\r\n") || len(setup.BaseURL) > 2048 || len(version) > 128 {
		return CloudModelSetup{}, errors.New("invalid configuration size")
	}
	form := newCloudModelSetupForm(provider, setup.Model, setup.BaseURL, setup.Options, 80)
	form.apiVersion.SetValue(version)
	return form.configuration()
}
