package googlework

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CaptureKind selects the Google object a quick capture will eventually
// create. Parsing itself has no side effect and can be reviewed before an
// approval-bound task or event mutation is proposed.
type CaptureKind string

const (
	CaptureTask  CaptureKind = "task"
	CaptureEvent CaptureKind = "event"
)

type Recognition struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type Recurrence struct {
	Frequency string `json:"frequency"`
	Interval  int    `json:"interval"`
	RRule     string `json:"rrule"`
}

// Capture is a deterministic quick-capture preview adapted from Hot Cross
// Buns' parser. It intentionally does not choose a task list or calendar;
// the Work manager must ask/select an explicit Google destination.
type Capture struct {
	Kind                 CaptureKind   `json:"kind"`
	RawTitle             string        `json:"raw_title"`
	ParsedTitle          string        `json:"parsed_title"`
	Date                 string        `json:"date,omitempty"`
	Time                 string        `json:"time,omitempty"`
	AllDay               bool          `json:"all_day"`
	EventReady           bool          `json:"event_ready"`
	EventDurationMinutes int           `json:"event_duration_minutes"`
	TaskPriority         string        `json:"task_priority"`
	Recurrence           *Recurrence   `json:"recurrence,omitempty"`
	Recognitions         []Recognition `json:"recognitions,omitempty"`
}

var (
	quickISODate    = regexp.MustCompile(`(?i)\b(\d{4})-(\d{2})-(\d{2})\b`)
	quickToday      = regexp.MustCompile(`(?i)\btoday\b`)
	quickTomorrow   = regexp.MustCompile(`(?i)\btomorrow\b`)
	quickNextDay    = regexp.MustCompile(`(?i)\bnext\s+(monday|tuesday|wednesday|thursday|friday|saturday|sunday)\b`)
	quickRelative   = regexp.MustCompile(`(?i)\bin\s+(\d{1,3})\s+(days?|weeks?)\b`)
	quickNamedDate  = regexp.MustCompile(`(?i)\b(january|february|march|april|may|june|july|august|september|october|november|december)\s+(\d{1,2})(?:st|nd|rd|th)?(?:,?\s+(\d{4}))?\b`)
	quick12Hour     = regexp.MustCompile(`(?i)\b(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)\b`)
	quick24Hour     = regexp.MustCompile(`(?i)\b(?:at\s+)?([01]?\d|2[0-3]):([0-5]\d)\b`)
	quickRecurrence = regexp.MustCompile(`(?i)\bevery(?:\s+(\d{1,3}))?\s+(day|week|month|year)s?\b`)
	quickDuration   = regexp.MustCompile(`(?i)\bfor\s+(\d{1,4})\s*(m|min|mins|minute|minutes|h|hr|hrs|hour|hours)\b`)
)

var quickMonths = map[string]time.Month{
	"january": time.January, "february": time.February, "march": time.March, "april": time.April,
	"may": time.May, "june": time.June, "july": time.July, "august": time.August,
	"september": time.September, "october": time.October, "november": time.November, "december": time.December,
}

var quickWeekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
	"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday,
}

