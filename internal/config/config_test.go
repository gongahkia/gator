package config

import (
	"github.com/gongahkia/norbot/internal/domain"
	"path/filepath"
	"testing"
)

func TestManifestRejectsDuplicateProvider(t *testing.T) {
	manifest := Manifest{Providers: []Provider{
		{ID: "one", Kind: "cli", Image: "agent:latest", Command: []string{"agent"}},
		{ID: "one", Kind: "cli", Image: "agent:latest", Command: []string{"agent"}},
	}, Profiles: []domain.Profile{domain.ProfileFullStack}}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected duplicate provider failure")
	}
}

func TestManifestProviderRespectsStage(t *testing.T) {
	manifest := Manifest{Providers: []Provider{{ID: "one", Kind: "cli", Image: "agent:latest", Command: []string{"agent"}, Stages: []domain.Stage{domain.StagePlanner}}}}
	if _, ok := manifest.Provider("one", domain.StagePlanner); !ok {
		t.Fatal("provider should support planner")
	}
	if _, ok := manifest.Provider("one", domain.StageBuilder); ok {
		t.Fatal("provider should not support builder")
	}
}

func TestManifestValidatesNativeProviderRequirements(t *testing.T) {
	manifest := Manifest{Providers: []Provider{{ID: "azure", Kind: "azure_openai_responses", Model: "deployment", BaseURL: "https://resource.openai.azure.com/openai/v1", CredentialEnv: "AZURE_OPENAI_KEY", Stages: []domain.Stage{domain.StagePlanner}}, {ID: "ollama", Kind: "ollama_chat", Model: "qwen", BaseURL: "http://host.docker.internal:11434", Stages: []domain.Stage{domain.StageBuilder}}, {ID: "bedrock", Kind: "aws_bedrock_converse", Model: "amazon.nova-lite-v1:0", Region: "us-east-1", Stages: []domain.Stage{domain.StageVerifier}}, {ID: "vertex", Kind: "vertex_ai_generate_content", Model: "gemini", BaseURL: "https://us-central1-aiplatform.googleapis.com", Project: "project", Region: "us-central1", Stages: []domain.Stage{domain.StagePlanner}}}, Profiles: []domain.Profile{domain.ProfileFullStack}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid native providers rejected: %v", err)
	}
	manifest.Providers[2].Region = ""
	if err := manifest.Validate(); err == nil {
		t.Fatal("bedrock region is required")
	}
	manifest.Providers[2].Region = "us-east-1"
	manifest.Providers[3].Project = ""
	if err := manifest.Validate(); err == nil {
		t.Fatal("vertex project is required")
	}
}

func TestManifestRejectsUnknownProviderKind(t *testing.T) {
	manifest := Manifest{Providers: []Provider{{ID: "unknown", Kind: "unsupported", Model: "model", BaseURL: "https://example.com", CredentialEnv: "KEY", Stages: []domain.Stage{domain.StagePlanner}}}, Profiles: []domain.Profile{domain.ProfileFullStack}}
	if err := manifest.Validate(); err == nil {
		t.Fatal("unknown provider kind accepted")
	}
}

func TestManifestAllowsUnauthenticatedCompatibleEndpoint(t *testing.T) {
	manifest := Manifest{Providers: []Provider{{ID: "local", Kind: "openai_compatible", Model: "local-model", BaseURL: "http://host.docker.internal:1234/v1", Stages: []domain.Stage{domain.StagePlanner}}}, Profiles: []domain.Profile{domain.ProfileFullStack}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("unauthenticated compatible endpoint rejected: %v", err)
	}
}

func TestManifestRejectsUnknownPluginProvider(t *testing.T) {
	manifest := Manifest{Providers: []Provider{{ID: "external", Kind: "plugin", PluginID: "missing", Stages: []domain.Stage{domain.StagePlanner}}}, Profiles: []domain.Profile{domain.ProfileFullStack}}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected unknown plugin error")
	}
}

