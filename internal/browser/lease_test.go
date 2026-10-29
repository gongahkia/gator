package browser

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
)

func TestControllerLeaseDoesNotRevokeLiveOrLegacySessions(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Start(StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = CreateToken(store, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Missing lease means an old controller or a first startup, not a crash.
	if recovered, err := store.recoverController(session.ID); err != nil || recovered {
		t.Fatalf("legacy/startup session: %t, %v", recovered, err)
	}
	lease, err := store.controllerLease(session.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if recovered, err := store.recoverController(session.ID); err != nil || recovered {
		t.Fatalf("live session: %t, %v", recovered, err)
	}
	second, err := store.controllerLease(session.ID, true)
	if second != nil {
		_ = second.Close()
	}
	if !errors.Is(err, filelock.ErrWouldBlock) {
		t.Fatalf("second controller acquired ownership: %v", err)
	}
	current, err := store.Get(session.ID)
	if err != nil || current.State != StateRunning {
		t.Fatalf("live session changed: %#v, %v", current, err)
	}
	if _, err := os.Stat(tokenPath(store, session.ID)); err != nil {
		t.Fatal(err)
	}
	_ = lease.Close()
	if recovered, err := store.recoverController(session.ID); err != nil || !recovered {
		t.Fatalf("released owner: %t, %v", recovered, err)
	}
	current, err = store.Get(session.ID)
	if err != nil || current.State != StateStopped {
		t.Fatalf("dead owner not revoked: %#v, %v", current, err)
	}
	if _, err := os.Stat(tokenPath(store, session.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dead owner token retained: %v", err)
	}
}

func TestDuplicateControllerCannotRevokeOrReplaceLiveOwner(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Start(StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := CreateToken(store, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	config := ServiceConfig{Store: store, SessionID: session.ID, Token: token, Driver: &serviceDriver{}, Socket: socketPath(store, session.ID)}
	first, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- first.Run(ctx) }()
	waitForBrowserSocket(t, config.Socket)
	second, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Run(ctx); !errors.Is(err, filelock.ErrWouldBlock) {
		t.Fatalf("duplicate service: %v", err)
	}
	client, err := NewClient(store, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := client.Session(ctx, session.ID)
	if err != nil || current.State != StateRunning {
		t.Fatalf("original controller damaged: %#v, %v", current, err)
	}
	if _, err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("controller did not close")
	}
}
