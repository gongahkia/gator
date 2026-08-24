package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
)

const localUsage = `usage:
  gator local list [--url LOOPBACK_URL]
  gator local status [--url LOOPBACK_URL]
  gator local serve
  gator local pull MODEL --yes [--url LOOPBACK_URL]
  gator local use MODEL [--url LOOPBACK_URL]
  gator local remove MODEL --yes [--url LOOPBACK_URL]

MODEL is a Gator catalog ID or exact curated Ollama tag. Run 'gator local list'
to see the catalog and installed state.`

// localCommand manages Gator's reviewed local coding models. It invokes only a
// loopback Ollama API and adds a normal Gator custom provider on `use`, so the
// same native loop, tools, worktrees, TUI, RPC, ACP, and extensions apply.
func localCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(localUsage)
	}
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	settings, err := store.Load()
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "list":
		return listLocalModels(arguments[1:], settings, out)
	case "status":
		return localStatus(arguments[1:], settings, out)
	case "serve":
		if len(arguments) != 1 {
			return errors.New(localUsage)
		}
		return serveLocalRuntime()
	case "pull":
		return pullLocalModel(arguments[1:], settings, out)
	case "use":
		return useLocalModel(arguments[1:], store, settings, out)
	case "remove":
		return removeLocalModel(arguments[1:], store, settings, out)
	default:
		return fmt.Errorf("unknown local command %q\n%s", arguments[0], localUsage)
	}
}

func listLocalModels(arguments []string, settings config.Settings, out io.Writer) error {
	client, err := localClientFromFlags("local list", arguments, settings)
	if err != nil {
		return err
	}
	context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	installed, err := client.Models(context)
	if err != nil {
		if _, writeErr := fmt.Fprintf(out, "Local runtime: unavailable (%v)\n\n", err); writeErr != nil {
			return writeErr
		}
		return writeLocalCatalog(out, nil, inspectLocalModelHost())
	}
	return writeLocalCatalog(out, installedModelNames(installed), inspectLocalModelHost())
}

