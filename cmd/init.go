package cmd

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gongahkia/paw/internal/config"
	"github.com/spf13/cobra"
)

var (
	initNonInteractive bool
	initLocation       string
	initBrainTransport string
	initBrainModel     string
	initBrainBaseURL   string
	initBrainAPIKey    string
	initDroneTransport string
	initDroneModel     string
	initDroneBaseURL   string
	initDroneAPIKey    string
)

type initOptions struct {
	Location       string
	BrainTransport string
	BrainModel     string
	BrainBaseURL   string
	BrainAPIKey    string
	DroneTransport string
	DroneModel     string
	DroneBaseURL   string
	DroneAPIKey    string
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a paw config",
	Example: `  paw init
  paw init --location repo
  paw init --non-interactive --brain-transport ollama --drone-transport ollama`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		opts, err := collectInitOptions(cmd)
		if err != nil {
			return err
		}
		path, err := initConfigPath(opts.Location)
		if err != nil {
			return err
		}
		cfg := initConfig(opts)
		if err := cfg.Save(path); err != nil {
			return err
		}
		loaded, err := config.Load(path)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if _, err := fmt.Fprintf(out, "wrote %s\n", path); err != nil {
			return err
		}
		if err := runDoctorModels(cmd.Context(), out, loaded); err != nil {
			if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "doctor models: %v\n", err); writeErr != nil {
				return writeErr
			}
		}
		_, err = fmt.Fprintln(out, `next: paw run --instruction "fix the failing test"`)
		return err
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVar(&initNonInteractive, "non-interactive", false, "write config without prompts")
	initCmd.Flags().StringVar(&initLocation, "location", "", "config location: user, repo")
	initCmd.Flags().StringVar(&initBrainTransport, "brain-transport", "", "brain transport")
	initCmd.Flags().StringVar(&initBrainModel, "brain-model", "", "brain model")
	initCmd.Flags().StringVar(&initBrainBaseURL, "brain-base-url", "", "brain base URL")
	initCmd.Flags().StringVar(&initBrainAPIKey, "brain-api-key", "", "brain API key")
	initCmd.Flags().StringVar(&initDroneTransport, "drone-transport", "", "drone transport")
	initCmd.Flags().StringVar(&initDroneModel, "drone-model", "", "drone model")
	initCmd.Flags().StringVar(&initDroneBaseURL, "drone-base-url", "", "drone base URL")
	initCmd.Flags().StringVar(&initDroneAPIKey, "drone-api-key", "", "drone API key")
}

func collectInitOptions(cmd *cobra.Command) (initOptions, error) {
	opts := initOptions{
		Location:       initLocation,
		BrainTransport: initBrainTransport,
		BrainModel:     initBrainModel,
		BrainBaseURL:   initBrainBaseURL,
		BrainAPIKey:    initBrainAPIKey,
		DroneTransport: initDroneTransport,
		DroneModel:     initDroneModel,
		DroneBaseURL:   initDroneBaseURL,
		DroneAPIKey:    initDroneAPIKey,
	}
	if !initNonInteractive {
		if err := promptInitOptions(cmd.InOrStdin(), cmd.OutOrStdout(), &opts); err != nil {
			return initOptions{}, err
		}
	}
	completeInitOptions(&opts)
	return opts, validateInitOptions(opts)
}

func promptInitOptions(r io.Reader, w io.Writer, opts *initOptions) error {
	reader := bufio.NewReader(r)
	var err error
	opts.BrainTransport, err = ask(reader, w, "brain transport", defaultString(opts.BrainTransport, "ollama"))
	if err != nil {
		return err
	}
	opts.BrainModel, err = ask(reader, w, "brain model", defaultString(opts.BrainModel, defaultModel(opts.BrainTransport, "brain")))
	if err != nil {
		return err
	}
	opts.DroneTransport, err = ask(reader, w, "drone transport", defaultString(opts.DroneTransport, "ollama"))
	if err != nil {
		return err
	}
	opts.DroneModel, err = ask(reader, w, "drone model", defaultString(opts.DroneModel, defaultModel(opts.DroneTransport, "drone")))
	if err != nil {
		return err
	}
	if configPath == "" {
		opts.Location, err = ask(reader, w, "config location (user/repo)", defaultString(opts.Location, "user"))
	}
	return err
}

