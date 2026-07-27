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
	Enabled             bool     `json:"enabled"`
	ApprovalRequired    bool     `json:"approval_required"`
	Roles               []string `json:"roles"`
	AllowedHosts        []string `json:"allowed_hosts"`
	AllowedCommands     []string `json:"allowed_commands"`
	AllowedPathPrefixes []string `json:"allowed_path_prefixes"`
	MaxCalls            int      `json:"max_calls"`
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
	Image             string `json:"image"`
	CPUMilli          int64  `json:"cpu_milli"`
	MemoryMiB         int64  `json:"memory_mib"`
	TimeoutS          int    `json:"timeout_seconds"`
	EgressProxyURL    string `json:"egress_proxy_url"`
	EgressProxySecret string `json:"egress_proxy_secret_env"`
}

type OIDC struct {
	Issuer         string   `json:"issuer"`
	Audience       string   `json:"audience"`
	GroupsClaim    string   `json:"groups_claim"`
	OperatorGroups []string `json:"operator_groups"`
	ClientID       string   `json:"client_id"`
	Scopes         []string `json:"scopes"`
}

type Security struct {
	OIDC   OIDC       `json:"oidc"`
	Public bool       `json:"public"`
	HTTP   HTTPPolicy `json:"http"`
}

type HTTPPolicy struct {
	RequireHTTPS    bool   `json:"require_https"`
	RatePerMinute   int    `json:"rate_per_minute"`
	RateBurst       int    `json:"rate_burst"`
	MetricsTokenEnv string `json:"metrics_token_env"`
}

type ArtifactStore struct {
	Enabled        bool   `json:"enabled"`
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	Bucket         string `json:"bucket"`
	AccessKeyEnv   string `json:"access_key_env"`
	SecretKeyEnv   string `json:"secret_key_env"`
	ForcePathStyle bool   `json:"force_path_style"`
}

// Retention is opt-in; zero keeps the corresponding records until an operator deletes them.
type Retention struct {
	AgentTurnsDays      int `json:"agent_turns_days"`
	ChannelMessagesDays int `json:"channel_messages_days"`
	RunEventsDays       int `json:"run_events_days"`
	ProviderUsageDays   int `json:"provider_usage_days"`
	TraceEventsDays     int `json:"trace_events_days"`
	ForensicPayloadDays int `json:"forensic_payload_days"`
}

type Forensics struct {
	RawCapture   bool   `json:"raw_capture"`
	MasterKeyEnv string `json:"master_key_env"`
}

type Workflow struct {
	MaxFixes      int           `json:"max_fixes"`
	PlanningSwarm PlanningSwarm `json:"planning_swarm"`
}

type PlanningSwarm struct {
	Enabled     bool `json:"enabled"`
	MaxParallel int  `json:"max_parallel"`
	TimeoutS    int  `json:"timeout_seconds"`
}

func (p PlanningSwarm) Parallelism() int {
	if p.MaxParallel == 0 {
		return 3
	}
	return p.MaxParallel
}

func (p PlanningSwarm) Timeout() int {
	if p.TimeoutS == 0 {
		return 180
	}
	return p.TimeoutS
}

type Kubernetes struct {
	Kubeconfig                 string `json:"kubeconfig"`
	Context                    string `json:"context"`
	Namespace                  string `json:"namespace"`
	ServiceAccount             string `json:"service_account"`
	RegistryRepository         string `json:"registry_repository"`
	RegistryPullSecret         string `json:"registry_pull_secret"`
	RegistryInsecure           bool   `json:"registry_insecure"`
	IngressClass               string `json:"ingress_class"`
	IngressBaseDomain          string `json:"ingress_base_domain"`
	IngressControllerNamespace string `json:"ingress_controller_namespace"`
	WorkspaceImage             string `json:"workspace_image"`
	KanikoImage                string `json:"kaniko_image"`
	VerifierImage              string `json:"verifier_image"`
	CPUMilli                   int64  `json:"cpu_milli"`
	MemoryMiB                  int64  `json:"memory_mib"`
	Replicas                   int32  `json:"replicas"`
	EgressProxyImage           string `json:"egress_proxy_image"`
	EgressProxySecret          string `json:"egress_proxy_secret"`
	EgressProxySecretKey       string `json:"egress_proxy_secret_key"`
	EgressProxyPort            int32  `json:"egress_proxy_port"`
	NetworkPolicyEnforced      bool   `json:"network_policy_enforced"`
	QuotaCPUMilli              int64  `json:"quota_cpu_milli"`
	QuotaMemoryMiB             int64  `json:"quota_memory_mib"`
	QuotaStorageGiB            int64  `json:"quota_storage_gib"`
	QuotaPods                  int64  `json:"quota_pods"`
	QuotaJobs                  int64  `json:"quota_jobs"`
	QuotaPVCs                  int64  `json:"quota_pvcs"`
}

