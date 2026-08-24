package model

import "strings"

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
	return newBackend(provider, config)
}
