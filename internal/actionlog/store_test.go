package actionlog

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
)

func proposalFixture(t *testing.T) action.Proposal {
	t.Helper()
	p, err := action.NewProposal("action-fixture", action.ConnectedMutate, "google", "gmail_send", "https://example.test/send", `{"to":"one@example.test"}`, []byte(`{"to":"one@example.test"}`))
	if err != nil {
		t.Fatal(err)
	}
	p.CorrelationID = strings.Repeat("a", 32)
	return p
}
func TestActionLogPersistsBeforeDispatchAndRetainsImmutableOutcome(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proposal := proposalFixture(t)
	attempt, err := store.Begin("work-chat", "run-one", proposal, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer attempt.Close()
	entries, err := store.List("run-one")
	if err != nil || len(entries) != 1 || !entries[0].Live || entries[0].Phase != "executing" || entries[0].Action.Status != action.Unknown || entries[0].Action.Proposal != proposal {
		t.Fatal("pre-dispatch receipt missing", entries, err)
	}
	record := action.Record{Proposal: proposal, Status: action.Executed, ReceiptID: "provider-one"}
	wrong := record
	wrong.Proposal.Target = "https://other.test"
	if err := attempt.Finish(wrong); err == nil {
		t.Fatal("receipt changed approved target")
	}
	if err := attempt.Finish(record); err != nil {
		t.Fatal(err)
	}
	if err := attempt.Finish(record); err != nil {
		t.Fatal("identical finish was not idempotent", err)
	}
	if err := attempt.Finish(action.Record{Proposal: proposal, Status: action.Failed, Error: "other"}); err == nil {
		t.Fatal("terminal outcome was overwritten")
	}
	entries, err = store.List("")
	if err != nil || len(entries) != 1 || entries[0].Live || entries[0].Phase != "settled" || entries[0].Action.ReceiptID != "provider-one" || entries[0].FinishedAt.IsZero() {
		t.Fatal("terminal receipt unavailable", entries, err)
	}
	if err := attempt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := attempt.Finish(record); err == nil {
		t.Fatal("closed attempt accepted a write")
	}
}
func TestActionLogConcurrentBeginsHaveIndependentExactOwners(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := proposalFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := fmt.Sprintf("chat-%d", i)
			attempt, err := store.Begin(owner, "same-run", p, time.Now())
			if err != nil {
				t.Error(err)
				return
			}
			defer attempt.Close()
			if i%2 == 0 {
				if err := attempt.Finish(action.Record{Proposal: p, Status: action.Failed, Error: "provider rejected"}); err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
	entries, err := store.List("same-run")
	if err != nil || len(entries) != 12 {
		t.Fatal("concurrent entries lost", len(entries), err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.ID] || seen[e.ConversationID] || e.Live {
			t.Fatal("duplicate or live abandoned identity", e)
		}
		seen[e.ID] = true
		seen[e.ConversationID] = true
	}
}
func TestActionLogBoundsAndCorruptionFailClosed(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin("../chat", "run", proposalFixture(t), time.Now()); err == nil {
		t.Fatal("unowned traversal accepted")
	}
	attempt, err := store.Begin("chat", "run", proposalFixture(t), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	attempt.Close()
	entries, err := store.List("run")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.path, "run", entries[0].ID+".json")
	data, _ := os.ReadFile(path)
	for _, bad := range [][]byte{append(data, []byte(`{}`)...), []byte(strings.Repeat("x", maxEntryBytes+1)), []byte(strings.Replace(string(data), `"run_id":"run"`, `"run_id":"other"`, 1))} {
		if err := os.WriteFile(path, bad, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.List(""); err == nil {
			t.Fatal("corrupt journal treated as empty")
		}
	}
	if _, err := store.List("../other"); err == nil {
		t.Fatal("invalid run read accepted")
	}
}
func TestActionLogCrashHelper(t *testing.T) {
	state := os.Getenv("GATOR_TEST_ACTIONLOG_CHILD")
	if state == "" {
		return
	}
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Begin("crash-chat", "crash-run", proposalFixture(t), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer attempt.Close()
	fmt.Println("ready")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
	os.Exit(9)
}
func TestActionLogProcessDeathLeavesUncertainReceiptWithoutReplaying(t *testing.T) {
	state := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestActionLogCrashHelper$")
	child.Env = append(os.Environ(), "GATOR_TEST_ACTIONLOG_CHILD="+state)
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "ready\n" {
		t.Fatal("child did not retain request", line, err)
	}
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := store.List("crash-run")
	if err != nil || len(entries) != 1 || !entries[0].Live {
		t.Fatal("active child lease missing", entries, err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	reopened, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	entries, err = reopened.List("crash-run")
	if err != nil || len(entries) != 1 || entries[0].Live || entries[0].Action.Status != action.Unknown || entries[0].Phase != "executing" {
		t.Fatal("crashed dispatch was mistaken for failure or resumed", entries, err)
	}
	// Reading is observational: it cannot turn an unconfirmed dispatch into a
	// new request, manufacture a provider receipt, or erase its identity.
	again, err := reopened.List("")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(entries)
	after, _ := json.Marshal(again)
	if string(before) != string(after) {
		t.Fatal("reopening changed uncertain request")
	}
}
