package cliagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/patch"
)

func TestSessionRetainsNativeHomeCheckoutAndTrustedPatches(t *testing.T) {
	r := fixtureRequest(t, "")
	script, err := os.ReadFile("testdata/codex_rpc.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.Config.Executable, script, 0700); err != nil {
		t.Fatal(err)
	}
	r.Edit = true
	request := SessionRequest{Request: r, Store: filepath.Join(r.Scratch, "sessions"), Owner: "conversation", Capture: "source-one"}
	var first string
	for i, want := range []string{"after-1", "after-2"} {
		value, err := RunSession(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(value.Patch), "+"+want) || !strings.Contains(string(value.Patch), "-original") {
			t.Fatalf("wrong cumulative patch: %s", value.Patch)
		}
		if i == 0 {
			first = value.SessionID
		} else if value.SessionID != first {
			t.Fatal("lost session")
		}
		target := filepath.Join(r.Scratch, want)
		if err := patch.InitSnapshot(context.Background(), r.Source, target); err != nil {
			t.Fatal(err)
		}
		if err := patch.ApplySnapshot(context.Background(), target, value.Patch); err != nil {
			t.Fatal(err)
		}
		request.SessionID, request.Task = value.SessionID, "Revise the same file"
	}
	data, err := os.ReadFile(filepath.Join(r.Source, "note.txt"))
	if err != nil || string(data) != "original\n" {
		t.Fatal("source changed", err)
	}
	request.Owner = "other-conversation"
	if _, err := RunSession(context.Background(), request); err == nil {
		t.Fatal("cross-conversation session access")
	}
}

func TestSessionDoesNotAcceptNewBaselineOnContinuation(t *testing.T) {
	dir, binding := sessionFixture(t)
	s := readySession(t, dir, binding)
	id := s.record.ID
	s.Close()
	t.Setenv("OPENAI_API_KEY", "synthetic-key")
	_, err := RunSession(context.Background(), SessionRequest{Request: Request{Config: binding.Config, Task: "continue", Baseline: []byte("different")}, Store: dir, SessionID: id, Owner: binding.Owner, Capture: binding.Capture})
	if err == nil || !strings.Contains(err.Error(), "existing") && !strings.Contains(err.Error(), "retained checkout") {
		t.Fatalf("accepted changed baseline: %v", err)
	}
}
