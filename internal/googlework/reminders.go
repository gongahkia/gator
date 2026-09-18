package googlework

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Reminder is a due notification derived from live-mirrored Google Tasks or
// Calendar data. Key is stable across process restarts for delivery de-dupe.
type Reminder struct {
	Key      string    `json:"key"`
	Kind     string    `json:"kind"`
	SourceID string    `json:"source_id"`
	Title    string    `json:"title"`
	At       time.Time `json:"at"`
}

// DueReminders evaluates explicit Calendar overrides and portable task
// reminder metadata inside the requested window. Calendar defaults are not
// guessed because they are account/calendar policy rather than event data.
func DueReminders(tasks, events []Record, start, end time.Time, fallback *time.Location) []Reminder {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil
	}
	if fallback == nil {
		fallback = time.Local
	}
	result := make([]Reminder, 0)
	for _, task := range tasks {
		var item struct {
			Title string `json:"title"`
			Due   string `json:"due"`
			Notes string `json:"notes"`
		}
		if json.Unmarshal(task.Raw, &item) != nil || strings.TrimSpace(item.Due) == "" {
			continue
		}
		metadata := DecodeTaskNotes(item.Notes)
		if !metadata.Managed || metadata.Metadata == nil || metadata.Metadata.ReminderTime == "" {
			continue
		}
		location := fallback
		if metadata.Metadata.ReminderZone != "" {
			loaded, err := time.LoadLocation(metadata.Metadata.ReminderZone)
			if err != nil {
				continue
			}
			location = loaded
		}
		date, err := time.Parse("2006-01-02", item.Due[:min(len(item.Due), len("2006-01-02"))])
		hour, minute, clockErr := parseClock(metadata.Metadata.ReminderTime)
		if err != nil || clockErr != nil {
			continue
		}
		at := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location)
		if inReminderWindow(at, start, end) {
			result = append(result, Reminder{Key: fmt.Sprintf("task:%s:%d", task.ID, at.Unix()), Kind: "task", SourceID: task.ID, Title: item.Title, At: at.UTC()})
		}
	}
	for _, event := range events {
		var item struct {
			Summary string `json:"summary"`
			Start   struct {
				DateTime string `json:"dateTime"`
			} `json:"start"`
			Reminders struct {
				Overrides []struct {
					Method  string `json:"method"`
					Minutes int    `json:"minutes"`
				} `json:"overrides"`
			} `json:"reminders"`
		}
		if json.Unmarshal(event.Raw, &item) != nil || item.Start.DateTime == "" {
			continue
		}
		startsAt, err := time.Parse(time.RFC3339, item.Start.DateTime)
		if err != nil {
			continue
		}
		seen := map[int]struct{}{}
		for _, override := range item.Reminders.Overrides {
			if override.Minutes < 0 || override.Minutes > 40320 || (override.Method != "popup" && override.Method != "email") {
				continue
			}
			if _, duplicate := seen[override.Minutes]; duplicate {
				continue
			}
			seen[override.Minutes] = struct{}{}
			at := startsAt.Add(-time.Duration(override.Minutes) * time.Minute)
			if inReminderWindow(at, start, end) {
				result = append(result, Reminder{Key: fmt.Sprintf("event:%s:%d", event.ID, at.Unix()), Kind: "event", SourceID: event.ID, Title: item.Summary, At: at.UTC()})
			}
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].At.Equal(result[right].At) {
			return result[left].Key < result[right].Key
		}
		return result[left].At.Before(result[right].At)
	})
	return result
}

func inReminderWindow(value, start, end time.Time) bool {
	return value.After(start) && (value.Before(end) || value.Equal(end))
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
