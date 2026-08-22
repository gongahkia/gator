package sandbox

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPolicyRejectsBroadAndMalformedGrants(t *testing.T) {
	for _, policy := range []Policy{
		{Mode: "nope"},
		{Network: "maybe"},
		{ReadOnlyRoots: []string{"relative"}},
		{WritableRoots: []string{string(filepath.Separator)}},
		{Environment: []string{"NOT-VALID"}},
	} {
		if err := policy.Validate(); err == nil {
			t.Fatalf("policy %#v unexpectedly validated", policy)
		}
	}
}

func TestStrictSandboxRunsAStandardGoVerifier(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("strict sandbox has no implementation on this platform")
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("sandbox-exec"); err != nil {
			t.Skip("sandbox-exec is unavailable")
		}
	}
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("bwrap"); err != nil {
			t.Skip("bubblewrap is unavailable")
		}
	}
	root := t.TempDir()
	writeSandboxFixture(t, root, "go.mod", "module example.com/sandboxfixture\n\ngo 1.25.0\n")
	writeSandboxFixture(t, root, "greeting.go", "package sandboxfixture\n\nfunc Greeting() string { return \"hello\" }\n")
	writeSandboxFixture(t, root, "greeting_test.go", "package sandboxfixture\n\nimport \"testing\"\n\nfunc TestGreeting(t *testing.T) { if Greeting() != \"hello\" { t.Fatal(\"bad greeting\") } }\n")
	prepared, err := Prepare(context.Background(), Request{Dir: root, Argv: []string{"go", "test", "./..."}, Policy: DefaultPolicy()})
	if err != nil {
		t.Fatalf("prepare strict command: %v", err)
	}
	defer prepared.Cleanup()
	var output bytes.Buffer
	prepared.Command.Stdout = &output
	prepared.Command.Stderr = &output
	if err := prepared.Command.Run(); err != nil {
		t.Fatalf("strict Go verifier failed: %v\n%s", err, output.String())
	}
}

func TestStrictSandboxDoesNotWriteOutsideWorktree(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Seatbelt boundary test runs on macOS")
	}
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec is unavailable")
	}
	root := t.TempDir()
	external := t.TempDir()
	marker := filepath.Join(external, "outside")
	prepared, err := Prepare(context.Background(), Request{Dir: root, Argv: []string{"/bin/sh", "-c", "touch \"$1\"", "gator", marker}, Policy: DefaultPolicy()})
	if err != nil {
		t.Fatalf("prepare strict command: %v", err)
	}
	defer prepared.Cleanup()
	output, err := prepared.Command.CombinedOutput()
	if err == nil {
		t.Fatalf("sandbox command unexpectedly wrote outside its worktree: %s", output)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("sandbox command created external marker: %v", statErr)
	}
}

func writeSandboxFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDarwinProfileEscapesPaths(t *testing.T) {
	profile := darwinProfile([]string{"/tmp/a\\b\"c"}, []string{"/tmp/write"}, DenyNetwork)
	if !strings.Contains(profile, `a\\b\"c`) || !strings.Contains(profile, "(deny network*)") {
		t.Fatalf("profile = %s", profile)
	}
}
