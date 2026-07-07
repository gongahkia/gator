package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadInstructionPaths(t *testing.T) {
	resetCLIState(t)
	defer resetCLIState(t)

	runInstruction = "inline"
	got, err := readInstruction()
	if err != nil || got != "inline" {
		t.Fatalf("inline instruction = %q, %v", got, err)
	}

	resetCLIState(t)
	path := filepath.Join(t.TempDir(), "instruction.txt")
	if err := os.WriteFile(path, []byte("from file"), 0o644); err != nil {
		t.Fatalf("write instruction: %v", err)
	}
	runInstructionFile = path
	got, err = readInstruction()
	if err != nil || got != "from file" {
		t.Fatalf("file instruction = %q, %v", got, err)
	}

	resetCLIState(t)
	runInstruction = "inline"
	runInstructionFile = path
	if _, err := readInstruction(); err == nil || !strings.Contains(err.Error(), "use --instruction or --instruction-file") {
		t.Fatalf("both flags err = %v", err)
	}

	resetCLIState(t)
	if _, err := readInstruction(); err == nil || !strings.Contains(err.Error(), "missing --instruction or --instruction-file") {
		t.Fatalf("missing flags err = %v", err)
	}

	resetCLIState(t)
	runInstructionFile = filepath.Join(t.TempDir(), "missing.txt")
	if _, err := readInstruction(); err == nil {
		t.Fatal("missing file err = nil")
	}
}

func TestCommandFlagErrors(t *testing.T) {
	isolateEnv(t)

	t.Run("gather missing instruction", func(t *testing.T) {
		_, stderr, err := executeRootErr(t, append(configArgs(t), "gather"), "")
		if err == nil || !strings.Contains(err.Error(), "missing --instruction") {
			t.Fatalf("err = %v stderr=%s", err, stderr)
		}
	})

	t.Run("run missing instruction", func(t *testing.T) {
		_, stderr, err := executeRootErr(t, append(configArgs(t), "run", "--raw-context"), "")
		if err == nil || !strings.Contains(err.Error(), "missing --instruction or --instruction-file") {
			t.Fatalf("err = %v stderr=%s", err, stderr)
		}
	})

	t.Run("run exclusive instruction flags", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "instruction.txt")
		if err := os.WriteFile(path, []byte("from file"), 0o644); err != nil {
			t.Fatalf("write instruction: %v", err)
		}
		args := append(configArgs(t), "run", "--raw-context", "--instruction", "inline", "--instruction-file", path)
		_, stderr, err := executeRootErr(t, args, "")
		if err == nil || !strings.Contains(err.Error(), "use --instruction or --instruction-file") {
			t.Fatalf("err = %v stderr=%s", err, stderr)
		}
	})

	t.Run("invalid config path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.toml")
		_, stderr, err := executeRootErr(t, []string{"--config", path, "gather", "--instruction", "target"}, "")
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("err = %v stderr=%s", err, stderr)
		}
	})
}

func TestVersionOutput(t *testing.T) {
	out := executeRoot(t, []string{"version"}, "")
	for _, want := range []string{"version: ", "git_commit: ", "go: "} {
		if !strings.Contains(out, want) {
			t.Fatalf("version output missing %q:\n%s", want, out)
		}
	}
}
