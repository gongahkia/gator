package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gongahkia/norbot/internal/api"
	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/engine"
	"github.com/gongahkia/norbot/internal/extension"
	"github.com/gongahkia/norbot/internal/runtime"
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
	fmt.Fprintln(os.Stderr, "usage: norbot init | norbot kube bootstrap | norbot serve | norbot tui --api http://127.0.0.1:8080")
	os.Exit(2)
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
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: api.New(service, st, logger).Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = api.Shutdown(stopCtx, server)
	}()
	logger.Info("norbot listening", "addr", cfg.HTTPAddr, "workers", cfg.Workers)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("serve", "error", err)
		os.Exit(1)
	}
}
