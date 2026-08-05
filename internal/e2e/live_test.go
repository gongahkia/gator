//go:build livee2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/runtime"
)

func TestLiveControlPlaneAndProvider(t *testing.T) {
	requireLive(t, "NORBOT_E2E_PUBLIC_URL", "NORBOT_E2E_OPERATOR_JWT", "OPENAI_API_KEY", "NORBOT_E2E_PROVIDER_MODEL")
	base := strings.TrimRight(os.Getenv("NORBOT_E2E_PUBLIC_URL"), "/")
	response, err := http.Get(base + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", response.StatusCode)
	}
	var accounts []map[string]any
	apiJSON(t, http.MethodGet, base+"/api/channels/accounts", nil, &accounts)
	body, _ := json.Marshal(map[string]any{"model": os.Getenv("NORBOT_E2E_PROVIDER_MODEL"), "input": "Return exactly: norbot-live-e2e"})
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		t.Fatalf("provider status=%d body=%s", response.StatusCode, raw)
	}
	if !bytes.Contains(raw, []byte("norbot-live-e2e")) {
		t.Fatalf("provider response missing assertion: %s", raw)
	}
}

func TestLiveKubernetesSandboxPrerequisite(t *testing.T) {
	requireLive(t, "NORBOT_E2E_NAMESPACE", "KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	name := fmt.Sprintf("norbot-live-%d", time.Now().UnixNano())
	zero := int32(0)
	ttl := int32(60)
	uid := int64(65532)
	container := corev1.Container{Name: "assert", Image: "busybox:1.37@sha256:9532d8c39891ca2ecde4d30d7710e01fb739c87a8b9299685c63704296b16028", Command: []string{"sh", "-ceu", "test \"$(id -u)\" = 65532"}, SecurityContext: &corev1.SecurityContext{ReadOnlyRootFilesystem: boolPtr(true), AllowPrivilegeEscalation: boolPtr(false), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}}
	pod := corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: boolPtr(false), SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true), RunAsUser: &uid}, Containers: []corev1.Container{container}}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot", "norbot.e2e": "true"}}, Spec: batchv1.JobSpec{BackoffLimit: &zero, TTLSecondsAfterFinished: &ttl, Template: corev1.PodTemplateSpec{Spec: pod}}}
	jobs := client.BatchV1().Jobs(os.Getenv("NORBOT_E2E_NAMESPACE"))
	if _, err = jobs.Create(ctx, job, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	defer jobs.Delete(context.Background(), name, metav1.DeleteOptions{})
	for {
		current, err := jobs.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if current.Status.Succeeded == 1 {
			return
		}
		if current.Status.Failed > 0 {
			t.Fatal("sandbox assertion job failed")
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func TestLiveNetworkPolicyEnforcement(t *testing.T) {
	requireLive(t, "NORBOT_E2E_NAMESPACE", "KUBECONFIG")
	probe, err := runtime.NewKubernetesRuntime(config.Kubernetes{Kubeconfig: os.Getenv("KUBECONFIG"), Namespace: os.Getenv("NORBOT_E2E_NAMESPACE")}, ".norbot/e2e-artifacts")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := probe.VerifyNetworkPolicyEnforcement(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLiveOutboundChannels(t *testing.T) {
	requireLive(t, "NORBOT_E2E_PUBLIC_URL", "NORBOT_E2E_OPERATOR_JWT", "NORBOT_E2E_TELEGRAM_ACCOUNT", "NORBOT_E2E_TELEGRAM_TARGET", "NORBOT_E2E_SLACK_ACCOUNT", "NORBOT_E2E_SLACK_TARGET", "NORBOT_E2E_DISCORD_ACCOUNT", "NORBOT_E2E_DISCORD_TARGET", "NORBOT_E2E_WHATSAPP_ACCOUNT", "NORBOT_E2E_WHATSAPP_TARGET")
	base := strings.TrimRight(os.Getenv("NORBOT_E2E_PUBLIC_URL"), "/")
	for _, adapter := range []string{"TELEGRAM", "SLACK", "DISCORD", "WHATSAPP"} {
		account := os.Getenv("NORBOT_E2E_" + adapter + "_ACCOUNT")
		target := os.Getenv("NORBOT_E2E_" + adapter + "_TARGET")
		var queued map[string]any
		apiJSON(t, http.MethodPost, base+"/api/channels/accounts/"+account+"/messages", map[string]string{"external_id": target, "text": "norbot live e2e " + strings.ToLower(adapter)}, &queued)
		deadline := time.Now().Add(90 * time.Second)
		for {
			var values []map[string]any
			apiJSON(t, http.MethodGet, base+"/api/channels/accounts/"+account+"/messages/"+target, nil, &values)
			for _, value := range values {
				if value["id"] == queued["id"] && value["state"] == "delivered" {
					goto delivered
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s outbound delivery did not complete: %#v", adapter, values)
			}
			time.Sleep(time.Second)
		}
	delivered:
	}
}

func apiJSON(t *testing.T, method, url string, payload any, target any) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+os.Getenv("NORBOT_E2E_OPERATOR_JWT"))
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		t.Fatalf("%s %s status=%d body=%s", method, url, response.StatusCode, raw)
	}
	if target != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
}
func requireLive(t *testing.T, keys ...string) {
	t.Helper()
	if os.Getenv("NORBOT_E2E_LIVE") != "1" {
		t.Skip("set NORBOT_E2E_LIVE=1")
	}
	for _, key := range keys {
		if os.Getenv(key) == "" {
			t.Fatalf("%s is required", key)
		}
	}
}
func boolPtr(value bool) *bool { return &value }
