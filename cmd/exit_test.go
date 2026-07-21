package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gongahkia/paw/internal/llm/faketest"
)

func TestExecuteExitCodes(t *testing.T) {
	exe := buildPawBinary(t)
	tests := []struct {
		name  string
		setup func(*testing.T) (string, []string, []string)
		want  int
	}{
		{
			name: "usage",
			setup: func(t *testing.T) (string, []string, []string) {
				return t.TempDir(), []string{"run", "--instruction", "", "--instruction-file", "", "--quiet"}, nil
			},
			want: int(ExitUsageError),
		},
		{
			name: "system",
			setup: func(t *testing.T) (string, []string, []string) {
				server := faketest.NewServer()
				t.Cleanup(server.Close)
				server.Respond("", 500, `{"error":"unreachable"}`)
				dir := t.TempDir()
				writeTestFile(t, dir, "notes.txt", "target\n")
				config := writeExitConfig(t, dir, server.URL)
				return dir, []string{"--config", config, "run", "--raw-context", "--instruction", "target", "--quiet"}, nil
			},
			want: int(ExitSystemError),
		},
		{
			name: "review session skips verification",
			setup: func(t *testing.T) (string, []string, []string) {
				server := faketest.NewServer()
				t.Cleanup(server.Close)
				server.RespondOpenAI("Prior VerifyResult JSON", `{"done":false,"reasoning":"edit","next_action":{"kind":"edit_file","description":"update file","target_path":"file.txt"}}`)
				server.RespondOpenAI("Target path", "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n")
				dir := t.TempDir()
				writeTestFile(t, dir, "file.txt", "old\n")
				config := writeExitConfig(t, dir, server.URL)
				args := []string{"--config", config, "run", "--raw-context", "--instruction", "target", "--max-turns", "1", "--quiet"}
				return dir, args, []string{"PAW_VERIFY_CMD=exit 7"}
			},
			want: int(ExitSuccess),
		},
		{
			name: "success",
			setup: func(t *testing.T) (string, []string, []string) {
				server := faketest.NewServer()
				t.Cleanup(server.Close)
				server.RespondOpenAI("Prior VerifyResult JSON", `{"done":true,"reasoning":"done"}`)
				dir := t.TempDir()
				writeTestFile(t, dir, "notes.txt", "target\n")
				config := writeExitConfig(t, dir, server.URL)
				return dir, []string{"--config", config, "run", "--raw-context", "--instruction", "target", "--quiet"}, nil
			},
			want: int(ExitSuccess),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, args, env := tt.setup(t)
			code, stdout, stderr := runPawBinary(t, exe, dir, args, env)
			if code != tt.want {
				t.Fatalf("exit code = %d; want %d\nstdout:\n%s\nstderr:\n%s", code, tt.want, stdout, stderr)
			}
		})
	}
}

func buildPawBinary(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "paw")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, "..")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build paw: %v\n%s", err, stderr.String())
	}
	return exe
}

func runPawBinary(t *testing.T, exe, dir string, args []string, extraEnv []string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), stdout.String(), stderr.String()
	}
	t.Fatalf("run paw: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	return -1, stdout.String(), stderr.String()
}

func writeExitConfig(t *testing.T, dir, baseURL string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
[brain]
transport = "openai"
base_url = "`+baseURL+`"
api_key = "test-key"
model = "test-brain"

[policy.provider]
allowed_transports = ["ollama", "openai"]
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
