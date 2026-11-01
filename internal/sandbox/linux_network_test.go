package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestLinuxNetworkGrantOverridesUnshareAll(t *testing.T) {
	bin := t.TempDir()
	name := "bwrap"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	// Inspect the actual prepared argv without running a namespace helper or
	// making a network request. Native Linux acceptance remains separate.
	if err := os.WriteFile(filepath.Join(bin, name), []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	for _, network := range []Network{AllowNetwork, DenyNetwork} {
		root, scratch := t.TempDir(), t.TempDir()
		command, err := prepareLinux(context.Background(), root, scratch, []string{"true"}, nil, Policy{Mode: Strict, Network: network})
		if err != nil {
			t.Fatal(err)
		}
		unshare, share := slices.Index(command.Args, "--unshare-all"), slices.Index(command.Args, "--share-net")
		if unshare < 0 || network == AllowNetwork && share <= unshare || network == DenyNetwork && share >= 0 {
			t.Fatalf("network %q has incorrect namespace policy: %v", network, command.Args)
		}
		if network == DenyNetwork && !slices.Contains(command.Args, "--unshare-net") {
			t.Fatal("network denial must remain explicit")
		}
	}
}