// ParseQuickCapture recognizes the same practical phrases as HCB's quick
// capture: dates, times, durations, simple recurrence, task/event aliases,
// and priority. Its results are timezone-local wall-clock values.
func ParseQuickCapture(text string, requested CaptureKind, now time.Time) (Capture, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 4096 {
		return Capture{}, errors.New("quick capture text must be 1 through 4096 characters")
	}
	if requested != CaptureTask && requested != CaptureEvent {
		return Capture{}, errors.New("quick capture kind must be task or event")
	}
	if now.IsZero() {
		now = time.Now()
	}
	result := Capture{Kind: requested, RawTitle: text, EventDurationMinutes: 30, TaskPriority: "none"}
	spans := make([]captureSpan, 0, 8)
	if kind, span, found := captureKind(text); found {
		result.Kind = kind
		spans = append(spans, span)
		result.Recognitions = append(result.Recognitions, Recognition{Kind: "type", Label: strings.Title(string(kind))})
	}
	if result.Kind == CaptureTask {
		if priority, span, found := capturePriority(text); found {
			result.TaskPriority = priority
			spans = append(spans, span)
			result.Recognitions = append(result.Recognitions, Recognition{Kind: "priority", Label: strings.Title(priority) + " priority"})
		}
	}
	if frequency, interval, span, found := captureRecurrence(text); found {
		result.Recurrence = &Recurrence{Frequency: frequency, Interval: interval, RRule: fmt.Sprintf("RRULE:FREQ=%s;INTERVAL=%d", strings.ToUpper(frequency), interval)}
		spans = append(spans, span)
		label := "Repeats every " + frequency
		if interval != 1 {
			label = fmt.Sprintf("Repeats every %d %ss", interval, frequency)
		}
		result.Recognitions = append(result.Recognitions, Recognition{Kind: "recurrence", Label: label})
	}
	if result.Kind == CaptureEvent {
		if duration, span, found := captureDuration(text); found {
			result.EventDurationMinutes = duration
			spans = append(spans, span)
			result.Recognitions = append(result.Recognitions, Recognition{Kind: "duration", Label: fmt.Sprintf("%d minutes", duration)})
		}
	}
	if date, span, found := captureDate(text, now); found {
		result.Date = date
		spans = append(spans, span)
		result.Recognitions = append(result.Recognitions, Recognition{Kind: "date", Label: date})
	}
	if clock, span, found := captureTime(text); found {
		result.Time = clock
		if result.Kind == CaptureEvent {
			spans = append(spans, span)
			result.Recognitions = append(result.Recognitions, Recognition{Kind: "time", Label: clock})
		} else {
			result.Recognitions = append(result.Recognitions, Recognition{Kind: "time", Label: clock + " remains in task title"})
		}
	}
	if result.Kind == CaptureTask && result.Recurrence != nil && result.Date == "" {
		result.Date = localDate(now)
	}
	if result.Kind == CaptureEvent && result.Time != "" && result.Date == "" {
		hour, minute, _ := parseClock(result.Time)
		candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		if !candidate.After(now) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		result.Date = localDate(candidate)
	}
	result.AllDay = result.Kind == CaptureEvent && result.Date != "" && result.Time == ""
	result.EventReady = result.Kind == CaptureTask || result.Date != ""
	result.ParsedTitle = removeCaptureSpans(text, spans)
	if result.ParsedTitle == "" {
		return Capture{}, errors.New("quick capture needs a title outside recognized scheduling text")
	}
	return result, nil
}

type captureSpan struct{ start, end int }

func spanFor(match []int) captureSpan { return captureSpan{start: match[0], end: match[1]} }

func captureKind(text string) (CaptureKind, captureSpan, bool) {
	var selected struct {
		kind CaptureKind
		span captureSpan
		ok   bool
	}
	for _, candidate := range []struct {
		kind CaptureKind
		re   *regexp.Regexp
	}{{CaptureEvent, regexp.MustCompile(`(?i)\b(?:event|meeting)\b`)}, {CaptureTask, regexp.MustCompile(`(?i)\b(?:task|todo)\b`)}} {
		if match := candidate.re.FindStringIndex(text); match != nil {
			if !selected.ok || match[0] < selected.span.start {
				selected = struct {
					kind CaptureKind
					span captureSpan
					ok   bool
				}{kind: candidate.kind, span: spanFor(match), ok: true}
			}
		}
	}
	return selected.kind, selected.span, selected.ok
}

func capturePriority(text string) (string, captureSpan, bool) {
	for _, candidate := range []struct{ priority, expression string }{{"high", `(?i)\b(?:high priority|urgent|!high)\b`}, {"medium", `(?i)\b(?:medium priority|!medium)\b`}, {"low", `(?i)\b(?:low priority|!low)\b`}} {
		re := regexp.MustCompile(candidate.expression)
		if match := re.FindStringIndex(text); match != nil {
			return candidate.priority, spanFor(match), true
		}
	}
	return "", captureSpan{}, false
}

func captureRecurrence(text string) (string, int, captureSpan, bool) {
	match := quickRecurrence.FindStringSubmatchIndex(text)
	if match == nil {
		return "", 0, captureSpan{}, false
	}
	interval := 1
	if match[2] >= 0 {
		interval, _ = strconv.Atoi(text[match[2]:match[3]])
	}
	if interval < 1 || interval > 1000 {
		return "", 0, captureSpan{}, false
	}
	unit := strings.ToLower(text[match[4]:match[5]])
	return map[string]string{"day": "daily", "week": "weekly", "month": "monthly", "year": "yearly"}[unit], interval, spanFor(match[:2]), true
}

