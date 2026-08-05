package runtime

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const egressProxyName = "norbot-egress-proxy"

func (k *KubernetesRuntime) EgressProxyService() string { return egressProxyName }

func (k *KubernetesRuntime) ensureEgressProxy(ctx context.Context) error {
	if k.config.EgressProxyImage == "" {
		return nil
	}
	labels := map[string]string{"app.kubernetes.io/managed-by": "norbot", "app.kubernetes.io/name": egressProxyName, roleLabel: "egress-proxy"}
	container := corev1.Container{Name: "egress-proxy", Image: k.config.EgressProxyImage, Command: []string{"norbot", "egress-proxy"}, Ports: []corev1.ContainerPort{{Name: "proxy", ContainerPort: k.config.EgressProxyPort}}, Env: []corev1.EnvVar{{Name: "NORBOT_EGRESS_PROXY_ADDR", Value: fmt.Sprintf(":%d", k.config.EgressProxyPort)}, {Name: "NORBOT_EGRESS_PROXY_SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: k.config.EgressProxySecret}, Key: k.config.EgressProxySecretKey}}}}, SecurityContext: restrictedSecurityContext()}
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: egressProxyName, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": egressProxyName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName:           k.config.ServiceAccount,
					AutomountServiceAccountToken: ptr(false),
					SecurityContext:              &corev1.PodSecurityContext{RunAsNonRoot: ptr(true), RunAsUser: ptr(int64(65532)), FSGroup: ptr(int64(65532)), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
					ImagePullSecrets:             []corev1.LocalObjectReference{{Name: k.config.RegistryPullSecret}},
					Containers:                   []corev1.Container{container},
				},
			},
		},
	}
	if err := k.upsertDeployment(ctx, deployment); err != nil {
		return fmt.Errorf("apply egress proxy deployment: %w", err)
	}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: egressProxyName, Labels: labels}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app.kubernetes.io/name": egressProxyName}, Ports: []corev1.ServicePort{{Name: "proxy", Port: k.config.EgressProxyPort, TargetPort: intstr.FromInt32(k.config.EgressProxyPort)}}}}
	if err := k.upsertService(ctx, service); err != nil {
		return fmt.Errorf("apply egress proxy service: %w", err)
	}
	if err := k.upsertNetworkPolicy(ctx, k.egressProxyPolicy()); err != nil {
		return fmt.Errorf("apply egress proxy network policy: %w", err)
	}
	return nil
}

func (k *KubernetesRuntime) egressProxyPolicy() *networkingv1.NetworkPolicy {
	port := intstr.FromInt32(k.config.EgressProxyPort)
	return &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: egressProxyName + "-network", Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot"}}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": egressProxyName}}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}, Ingress: []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{roleLabel: "sandbox"}}}}, Ports: []networkingv1.NetworkPolicyPort{{Protocol: ptr(corev1.ProtocolTCP), Port: &port}}}}, Egress: []networkingv1.NetworkPolicyEgressRule{{Ports: dnsPorts()}, {To: []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}}}, Ports: []networkingv1.NetworkPolicyPort{{Protocol: ptr(corev1.ProtocolTCP), Port: ptr(intstr.FromInt(443))}}}}}}
}

func (k *KubernetesRuntime) applySandboxNetworkPolicy(ctx context.Context, runID string) error {
	if k.config.EgressProxyImage == "" {
		return fmt.Errorf("kubernetes egress proxy is not provisioned")
	}
	port := intstr.FromInt32(k.config.EgressProxyPort)
	policy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: k.Name(runID) + "-sandbox-network", Labels: k.labels(runID, "sandbox")}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{runLabel: runID, roleLabel: "sandbox"}}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}, Egress: []networkingv1.NetworkPolicyEgressRule{{Ports: dnsPorts()}, {To: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": egressProxyName}}}}, Ports: []networkingv1.NetworkPolicyPort{{Protocol: ptr(corev1.ProtocolTCP), Port: &port}}}}}}
	return k.upsertNetworkPolicy(ctx, policy)
}

func dnsPorts() []networkingv1.NetworkPolicyPort {
	return []networkingv1.NetworkPolicyPort{{Protocol: ptr(corev1.ProtocolUDP), Port: ptr(intstr.FromInt(53))}, {Protocol: ptr(corev1.ProtocolTCP), Port: ptr(intstr.FromInt(53))}}
}

func (k *KubernetesRuntime) upsertService(ctx context.Context, service *corev1.Service) error {
	_, err := k.client.CoreV1().Services(k.config.Namespace).Create(ctx, service, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	existing, err := k.client.CoreV1().Services(k.config.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	existing.Labels = service.Labels
	existing.Spec.Selector = service.Spec.Selector
	existing.Spec.Ports = service.Spec.Ports
	_, err = k.client.CoreV1().Services(k.config.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (k *KubernetesRuntime) upsertNetworkPolicy(ctx context.Context, policy *networkingv1.NetworkPolicy) error {
	_, err := k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Create(ctx, policy, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	existing, err := k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Get(ctx, policy.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	existing.Labels = policy.Labels
	existing.Spec = policy.Spec
	_, err = k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}
