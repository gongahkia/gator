package main

import (
	"bytes"
	"github.com/gongahkia/gator/internal/followup"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServiceRepairRebindsInstallationAndRejectsChangedReview(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Unix login service fixture")
	}
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("GATOR_STATE_DIR", filepath.Join(root, "state"))
	t.Setenv("GATOR_CONFIG_DIR", filepath.Join(root, "configuration"))
	stateDir := os.Getenv("GATOR_STATE_DIR")
	draft, err := followup.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, definition, err := saveFollowup(stateDir, draft)
	if err != nil {
		t.Fatal(err)
	}
	definitionPath := filepath.Join(stateDir, "gator", "jobs", "definitions", definition.ID+".json")
	savedCheck, err := os.ReadFile(definitionPath)
	if err != nil {
		t.Fatal(err)
	}
	path, err := recoveryServicePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	old := launchAgentPlist("com.gator.work-supervisor", filepath.Join(root, "removed", "gator"), stateDir, "/log", os.Getenv("GATOR_CONFIG_DIR"))
	invoke := jobLaunchAgent
	control := "launchctl"
	if runtime.GOOS == "linux" {
		old, err = systemdJobUnit(filepath.Join(root, "removed", "gator"), stateDir, os.Getenv("GATOR_CONFIG_DIR"))
		if err != nil {
			t.Fatal(err)
		}
		invoke, control = jobSystemd, "systemctl"
	}
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, digest, err := recoveryServiceReview(path, stateDir, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "service-calls")
	t.Setenv("GATOR_SERVICE_TEST_LOG", logPath)
	// This fixture never invokes the real user service manager.
	if err := os.WriteFile(filepath.Join(bin, control), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$GATOR_SERVICE_TEST_LOG\"\nif [ \"$GATOR_SERVICE_TEST_FAIL\" = 1 ]; then case \"$*\" in *bootstrap*|*enable*) exit 9;; esac; fi\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	var output bytes.Buffer
	if err := invoke([]string{"install", "--expected", digest}, &output); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	binary, _ := os.Executable()
	expected := launchAgentPlist("com.gator.work-supervisor", binary, stateDir, filepath.Join(stateDir, "gator", "jobs", "launchagent.log"), os.Getenv("GATOR_CONFIG_DIR"))
	if runtime.GOOS == "linux" {
		expected, err = systemdJobUnit(binary, stateDir, os.Getenv("GATOR_CONFIG_DIR"))
		if err != nil {
			t.Fatal(err)
		}
	}
	if string(updated) != expected {
		t.Fatalf("repair did not bind the current installation: %s", updated)
	}
	calls, err := os.ReadFile(logPath)
	if err != nil || len(calls) == 0 {
		t.Fatalf("service not reloaded: %s %v", calls, err)
	}
	for _, args := range [][]string{{"install", "--expected", digest}, {"remove", "--yes", "--expected", digest}} {
		if err := invoke(args, &output); err == nil || !strings.Contains(err.Error(), "changed during review") {
			t.Fatalf("stale review accepted: %v", err)
		}
	}
	after, _ := os.ReadFile(logPath)
	if !bytes.Equal(calls, after) {
		t.Fatal("stale review reached service manager")
	}
	retained, _ := os.ReadFile(path)
	if !bytes.Equal(updated, retained) {
		t.Fatal("stale review changed service")
	}
	_, _, currentDigest, err := recoveryServiceReview(path, stateDir, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATOR_SERVICE_TEST_FAIL", "1")
	if err := invoke([]string{"install", "--expected", currentDigest}, &output); err == nil || !strings.Contains(err.Error(), "is saved") {
		t.Fatalf("activation failure did not disclose saved configuration: %v", err)
	}
	t.Setenv("GATOR_SERVICE_TEST_FAIL", "0")
	if err := invoke([]string{"install", "--expected", currentDigest}, &output); err != nil {
		t.Fatalf("failed activation could not be repaired: %v", err)
	}
	retainedCheck, err := os.ReadFile(definitionPath)
	if err != nil || !bytes.Equal(savedCheck, retainedCheck) {
		t.Fatalf("repair changed the saved check: %v", err)
	}
	calls, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATOR_STATE_DIR", filepath.Join(root, "other-state"))
	if err := invoke([]string{"install"}, &output); err == nil {
		t.Fatal("another profile overwrote service")
	}
	if err := invoke([]string{"remove", "--yes"}, &output); err == nil {
		t.Fatal("another profile removed service")
	}
	after, _ = os.ReadFile(logPath)
	if !bytes.Equal(calls, after) {
		t.Fatal("another profile reached service manager")
	}
}

func TestServiceChangeRejectsLinkedConfiguration(t *testing.T) {
	root := t.TempDir()
	target, link := filepath.Join(root, "target"), filepath.Join(root, "service")
	if err := os.WriteFile(target, []byte(launchAgentPlist("gator", "/gator", root, "/log")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := checkJobServiceChange(link, root, "darwin", ""); err == nil {
		t.Fatal("linked configuration accepted")
	}
}
