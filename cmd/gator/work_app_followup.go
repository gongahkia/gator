package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workhome"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

type followupChange struct {
	Action          string       `json:"action"`
	ID              string       `json:"id,omitempty"`
	ExpectedDigest  string       `json:"expected_digest,omitempty"`
	Title           *string      `json:"title,omitempty"`
	Outcome         *string      `json:"outcome,omitempty"`
	AlertWhen       *string      `json:"alert_when,omitempty"`
	EndsAt          *string      `json:"ends_at,omitempty"`
	IntervalMinutes *int         `json:"interval_minutes,omitempty"`
	ConnectorIDs    *[]string    `json:"connector_ids,omitempty"`
	GmailQuery      *string      `json:"gmail_query,omitempty"`
	CalendarDays    *int         `json:"calendar_days,omitempty"`
	Mode            *action.Mode `json:"mode,omitempty"`
	MaxChecksPerDay *int         `json:"max_checks_per_day,omitempty"`
	StartLocal      bool         `json:"start_local,omitempty"`
}

type followupView struct {
	ID        string    `json:"id"`
	Digest    string    `json:"expected_digest"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Issue     string    `json:"issue,omitempty"`
	NextCheck time.Time `json:"next_check,omitzero"`
	Agreement []string  `json:"agreement"`
}

func (c gatorAppController) followupConversation(id string) error {
	if c.request == nil || id == "" {
		return errors.New("a saved chat is required for follow-up updates")
	}
	sessions, err := worksession.Open(c.stateDir)
	if err != nil {
		return err
	}
	chat, err := sessions.Load(id)
	if err != nil {
		return err
	}
	if filepath.Clean(chat.SourcePath) != filepath.Clean(c.request.SourcePath) {
		return errors.New("this chat's folder changed; open the requesting chat again")
	}
	return nil
}

func (c gatorAppController) inspectFollowups(query workrun.AppQuery) (any, error) {
	if err := c.followupConversation(query.ConversationID); err != nil {
		return nil, err
	}
	checks, online, err := readFollowupChecks(c.stateDir)
	if err != nil {
		return nil, err
	}
	views := []followupView{}
	total := 0
	for _, check := range checks {
		if check.Definition.OriginConversationID != query.ConversationID || !check.Definition.GoalWatch {
			continue
		}
		if query.ID != "" && check.ID != query.ID {
			continue
		}
		total++
		if len(views) == 50 {
			continue
		}
		views = append(views, projectFollowup(check))
	}
	if query.ID != "" && len(views) == 0 {
		return nil, errors.New("that check does not belong to this chat; open its original conversation or manage it in Following")
	}
	return map[string]any{"checks": views, "total": total, "supervisor_running": online, "host": "this computer", "note": "Only this chat's responsibilities are listed. Use the exact id and expected_digest for edits. Next check is a schedule, not a guarantee when this computer is asleep or off."}, nil
}

func projectFollowup(check workhome.Check) followupView {
	draft, _ := followup.FromDefinition(check.Definition)
	digest, _ := check.Definition.PolicyDigest()
	return followupView{ID: check.ID, Digest: digest, Title: check.Title, State: check.State, Issue: check.Issue, NextCheck: check.NextDue, Agreement: draft.ReviewLines()}
}

func (c gatorAppController) prepareFollowup(ctx context.Context, change workrun.AppChange) (workrun.AppMutation, error) {
	if err := ctx.Err(); err != nil {
		return workrun.AppMutation{}, err
	}
	if err := c.followupConversation(change.ConversationID); err != nil {
		return workrun.AppMutation{}, err
	}
	var v followupChange
	if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
		return workrun.AppMutation{}, err
	}
	if !slices.Contains([]string{"create", "edit", "pause", "resume"}, v.Action) {
		return workrun.AppMutation{}, errors.New("follow-up action must be create, edit, pause or resume")
	}
	store, err := jobs.Open(c.stateDir)
	if err != nil {
		return workrun.AppMutation{}, err
	}
	var draft followup.Draft
	var prior jobs.Definition
	if v.Action == "create" {
		if v.ID != "" || v.ExpectedDigest != "" {
			return workrun.AppMutation{}, errors.New("new checks cannot replace an existing identity")
		}
		if v.Title == nil || v.Outcome == nil || v.AlertWhen == nil || v.EndsAt == nil || v.IntervalMinutes == nil {
			return workrun.AppMutation{}, errors.New("agree on title, outcome, notification conditions, check frequency and end date (empty for no end date) before creating a check")
		}
		draft, err = followup.New(c.request.SourcePath, "")
		draft.OriginConversationID = change.ConversationID
	} else {
		prior, err = store.Load(v.ID)
		if err == nil && prior.OriginConversationID != change.ConversationID {
			err = errors.New("this check belongs to another chat; open its original conversation")
		}
		if err == nil {
			digest, _ := prior.PolicyDigest()
			if v.ExpectedDigest == "" || v.ExpectedDigest != digest {
				err = errors.New("this check changed; inspect followups and review the current agreement")
			}
		}
		if err == nil {
			draft, err = followup.FromDefinition(prior)
		}
	}
	if err != nil {
		return workrun.AppMutation{}, err
	}
	if v.Action == "pause" || v.Action == "resume" {
		if v.Title != nil || v.Outcome != nil || v.AlertWhen != nil || v.EndsAt != nil || v.IntervalMinutes != nil || v.ConnectorIDs != nil || v.GmailQuery != nil || v.CalendarDays != nil || v.Mode != nil || v.MaxChecksPerDay != nil || v.StartLocal && v.Action == "pause" {
			return workrun.AppMutation{}, errors.New("pause/resume only changes whether this check is enabled; use edit for agreement changes")
		}
		draft.Enabled = v.Action == "resume"
	}
	if v.Title != nil {
		draft.Title = strings.TrimSpace(*v.Title)
	}
	if v.Outcome != nil {
		draft.Outcome = strings.TrimSpace(*v.Outcome)
	}
	if v.AlertWhen != nil {
		draft.AlertWhen = strings.TrimSpace(*v.AlertWhen)
	}
	if v.EndsAt != nil {
		draft.EndsAt = *v.EndsAt
	}
	if v.IntervalMinutes != nil {
		draft.IntervalMinutes = *v.IntervalMinutes
	}
	if v.ConnectorIDs != nil {
		draft.ConnectorIDs = append([]string{}, (*v.ConnectorIDs)...)
	}
	if v.GmailQuery != nil {
		draft.GmailQuery = *v.GmailQuery
	}
	if v.CalendarDays != nil {
		draft.CalendarDays = *v.CalendarDays
	}
	if v.Mode != nil {
		draft.Mode = *v.Mode
	}
	if v.MaxChecksPerDay != nil {
		draft.MaxChecksPerDay = *v.MaxChecksPerDay
	}
	if err := draft.Validate(); err != nil {
		return workrun.AppMutation{}, err
	}
	if end, _ := draft.EndTime(); draft.Enabled && !end.IsZero() && !end.After(time.Now()) {
		return workrun.AppMutation{}, errors.New("this check has ended; edit a future end date before resuming")
	}
	if v.StartLocal && !draft.Enabled {
		return workrun.AppMutation{}, errors.New("this check is paused; resume it before starting the background engine")
	}
	// New sources must already be selected for this foreground request. Existing
	// reviewed sources can be retained while editing timing or notification text.
	for _, id := range draft.ConnectorIDs {
		if !slices.Contains(c.request.ConnectorIDs, id) && !slices.Contains(prior.ConnectorIDs, id) {
			return workrun.AppMutation{}, fmt.Errorf("select connection %s in this chat before including it in a follow-up", id)
		}
	}
	authority := ""
	if v.Action != "pause" {
		_, _, authority, err = followupSources(draft)
		if err != nil {
			return workrun.AppMutation{}, err
		}
	}
	_, online, err := readFollowupChecks(c.stateDir)
	if err != nil {
		return workrun.AppMutation{}, err
	}
	lines := draft.ReviewLines()
	availability := "Offline — saved checks wait until the local background engine runs."
	if online {
		availability = "Local background engine is running; this computer must remain on."
	}
	lines = append(lines, "Availability: "+availability)
	if v.StartLocal {
		lines = append(lines, "Background engine: Start it after saving. This also runs other enabled checks in this Gator profile. It does not enable start at login.")
	}
	if v.Action == "pause" {
		lines = append(lines, "Pausing stops future checks; a current check may finish.")
	}
	summary := "Follow-up saved: " + draft.Title + ". Open Following for its current availability."
	if v.Action == "pause" {
		summary = "Paused follow-up: " + draft.Title + ". A current check may finish."
	}
	return workrun.AppMutation{Review: true, Summary: summary, Preview: workrun.AppAgreement{Title: draft.Title, Action: v.Action, Lines: lines}, Apply: func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := c.followupConversation(change.ConversationID); err != nil {
			return nil, err
		}
		var definition jobs.Definition
		if v.Action == "pause" {
			prior.Enabled = false
			definition, err = store.SaveReviewed(prior, v.ExpectedDigest)
		} else {
			_, definition, err = saveFollowupReviewed(c.stateDir, draft, authority)
		}
		if err != nil {
			return nil, err
		}
		startIssue := ""
		if v.StartLocal {
			start := c.startSupervisor
			if start == nil {
				start = startLocalSupervisor
			}
			if err := start(c.stateDir); err != nil {
				startIssue = "Saved, but the background engine could not start: " + err.Error()
			}
		}
		checks, online, err := readFollowupChecks(c.stateDir)
		if err != nil {
			return map[string]any{"saved": true, "id": definition.ID, "availability": "unknown", "issue": err.Error()}, nil
		}
		for _, check := range checks {
			if check.ID == definition.ID {
				return map[string]any{"saved": true, "check": projectFollowup(check), "supervisor_running": online, "issue": startIssue}, nil
			}
		}
		return map[string]any{"saved": true, "id": definition.ID, "availability": "unknown", "issue": "Saved check is unavailable; refresh Following."}, nil
	}}, nil
}
