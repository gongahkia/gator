package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
)

const (
	runLabel  = "norbot.run_id"
	roleLabel = "norbot.role"
)

type KubernetesRuntime struct {
	client       kubernetes.Interface
	restConfig   *rest.Config
	config       config.Kubernetes
	artifactsDir string
	poll         time.Duration
}

func NewKubernetesRuntime(k config.Kubernetes, artifactsDir string) (*KubernetesRuntime, error) {
	k = k.Normalized()
	loading := &clientcmd.ClientConfigLoadingRules{ExplicitPath: k.Kubeconfig}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: k.Context}
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return NewKubernetesRuntimeWithClient(k, artifactsDir, client, restConfig), nil
}

func NewKubernetesRuntimeWithClient(k config.Kubernetes, artifactsDir string, client kubernetes.Interface, restConfig *rest.Config) *KubernetesRuntime {
	return &KubernetesRuntime{client: client, restConfig: restConfig, config: k.Normalized(), artifactsDir: artifactsDir, poll: time.Second}
}

func (k *KubernetesRuntime) Target() domain.DeploymentTarget { return domain.DeploymentKubernetes }

func (k *KubernetesRuntime) Validate(ctx context.Context) error {
	if _, err := k.client.CoreV1().Namespaces().Get(ctx, k.config.Namespace, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("get kubernetes namespace %q: %w", k.config.Namespace, err)
	}
	if _, err := k.client.CoreV1().ServiceAccounts(k.config.Namespace).Get(ctx, k.config.ServiceAccount, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("get kubernetes service account %q: %w", k.config.ServiceAccount, err)
	}
	if _, err := k.client.CoreV1().ResourceQuotas(k.config.Namespace).Get(ctx, "norbot-runtime", metav1.GetOptions{}); err != nil {
		return fmt.Errorf("get norbot resource quota: %w", err)
	}
	if _, err := k.client.CoreV1().LimitRanges(k.config.Namespace).Get(ctx, "norbot-runtime", metav1.GetOptions{}); err != nil {
		return fmt.Errorf("get norbot limit range: %w", err)
	}
	if _, err := k.client.CoreV1().Secrets(k.config.Namespace).Get(ctx, k.config.RegistryPullSecret, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("get registry pull secret %q: %w", k.config.RegistryPullSecret, err)
	}
	if k.config.EgressProxyImage != "" {
		secret, err := k.client.CoreV1().Secrets(k.config.Namespace).Get(ctx, k.config.EgressProxySecret, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get egress proxy secret %q: %w", k.config.EgressProxySecret, err)
		}
		if len(secret.Data[k.config.EgressProxySecretKey]) == 0 {
			return fmt.Errorf("egress proxy secret %q has no %q key", k.config.EgressProxySecret, k.config.EgressProxySecretKey)
		}
		if _, err := k.client.CoreV1().Services(k.config.Namespace).Get(ctx, k.EgressProxyService(), metav1.GetOptions{}); err != nil {
			return fmt.Errorf("get egress proxy service: %w", err)
		}
		if _, err := k.client.AppsV1().Deployments(k.config.Namespace).Get(ctx, k.EgressProxyService(), metav1.GetOptions{}); err != nil {
			return fmt.Errorf("get egress proxy deployment: %w", err)
		}
	}
	return nil
}

func (k *KubernetesRuntime) Capacity(ctx context.Context, configuredWorkers, maxWorkers int) (Capacity, error) {
	nodes, err := k.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Capacity{}, fmt.Errorf("list kubernetes nodes: %w", err)
	}
	capacity := Capacity{ConfiguredWorkers: configuredWorkers, KubernetesAvailable: true, Target: domain.DeploymentKubernetes}
	var cpuMilli, memoryBytes int64
	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}
		cpuMilli += node.Status.Allocatable.Cpu().MilliValue()
		memoryBytes += node.Status.Allocatable.Memory().Value()
	}
	capacity.CPUs = int(cpuMilli / 1000)
	capacity.MemoryBytes = memoryBytes
	byCPU, byMemory := int(cpuMilli/(k.config.CPUMilli*2)), int(memoryBytes/(4<<30))
	if byCPU < 1 {
		byCPU = 1
	}
	if byMemory < 1 {
		byMemory = 1
	}
	capacity.RecommendedWorkers = min(maxWorkers, min(byCPU, byMemory))
	capacity.Recommendation = "Kubernetes allocatable CPU/RAM recommendation; provider quotas and namespace quotas are applied separately."
	return capacity, nil
}
func (k *KubernetesRuntime) Name(runID string) string         { return ProjectName(runID) }
func (k *KubernetesRuntime) PVC(runID string) string          { return k.Name(runID) + "-workspace" }
func (k *KubernetesRuntime) WorkspacePod(runID string) string { return k.Name(runID) + "-workspace" }
func (k *KubernetesRuntime) RunPath(runID string) string      { return filepath.Join(k.artifactsDir, runID) }
func (k *KubernetesRuntime) labels(runID, role string) map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "norbot", runLabel: runID, roleLabel: role}
}

func (k *KubernetesRuntime) runSelector(runID string) string {
	return labels.Set(map[string]string{"app.kubernetes.io/managed-by": "norbot", runLabel: runID}).AsSelector().String()
}

