package googlework

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	taskMetadataPrefix = "[GATOR-TASK v1]\n"
	taskMetadataSuffix = "\n[/GATOR-TASK]"
	maxTaskNotes       = 8192
)

var (
	taskClock = regexp.MustCompile(`^(?:[01]\d|2[0-3]):[0-5]\d$`)
	taskRRule = regexp.MustCompile(`^RRULE:FREQ=(?:DAILY|WEEKLY|MONTHLY|YEARLY);INTERVAL=[1-9]\d{0,2}$`)
)

// TaskMetadata is Gator-owned portable metadata for Google Tasks, whose
// native API has no priority, recurring-task, or task-reminder fields. It is
// bounded and stored alongside the user's free-form notes, never in a local
// outbox. Existing non-Gator notes pass through unchanged.
type TaskMetadata struct {
	Priority        string `json:"priority,omitempty"`
	RecurrenceRRule string `json:"recurrence_rrule,omitempty"`
	ReminderTime    string `json:"reminder_time,omitempty"`
	ReminderZone    string `json:"reminder_zone,omitempty"`
}

type TaskNotes struct {
	UserNotes string        `json:"user_notes"`
	Metadata  *TaskMetadata `json:"metadata,omitempty"`
	Managed   bool          `json:"managed"`
}

// EncodeTaskNotes adds one validated Gator metadata block after user notes.
// The result is suitable as the `notes` field of an approval-bound
// `tasks_create` or `tasks_update` payload.
func EncodeTaskNotes(userNotes string, metadata TaskMetadata) (string, error) {
	userNotes = strings.TrimSpace(userNotes)
	if len(userNotes) > maxTaskNotes {
		return "", errors.New("task notes exceed 8192 bytes")
	}
	if metadata.Priority == "" {
		metadata.Priority = "none"
	}
	if err := validateTaskMetadata(metadata); err != nil {
		return "", err
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode task metadata: %w", err)
	}
	result := taskMetadataPrefix + string(payload) + taskMetadataSuffix
	if userNotes != "" {
		result = userNotes + "\n\n" + result
	}
	if len(result) > maxTaskNotes {
		return "", errors.New("task notes plus metadata exceed 8192 bytes")
	}
	return result, nil
}

// DecodeTaskNotes separates a valid Gator metadata block from user notes. A
// malformed or duplicate block is returned as ordinary user notes so Gator
// never destroys a task's text while interpreting it.
func DecodeTaskNotes(notes string) TaskNotes {
	if len(notes) > maxTaskNotes {
		return TaskNotes{UserNotes: notes}
	}
	first := strings.Index(notes, taskMetadataPrefix)
	if first < 0 || strings.Count(notes, taskMetadataPrefix) != 1 {
		return TaskNotes{UserNotes: notes}
	}
	end := strings.Index(notes[first+len(taskMetadataPrefix):], taskMetadataSuffix)
	if end < 0 {
		return TaskNotes{UserNotes: notes}
	}
	end += first + len(taskMetadataPrefix)
	if strings.TrimSpace(notes[end+len(taskMetadataSuffix):]) != "" {
		return TaskNotes{UserNotes: notes}
	}
	var metadata TaskMetadata
	if err := json.Unmarshal([]byte(notes[first+len(taskMetadataPrefix):end]), &metadata); err != nil || validateTaskMetadata(metadata) != nil {
		return TaskNotes{UserNotes: notes}
	}
	return TaskNotes{UserNotes: strings.TrimSpace(notes[:first]), Metadata: &metadata, Managed: true}
}

func validateTaskMetadata(metadata TaskMetadata) error {
	if metadata.Priority == "" {
		metadata.Priority = "none"
	}
	if metadata.Priority != "none" && metadata.Priority != "low" && metadata.Priority != "medium" && metadata.Priority != "high" {
		return errors.New("task priority must be none, low, medium, or high")
	}
	if metadata.RecurrenceRRule != "" && !taskRRule.MatchString(metadata.RecurrenceRRule) {
		return errors.New("task recurrence must be a bounded daily, weekly, monthly, or yearly RRULE")
	}
	if metadata.ReminderTime != "" && !taskClock.MatchString(metadata.ReminderTime) {
		return errors.New("task reminder time must be HH:MM")
	}
	if metadata.ReminderTime == "" && metadata.ReminderZone != "" {
		return errors.New("task reminder timezone requires a reminder time")
	}
	if metadata.ReminderZone != "" {
		if len(metadata.ReminderZone) > 128 || strings.TrimSpace(metadata.ReminderZone) != metadata.ReminderZone || strings.ContainsRune(metadata.ReminderZone, 0) {
			return errors.New("task reminder timezone is invalid")
		}
		if _, err := time.LoadLocation(metadata.ReminderZone); err != nil {
			return errors.New("task reminder timezone is invalid")
		}
	}
	return nil
}
