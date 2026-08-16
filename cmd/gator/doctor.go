package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gongahkia/gator/internal/model"
)

func doctor(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	defaultProvider := os.Getenv("GATOR_PROVIDER")
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
	provider, err := model.ParseProvider(*providerName)
	if err != nil {
		return err
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
	if provider == model.GoogleVertex {
		authentication = model.CredentialHint(provider)
	} else if model.SupportsAPIKeyLogin(provider) {
		authentication += " or " + model.CredentialHint(provider)
	}
	authenticationStatus := "missing"
	if !model.SupportsDirect(provider) {
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
		case stored && (credential.IsAPIKey() || credential.IsOAuth()):
			authenticationStatus = "stored"
		case model.SupportsAPIKeyLogin(provider) && model.APIKeyEnvironment(provider) != "" && os.Getenv(model.APIKeyEnvironment(provider)) != "":
			authenticationStatus = "set in environment"
		}
	}
	if _, err := fmt.Fprintf(out, "Repository: %s\nProvider: %s\nAuthentication (%s): %s\n", gitStatus, provider, authentication, authenticationStatus); err != nil {
		return err
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
