package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/dependency"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

type doctorFacts struct {
	Repository, Provider, Authentication, AuthenticationStatus string
	Sandbox, WebSearch, SuggestionDirectory                    string
	SandboxAvailable, WebSearchConfigured, CustomLocal         bool
	Dependencies                                               []dependency.Status
	Host                                                       localmodel.Host
	Suggestions                                                []string
}

func inspectDoctor(arguments []string) (doctorFacts, error) {
	var facts doctorFacts
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	defaults, err := configuredDefaults()
	if err != nil {
		return facts, err
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
		return facts, err
	}
	if len(flags.Args()) != 0 {
		return facts, errors.New("doctor does not accept positional arguments")
	}
	settings, err := loadSettings()
	if err != nil {
		return facts, err
	}
	custom, customProvider := configuredCustomProvider(settings, *providerName)
	provider := model.Provider("")
	if !customProvider {
		provider, err = model.ParseProvider(*providerName)
		if err != nil {
			return facts, err
		}
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return facts, fmt.Errorf("get working directory: %w", err)
	}
	repository, err := gitRepositoryRoot(workingDirectory)
	gitStatus := "not detected"
	if err == nil {
		gitStatus = "detected"
	}
	authentication, authenticationStatus, err := inspectProviderAuth(custom, customProvider, provider)
	if err != nil {
		return facts, err
	}
	sandboxStatus, sandboxErr := sandbox.StrictAvailability()
	if sandboxErr != nil {
		sandboxStatus = "unavailable: " + sandboxErr.Error()
	}
	providerDisplay := string(provider)
	if customProvider {
		providerDisplay = custom.ID
	}
	facts = doctorFacts{Repository: gitStatus, Provider: providerDisplay, Authentication: authentication,
		AuthenticationStatus: authenticationStatus, Sandbox: sandboxStatus, SandboxAvailable: sandboxErr == nil,
		WebSearchConfigured: strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")) != "",
		CustomLocal:         customProvider && custom.ID == localmodel.ProviderID,
		Dependencies:        dependency.Detect(), Host: inspectLocalModelHost()}
	facts.WebSearch = "not configured (set BRAVE_SEARCH_API_KEY)"
	if facts.WebSearchConfigured {
		facts.WebSearch = "configured (requires --network allow)"
	}
	facts.SuggestionDirectory = workingDirectory
	if gitStatus == "detected" {
		facts.SuggestionDirectory = repository
	}
	facts.Suggestions = suggestedVerificationCommands(facts.SuggestionDirectory)
	return facts, nil
}

func doctor(arguments []string, out io.Writer) error {
	facts, err := inspectDoctor(arguments)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Repository: %s\nProvider: %s\nAuthentication (%s): %s\nStrict sandbox: %s\nWeb search: %s\n", facts.Repository, facts.Provider, facts.Authentication, facts.AuthenticationStatus, facts.Sandbox, facts.WebSearch); err != nil {
		return err
	}
	if err := writeDependencyDoctor(out, facts.Dependencies); err != nil {
		return err
	}
	if err := writeLocalModelDoctor(out, facts.Host); err != nil {
		return err
	}
	if facts.CustomLocal {
		if _, err := fmt.Fprintln(out, "Local runtime: open Settings → Models → Local model in Gator to inspect the Ollama server and model inventory."); err != nil {
			return err
		}
	}
	for _, suggestion := range facts.Suggestions {
		if _, err := fmt.Fprintf(out, "Suggested verification: %s\n", suggestion); err != nil {
			return err
		}
	}
	if facts.Repository != "detected" {
		_, err := fmt.Fprintln(out, "Run Gator from a Git checkout.")
		return err
	}
	return nil
}

