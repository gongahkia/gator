package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/envelope"
)

func TestVerifyPassingCommand(t *testing.T) {
	stage := New("printf '%s\n' ok")
	env := &envelope.Envelope{Cwd: t.TempDir()}

	got, err := stage.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.Verify == nil {
		t.Fatal("verify result missing")
	}
	if !got.Verify.Passed {
		t.Fatalf("expected pass, got %#v", got.Verify)
	}
	if got.Verify.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", got.Verify.ExitCode)
	}
	if got.Verify.Command != stage.Command {
		t.Fatalf("command mismatch: %q", got.Verify.Command)
	}
	if got.Verify.StdoutBytes != 3 || got.Verify.StderrBytes != 0 || got.Verify.TimedOut {
		t.Fatalf("verify metadata = %#v", got.Verify)
	}
}

func TestVerifyFailingCommandDigestKeepsMarkersAndCaps(t *testing.T) {
	stage := &Verify{
		Command: strings.Join([]string{
			"printf '%s\\n'",
			"'prefix one'",
			"'FAIL short'",
			"'expected got'",
			"'tail 012345678901234567890123456789'",
			"'tail 123456789012345678901234567890'",
			"; exit 7",
		}, " "),
		MaxDigestBytes: 64,
	}
	env := &envelope.Envelope{Cwd: t.TempDir()}

	got, err := stage.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.Verify == nil {
		t.Fatal("verify result missing")
	}
	if got.Verify.Passed {
		t.Fatalf("expected failure, got %#v", got.Verify)
	}
	if got.Verify.ExitCode != 7 {
		t.Fatalf("expected exit 7, got %d", got.Verify.ExitCode)
	}
	if !strings.Contains(got.Verify.FailureDigest, "FAIL short") {
		t.Fatalf("digest missing FAIL marker: %q", got.Verify.FailureDigest)
	}
	if !strings.Contains(got.Verify.FailureDigest, "expected got") {
		t.Fatalf("digest missing expected/got marker: %q", got.Verify.FailureDigest)
	}
	if len(got.Verify.FailureDigest) > stage.MaxDigestBytes {
		t.Fatalf("digest exceeded cap: %d > %d", len(got.Verify.FailureDigest), stage.MaxDigestBytes)
	}
	if got.Verify.RawTailBytes <= len(got.Verify.FailureDigest) {
		t.Fatalf("raw bytes not recorded before truncation: raw=%d digest=%d", got.Verify.RawTailBytes, len(got.Verify.FailureDigest))
	}
}

func TestVerifyRecordsStderrBytes(t *testing.T) {
	stage := New("printf out; printf err >&2; exit 3")
	env := &envelope.Envelope{Cwd: t.TempDir()}
	got, err := stage.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.Verify.Passed || got.Verify.ExitCode != 3 {
		t.Fatalf("verify = %#v", got.Verify)
	}
	if got.Verify.StdoutBytes != 3 || got.Verify.StderrBytes != 3 || !strings.Contains(got.Verify.FailureDigest, "out") || !strings.Contains(got.Verify.FailureDigest, "err") {
		t.Fatalf("verify metadata = %#v", got.Verify)
	}
}

func TestVerifyTimeoutRecordsFailure(t *testing.T) {
	stage := New("sleep 2")
	stage.Timeout = 50 * time.Millisecond
	env := &envelope.Envelope{Cwd: t.TempDir()}
	got, err := stage.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.Verify.Passed || !got.Verify.TimedOut || got.Verify.ExitCode != 124 {
		t.Fatalf("timeout verify = %#v", got.Verify)
	}
	if !strings.Contains(got.Verify.FailureDigest, "timed out") {
		t.Fatalf("timeout digest = %q", got.Verify.FailureDigest)
	}
}

func TestResolveCommandRepoAware(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{name: "makefile", files: map[string]string{"Makefile": "test:\n\t@echo ok\n"}, want: "make test"},
		{name: "node", files: map[string]string{"package.json": `{"scripts":{"test":"vitest run"}}`}, want: "npm test"},
		{name: "go", files: map[string]string{"go.mod": "module example.com/x\n"}, want: "go test ./..."},
		{name: "rust", files: map[string]string{"Cargo.toml": "[package]\nname='x'\nversion='0.1.0'\n"}, want: "cargo test"},
		{name: "python", files: map[string]string{"pyproject.toml": "[project]\nname='x'\n"}, want: "python -m pytest"},
		{name: "unknown", files: map[string]string{"README.md": "x\n"}, want: "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, text := range tt.files {
				writeVerifyFile(t, dir, name, text)
			}
			if got := ResolveCommand(dir); got != tt.want {
				t.Fatalf("command = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifyEnvCommandStillWins(t *testing.T) {
	t.Setenv("PAW_VERIFY_CMD", "printf env")
	stage := New("printf explicit")
	if got := stage.command(t.TempDir()); got != "printf env" {
		t.Fatalf("command = %q", got)
	}
}

func writeVerifyFile(t *testing.T, dir, name, text string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
