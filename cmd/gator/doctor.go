package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

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
	authentication := model.CredentialHint(provider)
	authenticationStatus := "check with provider CLI"
	if !model.IsHarness(provider) {
		authenticationStatus = "missing"
		if os.Getenv(authentication) != "" {
			authenticationStatus = "set"
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
