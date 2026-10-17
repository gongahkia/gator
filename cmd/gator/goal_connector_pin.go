package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/jobs"
)

// captureGoalOAuthAccountPins binds new Google follow-ups to the live OIDC
// subject. The digest survives token rotation without exposing the subject.
func captureGoalOAuthAccountPins(ctx context.Context, selected []string, registry connector.Registry, credentials auth.Store) (map[string]string, error) {
	var pins map[string]string
	runtime := connector.Runtime{Registry: registry, Credentials: credentials}
	for _, id := range selected {
		descriptor, found := registry.Get(id)
		if !found {
			return nil, fmt.Errorf("connector %q is not configured", id)
		}
		if descriptor.Kind != connector.KindGoogle || descriptor.Authentication != connector.AuthOAuth {
			continue
		}
		credential, found, err := credentials.Read(descriptor.CredentialRef())
		if err != nil {
			return nil, fmt.Errorf("read connector %q credential: %w", id, err)
		}
		if !found || !credential.IsOAuth() || credential.Refresh == "" {
			return nil, fmt.Errorf("log in to Google connector %q before scheduling a recurring read", id)
		}
		if pins == nil {
			pins = make(map[string]string)
		}
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		pin, err := runtime.GoogleAccountPin(probeCtx, id)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("verify Google connector %q account before scheduling: %w", id, err)
		}
		pins[id] = pin
	}
	return pins, nil
}

func captureConfiguredGoalOAuthAccountPins(ctx context.Context, selected []string) (map[string]string, error) {
	if len(selected) == 0 {
		return nil, nil
	}
	settings, err := loadSettings()
	if err != nil {
		return nil, err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return nil, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return nil, err
	}
	return captureGoalOAuthAccountPins(ctx, selected, registry, credentials)
}

func verifyGoalOAuthPins(ctx context.Context, definition jobs.Definition, registry connector.Registry, credentials auth.Store) error {
	for id, pinned := range definition.OAuthRefreshPins {
		if err := verifyGoalOAuthPin(id, pinned, registry, credentials); err != nil {
			return err
		}
	}
	if len(definition.OAuthAccountPins) > 0 {
		runtime := connector.Runtime{Registry: registry, Credentials: credentials}
		for id, pinned := range definition.OAuthAccountPins {
			probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			actual, err := runtime.GoogleAccountPin(probeCtx, id)
			cancel()
			if err != nil {
				return fmt.Errorf("Google connector %q account identity unavailable: %w", id, err)
			}
			if actual != pinned {
				return fmt.Errorf("Google connector %q account changed; recreate this scheduled check after confirming the account", id)
			}
		}
	}
	return nil
}

func verifyGoalOAuthPin(id, pinned string, registry connector.Registry, credentials auth.Store) error {
	descriptor, found := registry.Get(id)
	if !found || descriptor.Kind != connector.KindGoogle || descriptor.Authentication != connector.AuthOAuth {
		return fmt.Errorf("Google connector %q changed; recreate this scheduled check after reviewing its source", id)
	}
	credential, found, err := credentials.Read(descriptor.CredentialRef())
	if err != nil {
		return errors.New("Google connector credential status is unavailable")
	}
	if !found || !credential.IsOAuth() || credential.Refresh == "" || connector.OAuthGrantPin(descriptor, credential.Refresh) != pinned {
		return fmt.Errorf("Google connector %q authorization changed; recreate this scheduled check after confirming the account", id)
	}
	return nil
}

func verifyConfiguredGoalOAuthPins(ctx context.Context, definition jobs.Definition) error {
	if len(definition.OAuthRefreshPins) == 0 && len(definition.OAuthAccountPins) == 0 {
		return nil
	}
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return err
	}
	return verifyGoalOAuthPins(ctx, definition, registry, credentials)
}
