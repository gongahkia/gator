package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"os/signal"
	"time"

	"github.com/gongahkia/gator/internal/reviewweb"
)

type reviewWebOptions struct {
	statePath string
	listen    string
	open      bool
}

func reviewCommand(arguments []string, out io.Writer) error {
	options, err := parseReviewWebOptions(arguments)
	if err != nil {
		return err
	}
	server, err := reviewweb.New(reviewweb.Config{StatePath: options.statePath})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", options.listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", options.listen, err)
	}
	defer listener.Close()
	url, err := server.BootstrapURL(listener.Addr().String())
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), serveSignals()...)
	defer stop()
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	if _, err := fmt.Fprintf(out, "Gator browser review (loopback-only)\n  URL: %s\n  Scope: retained worktree only; the active checkout is untouched\n  Security: one-use URL, HttpOnly same-site session, no RPC access\n  Stop: Ctrl+C\n", url); err != nil {
		return err
	}
	if options.open {
		if err := exec.Command("xdg-open", url).Start(); err != nil {
			if _, outputErr := fmt.Fprintf(out, "Could not open a browser automatically: %v\n", err); outputErr != nil {
				return outputErr
			}
		}
	}
	err = httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func parseReviewWebOptions(arguments []string) (reviewWebOptions, error) {
	flags := flag.NewFlagSet("review", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", "127.0.0.1:0", "loopback TCP address to listen on")
	open := flags.Bool("open", false, "open the one-use local URL with xdg-open")
	if err := flags.Parse(arguments); err != nil {
		return reviewWebOptions{}, err
	}
	if len(flags.Args()) != 1 {
		return reviewWebOptions{}, errors.New("usage: gator review RUN_RECORD_PATH [--listen 127.0.0.1:PORT] [--open]")
	}
	if err := validateLoopbackAddress(*listen); err != nil {
		return reviewWebOptions{}, err
	}
	return reviewWebOptions{statePath: flags.Arg(0), listen: *listen, open: *open}, nil
}
