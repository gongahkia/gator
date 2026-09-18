package googlework

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDueRemindersIncludesExplicitCalendarAndPortableTaskReminder(t *testing.T) {
	notes, err := EncodeTaskNotes("", TaskMetadata{ReminderTime: "09:00", ReminderZone: "Asia/Singapore"})
	if err != nil {
		t.Fatal(err)
	}
	tasks := []Record{{ID: "task-1", Raw: json.RawMessage(`{"title":"Submit report","due":"2026-09-19T00:00:00.000Z","notes":` + mustJSON(t, notes) + `}`)}}
	events := []Record{{ID: "event-1", Raw: json.RawMessage(`{"summary":"Planning","start":{"dateTime":"2026-09-19T02:30:00Z"},"reminders":{"overrides":[{"method":"popup","minutes":30}]}}`)}}
	start := time.Date(2026, 9, 19, 0, 29, 0, 0, time.UTC)
	end := time.Date(2026, 9, 19, 2, 1, 0, 0, time.UTC)
	due := DueReminders(tasks, events, start, end, time.UTC)
	if len(due) != 2 || due[0].Kind != "task" || due[1].Kind != "event" {
		t.Fatalf("due reminders = %#v", due)
	}
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
