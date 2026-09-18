package googlework

import (
	"testing"
	"time"
)

func TestParseQuickCaptureBuildsReviewableEventPreview(t *testing.T) {
	now := time.Date(2026, 9, 18, 9, 0, 0, 0, time.FixedZone("SGT", 8*60*60))
	capture, err := ParseQuickCapture("meeting Roadmap review tomorrow at 2:30pm for 45 minutes every week", CaptureTask, now)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Kind != CaptureEvent || capture.ParsedTitle != "Roadmap review" || capture.Date != "2026-09-19" || capture.Time != "14:30" || capture.EventDurationMinutes != 45 || capture.Recurrence == nil || capture.Recurrence.RRule != "RRULE:FREQ=WEEKLY;INTERVAL=1" || !capture.EventReady {
		t.Fatalf("capture = %#v", capture)
	}
}

func TestParseQuickCaptureKeepsTaskTimeInTitleAndAnchorsRecurrence(t *testing.T) {
	now := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	capture, err := ParseQuickCapture("todo Submit report at 17:00 high priority every 2 days", CaptureEvent, now)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Kind != CaptureTask || capture.ParsedTitle != "Submit report at 17:00" || capture.Date != "2026-09-18" || capture.TaskPriority != "high" || capture.Recurrence == nil || capture.Recurrence.Interval != 2 {
		t.Fatalf("capture = %#v", capture)
	}
}