func (k *KubernetesRuntime) Ensure(ctx context.Context, runID string) error {
	if err := os.MkdirAll(k.RunPath(runID), 0o750); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	pvcs := k.client.CoreV1().PersistentVolumeClaims(k.config.Namespace)
	if _, err := pvcs.Get(ctx, k.PVC(runID), metav1.GetOptions{}); apierrors.IsNotFound(err) {
		_, err = pvcs.Create(ctx, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: k.PVC(runID), Labels: k.labels(runID, "workspace")}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")}}}}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create workspace pvc: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get workspace pvc: %w", err)
	}
	pods := k.client.CoreV1().Pods(k.config.Namespace)
	if _, err := pods.Get(ctx, k.WorkspacePod(runID), metav1.GetOptions{}); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get workspace pod: %w", err)
	}
	_, err := pods.Create(ctx, workspacePod(k.config, runID, k.PVC(runID)), metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create workspace pod: %w", err)
	}
	return k.waitPodReady(ctx, k.WorkspacePod(runID))
}

func workspacePod(cfg config.Kubernetes, runID, pvc string) *corev1.Pod {
	noRoot := true
	uid := int64(65532)
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: ProjectName(runID) + "-workspace", Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot", runLabel: runID, roleLabel: "workspace"}}, Spec: corev1.PodSpec{ServiceAccountName: cfg.ServiceAccount, AutomountServiceAccountToken: ptr(false), RestartPolicy: corev1.RestartPolicyAlways, SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: &noRoot, RunAsUser: &uid, FSGroup: &uid, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Containers: []corev1.Container{{Name: "workspace", Image: cfg.WorkspaceImage, Command: []string{"sh", "-ceu", "mkdir -p /workspace/agent-state && sleep infinity"}, VolumeMounts: []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}}, SecurityContext: restrictedSecurityContext()}}, Volumes: []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvc}}}}}}
}

func restrictedSecurityContext() *corev1.SecurityContext {
	noRoot, readOnly, allowEscalation := true, true, false
	return &corev1.SecurityContext{RunAsNonRoot: &noRoot, ReadOnlyRootFilesystem: &readOnly, AllowPrivilegeEscalation: &allowEscalation, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}
}
func ptr[T any](v T) *T { return &v }

func (k *KubernetesRuntime) WriteArtifact(runID, relative string, content []byte) (string, error) {
	if !safeRelative(relative) {
		return "", fmt.Errorf("unsafe artifact path")
	}
	path := filepath.Join(k.RunPath(runID), relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, content, 0o640); err != nil {
		return "", err
	}
	return path, nil
}

func (k *KubernetesRuntime) MirrorToVolume(ctx context.Context, runID, relative string) error {
	if !safeRelative(relative) {
		return fmt.Errorf("unsafe artifact path")
	}
	return k.copyToWorkspace(ctx, runID, filepath.Join(k.RunPath(runID), relative), relative)
}

func (k *KubernetesRuntime) MirrorGeneratedApp(ctx context.Context, runID string) error {
	return k.copyToWorkspace(ctx, runID, filepath.Join(k.RunPath(runID), "generated-app"), "generated-app")
}

func (k *KubernetesRuntime) SyncGeneratedApp(ctx context.Context, runID string) error {
	var output bytes.Buffer
	if err := k.exec(ctx, runID, []string{"tar", "-C", "/workspace", "-cf", "-", "generated-app"}, nil, &output, nil); err != nil {
		return err
	}
	return extractTar(bytes.NewReader(output.Bytes()), k.RunPath(runID))
}

func (k *KubernetesRuntime) copyToWorkspace(ctx context.Context, runID, source, destination string) error {
	if !safeRelative(destination) {
		return fmt.Errorf("unsafe workspace destination")
	}
	archive, err := tarPath(source, destination)
	if err != nil {
		return err
	}
	defer archive.Close()
	return k.exec(ctx, runID, []string{"sh", "-ceu", "tar -C /workspace -xf -"}, archive, nil, nil)
}

func (k *KubernetesRuntime) exec(ctx context.Context, runID string, command []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return k.execPod(ctx, k.WorkspacePod(runID), "workspace", command, stdin, stdout, stderr)
}

func (k *KubernetesRuntime) execPod(ctx context.Context, pod, container string, command []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if k.restConfig == nil {
		return fmt.Errorf("kubernetes pod exec requires a REST config")
	}
	req := k.client.CoreV1().RESTClient().Post().Resource("pods").Name(pod).Namespace(k.config.Namespace).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: container, Command: command, Stdin: stdin != nil, Stdout: stdout != nil, Stderr: stderr != nil, TTY: false}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(k.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: stdin, Stdout: stdout, Stderr: stderr})
}

func (k *KubernetesRuntime) RunCLI(ctx context.Context, runID, image, network string, command []string, prompt, credentialEnv, credentialSecret, credentialSecretKey string) (string, error) {
	if image == "" || len(command) == 0 {
		return "", fmt.Errorf("cli runner image and command are required")
	}
	if len(prompt) > 768<<10 {
		return "", fmt.Errorf("cli prompt exceeds kubernetes configmap limit")
	}
	name := k.Name(runID) + "-cli-" + shortID()
	input := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: k.labels(runID, "cli-input")}, Data: map[string]string{"prompt": prompt}}
	if _, err := k.client.CoreV1().ConfigMaps(k.config.Namespace).Create(ctx, input, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("create cli input: %w", err)
	}
	defer k.client.CoreV1().ConfigMaps(k.config.Namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	container := hardenedContainer("agent", image, []string{"sh", "-ceu", "cat /norbot/input/prompt | exec " + shellArgs(command)}, k.PVC(runID), "/workspace", k.config)
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "input", MountPath: "/norbot/input", ReadOnly: true})
	if credentialEnv != "" {
		if credentialSecret == "" || credentialSecretKey == "" {
			return "", fmt.Errorf("kubernetes CLI provider %q requires kubernetes_secret and kubernetes_secret_key", credentialEnv)
		}
		container.Env = append(container.Env, corev1.EnvVar{Name: credentialEnv, ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: credentialSecret}, Key: credentialSecretKey}}})
	}
	job := oneShotJob(name, k.config, runID, "agent", container, []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: k.PVC(runID)}}}, {Name: "input", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}}}}})
	return k.runJob(ctx, job, true)
}

