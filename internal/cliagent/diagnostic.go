package cliagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/model/providerhttp"
)

// cliFailure records only host-known metadata, never CLI output or guessed HTTP status.
func cliFailure(cfg Config, operation, phase string, started time.Time, cause, exitErr error) error {
	if agent.ProviderDiagnosticFrom(cause) != nil {
		return cause
	}
	secret := os.Getenv(cfg.APIKeyEnv)
	d := agent.ProviderDiagnostic{
		Provider: providerhttp.SafeValue(cfg.Kind, secret), Model: providerhttp.SafeValue(cfg.Model, secret),
		Transport: "cli", Phase: phase, Method: operation, Endpoint: providerhttp.SafeEndpoint(cfg.BaseURL, secret),
		CredentialSource: "environment", Auth: "child-environment", ElapsedMS: max(0, time.Since(started).Milliseconds()), Code: "cli_failure",
	}
	if secret == "" {
		d.CredentialSource, d.Auth = "none", "none"
	}
	switch {
	case errors.Is(cause, context.DeadlineExceeded):
		d.Code = "cli_timeout"
	case errors.Is(cause, context.Canceled):
		d.Code = "cli_cancelled"
	}
	var exited *exec.ExitError
	if errors.As(exitErr, &exited) {
		code := exited.ExitCode()
		d.ExitCode = &code
	}
	return agent.ProviderRequestError{Diagnostic: d, Summary: fmt.Sprintf("%s CLI failed during %s", providerhttp.SafeValue(cfg.Kind, secret), phase), Cause: cause}
}
