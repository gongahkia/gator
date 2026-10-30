package cliagent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/filelock"
)

func sessionFixture(t *testing.T) (string, SessionBinding) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir, SessionBinding{Owner: "conversation-one", Capture: "snapshot-one", Config: Config{Kind: "codex", Executable: filepath.Join(dir, "codex"), APIKeyEnv: "OPENAI_API_KEY", TimeoutSeconds: 60}}
}

func readySession(t *testing.T, dir string, binding SessionBinding) *sessionLease {
	t.Helper()
	s, err := openSession(dir, "", binding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.record.Initialized = true
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSessionExclusiveOwnerAndAuthority(t *testing.T) {
	dir, binding := sessionFixture(t)
	s := readySession(t, dir, binding)
	id := s.record.ID
	if _, err := openSession(dir, id, binding); !errors.Is(err, filelock.ErrWouldBlock) {
		t.Fatalf("second writer acquired live lease: %v", err)
	}
	s.Close()
	for _, change := range []func(*SessionBinding){
		func(b *SessionBinding) { b.Owner = "another-conversation" },
		func(b *SessionBinding) { b.Capture = "another-source" },
		func(b *SessionBinding) { b.Config.AllowEdits = true; b.Edit = true },
		func(b *SessionBinding) { b.Scopes = []string{"src"} },
		func(b *SessionBinding) { b.Config.BaseURL = "https://other.example/v1" },
	} {
		other := binding
		change(&other)
		if lease, err := openSession(dir, id, other); err == nil {
			lease.Close()
			t.Fatal("resumed with changed authority")
		}
	}
	resumed, err := openSession(dir, id, binding)
	if err != nil {
		t.Fatal(err)
	}
	resumed.Close()
}

func TestSessionAdmissionIsDurableAndIdempotent(t *testing.T) {
	dir, binding := sessionFixture(t)
	s := readySession(t, dir, binding)
	id := s.record.ID
	payload := []byte("synthetic-private-answer-never-store-this")
	if send, err := s.admit("command-one", "send", payload); !send || err != nil {
		t.Fatal(send, err)
	}
	data, err := os.ReadFile(filepath.Join(s.path, "session.json"))
	if err != nil || !strings.Contains(string(data), "admitted") || strings.Contains(string(data), string(payload)) {
		t.Fatalf("bad admission record: %v", err)
	}
	if send, err := s.admit("command-one", "send", payload); send || err != nil {
		t.Fatalf("duplicate would execute again: %v %v", send, err)
	}
	if _, err := s.admit("command-one", "send", []byte("different")); err == nil {
		t.Fatal("accepted reused ID for different input")
	}
	if err := s.commandStatus("command-one", "acknowledged"); err != nil {
		t.Fatal(err)
	}
	if err := s.commandStatus("command-one", "completed"); err != nil {
		t.Fatal(err)
	}
	if err := s.commandStatus("command-one", "acknowledged"); err == nil {
		t.Fatal("regressed completed command")
	}
	s.Close()
	s, err = openSession(dir, id, binding)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if send, err := s.admit("command-one", "send", payload); send || err != nil {
		t.Fatalf("duplicate after restart would execute: %v %v", send, err)
	}
}

func TestSessionUncertainRecoveryDoesNotReplay(t *testing.T) {
	for _, stage := range []string{"initializing", "process_open", "admitted", "acknowledged"} {
		t.Run(stage, func(t *testing.T) {
			dir, binding := sessionFixture(t)
			s := readySession(t, dir, binding)
			id := s.record.ID
			switch stage {
			case "initializing":
				s.record.Initialized = false
			case "process_open":
				s.record.ProcessOpen = true
			default:
				if _, err := s.admit("one", "send", []byte("edit")); err != nil {
					t.Fatal(err)
				}
				if stage == "acknowledged" {
					if err := s.commandStatus("one", stage); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := s.saveLocked(); err != nil {
				t.Fatal(err)
			}
			s.Close() // models lease release on process death; metadata persists
			if _, err := openSession(dir, id, binding); !errors.Is(err, ErrSessionReconciliation) {
				t.Fatalf("uncertain state was resumed: %v", err)
			}
		})
	}
}

func TestSessionRejectsSymlinksAndFailedJournal(t *testing.T) {
	dir, binding := sessionFixture(t)
	s := readySession(t, dir, binding)
	id := s.record.ID
	s.Close()
	if err := os.Rename(filepath.Join(dir, id), filepath.Join(dir, "real")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, id)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := openSession(dir, id, binding); err == nil {
		t.Fatal("followed session symlink")
	}
	s = readySession(t, dir, binding)
	if err := s.root.Remove("session.json"); err != nil {
		t.Fatal(err)
	}
	if err := s.root.Mkdir("session.json", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.admit("one", "send", []byte("edit")); err == nil {
		t.Fatal("ignored journal failure")
	}
	if _, err := s.admit("one", "send", []byte("edit")); err == nil {
		t.Fatal("continued with poisoned journal")
	}
}
