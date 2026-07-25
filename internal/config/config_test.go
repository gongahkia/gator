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
