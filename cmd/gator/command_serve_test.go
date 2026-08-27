package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCreateServeTokenCreatesPrivateReadableToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens", "gator")
	var output bytes.Buffer
	if err := createServeToken([]string{path}, &output); err != nil {
		t.Fatalf("create token: %v", err)
	}
	if !strings.Contains(output.String(), path) {
		t.Fatalf("create output = %q", output.String())
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("token file info = %#v, %v", info, err)
	}
	token, err := readServeToken(path)
	if err != nil || len(token) < 32 || bytes.Contains(token, []byte("\n")) {
		t.Fatalf("read token bytes=%d err=%v", len(token), err)
	}
	if err := createServeToken([]string{path}, &output); err == nil {
		t.Fatal("existing token file was overwritten")
	}
}

func TestReadServeTokenRejectsSymlinkSwapTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permission behavior varies on Windows")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "target-token")
	if err := os.WriteFile(target, []byte("gator-app-server-test-token-0123456789\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "token-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readServeToken(link); err == nil || !strings.Contains(err.Error(), "private regular file") {
		t.Fatalf("symlink token read error = %v", err)
	}
}

func TestValidateLoopbackAddressRejectsRemoteListeners(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "[::1]:49152"} {
		if err := validateLoopbackAddress(address); err != nil {
			t.Fatalf("validate %q: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:49152", "localhost:49152", "example.com:49152", "127.0.0.1"} {
		if err := validateLoopbackAddress(address); err == nil {
			t.Fatalf("accepted non-loopback listener %q", address)
		}
	}
}

func TestServeSignalsIncludeGracefulTermination(t *testing.T) {
	signals := serveSignals()
	if len(signals) == 0 || signals[0] != os.Interrupt {
		t.Fatalf("serve signals = %#v", signals)
	}
	if runtime.GOOS != "windows" && len(signals) < 2 {
		t.Fatalf("serve signals omit termination signal: %#v", signals)
	}
}

func TestServeServiceStateIsPrivateAndRepositoryScoped(t *testing.T) {
	repository := t.TempDir()
	stateDirectory := t.TempDir()
	_, path, err := serveServicePaths(stateDirectory, repository)
	if err != nil {
		t.Fatalf("service paths: %v", err)
	}
	canonical, err := canonicalServeRepository(repository)
	if err != nil {
		t.Fatalf("canonical repository: %v", err)
	}
	want := serveService{Version: serveServiceVersion, Repository: canonical, PID: 42, URL: "http://127.0.0.1:49152", StartedAt: time.Date(2026, time.August, 23, 0, 0, 0, 0, time.UTC)}
	if err := writeServeService(path, want); err != nil {
		t.Fatalf("write service state: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("service state info = %#v, %v", info, err)
	}
	got, found, err := loadServeService(path, repository)
	if err != nil || !found || !reflect.DeepEqual(got, want) {
		t.Fatalf("load service state = %#v, found=%t, err=%v", got, found, err)
	}
	otherRepository := t.TempDir()
	if _, _, err := loadServeService(path, otherRepository); err == nil || !strings.Contains(err.Error(), "another repository") {
		t.Fatalf("cross-repository service state error = %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadServeService(path, repository); err == nil || !strings.Contains(err.Error(), "private regular file") {
		t.Fatalf("world-readable service state error = %v", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeServeState(path, want); err == nil || !strings.Contains(err.Error(), "state directory must be private") {
		t.Fatalf("world-readable service directory error = %v", err)
	}
}

func TestServeReadyIsExclusiveAndLoopbackOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready.json")
	ready := serveReady{PID: 42, URL: "http://127.0.0.1:49152"}
	if err := writeServeReady(path, ready); err != nil {
		t.Fatalf("write readiness: %v", err)
	}
	got, found, err := loadServeReady(path)
	if err != nil || !found || got != ready {
		t.Fatalf("load readiness = %#v, found=%t, err=%v", got, found, err)
	}
	if err := writeServeReady(path, ready); err == nil {
		t.Fatal("readiness file was overwritten")
	}
	for _, endpoint := range []string{"https://127.0.0.1:49152", "http://example.com:49152", "http://127.0.0.1:49152/path"} {
		if err := validateServeEndpoint(endpoint); err == nil {
			t.Fatalf("accepted non-local service endpoint %q", endpoint)
		}
	}
}

func TestRequestServeShutdownUsesOnlyTheAuthenticatedLoopbackEndpoint(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/shutdown" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		called = true
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	if err := requestServeShutdown(context.Background(), server.URL, []byte("test-token")); err != nil {
		t.Fatalf("request shutdown: %v", err)
	}
	if !called {
		t.Fatal("shutdown endpoint was not called")
	}
	if err := requestServeShutdown(context.Background(), "http://example.com:49152", []byte("test-token")); err == nil {
		t.Fatal("non-loopback shutdown endpoint was accepted")
	}
}

func TestAbortServeProcessTerminatesAReadinessFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Unix service supervisor uses SIGTERM")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	command := exec.Command(sh, "-c", "sleep 30")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	abortServeProcess(command.Process, done)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("readiness-failed service remained running")
	}
}
