package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/hostproc"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workhome"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/workspace"
)

func saveFollowup(stateDir string, draft followup.Draft) (goal.Record, jobs.Definition, error) {
	release, guardErr := state.Guard(stateDir, false)
	if guardErr != nil {
		return goal.Record{}, jobs.Definition{}, guardErr
	}
	defer release()
	if err := draft.Validate(); err != nil {
		return goal.Record{}, jobs.Definition{}, err
	}
	if draft.OriginConversationID != "" {
		sessions, err := worksession.Open(stateDir)
		if err != nil {
			return goal.Record{}, jobs.Definition{}, err
		}
		if _, err := sessions.Load(draft.OriginConversationID); err != nil {
			return goal.Record{}, jobs.Definition{}, fmt.Errorf("the requesting conversation is unavailable: %w", err)
		}
	}
	root, err := workspace.Open(draft.SourcePath)
	if err != nil {
		return goal.Record{}, jobs.Definition{}, err
	}
	draft.SourcePath = root.Path()
	settings, err := loadSettings()
	if err != nil {
		return goal.Record{}, jobs.Definition{}, err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return goal.Record{}, jobs.Definition{}, err
	}
	var sources []jobs.WatchSource
	hasGoogle := false
	seen := map[string]bool{}
	for _, id := range draft.ConnectorIDs {
		if seen[id] {
			return goal.Record{}, jobs.Definition{}, errors.New("a connected source is selected twice")
		}
		seen[id] = true
		descriptor, found := registry.Get(id)
		if !found {
			return goal.Record{}, jobs.Definition{}, fmt.Errorf("source %s is no longer configured", id)
		}
		switch descriptor.Kind {
		case connector.KindHTTPJSON:
			sources = append(sources, jobs.WatchSource{ConnectorID: id, Operation: "fetch"})
		case connector.KindGoogle:
			hasGoogle = true
			if draft.GmailQuery == "" && draft.CalendarDays == 0 {
				return goal.Record{}, jobs.Definition{}, errors.New("select a mail query or calendar window for Google")
			}
			if draft.GmailQuery != "" {
				sources = append(sources, jobs.WatchSource{ConnectorID: id, Operation: "gmail_list", Query: draft.GmailQuery})
			}
			if draft.CalendarDays > 0 {
				sources = append(sources, jobs.WatchSource{ConnectorID: id, Operation: "calendar_events", Days: draft.CalendarDays})
			}
		default:
			return goal.Record{}, jobs.Definition{}, fmt.Errorf("%s has no bounded follow-up reader", id)
		}
	}
	if !hasGoogle && (draft.GmailQuery != "" || draft.CalendarDays != 0) {
		return goal.Record{}, jobs.Definition{}, errors.New("mail and calendar selections require a Google connector")
	}
	if len(sources) > 4 {
		return goal.Record{}, jobs.Definition{}, errors.New("select at most four connected read surfaces")
	}
	permissions := connector.PermissionSet(settings.ConnectorPermissions)
	for _, source := range sources {
		operations := []string{source.Operation}
		if source.Operation == "gmail_list" {
			operations = append(operations, "gmail_get")
		}
		for _, name := range operations {
			_, operation, err := registry.Operation(source.ConnectorID, name)
			if err != nil {
				return goal.Record{}, jobs.Definition{}, err
			}
			if permissions.Resolve(source.ConnectorID, name, operation.Capability) != connector.PermissionAllow {
				return goal.Record{}, jobs.Definition{}, fmt.Errorf("%s %s needs unattended read permission; configure /connector first", source.ConnectorID, name)
			}
		}
	}
	pins, err := captureConfiguredGoalOAuthAccountPins(context.Background(), draft.ConnectorIDs)
	if err != nil {
		return goal.Record{}, jobs.Definition{}, err
	}
	goals, err := goal.Open(stateDir)
	if err != nil {
		return goal.Record{}, jobs.Definition{}, err
	}
	store, err := jobs.Open(stateDir)
	if err != nil {
		return goal.Record{}, jobs.Definition{}, err
	}
	goalID := draft.GoalID
	if goalID == "" {
		goalID = "goal-" + draft.ID
	}
	definitions, err := store.List()
	if err != nil {
		return goal.Record{}, jobs.Definition{}, err
	}
	for _, definition := range definitions {
		if definition.GoalID == goalID && definition.GoalWatch && definition.Enabled && definition.ID != draft.JobID && definition.ID != "job-"+draft.ID {
			return goal.Record{}, jobs.Definition{}, fmt.Errorf("this goal already has a check; edit %s from /followup", definition.ID)
		}
	}
	var record goal.Record
	if draft.GoalID == "" {
		record, err = goals.AddWithID(goalID, draft.Title, draft.Outcome, draft.SourcePath)
	} else {
		record, err = goals.Load(goalID)
	}
	if err != nil {
		return record, jobs.Definition{}, err
	}
	if record.Status != goal.Active || record.SourcePath != draft.SourcePath {
		return record, jobs.Definition{}, errors.New("select the active goal's workspace before editing its check")
	}
	definition, err := draft.Definition(goalID, sources)
	if err != nil {
		return record, definition, err
	}
	if draft.JobID != "" {
		prior, loadErr := store.Load(draft.JobID)
		if loadErr != nil {
			return record, definition, loadErr
		}
		digest, _ := prior.PolicyDigest()
		if draft.ExpectedDigest == "" || digest != draft.ExpectedDigest || prior.GoalID != goalID || prior.OriginConversationID != draft.OriginConversationID {
			return record, definition, errors.New("this check changed while you edited it; refresh and review it again")
		}
		definition.CreatedAt, definition.LastScheduledAt = prior.CreatedAt, prior.LastScheduledAt
	}
	definition.OAuthAccountPins = pins
	if draft.JobID != "" {
		definition, err = store.SaveReviewed(definition, draft.ExpectedDigest)
	} else {
		prior, loadErr := store.Load(definition.ID)
		if loadErr == nil {
			definition.Version, definition.CreatedAt = prior.Version, prior.CreatedAt
			wanted, _ := definition.PolicyDigest()
			actual, _ := prior.PolicyDigest()
			if wanted != actual {
				return record, definition, errors.New("this saved check has changed; refresh and edit it from the follow-up list")
			}
			definition = prior
		} else if errors.Is(loadErr, os.ErrNotExist) {
			definition, err = store.Save(definition)
		} else {
			err = loadErr
		}
	}
	if err != nil {
		return record, definition, err
	}
	if draft.GoalID == "" {
		_, err = goals.InitializePlan(goalID, draft.Outcome, []string{"Review a meaningful project change and choose the next useful step"})
	}
	return record, definition, err
}