func doctorResult(arguments []string) (*uisnapshot.Result, error) {
	facts, err := inspectDoctor(arguments)
	if err != nil {
		return nil, err
	}
	result := &uisnapshot.Result{Kind: "report", Title: "Gator doctor", Sections: []uisnapshot.ResultSection{{
		Title: "Environment", Fields: []uisnapshot.Field{
			{Label: "Repository", Value: facts.Repository, Tone: doctorTone(facts.Repository == "detected", true)},
			{Label: "Provider", Value: facts.Provider},
			{Label: "Authentication (" + facts.Authentication + ")", Value: facts.AuthenticationStatus, Tone: doctorTone(facts.AuthenticationStatus == "stored" || facts.AuthenticationStatus == "set in environment" || facts.AuthenticationStatus == "not required", true)},
			{Label: "Strict sandbox", Value: facts.Sandbox, Tone: doctorTone(facts.SandboxAvailable, true)},
			{Label: "Web search", Value: facts.WebSearch, Tone: doctorTone(facts.WebSearchConfigured, false)},
		},
	}, {Title: "Dependencies"}, {Title: "Local model host"}, {Title: "Reviewed local models"}}}
	for _, status := range facts.Dependencies {
		usable := status.Installed && (status.ID != "node" || supportedNodeTools(status.Path))
		state := "installed"
		if !status.Installed {
			state = "missing"
		} else if !usable {
			state = "unsupported version"
		}
		role := "optional"
		if status.Required {
			role = "required"
		}
		detail := role + " for " + status.Purpose
		if !usable {
			if status.HelpURL != "" {
				detail += "\nOfficial source: " + status.HelpURL
			}
			if status.Instructions != nil {
				detail += "\n" + strings.Join(status.Instructions(facts.Host.OS), "\n")
			}
		}
		result.Sections[1].Items = append(result.Sections[1].Items, uisnapshot.ResultItem{Label: status.Name, Detail: detail, State: state, Tone: doctorTone(usable, role == "required")})
	}
	host := facts.Host
	memory := "unavailable"
	if host.TotalMemoryBytes > 0 {
		memory = localmodel.FormatBytes(host.TotalMemoryBytes) + " total"
		if host.AvailableMemoryKnown || host.AvailableMemoryBytes > 0 {
			memory += " · " + localmodel.FormatBytes(host.AvailableMemoryBytes) + " available"
		}
	}
	if host.MemoryError != "" {
		memory += " · " + host.MemoryError
	}
	storage := host.ModelDirectory
	if storage == "" {
		storage = "unavailable"
	}
	if host.ModelDirectorySource != "" {
		storage += " (" + host.ModelDirectorySource + ")"
	}
	if host.AvailableDiskKnown || host.AvailableDiskBytes > 0 {
		storage += " · " + localmodel.FormatBytes(host.AvailableDiskBytes) + " free"
	}
	if host.DiskError != "" {
		storage += " · " + host.DiskError
	}
	result.Sections[2].Fields = []uisnapshot.Field{
		{Label: "Host", Value: host.OS + "/" + host.Architecture},
		{Label: "Memory", Value: memory, Tone: doctorTone(host.TotalMemoryBytes > 0, false)},
		{Label: "Ollama model storage", Value: storage, Tone: doctorTone(host.ModelDirectory != "", false)},
	}
	for _, entry := range localmodel.Catalog() {
		eligibility := localmodel.Assess(entry, host)
		state := "enabled"
		if !eligibility.Allowed {
			state = "disabled: " + eligibility.Reason
		}
		result.Sections[3].Items = append(result.Sections[3].Items, uisnapshot.ResultItem{Label: entry.ID,
			Detail: "Needs " + localmodel.FormatBytes(eligibility.RequiredMemoryBytes) + " RAM and " + localmodel.FormatBytes(eligibility.RequiredDiskBytes) + " free disk",
			State:  state, Tone: doctorTone(eligibility.Allowed, false)})
	}
	result.Sections = append(result.Sections, uisnapshot.ResultSection{Title: "Next steps", Lines: []string{
		"Gator blocks reviewed models that fail its conservative RAM, disk, or OS guardrail. Context length, parallel requests, and GPU/VRAM can increase actual requirements; use 'ollama ps' to inspect a loaded model's processor.",
	}})
	if facts.CustomLocal {
		result.Sections[len(result.Sections)-1].Lines = append(result.Sections[len(result.Sections)-1].Lines, "Open /model → Local to inspect Ollama and installed models.")
	}
	for _, suggestion := range facts.Suggestions {
		result.Sections[len(result.Sections)-1].Items = append(result.Sections[len(result.Sections)-1].Items, uisnapshot.ResultItem{Label: suggestion})
	}
	if facts.Repository != "detected" {
		result.Sections[len(result.Sections)-1].Lines = append(result.Sections[len(result.Sections)-1].Lines, "Run Gator from a Git checkout.")
	}
	return result, nil
}

func doctorTone(healthy, required bool) string {
	if healthy {
		return "success"
	}
	if required {
		return "error"
	}
	return "warning"
}

func inspectProviderAuth(custom config.CustomProvider, customProvider bool, provider model.Provider) (string, string, error) {
	authentication := "Gator credential"
	if customProvider {
		if custom.APIKeyEnv == "" {
			authentication = "no API key"
		} else {
			authentication = "API key from " + custom.APIKeyEnv
		}
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
		return authentication, authenticationStatus, nil
	}
	if !model.SupportsDirect(provider) {
		return authentication, "unsupported", nil
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return "", "", err
	}
	credential, stored, err := credentials.Read(string(provider))
	if err != nil {
		return "", "", err
	}
	switch {
	case stored && credential.Expired(time.Now()):
		authenticationStatus = "expired"
	case stored && credential.IsAPIKey():
		authenticationStatus = "stored"
	case model.SupportsAPIKeyLogin(provider) && model.APIKeyEnvironment(provider) != "" && os.Getenv(model.APIKeyEnvironment(provider)) != "":
		authenticationStatus = "set in environment"
	}
	return authentication, authenticationStatus, nil
}
