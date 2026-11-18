package modelcatalog

import (
	"strings"
	"testing"
)

func TestNativeCloudNormalizationUsesExistingProviderRules(t *testing.T) {
	for _, tt := range []struct {
		provider, model, endpoint, version string
		options                            map[string]string
		valid                              bool
	}{
		{"openai", "fixture", "https://example.invalid/v1", "", nil, true},
		{"azure-openai", "deployment", "https://resource.openai.azure.com", "2025-04-01-preview", nil, true},
		{"cloudflare-ai-gateway", "model", "", "", map[string]string{"account_id": "account", "gateway_id": "gateway", "gateway_protocol": "openai-responses"}, true},
		{"cloudflare-ai-gateway", "model", "", "", map[string]string{"gateway_protocol": "invented"}, false},
		{"openai", "model", "", "", map[string]string{"unsupported": "value"}, false},
		{"pi", "model", "", "", nil, false},
		{"azure-openai", "bad deployment", "https://resource.openai.azure.com", "v1", nil, false},
	} {
		t.Run(tt.provider+tt.model, func(t *testing.T) {
			r, err := NormalizeNativeCloudSetup(CloudModelSetup{Provider: tt.provider, Model: tt.model, BaseURL: tt.endpoint, Options: tt.options}, tt.version)
			if (err == nil) != tt.valid {
				t.Fatal("unexpected normalization outcome", err)
			}
			if err == nil && tt.provider == "azure-openai" && !strings.Contains(r.BaseURL, "api-version=2025-04-01-preview") {
				t.Fatal("Azure normalization missing")
			}
		})
	}
}
