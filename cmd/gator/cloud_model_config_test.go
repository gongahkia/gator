package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/modelcatalog"
)

func TestSaveTUICloudModelConfigurationStoresAPIKeyAndSettings(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	endpoint := "https://example-resource.openai.azure.com/openai/deployments/review/chat/completions?api-version=2025-04-01-preview"
	err = saveTUICloudModelConfiguration(settingsStore, stateDir)(modelcatalog.CloudModelSetup{
		Provider: "azure-openai", Model: "review", BaseURL: endpoint, APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("save Azure configuration: %v", err)
	}
	settings, err := settingsStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Defaults.Provider != "azure-openai" || settings.Defaults.Model != "review" || settings.ProviderEndpoint("azure-openai") != endpoint {
		t.Fatalf("settings = %#v", settings)
	}
	contents, err := os.ReadFile(settingsStore.Path())
	if err != nil || strings.Contains(string(contents), "test-api-key") {
		t.Fatalf("settings leaked API key: %v %s", err, contents)
	}
	credentials, err := auth.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	credential, found, err := credentials.Read("azure-openai")
	if err != nil || !found || !credential.IsAPIKey() || credential.Key != "test-api-key" {
		t.Fatalf("stored credential = %#v found=%v err=%v", credential, found, err)
	}
}

func TestCloudSetupCredentialFailureLeavesSettingsUntouched(t *testing.T) {
	settings, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	before := config.Default()
	before.Defaults = config.Defaults{Provider: "anthropic", Model: "previous"}
	if err := settings.Save(before); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(settings.Path())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "gator", "auth.json"), 0700); err != nil {
		t.Fatal(err)
	}
	err = saveTUICloudModelConfiguration(settings, root)(modelcatalog.CloudModelSetup{Provider: "openai", Model: "replacement", APIKey: "private-failed-key"})
	if err == nil || strings.Contains(err.Error(), "private-failed-key") {
		t.Fatalf("unsafe/missing failure: %v", err)
	}
	current, _ := os.ReadFile(settings.Path())
	if !bytes.Equal(original, current) {
		t.Fatal("credential failure changed selected model")
	}
}

func TestCloudSetupSettingsFailureRestoresPreviousCredential(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "existing"}[existing], func(t *testing.T) {
			settings, err := config.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := settings.Save(config.Default()); err != nil {
				t.Fatal(err)
			}
			original, _ := os.ReadFile(settings.Path())
			root := t.TempDir()
			credentials, err := auth.New(root)
			if err != nil {
				t.Fatal(err)
			}
			prior := auth.Credential{Type: "api_key", Key: "previous-private-key"}
			if existing {
				if err := credentials.Put("openai", prior); err != nil {
					t.Fatal(err)
				}
			}
			// The real settings store rejects this oversized model after the
			// credential write, exercising compensation without a mock disk.
			err = saveTUICloudModelConfiguration(settings, root)(modelcatalog.CloudModelSetup{Provider: "openai", Model: strings.Repeat("x", 513), APIKey: "replacement-private-key"})
			if err == nil || !strings.Contains(err.Error(), "not saved") {
				t.Fatalf("missing save failure: %v", err)
			}
			credential, found, err := credentials.Read("openai")
			if err != nil || found != existing || (found && credential.Key != prior.Key) {
				t.Fatal("previous credential was not restored")
			}
			current, _ := os.ReadFile(settings.Path())
			if !bytes.Equal(original, current) {
				t.Fatal("failed validation changed settings")
			}
		})
	}
}

type cloudUpdateFunc func(func(*config.Settings) error) error

func (f cloudUpdateFunc) Update(change func(*config.Settings) error) error { return f(change) }