func (k *KubernetesRuntime) Cleanup(ctx context.Context, runID string) error {
	selector := k.runSelector(runID)
	_ = k.client.BatchV1().Jobs(k.config.Namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: selector})
	_ = k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Delete(ctx, k.Name(runID)+"-sandbox-network", metav1.DeleteOptions{})
	_ = k.client.CoreV1().Pods(k.config.Namespace).Delete(ctx, k.WorkspacePod(runID), metav1.DeleteOptions{})
	if err := k.client.CoreV1().PersistentVolumeClaims(k.config.Namespace).Delete(ctx, k.PVC(runID), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (k *KubernetesRuntime) Deploy(ctx context.Context, run domain.Run, root string) (string, error) {
	if err := k.Ensure(ctx, run.ID); err != nil {
		return "", err
	}
	images, err := k.buildImages(ctx, run, "")
	if err != nil {
		return "", err
	}
	appID := ApplicationID(run)
	if err := k.applyApplication(ctx, run, images, "", appID); err != nil {
		return "", err
	}
	if err := k.waitDeployment(ctx, k.frontendName(appID)); err != nil {
		return "", err
	}
	if run.Profile != domain.ProfileFrontend {
		if err := k.waitDeployment(ctx, k.backendName(appID)); err != nil {
			return "", err
		}
	}
	appRun := run
	appRun.ID = appID
	return k.publicURL(appRun), nil
}

func (k *KubernetesRuntime) Stop(ctx context.Context, runID, root string) error {
	return k.scale(ctx, k.frontendName(runID), 0, k.backendName(runID))
}
func (k *KubernetesRuntime) Start(ctx context.Context, runID, root string) error {
	return k.scale(ctx, k.frontendName(runID), k.config.Replicas, k.backendName(runID))
}

func (k *KubernetesRuntime) scale(ctx context.Context, frontend string, replicas int32, backend string) error {
	for _, name := range []string{frontend, backend} {
		deployment, err := k.client.AppsV1().Deployments(k.config.Namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) && name == backend {
			continue
		}
		if err != nil {
			return err
		}
		deployment.Spec.Replicas = &replicas
		if _, err := k.client.AppsV1().Deployments(k.config.Namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func (k *KubernetesRuntime) Delete(ctx context.Context, runID, root string) error {
	return k.deleteApplication(ctx, runID, "")
}

func (k *KubernetesRuntime) Status(ctx context.Context, runID, root string) (DeploymentStatus, error) {
	project := k.Name(runID)
	selector := k.runSelector(runID) + "," + roleLabel + "=application"
	pods, err := k.client.CoreV1().Pods(k.config.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return DeploymentStatus{}, err
	}
	services := make([]map[string]any, 0, len(pods.Items))
	for _, pod := range pods.Items {
		services = append(services, map[string]any{"name": pod.Name, "phase": pod.Status.Phase, "node": pod.Spec.NodeName, "containers": containerStatuses(pod.Status.ContainerStatuses)})
	}
	return DeploymentStatus{Target: k.Target(), Project: project, Namespace: k.config.Namespace, Workload: k.frontendName(runID), Image: k.repository(runID, "frontend"), Services: services}, nil
}

func (k *KubernetesRuntime) Logs(ctx context.Context, runID, root string, lines int) (string, error) {
	if lines < 1 || lines > 10000 {
		return "", fmt.Errorf("log line limit must be 1-10000")
	}
	selector := k.runSelector(runID) + "," + roleLabel + "=application"
	pods, err := k.client.CoreV1().Pods(k.config.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", err
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	var out strings.Builder
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			stream, err := k.client.CoreV1().Pods(k.config.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container.Name, TailLines: ptr(int64(lines))}).Stream(ctx)
			if err != nil {
				return "", err
			}
			data, readErr := io.ReadAll(io.LimitReader(stream, 2<<20))
			stream.Close()
			if readErr != nil {
				return "", readErr
			}
			fmt.Fprintf(&out, "[%s/%s]\n%s", pod.Name, container.Name, data)
		}
	}
	return out.String(), nil
}

func (k *KubernetesRuntime) InvokeAgent(ctx context.Context, run domain.Run, root string, input AgentInvocation) (AgentResponse, error) {
	if run.Profile != domain.ProfileAgentic {
		return AgentResponse{}, fmt.Errorf("run is not an agentic application")
	}
	pods, err := k.client.CoreV1().Pods(k.config.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + k.backendName(ApplicationID(run))})
	if err != nil {
		return AgentResponse{}, err
	}
	if len(pods.Items) != 1 {
		return AgentResponse{}, fmt.Errorf("expected one backend pod, found %d", len(pods.Items))
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return AgentResponse{}, err
	}
	var output, stderr bytes.Buffer
	if err := k.execPod(ctx, pods.Items[0].Name, "backend", []string{"wget", "-qO-", "--header=Content-Type: application/json", "--post-file=-", "http://127.0.0.1:8000/api/agents/run"}, bytes.NewReader(payload), &output, &stderr); err != nil {
		return AgentResponse{}, fmt.Errorf("invoke agent: %w: %s", err, stderr.String())
	}
	var response AgentResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return AgentResponse{}, fmt.Errorf("decode agent response: %w", err)
	}
	if response.State == "" {
		if response.Status != "" {
			response.State = response.Status
		} else {
			response.State = "completed"
		}
	}
	return response, nil
}

