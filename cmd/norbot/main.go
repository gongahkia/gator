package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gongahkia/norbot/internal/api"
	"github.com/gongahkia/norbot/internal/artifact"
	"github.com/gongahkia/norbot/internal/channel"
	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/egress"
	"github.com/gongahkia/norbot/internal/engine"
	"github.com/gongahkia/norbot/internal/eval"
	"github.com/gongahkia/norbot/internal/extension"
	"github.com/gongahkia/norbot/internal/runtime"
	"github.com/gongahkia/norbot/internal/skill"
	"github.com/gongahkia/norbot/internal/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		initCommand(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "kube" {
		kubeCommand(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		serveCommand(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "health" {
		healthCommand(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "egress-proxy" {
		egressProxyCommand(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "eval" {
		evalCommand(os.Args[2:])
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "live-e2e" && os.Args[2] == "inbound" {
		liveInboundCommand(os.Args[3:])
		return
	}
	fmt.Fprintln(os.Stderr, "usage: norbot init | norbot kube bootstrap|local|secret-template|network-policy-check | norbot serve | norbot egress-proxy | norbot health [--json] | norbot eval validate|run|score | norbot live-e2e inbound")
	os.Exit(2)
}

func evalCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: norbot eval validate|run|score")
		os.Exit(2)
	}
	switch args[0] {
	case "validate":
		flags := flag.NewFlagSet("eval validate", flag.ExitOnError)
		dir := flags.String("dir", "evals/v1", "corpus directory")
		_ = flags.Parse(args[1:])
		corpus, err := eval.Load(*dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%s: %d cases valid\n", corpus.Version, len(corpus.Cases))
	case "run":
		flags := flag.NewFlagSet("eval run", flag.ExitOnError)
		dir := flags.String("dir", "evals/v1", "corpus directory")
		providerID := flags.String("provider", "", "provider label")
		model := flags.String("model", "", "model label")
		caseID := flags.String("case", "", "single case")
		maxCost := flags.Float64("max-cost", 0, "declared cost ceiling")
		_ = flags.Parse(args[1:])
		if *providerID == "" || *model == "" || *maxCost <= 0 {
			fmt.Fprintln(os.Stderr, "eval run requires --provider, --model, and positive --max-cost")
			os.Exit(2)
		}
		corpus, err := eval.Load(*dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		results := eval.DeterministicResults(corpus, *providerID, *model, *caseID)
		if *caseID != "" && len(results) == 0 {
			fmt.Fprintln(os.Stderr, "unknown eval case")
			os.Exit(2)
		}
		score, err := eval.ScoreResults(corpus, results)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"mode": "deterministic_contract_evaluation", "provider": *providerID, "model": *model, "max_cost": *maxCost, "results": results, "score": score})
	case "score":
		flags := flag.NewFlagSet("eval score", flag.ExitOnError)
		dir := flags.String("dir", "evals/v1", "corpus directory")
		input := flags.String("input", "", "results JSON path")
		_ = flags.Parse(args[1:])
		if *input == "" {
			fmt.Fprintln(os.Stderr, "eval score requires --input")
			os.Exit(2)
		}
		corpus, err := eval.Load(*dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		data, err := os.ReadFile(*input)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var results []eval.Result
		if err := json.Unmarshal(data, &results); err != nil {
			var envelope struct {
				Results []eval.Result `json:"results"`
			}
			if envelopeErr := json.Unmarshal(data, &envelope); envelopeErr != nil || envelope.Results == nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			results = envelope.Results
		}
		score, err := eval.ScoreResults(corpus, results)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(score)
	default:
		fmt.Fprintln(os.Stderr, "usage: norbot eval validate|run|score")
		os.Exit(2)
	}
}

func egressProxyCommand(args []string) {
	flags := flag.NewFlagSet("egress-proxy", flag.ExitOnError)
	address := flags.String("addr", envOr("NORBOT_EGRESS_PROXY_ADDR", ":8181"), "listen address")
	_ = flags.Parse(args)
	secret := strings.TrimSpace(os.Getenv("NORBOT_EGRESS_PROXY_SECRET"))
	server, err := egress.Serve(*address, secret)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func liveInboundCommand(args []string) {
	flags := flag.NewFlagSet("live-e2e inbound", flag.ExitOnError)
	account := flags.String("account", "", "channel account id")
	external := flags.String("external", "", "dedicated human test identity")
	marker := flags.String("marker", "", "unique message marker")
	timeout := flags.Duration("timeout", 10*time.Minute, "maximum wait")
	_ = flags.Parse(args)
	if *account == "" || *external == "" || *marker == "" {
		fmt.Fprintln(os.Stderr, "usage: norbot live-e2e inbound --account ID --external ID --marker TEXT")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("send marker from the dedicated human identity:", *marker)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	seen := false
	for {
		messages, err := st.ChannelMessages(ctx, *account, *external)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, message := range messages {
			if message.Direction == "inbound" && strings.Contains(message.Text, *marker) {
				seen = true
			}
			if seen && message.Direction == "outbound" && message.State == "delivered" {
				fmt.Println("live inbound and reply delivery verified")
				return
			}
		}
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "live inbound assertion timed out")
			os.Exit(1)
		case <-ticker.C:
		}
	}
}

func healthCommand(args []string) {
	flags := flag.NewFlagSet("health", flag.ExitOnError)
	asJSON := flags.Bool("json", false, "write JSON")
	_ = flags.Parse(args)
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	report := engine.New(st, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))).Health(ctx)
	if *asJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("%s\n", strings.ToUpper(string(report.State)))
		for _, check := range report.Checks {
			fmt.Printf("%-28s %-9s %4dms %s\n", check.ID, check.State, check.LatencyMS, check.Message)
		}
	}
	if report.State == domain.HealthDown {
		os.Exit(1)
	}
}

func initCommand(args []string) {
	flags := flag.NewFlagSet("init", flag.ExitOnError)
	path := flags.String("config", "config.json", "config output path")
	target := flags.String("target", "", "docker or kubernetes")
	force := flags.Bool("force", false, "overwrite config")
	_ = flags.Parse(args)
	reader := bufio.NewReader(os.Stdin)
	selected := domain.DeploymentTarget(strings.TrimSpace(*target))
	if selected == "" {
		selected = domain.DeploymentTarget(prompt(reader, "Deployment target [docker/kubernetes]", "docker"))
	}
	if !selected.Valid() {
		fmt.Fprintln(os.Stderr, "target must be docker or kubernetes")
		os.Exit(2)
	}
	kube := config.Kubernetes{}
	if selected == domain.DeploymentKubernetes {
		kube.Kubeconfig = prompt(reader, "Kubeconfig path", "")
		kube.Context = prompt(reader, "Kubernetes context (optional)", "")
		kube.Namespace = prompt(reader, "Dedicated namespace", "norbot")
		kube.ServiceAccount = prompt(reader, "Service account", "norbot-runtime")
		kube.RegistryRepository = prompt(reader, "OCI registry repository", "")
		kube.RegistryPullSecret = prompt(reader, "Existing registry pull Secret", "registry-pull")
		if prompt(reader, "Configure ingress now? [y/N]", "n") == "y" {
			kube.IngressClass = prompt(reader, "Ingress class", "")
			kube.IngressBaseDomain = prompt(reader, "Ingress base domain", "")
			kube.IngressControllerNamespace = prompt(reader, "Ingress controller namespace", "ingress-nginx")
		}
	}
	if err := config.WriteManifest(*path, config.InitialManifest(selected, kube), *force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *path)
	if selected == domain.DeploymentKubernetes {
		fmt.Println("next: create the registry secret, then run: norbot kube bootstrap")
	}
}

func kubeCommand(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: norbot kube bootstrap | norbot kube local | norbot kube secret-template | norbot kube network-policy-check")
		os.Exit(2)
	}
	if args[0] == "local" {
		kubeLocalCommand(args[1:])
		return
	}
	flags := flag.NewFlagSet("kube "+args[0], flag.ExitOnError)
	probeTimeout := flags.Duration("timeout", 2*time.Minute, "maximum duration for network policy enforcement probe")
	_ = flags.Parse(args[1:])
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	kube, err := runtime.NewKubernetesRuntime(cfg.Manifest.Runtime.Kubernetes, cfg.ArtifactsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	switch args[0] {
	case "bootstrap":
		err = kube.Bootstrap(context.Background())
	case "secret-template":
		fmt.Print(runtime.RegistrySecretTemplate(cfg.Manifest.Runtime.Kubernetes.RegistryPullSecret))
		return
	case "network-policy-check":
		ctx, cancel := context.WithTimeout(context.Background(), *probeTimeout)
		defer cancel()
		err = kube.VerifyNetworkPolicyEnforcement(ctx)
	default:
		fmt.Fprintln(os.Stderr, "usage: norbot kube bootstrap | norbot kube local | norbot kube secret-template | norbot kube network-policy-check")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if args[0] == "network-policy-check" {
		fmt.Println("network policy enforcement verified")
		return
	}
	fmt.Println("kubernetes bootstrap complete")
}

func kubeLocalCommand(args []string) {
	flags := flag.NewFlagSet("kube local", flag.ExitOnError)
	name := flags.String("name", "norbot-local", "kind cluster name")
	namespace := flags.String("namespace", "norbot", "Norbot namespace")
	registryPort := flags.Int("registry-port", 5001, "host port for the local OCI registry")
	configPath := flags.String("config", "config.local-kubernetes.json", "local Kubernetes config output")
	envPath := flags.String("env", ".norbot/local-kubernetes.env", "local secret environment file")
	cilium := flags.Bool("cilium", false, "create a new Kind cluster with Cilium NetworkPolicy enforcement")
	verifyPolicy := flags.Bool("verify-network-policy", false, "run a real deny-egress NetworkPolicy enforcement probe")
	confirmPolicy := flags.Bool("confirm-network-policy", false, "deprecated alias for --verify-network-policy")
	force := flags.Bool("force", false, "overwrite generated local config and environment files")
	_ = flags.Parse(args)
	if !validKubeLocalName(*name) || !validKubeLocalName(*namespace) || *registryPort < 1024 || *registryPort > 65535 {
		fmt.Fprintln(os.Stderr, "name/namespace must be lowercase DNS labels and registry-port must be 1024..65535")
		os.Exit(2)
	}
	if !*force {
		if _, err := os.Stat(*configPath); err == nil {
			fmt.Fprintln(os.Stderr, "config", *configPath, "already exists; use --force")
			os.Exit(1)
		} else if !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if _, err := os.Stat("Dockerfile"); err != nil {
		fmt.Fprintln(os.Stderr, "run norbot kube local from the Norbot repository root:", err)
		os.Exit(1)
	}
	binaries := []string{"docker", "kind", "kubectl"}
	if *cilium {
		binaries = append(binaries, "cilium")
	}
	for _, binary := range binaries {
		if _, err := exec.LookPath(binary); err != nil {
			fmt.Fprintf(os.Stderr, "local Kubernetes requires %s in PATH\n", binary)
			os.Exit(1)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := localCommand(ctx, "", "docker", "info"); err != nil {
		fmt.Fprintln(os.Stderr, "Docker daemon is unavailable:", err)
		os.Exit(1)
	}
	clusters, err := localCommand(ctx, "", "kind", "get", "clusters")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exists := linePresent(clusters, *name)
	if *cilium && exists {
		fmt.Fprintln(os.Stderr, "--cilium requires a new Kind cluster; choose an unused --name")
		os.Exit(1)
	}
	if !exists {
		if err := createLocalKind(ctx, *name, *cilium); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := ensureLocalRegistry(ctx, *registryPort); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	contextName := "kind-" + *name
	if *cilium {
		if _, err := localCommand(ctx, "", "cilium", "install", "--context", contextName, "--wait"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if _, err := localCommand(ctx, "", "cilium", "status", "--context", contextName, "--wait"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	registryConfig := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: local-registry-hosting\n  namespace: kube-public\ndata:\n  localRegistryHosting.v1: |\n    host: \"kind-registry:5000\"\n    help: \"https://kind.sigs.k8s.io/docs/user/local-registry/\"\n"
	if _, err := localCommandInput(ctx, "", registryConfig, "kubectl", "--context", contextName, "apply", "-f", "-"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	secret, err := localEgressSecret(*envPath, *force)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	resources := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: " + *namespace + "\n---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: registry-pull\n  namespace: " + *namespace + "\ntype: kubernetes.io/dockerconfigjson\nstringData:\n  .dockerconfigjson: '{\"auths\":{}}'\n---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: norbot-egress-proxy\n  namespace: " + *namespace + "\ntype: Opaque\nstringData:\n  secret: \"" + secret + "\"\n"
	if _, err := localCommandInput(ctx, "", resources, "kubectl", "--context", contextName, "apply", "-f", "-"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	proxyHostImage := fmt.Sprintf("localhost:%d/norbot-egress-proxy:local", *registryPort)
	if _, err := localCommand(ctx, ".", "docker", "build", "-t", proxyHostImage, "."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := localCommand(ctx, "", "docker", "push", proxyHostImage); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	hostKubeconfig := filepath.Join(userHome, ".kube", "config")
	if paths := filepath.SplitList(strings.TrimSpace(os.Getenv("KUBECONFIG"))); len(paths) > 0 && paths[0] != "" {
		hostKubeconfig = paths[0]
	}
	kube := config.Kubernetes{Kubeconfig: "/etc/norbot/kubeconfig", Context: contextName, Namespace: *namespace, ServiceAccount: "norbot-runtime", RegistryRepository: "kind-registry:5000/norbot", RegistryPullSecret: "registry-pull", RegistryInsecure: true, EgressProxyImage: "kind-registry:5000/norbot-egress-proxy:local", EgressProxySecret: "norbot-egress-proxy", EgressProxySecretKey: "secret", EgressProxyPort: 8181}
	bootstrapKube := kube
	bootstrapKube.Kubeconfig = hostKubeconfig
	kubeRuntime, err := runtime.NewKubernetesRuntime(bootstrapKube, ".norbot/artifacts")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := kubeRuntime.Bootstrap(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *verifyPolicy || *confirmPolicy || *cilium {
		if err := kubeRuntime.VerifyNetworkPolicyEnforcement(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "network policy enforcement check failed:", err)
			os.Exit(1)
		}
		kube.NetworkPolicyEnforced = true
	}
	manifestKube := kube
	manifest := config.InitialManifest(domain.DeploymentKubernetes, manifestKube)
	manifest.Runtime.Sandbox = config.Sandbox{Image: "alpine:3.21", CPUMilli: 500, MemoryMiB: 512, TimeoutS: 60}
	if kube.NetworkPolicyEnforced {
		manifest.Runtime.Sandbox.EgressProxyURL = "http://norbot-egress-proxy." + *namespace + ".svc.cluster.local:8181"
		manifest.Runtime.Sandbox.EgressProxySecret = "NORBOT_EGRESS_PROXY_SECRET"
	} else {
		manifest.Runtime.Kubernetes.EgressProxyImage = ""
		manifest.Runtime.Kubernetes.EgressProxySecret = ""
		manifest.Runtime.Kubernetes.EgressProxySecretKey = ""
		manifest.Runtime.Kubernetes.EgressProxyPort = 0
	}
	if err := config.WriteManifest(*configPath, manifest, *force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("local Kubernetes bootstrap complete")
	fmt.Println("start Norbot with: set -a; source " + *envPath + "; set +a; NORBOT_CONFIG_HOST=" + *configPath + " NORBOT_KUBECONFIG_HOST=" + hostKubeconfig + " docker compose up --build")
	if !*verifyPolicy && !*confirmPolicy && !*cilium {
		fmt.Println("HTTPS sandbox tools are fail-closed until a NetworkPolicy-enforcing CNI is installed and you rerun with --verify-network-policy.")
	}
}

func createLocalKind(ctx context.Context, name string, cilium bool) error {
	args := []string{"create", "cluster", "--name", name}
	if !cilium {
		_, err := localCommand(ctx, "", "kind", args...)
		return err
	}
	file, err := os.CreateTemp("", "norbot-kind-cilium-*.yaml")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.WriteString("kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnetworking:\n  disableDefaultCNI: true\n"); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	args = append(args, "--config", path)
	_, err = localCommand(ctx, "", "kind", args...)
	return err
}

func ensureLocalRegistry(ctx context.Context, port int) error {
	if _, err := localCommand(ctx, "", "docker", "inspect", "kind-registry"); err != nil {
		if _, err := localCommand(ctx, "", "docker", "run", "-d", "--restart=always", "-p", fmt.Sprintf("127.0.0.1:%d:5000", port), "--network", "bridge", "--name", "kind-registry", "registry:3"); err != nil {
			return err
		}
	}
	if _, err := localCommand(ctx, "", "docker", "network", "connect", "kind", "kind-registry"); err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	return nil
}

func localEgressSecret(path string, force bool) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		if !force {
			for _, line := range strings.Split(string(data), "\n") {
				if value, ok := strings.CutPrefix(line, "NORBOT_EGRESS_PROXY_SECRET="); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value), nil
				}
			}
			return "", fmt.Errorf("local secret file has no NORBOT_EGRESS_PROXY_SECRET")
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(bytes)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte("NORBOT_EGRESS_PROXY_SECRET="+secret+"\n"), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

func localCommand(ctx context.Context, dir, command string, args ...string) (string, error) {
	process := exec.CommandContext(ctx, command, args...)
	process.Dir = dir
	output, err := process.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", command, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func localCommandInput(ctx context.Context, dir, input, command string, args ...string) (string, error) {
	process := exec.CommandContext(ctx, command, args...)
	process.Dir = dir
	process.Stdin = strings.NewReader(input)
	output, err := process.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", command, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func linePresent(output, target string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
}

func validKubeLocalName(value string) bool {
	if value == "" || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '-' {
			return false
		}
	}
	return true
}

func prompt(reader *bufio.Reader, label, fallback string) string {
	if fallback == "" {
		fmt.Printf("%s: ", label)
	} else {
		fmt.Printf("%s [%s]: ", label, fallback)
	}
	value, err := reader.ReadString('\n')
	if err != nil && len(value) == 0 {
		return fallback
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func serveCommand(args []string) {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	_ = flags.Parse(args)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	if host, _, splitErr := net.SplitHostPort(cfg.HTTPAddr); splitErr == nil && host != "127.0.0.1" && host != "::1" && host != "localhost" && cfg.Manifest.Security.OIDC.Issuer == "" && !cfg.AllowUnauthenticatedLocal {
		fmt.Fprintln(os.Stderr, "public norbot API requires security.oidc configuration")
		os.Exit(1)
	}
	if cfg.Manifest.Forensics.RawCapture {
		host, _, splitErr := net.SplitHostPort(cfg.HTTPAddr)
		if splitErr != nil || (host != "127.0.0.1" && host != "::1" && host != "localhost") {
			fmt.Fprintln(os.Stderr, "forensics raw_capture requires a loopback NORBOT_HTTP_ADDR")
			os.Exit(1)
		}
	}
	if cfg.Manifest.DefaultTarget() == domain.DeploymentDocker {
		docker := runtime.NewDockerClient(cfg.DockerBin, cfg.Manifest.Runtime.Docker, runtime.OSRunner{})
		if err := docker.Validate(context.Background()); err != nil {
			logger.Error("validate Docker runtime", "error", err)
			os.Exit(1)
		}
	}
	plugins, err := extension.LoadProcessPlugins(cfg.Manifest.Plugins)
	if err != nil {
		logger.Error("load process plugins", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	extensions := extension.NewRegistry()
	if err := extension.RegisterProcessPlugins(ctx, extensions, plugins); err != nil {
		logger.Error("register process plugins", "error", err)
		os.Exit(1)
	}
	shutdownTelemetry, err := api.SetupTelemetry(ctx, cfg.OTelEndpoint)
	if err != nil {
		logger.Error("setup telemetry", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTelemetry(context.Background()) }()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		logger.Error("migrate database", "error", err)
		os.Exit(1)
	}
	if err := st.ConfigureForensics(cfg.Manifest.Forensics.RawCapture, cfg.Manifest.Forensics.MasterKeyEnv); err != nil {
		logger.Error("configure local forensics", "error", err)
		os.Exit(1)
	}
	objects, err := artifact.New(cfg.Manifest.Artifacts)
	if err != nil {
		logger.Error("configure artifact storage", "error", err)
		os.Exit(1)
	}
	service := engine.NewWithExtensionsAndArtifacts(st, cfg, logger, extensions, objects)
	service.StartWorkers(ctx)
	var proxy *http.Server
	if cfg.Manifest.Runtime.Sandbox.EgressProxyURL != "" {
		parsed, parseErr := url.Parse(cfg.Manifest.Runtime.Sandbox.EgressProxyURL)
		secret := os.Getenv(cfg.Manifest.Runtime.Sandbox.EgressProxySecret)
		if parseErr != nil || parsed.Port() == "" || secret == "" {
			logger.Error("invalid sandbox egress proxy configuration")
			os.Exit(1)
		}
		address := os.Getenv("NORBOT_EGRESS_PROXY_ADDR")
		if address == "" {
			address = "0.0.0.0:" + parsed.Port()
		}
		proxy, err = egress.Serve(address, secret)
		if err != nil {
			logger.Error("create egress proxy", "error", err)
			os.Exit(1)
		}
		go func() {
			if err := proxy.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("egress proxy", "error", err)
			}
		}()
	}
	skills := skill.New(st, cfg.ArtifactsDir)
	channels := channel.New(st, service, cfg.ArtifactsDir, objects)
	channels.Start(ctx)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: api.NewWithComponents(service, st, logger, skills, channels).WithOIDC(cfg.Manifest.Security.OIDC).WithSecurity(cfg.Manifest.Security).Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = api.Shutdown(stopCtx, server)
		if proxy != nil {
			_ = proxy.Shutdown(stopCtx)
		}
	}()
	logger.Info("norbot listening", "addr", cfg.HTTPAddr, "workers", cfg.Workers)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("serve", "error", err)
		os.Exit(1)
	}
}
