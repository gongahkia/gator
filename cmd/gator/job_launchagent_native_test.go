package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Opt in with a freshly built runtime. Uses its own random label and temporary
// profile, never the user's Gator service or ~/Library/LaunchAgents directory.
func TestNativeLaunchAgentExecutableRelocation(t *testing.T) {
	binary := os.Getenv("GATOR_NATIVE_LAUNCHAGENT_BINARY")
	if runtime.GOOS != "darwin" || binary == "" {
		t.Skip("requires macOS login session and GATOR_NATIVE_LAUNCHAGENT_BINARY")
	}
	root := t.TempDir()
	stateDir, configuration := filepath.Join(root, "state"), filepath.Join(root, "configuration")
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("GATOR_CONFIG_DIR", configuration)
	_, _, err := jobStore()
	if err != nil {
		t.Fatal(err)
	}
	oldBinary, newBinary := filepath.Join(root, "old installation", "gator"), filepath.Join(root, "new installation", "gator")
	for _, target := range []string{oldBinary, newBinary} {
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		source, err := os.Open(binary)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			source.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(file, source)
		source.Close()
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatalf("copy runtime: %v %v", copyErr, closeErr)
		}
	}
	label := fmt.Sprintf("com.gator.test.relocation.%d.%d", os.Getpid(), time.Now().UnixNano())
	domain := "gui/" + strconv.Itoa(os.Getuid())
	service := domain + "/" + label
	launchctl := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "/bin/launchctl", args...).CombinedOutput()
	}
	loaded := false
	waitRemoved := func() {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			_, err := launchctl("print", service)
			var exited *exec.ExitError
			if errors.As(err, &exited) && exited.ExitCode() == 113 {
				loaded = false
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("temporary LaunchAgent still registered after bootout")
	}
	t.Cleanup(func() {
		if loaded {
			if output, err := launchctl("bootout", service); err != nil {
				t.Errorf("remove temporary LaunchAgent: %v %s", err, output)
			}
		}
	})
	plistPath := filepath.Join(root, "service.plist")
	logPath := filepath.Join(root, "supervisor.log")
	writeService := func(executable string) {
		t.Helper()
		if err := writePrivateFile(plistPath, []byte(launchAgentPlist(label, executable, stateDir, logPath, configuration))); err != nil {
			t.Fatal(err)
		}
	}
	start := func() {
		t.Helper()
		if output, err := launchctl("bootstrap", domain, plistPath); err != nil {
			t.Fatalf("bootstrap temporary service: %v %s", err, output)
		}
		loaded = true
	}
	waitHealth := func(want bool) supervisorState {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			var output bytes.Buffer
			healthErr := jobSupervisorRequestAt(stateDir, "health", &output)
			if (healthErr == nil) == want {
				var state supervisorState
				if want {
					data, err := os.ReadFile(filepath.Join(stateDir, "gator", "jobs", "supervisor.json"))
					if err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(data, &state); err != nil {
						t.Fatal(err)
					}
				}
				return state
			}
			time.Sleep(100 * time.Millisecond)
		}
		log, _ := os.ReadFile(logPath)
		t.Fatalf("supervisor health did not become %t: %s", want, log)
		return supervisorState{}
	}
	writeService(oldBinary)
	start()
	first := waitHealth(true)
	if output, err := launchctl("bootout", service); err != nil {
		t.Fatalf("unload old installation: %v %s", err, output)
	}
	waitRemoved()
	waitHealth(false)
	if err := os.Remove(oldBinary); err != nil {
		t.Fatal(err)
	}
	// The old executable no longer exists. Publish the same generated service
	// shape with the replacement path and verify a new authenticated supervisor.
	writeService(newBinary)
	start()
	second := waitHealth(true)
	if first.PID == second.PID || first.Nonce == second.Nonce {
		t.Fatal("old supervisor identity survived relocation")
	}
	output, err := launchctl("print", service)
	if err != nil || !strings.Contains(string(output), newBinary) {
		t.Fatalf("launchd is not using replacement executable: %v %s", err, output)
	}
	if output, err := launchctl("bootout", service); err != nil {
		t.Fatalf("remove relocated service: %v %s", err, output)
	}
	waitRemoved()
	waitHealth(false)
	t.Log("old supervisor stopped; new executable served authenticated health; temporary service removed")
}
