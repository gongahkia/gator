package agent

import (
	"encoding/json"
	"errors"
)

// ProviderDiagnostic is bounded request metadata, never a request or response body.
type ProviderDiagnostic struct {
	Transport        string `json:"transport,omitempty"`
	Phase            string `json:"phase,omitempty"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Endpoint         string `json:"endpoint"`
	Method           string `json:"method"`
	Streaming        bool   `json:"streaming"`
	Status           int    `json:"status,omitempty"`
	ElapsedMS        int64  `json:"elapsed_ms"`
	Auth             string `json:"auth"`
	CredentialSource string `json:"credential_source"`
	RequestID        string `json:"request_id,omitempty"`
	Code             string `json:"code,omitempty"`
	Type             string `json:"type,omitempty"`
	Parameter        string `json:"parameter,omitempty"`
}

// ProviderRequestError keeps structured diagnostics available through error wrappers.
type ProviderRequestError struct {
	Diagnostic ProviderDiagnostic
	Summary    string
	Cause      error
}

func (e ProviderRequestError) Error() string {
	data, _ := json.Marshal(e.Diagnostic)
	return e.Summary + "; provider_request=" + string(data)
}

func ProviderDiagnosticFrom(err error) *ProviderDiagnostic {
	var request ProviderRequestError
	if !errors.As(err, &request) {
		return nil
	}
	diagnostic := request.Diagnostic
	return &diagnostic
}

func (e ProviderRequestError) Unwrap() error { return e.Cause }
