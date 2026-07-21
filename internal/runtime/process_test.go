package runtime

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"testing"
)

func TestNativeRunnerCapturesExitOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX")
	}
	result, err := (NativeRunner{}).Run(context.Background(), Command{Path: "sh", Args: []string{"-c", "printf out; printf err >&2; exit 7"}})
	if err == nil {
		t.Fatal("expected exit error")
	}
	if result.ExitCode != 7 || string(result.Stdout) != "out" || string(result.Stderr) != "err" {
		t.Fatalf("result = %#v", result)
	}
}

func TestNativeRunnerRejectsMissingPath(t *testing.T) {
	_, err := (NativeRunner{}).Run(context.Background(), Command{})
	if err == nil || err.Error() != "command path is required" {
		t.Fatalf("run error = %v", err)
	}
}

func TestNativeRunnerPreservesNotFoundError(t *testing.T) {
	_, err := (NativeRunner{}).Run(context.Background(), Command{Path: "paw-command-that-does-not-exist"})
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("run error = %v", err)
	}
}

func TestDetectWSL(t *testing.T) {
	readFile := func(path string) ([]byte, error) {
		if path == "/proc/sys/kernel/osrelease" {
			return []byte("6.6.0-microsoft-standard-WSL2"), nil
		}
		return nil, errors.New("unexpected path")
	}
	if !isWSL("linux", readFile) {
		t.Fatal("expected WSL detection")
	}
	if isWSL("darwin", readFile) {
		t.Fatal("non-Linux runtime reported WSL")
	}
}
