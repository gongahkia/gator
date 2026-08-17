package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/config"
)

func configure(arguments []string, out io.Writer) error {
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	if len(arguments) == 0 || arguments[0] == "show" {
		if len(arguments) > 1 {
			return errors.New("usage: gator config [show]")
		}
		settings, err := store.Load()
		if err != nil {
			return err
		}
		payload, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return fmt.Errorf("encode configuration: %w", err)
		}
		_, err = fmt.Fprintf(out, "Configuration: %s\n%s\n", store.Path(), payload)
		return err
	}
	if len(arguments) != 3 || arguments[0] != "set" {
		return errors.New("usage: gator config set default-provider PROVIDER | gator config set default-model MODEL")
	}
	settings, err := store.Load()
	if err != nil {
		return err
	}
	value := strings.TrimSpace(arguments[2])
	if value == "" {
		return errors.New("configuration value is required")
	}
	switch arguments[1] {
	case "default-provider":
		provider, _, err := resolveConfiguredProvider(value, "")
		if err != nil {
			return err
		}
		settings.Defaults.Provider = provider
	case "default-model":
		settings.Defaults.Model = value
	default:
		return fmt.Errorf("unknown configuration key %q; choose default-provider or default-model", arguments[1])
	}
	if err := store.Save(settings); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Updated %s in %s.\n", arguments[1], store.Path())
	return err
}
