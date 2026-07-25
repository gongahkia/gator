package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gongahkia/norbot/internal/domain"
)

type Provider struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	Model         string         `json:"model"`
	BaseURL       string         `json:"base_url"`
	CredentialEnv string         `json:"credential_env"`
	Command       []string       `json:"command"`
	Image         string         `json:"image"`
	Network       string         `json:"network"`
	Stages        []domain.Stage `json:"stages"`
	Budget        ProviderBudget `json:"budget"`
}

type ProviderBudget struct {
	MaxConcurrent     int `json:"max_concurrent"`
	RequestsPerMinute int `json:"requests_per_minute"`
}

type ToolPolicy struct {
	Enabled          bool     `json:"enabled"`
	ApprovalRequired bool     `json:"approval_required"`
	Roles            []string `json:"roles"`
	AllowedHosts     []string `json:"allowed_hosts"`
	AllowedCommands  []string `json:"allowed_commands"`
}

type ProcessPlugin struct {
	ID      string   `json:"id"`
	Command string   `json:"command"`
	SHA256  string   `json:"sha256"`
	Methods []string `json:"methods"`
}

type Manifest struct {
	Providers  []Provider            `json:"providers"`
	Profiles   []domain.Profile      `json:"profiles"`
	ToolPolicy map[string]ToolPolicy `json:"tool_policy"`
	Plugins    []ProcessPlugin       `json:"plugins"`
}

type Config struct {
	DatabaseURL  string
	HTTPAddr     string
	ArtifactsDir string
	OTelEndpoint string
	Workers      int
	MaxWorkers   int
	DockerBin    string
	Manifest     Manifest
}

func Load() (Config, error) {
	workers := envInt("NORBOT_WORKERS", 2)
	maxWorkers := envInt("NORBOT_MAX_WORKERS", 4)
	if workers < 1 || maxWorkers < workers {
		return Config{}, fmt.Errorf("NORBOT_WORKERS must be >=1 and <= NORBOT_MAX_WORKERS")
	}
	cfg := Config{
		DatabaseURL:  env("NORBOT_DATABASE_URL", "postgres://norbot:norbot@127.0.0.1:5432/norbot?sslmode=disable"),
		HTTPAddr:     env("NORBOT_HTTP_ADDR", "127.0.0.1:8080"),
		ArtifactsDir: env("NORBOT_ARTIFACTS_DIR", ".norbot/artifacts"),
		OTelEndpoint: strings.TrimSpace(os.Getenv("NORBOT_OTEL_ENDPOINT")),
		Workers:      workers,
		MaxWorkers:   maxWorkers,
		DockerBin:    env("NORBOT_DOCKER_BIN", "docker"),
	}
	path := env("NORBOT_CONFIG", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read NORBOT_CONFIG: %w", err)
	}
	if err := json.Unmarshal(data, &cfg.Manifest); err != nil {
		return Config{}, fmt.Errorf("parse NORBOT_CONFIG: %w", err)
	}
	if err := cfg.Manifest.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (m Manifest) Validate() error {
	if len(m.Providers) == 0 {
		return fmt.Errorf("manifest needs at least one provider")
	}
	seen := map[string]struct{}{}
	for _, p := range m.Providers {
		if p.ID == "" || p.Kind == "" {
			return fmt.Errorf("provider requires id and kind")
		}
		if _, ok := seen[p.ID]; ok {
			return fmt.Errorf("duplicate provider id %q", p.ID)
		}
		seen[p.ID] = struct{}{}
		if p.Kind == "cli" && (len(p.Command) == 0 || p.Image == "") {
			return fmt.Errorf("cli provider %q needs command and image", p.ID)
		}
		if p.Kind != "cli" && (p.BaseURL == "" || p.Model == "" || p.CredentialEnv == "") {
			return fmt.Errorf("api provider %q needs base_url, model, and credential_env", p.ID)
		}
		if p.Budget.MaxConcurrent < 0 || p.Budget.RequestsPerMinute < 0 {
			return fmt.Errorf("provider %q has negative budget", p.ID)
		}
	}
	if len(m.Profiles) == 0 {
		return fmt.Errorf("manifest needs at least one profile")
	}
	for _, profile := range m.Profiles {
		if !profile.Valid() {
			return fmt.Errorf("unsupported profile %q", profile)
		}
	}
	pluginIDs := map[string]struct{}{}
	for _, plugin := range m.Plugins {
		if plugin.ID == "" || !filepath.IsAbs(plugin.Command) || len(plugin.Methods) == 0 {
			return fmt.Errorf("plugin requires id, absolute command, and methods")
		}
		if len(plugin.SHA256) != 64 {
			return fmt.Errorf("plugin %q requires a 64-character sha256 digest", plugin.ID)
		}
		if _, exists := pluginIDs[plugin.ID]; exists {
			return fmt.Errorf("duplicate plugin id %q", plugin.ID)
		}
		pluginIDs[plugin.ID] = struct{}{}
	}
	return nil
}

func (m Manifest) Provider(id string, stage domain.Stage) (Provider, bool) {
	for _, provider := range m.Providers {
		if provider.ID != id {
			continue
		}
		for _, permitted := range provider.Stages {
			if permitted == stage {
				return provider, true
			}
		}
	}
	return Provider{}, false
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