func captureDuration(text string) (int, captureSpan, bool) {
	match := quickDuration.FindStringSubmatchIndex(text)
	if match == nil {
		return 0, captureSpan{}, false
	}
	amount, _ := strconv.Atoi(text[match[2]:match[3]])
	unit := strings.ToLower(text[match[4]:match[5]])
	if strings.HasPrefix(unit, "h") {
		amount *= 60
	}
	if amount < 1 || amount > 1440 {
		return 0, captureSpan{}, false
	}
	return amount, spanFor(match[:2]), true
}

func captureDate(text string, now time.Time) (string, captureSpan, bool) {
	if match := quickISODate.FindStringSubmatchIndex(text); match != nil {
		year, _ := strconv.Atoi(text[match[2]:match[3]])
		month, _ := strconv.Atoi(text[match[4]:match[5]])
		day, _ := strconv.Atoi(text[match[6]:match[7]])
		candidate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
		if candidate.Year() == year && int(candidate.Month()) == month && candidate.Day() == day {
			return localDate(candidate), spanFor(match[:2]), true
		}
	}
	if match := quickToday.FindStringIndex(text); match != nil {
		return localDate(now), spanFor(match), true
	}
	if match := quickTomorrow.FindStringIndex(text); match != nil {
		return localDate(now.AddDate(0, 0, 1)), spanFor(match), true
	}
	if match := quickNextDay.FindStringSubmatchIndex(text); match != nil {
		target := quickWeekdays[strings.ToLower(text[match[2]:match[3]])]
		days := (int(target) - int(now.Weekday()) + 7) % 7
		if days == 0 {
			days = 7
		}
		return localDate(now.AddDate(0, 0, days)), spanFor(match[:2]), true
	}
	if match := quickRelative.FindStringSubmatchIndex(text); match != nil {
		amount, _ := strconv.Atoi(text[match[2]:match[3]])
		if strings.HasPrefix(strings.ToLower(text[match[4]:match[5]]), "week") {
			amount *= 7
		}
		return localDate(now.AddDate(0, 0, amount)), spanFor(match[:2]), true
	}
	if match := quickNamedDate.FindStringSubmatchIndex(text); match != nil {
		month := quickMonths[strings.ToLower(text[match[2]:match[3]])]
		day, _ := strconv.Atoi(text[match[4]:match[5]])
		year := now.Year()
		if match[6] >= 0 {
			year, _ = strconv.Atoi(text[match[6]:match[7]])
		}
		candidate := time.Date(year, month, day, 0, 0, 0, 0, now.Location())
		if candidate.Month() != month || candidate.Day() != day {
			return "", captureSpan{}, false
		}
		if match[6] < 0 && candidate.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())) {
			candidate = candidate.AddDate(1, 0, 0)
		}
		return localDate(candidate), spanFor(match[:2]), true
	}
	return "", captureSpan{}, false
}

func captureTime(text string) (string, captureSpan, bool) {
	if match := quick12Hour.FindStringSubmatchIndex(text); match != nil {
		hour, _ := strconv.Atoi(text[match[2]:match[3]])
		minute := 0
		if match[4] >= 0 {
			minute, _ = strconv.Atoi(text[match[4]:match[5]])
		}
		if hour >= 1 && hour <= 12 && minute <= 59 {
			hour %= 12
			if strings.EqualFold(text[match[6]:match[7]], "pm") {
				hour += 12
			}
			return fmt.Sprintf("%02d:%02d", hour, minute), spanFor(match[:2]), true
		}
	}
	if match := quick24Hour.FindStringSubmatchIndex(text); match != nil {
		hour, _ := strconv.Atoi(text[match[2]:match[3]])
		minute, _ := strconv.Atoi(text[match[4]:match[5]])
		return fmt.Sprintf("%02d:%02d", hour, minute), spanFor(match[:2]), true
	}
	return "", captureSpan{}, false
}

func parseClock(value string) (int, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid clock")
	}
	hour, first := strconv.Atoi(parts[0])
	minute, second := strconv.Atoi(parts[1])
	if first != nil || second != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, errors.New("invalid clock")
	}
	return hour, minute, nil
}

func localDate(value time.Time) string { return value.Format("2006-01-02") }

func removeCaptureSpans(text string, spans []captureSpan) string {
	if len(spans) == 0 {
		return strings.TrimSpace(text)
	}
	sort.Slice(spans, func(left, right int) bool { return spans[left].start > spans[right].start })
	for _, span := range spans {
		if span.start >= 0 && span.end <= len(text) && span.start < span.end {
			text = text[:span.start] + text[span.end:]
		}
	}
	return strings.Join(strings.Fields(text), " ")
}
