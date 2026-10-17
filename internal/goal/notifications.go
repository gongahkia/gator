package goal

import (
	"errors"
	"time"
)

// NotificationPolicy narrows desktop interruptions for one goal. Inbox entries
// are retained regardless of this policy.
type NotificationPolicy struct {
	DesktopMuted bool   `json:"desktop_muted,omitempty"`
	QuietStart   string `json:"quiet_start,omitempty"`
	QuietEnd     string `json:"quiet_end,omitempty"`
	Timezone     string `json:"timezone,omitempty"`
}

func (p NotificationPolicy) Validate() error {
	if p.QuietStart == "" && p.QuietEnd == "" && p.Timezone == "" {
		return nil
	}
	if !validQuietClock(p.QuietStart) || !validQuietClock(p.QuietEnd) || p.QuietStart == p.QuietEnd || p.Timezone == "" || len(p.Timezone) > 128 {
		return errors.New("quiet hours require distinct HH:MM start/end and an IANA timezone")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return errors.New("quiet hours timezone is invalid")
	}
	return nil
}

func (p NotificationPolicy) AllowsDesktop(now time.Time) bool {
	if p.DesktopMuted {
		return false
	}
	if p.QuietStart == "" {
		return true
	}
	location, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return false
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	start, _ := quietMinute(p.QuietStart)
	end, _ := quietMinute(p.QuietEnd)
	if start < end {
		return minute < start || minute >= end
	}
	return minute < start && minute >= end
}

func validQuietClock(value string) bool {
	_, ok := quietMinute(value)
	return ok
}

func quietMinute(value string) (int, bool) {
	if len(value) != 5 || value[2] != ':' {
		return 0, false
	}
	for _, index := range []int{0, 1, 3, 4} {
		if value[index] < '0' || value[index] > '9' {
			return 0, false
		}
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[3]-'0')*10 + int(value[4]-'0')
	if hour > 23 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}
