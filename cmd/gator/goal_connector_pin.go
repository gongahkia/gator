package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/jobs"
)

// captureGoalOAuthPins binds new Google follow-ups to the currently authorized
// refresh grant. A new login, even for the same account, requires an explicit
// replacement of the check rather than silently changing its source identity.
func captureGoalOAuthPins(selected []string, registry connector.Registry, credentials auth.Store) (map[string]string, error) {
	var pins map[string]string
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
		pins[id] = goalOAuthPin(descriptor, credential)
	}
	return pins, nil
}

func captureConfiguredGoalOAuthPins(selected []string) (map[string]string, error) {
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
	return captureGoalOAuthPins(selected, registry, credentials)
}

func verifyGoalOAuthPins(definition jobs.Definition, registry connector.Registry, credentials auth.Store) error {
	for id, pinned := range definition.OAuthRefreshPins {
		descriptor, found := registry.Get(id)
		if !found || descriptor.Kind != connector.KindGoogle || descriptor.Authentication != connector.AuthOAuth {
			return fmt.Errorf("Google connector %q changed; recreate this scheduled check after reviewing its source", id)
		}
		credential, found, err := credentials.Read(descriptor.CredentialRef())
		if err != nil {
			return errors.New("Google connector credential status is unavailable")
		}
		if !found || !credential.IsOAuth() || credential.Refresh == "" || goalOAuthPin(descriptor, credential) != pinned {
			return fmt.Errorf("Google connector %q authorization changed; recreate this scheduled check after confirming the account", id)
		}
	}
	return nil
}

func goalOAuthPin(descriptor connector.Descriptor, credential auth.Credential) string {
	digest := sha256.Sum256([]byte(descriptor.CredentialRef() + "\x00" + credential.Refresh))
	return hex.EncodeToString(digest[:])
}
