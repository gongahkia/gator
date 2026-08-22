package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBundleHashPinsManifestAndExecutableContents(t *testing.T) {
	repository := hookRepository(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"decision":"allow","message":"checked"}'
`)
	digest, err := BundleHash(repository)
	if err != nil {
		t.Fatalf("hash hooks: %v", err)
	}
	if len(digest) != 64 {
		t.Fatalf("hook hash = %q", digest)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gator", "hooks", "allow"), []byte("#!/bin/sh\nprintf '%s\\n' '{\"decision\":\"deny\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	changed, err := BundleHash(repository)
	if err != nil {
		t.Fatalf("hash changed hooks: %v", err)
	}
	if changed == digest {
		t.Fatal("hook hash did not change after executable changed")
	}
}

func TestLoadDisablesUntrustedHookBundleAndRunsTrustedBundle(t *testing.T) {
	repository := hookRepository(t, `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"decision":"allow","message":"checked"}'
`)
	digest, err := BundleHash(repository)
	if err != nil {
		t.Fatal(err)
	}
	untrusted, err := Load(repository, repository, "")
	if err != nil {
		t.Fatal(err)
	}
	if !untrusted.Configured() || untrusted.Trusted() {
		t.Fatalf("untrusted engine = %#v", untrusted)
	}
	if err := untrusted.Run(context.Background(), PreToolUse, "apply_patch", map[string]any{"test": true}); err != nil {
		t.Fatalf("untrusted hook run = %v", err)
	}
	trusted, err := Load(repository, repository, digest)
	if err != nil {
		t.Fatal(err)
	}
	if !trusted.Trusted() {
		t.Fatal("trusted engine is inactive")
	}
	var statuses []Status
	trusted.Emit = func(status Status) { statuses = append(statuses, status) }
	if err := trusted.Run(context.Background(), PreToolUse, "apply_patch", map[string]any{"test": true}); err != nil {
		t.Fatalf("trusted hook run: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Hook != "guard" || !statuses[0].Allowed {
		t.Fatalf("hook statuses = %#v", statuses)
	}
}

func TestLoadDisablesChangedBundleUntilNewTrust(t *testing.T) {
	repository := hookRepository(t, `#!/bin/sh
printf '%s\n' '{"decision":"allow"}'
`)
	digest, err := BundleHash(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gator", "hooks", "allow"), []byte("#!/bin/sh\nprintf '%s\\n' '{\"decision\":\"allow\",\"message\":\"changed\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	engine, err := Load(repository, repository, digest)
	if err != nil {
		t.Fatal(err)
	}
	if !engine.Configured() || engine.Trusted() {
		t.Fatalf("changed engine = %#v", engine)
	}
}

func hookRepository(t *testing.T, script string) string {
	t.Helper()
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, ".gator", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version": 1,
  "hooks": [{
    "name": "guard",
    "event": "pre_tool_use",
    "tool": "apply_patch",
    "command": [".gator/hooks/allow"]
  }]
}`
	if err := os.WriteFile(filepath.Join(repository, ".gator", "hooks.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gator", "hooks", "allow"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return repository
}
