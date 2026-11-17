// Package providerhttp retains bounded HTTP failure metadata without provider prose.
package providerhttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gongahkia/gator/internal/agent"
)

var opaqueID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var keyPattern = regexp.MustCompile(`sk-[A-Za-z0-9_-]+`)

// NewError intentionally ignores message text: providers may echo keys or input.
// Retry classification stays with the adapter's existing HTTPStatusError wrapper.
func NewError(provider, label, model, source string, request *http.Request, response *http.Response, body []byte, started time.Time, secrets ...string) error {
	if response.Request != nil {
		request = response.Request
	}
	for _, name := range []string{"Authorization", "x-goog-api-key", "api-key"} {
		if value := request.Header.Get(name); value != "" {
			secrets = append(secrets, value)
			if strings.HasPrefix(strings.ToLower(value), "bearer ") {
				secrets = append(secrets, value[7:])
			}
		}
	}
	clean := func(value string) string {
		for _, secret := range secrets {
			if secret != "" {
				value = strings.ReplaceAll(value, secret, "[redacted]")
			}
		}
		value = keyPattern.ReplaceAllString(value, "[redacted]")
		value = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, value)
		runes := []rune(value)
		if len(runes) > 256 {
			value = string(runes[:256])
		}
		return value
	}
	endpoint := *request.URL
	endpoint.User, endpoint.RawQuery, endpoint.Fragment, endpoint.RawFragment = nil, "", "", ""
	endpoint.ForceQuery = false
	diagnostic := agent.ProviderDiagnostic{
		Provider: clean(provider), Model: clean(model), Endpoint: clean(endpoint.String()),
		Method: request.Method, Streaming: request.Header.Get("Accept") == "text/event-stream",
		Status: response.StatusCode, ElapsedMS: max(0, time.Since(started).Milliseconds()), Auth: "none",
	}
	switch source {
	case "explicit", "gator-store", "environment":
		diagnostic.CredentialSource = source
	default:
		diagnostic.CredentialSource = "unspecified"
	}
	if request.Header.Get("Authorization") != "" {
		diagnostic.Auth = "authorization-header"
		if strings.HasPrefix(strings.ToLower(request.Header.Get("Authorization")), "bearer ") {
			diagnostic.Auth = "bearer"
		}
	} else if request.Header.Get("x-goog-api-key") != "" || request.Header.Get("api-key") != "" {
		diagnostic.Auth = "api-key-header"
	}
	for _, name := range []string{"x-request-id", "request-id", "x-goog-request-id"} {
		id := response.Header.Get(name)
		if opaqueID.MatchString(id) && clean(id) == id {
			diagnostic.RequestID = id
			break
		}
	}
	var payload struct {
		Error struct {
			Code    json.RawMessage `json:"code"`
			Type    string          `json:"type"`
			Status  string          `json:"status"`
			Param   string          `json:"param"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		var code string
		_ = json.Unmarshal(payload.Error.Code, &code)
		diagnostic.Code, diagnostic.Type = knownCode(clean(code)), knownCode(clean(payload.Error.Type))
		if diagnostic.Code == "" {
			diagnostic.Code = knownCode(clean(payload.Error.Status))
		}
		for _, detail := range payload.Error.Details {
			if reason := knownCode(clean(detail.Reason)); reason != "" {
				diagnostic.Code = reason
				break
			}
		}
		switch payload.Error.Param {
		case "model", "input", "messages", "tools", "stream", "max_output_tokens", "max_tokens", "temperature", "reasoning":
			diagnostic.Parameter = payload.Error.Param
		}
	}
	summary := "request rejected"
	switch response.StatusCode {
	case 401:
		summary = "authentication rejected"
	case 403:
		summary = "permission denied"
	case 404:
		summary = "endpoint or model not found"
	case 429:
		summary = "rate limit reached"
	case 500, 502, 503, 504:
		summary = "provider unavailable"
	}
	return agent.ProviderRequestError{Diagnostic: diagnostic, Summary: fmt.Sprintf("%s returned HTTP %d: %s", label, response.StatusCode, summary)}
}

// Unknown strings are omitted rather than assuming an arbitrary error field is safe.
func knownCode(value string) string {
	switch value {
	case "invalid_api_key", "invalid_request_error", "authentication_error", "permission_error", "rate_limit_error", "rate_limit_exceeded", "insufficient_quota", "model_not_found", "unsupported_parameter", "invalid_parameter", "context_length_exceeded", "server_error", "api_error", "overloaded_error", "billing_hard_limit_reached",
		"UNAUTHENTICATED", "PERMISSION_DENIED", "RESOURCE_EXHAUSTED", "INVALID_ARGUMENT", "NOT_FOUND", "UNAVAILABLE", "INTERNAL", "API_KEY_INVALID", "API_KEY_EXPIRED", "API_KEY_SERVICE_BLOCKED", "API_KEY_HTTP_REFERRER_BLOCKED", "API_KEY_IP_ADDRESS_BLOCKED", "ACCESS_TOKEN_EXPIRED", "CREDENTIALS_MISSING", "BILLING_DISABLED", "SERVICE_DISABLED":
		return value
	}
	return ""
}
