package main

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/gongahkia/gator/internal/hostproc"
	"github.com/gongahkia/gator/internal/inbox"
)

type desktopSender func(context.Context, string, string) error

func sendJobNotification(ctx context.Context, stateDir, id string, now time.Time, send desktopSender) error {
	if send == nil {
		return errors.New("desktop sender unavailable")
	}
	store, err := inbox.Open(stateDir)
	if err != nil {
		return err
	}
	_, err = store.SendDesktopNotification(id, now, func(e inbox.Entry) error {
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return send(bounded, e.Title, e.Summary)
	})
	return err
}

// Retry only deliveries already admitted, never every old unread inbox entry.
// Recheck quiet hours and mute policy on every pass. The delivery store also
// rechecks read/resolved state and limits attempts across supervisor restarts.
func retryJobNotifications(ctx context.Context, stateDir string, now time.Time, send desktopSender) {
	store, err := inbox.Open(stateDir)
	if err != nil {
		return
	}
	entries, err := store.List(true, 0)
	if err != nil {
		return
	}
	attempts := 0
	for _, entry := range entries {
		if ctx.Err() != nil || attempts == 4 {
			return
		}
		if !entry.DesktopRetryDue(now) || !goalAllowsDesktopNotification(stateDir, entry.GoalID, now, entry.Urgent) {
			continue
		}
		attempts++
		_ = sendJobNotification(ctx, stateDir, entry.ID, now, send)
	}
}

func desktopNotificationCommand(platform, title, body string) (string, []string, error) {
	switch platform {
	case "darwin":
		// Source is constant. Untrusted model/source text is passed as argv so it
		// cannot end a quoted literal and execute AppleScript commands.
		return "osascript", []string{"-e", "on run argv\ndisplay notification (item 1 of argv) with title (item 2 of argv)\nend run", "--", body, "Gator: " + title}, nil
	case "linux":
		return "notify-send", []string{"--", "Gator: " + title, body}, nil
	default:
		return "", nil, errors.New("desktop notifications are unavailable on this platform")
	}
}

func desktopNotification(ctx context.Context, title, body string) error {
	name, args, err := desktopNotificationCommand(runtime.GOOS, title, body)
	if err != nil {
		return err
	}
	command := hostproc.CommandContext(ctx, name, args...)
	command.WaitDelay = time.Second
	return command.Run()
}
