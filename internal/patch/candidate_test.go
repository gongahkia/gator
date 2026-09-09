package patch

import (
	"context"
	"github.com/gongahkia/gator/internal/sandbox"
	"os"
	"path/filepath"
	"testing"
)

func TestCandidateCompositionVerificationAndChangedTarget(t *testing.T) {
	source := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte("base\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	fragment := func(name, value string) []byte {
		return []byte("diff --git a/" + name + " b/" + name + "\n--- a/" + name + "\n+++ b/" + name + "\n@@ -1 +1 @@\n-base\n+" + value + "\n")
	}
	c, err := Integrate(context.Background(), source, filepath.Join(t.TempDir(), "candidate"), [][]byte{fragment("a.txt", "alpha"), fragment("b.txt", "beta")}, []string{"a", "b"}, nil, sandbox.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "verified" || len(c.ChangedPaths) != 2 {
		t.Fatalf("%+v", c)
	}
	conflict, err := Integrate(context.Background(), source, filepath.Join(t.TempDir(), "conflict"), [][]byte{fragment("a.txt", "alpha"), fragment("a.txt", "beta")}, []string{"a", "b"}, nil, sandbox.DefaultPolicy())
	if err == nil || conflict.Status == "verified" {
		t.Fatalf("conflict: %+v %v", conflict, err)
	}
	failed, err := Integrate(context.Background(), source, filepath.Join(t.TempDir(), "failed"), [][]byte{fragment("a.txt", "alpha")}, []string{"a"}, [][]string{{"false"}}, sandbox.DefaultPolicy())
	if err == nil || len(failed.Patch) == 0 || failed.Status == "verified" {
		t.Fatalf("verifier: %+v %v", failed, err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := InitSnapshot(context.Background(), source, target); err != nil {
		t.Fatal(err)
	}
	if err := ApplyCandidate(context.Background(), target, c, c.Patch, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyCandidate(context.Background(), target, c, c.Patch, false); err == nil {
		t.Fatal("changed target accepted")
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyCandidate(context.Background(), target, c, c.Patch, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "b.txt"))
	if err != nil || string(data) != "beta\n" {
		t.Fatalf("apply: %q %v", data, err)
	}
}
