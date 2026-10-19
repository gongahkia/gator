// Package followup defines the user's reviewed agreement for an ongoing check.
// Creating a draft has no side effects; only the application adapter saves it.
package followup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/jobs"
)

type Draft struct {
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

func (d Draft) Definition(goalID string, sources []jobs.WatchSource) (jobs.Definition, error) {
	if err := d.Validate(); err != nil {
		return jobs.Definition{}, err
	}
	schedule, _ := jobs.WatchSchedule(d.IntervalMinutes)
	contract := artifact.InspectionContract()
	if d.Mode == action.Draft {
		contract = artifact.OptionalDraftContract()
	}
	id := d.JobID
	if id == "" {
		id = "job-" + d.ID
	}
	return jobs.Definition{ID: id, Name: d.Title, GoalID: goalID, Enabled: d.Enabled, GoalWatch: true, ChangeOnly: true,
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
	if d.CalendarDays > 0 {
		lines = append(lines, fmt.Sprintf("Calendar selection: next %d days", d.CalendarDays))
	}
	if !d.Enabled {
		lines = append(lines, "This check remains paused after saving. Resume it explicitly when ready.")
	}
	return lines
}
