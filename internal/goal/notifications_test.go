package goal

import (
	"testing"
	"time"
)

func TestGoalNotificationPolicyPersistsAndHandlesOvernightQuietHours(t *testing.T) {
	stateDir := t.TempDir()
	store, err := Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("Plan the offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	policy := NotificationPolicy{QuietStart: "22:00", QuietEnd: "08:00", Timezone: "Asia/Singapore"}
	if _, err := store.SetNotificationPolicy(record.ID, policy); err != nil {
		t.Fatal(err)
	}
	store, err = Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		utc     string
		allowed bool
	}{
		{"2026-10-17T13:59:00Z", true},  // 21:59 Singapore
		{"2026-10-17T14:00:00Z", false}, // 22:00 Singapore
		{"2026-10-17T23:59:00Z", false}, // 07:59 Singapore
		{"2026-10-18T00:00:00Z", true},  // 08:00 Singapore
	} {
		at, _ := time.Parse(time.RFC3339, check.utc)
		if got := loaded.Notifications.AllowsDesktop(at); got != check.allowed {
			t.Fatalf("desktop allowed at %s = %t", check.utc, got)
		}
	}
	policy.DesktopMuted = true
	if policy.AllowsDesktop(time.Date(2026, 10, 18, 4, 0, 0, 0, time.UTC)) {
		t.Fatal("muted goal allowed a desktop notification")
	}
	for _, invalid := range []NotificationPolicy{
		{QuietStart: "9:00", QuietEnd: "10:00", Timezone: "UTC"},
		{QuietStart: "08:00", QuietEnd: "08:00", Timezone: "UTC"},
		{QuietStart: "22:00", QuietEnd: "08:00", Timezone: "No/Such_Zone"},
	} {
		if _, err := store.SetNotificationPolicy(record.ID, invalid); err == nil {
			t.Fatalf("invalid notification policy accepted: %#v", invalid)
		}
	}
}
