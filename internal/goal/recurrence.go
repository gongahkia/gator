package goal

import (
	"errors"
	"sort"
	"time"
	// Packaged Windows runtimes and minimal hosts may have no IANA zone files.
	_ "time/tzdata"
)

// Recurrence describes local-wall-clock blocks. Expansion is read-only; it
// never creates task records, notifications or inferred completion.
type Recurrence struct {
	Frequency string    `json:"frequency"`
	Interval  int       `json:"interval"`
	Timezone  string    `json:"timezone"`
	Until     time.Time `json:"until,omitempty"`
}
type OccurrenceOverride struct {
	Date     string    `json:"date"`
	Status   string    `json:"status"`
	StartsAt time.Time `json:"starts_at,omitempty"`
	EndsAt   time.Time `json:"ends_at,omitempty"`
}
type BlockOccurrence struct {
	Date string `json:"date"`
	Step Step   `json:"step"`
}

func validateRecurrence(step Step) error {
	r := step.Recurrence
	if r == nil {
		if len(step.Occurrences) > 0 {
			return errors.New("occurrence overrides require a recurring block")
		}
		return nil
	}
	if r.Frequency != "daily" && r.Frequency != "weekly" || r.Interval < 1 || r.Interval > 52 {
		return errors.New("recurrence requires daily or weekly and interval 1 through 52")
	}
	if r.Timezone == "" || r.Timezone == "Local" {
		return errors.New("recurrence requires an explicit IANA timezone")
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return errors.New("invalid recurrence timezone")
	}
	if step.StartsAt.IsZero() || !step.EndsAt.After(step.StartsAt) || step.EndsAt.Sub(step.StartsAt) > 24*time.Hour || !step.DueAt.IsZero() {
		return errors.New("recurring blocks require start/end within 24 hours and no separate deadline")
	}
	if !r.Until.IsZero() && r.Until.Before(step.StartsAt) {
		return errors.New("recurrence ends before its first block")
	}
	if len(step.Occurrences) > 128 {
		return errors.New("recurring block supports at most 128 occurrence overrides")
	}
	seen := map[string]bool{}
	for _, o := range step.Occurrences {
		if seen[o.Date] {
			return errors.New("duplicate occurrence date")
		}
		seen[o.Date] = true
		if _, ok := nominalOccurrence(step, o.Date); !ok {
			return errors.New("override date does not belong to the recurrence")
		}
		if o.Status != "skipped" && !validStepStatus(StepStatus(o.Status)) {
			return errors.New("invalid occurrence status")
		}
		if err := validateSchedule(o.StartsAt, o.EndsAt, nil); err != nil {
			return err
		}
		if !o.StartsAt.IsZero() && o.EndsAt.Sub(o.StartsAt) > 24*time.Hour {
			return errors.New("rescheduled occurrence exceeds 24 hours")
		}
	}
	return nil
}

func nominalOccurrence(step Step, date string) (time.Time, bool) {
	r := step.Recurrence
	if r == nil {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		return time.Time{}, false
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}, false
	}
	anchor := step.StartsAt.In(loc)
	anchorDay := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)
	days := int((day.Unix() - anchorDay.Unix()) / 86400)
	interval := r.Interval
	if r.Frequency == "weekly" {
		interval *= 7
	}
	if interval <= 0 || days < 0 || days%interval != 0 {
		return time.Time{}, false
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), loc)
	if date == anchor.Format("2006-01-02") {
		start = step.StartsAt.In(loc)
	}
	// A nonexistent spring-forward wall time is skipped, never silently shifted.
	if start.Format("2006-01-02") != date || start.Hour() != anchor.Hour() || start.Minute() != anchor.Minute() {
		return time.Time{}, false
	}
	if start.Before(step.StartsAt) || !r.Until.IsZero() && start.After(r.Until) {
		return time.Time{}, false
	}
	return start, true
}

// ExpandBlock returns only occurrences intersecting [from, until). Past
// occurrences remain derivable and keep explicit overrides, never auto-complete.
func ExpandBlock(step Step, from, until time.Time) ([]BlockOccurrence, error) {
	if err := validateRecurrence(step); err != nil {
		return nil, err
	}
	if step.Recurrence == nil {
		return nil, errors.New("step is not recurring")
	}
	if !until.After(from) || until.Sub(from) > 32*24*time.Hour {
		return nil, errors.New("recurrence expansion requires a window up to 32 days")
	}
	loc, _ := time.LoadLocation(step.Recurrence.Timezone)
	first := from.In(loc).AddDate(0, 0, -1)
	day := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.UTC)
	dates := map[string]bool{}
	for i := 0; i < 36; i++ {
		dates[day.AddDate(0, 0, i).Format("2006-01-02")] = true
	}
	overrides := map[string]OccurrenceOverride{}
	for _, o := range step.Occurrences {
		overrides[o.Date] = o
		dates[o.Date] = true
	}
	result := []BlockOccurrence{}
	for date := range dates {
		start, ok := nominalOccurrence(step, date)
		if !ok {
			continue
		}
		end := start.Add(step.EndsAt.Sub(step.StartsAt))
		status := step.Status
		if o, exists := overrides[date]; exists {
			if o.Status == "skipped" {
				continue
			}
			status = StepStatus(o.Status)
			if !o.StartsAt.IsZero() {
				start, end = o.StartsAt, o.EndsAt
			}
		}
		if !end.After(from) || !start.Before(until) {
			continue
		}
		copy := step
		copy.StartsAt, copy.EndsAt, copy.Status = start, end, status
		copy.Recurrence, copy.Occurrences = nil, nil
		result = append(result, BlockOccurrence{Date: date, Step: copy})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Step.StartsAt.Before(result[j].Step.StartsAt) })
	return result, nil
}
