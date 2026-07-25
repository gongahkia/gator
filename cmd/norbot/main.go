package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gongahkia/norbot/internal/api"
	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/engine"
	"github.com/gongahkia/norbot/internal/extension"
	"github.com/gongahkia/norbot/internal/store"
	"github.com/gongahkia/norbot/internal/tui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tui" {
		tuiCommand(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		serveCommand(os.Args[2:])
		return
	}
	fmt.Fprintln(os.Stderr, "usage: norbot serve | norbot tui --api http://127.0.0.1:8080")
	os.Exit(2)
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
