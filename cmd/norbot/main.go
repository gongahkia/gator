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
	"github.com/gongahkia/norbot/internal/extension"
	"github.com/gongahkia/norbot/internal/runtime"
	"github.com/gongahkia/norbot/internal/skill"
	"github.com/gongahkia/norbot/internal/store"
	"github.com/gongahkia/norbot/internal/tui"
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
	if len(os.Args) > 1 && os.Args[1] == "tui" {
		tuiCommand(os.Args[2:])
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
	if len(os.Args) > 2 && os.Args[1] == "live-e2e" && os.Args[2] == "inbound" {
		liveInboundCommand(os.Args[3:])
		return
	}
	fmt.Fprintln(os.Stderr, "usage: norbot init | norbot kube bootstrap|local|secret-template | norbot serve | norbot egress-proxy | norbot health [--json] | norbot live-e2e inbound | norbot tui --api http://127.0.0.1:8080")
	os.Exit(2)
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
		fmt.Fprintln(os.Stderr, "usage: norbot kube bootstrap | norbot kube local | norbot kube secret-template")
		os.Exit(2)
	}
	if args[0] == "local" {
		kubeLocalCommand(args[1:])
		return
	}
	flags := flag.NewFlagSet("kube "+args[0], flag.ExitOnError)
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
	default:
		fmt.Fprintln(os.Stderr, "usage: norbot kube bootstrap | norbot kube local | norbot kube secret-template")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
	confirmPolicy := flags.Bool("confirm-network-policy", false, "confirm the installed CNI enforces NetworkPolicy")
	force := flags.Bool("force", false, "overwrite generated local config and environment files")
	_ = flags.Parse(args)
	if !validKubeLocalName(*name) || !validKubeLocalName(*namespace) || *registryPort < 1024 || *registryPort > 65535 {
		fmt.Fprintln(os.Stderr, "name/namespace must be lowercase DNS labels and registry-port must be 1024..65535")
		os.Exit(2)
	}
	if _, err := os.Stat("Dockerfile"); err != nil {
		fmt.Fprintln(os.Stderr, "run norbot kube local from the Norbot repository root:", err)
		os.Exit(1)
	}
	for _, binary := range []string{"docker", "kind", "kubectl"} {
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
	if !linePresent(clusters, *name) {
		if _, err := localCommand(ctx, "", "kind", "create", "cluster", "--name", *name); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := ensureLocalRegistry(ctx, *registryPort); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	contextName := "kind-" + *name
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
	kube := config.Kubernetes{Kubeconfig: "/etc/norbot/kubeconfig", Context: contextName, Namespace: *namespace, ServiceAccount: "norbot-runtime", RegistryRepository: "kind-registry:5000/norbot", RegistryPullSecret: "registry-pull", RegistryInsecure: true, EgressProxyImage: "kind-registry:5000/norbot-egress-proxy:local", EgressProxySecret: "norbot-egress-proxy", EgressProxySecretKey: "secret", EgressProxyPort: 8181, NetworkPolicyEnforced: *confirmPolicy}
	manifest := config.InitialManifest(domain.DeploymentKubernetes, kube)
	manifest.Runtime.Sandbox = config.Sandbox{Image: "alpine:3.21", CPUMilli: 500, MemoryMiB: 512, TimeoutS: 60, EgressProxyURL: "http://norbot-egress-proxy." + *namespace + ".svc.cluster.local:8181", EgressProxySecret: "NORBOT_EGRESS_PROXY_SECRET"}
	if err := config.WriteManifest(*configPath, manifest, *force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
	fmt.Println("local Kubernetes bootstrap complete")
	fmt.Println("start Norbot with: set -a; source " + *envPath + "; set +a; NORBOT_CONFIG_HOST=" + *configPath + " NORBOT_KUBECONFIG_HOST=" + hostKubeconfig + " docker compose up --build")
	if !*confirmPolicy {
		fmt.Println("HTTPS sandbox tools are fail-closed until a NetworkPolicy-enforcing CNI is installed and you rerun with --confirm-network-policy.")
	}
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

func tuiCommand(args []string) {
	flags := flag.NewFlagSet("tui", flag.ExitOnError)
	apiBase := flags.String("api", "http://127.0.0.1:8080", "Norbot API base URL")
	_ = flags.Parse(args)
	if err := tui.Run(*apiBase); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
	if host, _, splitErr := net.SplitHostPort(cfg.HTTPAddr); splitErr == nil && host != "127.0.0.1" && host != "::1" && host != "localhost" && cfg.Manifest.Security.OIDC.Issuer == "" {
		fmt.Fprintln(os.Stderr, "public norbot API requires security.oidc configuration")
		os.Exit(1)
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
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: api.NewWithComponents(service, st, logger, skills, channels).WithOIDC(cfg.Manifest.Security.OIDC).Handler(), ReadHeaderTimeout: 10 * time.Second}
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