func ask(reader *bufio.Reader, w io.Writer, label string, fallback string) (string, error) {
	if fallback == "" {
		if _, err := fmt.Fprintf(w, "%s: ", label); err != nil {
			return "", err
		}
	} else if _, err := fmt.Fprintf(w, "%s [%s]: ", label, fallback); err != nil {
		return "", err
	}
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return fallback, nil
	}
	return line, nil
}

func completeInitOptions(opts *initOptions) {
	opts.BrainTransport = defaultString(opts.BrainTransport, "ollama")
	opts.DroneTransport = defaultString(opts.DroneTransport, "ollama")
	opts.BrainModel = defaultString(opts.BrainModel, defaultModel(opts.BrainTransport, "brain"))
	opts.DroneModel = defaultString(opts.DroneModel, defaultModel(opts.DroneTransport, "drone"))
	opts.BrainBaseURL = defaultString(opts.BrainBaseURL, defaultBaseURL(opts.BrainTransport))
	opts.DroneBaseURL = defaultString(opts.DroneBaseURL, defaultBaseURL(opts.DroneTransport))
	if configPath == "" {
		opts.Location = defaultString(opts.Location, "user")
	}
}

func validateInitOptions(opts initOptions) error {
	for _, transport := range []string{opts.BrainTransport, opts.DroneTransport} {
		if !supportedInitTransport(transport) {
			return usageErrorf("unsupported transport %q", transport)
		}
	}
	if configPath == "" && opts.Location != "user" && opts.Location != "repo" {
		return usageErrorf("unsupported config location %q", opts.Location)
	}
	return nil
}

func initConfigPath(location string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	switch location {
	case "user":
		return config.DefaultPath(), nil
	case "repo":
		return filepath.Join(".paw", "config.toml"), nil
	default:
		return "", usageErrorf("unsupported config location %q", location)
	}
}

func initConfig(opts initOptions) config.Config {
	cfg := config.Defaults()
	cfg.Brain = initEndpoint(opts.BrainTransport, opts.BrainModel, opts.BrainBaseURL, opts.BrainAPIKey)
	cfg.Drone = initEndpoint(opts.DroneTransport, opts.DroneModel, opts.DroneBaseURL, opts.DroneAPIKey)
	return cfg
}

func initEndpoint(transport string, model string, baseURL string, apiKey string) config.EndpointConfig {
	return config.EndpointConfig{
		Transport: transport,
		BaseURL:   baseURL,
		APIKey:    apiKey,
		Model:     model,
	}
}

func supportedInitTransport(transport string) bool {
	switch transport {
	case "ollama", "openai", "anthropic", "codex-cli", "gemini-cli", "claude-cli", "opencode-cli", "aider-cli", "goose-cli", "qwen-cli", "cursor-cli":
		return true
	default:
		return false
	}
}

func defaultModel(transport string, role string) string {
	switch transport {
	case "ollama":
		if role == "drone" {
			return config.Defaults().Drone.Model
		}
		return config.Defaults().Brain.Model
	case "openai":
		if role == "drone" {
			return "gpt-5.4-mini"
		}
		return "gpt-5.5"
	case "anthropic":
		if role == "drone" {
			return "claude-haiku-4-5"
		}
		return "claude-sonnet-5"
	default:
		return ""
	}
}

func defaultBaseURL(transport string) string {
	switch transport {
	case "ollama":
		return config.Defaults().Brain.BaseURL
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com"
	default:
		return ""
	}
}

func defaultString(current string, fallback string) string {
	if current != "" {
		return current
	}
	return fallback
}
