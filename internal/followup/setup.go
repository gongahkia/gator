// Package followup defines the user's reviewed agreement for an ongoing check.
// Creating a draft has no side effects; only the application adapter saves it.
package followup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/jobs"
)

type Draft struct {
	EndsAt                                   string
	OriginConversationID                     string
	Enabled                                  bool
	ID, GoalID, JobID, ExpectedDigest        string
	Title, Outcome, SourcePath, AlertWhen    string
	ConnectorIDs                             []string
	GmailQuery                               string
	CalendarDays                             int
	IntervalMinutes                          int
	Mode                                     action.Mode
	MaxRequests, MaxSeconds, MaxChecksPerDay int
	MaxTokens                                int64
}

func New(source, goalID string) (Draft, error) {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Draft{}, err
	}
	return Draft{ID: hex.EncodeToString(id[:]), GoalID: goalID, Title: "Project follow-up", SourcePath: source,
		Outcome:   "Watch project changes, identify the next useful step, and draft a response when useful.",
		AlertWhen: "A deadline, commitment, decision, or blocker changes; stay quiet for unrelated updates.",
		Enabled:   true, IntervalMinutes: 15, Mode: action.Draft, MaxRequests: 32, MaxSeconds: 300, MaxTokens: 100000, MaxChecksPerDay: 24}, nil
}

