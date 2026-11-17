package model

import (
	"strings"

	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/gemini"
	"github.com/gongahkia/gator/internal/model/openai"
	"github.com/gongahkia/gator/internal/model/radius"
)

// New resolves one configured direct provider. It never launches a vendor CLI
// or reads another application's credential store.
func New(config Config) (Backend, error) {
	provider, err := ParseProvider(string(config.Provider))
	if err != nil {
		return Backend{}, err
	}
	config.Provider = provider
	if strings.TrimSpace(config.Model) == "" {
		config.Model = DefaultModel(provider)
	}
	backend, err := newBackend(provider, config)
	if err != nil {
		return Backend{}, err
	}
	switch adapter := backend.Model.(type) {
	case openai.Responses:
		adapter.ProviderID = string(provider)
		backend.Model = adapter
	case anthropic.Messages:
		adapter.ProviderID = string(provider)
		backend.Model = adapter
	case gemini.GenerateContent:
		adapter.ProviderID = string(provider)
		backend.Model = adapter
	case radius.Messages:
		adapter.ProviderID = string(provider)
		backend.Model = adapter
	case chatcompletions.Model:
		adapter.Config.ProviderID = string(provider)
		backend.Model = adapter
	}
	return backend, nil
}
