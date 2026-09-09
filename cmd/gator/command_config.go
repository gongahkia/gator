package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/sandbox"
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
		return errors.New("usage: gator config set default-provider PROVIDER | gator config set default-model MODEL | gator config set sandbox strict|off | gator config set network deny|allow")
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
	case "sandbox":
		settings.Execution.Mode = sandbox.Mode(value)
		if err := settings.Execution.Validate(); err != nil {
			return err
		}
	case "network":
		settings.Execution.Network = sandbox.Network(value)
		if err := settings.Execution.Validate(); err != nil {
			return err
		}
	case "snapshot-max-files":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return errors.New("snapshot-max-files requires an integer")
		}
		settings.Snapshots.MaxFiles = parsed
	case "snapshot-max-total-mib":
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 {
			return errors.New("snapshot-max-total-mib requires a positive integer")
		}
		settings.Snapshots.MaxTotalBytes = parsed * 1024 * 1024
	case "snapshot-max-file-mib":
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 {
			return errors.New("snapshot-max-file-mib requires a positive integer")
		}
		settings.Snapshots.MaxFileBytes = parsed * 1024 * 1024
	case "snapshot-exclude":
		settings.Snapshots.Excludes = append(settings.Snapshots.Excludes, value)
	case "desktop-notifications":
		if value != "on" && value != "off" {
			return errors.New("desktop-notifications must be on or off")
		}
		settings.Notifications.Desktop = value == "on"
	case "job-timezone":
		settings.JobDefaults.Timezone = value
	case "job-missed":
		settings.JobDefaults.Missed = value
	default:
		return fmt.Errorf("unknown configuration key %q", arguments[1])
	}
	if err := store.Save(settings); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Updated %s in %s.\n", arguments[1], store.Path())
	return err
}