func (d Draft) Validate() error {
	if _, err := d.EndTime(); err != nil {
		return err
	}
	if d.OriginConversationID != "" && !regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`).MatchString(d.OriginConversationID) {
		return errors.New("invalid originating conversation")
	}
	if len(d.ID) != 24 {
		return errors.New("invalid follow-up draft identity")
	}
	if _, err := hex.DecodeString(d.ID); err != nil {
		return errors.New("invalid follow-up draft identity")
	}
	if strings.TrimSpace(d.Title) == "" || len(d.Title) > 100 || strings.TrimSpace(d.Outcome) == "" || len(d.Outcome) > 4096 || strings.TrimSpace(d.AlertWhen) == "" || len(d.AlertWhen) > 2048 || strings.ContainsAny(d.Title, "\x00\r\n") || strings.ContainsRune(d.Outcome+d.AlertWhen, 0) {
		return errors.New("provide a title, desired outcome, and notification conditions")
	}
	if !filepath.IsAbs(d.SourcePath) || filepath.Clean(d.SourcePath) != d.SourcePath {
		return errors.New("select an absolute workspace folder")
	}
	if d.Mode != action.Inspect && d.Mode != action.Draft {
		return errors.New("unattended checks may read or prepare drafts only")
	}
	if _, err := jobs.WatchSchedule(d.IntervalMinutes); err != nil {
		return err
	}
	if d.MaxRequests < 1 || d.MaxRequests > 256 || d.MaxSeconds < 30 || d.MaxSeconds > 1800 || d.MaxTokens < 1000 || d.MaxTokens > 1000000 || d.MaxChecksPerDay < 1 || d.MaxChecksPerDay > 288 {
		return errors.New("check limits are outside the supported range")
	}
	if len(d.ConnectorIDs) > 4 || d.CalendarDays < 0 || d.CalendarDays > 30 || len(d.GmailQuery) > 1024 || strings.ContainsRune(d.GmailQuery, 0) {
		return errors.New("select at most four connectors and a calendar window of 0–30 days")
	}
	if len(d.ConnectorIDs) == 0 && (d.GmailQuery != "" || d.CalendarDays != 0) {
		return errors.New("select a Google connector before adding a mail query or calendar window")
	}
	return nil
}

func (d Draft) EndTime() (time.Time, error) {
	if d.EndsAt == "" {
		return time.Time{}, nil
	}
	end, err := time.Parse(time.RFC3339, d.EndsAt)
	if err != nil || end.IsZero() {
		return time.Time{}, errors.New("end date must include a time and timezone, such as 2026-10-30T18:00:00+08:00; leave blank for no end date")
	}
	return end, nil
}

func (d Draft) Definition(goalID string, sources []jobs.WatchSource) (jobs.Definition, error) {
	if err := d.Validate(); err != nil {
		return jobs.Definition{}, err
	}
	schedule, _ := jobs.WatchSchedule(d.IntervalMinutes)
	end, _ := d.EndTime()
	contract := artifact.InspectionContract()
	if d.Mode == action.Draft {
		contract = artifact.OptionalDraftContract()
	}
	id := d.JobID
	if id == "" {
		id = "job-" + d.ID
	}
	return jobs.Definition{ID: id, Name: d.Title, GoalID: goalID, EndsAt: end, OriginConversationID: d.OriginConversationID, Enabled: d.Enabled, GoalWatch: true, ChangeOnly: true,
		RefreshSnapshot: true, SourcePath: d.SourcePath, Objective: d.Outcome, AlertWhen: d.AlertWhen,
		Schedule: schedule, PollMinutes: d.IntervalMinutes, Timezone: "UTC", Missed: "run_once", Mode: d.Mode,
		Contract: contract, ConnectorIDs: append([]string(nil), d.ConnectorIDs...), WatchSources: sources,
		MaxSteps: 24, MaxChecksPerDay: d.MaxChecksPerDay, Limits: agent.Limits{ModelRequests: d.MaxRequests, WallSeconds: d.MaxSeconds, Tokens: d.MaxTokens}}, nil
}

func (d Draft) ReviewLines() []string {
	sources := d.SourcePath
	if len(d.ConnectorIDs) > 0 {
		sources += "; " + strings.Join(d.ConnectorIDs, ", ")
	}
	permission := "Read and recommend"
	if d.Mode == action.Draft {
		permission = "Read and prepare drafts for review"
	}
	lines := []string{"Outcome: " + d.Outcome, "Sources: " + sources, fmt.Sprintf("When: every %d minutes while this computer and the supervisor are running", d.IntervalMinutes), "Notify when: " + d.AlertWhen, "Permitted actions: " + permission, "Sending and publishing require a separate foreground approval.", fmt.Sprintf("Limits: %d Work checks/day; per try %d requests, %d tokens, %ds", d.MaxChecksPerDay, d.MaxRequests, d.MaxTokens, d.MaxSeconds), "Transient failures may retry up to 3 times per check. Cost estimate unavailable; usage is recorded."}
	if d.GmailQuery != "" {
		lines = append(lines, "Mail selection: "+d.GmailQuery)
	}
	if end, _ := d.EndTime(); !end.IsZero() {
		lines = append(lines, "Ends: "+end.Format("Mon 2 Jan 2006, 15:04 MST (Z07:00)"), "No new checks or retries start after the end date. A current check may finish.")
	} else {
		lines = append(lines, "Ends: When you pause this check")
	}
	if d.OriginConversationID != "" {
		lines = append(lines, "Updates return to the conversation where you requested this check.")
	}
	if d.CalendarDays > 0 {
		lines = append(lines, fmt.Sprintf("Calendar selection: next %d days", d.CalendarDays))
	}
	if !d.Enabled {
		lines = append(lines, "This check remains paused after saving. Resume it explicitly when ready.")
	}
	return lines
}

// FromDefinition preserves the exact agreement while a user edits a subset.
func FromDefinition(definition jobs.Definition) (Draft, error) {
	if !definition.GoalWatch {
		return Draft{}, errors.New("this schedule uses advanced settings; open Following to manage it")
	}
	draft, err := New(definition.SourcePath, definition.GoalID)
	if err != nil {
		return Draft{}, err
	}
	draft.Enabled = definition.Enabled
	draft.OriginConversationID = definition.OriginConversationID
	draft.JobID, draft.Title, draft.Outcome = definition.ID, definition.Name, definition.Objective
	if !definition.EndsAt.IsZero() {
		draft.EndsAt = definition.EndsAt.Format(time.RFC3339)
	}
	draft.ExpectedDigest, _ = definition.PolicyDigest()
	draft.ConnectorIDs, draft.AlertWhen, draft.Mode = append([]string(nil), definition.ConnectorIDs...), definition.AlertWhen, definition.Mode
	if draft.AlertWhen == "" {
		draft.AlertWhen = "Material changes and due steps requiring review"
	}
	if definition.PollMinutes != 0 {
		draft.IntervalMinutes = definition.PollMinutes
	}
	if definition.Limits.ModelRequests > 0 {
		draft.MaxRequests = definition.Limits.ModelRequests
	}
	if definition.Limits.WallSeconds > 0 {
		draft.MaxSeconds = definition.Limits.WallSeconds
	}
	if definition.Limits.Tokens > 0 {
		draft.MaxTokens = definition.Limits.Tokens
	}
	if definition.MaxChecksPerDay > 0 {
		draft.MaxChecksPerDay = definition.MaxChecksPerDay
	}
	for _, source := range definition.WatchSources {
		if source.Operation == "gmail_list" {
			draft.GmailQuery = source.Query
		}
		if source.Operation == "calendar_events" {
			draft.CalendarDays = source.Days
		}
	}
	return draft, nil
}
