package memory

import (
	"errors"
	"os"
)

// These are deliberately narrow, documented label aliases. They do not claim
// to resolve arbitrary paraphrases, infer ownership or merge unrelated facts.
var conceptAliases = map[string]string{
	"preference.length":       "preference.response_length",
	"preference.reply_length": "preference.response_length",
	"profile.home_city":       "profile.city",
	"profile.residence_city":  "profile.city",
}

func CanonicalSubject(subject string) string {
	if canonical, ok := conceptAliases[subject]; ok {
		return canonical
	}
	return subject
}

// Old tombstones used the model's literal label. Check those as well as the
// canonical label so upgrading cannot resurrect previously forgotten memory.
func (s Store) blockedSubject(goalID, subject string) (bool, error) {
	canonical := CanonicalSubject(subject)
	labels := []string{canonical}
	for alias, target := range conceptAliases {
		if target == canonical {
			labels = append(labels, alias)
		}
	}
	for _, label := range labels {
		if _, err := os.Lstat(s.subjectPath(goalID, label)); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func (s Store) unblockSubject(goalID, subject string) error {
	canonical := CanonicalSubject(subject)
	labels := []string{canonical}
	for alias, target := range conceptAliases {
		if target == canonical {
			labels = append(labels, alias)
		}
	}
	for _, label := range labels {
		if err := os.Remove(s.subjectPath(goalID, label)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s Store) annotateConcepts(records []Record) error {
	groups := map[string][]int{}
	for i, record := range records {
		if record.Subject == "" {
			continue
		}
		blocked, err := s.blockedSubject(record.GoalID, record.Subject)
		if err != nil {
			return err
		}
		if blocked {
			records[i].RecallIssue = "This concept was forgotten; excluded from automatic recall."
		}
		if !record.ExpiresAt.IsZero() && !record.ExpiresAt.After(s.clock()) {
			continue
		}
		key := record.GoalID + "\x00" + CanonicalSubject(record.Subject)
		groups[key] = append(groups[key], i)
	}
	for _, indices := range groups {
		if len(indices) < 2 {
			continue
		}
		for _, i := range indices {
			if records[i].RecallIssue == "" {
				records[i].RecallIssue = "Multiple records share this concept; reconcile them before automatic recall."
			}
		}
	}
	return nil
}

func (r Record) Recallable() bool {
	return r.RecallIssue == "" && r.SourceStatus != "changed" && r.SourceStatus != "unavailable"
}

// Inspect includes current concept annotations for a direct ID lookup. Load is
// the storage primitive used inside mutations; inspecting never rewrites files.
func (s Store) Inspect(id string) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	records, err := s.List(record.GoalID, true)
	if err != nil && ReadWarning(err) == "" {
		return Record{}, err
	}
	for _, current := range records {
		if current.ID == id {
			return current, nil
		}
	}
	return Record{}, os.ErrNotExist
}