func readFollowupChecks(stateDir string) ([]workhome.Check, bool, error) {
	running := jobSupervisorRequestAt(stateDir, "health", io.Discard) == nil
	checks, err := workhome.ReadChecks(stateDir, running, time.Now())
	return checks, running, err
}

func startLocalSupervisor(stateDir string) error {
	if jobSupervisorRequestAt(stateDir, "health", io.Discard) == nil {
		return nil
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	root := filepath.Join(stateDir, "gator", "jobs")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(root, "supervisor.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	if err := log.Chmod(0600); err != nil {
		return err
	}
	command := hostproc.Command(binary, "job", "supervisor")
	command.Env = append(os.Environ(), "GATOR_STATE_DIR="+stateDir)
	command.Stdout, command.Stderr = log, log
	configureDetached(command, true)
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	for attempt := 0; attempt < 20; attempt++ {
		if jobSupervisorRequestAt(stateDir, "health", io.Discard) == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("supervisor did not become reachable; inspect the local supervisor log or run gator job supervisor")
}

func followupObjective(definition jobs.Definition) string {
	if strings.TrimSpace(definition.AlertWhen) == "" {
		return ""
	}
	return "\nUser's notification conditions: " + definition.AlertWhen + "\nEvaluate materiality against these conditions; a failed read is not evidence of no change."
}
