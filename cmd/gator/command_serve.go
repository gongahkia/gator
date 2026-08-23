package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/appserver"
	"github.com/gongahkia/gator/internal/journal"
	internalrpc "github.com/gongahkia/gator/internal/rpc"
)

func serveCommand(arguments []string, out io.Writer) error {
	if len(arguments) > 0 && arguments[0] == "token" {
		return createServeToken(arguments[1:], out)
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", "127.0.0.1:0", "loopback TCP address to listen on")
	tokenFile := flags.String("token-file", "", "absolute path to a private bearer-token file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("usage: gator serve --token-file ABSOLUTE_PATH [--listen 127.0.0.1:PORT]")
	}
	if err := validateLoopbackAddress(*listen); err != nil {
		return err
	}
	token, err := readServeToken(*tokenFile)
	if err != nil {
		return err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	repository, err := gitRepositoryRoot(workingDirectory)
	if err != nil {
		return errors.New("app-server mode must start inside a Git checkout")
	}
	defaults, err := configuredDefaults()
	if err != nil {
		return err
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	bridge, err := appserver.New(appserver.Config{
		RPC: internalrpc.Config{
			RepositoryPath: repository, StateDir: stateDir, DefaultProvider: defaults.Provider, DefaultModel: defaults.Model,
			ResolveProvider: resolveConfiguredProvider, NewExecutor: newExecutor,
		},
		Token: token, Version: version,
	})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *listen, err)
	}
	defer listener.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := bridge.Start(ctx); err != nil {
		return err
	}
	defer bridge.Close()
	server := &http.Server{
		Handler:           bridge.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	address := listener.Addr().String()
	if _, err := fmt.Fprintf(out, "Gator local app server\n  URL: http://%s\n  RPC: POST /v1/rpc\n  Events: GET /v1/events/{request-id}\n  Spec: GET /openapi.json\n", address); err != nil {
		return err
	}
	err = server.Serve(listener)
	_ = bridge.Close()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	if err != nil {
		return err
	}
	if waitErr := bridge.Wait(); waitErr != nil {
		return fmt.Errorf("app server stopped: %w", waitErr)
	}
	return nil
}

func validateLoopbackAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(port) == "" {
		return errors.New("--listen must be an IP address and TCP port, such as 127.0.0.1:0")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("gator serve accepts loopback addresses only; use SSH port forwarding for remote access")
	}
	return nil
}

func readServeToken(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("gator serve requires --token-file; create one with 'gator serve token ABSOLUTE_PATH'")
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("--token-file must be an absolute path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("stat app-server token file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0o077 != 0 || info.Size() > 4097 {
		return nil, errors.New("app-server token file must be a private regular file with mode 0600 and at most 4097 bytes")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read app-server token file: %w", err)
	}
	return contents, nil
}

func createServeToken(arguments []string, out io.Writer) error {
	if len(arguments) != 1 || !filepath.IsAbs(arguments[0]) {
		return errors.New("usage: gator serve token ABSOLUTE_PATH")
	}
	path := arguments[0]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create app-server token file: %w", err)
	}
	completed := false
	defer func() {
		if !completed {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("generate app-server token: %w", err)
	}
	if _, err := io.WriteString(file, base64.RawURLEncoding.EncodeToString(random[:])+"\n"); err != nil {
		return fmt.Errorf("write app-server token: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close app-server token file: %w", err)
	}
	completed = true
	_, err = fmt.Fprintf(out, "Created private app-server token file: %s\n", path)
	return err
}
