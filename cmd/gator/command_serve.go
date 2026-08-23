package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/appserver"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/lsp"
	internalrpc "github.com/gongahkia/gator/internal/rpc"
	"github.com/gongahkia/gator/internal/terminal"
)

func serveCommand(arguments []string, out io.Writer) error {
	if len(arguments) > 0 {
		switch arguments[0] {
		case "token":
			return createServeToken(arguments[1:], out)
		case "start":
			return startServeService(arguments[1:], out)
		case "status":
			return serveServiceStatus(arguments[1:], out)
		case "stop":
			return stopServeService(arguments[1:], out)
		}
	}
	return serveForeground(arguments, out)
}

func serveForeground(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", "127.0.0.1:0", "loopback TCP address to listen on")
	tokenFile := flags.String("token-file", "", "absolute path to a private bearer-token file")
	readyFile := flags.String("ready-file", "", "internal service readiness path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("usage: gator serve --token-file ABSOLUTE_PATH [--listen 127.0.0.1:PORT]")
	}
	if err := validateLoopbackAddress(*listen); err != nil {
		return err
	}
	if *readyFile != "" && !filepath.IsAbs(*readyFile) {
		return errors.New("--ready-file must be an absolute path")
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
	terminals := terminal.NewRegistry()
	defer terminals.Close()
	lsps := lsp.NewRegistry()
	defer lsps.Close()
	bridge, err := appserver.New(appserver.Config{
		RPC: internalrpc.Config{
			RepositoryPath: repository, StateDir: stateDir, DefaultProvider: defaults.Provider, DefaultModel: defaults.Model,
			ResolveProvider: resolveConfiguredProvider, NewExecutor: newExecutor, TerminalRegistry: terminals, LSPRegistry: lsps,
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
	ctx, stop := signal.NotifyContext(context.Background(), serveSignals()...)
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
	if *readyFile != "" {
		if err := writeServeReady(*readyFile, serveReady{PID: os.Getpid(), URL: "http://" + address}); err != nil {
			return err
		}
	}
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
	contents = bytes.TrimSpace(contents)
	if len(contents) == 0 {
		return nil, errors.New("app-server token file must contain a token")
	}
	return append([]byte(nil), contents...), nil
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

const (
	serveServiceVersion  = 1
	serveReadyTimeout    = 5 * time.Second
	serveShutdownTimeout = 10 * time.Second
	maxServeStateBytes   = 16 * 1024
)

type serveReady struct {
	PID int    `json:"pid"`
	URL string `json:"url"`
}

type serveService struct {
	Version    int       `json:"version"`
	Repository string    `json:"repository"`
	PID        int       `json:"pid"`
	URL        string    `json:"url"`
	StartedAt  time.Time `json:"started_at"`
}

type serveServiceOptions struct {
	listen    string
	tokenFile string
}

func startServeService(arguments []string, out io.Writer) error {
	options, err := parseServeServiceOptions(arguments, true)
	if err != nil {
		return err
	}
	token, err := readServeToken(options.tokenFile)
	if err != nil {
		return err
	}
	repository, stateDir, err := serveServiceRepository()
	if err != nil {
		return err
	}
	directory, statePath, err := serveServicePaths(stateDir, repository)
	if err != nil {
		return err
	}
	if existing, found, err := loadServeService(statePath, repository); err != nil {
		return err
	} else if found {
		if healthy, _ := serveServiceHealthy(context.Background(), existing.URL, token); healthy {
			return fmt.Errorf("Gator app server is already running at %s (pid %d)", existing.URL, existing.PID)
		}
		if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale app-server state: %w", err)
		}
	}
	if err := ensureServeStateDirectory(directory); err != nil {
		return err
	}
	ready, err := os.CreateTemp(directory, ".ready-")
	if err != nil {
		return fmt.Errorf("create app-server readiness path: %w", err)
	}
	readyPath := ready.Name()
	if err := ready.Close(); err != nil {
		return fmt.Errorf("close app-server readiness path: %w", err)
	}
	if err := os.Remove(readyPath); err != nil {
		return fmt.Errorf("prepare app-server readiness path: %w", err)
	}
	defer os.Remove(readyPath)
	log, err := openServeLog(statePath + ".log")
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		_ = log.Close()
		return fmt.Errorf("locate Gator executable for app-server service: %w", err)
	}
	command := exec.Command(executable, "serve", "--token-file", options.tokenFile, "--listen", options.listen, "--ready-file", readyPath)
	command.Dir = repository
	command.Stdout, command.Stderr = log, log
	detachServeProcess(command)
	if err := command.Start(); err != nil {
		_ = log.Close()
		return fmt.Errorf("start app-server service: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	if err := log.Close(); err != nil {
		abortServeProcess(command.Process, done)
		return fmt.Errorf("close app-server log parent handle: %w", err)
	}
	readyState, err := waitServeReady(readyPath, command.Process.Pid, token, done)
	if err != nil {
		abortServeProcess(command.Process, done)
		return fmt.Errorf("wait for app-server service readiness (see %s): %w", statePath+".log", err)
	}
	service := serveService{Version: serveServiceVersion, Repository: repository, PID: readyState.PID, URL: readyState.URL, StartedAt: time.Now().UTC()}
	if err := writeServeService(statePath, service); err != nil {
		abortServeProcess(command.Process, done)
		return err
	}
	_, err = fmt.Fprintf(out, "Gator app server started\n  URL: %s\n  PID: %d\n  Log: %s\n  Status: gator serve status --token-file %s\n", service.URL, service.PID, statePath+".log", options.tokenFile)
	return err
}

func serveServiceStatus(arguments []string, out io.Writer) error {
	options, err := parseServeServiceOptions(arguments, false)
	if err != nil {
		return err
	}
	token, err := readServeToken(options.tokenFile)
	if err != nil {
		return err
	}
	repository, stateDir, err := serveServiceRepository()
	if err != nil {
		return err
	}
	_, statePath, err := serveServicePaths(stateDir, repository)
	if err != nil {
		return err
	}
	service, found, err := loadServeService(statePath, repository)
	if err != nil {
		return err
	}
	if !found {
		_, err := fmt.Fprintln(out, "Gator app server is not running for this repository.")
		return err
	}
	healthy, _ := serveServiceHealthy(context.Background(), service.URL, token)
	if !healthy {
		if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale app-server state: %w", err)
		}
		_, err := fmt.Fprintln(out, "Gator app server is not running for this repository.")
		return err
	}
	_, err = fmt.Fprintf(out, "Gator app server is running\n  URL: %s\n  PID: %d\n  Started: %s\n  Log: %s\n", service.URL, service.PID, service.StartedAt.Format(time.RFC3339), statePath+".log")
	return err
}

func stopServeService(arguments []string, out io.Writer) error {
	options, err := parseServeServiceOptions(arguments, false)
	if err != nil {
		return err
	}
	token, err := readServeToken(options.tokenFile)
	if err != nil {
		return err
	}
	repository, stateDir, err := serveServiceRepository()
	if err != nil {
		return err
	}
	_, statePath, err := serveServicePaths(stateDir, repository)
	if err != nil {
		return err
	}
	service, found, err := loadServeService(statePath, repository)
	if err != nil {
		return err
	}
	if !found {
		_, err := fmt.Fprintln(out, "Gator app server is not running for this repository.")
		return err
	}
	if healthy, _ := serveServiceHealthy(context.Background(), service.URL, token); !healthy {
		if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale app-server state: %w", err)
		}
		_, err := fmt.Fprintln(out, "Gator app server is not running for this repository.")
		return err
	}
	process, err := os.FindProcess(service.PID)
	if err != nil {
		return fmt.Errorf("find app-server process: %w", err)
	}
	if err := terminateServeProcess(process); err != nil {
		return fmt.Errorf("request app-server shutdown: %w", err)
	}
	deadline := time.Now().Add(serveShutdownTimeout)
	for time.Now().Before(deadline) {
		if healthy, _ := serveServiceHealthy(context.Background(), service.URL, token); !healthy {
			if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove stopped app-server state: %w", err)
			}
			_, err := fmt.Fprintln(out, "Gator app server stopped.")
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("app-server process did not stop within 10 seconds; inspect %s before retrying", statePath+".log")
}

func parseServeServiceOptions(arguments []string, includeListen bool) (serveServiceOptions, error) {
	flags := flag.NewFlagSet("serve service", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	options := serveServiceOptions{}
	if includeListen {
		flags.StringVar(&options.listen, "listen", "127.0.0.1:0", "loopback TCP address to listen on")
	}
	flags.StringVar(&options.tokenFile, "token-file", "", "absolute path to a private bearer-token file")
	if err := flags.Parse(arguments); err != nil {
		return serveServiceOptions{}, err
	}
	if len(flags.Args()) != 0 {
		return serveServiceOptions{}, errors.New("usage: gator serve start|status|stop --token-file ABSOLUTE_PATH")
	}
	if includeListen {
		if err := validateLoopbackAddress(options.listen); err != nil {
			return serveServiceOptions{}, err
		}
	}
	return options, nil
}

func serveServiceRepository() (string, string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("get working directory: %w", err)
	}
	repository, err := gitRepositoryRoot(workingDirectory)
	if err != nil {
		return "", "", errors.New("app-server service commands must run inside a Git checkout")
	}
	repository, err = canonicalServeRepository(repository)
	if err != nil {
		return "", "", err
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return "", "", err
	}
	return repository, stateDir, nil
}

func canonicalServeRepository(repository string) (string, error) {
	abs, err := filepath.Abs(repository)
	if err != nil {
		return "", fmt.Errorf("resolve app-server repository: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("canonicalize app-server repository: %w", err)
	}
	return filepath.Clean(canonical), nil
}

func serveServicePaths(stateDir, repository string) (string, string, error) {
	if !filepath.IsAbs(stateDir) {
		return "", "", errors.New("app-server state directory must be absolute")
	}
	repository, err := canonicalServeRepository(repository)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(repository))
	directory := filepath.Join(stateDir, "gator", "app-server")
	return directory, filepath.Join(directory, hex.EncodeToString(digest[:16])+".json"), nil
}

func writeServeReady(path string, ready serveReady) error {
	if !filepath.IsAbs(path) {
		return errors.New("app-server readiness path must be absolute")
	}
	if err := validateServeEndpoint(ready.URL); err != nil || ready.PID < 2 {
		return errors.New("app-server readiness payload is invalid")
	}
	payload, err := json.Marshal(ready)
	if err != nil {
		return fmt.Errorf("encode app-server readiness: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create app-server readiness file: %w", err)
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("write app-server readiness file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close app-server readiness file: %w", err)
	}
	return nil
}

func waitServeReady(path string, pid int, token []byte, done <-chan error) (serveReady, error) {
	deadline := time.NewTimer(serveReadyTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ready, found, err := loadServeReady(path); err == nil && found && ready.PID == pid {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			healthy, _ := serveServiceHealthy(ctx, ready.URL, token)
			cancel()
			if healthy {
				return ready, nil
			}
		}
		select {
		case err := <-done:
			if err == nil {
				return serveReady{}, errors.New("app-server service exited before becoming ready")
			}
			return serveReady{}, fmt.Errorf("app-server service exited before becoming ready: %w", err)
		case <-deadline.C:
			return serveReady{}, errors.New("app-server service did not become ready within 5 seconds")
		case <-ticker.C:
		}
	}
}

func abortServeProcess(process *os.Process, done <-chan error) {
	if process != nil {
		_ = terminateServeProcess(process)
	}
	if done == nil {
		return
	}
	select {
	case <-done:
	default:
	}
}

func loadServeReady(path string) (serveReady, bool, error) {
	var ready serveReady
	if err := readServeState(path, &ready); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return serveReady{}, false, nil
		}
		return serveReady{}, false, err
	}
	if ready.PID < 2 || validateServeEndpoint(ready.URL) != nil {
		return serveReady{}, false, errors.New("app-server readiness file is invalid")
	}
	return ready, true, nil
}

func writeServeService(path string, service serveService) error {
	if err := validateServeService(service, service.Repository); err != nil {
		return err
	}
	return writeServeState(path, service)
}

func loadServeService(path, repository string) (serveService, bool, error) {
	var service serveService
	if err := readServeState(path, &service); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return serveService{}, false, nil
		}
		return serveService{}, false, err
	}
	if err := validateServeService(service, repository); err != nil {
		return serveService{}, false, err
	}
	return service, true, nil
}