func TestOverlappingCloudSetupsCannotRestoreEachOthersTemporaryKeys(t *testing.T) {
	root := t.TempDir()
	credentials, err := auth.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put("openai", auth.Credential{Type: "api_key", Key: "original"}); err != nil {
		t.Fatal(err)
	}
	settings, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	update := cloudUpdateFunc(func(change func(*config.Settings) error) error {
		err := saveTUICloudModelConfiguration(settings, root)(modelcatalog.CloudModelSetup{Provider: "openai", Model: "overlapping", APIKey: "second-temporary"})
		if err == nil || !strings.Contains(err.Error(), "in progress") {
			t.Fatalf("overlapping setup was not refused: %v", err)
		}
		return errors.New("first settings publication failed")
	})
	if err := saveTUICloudModelConfiguration(update, root)(modelcatalog.CloudModelSetup{Provider: "openai", Model: "first", APIKey: "first-temporary"}); err == nil {
		t.Fatal("missing publication failure")
	}
	current, found, err := credentials.Read("openai")
	if err != nil || !found || current.Key != "original" {
		t.Fatal("temporary credential survived failed overlapping setups")
	}
	if err := saveTUICloudModelConfiguration(settings, root)(modelcatalog.CloudModelSetup{Provider: "openai", Model: "retry", APIKey: "retry-key"}); err != nil {
		t.Fatalf("setup lease was not released: %v", err)
	}
}

func TestCloudSetupDoesNotUndoConcurrentCredentialReplacement(t *testing.T) {
	root := t.TempDir()
	credentials, err := auth.New(root)
	if err != nil {
		t.Fatal(err)
	}
	newer := auth.Credential{Type: "api_key", Key: "newer-private-key"}
	update := cloudUpdateFunc(func(change func(*config.Settings) error) error {
		if err := credentials.Put("openai", newer); err != nil {
			return err
		}
		settings := config.Default()
		return change(&settings)
	})
	err = saveTUICloudModelConfiguration(update, root)(modelcatalog.CloudModelSetup{Provider: "openai", Model: "selected", APIKey: "superseded-private-key"})
	if err == nil || !strings.Contains(err.Error(), "credential recovery was not applied") {
		t.Fatalf("missing concurrent-change explanation: %v", err)
	}
	current, found, err := credentials.Read("openai")
	if err != nil || !found || current.Key != newer.Key {
		t.Fatal("failed setup overwrote newer credential")
	}
}

func TestCloudSetupWaitsForAndPreservesConcurrentSettingsUpdate(t *testing.T) {
	settings, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	otherDone, cloudDone := make(chan error, 1), make(chan error, 1)
	go func() {
		otherDone <- settings.Update(func(value *config.Settings) error {
			close(entered)
			<-release
			value.Theme = "light"
			value.ModelAliases = map[string]string{"anthropic:kept": "Keep this alias"}
			return nil
		})
	}()
	<-entered
	root := t.TempDir()
	go func() {
		cloudDone <- saveTUICloudModelConfiguration(settings, root)(modelcatalog.CloudModelSetup{Provider: "openai", Model: "selected"})
	}()
	select {
	case err := <-cloudDone:
		close(release)
		<-otherDone
		t.Fatalf("cloud setup ignored the active settings transaction: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-otherDone; err != nil {
		t.Fatal(err)
	}
	if err := <-cloudDone; err != nil {
		t.Fatal(err)
	}
	got, err := settings.Load()
	if err != nil || got.Theme != "light" || got.ModelAliases["anthropic:kept"] != "Keep this alias" || got.Defaults.Model != "selected" {
		t.Fatalf("settings edits lost: %v", err)
	}
}

func TestSaveTUICloudModelConfigurationRejectsRemovedProviderWithoutWriting(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = saveTUICloudModelConfiguration(settingsStore, t.TempDir())(modelcatalog.CloudModelSetup{
		Provider: "codex", Model: "gpt-5.6", APIKey: "must-not-be-persisted",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("removed provider error = %v", err)
	}
	settings, err := settingsStore.Load()
	if err != nil || settings.Defaults.Provider != "" || settings.Defaults.Model != "" {
		t.Fatalf("settings unexpectedly changed: %#v err=%v", settings, err)
	}
}

func TestSaveTUICloudModelConfigurationSupportsEveryAPIKeyProvider(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	save := saveTUICloudModelConfiguration(settingsStore, t.TempDir())
	for _, providerName := range model.Names() {
		t.Run(providerName, func(t *testing.T) {
			provider, err := model.ParseProvider(providerName)
			if err != nil {
				t.Fatal(err)
			}
			if !model.SupportsAPIKeyLogin(provider) {
				t.Fatalf("%s lacks API-key setup", provider)
			}
			if err := save(modelcatalog.CloudModelSetup{Provider: providerName, Model: "test-model", APIKey: "test-api-key"}); err != nil {
				t.Fatalf("save provider configuration: %v", err)
			}
		})
	}
}
