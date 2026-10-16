package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSystemdJobUnitQuotesPathsAndRejectsControlCharacters(t *testing.T) {
	unit, err := systemdJobUnit("/opt/Gator App/gator%v", "/tmp/gator state", "/tmp/gator config")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`ExecStart="/opt/Gator App/gator%%v" job supervisor`, `Environment="GATOR_STATE_DIR=/tmp/gator state"`, `Environment="GATOR_CONFIG_DIR=/tmp/gator config"`, "WantedBy=default.target"} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit missing %q:\n%s", want, unit)
		}
	}
	for _, input := range []struct{ executable, state, config string }{{"relative", "/tmp/state", ""}, {"/bin/gator", "/tmp/state", "relative"}, {"/bin/gator\nExecStart=/bin/sh", "/tmp/state", ""}} {
		if _, err := systemdJobUnit(input.executable, input.state, input.config); err == nil {
			t.Fatalf("accepted invalid unit paths: %+v", input)
		}
	}
}

func TestJobSystemdManagesOnlyItsUserUnit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd user service is Linux-only")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "systemctl.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$GATOR_SYSTEMCTL_LOG\"\n"
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GATOR_SYSTEMCTL_LOG", logPath)
	t.Setenv("GATOR_STATE_DIR", filepath.Join(root, "state"))
	t.Setenv("GATOR_CONFIG_DIR", filepath.Join(root, "gator config"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg config"))
	var output bytes.Buffer
	if err := jobSystemd([]string{"install"}, &output); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(root, "xdg config", "systemd", "user", jobSystemdUnitName)
	info, err := os.Stat(unitPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("installed unit = %v, %v", info, err)
	}
	unit, err := os.ReadFile(unitPath)
	if err != nil || !strings.Contains(string(unit), `Environment="GATOR_CONFIG_DIR=`) {
		t.Fatalf("unit = %s, %v", unit, err)
	}
	if err := jobSystemd([]string{"remove"}, &output); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("remove without confirmation = %v", err)
	}
	if err := jobSystemd([]string{"status"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := jobSystemd([]string{"remove", "--yes"}, &output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("unit still exists: %v", err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--user daemon-reload", "--user enable --now " + jobSystemdUnitName, "--user --no-pager status " + jobSystemdUnitName, "--user disable --now " + jobSystemdUnitName} {
		if !strings.Contains(string(logData), want) {
			t.Fatalf("systemctl calls missing %q:\n%s", want, logData)
		}
	}
}
