package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
)

func gmailSendingDescriptor(d connector.Descriptor, enabled bool) (connector.Descriptor, error) {
	if d.Kind != connector.KindGoogle || d.Authentication != connector.AuthOAuth {
		return d, errors.New("email sending permissions require a Google browser connection")
	}
	scopes := strings.Fields(d.OAuthScopes)
	current := slices.Contains(scopes, connector.GoogleSendOAuthScope)
	if current == enabled {
		return d, errors.New("email sending permissions changed; refresh this connection")
	}
	if enabled {
		scopes = append(scopes, connector.GoogleSendOAuthScope)
	} else {
		scopes = slices.DeleteFunc(scopes, func(scope string) bool { return scope == connector.GoogleSendOAuthScope })
	}
	d.OAuthScopes = strings.Join(scopes, " ")
	return d, nil
}

// Publish the expanded surface only after the new grant is stored. If the
// configuration cannot be published, restore the prior credential using CAS;
// a concurrently replaced/disconnected account is never overwritten. A crash
// between stores leaves the old, narrower operation surface configured.
func publishGmailSendingPermission(prior, next connector.Descriptor, credentials auth.Store, expected *auth.Credential, credential auth.Credential) error {
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	if err := credentials.ReplaceReviewed(prior.CredentialRef(), expected, &credential); err != nil {
		return err
	}
	err = replaceReviewedConnection(store, prior, next)
	if err == nil {
		return nil
	}
	if restoreErr := credentials.ReplaceReviewed(prior.CredentialRef(), &credential, expected); restoreErr != nil {
		return fmt.Errorf("sending settings were not updated (%v); sign-in also changed, so check this connection before continuing", err)
	}
	return fmt.Errorf("sending settings were not updated; prior local sign-in restored: %w", err)
}

func replaceReviewedConnection(store config.Store, prior, next connector.Descriptor) error {
	return store.Update(func(settings *config.Settings) error {
		for i, d := range settings.Connectors {
			if d.ID != prior.ID {
				continue
			}
			if connectorLoginReview(d) != connectorLoginReview(prior) {
				return errors.New("connection changed; review its permissions again")
			}
			settings.Connectors[i] = next
			return nil
		}
		return errors.New("connection was removed; review its permissions again")
	})
}
