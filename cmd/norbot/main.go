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
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gongahkia/norbot/internal/api"
	"github.com/gongahkia/norbot/internal/channel"
	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/engine"
	"github.com/gongahkia/norbot/internal/eval"
	"github.com/gongahkia/norbot/internal/extension"
	"github.com/gongahkia/norbot/internal/runtime"
	"github.com/gongahkia/norbot/internal/skill"
	"github.com/gongahkia/norbot/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "init":
		initCommand(os.Args[2:])
	case "serve":
		serveCommand(os.Args[2:])
	case "health":
		healthCommand(os.Args[2:])
	case "local":
		localCommand(os.Args[2:])
	case "eval":
		evalCommand(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: norbot init | serve | health [--json] | local doctor|reset --yes | eval validate|run|score")
	os.Exit(2)
}

func initCommand(args []string) {
	flags := flag.NewFlagSet("init", flag.ExitOnError)
	path := flags.String("config", "config.json", "config output path")
	force := flags.Bool("force", false, "overwrite config")
	_ = flags.Parse(args)
	if err := config.WriteManifest(*path, config.InitialManifest(domain.DeploymentDocker, config.Kubernetes{}), *force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *path)
}

func healthCommand(args []string) {
	flags := flag.NewFlagSet("health", flag.ExitOnError)
	asJSON := flags.Bool("json", false, "write JSON")
	_ = flags.Parse(args)
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := openStore(ctx, cfg)
	if err != nil {
		fatal(err)
	}
	defer st.Close()
	report := engine.New(st, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))).Health(ctx)
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(report)
	} else {
		fmt.Println(strings.ToUpper(string(report.State)))
		for _, check := range report.Checks {
			fmt.Printf("%-20s %-9s %4dms %s\n", check.ID, check.State, check.LatencyMS, check.Message)
		}
	}
	if report.State == domain.HealthDown {
		os.Exit(1)
	}
}

func localCommand(args []string) {
	if len(args) != 1 && !(len(args) == 2 && args[0] == "reset" && args[1] == "--yes") {
		usage()
	}
	switch args[0] {
	case "doctor":
		healthCommand(nil)
	case "reset":
		if len(args) != 2 || args[1] != "--yes" {
			fatal(errors.New("local reset requires --yes"))
		}
		cfg, err := config.Load()
		if err != nil {
			fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if err := runtime.ResetLocal(ctx, runtime.NewDockerClient(cfg.DockerBin, cfg.Manifest.Runtime.Docker, runtime.OSRunner{})); err != nil {
			fatal(err)
		}
		st, err := openStore(ctx, cfg)
		if err != nil {
			fatal(err)
		}
		defer st.Close()
		if err := st.ResetLocal(ctx); err != nil {
			fatal(err)
		}
		fmt.Println("Norbot local state reset")
	default:
		usage()
	}
}

func evalCommand(args []string) {
	if len(args) == 0 {
		usage()
	}
	switch args[0] {
	case "validate":
		flags := flag.NewFlagSet("eval validate", flag.ExitOnError)
		dir := flags.String("dir", "evals/v1", "corpus directory")
		_ = flags.Parse(args[1:])
		corpus, err := eval.Load(*dir)
		if err != nil {
			fatal(err)
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
			fatal(errors.New("eval run requires --provider, --model, and positive --max-cost"))
		}
		corpus, err := eval.Load(*dir)
		if err != nil {
			fatal(err)
		}
		results := eval.DeterministicResults(corpus, *providerID, *model, *caseID)
		score, err := eval.ScoreResults(corpus, results)
		if err != nil {
			fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"mode": "deterministic_contract_evaluation", "provider": *providerID, "model": *model, "max_cost": *maxCost, "results": results, "score": score})
	case "score":
		flags := flag.NewFlagSet("eval score", flag.ExitOnError)
		dir := flags.String("dir", "evals/v1", "corpus directory")
		input := flags.String("input", "", "results JSON path")
		_ = flags.Parse(args[1:])
		if *input == "" {
			fatal(errors.New("eval score requires --input"))
		}
		corpus, err := eval.Load(*dir)
		if err != nil {
			fatal(err)
		}
		data, err := os.ReadFile(*input)
		if err != nil {
			fatal(err)
		}
		var results []eval.Result
		if err := json.Unmarshal(data, &results); err != nil {
			fatal(err)
		}
		score, err := eval.ScoreResults(corpus, results)
		if err != nil {
			fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(score)
	default:
		usage()
	}
}

func serveCommand(args []string) {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	_ = flags.Parse(args)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	if !loopback(cfg.HTTPAddr) || !loopback(cfg.WebhookAddr) {
		fatal(errors.New("local console and webhook listeners must bind to loopback"))
	}
	docker := runtime.NewDockerClient(cfg.DockerBin, cfg.Manifest.Runtime.Docker, runtime.OSRunner{})
	if err := docker.Validate(context.Background()); err != nil {
		fatal(err)
	}
	plugins, err := extension.LoadProcessPlugins(cfg.Manifest.Plugins)
	if err != nil {
		fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	extensions := extension.NewRegistry()
	if err := extension.RegisterProcessPlugins(ctx, extensions, plugins); err != nil {
		fatal(err)
	}
	st, err := openStore(ctx, cfg)
	if err != nil {
		fatal(err)
	}
	defer st.Close()
	service := engine.NewWithExtensionsAndArtifacts(st, cfg, logger, extensions, nil)
	service.StartWorkers(ctx)
	skills := skill.New(st, cfg.ArtifactsDir)
	channels := channel.New(st, service, cfg.ArtifactsDir, nil)
	app := api.NewWithComponents(service, st, logger, skills, channels)
	console := &http.Server{Addr: cfg.HTTPAddr, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 16 << 10}
	webhooks := &http.Server{Addr: cfg.WebhookAddr, Handler: app.WebhookHandler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 16 << 10}
	errCh := make(chan error, 2)
	go func() { errCh <- console.ListenAndServe() }()
	go func() { errCh <- webhooks.ListenAndServe() }()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve", "error", err)
		}
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	_ = console.Shutdown(shutdownCtx)
	_ = webhooks.Shutdown(shutdownCtx)
}

func openStore(ctx context.Context, cfg config.Config) (*store.Store, error) {
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := st.Migrate(ctx); err != nil {
		st.Close()
		return nil, err
	}
	return st, nil
}

func loopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return true
	}
	return host == "0.0.0.0" && os.Getenv("NORBOT_CONTAINER_LOCAL") == "true"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
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
	if value = strings.TrimSpace(value); value == "" {
		return fallback
	}
	return value
}
