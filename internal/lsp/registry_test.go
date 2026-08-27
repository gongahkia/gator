package lsp

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/workspace"
)

func TestRegistryReusesCompatibleTrustedManager(t *testing.T) {
	registry := newRegistry(2)
	set := registryTestSet(t, "first")

	first, releaseFirst, err := registry.Acquire(set)
	if err != nil {
		t.Fatalf("acquire first manager: %v", err)
	}
	connection := &fakeClient{}
	first.clients["fixture"] = connection
	releaseFirst()

	second, releaseSecond, err := registry.Acquire(set)
	if err != nil {
		t.Fatalf("acquire reused manager: %v", err)
	}
	if second != first || connection.closed {
		t.Fatalf("manager reuse=%t connection=%#v", second == first, connection)
	}
	releaseSecond()
	registry.Close()
	if !connection.closed {
		t.Fatal("closing registry did not close cached LSP client")
	}
}

func TestRegistryRetiresUntrustedAndChangedConfiguration(t *testing.T) {
	registry := newRegistry(2)
	firstSet := registryTestSet(t, "first")
	first, release, err := registry.Acquire(firstSet)
	if err != nil {
		t.Fatalf("acquire first manager: %v", err)
	}
	connection := &fakeClient{}
	first.clients["fixture"] = connection
	release()

	changedSet := registryTestSetForRoot(firstSet.root, "second")
	registry.Reconcile(changedSet)
	if !connection.closed {
		t.Fatal("changed configuration did not retire cached manager")
	}
	second, releaseSecond, err := registry.Acquire(changedSet)
	if err != nil {
		t.Fatalf("acquire changed manager: %v", err)
	}
	if second == first {
		t.Fatal("changed configuration reused retired manager")
	}
	secondConnection := &fakeClient{}
	second.clients["fixture"] = secondConnection
	releaseSecond()
	registry.Reconcile(Set{root: firstSet.root})
	if !secondConnection.closed {
		t.Fatal("untrusted configuration did not retire cached manager")
	}
}

func TestLoadWithoutManifestKeepsRootForRegistryReconciliation(t *testing.T) {
	root := testWorkspace(t)
	set, err := Load(root.Path(), "")
	if err != nil {
		t.Fatalf("load unconfigured workspace: %v", err)
	}
	if set.Configured() || set.Trusted() || set.root.Path() != root.Path() {
		t.Fatalf("unconfigured LSP set = %#v", set)
	}
}

func TestRegistryEvictsOldestIdleManagerAndRejectsActiveOverflow(t *testing.T) {
	registry := newRegistry(2)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	registry.now = func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	firstSet := registryTestSet(t, "first")
	secondSet := registryTestSet(t, "second")
	thirdSet := registryTestSet(t, "third")
	first, releaseFirst, err := registry.Acquire(firstSet)
	if err != nil {
		t.Fatalf("acquire first manager: %v", err)
	}
	connection := &fakeClient{}
	first.clients["fixture"] = connection
	releaseFirst()
	_, releaseSecond, err := registry.Acquire(secondSet)
	if err != nil {
		t.Fatalf("acquire second manager: %v", err)
	}
	releaseSecond()
	_, releaseThird, err := registry.Acquire(thirdSet)
	if err != nil {
		t.Fatalf("acquire third manager: %v", err)
	}
	releaseThird()
	if !connection.closed {
		t.Fatal("oldest idle manager was not evicted")
	}

	full := newRegistry(1)
	_, releaseActive, err := full.Acquire(firstSet)
	if err != nil {
		t.Fatalf("acquire active manager: %v", err)
	}
	if _, _, err := full.Acquire(secondSet); err == nil {
		t.Fatal("active-only registry accepted an incompatible manager")
	}
	releaseActive()
	full.Close()
}

func TestRegistrySnapshotDoesNotCreateOrStartManagers(t *testing.T) {
	registry := newRegistry(2)
	if snapshot := registry.Snapshot(); len(snapshot) != 0 {
		t.Fatalf("empty snapshot = %#v", snapshot)
	}
	if len(registry.entries) != 0 {
		t.Fatalf("snapshot created entries: %#v", registry.entries)
	}
	set := registryTestSet(t, "idle")
	manager := set.NewManager()
	connects := 0
	manager.connect = func(context.Context, workspace.Root, server) (client, error) {
		connects++
		return &fakeClient{}, nil
	}
	registry.entries[registryKey(set.root.Path(), set.configuredHash)] = &registryEntry{
		root: set.root.Path(), hash: set.configuredHash, manager: manager,
	}
	snapshot := registry.Snapshot()
	if len(snapshot) != 1 || snapshot[0].Hash != "idle" || len(snapshot[0].Servers) != 1 || snapshot[0].Servers[0].Started {
		t.Fatalf("idle snapshot = %#v", snapshot)
	}
	if connects != 0 {
		t.Fatalf("snapshot started %d language server(s)", connects)
	}
	if len(registry.entries) != 1 {
		t.Fatalf("snapshot mutated entries: %#v", registry.entries)
	}
}

func TestManagerSerializesRequestsOverSharedTransport(t *testing.T) {
	connection := &serialClient{started: make(chan struct{}), unblock: make(chan struct{})}
	manager := &Manager{
		root:    testWorkspace(t),
		trusted: true,
		clients: map[string]client{"fixture": connection},
	}
	specification := server{Name: "fixture", Language: "go"}
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			if _, err := manager.request(context.Background(), specification, hoverOperation, "textDocument/hover", nil); err != nil {
				t.Errorf("shared manager request: %v", err)
			}
		}()
	}
	<-connection.started
	select {
	case <-connection.second:
		t.Fatal("LSP transport received concurrent requests")
	case <-time.After(100 * time.Millisecond):
	}
	close(connection.unblock)
	wait.Wait()
	if atomic.LoadInt32(&connection.maximum) != 1 {
		t.Fatalf("maximum concurrent LSP requests = %d", connection.maximum)
	}
}

func registryTestSet(t *testing.T, hash string) Set {
	t.Helper()
	return registryTestSetForRoot(testWorkspace(t), hash)
}

func registryTestSetForRoot(root workspace.Root, hash string) Set {
	return Set{
		configuredHash: hash,
		trusted:        true,
		root:           root,
		servers:        []server{{Name: "fixture", Language: "go"}},
	}
}

type serialClient struct {
	started chan struct{}
	second  chan struct{}
	unblock chan struct{}
	once    sync.Once
	calls   int32
	active  int32
	maximum int32
}

func (c *serialClient) Request(context.Context, string, any) (json.RawMessage, error) {
	calls := atomic.AddInt32(&c.calls, 1)
	if calls == 2 && c.second != nil {
		close(c.second)
	}
	active := atomic.AddInt32(&c.active, 1)
	for {
		maximum := atomic.LoadInt32(&c.maximum)
		if active <= maximum || atomic.CompareAndSwapInt32(&c.maximum, maximum, active) {
			break
		}
	}
	c.once.Do(func() { close(c.started) })
	<-c.unblock
	atomic.AddInt32(&c.active, -1)
	return json.RawMessage(`{"contents":"fixture"}`), nil
}

func (c *serialClient) Supports(lspOperation) bool { return true }
func (c *serialClient) Close() error               { return nil }
