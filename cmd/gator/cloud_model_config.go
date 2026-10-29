package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/modelcatalog"
	"github.com/gongahkia/gator/internal/privatefs"
)

type cloudSettingsUpdater interface {
	Update(func(*config.Settings) error) error
}

// saveTUICloudModelConfiguration keeps the UI's secret entry boundary small:
// the TUI only holds the typed value until this callback stores it in auth.json.
// Endpoint overrides remain non-secret settings shared by TUI and CLI runs.
func saveTUICloudModelConfiguration(settingsStore cloudSettingsUpdater, stateDir string) func(modelcatalog.CloudModelSetup) error {
	return func(setup modelcatalog.CloudModelSetup) error {
		provider, err := model.ParseProvider(setup.Provider)
		if err != nil {
			return err
		}
		if !model.SupportsDirect(provider) {
			return fmt.Errorf("provider %q has no direct Gator cloud configuration", provider)
		}
		credentialProvider, credential, storeCredential, err := tuiCloudCredential(provider, setup.APIKey)
		if err != nil {
			return err
		}
		var credentials auth.Store
		var previous *auth.Credential
		if storeCredential {
			credentials, err = auth.New(stateDir)
			if err != nil {
				return fmt.Errorf("open cloud credential store; model settings were not changed: %w", err)
			}
			release, err := lockCloudCredentialSetup(credentials)
			if err != nil {
				return err
			}
			defer release()
			prior, found, readErr := credentials.Read(credentialProvider)
			if readErr != nil {
				return fmt.Errorf("read cloud credential; model settings were not changed: %w", readErr)
			}
			if found {
				previous = &prior
			}
			if err := credentials.ReplaceReviewed(credentialProvider, previous, &credential); err != nil {
				return fmt.Errorf("save cloud credential; model settings were not changed: %w", err)
			}
		}
		err = settingsStore.Update(func(settings *config.Settings) error {
			// Do not publish settings for a credential replaced while waiting
			// for another chat's settings transaction.
			if storeCredential {
				current, found, err := credentials.Read(credentialProvider)
				if err != nil {
					return err
				}
				if !found || !reflect.DeepEqual(current, credential) {
					return auth.ErrCredentialChanged
				}
			}

			settings.Defaults.Provider = string(provider)
			settings.Defaults.Model = strings.TrimSpace(setup.Model)
			if settings.ProviderEndpoints == nil {
				settings.ProviderEndpoints = make(map[string]string)
			}
			if baseURL := strings.TrimSpace(setup.BaseURL); baseURL == "" {
				delete(settings.ProviderEndpoints, string(provider))
			} else {
				settings.ProviderEndpoints[string(provider)] = baseURL
			}
			if settings.ProviderOptions == nil {
				settings.ProviderOptions = make(map[string]map[string]string)
			}
			if len(setup.Options) == 0 {
				delete(settings.ProviderOptions, string(provider))
			} else {
				settings.ProviderOptions[string(provider)] = cloneTUIProviderOptions(setup.Options)
			}
			return nil
		})
		if err != nil {
			if storeCredential {
				// Restore only our own write. A concurrent sign-in/disconnect
				// wins and must never be undone by this failed setup.
				if restoreErr := credentials.ReplaceReviewed(credentialProvider, &credential, previous); restoreErr != nil {
					return fmt.Errorf("model settings were not saved; credential recovery was not applied (%v); check credentials before retrying: %w", restoreErr, err)
				}
			}
			return fmt.Errorf("model settings were not saved; previous credential retained: %w", err)
		}

		return nil
	}
}

// Serialize setups across chats/processes so two failed saves cannot restore
// each other's temporary credentials. Direct credential changes still use the
// auth store's compare-and-swap checks. This is not a cross-file crash journal.
func lockCloudCredentialSetup(credentials auth.Store) (func(), error) {
	directory := filepath.Dir(credentials.Path())
	if err := privatefs.MkdirAll(directory); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := filelock.OpenRootFile(root, ".cloud-setup.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := filelock.Lock(file, filelock.Exclusive|filelock.Nonblocking); err != nil {
		file.Close()
		if errors.Is(err, filelock.ErrWouldBlock) {
			return nil, fmt.Errorf("another cloud setup is in progress; retry after it finishes: %w", err)
		}
		return nil, fmt.Errorf("lock cloud setup: %w", err)
	}
	return func() { _ = filelock.Unlock(file); _ = file.Close() }, nil
}

// tuiCloudCredential validates the credential mode before any non-secret
// settings are written. This prevents an account-mode submission from
// partially changing the selected provider or endpoint.
func tuiCloudCredential(provider model.Provider, value string) (string, auth.Credential, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", auth.Credential{}, false, nil
	}
	if !model.SupportsAPIKeyLogin(provider) {
		return "", auth.Credential{}, false, fmt.Errorf("provider %q has no API-key setup", provider)
	}
	return string(provider), auth.Credential{Type: "api_key", Key: value}, true, nil
}

func cloneTUIProviderOptions(options map[string]string) map[string]string {
	cloned := make(map[string]string, len(options))
	for name, value := range options {
		cloned[name] = strings.TrimSpace(value)
	}
	return cloned
}
