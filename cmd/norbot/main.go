package main

import (
	"bufio"
	"context"
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
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gongahkia/norbot/internal/api"
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
	if len(os.Args) > 2 && os.Args[1] == "live-e2e" && os.Args[2] == "inbound" {
		liveInboundCommand(os.Args[3:])
		return
	}
	fmt.Fprintln(os.Stderr, "usage: norbot init | norbot kube bootstrap | norbot serve | norbot health [--json] | norbot live-e2e inbound | norbot tui --api http://127.0.0.1:8080")
	os.Exit(2)
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
		fmt.Fprintln(os.Stderr, "usage: norbot kube bootstrap | norbot kube secret-template")
		os.Exit(2)
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
		fmt.Fprintln(os.Stderr, "usage: norbot kube bootstrap | norbot kube secret-template")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("kubernetes bootstrap complete")
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
	service := engine.NewWithExtensions(st, cfg, logger, extensions)
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
	channels := channel.New(st, service, cfg.ArtifactsDir)
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