func localStatus(arguments []string, settings config.Settings, out io.Writer) error {
	client, err := localClientFromFlags("local status", arguments, settings)
	if err != nil {
		return err
	}
	if err := writeLocalModelDoctor(out, inspectLocalModelHost()); err != nil {
		return err
	}
	if binary, lookupErr := exec.LookPath("ollama"); lookupErr == nil {
		if _, err := fmt.Fprintf(out, "Ollama executable: %s\n", binary); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintln(out, "Ollama executable: not found (install Ollama from https://ollama.com/download)"); err != nil {
		return err
	}
	context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	version, versionErr := client.Version(context)
	if versionErr != nil {
		_, err := fmt.Fprintf(out, "Local runtime (%s): unavailable (%v)\nStart it with 'ollama serve' or 'gator local serve'.\n", client.BaseURL(), versionErr)
		return err
	}
	installed, modelErr := client.Models(context)
	if modelErr != nil {
		_, err := fmt.Fprintf(out, "Local runtime (%s): Ollama %s\nModel inventory: unavailable (%v)\n", client.BaseURL(), version, modelErr)
		return err
	}
	if _, err := fmt.Fprintf(out, "Local runtime (%s): Ollama %s\nInstalled curated models: %d\n", client.BaseURL(), version, countInstalledCatalogModels(installed)); err != nil {
		return err
	}
	if provider, found := configuredCustomProvider(settings, localmodel.ProviderID); found {
		_, err := fmt.Fprintf(out, "Gator local provider: configured (default %s)\n", provider.DefaultModel)
		return err
	}
	_, err = fmt.Fprintln(out, "Gator local provider: not selected (run 'gator local use MODEL')")
	return err
}

func serveLocalRuntime() error {
	binary, err := exec.LookPath("ollama")
	if err != nil {
		return errors.New("Ollama executable was not found; install it from https://ollama.com/download")
	}
	command := exec.Command(binary, "serve")
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func pullLocalModel(arguments []string, settings config.Settings, out io.Writer) error {
	model, client, confirmed, err := localModelCommand("local pull", arguments, settings, true)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("%s downloads approximately %s from %s; re-run with --yes to confirm", model.Name, model.Download, model.SourceURL)
	}
	if err := requireLocalModelEligibility(model); err != nil {
		return err
	}
	startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Version(startupContext); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Downloading %s (%s) from the reviewed Ollama catalog…\n", model.Name, model.Download); err != nil {
		return err
	}
	lastStatus := ""
	lastPercent := -1
	err = client.Pull(context.Background(), model, func(progress localmodel.Progress) {
		status := safeLocalDisplay(progress.Status)
		percent := -1
		if progress.Total > 0 && progress.Completed >= 0 {
			percent = int(progress.Completed * 100 / progress.Total)
		}
		if status == lastStatus && (percent < 0 || percent-lastPercent < 5) {
			return
		}
		lastStatus = status
		lastPercent = percent
		if percent >= 0 {
			_, _ = fmt.Fprintf(out, "  %s (%d%%)\n", status, percent)
			return
		}
		_, _ = fmt.Fprintf(out, "  %s\n", status)
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Installed %s. Run 'gator local use %s' to select it for Gator.\n", model.OllamaModel, model.ID)
	return err
}

func useLocalModel(arguments []string, store config.Store, settings config.Settings, out io.Writer) error {
	model, client, _, err := localModelCommand("local use", arguments, settings, false)
	if err != nil {
		return err
	}
	if err := requireLocalModelEligibility(model); err != nil {
		return err
	}
	context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	installed, err := client.Models(context)
	if err != nil {
		return err
	}
	if !hasInstalledModel(installed, model.OllamaModel) {
		return fmt.Errorf("%s is not installed; run 'gator local pull %s --yes' first", model.OllamaModel, model.ID)
	}
	configuredModels := installedCatalogModels(installed)
	settings.CustomProviders = setCustomProvider(settings.CustomProviders, config.CustomProvider{
		ID:           localmodel.ProviderID,
		BaseURL:      client.ChatCompletionsURL(),
		Models:       configuredModels,
		DefaultModel: model.OllamaModel,
	})
	settings.Defaults.Provider = localmodel.ProviderID
	settings.Defaults.Model = model.OllamaModel
	if err := store.Save(settings); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Selected %s for Gator. Native TUI, run, resume, fork, clone, RPC, ACP, tools, worktrees, and verification now use provider %q by default.\n", model.Name, localmodel.ProviderID)
	return err
}

func removeLocalModel(arguments []string, store config.Store, settings config.Settings, out io.Writer) error {
	model, client, confirmed, err := localModelCommand("local remove", arguments, settings, true)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("removing %s deletes its local model data; re-run with --yes to confirm", model.OllamaModel)
	}
	context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Version(context); err != nil {
		return err
	}
	if err := client.Remove(context, model); err != nil {
		return err
	}
	if localProvider, found := configuredCustomProvider(settings, localmodel.ProviderID); found {
		localProvider.Models = removeLocalProviderModel(localProvider.Models, model.OllamaModel)
		if len(localProvider.Models) == 0 {
			settings.CustomProviders = removeCustomProvider(settings.CustomProviders, localmodel.ProviderID)
			if settings.Defaults.Provider == localmodel.ProviderID {
				settings.Defaults.Provider = ""
				settings.Defaults.Model = ""
			}
		} else {
			if localProvider.DefaultModel == model.OllamaModel {
				localProvider.DefaultModel = localProvider.Models[0]
			}
			settings.CustomProviders = setCustomProvider(settings.CustomProviders, localProvider)
			if settings.Defaults.Provider == localmodel.ProviderID && settings.Defaults.Model == model.OllamaModel {
				settings.Defaults.Model = localProvider.DefaultModel
			}
		}
		if err := store.Save(settings); err != nil {
			return fmt.Errorf("removed %s but could not update Gator's local provider configuration: %w", model.OllamaModel, err)
		}
	}
	_, err = fmt.Fprintf(out, "Removed local model %s.\n", model.OllamaModel)
	return err
}

func localModelCommand(name string, arguments []string, settings config.Settings, destructive bool) (localmodel.Model, localmodel.Client, bool, error) {
	if len(arguments) == 0 {
		return localmodel.Model{}, localmodel.Client{}, false, errors.New(localUsage)
	}
	model, found := localmodel.Resolve(arguments[0])
	if !found {
		return localmodel.Model{}, localmodel.Client{}, false, fmt.Errorf("%q is not in Gator's curated local-model catalog; run 'gator local list'", arguments[0])
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runtimeURL := flags.String("url", "", "loopback Ollama runtime URL")
	var confirmation *bool
	if destructive {
		confirmation = flags.Bool("yes", false, "confirm model download or deletion")
	}
	if err := flags.Parse(arguments[1:]); err != nil {
		return localmodel.Model{}, localmodel.Client{}, false, err
	}
	if len(flags.Args()) != 0 {
		return localmodel.Model{}, localmodel.Client{}, false, errors.New(localUsage)
	}
	client, err := localClient(*runtimeURL, settings)
	if err != nil {
		return localmodel.Model{}, localmodel.Client{}, false, err
	}
	confirmed := confirmation != nil && *confirmation
	return model, client, confirmed, nil
}

func localClientFromFlags(name string, arguments []string, settings config.Settings) (localmodel.Client, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runtimeURL := flags.String("url", "", "loopback Ollama runtime URL")
	if err := flags.Parse(arguments); err != nil {
		return localmodel.Client{}, err
	}
	if len(flags.Args()) != 0 {
		return localmodel.Client{}, errors.New(localUsage)
	}
	return localClient(*runtimeURL, settings)
}

func localClient(override string, settings config.Settings) (localmodel.Client, error) {
	runtimeURL := strings.TrimSpace(override)
	if runtimeURL == "" {
		if provider, found := configuredCustomProvider(settings, localmodel.ProviderID); found {
			var err error
			runtimeURL, err = localRuntimeURL(provider.BaseURL)
			if err != nil {
				return localmodel.Client{}, fmt.Errorf("configured local provider: %w", err)
			}
		}
	}
	return localmodel.NewClient(runtimeURL)
}

func localRuntimeURL(chatCompletionsURL string) (string, error) {
	parsed, err := url.Parse(chatCompletionsURL)
	if err != nil {
		return "", err
	}
	const suffix = "/v1/chat/completions"
	if !strings.HasSuffix(parsed.Path, suffix) {
		return "", errors.New("base URL must end in /v1/chat/completions")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, suffix)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func writeLocalCatalog(out io.Writer, installed map[string]struct{}, host localmodel.Host) error {
	if _, err := fmt.Fprintln(out, "Curated local coding models:"); err != nil {
		return err
	}
	for _, model := range localmodel.Catalog() {
		state := "not installed"
		if _, found := installed[model.OllamaModel]; found {
			state = "installed"
		}
		eligibility := localmodel.Assess(model, host)
		availability := "enabled"
		if !eligibility.Allowed {
			availability = "disabled: " + eligibility.Reason
		}
		if _, err := fmt.Fprintf(out, "  %s  %s  %s  %s context  %s\n    %s\n    %s\n    %s · %s\n", model.ID, state, model.Download, model.Context, model.Name, model.Summary, model.SourceURL, availability, localModelRequirement(eligibility)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "Model weights and terms remain governed by each linked upstream source. Gator's eligibility policy is conservative and does not predict performance.")
	return err
}

func installedModelNames(models []localmodel.InstalledModel) map[string]struct{} {
	names := make(map[string]struct{}, len(models))
	for _, model := range models {
		names[model.Name] = struct{}{}
	}
	return names
}

func hasInstalledModel(models []localmodel.InstalledModel, name string) bool {
	_, found := installedModelNames(models)[name]
	return found
}

func installedCatalogModels(installed []localmodel.InstalledModel) []string {
	names := installedModelNames(installed)
	models := make([]string, 0, len(names))
	for _, model := range localmodel.Catalog() {
		if _, found := names[model.OllamaModel]; found {
			models = append(models, model.OllamaModel)
		}
	}
	return models
}

func countInstalledCatalogModels(installed []localmodel.InstalledModel) int {
	return len(installedCatalogModels(installed))
}

func removeLocalProviderModel(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func safeLocalDisplay(value string) string {
	value = strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return -1
		}
		return character
	}, strings.TrimSpace(value))
	if len(value) > 160 {
		return value[:160] + "…"
	}
	return value
}
