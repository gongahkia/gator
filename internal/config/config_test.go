package config

import (
	"strings"
	"testing"

	"github.com/gongahkia/norbot/internal/domain"
)

func TestLocalManifestDefaultsToDockerDesktopMode(t *testing.T) {
	manifest := InitialManifest(domain.DeploymentDocker, Kubernetes{})
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if manifest.Runtime.Docker.Normalized().Mode != DockerModeUnsafeLocalSocket {
		t.Fatal("local manifest did not select Docker Desktop mode")
	}
}

func TestLocalManifestRejectsRemoteAndPublicFeatures(t *testing.T) {
	for name, mutate := range map[string]func(*Manifest){
		"kubernetes": func(m *Manifest) { m.Runtime.DefaultTarget = domain.DeploymentKubernetes },
		"public":     func(m *Manifest) { m.Security.Public = true },
		"oidc":       func(m *Manifest) { m.Security.OIDC.Issuer = "https://issuer.example" },
		"artifacts":  func(m *Manifest) { m.Artifacts.Enabled = true },
		"forensics":  func(m *Manifest) { m.Forensics.RawCapture = true },
		"egress":     func(m *Manifest) { m.Runtime.Sandbox.EgressProxyURL = "http://proxy" },
		"http_tool":  func(m *Manifest) { m.ToolPolicy["http_get"] = ToolPolicy{Enabled: true} },
	} {
		t.Run(name, func(t *testing.T) {
			manifest := InitialManifest(domain.DeploymentDocker, Kubernetes{})
			mutate(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatal("unsupported feature accepted")
			}
		})
	}
}

func TestLocalDockerRequiresAcknowledgement(t *testing.T) {
	docker := Docker{Mode: DockerModeUnsafeLocalSocket}
	if err := docker.ValidateEnvironment(func(string) string { return "" }); err == nil || !strings.Contains(err.Error(), "NORBOT_ALLOW_UNSAFE_LOCAL_DOCKER_SOCKET") {
		t.Fatalf("missing acknowledgement accepted: %v", err)
	}
	if err := docker.ValidateEnvironment(func(string) string { return "true" }); err != nil {
		t.Fatal(err)
	}
}

func TestLocalManifestRetainsExtensionsAndOfflineTools(t *testing.T) {
	manifest := InitialManifest(domain.DeploymentDocker, Kubernetes{})
	manifest.Plugins = []ProcessPlugin{{ID: "plugin", Command: "/tmp/plugin", SHA256: strings.Repeat("a", 64), Methods: []string{"plan"}}}
	manifest.Providers = append(manifest.Providers, Provider{ID: "plugin", Kind: "plugin", PluginID: "plugin", Stages: []domain.Stage{domain.StagePlanner}})
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}
