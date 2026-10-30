package cliagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/sandbox"
)

func fixtureRequest(t *testing.T, body string) Request {
	t.Helper()
	if _, err := sandbox.StrictAvailability(); err != nil {
		t.Skip(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	scratch := filepath.Join(root, "scratch")
	for _, dir := range []string{source, scratch} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "fake-cli")
	script := "#!/bin/sh\nset -eu\ncat >/dev/null\n" + body + "\nprintf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"done\"}}' '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}'\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIXTURE_API_KEY", "synthetic-private-key-value")
	return Request{Config: Config{Kind: "codex", Executable: executable, APIKeyEnv: "FIXTURE_API_KEY", TimeoutSeconds: 10, AllowEdits: true}, Source: source, Scratch: scratch, ID: "task-1", Task: "inspect fixture"}
}
func TestRunnerIsolatesEditsAndTrustedGit(t *testing.T) {
	r := fixtureRequest(t, `printf 'updated\n' > note.txt
mkdir .git
printf 'malicious metadata\n' > .git/config`)
	r.Edit = true
	value, err := Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(value.Patch), "+updated") || value.Summary != "done" {
		t.Fatalf("unexpected result: %+v", value)
	}
	original, err := os.ReadFile(filepath.Join(r.Source, "note.txt"))
	if err != nil || string(original) != "original\n" {
		t.Fatalf("original modified: %q %v", original, err)
	}
	target := filepath.Join(r.Scratch, "apply-check")
	if err := patch.InitSnapshot(context.Background(), r.Source, target); err != nil {
		t.Fatal(err)
	}
	if err := patch.ApplySnapshot(context.Background(), target, value.Patch); err != nil {
		t.Fatalf("patch cannot be applied: %v", err)
	}
}
func TestRunnerEnforcesReadOnlyAndPathEnvelope(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(map[bool]string{false: "inspect", true: "scoped"}[edit], func(t *testing.T) {
			r := fixtureRequest(t, `if printf 'forbidden\n' > note.txt 2>/dev/null; then exit 9; fi`)
			r.Edit = edit
			if edit {
				r.Scopes = []string{"allowed"}
			}
			if _, err := Run(context.Background(), r); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestRunnerCanInspectSelectedBaseline(t *testing.T) {
	r := fixtureRequest(t, `test "$(cat note.txt)" = updated`)
	r.Baseline = []byte("diff --git a/note.txt b/note.txt\n--- a/note.txt\n+++ b/note.txt\n@@ -1 +1 @@\n-original\n+updated\n")
	value, err := Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Patch) != 0 {
		t.Fatal("read-only baseline returned as new edits")
	}
}
func TestRunnerCancellationStopsProcess(t *testing.T) {
	r := fixtureRequest(t, `sleep 30 &
wait`)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	_, err := Run(ctx, r)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected cancellation: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("cancellation waited for descendants")
	}
}
func TestRunnerRejectsFailedAndSecretResults(t *testing.T) {
	for _, body := range []string{`printf '%s\n' '{"type":"turn.failed"}'`, `printf '%s' "$CODEX_API_KEY" > secret.txt`} {
		r := fixtureRequest(t, body)
		r.Edit = true
		value, err := Run(context.Background(), r)
		if err == nil {
			t.Fatal("accepted failed or secret-bearing output")
		}
		if len(value.Patch) > 0 {
			t.Fatal("retained rejected patch")
		}
	}
}