func TestKubernetesRuntimeValidationAndInitialWrite(t *testing.T) {
	kube := Kubernetes{Kubeconfig: "/tmp/kubeconfig", Namespace: "norbot", ServiceAccount: "norbot-runtime", RegistryRepository: "registry.example/norbot", RegistryPullSecret: "registry-pull"}
	manifest := InitialManifest(domain.DeploymentKubernetes, kube)
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := WriteManifest(path, manifest, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(path, manifest, false); err == nil {
		t.Fatal("overwrite must require force")
	}
}

func TestKubernetesIngressConfigurationIsAllOrNothing(t *testing.T) {
	manifest := InitialManifest(domain.DeploymentKubernetes, Kubernetes{Kubeconfig: "/tmp/kubeconfig", Namespace: "norbot", ServiceAccount: "norbot-runtime", RegistryRepository: "registry.example/norbot", RegistryPullSecret: "registry-pull", IngressClass: "nginx"})
	if err := manifest.Validate(); err == nil {
		t.Fatal("partial ingress configuration accepted")
	}
}

func TestKubernetesEgressCanRemainConfiguredButFailClosed(t *testing.T) {
	kube := Kubernetes{Kubeconfig: "/tmp/kubeconfig", Namespace: "norbot", ServiceAccount: "norbot-runtime", RegistryRepository: "registry.example/norbot", RegistryPullSecret: "registry-pull", EgressProxyImage: "registry.example/proxy@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", EgressProxySecret: "proxy", EgressProxySecretKey: "secret", EgressProxyPort: 8181}
	manifest := InitialManifest(domain.DeploymentKubernetes, kube)
	manifest.Runtime.Sandbox = Sandbox{EgressProxyURL: "http://proxy:8181", EgressProxySecret: "NORBOT_EGRESS_PROXY_SECRET"}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("unverified egress configuration must load fail-closed: %v", err)
	}
}

func TestPublicSecurityRequiresCompleteIngressControls(t *testing.T) {
	manifest := InitialManifest(domain.DeploymentDocker, Kubernetes{})
	manifest.Security = Security{Public: true, HTTP: HTTPPolicy{RequireHTTPS: true, MetricsTokenEnv: "NORBOT_METRICS_TOKEN", RatePerMinute: 120, RateBurst: 30}}
	if err := manifest.Validate(); err == nil {
		t.Fatal("public profile without OIDC validated")
	}
	manifest.Security.OIDC = OIDC{Issuer: "https://issuer.example", Audience: "norbot", GroupsClaim: "groups", OperatorGroups: []string{"operators"}, ClientID: "norbot"}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("complete public profile rejected: %v", err)
	}
}

func TestManifestRejectsPublicOrOIDCForensics(t *testing.T) {
	manifest := InitialManifest(domain.DeploymentDocker, Kubernetes{})
	manifest.Forensics = Forensics{RawCapture: true, MasterKeyEnv: "NORBOT_FORENSICS_KEY"}
	manifest.Security.Public = true
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected public forensic rejection")
	}
	manifest.Security.Public = false
	manifest.Security.OIDC = OIDC{Issuer: "https://issuer.example", Audience: "norbot", GroupsClaim: "groups", OperatorGroups: []string{"operators"}, ClientID: "norbot"}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected oidc forensic rejection")
	}
}

func TestPlanningSwarmBounds(t *testing.T) {
	manifest := InitialManifest(domain.DeploymentDocker, Kubernetes{})
	manifest.Workflow.PlanningSwarm = PlanningSwarm{Enabled: true, MaxParallel: 4, TimeoutS: 180}
	if err := manifest.Validate(); err == nil {
		t.Fatal("accepted more than three swarm candidates")
	}
	manifest.Workflow.PlanningSwarm = PlanningSwarm{Enabled: true, MaxParallel: 3, TimeoutS: 29}
	if err := manifest.Validate(); err == nil {
		t.Fatal("accepted an unbounded swarm timeout")
	}
	manifest.Workflow.PlanningSwarm = PlanningSwarm{Enabled: true, MaxParallel: 3, TimeoutS: 180}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid swarm configuration rejected: %v", err)
	}
}
