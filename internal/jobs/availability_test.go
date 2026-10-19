package jobs

import (
	"testing"
	"time"
)

func TestAvailabilityRetainsUnobservedIntervalsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	now := time.Now().UTC()
	for _, offset := range []time.Duration{0, 15 * time.Second, 30 * time.Second, 5 * time.Minute} {
		if err := s.Observe(now.Add(offset), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Observe(now.Add(5*time.Minute+time.Second), true); err != nil {
		t.Fatal(err)
	}
	reopened, _ := Open(root)
	a, err := reopened.Availability()
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Gaps) != 1 || !a.Gaps[0].From.Equal(now.Add(30*time.Second)) || !a.Gaps[0].To.Equal(now.Add(5*time.Minute)) || a.StoppedAt.IsZero() {
		t.Fatalf("incorrect coverage history: %#v", a)
	}
	if err := reopened.Observe(now.Add(time.Hour), false); err != nil {
		t.Fatal(err)
	}
	a, _ = reopened.Availability()
	if len(a.Gaps) != 2 || !a.StoppedAt.IsZero() {
		t.Fatalf("restart lost gap: %#v", a)
	}
}
