package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/browser"
	"github.com/gongahkia/gator/internal/workapp"
)

type controlsBrowserDriver struct {
	browser.Driver
	tabs []browser.Tab
}

func (d *controlsBrowserDriver) CandidateTabs(context.Context) ([]browser.Tab, error) {
	return d.tabs, nil
}
func (*controlsBrowserDriver) SetOrigins(context.Context, []browser.Origin) error { return nil }
func (*controlsBrowserDriver) InvalidateReferences(context.Context) error         { return nil }
func (*controlsBrowserDriver) Close() error                                       { return nil }

func browserControlsFixture(t *testing.T) (string, *browser.Store, browser.Session) {
	t.Helper()
	root := t.TempDir()
	store, err := browser.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Start(browser.StartOptions{Headed: true, Paused: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err = store.SelectTabs(session.ID, []browser.Tab{{ID: "tab-one", URL: "about:blank"}})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := browser.CreateToken(store, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := browser.NewService(browser.ServiceConfig{Store: store, SessionID: session.ID, Token: token, Socket: browser.SocketPath(store, session.ID), Driver: &controlsBrowserDriver{tabs: session.SelectedTabs}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("browser service did not stop")
		}
	})
	client, _ := browser.NewClient(store, session.ID)
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		if _, err = client.Session(ctx, session.ID); err == nil {
			return root, store, session
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("browser did not start", err)
	return "", nil, browser.Session{}
}

func browserAction(t *testing.T, screen workapp.ControlScreen, name string) workapp.ControlAction {
	t.Helper()
	for _, a := range screen.Actions {
		if a.Action == name {
			return a
		}
	}
	t.Fatalf("missing %s in %#v", name, screen.Actions)
	return workapp.ControlAction{}
}

func TestBrowserControlsPrivateTakeoverGrantAndStaleReview(t *testing.T) {
	root, store, session := browserControlsFixture(t)
	ctx := context.Background()
	screen, err := browserControl(ctx, root, "status", []string{session.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range screen.Actions {
		if a.Action == "grant" {
			t.Fatal("paused browser offered grant")
		}
	}
	add := browserAction(t, screen, "add-origin")
	screen, err = browserControl(ctx, root, "add-origin", append(append([]string{}, add.Args...), "http://127.0.0.1:4500"))
	if err != nil {
		t.Fatal(err)
	}
	resume := browserAction(t, screen, "resume")
	if !resume.Review {
		t.Fatal("resume bypassed review")
	}
	screen, err = browserControl(ctx, root, "resume", resume.Args)
	if err != nil {
		t.Fatal(err)
	}
	grant := browserAction(t, screen, "grant")
	if !grant.Review || !strings.Contains(grant.Description, "127.0.0.1:4500") || !strings.Contains(grant.Description, "tab-one") {
		t.Fatal("grant omitted scope")
	}
	screen, err = browserControl(ctx, root, "grant", grant.Args)
	if err != nil {
		t.Fatal(err)
	}
	if screen.BrowserGrant == nil || screen.BrowserGrant.SessionID != session.ID || screen.BrowserGrant.Policy != grant.Args[1] {
		t.Fatal("grant not bound to review")
	}
	visual := browserAction(t, screen, "visual")
	if _, err = browserControl(ctx, root, "visual", visual.Args); err != nil {
		t.Fatal(err)
	}
	if _, err = browserControl(ctx, root, "grant", grant.Args); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal("stale grant accepted", err)
	}
	screen, err = browserControl(ctx, root, "status", []string{session.ID})
	if err != nil {
		t.Fatal(err)
	}
	pause := browserAction(t, screen, "pause")
	if _, err = browserControl(ctx, root, "pause", pause.Args); err != nil {
		t.Fatal(err)
	}
	current, err := store.Get(session.ID)
	if err != nil || !current.Paused {
		t.Fatal("takeover did not pause", err)
	}
	screen, err = browserControl(ctx, root, "clear-grant", []string{session.ID})
	if err != nil || screen.BrowserGrant == nil || screen.BrowserGrant.SessionID != "" {
		t.Fatal("grant not cleared", err)
	}
}

func TestBrowserControlsUnavailableRevocation(t *testing.T) {
	root := t.TempDir()
	store, err := browser.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Start(browser.StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = browser.CreateToken(store, session.ID); err != nil {
		t.Fatal(err)
	}
	screen, err := browserControl(context.Background(), root, "status", []string{session.ID})
	if err != nil {
		t.Fatal(err)
	}
	stop := browserAction(t, screen, "stop")
	for _, a := range screen.Actions {
		if a.Action == "grant" || a.Action == "resume" {
			t.Fatal("unavailable controller offered access")
		}
	}
	if _, err = browserControl(context.Background(), root, "stop", stop.Args); err != nil {
		t.Fatal(err)
	}
	current, err := store.Get(session.ID)
	if err != nil || current.State != browser.StateStopped {
		t.Fatal("session not revoked", err)
	}
	if _, err = browser.ReadToken(store, session.ID); err == nil {
		t.Fatal("revoked token retained")
	}
}
