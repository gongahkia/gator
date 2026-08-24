package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/dependency"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/sandbox"
)

func doctor(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	defaults, err := configuredDefaults()
	if err != nil {
		return err
	}
	defaultProvider := defaults.Provider
	if configured := os.Getenv("GATOR_PROVIDER"); configured != "" {
		defaultProvider = configured
	}
	if defaultProvider == "" {
		defaultProvider = string(model.OpenAI)
	}
	providerName := flags.String("provider", defaultProvider, "provider to inspect")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("doctor does not accept positional arguments")
	}
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	custom, customProvider := configuredCustomProvider(settings, *providerName)
	provider := model.Provider("")
	if !customProvider {
		provider, err = model.ParseProvider(*providerName)
		if err != nil {
			return err
		}
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	repository, err := gitRepositoryRoot(workingDirectory)
	gitStatus := "not detected"
	if err == nil {
		gitStatus = "detected"
	}
	authentication := "Gator credential"
	if customProvider {
		if custom.APIKeyEnv == "" {
			authentication = "no API key"
		} else {
			authentication = "API key from " + custom.APIKeyEnv
		}
	} else if provider == model.Claude {
		authentication = "unsupported native Claude.ai subscription OAuth; use provider anthropic with an API key"
	} else if provider == model.GoogleVertex {
		authentication = model.CredentialHint(provider)
	} else if provider == model.AmazonBedrock {
		authentication += " or " + model.CredentialHint(provider)
	} else if model.SupportsAPIKeyLogin(provider) {
		authentication += " or " + model.CredentialHint(provider)
	}
	authenticationStatus := "missing"
	if customProvider {
		if custom.APIKeyEnv == "" {
			authenticationStatus = "not required"
		} else if strings.TrimSpace(os.Getenv(custom.APIKeyEnv)) != "" {
			authenticationStatus = "set in environment"
		}
	} else if provider == model.Claude {
		authenticationStatus = "use gator connect claude"
	} else if !model.SupportsDirect(provider) {
		authenticationStatus = "unsupported"
	} else if provider == model.GoogleVertex {
		if model.AmbientCredentialAvailable(provider) {
			authenticationStatus = "configured"
		}
	} else {
		credentials, credentialErr := gatorCredentials()
		if credentialErr != nil {
			return credentialErr
		}
		credential, stored, credentialErr := credentials.Read(string(provider))
		if credentialErr != nil {
			return credentialErr
		}
		switch {
		case stored && credential.Expired(time.Now()):
			authenticationStatus = "expired"
		case stored && (credential.IsAPIKey() || credential.IsBearerToken() || credential.IsOAuth()):
			authenticationStatus = "stored"
		case provider == model.AmazonBedrock && model.AmbientCredentialAvailable(provider):
			authenticationStatus = model.AmbientCredentialSource(provider)
		case provider == model.AzureOpenAIResponses && model.AmbientCredentialAvailable(provider):
			authenticationStatus = "set in environment"
		case model.SupportsAPIKeyLogin(provider) && model.APIKeyEnvironment(provider) != "" && os.Getenv(model.APIKeyEnvironment(provider)) != "":
			authenticationStatus = "set in environment"
		}
	}
	sandboxStatus, sandboxErr := sandbox.StrictAvailability()
	if sandboxErr != nil {
		sandboxStatus = "unavailable: " + sandboxErr.Error()
	}
	webSearchStatus := "not configured (set BRAVE_SEARCH_API_KEY)"
	if strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")) != "" {
		webSearchStatus = "configured (requires --network allow)"
	}
	providerDisplay := string(provider)
	if customProvider {
		providerDisplay = custom.ID
	}
	if _, err := fmt.Fprintf(out, "Repository: %s\nProvider: %s\nAuthentication (%s): %s\nStrict sandbox: %s\nWeb search: %s\n", gitStatus, providerDisplay, authentication, authenticationStatus, sandboxStatus, webSearchStatus); err != nil {
		return err
	}
	if err := writeDependencyDoctor(out, dependency.Detect()); err != nil {
		return err
	}
	if err := writeLocalModelDoctor(out, inspectLocalModelHost()); err != nil {
		return err
	}
	if customProvider && custom.ID == localmodel.ProviderID {
		if _, err := fmt.Fprintln(out, "Local runtime: run 'gator local status' to verify the selected loopback Ollama server and model inventory."); err != nil {
			return err
		}
	}
	suggestionDirectory := workingDirectory
	if gitStatus == "detected" {
		suggestionDirectory = repository
	}
	for _, suggestion := range suggestedVerificationCommands(suggestionDirectory) {
		if _, err := fmt.Fprintf(out, "Suggested verification: %s\n", suggestion); err != nil {
			return err
		}
	}
	if gitStatus != "detected" {
		_, err := fmt.Fprintln(out, "Run Gator from a Git checkout.")
		return err
	}
	return nil
}
