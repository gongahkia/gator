// Package modelcatalog defines the application boundary for Gator's cloud and
// reviewed local-model catalog. Its contracts contain display-safe metadata;
// credential and runtime ownership remain in the command layer.
package modelcatalog

import (
	"context"

	"github.com/gongahkia/gator/internal/config"
)

// LocalManager owns local runtime and persistent selection operations.
type LocalManager interface {
	Status(context.Context) (LocalCatalog, error)
	Start(context.Context) (LocalCatalog, error)
	Pull(context.Context, string, func(LocalProgress)) (LocalCatalog, error)
	Use(context.Context, string) (LocalUpdate, error)
	Remove(context.Context, string) (LocalUpdate, error)
	Rename(context.Context, string, string, string) (map[string]string, error)
}

// LocalCatalog is display-safe state for the reviewed local catalog.
type LocalCatalog struct {
	RuntimeURL     string
	RuntimeVersion string
	RuntimeError   string
	Executable     string
	HostSummary    string
	HostAdvice     []string
	Dependencies   []LocalDependency
	Models         []LocalModel
}

// LocalDependency is display-safe prerequisite status and guidance.
type LocalDependency struct {
	ID           string
	Name         string
	Purpose      string
	Required     bool
	Installed    bool
	HelpURL      string
	Instructions []string
}

// LocalModel is one selectable reviewed local coding model.
type LocalModel struct {
	ID            string
	OllamaModel   string
	Name          string
	DefaultName   string
	Download      string
	Context       string
	Summary       string
	SourceURL     string
	Installed     bool
	Requirement   string
	BlockedReason string
}

// LocalProgress is a bounded, display-only pull update.
type LocalProgress struct {
	Status    string
	Completed int64
	Total     int64
}

// LocalUpdate returns a fresh catalog and persisted selection state.
type LocalUpdate struct {
	Catalog         LocalCatalog
	CustomProviders []config.CustomProvider
	ModelAliases    map[string]string
	Provider        string
	Model           string
}

// ManagementBackend is the secret-safe command-layer boundary for cloud
// credentials and custom OpenAI-compatible providers.
type ManagementBackend interface {
	CredentialStatuses() ([]StoredCredentialStatus, error)
	RemoveCredential(provider string) (CredentialRemovalResult, error)
	SaveCustomProvider(CustomProviderSetup) ([]config.CustomProvider, error)
	RemoveCustomProvider(id string) ([]config.CustomProvider, error)
	DiscoverCustomProvider(id string) (CustomProviderDiscovery, error)
	ApplyCustomProviderDiscovery(id string, models []string) ([]config.CustomProvider, error)
}

// StoredCredentialStatus contains no credential material.
type StoredCredentialStatus struct {
	Provider string
	StoreKey string
	Present  bool
	Kind     string
	Expired  bool
}

// CredentialRemovalResult reports deletion and remaining ambient sources.
type CredentialRemovalResult struct {
	Provider         string
	StoreKey         string
	Removed          bool
	Kind             string
	RemainingSources []string
}

// CustomProviderSetup is non-secret custom-provider metadata. APIKeyEnv is an
// environment variable name, never a key value.
type CustomProviderSetup struct {
	ID           string
	BaseURL      string
	APIKeyEnv    string
	Models       []string
	DefaultModel string
}

// CustomProviderDiscovery is an untrusted /models preview.
type CustomProviderDiscovery struct {
	ID           string
	Models       []string
	DefaultModel string
}

// CloudModelSetup crosses the UI/command boundary only while a save is in
// progress. APIKey must never be rendered, drafted, or included in completion.
type CloudModelSetup struct {
	Provider        string
	Model           string
	BaseURL         string
	Options         map[string]string
	APIKey          string
	CredentialType  string
	DelegateRuntime string
}
