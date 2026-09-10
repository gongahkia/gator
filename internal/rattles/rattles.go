// Package rattles provides the small, shared terminal activity treatment used
// by Gator's interactive surfaces.
//
// The frame sequence and interval are ported from vyfor/rattles' Braille Dots
// preset. The upstream crate is Rust and intentionally rendering-agnostic, so
// keeping this tiny Go representation avoids introducing a separate runtime.
// See THIRD_PARTY_NOTICES.md for attribution and license terms.
package rattles

import "time"

// Preset describes a terminal animation without assuming a UI framework.
type Preset struct {
	Frames   []string
	Interval time.Duration
}

// Frame returns the frame at index, wrapping safely for a long-running task.
func (p Preset) Frame(index int) string {
	if len(p.Frames) == 0 {
		return ""
	}
	if index < 0 {
		index = -index
	}
	return p.Frames[index%len(p.Frames)]
}

// FrameAt returns the frame that should be shown at a wall-clock instant.
func (p Preset) FrameAt(at time.Time) string {
	if p.Interval <= 0 {
		return p.Frame(0)
	}
	return p.Frame(int(at.UnixNano() / p.Interval.Nanoseconds()))
}

// BrailleDots is Rattles' 80 ms Braille Dots preset. It is deliberately a
// single-cell animation so status columns and narrow terminals do not shift.
var BrailleDots = Preset{
	Frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	Interval: 80 * time.Millisecond,
}