func (k *KubernetesRuntime) RunSandbox(ctx context.Context, runID string, request SandboxRequest, policy config.Sandbox) (SandboxResult, error) {
	if len(request.Command) == 0 {
		return SandboxResult{}, fmt.Errorf("sandbox command is required")
	}
	if err := k.Ensure(ctx, runID); err != nil {
		return SandboxResult{}, err
	}
	p := policy.Normalized()
	if len(request.AllowedHosts) > 0 && (p.EgressProxyURL == "" || p.EgressProxySecret == "") {
		return SandboxResult{}, fmt.Errorf("sandbox egress requires configured managed proxy")
	}
	if len(request.AllowedHosts) > 0 {
		if !k.config.NetworkPolicyEnforced {
			return SandboxResult{}, fmt.Errorf("sandbox egress is disabled until network_policy_enforced is set after a passing enforcement check")
		}
		if err := k.applySandboxNetworkPolicy(ctx, runID); err != nil {
			return SandboxResult{}, err
		}
		defer k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Delete(context.Background(), k.Name(runID)+"-sandbox-network", metav1.DeleteOptions{})
	}
	name := k.Name(runID) + "-sandbox-" + shortID()
	image := p.Image
	if request.Image != "" {
		image = request.Image
	}
	noRoot, readOnly, allowEscalation := true, true, false
	container := corev1.Container{Name: "sandbox", Image: image, WorkingDir: "/workspace", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: *resource.NewMilliQuantity(p.CPUMilli, resource.DecimalSI), corev1.ResourceMemory: *resource.NewQuantity(p.MemoryMiB<<20, resource.BinarySI)}, Limits: corev1.ResourceList{corev1.ResourceCPU: *resource.NewMilliQuantity(p.CPUMilli, resource.DecimalSI), corev1.ResourceMemory: *resource.NewQuantity(p.MemoryMiB<<20, resource.BinarySI)}}, SecurityContext: &corev1.SecurityContext{RunAsNonRoot: &noRoot, ReadOnlyRootFilesystem: &readOnly, AllowPrivilegeEscalation: &allowEscalation, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, VolumeMounts: []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace", ReadOnly: true}, {Name: "scratch", MountPath: "/scratch"}}}
	if strings.HasPrefix(image, "curlimages/curl") {
		container.Args = request.Command
	} else {
		container.Command = request.Command
	}
	if len(request.AllowedHosts) > 0 {
		container.Env = []corev1.EnvVar{{Name: "HTTPS_PROXY", Value: p.EgressProxyURL}, {Name: "HTTP_PROXY", Value: p.EgressProxyURL}, {Name: "NO_PROXY", Value: ""}}
	}
	job := oneShotJob(name, k.config, runID, "sandbox", container, []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: k.PVC(runID), ReadOnly: true}}}, {Name: "scratch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}})
	deadline := int64(p.TimeoutS)
	job.Spec.ActiveDeadlineSeconds = &deadline
	started := time.Now()
	output, err := k.runJob(ctx, job, true)
	result := SandboxResult{Output: output, DurationMS: time.Since(started).Milliseconds(), Network: "none"}
	if len(request.AllowedHosts) > 0 {
		result.Network = "managed-proxy"
	}
	if err != nil {
		result.ExitCode = 1
		return result, err
	}
	return result, nil
}

func (k *KubernetesRuntime) Verify(ctx context.Context, run domain.Run) (map[string]any, error) {
	return k.VerifyChanged(ctx, run, nil)
}

