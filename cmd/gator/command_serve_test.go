package main

import (
	"bytes"
	"os"
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