func validateServeService(service serveService, repository string) error {
	if service.Version != serveServiceVersion || service.PID < 2 || service.StartedAt.IsZero() {
		return errors.New("app-server service state is invalid")
	}
	canonical, err := canonicalServeRepository(repository)
	if err != nil || service.Repository != canonical {
		return errors.New("app-server service state belongs to another repository")
	}
	if err := validateServeEndpoint(service.URL); err != nil {
		return fmt.Errorf("app-server service endpoint is invalid: %w", err)
	}
	return nil
}

func validateServeEndpoint(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("endpoint must be a bare loopback HTTP URL")
	}
	return validateLoopbackAddress(parsed.Host)
}

func readServeState(path string, destination any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&0o077 != 0 || info.Size() > maxServeStateBytes {
		return errors.New("app-server state file must be a private regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode app-server state: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("app-server state contains multiple JSON values")
		}
		return fmt.Errorf("decode app-server state: %w", err)
	}
	return nil
}

func writeServeState(path string, value any) error {
	if err := ensureServeStateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode app-server state: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".service-")
	if err != nil {
		return fmt.Errorf("create app-server state file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set app-server state permissions: %w", err)
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write app-server state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close app-server state: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish app-server state: %w", err)
	}
	return nil
}

func ensureServeStateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create app-server state directory: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect app-server state directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("app-server state directory must be private")
	}
	return nil
}

func openServeLog(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&0o077 != 0 {
			return nil, errors.New("app-server log must be a private regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect app-server log: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open app-server log: %w", err)
	}
	return file, nil
}

func serveServiceHealthy(ctx context.Context, endpoint string, token []byte) (bool, error) {
	if err := validateServeEndpoint(endpoint); err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/openapi.json", nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+string(token))
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	response, err := client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("app-server endpoint returned HTTP %d", response.StatusCode)
	}
	return true, nil
}