func (k *KubernetesRuntime) VerifyChanged(ctx context.Context, run domain.Run, changed map[string]string) (map[string]any, error) {
	if err := k.Ensure(ctx, run.ID); err != nil {
		return nil, err
	}
	checks := []string{"profile contract"}
	fastChecks := []string{}
	fullChecks := []string{}
	failure := func(err error) (map[string]any, error) {
		return map[string]any{"status": "fail", "checks": checks, "fast_checks": fastChecks, "full_checks": fullChecks}, err
	}
	frontendKey, err := k.verificationCacheKey(run.ID, "node:22-alpine", "generated-app/frontend/package.json", "generated-app/frontend/package-lock.json")
	if err != nil {
		return failure(err)
	}
	frontendPrefix := k.nodeVerificationPrefix(frontendKey)
	if needsFastFrontend(changed) {
		if _, err := k.runVerifier(ctx, run, "frontend-fast", "node:22-alpine", []string{"sh", "-ceu", frontendPrefix + "npm test"}, "/workspace/generated-app/frontend"); err != nil {
			return failure(fmt.Errorf("fast frontend test: %w", err))
		}
		checks = append(checks, "fast frontend test")
		fastChecks = append(fastChecks, "frontend test")
	}
	if _, err := k.runVerifier(ctx, run, "frontend", "node:22-alpine", []string{"sh", "-ceu", frontendPrefix + "npm test && npm run build && npm audit --omit=dev --audit-level=high"}, "/workspace/generated-app/frontend"); err != nil {
		return failure(fmt.Errorf("frontend test/build/audit: %w", err))
	}
	checks = append(checks, "node dependency cache/test/build/audit")
	fullChecks = append(fullChecks, "frontend dependency cache/test/build/audit")
	if run.Profile != domain.ProfileFrontend {
		backendKey, err := k.verificationCacheKey(run.ID, "golang:1.26-alpine", "generated-app/backend/go.mod", "generated-app/backend/go.sum")
		if err != nil {
			return failure(err)
		}
		backendPrefix := k.goVerificationPrefix(backendKey)
		if needsFastBackend(changed) {
			if _, err := k.runVerifier(ctx, run, "backend-fast", "golang:1.26-alpine", []string{"sh", "-ceu", backendPrefix + "go test ./..."}, "/workspace/generated-app/backend"); err != nil {
				return failure(fmt.Errorf("fast Go test: %w", err))
			}
			checks = append(checks, "fast Go test")
			fastChecks = append(fastChecks, "Go test")
		}
		if _, err := k.runVerifier(ctx, run, "backend", "golang:1.26-alpine", []string{"sh", "-ceu", backendPrefix + "go test ./... && go build ./... && govulncheck ./..."}, "/workspace/generated-app/backend"); err != nil {
			return failure(fmt.Errorf("Go test/build/govulncheck: %w", err))
		}
		checks = append(checks, "Go dependency cache/test/build/govulncheck")
		fullChecks = append(fullChecks, "Go dependency cache/test/build/govulncheck")
	}
	images, err := k.buildImages(ctx, run, "verify")
	if err != nil {
		return failure(err)
	}
	if err := k.applyApplication(ctx, run, images, "verify", run.ID); err != nil {
		return failure(err)
	}
	defer k.deleteApplication(context.Background(), run.ID, "verify")
	if err := k.waitDeployment(ctx, k.frontendName(run.ID)+"-verify"); err != nil {
		return failure(err)
	}
	if run.Profile != domain.ProfileFrontend {
		if err := k.waitDeployment(ctx, k.backendName(run.ID)+"-verify"); err != nil {
			return failure(err)
		}
	}
	if _, err := k.runSmoke(ctx, run, "verify"); err != nil {
		return failure(err)
	}
	checks = append(checks, "kaniko image build", "kubernetes rollout/health/smoke")
	fullChecks = append(fullChecks, "kaniko image build", "Kubernetes rollout/health/smoke")
	return map[string]any{"status": "pass", "checks": checks, "fast_checks": fastChecks, "full_checks": fullChecks, "summary": "Kubernetes dependency, build, vulnerability, rollout, and in-cluster smoke checks passed. Operator approval is required before deployment."}, nil
}

func (k *KubernetesRuntime) nodeVerificationPrefix(key string) string {
	return "cache=/workspace/.norbot-cache/node-" + key + "; " + kubernetesCacheLockPrefix() + "if [ ! -f \"$cache/.norbot-key\" ] || [ \"$(cat \"$cache/.norbot-key\")\" != '" + key + "' ]; then rm -rf \"$cache\"; mkdir -p \"$cache\"; npm ci; mv node_modules \"$cache/node_modules\"; printf '%s' '" + key + "' >\"$cache/.norbot-key\"; fi; rm -rf node_modules; ln -s \"$cache/node_modules\" node_modules; trap 'rm -f node_modules; rmdir \"$lock\"' EXIT; "
}

func (k *KubernetesRuntime) goVerificationPrefix(key string) string {
	return "cache=/workspace/.norbot-cache/go-" + key + "; " + kubernetesCacheLockPrefix() + "if [ ! -f \"$cache/.norbot-key\" ] || [ \"$(cat \"$cache/.norbot-key\")\" != '" + key + "' ]; then rm -rf \"$cache\"; mkdir -p \"$cache\"; GOMODCACHE=\"$cache/mod\" GOCACHE=\"$cache/build\" GOBIN=\"$cache/bin\" go mod download; GOMODCACHE=\"$cache/mod\" GOCACHE=\"$cache/build\" GOBIN=\"$cache/bin\" go install golang.org/x/vuln/cmd/govulncheck@v1.6.0; printf '%s' '" + key + "' >\"$cache/.norbot-key\"; fi; PATH=\"$cache/bin:$PATH\" GOMODCACHE=\"$cache/mod\" GOCACHE=\"$cache/build\" "
}

func kubernetesCacheLockPrefix() string {
	return "mkdir -p /workspace/.norbot-cache; lock=\"$cache.lock\"; deadline=$(( $(date +%s) + 300 )); until mkdir \"$lock\" 2>/dev/null; do [ \"$(date +%s)\" -lt \"$deadline\" ] || { echo 'dependency cache lock timeout' >&2; exit 1; }; sleep 1; done; trap 'rmdir \"$lock\"' EXIT; "
}

func needsFastFrontend(changed map[string]string) bool {
	if len(changed) == 0 {
		return true
	}
	for path := range changed {
		if strings.HasPrefix(path, "generated-app/frontend/") || path == "generated-app/docker-compose.yml" {
			return true
		}
	}
	return false
}

func needsFastBackend(changed map[string]string) bool {
	if len(changed) == 0 {
		return true
	}
	for path := range changed {
		if strings.HasPrefix(path, "generated-app/backend/") || path == "generated-app/docker-compose.yml" {
			return true
		}
	}
	return false
}

