package runtime

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
)

func testKubernetesRuntime(client *fake.Clientset) *KubernetesRuntime {
	return NewKubernetesRuntimeWithClient(config.Kubernetes{Namespace: "norbot", ServiceAccount: "norbot-runtime", RegistryRepository: "registry.test/norbot", RegistryPullSecret: "registry-pull", CPUMilli: 500, MemoryMiB: 512, Replicas: 1}, tTempArtifacts(), client, nil)
}

func tTempArtifacts() string { return ".norbot-test-artifacts" }

func TestKubernetesApplicationResourcesAreRestricted(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	runtime := testKubernetesRuntime(client)
	run := domain.Run{ID: "run123", Profile: domain.ProfileAgentic, DeploymentTarget: domain.DeploymentKubernetes}
	if err := runtime.applyApplication(ctx, run, map[string]string{"frontend": "registry.test/frontend:run123", "backend": "registry.test/backend:run123"}, "", run.ID); err != nil {
		t.Fatal(err)
	}
	frontend, err := client.AppsV1().Deployments("norbot").Get(ctx, runtime.frontendName(run.ID), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if frontend.Spec.Replicas == nil || *frontend.Spec.Replicas != 1 {
		t.Fatalf("replicas=%v", frontend.Spec.Replicas)
	}
	container := frontend.Spec.Template.Spec.Containers[0]
	if container.SecurityContext == nil || container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("container is not restricted: %#v", container.SecurityContext)
	}
	backend, err := client.AppsV1().Deployments("norbot").Get(ctx, runtime.backendName(run.ID), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if backend.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Path != "/api/health" {
		t.Fatal("backend health probe missing")
	}
	if _, err := client.CoreV1().Services("norbot").Get(ctx, runtime.frontendName(run.ID), metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
	policy, err := client.NetworkingV1().NetworkPolicies("norbot").Get(ctx, runtime.Name(run.ID)+"-network", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Spec.PolicyTypes) != 2 || len(policy.Spec.Egress) != 2 {
		t.Fatalf("unexpected policy=%#v", policy.Spec)
	}
}

func TestKubernetesBootstrapIsNamespaced(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "registry-pull", Namespace: "norbot"}})
	runtime := testKubernetesRuntime(client)
	if err := runtime.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().ServiceAccounts("norbot").Get(ctx, "norbot-runtime", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
	role, err := client.RbacV1().Roles("norbot").Get(ctx, "norbot-runtime", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(role.Rules) < 4 {
		t.Fatalf("rules=%#v", role.Rules)
	}
	if _, err := client.RbacV1().RoleBindings("norbot").Get(ctx, "norbot-runtime", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestKubernetesEgressProxyResourcesAreIsolated(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	runtime := NewKubernetesRuntimeWithClient(config.Kubernetes{Namespace: "norbot", ServiceAccount: "norbot-runtime", RegistryRepository: "registry.test/norbot", RegistryPullSecret: "registry-pull", EgressProxyImage: "registry.test/norbot:latest", EgressProxySecret: "proxy-secret", EgressProxySecretKey: "secret", EgressProxyPort: 8181, NetworkPolicyEnforced: true}, tTempArtifacts(), client, nil)
	if err := runtime.ensureEgressProxy(ctx); err != nil {
		t.Fatal(err)
	}
	deployment, err := client.AppsV1().Deployments("norbot").Get(ctx, egressProxyName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Template.Spec.Containers[0].Command[1] != "egress-proxy" {
		t.Fatalf("command=%v", deployment.Spec.Template.Spec.Containers[0].Command)
	}
	if _, err := client.CoreV1().Services("norbot").Get(ctx, egressProxyName, metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
	policy, err := client.NetworkingV1().NetworkPolicies("norbot").Get(ctx, egressProxyName+"-network", metav1.GetOptions{})
	if err != nil || len(policy.Spec.Ingress) != 1 || len(policy.Spec.Egress) != 2 {
		t.Fatalf("policy=%#v err=%v", policy, err)
	}
	if err := runtime.applySandboxNetworkPolicy(ctx, "run123"); err != nil {
		t.Fatal(err)
	}
	sandbox, err := client.NetworkingV1().NetworkPolicies("norbot").Get(ctx, runtime.Name("run123")+"-sandbox-network", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sandbox.Spec.Egress) != 2 || len(sandbox.Spec.Ingress) != 0 {
		t.Fatalf("sandbox policy=%#v", sandbox.Spec)
	}
}

func TestKubernetesCapacityUsesAllocatableResources(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}, Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4"), corev1.ResourceMemory: resource.MustParse("16Gi")}}})
	runtime := testKubernetesRuntime(client)
	capacity, err := runtime.Capacity(ctx, 2, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !capacity.KubernetesAvailable || capacity.Target != domain.DeploymentKubernetes || capacity.RecommendedWorkers != 4 {
		t.Fatalf("capacity=%#v", capacity)
	}
}
