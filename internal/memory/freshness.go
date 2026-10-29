package memory

import (
	"errors"
	"time"
)

// This is a conservative review policy, not a prediction that a fact changed.
// Stable identity/preferences do not expire by age. Explicit expiration and
// source fingerprints still apply independently. Unknown subjects are not
// classified by guessing at their text.
func reviewInterval(subject string) time.Duration {
	switch CanonicalSubject(subject) {
	case "profile.city", "profile.timezone", "profile.job", "profile.role", "profile.employer", "profile.workplace":
		return 90 * 24 * time.Hour
	}
	return 0
}

func (s Store) annotateFreshness(records []Record) {
	for i := range records {
		record := &records[i]
		interval := reviewInterval(record.Subject)
		if interval == 0 {
			continue
		}
		if !record.LastConfirmedAt.IsZero() {
			record.ReviewAfter = record.LastConfirmedAt.Add(interval)
		}
		if record.RecallIssue != "" {
			continue
		}
		switch {
		case record.LastConfirmedAt.IsZero():
			record.RecallIssue = "This changeable fact has no confirmation date. Review it before automatic recall."
		case record.LastConfirmedAt.After(s.clock().Add(24 * time.Hour)):
			record.RecallIssue = "This fact's confirmation is in the future. Check its evidence and the system clock before automatic recall."
		case !record.ReviewAfter.After(s.clock()):
			record.RecallIssue = "This changeable fact is due for confirmation (90-day review). It is kept for review and excluded from automatic recall."
		}
	}
}

// ConfirmReviewed refreshes exactly the revision the person inspected. It does
// not resolve conflicts, renew an explicit expiry, clear a forget instruction,
// or assert that a changed linked file still supports the stored text.
func (s Store) ConfirmReviewed(id, updated string) (Record, error) {
	if updated == "" {
		return Record{}, errors.New("inspect this memory and provide its updated timestamp before confirming")
	}
	release, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer release()
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if record.UpdatedAt.Format(time.RFC3339Nano) != updated {
		return Record{}, errors.New("this memory changed; inspect it again before confirming")
	}
	if record.SourceStatus == "changed" || record.SourceStatus == "unavailable" || !record.ExpiresAt.IsZero() && !record.ExpiresAt.After(s.clock()) {
		return Record{}, errors.New("this memory expired or its source changed; review and correct its source or expiry instead")
	}
	records, err := s.List(record.GoalID, true)
	if err != nil {
		return Record{}, err
	}
	// List includes freshness annotations; inspect concept state separately so
	// confirmation can clear age alone, never a collision or suppression.
	for i := range records {
		records[i].RecallIssue = ""
	}
	if err := s.annotateConcepts(records); err != nil {
		return Record{}, err
	}
	for _, current := range records {
		if current.ID == record.ID && current.RecallIssue != "" {
			return Record{}, errors.New(current.RecallIssue)
		}
	}
	record.UpdatedAt = s.nextUpdate(record.UpdatedAt)
	record.observe("user-confirmed", nil)
	return record, s.write(record, false)
}