func (k *KubernetesRuntime) verificationCacheKey(runID, image string, files ...string) (string, error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte(image))
	for _, relative := range files {
		content, err := os.ReadFile(filepath.Join(k.RunPath(runID), relative))
		if err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(relative))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(content)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (k *KubernetesRuntime) runVerifier(ctx context.Context, run domain.Run, role, image string, command []string, directory string) (string, error) {
	name := k.Name(run.ID) + "-verify-" + role + "-" + shortID()
	container := hardenedContainer("verify", image, command, k.PVC(run.ID), directory, k.config)
	return k.runJob(ctx, oneShotJob(name, k.config, run.ID, "verify", container, workspaceVolume(k.PVC(run.ID))), true)
}

func (k *KubernetesRuntime) buildImages(ctx context.Context, run domain.Run, suffix string) (map[string]string, error) {
	images := map[string]string{}
	for _, component := range []string{"frontend"} {
		image, err := k.buildImage(ctx, run, component, suffix)
		if err != nil {
			return nil, err
		}
		images[component] = image
	}
	if run.Profile != domain.ProfileFrontend {
		image, err := k.buildImage(ctx, run, "backend", suffix)
		if err != nil {
			return nil, err
		}
		images["backend"] = image
	}
	return images, nil
}

func (k *KubernetesRuntime) buildImage(ctx context.Context, run domain.Run, component, suffix string) (string, error) {
	name := k.Name(run.ID) + "-kaniko-" + component + "-" + shortID()
	destination := k.repository(run.ID, component)
	if suffix != "" {
		destination += "-" + suffix
	}
	command := []string{"/kaniko/executor", "--context=dir:///workspace/generated-app/" + component, "--dockerfile=/workspace/generated-app/" + component + "/Dockerfile", "--destination=" + destination, "--digest-file=/dev/termination-log", "--snapshotMode=redo"}
	if k.config.RegistryInsecure {
		command = append(command, "--insecure", "--skip-tls-verify")
	}
	container := hardenedContainer("kaniko", k.config.KanikoImage, command, k.PVC(run.ID), "/workspace", k.config)
	container.SecurityContext.ReadOnlyRootFilesystem = ptr(false)
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "registry", MountPath: "/kaniko/.docker", ReadOnly: true})
	volumes := append(workspaceVolume(k.PVC(run.ID)), corev1.Volume{Name: "registry", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: k.config.RegistryPullSecret, Items: []corev1.KeyToPath{{Key: ".dockerconfigjson", Path: "config.json"}}}}})
	output, err := k.runJob(ctx, oneShotJob(name, k.config, run.ID, "kaniko", container, volumes), true)
	if err != nil {
		return "", err
	}
	_ = output
	return destination, nil
}

func (k *KubernetesRuntime) applyApplication(ctx context.Context, run domain.Run, images map[string]string, suffix, appID string) error {
	frontend := k.frontendName(appID)
	if suffix != "" {
		frontend += "-" + suffix
	}
	backend := k.backendName(appID)
	if suffix != "" {
		backend += "-" + suffix
	}
	if run.Profile != domain.ProfileFrontend {
		if err := k.applyDeployment(ctx, backend, appID, run.ID, "backend", images["backend"], 8000, k.config.Replicas); err != nil {
			return err
		}
		if err := k.applyService(ctx, backend, appID, "backend", 8000); err != nil {
			return err
		}
	}
	if err := k.applyDeployment(ctx, frontend, appID, run.ID, "frontend", images["frontend"], 80, k.config.Replicas); err != nil {
		return err
	}
	if err := k.applyService(ctx, frontend, appID, "frontend", 80); err != nil {
		return err
	}
	if err := k.applyNetworkPolicy(ctx, appID, frontend, backend); err != nil {
		return err
	}
	if suffix == "" && run.PublicIngress {
		appRun := run
		appRun.ID = appID
		if err := k.applyIngress(ctx, appRun, frontend); err != nil {
			return err
		}
	}
	return nil
}

func (k *KubernetesRuntime) applyDeployment(ctx context.Context, name, appID, workspaceID, component, image string, port int32, replicas int32) error {
	labels := k.labels(appID, "application")
	labels["app.kubernetes.io/name"] = name
	labels["norbot.component"] = component
	container := hardenedContainer(component, image, nil, k.PVC(workspaceID), "", k.config)
	container.Ports = []corev1.ContainerPort{{ContainerPort: port}}
	container.ReadinessProbe = httpProbe(port, "/")
	container.LivenessProbe = httpProbe(port, "/")
	if component == "backend" {
		container.ReadinessProbe = httpProbe(port, "/api/health")
		container.LivenessProbe = httpProbe(port, "/api/health")
	}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": name}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{ServiceAccountName: k.config.ServiceAccount, AutomountServiceAccountToken: ptr(false), SecurityContext: &corev1.PodSecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, ImagePullSecrets: []corev1.LocalObjectReference{{Name: k.config.RegistryPullSecret}}, Containers: []corev1.Container{container}, Volumes: workspaceVolume(k.PVC(workspaceID))}}}}
	return k.upsertDeployment(ctx, deployment)
}

