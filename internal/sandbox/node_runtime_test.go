package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNVMRuntimeRootNeverGrantsHomeOrArbitraryExecutables(t *testing.T) {
	home := t.TempDir()
	home, _ = filepath.EvalSymlinks(home)
	root := filepath.Join(home, ".nvm", "versions", "node", "v24.19.0")
	for _, path := range []string{filepath.Join(root, "bin", "node"), filepath.Join(root, "lib", "node_modules", "npm", "bin", "npm-cli.js"), filepath.Join(home, "secrets", "npm"), filepath.Join(root, "etc", "registry-tokens")} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{filepath.Join(root, "bin", "node"), filepath.Join(root, "lib", "node_modules", "npm", "bin", "npm-cli.js")} {
		if got := nvmRuntimeRoot(name, home); got != root {
			t.Fatalf("runtime=%q", got)
		}
	}
	for _, name := range []string{filepath.Join(home, "secrets", "npm"), filepath.Join(root, "etc", "registry-tokens")} {
		if got := nvmRuntimeRoot(name, home); got != "" {
			t.Fatalf("arbitrary file granted %q", got)
		}
	}
	if runtime.GOOS != "windows" {
		linked := filepath.Join(root, "bin", "npm")
		if err := os.Symlink("../lib/node_modules/npm/bin/npm-cli.js", linked); err != nil {
			t.Fatal(err)
		}
		if got := nvmRuntimeRoot(linked, home); got != root {
			t.Fatal("normal NVM npm link was not resolved")
		}
	}
}

func TestStrictNVMNPMCanRunDisposableProjectWithoutHomeAccess(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native macOS Seatbelt regression; Linux runtime acceptance is separate")
	}
	if _, err := StrictAvailability(); err != nil {
		t.Skip(err)
	}
	executable, err := exec.LookPath("npm")
	if err != nil {
		t.Skip("npm unavailable")
	}
	home, err := os.UserHomeDir()
	if err != nil || nvmRuntimeRoot(executable, home) == "" {
		t.Skip("host npm is not an NVM installation")
	}
	root := t.TempDir()
	for name, contents := range map[string]string{"package.json": `{"private":true,"scripts":{"test":"node verify.cjs"}}`, "verify.cjs": `console.log("isolated-nvm-check");`} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Probe the exact child-spawn failure without reading host diagnostics.
	probe, err := Prepare(context.Background(), Request{Dir: root, Argv: []string{"node", "-e", `const cp=require("node:child_process");const r=cp.spawnSync("sh",["-c","node verify.cjs"],{stdio:"inherit"});console.log(JSON.stringify({status:r.status,error:r.error&&{code:r.error.code,message:r.error.message}}));`}, Policy: DefaultPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	debug, debugErr := probe.Command.CombinedOutput()
	probe.Cleanup()
	if debugErr != nil || !strings.Contains(string(debug), `"status":0`) {
		t.Fatalf("controlled Node child probe: %v %s", debugErr, debug)
	}
	prepared, err := Prepare(context.Background(), Request{Dir: root, Argv: []string{"npm", "test"}, Policy: DefaultPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Cleanup()
	output, err := prepared.Command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "isolated-nvm-check") {
		t.Fatalf("strict NVM npm: %v %s", err, output)
	}
}
