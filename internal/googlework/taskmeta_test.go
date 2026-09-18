package googlework

import "testing"

func TestTaskMetadataRoundTripsWithoutTakingOverUserNotes(t *testing.T) {
	notes, err := EncodeTaskNotes("Discuss launch details", TaskMetadata{Priority: "high", RecurrenceRRule: "RRULE:FREQ=WEEKLY;INTERVAL=2", ReminderTime: "09:30", ReminderZone: "Asia/Singapore"})
	if err != nil {
		t.Fatal(err)
	}
	decoded := DecodeTaskNotes(notes)
	if !decoded.Managed || decoded.UserNotes != "Discuss launch details" || decoded.Metadata == nil || decoded.Metadata.Priority != "high" || decoded.Metadata.ReminderZone != "Asia/Singapore" {
		t.Fatalf("decoded task notes = %#v", decoded)
	}
	if decoded := DecodeTaskNotes("ordinary notes\n[GATOR-TASK v1]\nnot-json"); decoded.Managed || decoded.UserNotes == "" {
		t.Fatalf("malformed notes were not preserved: %#v", decoded)
	}
}

func TestTaskMetadataRejectsInvalidPortableValues(t *testing.T) {
	for _, metadata := range []TaskMetadata{
		{Priority: "critical"},
		{RecurrenceRRule: "RRULE:FREQ=HOURLY;INTERVAL=1"},
		{ReminderTime: "24:00"},
		{ReminderZone: "Asia/Singapore"},
	} {
		if _, err := EncodeTaskNotes("notes", metadata); err == nil {
			t.Fatalf("invalid metadata was accepted: %#v", metadata)
		}
	}
}
