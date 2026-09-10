package rattles

import (
	"testing"
	"time"
)

func TestBrailleDotsCyclesAndUsesItsPublishedInterval(t *testing.T) {
	if BrailleDots.Interval != 80*time.Millisecond {
		t.Fatalf("interval = %s, want 80ms", BrailleDots.Interval)
	}
	if got := BrailleDots.Frame(0); got != "⠋" {
		t.Fatalf("first frame = %q", got)
	}
	if got := BrailleDots.Frame(len(BrailleDots.Frames)); got != "⠋" {
		t.Fatalf("wrapped frame = %q", got)
	}
	if got := BrailleDots.FrameAt(time.Unix(0, int64(2*BrailleDots.Interval))); got != "⠹" {
		t.Fatalf("timed frame = %q", got)
	}
}
