package runtime

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gongahkia/norbot/internal/config"
)

const (
	networkPolicyProbeImage       = "busybox:1.37@sha256:9532d8c39891ca2ecde4d30d7710e01fb739c87a8b9299685c63704296b16028"
	networkPolicyProbePropagation = 30 * time.Second
)

// VerifyNetworkPolicyEnforcement proves a deny-egress policy blocks one direct pod-to-pod request.
// It creates two short-lived, restricted pods and removes every probe object before returning.
func (k *KubernetesRuntime) VerifyNetworkPolicyEnforcement(ctx context.Context) (err error) {
	if k.restConfig == nil {
		return fmt.Errorf("network policy probe requires a real Kubernetes REST config")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	probeID := shortID()
	serverName := "norbot-netpol-server-" + probeID
	clientName := "norbot-netpol-client-" + probeID
	policyName := "norbot-netpol-deny-" + probeID
	probeLabels := map[string]string{"app.kubernetes.io/managed-by": "norbot", "norbot.network-policy-probe": probeID}
	clientLabels := copyStringMap(probeLabels)
	clientLabels[roleLabel] = "network-policy-probe-client"
	serverLabels := copyStringMap(probeLabels)
	serverLabels[roleLabel] = "network-policy-probe-server"
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cleanupCancel()
	defer func() {
		cleanupErr := k.cleanupNetworkPolicyProbe(cleanupCtx, policyName, clientName, serverName)
		if cleanupErr != nil && err == nil {
			err = cleanupErr
		}
	}()
	pods := k.client.CoreV1().Pods(k.config.Namespace)
	if _, err = pods.Create(ctx, networkPolicyProbePod(serverName, serverLabels, true, k.config), metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create network policy probe server: %w", err)
	}
	if _, err = pods.Create(ctx, networkPolicyProbePod(clientName, clientLabels, false, k.config), metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create network policy probe client: %w", err)
	}
	if err = k.waitPodReady(ctx, serverName); err != nil {
		return fmt.Errorf("wait for network policy probe server: %w", err)
	}
	if err = k.waitPodReady(ctx, clientName); err != nil {
		return fmt.Errorf("wait for network policy probe client: %w", err)
	}
	server, getErr := pods.Get(ctx, serverName, metav1.GetOptions{})
	if getErr != nil {
		return fmt.Errorf("get network policy probe server: %w", getErr)
	}
	if net.ParseIP(server.Status.PodIP) == nil {
		return fmt.Errorf("network policy probe server has no valid pod IP")
	}
	if err = k.waitNetworkPolicyProbeReachable(ctx, clientName, server.Status.PodIP); err != nil {
		return fmt.Errorf("network policy baseline request failed; cannot test enforcement: %w", err)
	}
	policy := networkPolicyProbeDenyPolicy(policyName, clientLabels)
	if _, err = k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Create(ctx, policy, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create network policy probe deny policy: %w", err)
	}
	propagationCtx, propagationCancel := context.WithTimeout(ctx, networkPolicyProbePropagation)
	defer propagationCancel()
	for {
		if probeErr := k.networkPolicyProbeRequest(propagationCtx, clientName, server.Status.PodIP); probeErr != nil {
			if propagationCtx.Err() != nil {
				return fmt.Errorf("network policy probe ended before a blocked request was observed: %w", propagationCtx.Err())
			}
			return nil
		}
		select {
		case <-propagationCtx.Done():
			return fmt.Errorf("NetworkPolicy did not block direct pod egress within %s: %w", networkPolicyProbePropagation, propagationCtx.Err())
		case <-time.After(k.poll):
		}
	}
}

func (k *KubernetesRuntime) waitNetworkPolicyProbeReachable(ctx context.Context, clientName, serverIP string) error {
	var lastErr error
	for {
		if err := k.networkPolicyProbeRequest(ctx, clientName, serverIP); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("baseline request did not succeed: %w", lastErr)
		case <-time.After(k.poll):
		}
	}
}

func (k *KubernetesRuntime) networkPolicyProbeRequest(ctx context.Context, clientName, serverIP string) error {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	endpoint := "http://" + net.JoinHostPort(serverIP, "8080") + "/"
	if err := k.execPod(requestCtx, clientName, "client", []string{"wget", "-q", "-T", "3", "-O", "/dev/null", endpoint}, nil, nil, &stderr); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("request %s: %w: %s", endpoint, err, stderr.String())
		}
		return fmt.Errorf("request %s: %w", endpoint, err)
	}
	return nil
}

func (k *KubernetesRuntime) cleanupNetworkPolicyProbe(ctx context.Context, policyName, clientName, serverName string) error {
	policies := k.client.NetworkingV1().NetworkPolicies(k.config.Namespace)
	if err := policies.Delete(ctx, policyName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete network policy probe policy: %w", err)
	}
	pods := k.client.CoreV1().Pods(k.config.Namespace)
	for _, name := range []string{clientName, serverName} {
		if err := pods.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete network policy probe pod %q: %w", name, err)
		}
	}
	return nil
}

func networkPolicyProbeDenyPolicy(name string, clientLabels map[string]string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: copyStringMap(clientLabels)},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: copyStringMap(clientLabels)},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      []networkingv1.NetworkPolicyEgressRule{},
		},
	}
}

func networkPolicyProbePod(name string, labels map[string]string, server bool, cfg config.Kubernetes) *corev1.Pod {
	uid := int64(65532)
	command := []string{"sh", "-ceu", "sleep 300"}
	containerName := "client"
	if server {
		containerName = "server"
		command = []string{"sh", "-ceu", "printf ok >/tmp/index.html; exec httpd -f -p 8080 -h /tmp"}
	}
	container := corev1.Container{
		Name:            containerName,
		Image:           networkPolicyProbeImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         command,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: *resource.NewMilliQuantity(cfg.CPUMilli, resource.DecimalSI), corev1.ResourceMemory: *resource.NewQuantity(cfg.MemoryMiB<<20, resource.BinarySI)},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: *resource.NewMilliQuantity(cfg.CPUMilli, resource.DecimalSI), corev1.ResourceMemory: *resource.NewQuantity(cfg.MemoryMiB<<20, resource.BinarySI)},
		},
		SecurityContext: restrictedSecurityContext(),
		VolumeMounts:    []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}},
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: copyStringMap(labels)},
		Spec: corev1.PodSpec{
			ServiceAccountName:           cfg.ServiceAccount,
			AutomountServiceAccountToken: ptr(false),
			RestartPolicy:                corev1.RestartPolicyNever,
			SecurityContext:              &corev1.PodSecurityContext{RunAsNonRoot: ptr(true), RunAsUser: &uid, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
			Containers:                   []corev1.Container{container},
			Volumes:                      []corev1.Volume{{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
		},
	}
}

func copyStringMap(values map[string]string) map[string]string {
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}
