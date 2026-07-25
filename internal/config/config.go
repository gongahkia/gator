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
	ID                  string         `json:"id"`
	Kind                string         `json:"kind"`
	Model               string         `json:"model"`
	BaseURL             string         `json:"base_url"`
	CredentialEnv       string         `json:"credential_env"`
	Command             []string       `json:"command"`
	Image               string         `json:"image"`
	Network             string         `json:"network"`
	Stages              []domain.Stage `json:"stages"`
	Budget              ProviderBudget `json:"budget"`
	PluginID            string         `json:"plugin_id"`
	KubernetesSecret    string         `json:"kubernetes_secret"`
	KubernetesSecretKey string         `json:"kubernetes_secret_key"`
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

type Runtime struct {
	DefaultTarget domain.DeploymentTarget `json:"default_target"`
	Kubernetes    Kubernetes              `json:"kubernetes"`
	Sandbox       Sandbox                 `json:"sandbox"`
}

type Sandbox struct {
	Image     string `json:"image"`
	CPUMilli  int64  `json:"cpu_milli"`
	MemoryMiB int64  `json:"memory_mib"`
	TimeoutS  int    `json:"timeout_seconds"`
}

type Workflow struct {
	MaxFixes int `json:"max_fixes"`
}

type Kubernetes struct {
	Kubeconfig                 string `json:"kubeconfig"`
	Context                    string `json:"context"`
	Namespace                  string `json:"namespace"`
	ServiceAccount             string `json:"service_account"`
	RegistryRepository         string `json:"registry_repository"`
	RegistryPullSecret         string `json:"registry_pull_secret"`
	IngressClass               string `json:"ingress_class"`
	IngressBaseDomain          string `json:"ingress_base_domain"`
	IngressControllerNamespace string `json:"ingress_controller_namespace"`
	WorkspaceImage             string `json:"workspace_image"`
	KanikoImage                string `json:"kaniko_image"`
	VerifierImage              string `json:"verifier_image"`
	CPUMilli                   int64  `json:"cpu_milli"`
	MemoryMiB                  int64  `json:"memory_mib"`
	Replicas                   int32  `json:"replicas"`
}

type Manifest struct {
	Providers  []Provider            `json:"providers"`
	Profiles   []domain.Profile      `json:"profiles"`
	ToolPolicy map[string]ToolPolicy `json:"tool_policy"`
	Plugins    []ProcessPlugin       `json:"plugins"`
	Runtime    Runtime               `json:"runtime"`
	Workflow   Workflow              `json:"workflow"`
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
	if target := strings.TrimSpace(os.Getenv("NORBOT_DEPLOYMENT_TARGET")); target != "" {
		cfg.Manifest.Runtime.DefaultTarget = domain.DeploymentTarget(target)
		if err := cfg.Manifest.ValidateRuntime(); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func (m Manifest) Validate() error {
	if m.Workflow.MaxFixes < 0 || m.Workflow.MaxFixes > 10 {
		return fmt.Errorf("workflow max_fixes must be between 0 and 10")
	}
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
		if p.Kind == "plugin" && p.PluginID == "" {
			return fmt.Errorf("plugin provider %q needs plugin_id", p.ID)
		}
		if p.Kind == "cli" && (len(p.Command) == 0 || p.Image == "") {
			return fmt.Errorf("cli provider %q needs command and image", p.ID)
		}
		if p.Kind != "cli" && p.Kind != "plugin" && (p.BaseURL == "" || p.Model == "" || p.CredentialEnv == "") {
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
	for _, provider := range m.Providers {
		if provider.Kind == "plugin" {
			if _, ok := pluginIDs[provider.PluginID]; !ok {
				return fmt.Errorf("provider %q references unknown plugin %q", provider.ID, provider.PluginID)
			}
		}
	}
	return m.ValidateRuntime()
}

func (m Manifest) ValidateRuntime() error {
	if s := m.Runtime.Sandbox; s.CPUMilli < 0 || s.MemoryMiB < 0 || s.TimeoutS < 0 {
		return fmt.Errorf("sandbox resources cannot be negative")
	}
	target := m.Runtime.DefaultTarget
	if target == "" {
		target = domain.DeploymentDocker
	}
	if !target.Valid() {
		return fmt.Errorf("unsupported runtime default_target %q", target)
	}
	if target != domain.DeploymentKubernetes {
		return nil
	}
	k := m.Runtime.Kubernetes
	if k.Kubeconfig == "" || k.Namespace == "" || k.ServiceAccount == "" || k.RegistryRepository == "" || k.RegistryPullSecret == "" {
		return fmt.Errorf("kubernetes runtime needs kubeconfig, namespace, service_account, registry_repository, and registry_pull_secret")
	}
	if (k.IngressClass == "") != (k.IngressBaseDomain == "") || (k.IngressClass != "" && k.IngressControllerNamespace == "") {
		return fmt.Errorf("kubernetes ingress_class, ingress_base_domain, and ingress_controller_namespace must be configured together")
	}
	if k.CPUMilli < 0 || k.MemoryMiB < 0 || k.Replicas < 0 {
		return fmt.Errorf("kubernetes resources cannot be negative")
	}
	return nil
}

func (s Sandbox) Normalized() Sandbox {
	if s.Image == "" {
		s.Image = "alpine:3.21"
	}
	if s.CPUMilli == 0 {
		s.CPUMilli = 500
	}
	if s.MemoryMiB == 0 {
		s.MemoryMiB = 512
	}
	if s.TimeoutS == 0 {
		s.TimeoutS = 60
	}
	return s
}

func (m Manifest) DefaultTarget() domain.DeploymentTarget {
	if m.Runtime.DefaultTarget == "" {
		return domain.DeploymentDocker
	}
	return m.Runtime.DefaultTarget
}

func (k Kubernetes) Normalized() Kubernetes {
	if k.WorkspaceImage == "" {
		k.WorkspaceImage = "alpine:3.21"
	}
	if k.KanikoImage == "" {
		k.KanikoImage = "gcr.io/kaniko-project/executor:v1.23.2"
	}
	if k.VerifierImage == "" {
		k.VerifierImage = "golang:1.26-alpine"
	}
	if k.CPUMilli == 0 {
		k.CPUMilli = 500
	}
	if k.MemoryMiB == 0 {
		k.MemoryMiB = 512
	}
	if k.Replicas == 0 {
		k.Replicas = 1
	}
	return k
}

func InitialManifest(target domain.DeploymentTarget, kube Kubernetes) Manifest {
	if target == "" {
		target = domain.DeploymentDocker
	}
	return Manifest{Providers: []Provider{{ID: "openai", Kind: "openai_responses", Model: "gpt-5", BaseURL: "https://api.openai.com/v1", CredentialEnv: "OPENAI_API_KEY", Stages: []domain.Stage{domain.StagePlanner, domain.StageBuilder, domain.StageVerifier}, Budget: ProviderBudget{MaxConcurrent: 2, RequestsPerMinute: 60}}}, Profiles: []domain.Profile{domain.ProfileFrontend, domain.ProfileFullStack, domain.ProfileAgentic}, ToolPolicy: map[string]ToolPolicy{}, Plugins: []ProcessPlugin{}, Runtime: Runtime{DefaultTarget: target, Kubernetes: kube}, Workflow: Workflow{MaxFixes: 2}}
}

func WriteManifest(path string, manifest Manifest, force bool) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config %s already exists; use --force", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
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
