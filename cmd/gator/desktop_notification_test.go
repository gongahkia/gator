package main

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/inbox"
)

func TestDesktopNotificationTextCannotBecomeAppleScript(t *testing.T) {
	payload := `\"); do shell script "touch SHOULD_NEVER_RUN" --`
	name, args, err := desktopNotificationCommand("darwin", payload, payload)
	if err != nil || name != "osascript" || len(args) != 5 || strings.Contains(args[1], payload) || args[2] != "--" || args[3] != payload || args[4] != "Gator: "+payload {
		t.Fatal("notification text was interpolated into program", args, err)
	}
	if runtime.GOOS == "darwin" {
		// Exercise native argument parsing without displaying a notification or
		// evaluating the payload as program text.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "osascript", "-e", "on run argv\nreturn item 1 of argv\nend run", "--", payload).Output()
		if err != nil || strings.TrimSuffix(string(out), "\n") != payload {
			t.Fatal("native argv isolation failed", err)
		}
	}
	_, args, err = desktopNotificationCommand("linux", "--help", "--icon=bad")
	if err != nil || args[0] != "--" {
		t.Fatal("notification text can become command options")
	}
}

func TestDesktopNotificationRetryUsesOnlyAdmittedUnreadUpdates(t *testing.T) {
	state := t.TempDir()
	s, _ := inbox.Open(state)
	add := func(title string) inbox.Entry {
		e, err := s.Add(inbox.Entry{Kind: "job", Title: title, Status: "completed"})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	e := add("Queued")
	_ = add("Historical unread")
	now := time.Now()
	if err := sendJobNotification(context.Background(), state, e.ID, now, func(context.Context, string, string) error { return errors.New("unavailable") }); err == nil {
		t.Fatal("failed handoff accepted")
	}
	calls := 0
	send := func(_ context.Context, title, body string) error {
		calls++
		if title != "Queued" {
			t.Fatal("historical update was newly admitted")
		}
		return nil
	}
	retryJobNotifications(context.Background(), state, now.Add(time.Second), send)
	if calls != 0 {
		t.Fatal("retry backoff bypassed")
	}
	retryJobNotifications(context.Background(), state, now.Add(time.Minute), send)
	retryJobNotifications(context.Background(), state, now.Add(time.Hour), send)
	if calls != 1 {
		t.Fatal("retry did not deduplicate successful OS handoff", calls)
	}
}
