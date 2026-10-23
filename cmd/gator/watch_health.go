package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
)

// Health events have stable identities while the original attempts remain
// immutable. Repeated failures retain one chat incident and one OS notification.
func recordWatchHealthInbox(stateDir string, definition jobs.Definition, attempt jobs.Attempt) (inbox.Entry, error) {
	store, err := jobs.Open(stateDir)
	if err != nil {
		return inbox.Entry{}, err
	}
	history, err := store.History(definition.ID, 0)
	if err != nil {
		return inbox.Entry{}, err
	}
	health := jobs.ReduceWatchHealth(history, attempt)
	if health.First.ID == "" {
		return inbox.Entry{}, nil
	}
	mailbox, err := inbox.Open(stateDir)
	if err != nil {
		return inbox.Entry{}, err
	}
	first := health.First
	summary := "This check could not finish. " + first.Error
	if first.Status == "limited" {
		summary = "This check reached a request limit. " + first.Summary
	}
	entry, err := mailbox.Load(inbox.JobUpdateID(definition.ID, first.ID))
	if err == nil && (entry.JobID != definition.ID || entry.AttemptID != first.ID || entry.OriginConversationID != definition.OriginConversationID || entry.Status != first.Status) {
		return inbox.Entry{}, fmt.Errorf("health incident identity does not match its first attempt")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return inbox.Entry{}, err
	}
	if errors.Is(err, os.ErrNotExist) {
		entry, err = mailbox.Add(inbox.Entry{ID: inbox.JobUpdateID(definition.ID, first.ID), Kind: "job", Title: definition.Name, Summary: boundedJobError(summary), Status: first.Status, JobID: definition.ID, GoalID: definition.GoalID, OriginConversationID: definition.OriginConversationID, AttemptID: first.ID, ConversationID: first.Conversation, RevisionID: first.Revision, CreatedAt: first.FinishedAt, Urgent: first.Status != "limited"})
	}
	if err != nil {
		return inbox.Entry{}, err
	}
	if health.Recovered {
		return mailbox.Add(inbox.Entry{ID: inbox.JobUpdateID(definition.ID, "recovery-"+first.ID), Kind: "job_health", Title: definition.Name, Summary: fmt.Sprintf("Checks resumed after %d unsuccessful checks. This successful check does not establish what changed during the gap. Review the latest result and sources if the missed interval matters.", health.Attempts), Status: "recovered", JobID: definition.ID, GoalID: definition.GoalID, OriginConversationID: definition.OriginConversationID, AttemptID: attempt.ID, ConversationID: attempt.Conversation, RevisionID: attempt.Revision, CreatedAt: attempt.FinishedAt})
	}
	if first.ID != attempt.ID {
		return inbox.Entry{}, nil
	}
	return entry, nil
}

func claimJobNotification(stateDir, entryID string) bool {
	store, err := inbox.Open(stateDir)
	if err != nil {
		return false
	}
	claimed, err := store.ClaimDesktopNotification(entryID, time.Now())
	return err == nil && claimed
}
