package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if err != nil || len(bytes.TrimSpace(token)) < 32 {
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
