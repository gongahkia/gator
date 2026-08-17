package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/extension"
)

const extensionUsage = `usage:
  gator extension list
  gator extension install [--replace] DIRECTORY
  gator extension enable ID
  gator extension disable ID
  gator extension remove ID --yes
  gator extension trust
  gator extension untrust`

// extensionCommand owns the only installation and trust flow for executable
// extensions. Config editing is intentionally not a parallel interface.
func extensionCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(extensionUsage)
	}
	settingsStore, err := config.DefaultStore()
	if err != nil {
		return err
	}
	settings, err := settingsStore.Load()
	if err != nil {
		return err
	}
	extensionStore, err := extension.DefaultStore()
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "list":
		if len(arguments) != 1 {
			return errors.New(extensionUsage)
		}
		return listExtensions(extensionStore, settings, out)
	case "install":
		return installExtension(arguments[1:], extensionStore, settingsStore, settings, out)
	case "enable", "disable":
		if len(arguments) != 2 {
			return errors.New(extensionUsage)
		}
		return setExtensionEnabled(arguments[0], arguments[1], extensionStore, settingsStore, settings, out)
	case "remove":
		if len(arguments) != 3 || arguments[2] != "--yes" {
			return errors.New("removing an extension deletes its installed files; use: gator extension remove ID --yes")
		}
		if err := extensionStore.Remove(arguments[1]); err != nil {
			return err
		}
		settings.Extensions = removeExtensionSetting(settings.Extensions, arguments[1])
		if err := settingsStore.Save(settings); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "Removed extension %q.\n", arguments[1])
		return err
	case "trust", "untrust":
		if len(arguments) != 1 {
			return errors.New(extensionUsage)
		}
		return setProjectTrust(arguments[0] == "trust", settingsStore, settings, out)
	default:
		return fmt.Errorf("unknown extension command %q\n%s", arguments[0], extensionUsage)
	}
}

func listExtensions(store extension.Store, settings config.Settings, out io.Writer) error {
	installed, err := store.List()
	if err != nil {
		return err
	}
	enabled := extensionEnabled(settings.Extensions)
	if _, err := fmt.Fprintf(out, "Extensions: %s\n", store.Path()); err != nil {
		return err
	}
	if len(installed) == 0 {
		_, err := fmt.Fprintln(out, "No extensions installed.")
		return err
	}
	for _, candidate := range installed {
		state := "disabled"
		if enabled[candidate.Manifest.ID] {
			state = "enabled"
		}
		if _, err := fmt.Fprintf(out, "%s  %s  %s  skills:%d prompts:%d tools:%d\n", candidate.Manifest.ID, state, candidate.Manifest.Name, len(candidate.Manifest.Skills), len(candidate.Manifest.Prompts), len(candidate.Manifest.Tools)); err != nil {
			return err
		}
	}
	return nil
}

func installExtension(arguments []string, extensionStore extension.Store, settingsStore config.Store, settings config.Settings, out io.Writer) error {
	flags := flag.NewFlagSet("extension install", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	replace := flags.Bool("replace", false, "replace an installed extension")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 1 {
		return errors.New(extensionUsage)
	}
	context, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	installed, err := extensionStore.InstallSource(context, flags.Args()[0], *replace)
	if err != nil {
		return err
	}
	settings.Extensions = setExtensionSetting(settings.Extensions, installed.Manifest.ID, true)
	if err := settingsStore.Save(settings); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Installed and enabled extension %q. Installed extension tools run as code you chose to install.\n", installed.Manifest.ID)
	return err
}

func setExtensionEnabled(action, id string, extensionStore extension.Store, settingsStore config.Store, settings config.Settings, out io.Writer) error {
	installed, err := extensionStore.List()
	if err != nil {
		return err
	}
	found := false
	for _, candidate := range installed {
		if candidate.Manifest.ID == id {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("extension %q is not installed", id)
	}
	enable := action == "enable"
	settings.Extensions = setExtensionSetting(settings.Extensions, id, enable)
	if err := settingsStore.Save(settings); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s extension %q.\n", strings.Title(action), id)
	return err
}

func setProjectTrust(trust bool, settingsStore config.Store, settings config.Settings, out io.Writer) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	repository, err := gitRepositoryRoot(workingDirectory)
	if err != nil {
		return errors.New("project extensions can be trusted only from inside a Git checkout")
	}
	canonical, err := extension.CanonicalRepository(repository)
	if err != nil {
		return err
	}
	if trust {
		settings.TrustedRepositories = addString(settings.TrustedRepositories, canonical)
	} else {
		settings.TrustedRepositories = removeString(settings.TrustedRepositories, canonical)
	}
	if err := settingsStore.Save(settings); err != nil {
		return err
	}
	if trust {
		_, err = fmt.Fprintf(out, "Trusted project extensions in %s. They may provide declared prompts and executable tools on future native runs.\n", canonical)
	} else {
		_, err = fmt.Fprintf(out, "Stopped trusting project extensions in %s.\n", canonical)
	}
	return err
}

func extensionEnabled(values []config.Extension) map[string]bool {
	enabled := make(map[string]bool, len(values))
	for _, value := range values {
		enabled[value.ID] = value.Enabled
	}
	return enabled
}

func setExtensionSetting(values []config.Extension, id string, enabled bool) []config.Extension {
	for index := range values {
		if values[index].ID == id {
			values[index].Enabled = enabled
			return values
		}
	}
	values = append(values, config.Extension{ID: id, Enabled: enabled})
	sort.Slice(values, func(first, second int) bool { return values[first].ID < values[second].ID })
	return values
}

func removeExtensionSetting(values []config.Extension, id string) []config.Extension {
	result := values[:0]
	for _, value := range values {
		if value.ID != id {
			result = append(result, value)
		}
	}
	return result
}

func addString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	values = append(values, value)
	sort.Strings(values)
	return values
}

func removeString(values []string, value string) []string {
	result := values[:0]
	for _, existing := range values {
		if existing != value {
			result = append(result, existing)
		}
	}
	return result
}