type Manifest struct {
	Providers  []Provider            `json:"providers"`
	Profiles   []domain.Profile      `json:"profiles"`
	ToolPolicy map[string]ToolPolicy `json:"tool_policy"`
	Plugins    []ProcessPlugin       `json:"plugins"`
	Runtime    Runtime               `json:"runtime"`
	Workflow   Workflow              `json:"workflow"`
	Security   Security              `json:"security"`
	Artifacts  ArtifactStore         `json:"artifacts"`
	Retention  Retention             `json:"retention"`
	Forensics  Forensics             `json:"forensics"`
}

type Config struct {
	DatabaseURL               string
	HTTPAddr                  string
	ArtifactsDir              string
	OTelEndpoint              string
	Workers                   int
	MaxWorkers                int
	DockerBin                 string
	AllowUnauthenticatedLocal bool
	Manifest                  Manifest
}

func Load() (Config, error) {
	workers := envInt("NORBOT_WORKERS", 2)
	maxWorkers := envInt("NORBOT_MAX_WORKERS", 4)
	if workers < 1 || maxWorkers < workers {
		return Config{}, fmt.Errorf("NORBOT_WORKERS must be >=1 and <= NORBOT_MAX_WORKERS")
	}
	cfg := Config{
		DatabaseURL:               env("NORBOT_DATABASE_URL", "postgres://norbot:norbot@127.0.0.1:5432/norbot?sslmode=disable"),
		HTTPAddr:                  env("NORBOT_HTTP_ADDR", "127.0.0.1:8080"),
		ArtifactsDir:              env("NORBOT_ARTIFACTS_DIR", ".norbot/artifacts"),
		OTelEndpoint:              strings.TrimSpace(os.Getenv("NORBOT_OTEL_ENDPOINT")),
		Workers:                   workers,
		MaxWorkers:                maxWorkers,
		DockerBin:                 env("NORBOT_DOCKER_BIN", "docker"),
		AllowUnauthenticatedLocal: envBool("NORBOT_ALLOW_UNAUTHENTICATED_LOCAL", false),
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
	if cfg.Manifest.Security.Public {
		if strings.TrimSpace(os.Getenv(cfg.Manifest.Security.HTTP.MetricsTokenEnv)) == "" {
			return Config{}, fmt.Errorf("public security requires metrics token env %s", cfg.Manifest.Security.HTTP.MetricsTokenEnv)
		}
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
	if m.Workflow.PlanningSwarm.MaxParallel < 0 || m.Workflow.PlanningSwarm.MaxParallel > 3 {
		return fmt.Errorf("workflow planning_swarm max_parallel must be between 1 and 3")
	}
	if m.Workflow.PlanningSwarm.TimeoutS < 0 || m.Workflow.PlanningSwarm.TimeoutS > 600 || m.Workflow.PlanningSwarm.TimeoutS > 0 && m.Workflow.PlanningSwarm.TimeoutS < 30 {
		return fmt.Errorf("workflow planning_swarm timeout_seconds must be between 30 and 600")
	}
	if len(m.Providers) == 0 {
		return fmt.Errorf("manifest needs at least one provider")
	}
	if err := m.ValidateSecurity(); err != nil {
		return err
	}
	if err := m.ValidateArtifacts(); err != nil {
		return err
	}
	if err := m.ValidateRetention(); err != nil {
		return err
	}
	if err := m.ValidateForensics(); err != nil {
		return err
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
		if p.Kind == "cli" && p.Network != "" && p.Network != "none" && p.Network != "bridge" {
			return fmt.Errorf("cli provider %q network must be none or bridge", p.ID)
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
	for name, policy := range m.ToolPolicy {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("tool policy needs a name")
		}
		if policy.MaxCalls < 0 || policy.MaxCalls > 100 {
			return fmt.Errorf("tool policy %q max_calls must be between 0 and 100", name)
		}
		for _, prefix := range policy.AllowedPathPrefixes {
			if prefix == "" || strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "..") {
				return fmt.Errorf("tool policy %q has unsafe allowed_path_prefixes", name)
			}
		}
	}
	return m.ValidateRuntime()
}

func (m Manifest) ValidateRetention() error {
	for name, days := range map[string]int{
		"agent_turns_days": m.Retention.AgentTurnsDays, "channel_messages_days": m.Retention.ChannelMessagesDays,
		"run_events_days": m.Retention.RunEventsDays, "provider_usage_days": m.Retention.ProviderUsageDays,
		"trace_events_days": m.Retention.TraceEventsDays, "forensic_payload_days": m.Retention.ForensicPayloadDays,
	} {
		if days < 0 || days > 3650 {
			return fmt.Errorf("retention %s must be between 0 and 3650", name)
		}
	}
	return nil
}

func (m Manifest) ValidateForensics() error {
	if !m.Forensics.RawCapture {
		return nil
	}
	if m.Forensics.MasterKeyEnv == "" {
		return fmt.Errorf("forensics raw_capture requires master_key_env")
	}
	if m.Security.Public || m.Security.OIDC.Issuer != "" {
		return fmt.Errorf("forensics raw_capture requires local mode with public and oidc disabled")
	}
	return nil
}

func (m Manifest) ValidateSecurity() error {
	o := m.Security.OIDC
	h := m.Security.HTTP
	if o.Issuer == "" && o.Audience == "" && len(o.OperatorGroups) == 0 && o.GroupsClaim == "" && o.ClientID == "" && len(o.Scopes) == 0 {
		if !m.Security.Public {
			return nil
		}
	}
	if o.Issuer == "" || o.Audience == "" || len(o.OperatorGroups) == 0 {
		return fmt.Errorf("oidc issuer, audience, and operator_groups must be configured together")
	}
	if o.GroupsClaim == "" {
		return fmt.Errorf("oidc groups_claim is required when oidc is configured")
	}
	if o.ClientID == "" {
		return fmt.Errorf("oidc client_id is required when oidc is configured")
	}
	if m.Security.Public {
		if !h.RequireHTTPS || h.MetricsTokenEnv == "" || h.RatePerMinute < 1 || h.RateBurst < 1 {
			return fmt.Errorf("public security requires https, metrics_token_env, positive rate_per_minute, and positive rate_burst")
		}
		if !validEnvName(h.MetricsTokenEnv) {
			return fmt.Errorf("metrics_token_env is invalid")
		}
	}
	return nil
}

func (m Manifest) ValidateArtifacts() error {
	a := m.Artifacts
	if !a.Enabled {
		return nil
	}
	if a.Endpoint == "" || a.Bucket == "" || a.AccessKeyEnv == "" || a.SecretKeyEnv == "" {
		return fmt.Errorf("artifact endpoint, bucket, access_key_env, and secret_key_env must be configured together")
	}
	if !strings.HasPrefix(a.Endpoint, "https://") && !strings.HasPrefix(a.Endpoint, "http://127.0.0.1") && !strings.HasPrefix(a.Endpoint, "http://localhost") {
		return fmt.Errorf("artifact endpoint must use https outside localhost")
	}
	return nil
}

func (m Manifest) ValidateRuntime() error {
	if s := m.Runtime.Sandbox; s.CPUMilli < 0 || s.MemoryMiB < 0 || s.TimeoutS < 0 {
		return fmt.Errorf("sandbox resources cannot be negative")
	}
	if s := m.Runtime.Sandbox; (s.EgressProxyURL == "") != (s.EgressProxySecret == "") {
		return fmt.Errorf("sandbox egress_proxy_url and egress_proxy_secret_env must be configured together")
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
	s := m.Runtime.Sandbox
	if k.Kubeconfig == "" || k.Namespace == "" || k.ServiceAccount == "" || k.RegistryRepository == "" || k.RegistryPullSecret == "" {
		return fmt.Errorf("kubernetes runtime needs kubeconfig, namespace, service_account, registry_repository, and registry_pull_secret")
	}
	if (k.IngressClass == "") != (k.IngressBaseDomain == "") || (k.IngressClass != "" && k.IngressControllerNamespace == "") {
		return fmt.Errorf("kubernetes ingress_class, ingress_base_domain, and ingress_controller_namespace must be configured together")
	}
	if k.CPUMilli < 0 || k.MemoryMiB < 0 || k.Replicas < 0 || k.QuotaCPUMilli < 0 || k.QuotaMemoryMiB < 0 || k.QuotaStorageGiB < 0 || k.QuotaPods < 0 || k.QuotaJobs < 0 || k.QuotaPVCs < 0 {
		return fmt.Errorf("kubernetes resources cannot be negative")
	}
	proxyConfigured := k.EgressProxyImage != "" || k.EgressProxySecret != "" || k.EgressProxySecretKey != "" || k.EgressProxyPort != 0
	if proxyConfigured {
		if s.EgressProxyURL == "" || s.EgressProxySecret == "" || k.EgressProxyImage == "" || k.EgressProxySecret == "" || k.EgressProxySecretKey == "" {
			return fmt.Errorf("kubernetes egress proxy needs sandbox proxy configuration, image, secret, and secret key")
		}
		if k.EgressProxyPort < 0 || k.EgressProxyPort > 65535 {
			return fmt.Errorf("kubernetes egress proxy port is invalid")
		}
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
	if k.EgressProxyPort == 0 {
		k.EgressProxyPort = 8181
	}
	if k.QuotaCPUMilli == 0 {
		k.QuotaCPUMilli = k.CPUMilli * 20
	}
	if k.QuotaMemoryMiB == 0 {
		k.QuotaMemoryMiB = k.MemoryMiB * 20
	}
	if k.QuotaStorageGiB == 0 {
		k.QuotaStorageGiB = 40
	}
	if k.QuotaPods == 0 {
		k.QuotaPods = 50
	}
	if k.QuotaJobs == 0 {
		k.QuotaJobs = 30
	}
	if k.QuotaPVCs == 0 {
		k.QuotaPVCs = 30
	}
	return k
}

func InitialManifest(target domain.DeploymentTarget, kube Kubernetes) Manifest {
	if target == "" {
		target = domain.DeploymentDocker
	}
	return Manifest{Providers: []Provider{{ID: "openai", Kind: "openai_responses", Model: "gpt-5", BaseURL: "https://api.openai.com/v1", CredentialEnv: "OPENAI_API_KEY", Stages: []domain.Stage{domain.StagePlanner, domain.StageBuilder, domain.StageVerifier}, Budget: ProviderBudget{MaxConcurrent: 2, RequestsPerMinute: 60}}}, Profiles: []domain.Profile{domain.ProfileFrontend, domain.ProfileFullStack, domain.ProfileAgentic}, ToolPolicy: map[string]ToolPolicy{}, Plugins: []ProcessPlugin{}, Runtime: Runtime{DefaultTarget: target, Kubernetes: kube}, Workflow: Workflow{MaxFixes: 2}, Retention: Retention{AgentTurnsDays: 90, ChannelMessagesDays: 90, RunEventsDays: 365, ProviderUsageDays: 365, TraceEventsDays: 365, ForensicPayloadDays: 30}}
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

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func validEnvName(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if !(char == '_' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}
