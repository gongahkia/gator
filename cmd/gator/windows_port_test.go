//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/state"
	sharedusage "github.com/gongahkia/gator/internal/usage"
)

func TestWindowsProfileRoundTrip(t *testing.T) {
	base := filepath.Join(t.TempDir(), "a profile & 🐊")
	root, configuration, drafts := filepath.Join(base, "state"), filepath.Join(base, "config"), filepath.Join(base, "desktop", "drafts")
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", configuration)
	t.Setenv("GATOR_DESKTOP_DRAFT_DIR", drafts)
	t.Setenv("GATOR_INTERNAL_COMMANDS", "1")
	if err := preparePlatformProfile(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "gator"), filepath.Join(configuration, "gator"), drafts} {
		if err := privatefs.Check(path); err != nil {
			t.Fatalf("private profile %s: %v", path, err)
		}
	}
	credentials, err := auth.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put("openai", auth.Credential{Type: "api_key", Key: "fixture-only"}); err != nil {
		t.Fatal(err)
	}
	credential, found, err := credentials.Read("openai")
	if err != nil || !found || credential.Key != "fixture-only" {
		t.Fatalf("credential round trip: %v, %t", err, found)
	}
	if err := privatefs.Check(credentials.Path()); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runDesktopRuntime([]string{"note", "add", "--title", "Windows note 🐊", "--body", "A literal saved note."}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runDesktopRuntime([]string{"note", "list"}, &out); err != nil || !strings.Contains(out.String(), "Windows note 🐊") {
		t.Fatalf("knowledge: %s, %v", out.String(), err)
	}
	ledger, err := sharedusage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := ledger.Reserve(time.Now(), "chat", "fixture", 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.Finish(10, true, false, nil); err != nil {
		t.Fatal(err)
	}
	release, err := state.Guard(root, true)
	if err != nil {
		t.Fatalf("finished provider request retained its data lock: %v", err)
	}
	release()
}

func TestWindowsDefaultDirectories(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"GATOR_STATE_DIR", "GATOR_CONFIG_DIR", "XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		t.Setenv(name, "")
	}
	t.Setenv("LOCALAPPDATA", base)
	root, err := state.ResolveDir("")
	if err != nil || root != filepath.Join(base, "Gator", "state") {
		t.Fatalf("state: %s, %v", root, err)
	}
	configuration, err := config.ResolveDir("")
	if err != nil || configuration != filepath.Join(base, "Gator", "config") {
		t.Fatalf("config: %s, %v", configuration, err)
	}
}

func TestWindowsLoginProfileBindsExecutableAndDirectories(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", "")
	t.Setenv("GATOR_CONFIG_DIR", "")
	filename, err := windowsLoginProfilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := privatefs.MkdirAll(filepath.Dir(filename)); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	profile := windowsLoginProfile{Version: 1, Executable: binary, StateDir: filepath.Join(t.TempDir(), "selected state"), ConfigDir: filepath.Join(t.TempDir(), "selected config")}
	data, _ := json.Marshal(profile)
	if err := writePrivateFile(filename, data); err != nil {
		t.Fatal(err)
	}
	if err := loadLoginProfile(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GATOR_STATE_DIR") != profile.StateDir || os.Getenv("GATOR_CONFIG_DIR") != profile.ConfigDir {
		t.Fatal("login profile selected the wrong directories")
	}
	profile.Executable = filepath.Join(t.TempDir(), "other.exe")
	data, _ = json.Marshal(profile)
	if err := writePrivateFile(filename, data); err != nil {
		t.Fatal(err)
	}
	if err := loadLoginProfile(); err == nil {
		t.Fatal("accepted another installation's login profile")
	}
	if !processAlive(os.Getpid()) || processAlive(-1) {
		t.Fatal("incorrect Windows process liveness")
	}
}