func httpProbe(port int32, path string) *corev1.Probe {
	return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: path, Port: intstr.FromInt32(port)}}, InitialDelaySeconds: 3, PeriodSeconds: 5, TimeoutSeconds: 3, FailureThreshold: 12}
}
func (k *KubernetesRuntime) applyService(ctx context.Context, name, appID, component string, port int32) error {
	labels := k.labels(appID, "application")
	labels["app.kubernetes.io/name"] = name
	labels["norbot.component"] = component
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app.kubernetes.io/name": name}, Ports: []corev1.ServicePort{{Port: port, TargetPort: intstr.FromInt32(port)}}}}
	_, err := k.client.CoreV1().Services(k.config.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := k.client.CoreV1().Services(k.config.Namespace).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		existing.Spec.Selector = svc.Spec.Selector
		existing.Spec.Ports = svc.Spec.Ports
		_, err = k.client.CoreV1().Services(k.config.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	}
	return err
}
func (k *KubernetesRuntime) applyNetworkPolicy(ctx context.Context, runID, frontend, backend string) error {
	labels := k.labels(runID, "application")
	policy := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: k.Name(runID) + "-network", Labels: labels}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{runLabel: runID, roleLabel: "application"}}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}, Ingress: []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{runLabel: runID, roleLabel: "application"}}}}}}, Egress: []networkingv1.NetworkPolicyEgressRule{{Ports: []networkingv1.NetworkPolicyPort{{Protocol: ptr(corev1.ProtocolUDP), Port: ptr(intstr.FromInt(53))}, {Protocol: ptr(corev1.ProtocolTCP), Port: ptr(intstr.FromInt(53))}, {Protocol: ptr(corev1.ProtocolTCP), Port: ptr(intstr.FromInt(443))}}}, {To: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{runLabel: runID, roleLabel: "application"}}}}}}}}
	if k.config.IngressControllerNamespace != "" {
		policy.Spec.Ingress = append(policy.Spec.Ingress, networkingv1.NetworkPolicyIngressRule{From: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": k.config.IngressControllerNamespace}}}}})
	}
	_, err := k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Create(ctx, policy, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Get(ctx, policy.Name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		existing.Spec = policy.Spec
		_, err = k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	}
	return err
}
func (k *KubernetesRuntime) applyIngress(ctx context.Context, run domain.Run, service string) error {
	host := strings.ToLower(k.Name(run.ID)) + "." + k.config.IngressBaseDomain
	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: k.Name(run.ID), Labels: k.labels(run.ID, "application")},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ptr(k.config.IngressClass),
			Rules: []networkingv1.IngressRule{{
				Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
					Path: "/", PathType: &pathType,
					Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: service, Port: networkingv1.ServiceBackendPort{Number: 80}}},
				}}}},
			}},
		},
	}
	_, err := k.client.NetworkingV1().Ingresses(k.config.Namespace).Create(ctx, ingress, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := k.client.NetworkingV1().Ingresses(k.config.Namespace).Get(ctx, ingress.Name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		existing.Spec = ingress.Spec
		_, err = k.client.NetworkingV1().Ingresses(k.config.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	}
	return err
}
func (k *KubernetesRuntime) upsertDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	_, err := k.client.AppsV1().Deployments(k.config.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	existing, getErr := k.client.AppsV1().Deployments(k.config.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if getErr != nil {
		return getErr
	}
	existing.Spec = deployment.Spec
	existing.Labels = deployment.Labels
	_, err = k.client.AppsV1().Deployments(k.config.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}
func (k *KubernetesRuntime) deleteApplication(ctx context.Context, runID, suffix string) error {
	frontend := k.frontendName(runID)
	backend := k.backendName(runID)
	if suffix != "" {
		frontend += "-" + suffix
		backend += "-" + suffix
	}
	for _, name := range []string{frontend, backend} {
		_ = k.client.AppsV1().Deployments(k.config.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
		_ = k.client.CoreV1().Services(k.config.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	}
	if suffix == "" {
		_ = k.client.NetworkingV1().Ingresses(k.config.Namespace).Delete(ctx, k.Name(runID), metav1.DeleteOptions{})
		_ = k.client.NetworkingV1().NetworkPolicies(k.config.Namespace).Delete(ctx, k.Name(runID)+"-network", metav1.DeleteOptions{})
	}
	return nil
}
func (k *KubernetesRuntime) frontendName(runID string) string { return k.Name(runID) + "-frontend" }
func (k *KubernetesRuntime) backendName(runID string) string  { return k.Name(runID) + "-backend" }
func (k *KubernetesRuntime) repository(runID, component string) string {
	return strings.TrimRight(k.config.RegistryRepository, "/") + "/" + k.Name(runID) + "-" + component + ":" + runID
}
func (k *KubernetesRuntime) publicURL(run domain.Run) string {
	if run.PublicIngress {
		return "https://" + strings.ToLower(k.Name(run.ID)) + "." + k.config.IngressBaseDomain
	}
	return ""
}

func (k *KubernetesRuntime) waitPodReady(ctx context.Context, name string) error {
	return k.wait(ctx, func() (bool, error) {
		pod, err := k.client.CoreV1().Pods(k.config.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		if pod.Status.Phase == corev1.PodFailed {
			return false, fmt.Errorf("workspace pod failed")
		}
		return false, nil
	})
}
func (k *KubernetesRuntime) waitDeployment(ctx context.Context, name string) error {
	return k.wait(ctx, func() (bool, error) {
		d, err := k.client.AppsV1().Deployments(k.config.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		want := int32(1)
		if d.Spec.Replicas != nil {
			want = *d.Spec.Replicas
		}
		return d.Status.AvailableReplicas >= want, nil
	})
}
func (k *KubernetesRuntime) wait(ctx context.Context, check func() (bool, error)) error {
	ticker := time.NewTicker(k.poll)
	defer ticker.Stop()
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func hardenedContainer(name, image string, command []string, pvc, workingDir string, cfg config.Kubernetes) corev1.Container {
	return corev1.Container{Name: name, Image: image, Command: command, WorkingDir: workingDir, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: *resource.NewMilliQuantity(cfg.CPUMilli, resource.DecimalSI), corev1.ResourceMemory: *resource.NewQuantity(cfg.MemoryMiB<<20, resource.BinarySI)}, Limits: corev1.ResourceList{corev1.ResourceCPU: *resource.NewMilliQuantity(cfg.CPUMilli, resource.DecimalSI), corev1.ResourceMemory: *resource.NewQuantity(cfg.MemoryMiB<<20, resource.BinarySI)}}, SecurityContext: restrictedSecurityContext(), VolumeMounts: []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}}}
}
func workspaceVolume(pvc string) []corev1.Volume {
	return []corev1.Volume{{Name: "workspace", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvc}}}}
}
func oneShotJob(name string, cfg config.Kubernetes, runID, role string, container corev1.Container, volumes []corev1.Volume) *batchv1.Job {
	backoff := int32(0)
	deadline := int64(900)
	uid := int64(65532)
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot", runLabel: runID, roleLabel: role}}, Spec: batchv1.JobSpec{BackoffLimit: &backoff, ActiveDeadlineSeconds: &deadline, TTLSecondsAfterFinished: ptr(int32(300)), Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot", runLabel: runID, roleLabel: role}}, Spec: corev1.PodSpec{ServiceAccountName: cfg.ServiceAccount, AutomountServiceAccountToken: ptr(false), RestartPolicy: corev1.RestartPolicyNever, SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: ptr(true), RunAsUser: &uid, FSGroup: &uid, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, ImagePullSecrets: []corev1.LocalObjectReference{{Name: cfg.RegistryPullSecret}}, Containers: []corev1.Container{container}, Volumes: volumes}}}}
}
func (k *KubernetesRuntime) runJob(ctx context.Context, job *batchv1.Job, cleanup bool) (string, error) {
	jobs := k.client.BatchV1().Jobs(k.config.Namespace)
	if _, err := jobs.Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return "", err
	}
	if cleanup {
		defer jobs.Delete(context.Background(), job.Name, metav1.DeleteOptions{PropagationPolicy: ptr(metav1.DeletePropagationBackground)})
	}
	var failed bool
	err := k.wait(ctx, func() (bool, error) {
		current, err := jobs.Get(ctx, job.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if current.Status.Succeeded > 0 {
			return true, nil
		}
		if current.Status.Failed > 0 {
			failed = true
			return true, nil
		}
		return false, nil
	})
	logs, logErr := k.jobLogs(ctx, job.Name)
	if err != nil {
		return logs, err
	}
	if logErr != nil {
		return "", logErr
	}
	if failed {
		return logs, fmt.Errorf("kubernetes job %s failed: %s", job.Name, tail(logs, 2000))
	}
	return logs, nil
}
func (k *KubernetesRuntime) jobLogs(ctx context.Context, jobName string) (string, error) {
	pods, err := k.client.CoreV1().Pods(k.config.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			stream, err := k.client.CoreV1().Pods(k.config.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container.Name}).Stream(ctx)
			if err != nil {
				return "", err
			}
			data, readErr := io.ReadAll(io.LimitReader(stream, 2<<20))
			stream.Close()
			if readErr != nil {
				return "", readErr
			}
			out.Write(data)
		}
	}
	return out.String(), nil
}
func (k *KubernetesRuntime) runSmoke(ctx context.Context, run domain.Run, suffix string) (string, error) {
	name := k.Name(run.ID) + "-smoke-" + shortID()
	service := k.frontendName(run.ID) + "-" + suffix
	command := []string{"sh", "-ceu", "curl -fsS http://" + service + "/; " + func() string {
		if run.Profile != domain.ProfileFrontend {
			return "curl -fsS http://" + k.backendName(run.ID) + "-" + suffix + ":8000/api/health"
		}
		return "true"
	}()}
	container := hardenedContainer("smoke", "curlimages/curl:8.12.1", command, k.PVC(run.ID), "/workspace", k.config)
	return k.runJob(ctx, oneShotJob(name, k.config, run.ID, "smoke", container, workspaceVolume(k.PVC(run.ID))), true)
}

func tarPath(source, destination string) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		tw := tar.NewWriter(writer)
		err := filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			name := filepath.ToSlash(filepath.Join(destination, relative))
			if relative == "." {
				name = destination
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = name
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				_, err = io.Copy(tw, file)
				closeErr := file.Close()
				if err != nil {
					return err
				}
				return closeErr
			}
			return nil
		})
		if closeErr := tw.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
	}()
	return reader, nil
}
func extractTar(reader io.Reader, destination string) error {
	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if !safeRelative(header.Name) {
			return fmt.Errorf("unsafe archive path")
		}
		target := filepath.Join(destination, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, tr)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported archive entry")
		}
	}
}
func safeRelative(path string) bool {
	clean := filepath.Clean(path)
	return path != "" && !filepath.IsAbs(path) && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
func shellArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}
func shortID() string { return fmt.Sprintf("%x", time.Now().UnixNano())[:8] }
func containerStatuses(statuses []corev1.ContainerStatus) []map[string]any {
	result := make([]map[string]any, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, map[string]any{"name": status.Name, "ready": status.Ready, "restart_count": status.RestartCount, "image": status.Image})
	}
	return result
}
